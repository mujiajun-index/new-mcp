package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

// 管理端市场详情(stdio 条目)的进程视图与启停控制。共享条目(isolated_process=false)
// = 平台唯一子进程,会话池按条目键控(bridge.sessionKey{itemID}),全部安装用户复用;
// 独占条目 = 每个安装用户的引用行各自的子进程(行键控)。口径与自有服务详情的
// GetProcessStat/ControlProcess 一致(整棵进程树、完整终止序列),但目标是其他用户的
// 引用行,不做行归属校验(管理员运维视角,路由已在 admin 组)。

// GetProcessStat 条目进程视图:共享=Shared 单进程快照;独占=Instances 按安装引用行
// 逐行枚举(DB 为准,含从未连接的安装,未运行恒 Running:false 固定形态渲染)。
// 只读池内现状,绝不触发连接(看详情不拉起进程);全部实例合并为一次系统进程扫描。
func (s *MarketplaceService) GetProcessStat(itemID int64) (*dto.MarketplaceItemProcess, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	out := &dto.MarketplaceItemProcess{Isolated: item.IsolatedProcess}
	if item.TransportType != string(transport.TypeStdio) || SessionPool == nil {
		return out, nil
	}

	if !item.IsolatedProcess {
		// 共享:条目键控的唯一平台会话
		stat := &dto.ServiceProcessStat{}
		if sess := SessionPool.GetByItem(itemID); sess != nil && sess.Adapter.IsConnected() {
			if proc := sess.Adapter.GetStdioProcess(); proc != nil {
				fillProcessStat(stat, transport.CollectProcessTreeStat(proc.PID, proc.Command))
			}
		}
		out.Shared = stat
		return out, nil
	}

	// 独占:枚举该条目全部安装引用行,池内会话按行 ID 对齐
	rows, err := model.ListServiceRowsByMarketplaceItem(itemID)
	if err != nil {
		return nil, err
	}
	sessions := map[int64]*bridge.McpSession{}
	for _, sess := range SessionPool.GetSessionsByItem(itemID) {
		sessions[sess.ServiceID] = sess
	}
	var roots []transport.ProcessRoot
	for i := range rows {
		sess := sessions[rows[i].ID]
		if sess == nil || !sess.Adapter.IsConnected() {
			continue
		}
		if proc := sess.Adapter.GetStdioProcess(); proc != nil {
			roots = append(roots, transport.ProcessRoot{Key: rows[i].ID, RootPID: proc.PID, Command: proc.Command})
		}
	}
	treeStats := transport.CollectProcessTreesStat(roots)

	usernames := usernamesOfRows(rows)
	out.Instances = make([]dto.MarketplaceItemProcessInstance, 0, len(rows))
	for i := range rows {
		inst := dto.MarketplaceItemProcessInstance{
			ServiceID: rows[i].ID,
			UserID:    rows[i].UserID,
			Username:  usernames[rows[i].UserID],
			Name:      rows[i].DisplayName,
			Status:    rows[i].Status,
		}
		if inst.Name == "" {
			inst.Name = rows[i].Name
		}
		if tree := treeStats[rows[i].ID]; tree != nil {
			fillProcessStat(&inst.Stat, tree)
		}
		out.Instances = append(out.Instances, inst)
	}
	return out, nil
}

// ControlProcess 条目进程启停。共享模式操作平台唯一进程——start 即预热(不等首个
// 用户调用即拉起);独占模式按安装引用行逐个操作(引用行须属于该条目且已启用)。
// 停止走完整终止序列;启动/重启同步等待握手完成(npx/uvx 首拉可能下载依赖,
// 60s 超时与自有服务 ControlProcess 口径一致)。返回操作后的最新进程视图。
func (s *MarketplaceService) ControlProcess(itemID int64, req *dto.MarketplaceProcessControlReq) (*dto.MarketplaceItemProcess, error) {
	item, err := model.GetMarketplaceItemByID(itemID)
	if err != nil {
		return nil, err
	}
	if item.TransportType != string(transport.TypeStdio) {
		return nil, fmt.Errorf("仅 stdio 条目支持进程操作")
	}
	if item.Status != common.StatusEnabled {
		return nil, fmt.Errorf("市场项已下架,请先启用再操作进程")
	}
	if SessionPool == nil {
		return nil, fmt.Errorf("会话池未初始化")
	}

	if req.Action == "stop" || req.Action == "restart" {
		if item.IsolatedProcess {
			if _, mErr := itemRefService(itemID, req.ServiceID); mErr != nil {
				return nil, mErr
			}
			SessionPool.Remove(req.ServiceID)
			// 进程已停,库里的 healthy 是过期标记,还原为未知(与自有服务停止同口径)
			model.DB.Model(&model.McpService{}).Where("id = ?", req.ServiceID).
				Update("health_status", common.HealthUnknown)
		} else {
			// 共享:条目键控会话(共享模式下该条目在池内只有这一类会话)
			SessionPool.RemoveByMarketplaceItem(itemID)
		}
		if req.Action == "stop" {
			return s.GetProcessStat(itemID)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if item.IsolatedProcess {
		// 独占:物化目标引用行后按行键控入池
		svc, mErr := itemRefService(itemID, req.ServiceID)
		if mErr != nil {
			return nil, mErr
		}
		if svc.Status != common.StatusEnabled {
			return nil, fmt.Errorf("该安装服务已被用户禁用,无法拉起进程")
		}
		if mErr = (&McpServiceService{}).materializeMarketplace(svc); mErr != nil {
			return nil, mErr
		}
		if _, cErr := SessionPool.GetOrConnect(ctx, svc); cErr != nil {
			return nil, cErr
		}
	} else {
		// 共享进程预热:条目配置物化内存行(ID=0 不落库),SharedProcess 使池按条目键控
		if _, cErr := SessionPool.GetOrConnect(ctx, buildItemPrewarmService(item)); cErr != nil {
			return nil, cErr
		}
	}
	return s.GetProcessStat(itemID)
}

// itemRefService 校验 serviceID 是 itemID 的安装引用行并返回该行(管理员操作的是
// 其他用户的引用行,不走 GetServiceByID 的属主校验)。
func itemRefService(itemID, serviceID int64) (*model.McpService, error) {
	if serviceID <= 0 {
		return nil, fmt.Errorf("独占模式需指定要操作的安装服务")
	}
	svc, err := model.GetServiceByIDWithoutUser(serviceID)
	if err != nil || svc.Source != "marketplace" || svc.MarketplaceItemID == nil || *svc.MarketplaceItemID != itemID {
		return nil, fmt.Errorf("安装服务不属于该市场项")
	}
	return svc, nil
}

// buildItemPrewarmService 从市场项物化共享进程预热的内存服务行:解密 config、还原
// 真实 transport、标记 SharedProcess(会话池按条目键控)。ID=0:纯内存行,不落库。
func buildItemPrewarmService(item *model.MarketplaceItem) *model.McpService {
	config := item.ConfigTemplate
	if plain, dErr := common.Decrypt(item.ConfigTemplate); dErr == nil && plain != "" {
		config = plain
	}
	return &model.McpService{
		Name:              item.Name,
		TransportType:     item.TransportType,
		Config:            config,
		Source:            "marketplace",
		MarketplaceItemID: &item.ID,
		SharedProcess:     true,
	}
}

// fillProcessStat 把进程树采集结果拷进 DTO 快照。
func fillProcessStat(dst *dto.ServiceProcessStat, tree *transport.ProcessTreeStat) {
	if tree == nil {
		return
	}
	dst.Running = tree.Running
	dst.PID = tree.PID
	dst.Command = tree.Command
	dst.ProcessCount = tree.ProcessCount
	dst.MemoryRSS = tree.RSSBytes
	dst.MemoryVMS = tree.VMSBytes
	dst.CPUPercent = tree.CPUPercent
	dst.UptimeSeconds = tree.UptimeSeconds
}

// usernamesOfRows 引用行涉及的 user_id → username 批量映射(一次查询,查不到的留空)。
func usernamesOfRows(rows []model.ServiceRowRef) map[int64]string {
	out := map[int64]string{}
	ids := make([]int64, 0, len(rows))
	seen := map[int64]bool{}
	for _, r := range rows {
		if !seen[r.UserID] {
			seen[r.UserID] = true
			ids = append(ids, r.UserID)
		}
	}
	if len(ids) == 0 {
		return out
	}
	var users []struct {
		ID       int64
		Username string
	}
	if err := model.DB.Model(&model.User{}).Where("id IN ?", ids).
		Select("id, username").Find(&users).Error; err != nil {
		return out
	}
	for _, u := range users {
		out[u.ID] = u.Username
	}
	return out
}
