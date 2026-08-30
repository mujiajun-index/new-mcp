package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/camera"
	"github.com/mujkjk/newmcp/internal/mcp/installer"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"github.com/shirou/gopsutil/v4/mem"
)

var SessionPool *bridge.SessionPool
var VirtualRegistry *virtual.VirtualToolRegistry
var CameraStreamMgr *camera.CameraStreamManager

// manualBillingService 市场服务手动测试计费用的计费服务实例(无状态,包级复用)。
var manualBillingService = billing.NewBillingService()

type McpServiceService struct{}

func (s *McpServiceService) List(userID int64, page, pageSize int, filters map[string]string) ([]dto.ServiceListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	services, total, err := model.ListServicesByUser(userID, offset, pageSize, filters)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.ServiceListItem, len(services))
	for i, svc := range services {
		var tools []interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

		items[i] = dto.ServiceListItem{
			ID:            svc.ID,
			Name:          svc.Name,
			DisplayName:   svc.DisplayName,
			Description:   svc.Description,
			TransportType: svc.TransportType,
			Source:        svc.Source,
			HealthStatus:  svc.HealthStatus,
			ToolsCount:    len(tools),
			Status:        svc.Status,
			CreatedAt:     svc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}

func (s *McpServiceService) Create(userID int64, req *dto.CreateServiceReq) (*dto.ServiceDetail, error) {
	// 纯市场模式:禁止用户添加自有服务(§7.5)。市场引用服务经 /marketplace/:id/add 添加,不受此限。
	if !model.GetOptionBool("UserOwnedServicesEnabled") {
		return nil, fmt.Errorf("当前为纯市场模式,不允许添加自有服务")
	}
	// 检查同名服务是否已存在
	var count int64
	model.DB.Model(&model.McpService{}).Where("user_id = ? AND name = ?", userID, req.Name).Count(&count)
	if count > 0 {
		return nil, fmt.Errorf("服务名称 %q 已存在", req.Name)
	}

	configJSON, _ := json.Marshal(req.Config)
	authConfigJSON, _ := json.Marshal(req.AuthConfig)
	tags := strings.Join(req.Tags, ",")

	svc := &model.McpService{
		UserID:        userID,
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		TransportType: req.TransportType,
		Config:        string(configJSON),
		AuthType:      req.AuthType,
		AuthConfig:    string(authConfigJSON),
		Tags:          tags,
		Status:        common.StatusEnabled,
		HealthStatus:  common.HealthUnknown,
	}
	if svc.AuthType == "" {
		svc.AuthType = "none"
	}

	if err := svc.Insert(); err != nil {
		return nil, err
	}

	// 异步加载工具
	if SessionPool != nil {
		go func() {
			SessionPool.GetOrConnect(context.Background(), svc)
		}()
	}

	return s.toDetail(svc), nil
}

// TestConnection 测试连接但不创建服务，用于注册前的预验证
func (s *McpServiceService) TestConnection(req *dto.TestConnectionReq) (*dto.TestResult, error) {
	configJSON, _ := json.Marshal(req.Config)

	// 构造临时 McpService 用于创建 adapter
	svc := &model.McpService{
		ID:            -1,
		TransportType: req.TransportType,
		Config:        string(configJSON),
	}

	adapter := bridge.CreateAdapter(svc)
	if adapter == nil {
		return &dto.TestResult{Connected: false, Error: "不支持的传输类型"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	if err := adapter.Connect(ctx); err != nil {
		return &dto.TestResult{
			Connected: false,
			Error:     err.Error(),
			LatencyMs: time.Since(start).Milliseconds(),
		}, nil
	}
	defer adapter.Close()

	tools := adapter.GetTools()

	return &dto.TestResult{
		Connected:       true,
		ToolsCount:      len(tools),
		LatencyMs:       time.Since(start).Milliseconds(),
		ProtocolVersion: adapter.GetProtocolVersion(),
		ServerInfo:      handshakeInfoMap(adapter),
	}, nil
}

// handshakeInfoMap 从 adapter 握手结果取上游 serverInfo(name/version);未拿到时为 nil。
func handshakeInfoMap(adapter transport.TransportAdapter) map[string]interface{} {
	si := adapter.GetServerInfo()
	if si == nil {
		return nil
	}
	return map[string]interface{}{"name": si.Name, "version": si.Version}
}

// PrepareStdio runs the pre-flight detect/install for a stdio service
// (npx/uvx runtime + package prefetch, plus mirror injection). No DB access.
func (s *McpServiceService) PrepareStdio(req *dto.PrepareStdioReq) (*dto.PrepareStdioResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	res, err := installer.Prepare(ctx, &installer.PrepareReq{
		Command:  req.Command,
		Args:     req.Args,
		Env:      req.Env,
		Registry: req.Registry,
	})
	if err != nil {
		return nil, err
	}
	return &dto.PrepareStdioResult{
		Branch:       string(res.Branch),
		RuntimeFound: res.RuntimeFound,
		RuntimePath:  res.RuntimePath,
		DidInstall:   res.DidInstall,
		Installed:    res.Installed,
		PackageName:  res.PackageName,
		RegistryEnv:  res.RegistryEnv,
		Stdout:       res.Stdout,
		Stderr:       res.Stderr,
		DurationMs:   res.DurationMs,
		Message:      res.Message,
	}, nil
}

func (s *McpServiceService) GetByID(userID, serviceID int64) (*dto.ServiceDetail, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	return s.toDetail(svc), nil
}

func (s *McpServiceService) Update(userID, serviceID int64, req *dto.UpdateServiceReq) error {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return err
	}
	if req.DisplayName != nil {
		svc.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		svc.Description = *req.Description
	}
	if req.Config != nil {
		configJSON, _ := json.Marshal(req.Config)
		svc.Config = string(configJSON)
	}
	if req.AuthType != nil {
		svc.AuthType = *req.AuthType
	}
	if req.AuthConfig != nil {
		authConfigJSON, _ := json.Marshal(req.AuthConfig)
		svc.AuthConfig = string(authConfigJSON)
	}
	if req.Tags != nil {
		svc.Tags = strings.Join(req.Tags, ",")
	}
	if req.Status != nil {
		svc.Status = *req.Status
	}
	if err := svc.Update(); err != nil {
		return err
	}
	// 禁用服务（status 置为非启用值）时，从全部分组中移除该服务及其工具配置。
	// 分组聚合工具时只看 group_service.enabled，不检查 service.status，仅置 0 无法隐藏，
	// 必须删除 mcp_group_services / mcp_group_tools 中的关联行，否则分组仍会暴露已停用的服务。
	if req.Status != nil && *req.Status != common.StatusEnabled {
		if err := model.DeleteGroupServicesByServiceID(serviceID); err != nil {
			return err
		}
		if err := model.DeleteGroupToolsByServiceID(serviceID); err != nil {
			return err
		}
		if err := model.DeleteGroupItemsByServiceID(serviceID); err != nil {
			return err
		}
		// 禁用即停:踢掉池内会话,stdio 子进程走完整终止序列(关 stdin → SIGTERM →
		// SIGKILL → Wait)立即释放内存;重新启用后按需懒连接,不自动拉起。
		if SessionPool != nil {
			SessionPool.Remove(serviceID)
		}
	}
	// 配置（command/args/env/registry/url/headers）变更后，运行中的连接/子进程仍带旧配置。
	// 踢掉旧 session 并按新配置异步重连（与 Create 一致）：让新 env 立即生效，同时刷新
	// tools_cache 并预热连接。AuthConfig 不喂给 adapter、DisplayName/Description/Tags 为展示字段，均无需重连。
	// 同请求里一并禁用的服务不重连——禁用即停,重连会把刚停的进程又拉起来。
	if req.Config != nil && SessionPool != nil {
		SessionPool.Remove(serviceID)
		if req.Status == nil || *req.Status == common.StatusEnabled {
			go SessionPool.GetOrConnect(context.Background(), svc)
		}
	}
	return nil
}

func (s *McpServiceService) Delete(userID, serviceID int64) error {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return err
	}
	if svc.TransportType == "virtual" {
		sourceLabel := map[string]string{"vision": "视觉", "camera": "摄像头"}[svc.Source]
		if sourceLabel == "" {
			sourceLabel = "对应"
		}
		return fmt.Errorf("虚拟服务无法直接删除，请在%s配置页面操作", sourceLabel)
	}
	// 先清理该服务在全部分组中的关联与工具配置，再硬删除服务本身，
	// 避免留下指向已删除服务的孤儿关联行（分组聚合时会逐条查服务，孤儿行会刷 record not found 日志）。
	if err := model.DeleteGroupServicesByServiceID(serviceID); err != nil {
		return err
	}
	if err := model.DeleteGroupToolsByServiceID(serviceID); err != nil {
		return err
	}
	if err := model.DeleteGroupItemsByServiceID(serviceID); err != nil {
		return err
	}
	if err := svc.Delete(); err != nil {
		return err
	}
	// 服务已从 DB 删除，回收其连接及子进程，避免孤儿进程残留。
	// Remove → Adapter.Close() → SDK CommandTransport.Close()：
	// 关 stdin → 等 5s → SIGTERM → 再等 → SIGKILL，确保 stdio 子进程退出。
	if SessionPool != nil {
		SessionPool.Remove(serviceID)
	}
	return nil
}

func (s *McpServiceService) RefreshTools(userID, serviceID int64) (*dto.RefreshToolsResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}

	// 市场引用服务(source=marketplace):平台托管,用户侧无上游配置/凭证,不能直连上游刷新。
	// "重新同步"= 用市场项最新的 tools_snapshot 覆盖引用行的 tools_cache 快照(§4.2/§11 工具同步)。
	if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
		return s.refreshMarketplaceTools(svc)
	}

	if SessionPool == nil {
		return nil, nil
	}

	session, err := SessionPool.GetOrConnect(context.Background(), svc)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}

	// 同步刷新资源/提示缓存:"刷新"按钮一并更新服务详情的资源/提示列表。
	// 连接时的预热是异步的(不拖慢 tools/call 热路径),这里显式等它完成。
	SessionPool.RefreshItemCaches(context.Background(), session)

	// Re-read the service to get updated tools_cache
	svc, _ = model.GetServiceByID(userID, serviceID)
	var tools []interface{}
	_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

	return &dto.RefreshToolsResult{
		ToolsCount: len(tools),
		Tools:      tools,
	}, nil
}

// refreshMarketplaceTools 用市场项最新的 tools_snapshot 刷新用户引用服务的 tools_cache 快照(§4.2/§11)。
// 市场引用服务上游由平台托管,用户侧不持有配置/凭证,故无法直连上游刷新——以平台维护的工具快照为准。
func (s *McpServiceService) refreshMarketplaceTools(svc *model.McpService) (*dto.RefreshToolsResult, error) {
	item, err := model.GetMarketplaceItemByID(*svc.MarketplaceItemID)
	if err != nil {
		return nil, fmt.Errorf("市场项不存在或已下架")
	}
	now := time.Now()
	updates := map[string]interface{}{
		"tools_cache":      item.ToolsSnapshot,
		"tools_updated_at": now,
	}
	// 资源/提示快照一并同步;仅在市场项快照非空时覆盖,避免无快照的旧市场项清空引用行已有缓存
	if item.ResourcesSnapshot != "" {
		updates["resources_cache"] = item.ResourcesSnapshot
	}
	if item.PromptsSnapshot != "" {
		updates["prompts_cache"] = item.PromptsSnapshot
	}
	// 握手信息同理:市场项刷新过即有真实版本,一并同步到引用行
	if item.ProtocolVersion != "" {
		updates["protocol_version"] = item.ProtocolVersion
	}
	if item.ServerInfo != "" && item.ServerInfo != "{}" {
		updates["server_info"] = item.ServerInfo
	}
	if err := model.DB.Model(&model.McpService{}).Where("id = ?", svc.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	var tools []interface{}
	_ = json.Unmarshal([]byte(item.ToolsSnapshot), &tools)
	return &dto.RefreshToolsResult{ToolsCount: len(tools), Tools: tools}, nil
}

// materializeMarketplace 为市场引用服务(source=marketplace)从 marketplace_items 注入平台上游
// 配置/凭证到内存 McpService(不落库:引用行 config 始终为空,凭证不暴露给用户),并还原真实 transport_type。
// 与 internal/mcp/handler/gateway_handler.go materializeMarketplaceConfig 保持一致(§6.1)。
func (s *McpServiceService) materializeMarketplace(svc *model.McpService) error {
	item, err := model.GetMarketplaceItemByID(*svc.MarketplaceItemID)
	if err != nil {
		return fmt.Errorf("市场项不存在或已下架")
	}
	if item.Status != common.StatusEnabled {
		return fmt.Errorf("市场项 %s 已下架", item.Name)
	}
	// config_template 加密落库(§4.3):平台凭证 Decrypt 后注入;存量明文项 Decrypt 失败则回退原值
	if plain, dErr := common.Decrypt(item.ConfigTemplate); dErr == nil && plain != "" {
		svc.Config = plain
	} else {
		svc.Config = item.ConfigTemplate
	}
	if svc.TransportType == "" || svc.TransportType == "marketplace" {
		svc.TransportType = item.TransportType
	}
	// 共享 stdio 条目:会话池按条目键控(全部安装用户复用一个平台子进程);
	// 非 stdio/独占条目保持行键控。仅此处与网关 materializeMarketplaceConfig 设置。
	svc.SharedProcess = item.TransportType == string(transport.TypeStdio) && !item.IsolatedProcess
	return nil
}

func (s *McpServiceService) Test(userID, serviceID int64) (*dto.TestResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}

	// 市场引用服务:注入平台上游配置/凭证后再测试连通性(与网关 materializeMarketplaceConfig 一致,§6.1)。
	if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
		if mErr := s.materializeMarketplace(svc); mErr != nil {
			return &dto.TestResult{Connected: false, Error: mErr.Error()}, nil
		}
	}

	adapter := bridge.CreateAdapter(svc)
	if adapter == nil {
		return &dto.TestResult{Connected: false, Error: "不支持的传输类型"}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	if err := adapter.Connect(ctx); err != nil {
		return &dto.TestResult{
			Connected: false,
			Error:     err.Error(),
			LatencyMs: time.Since(start).Milliseconds(),
		}, nil
	}
	defer adapter.Close()

	tools := adapter.GetTools()

	return &dto.TestResult{
		Connected:       true,
		ToolsCount:      len(tools),
		LatencyMs:       time.Since(start).Milliseconds(),
		ProtocolVersion: adapter.GetProtocolVersion(),
		ServerInfo:      handshakeInfoMap(adapter),
	}, nil
}

func (s *McpServiceService) GetTools(userID, serviceID int64) ([]interface{}, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	var tools []interface{}
	_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
	return tools, nil
}

// CallTool 服务详情页工具测试:直连该服务的上游会话执行 tools/call(调试用途)。
// 虚拟服务(vision/camera)走 VirtualRegistry 分发;市场引用服务注入平台上游配置后调用,
// 并与网关调用同口径计费(见 callMarketplaceToolTested):预扣 → 成功确认/失败退款,
// 管理员默认同样扣费(ChargeAdmin),余额不足/未定价直接拒绝。
// 本地错误(连接失败/工具不存在)以 IsError+Error 返回,由前端在结果区展示。
// 测试结果同样落调用日志(recordManualTestLog),健康状态条与健康回写一并吃到数据。
func (s *McpServiceService) CallTool(userID, serviceID int64, req *dto.CallToolReq) (*dto.CallToolResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}

	// 平台托管(市场)服务:上游凭证在平台侧,测试即真实消耗平台上游,按网关价格正常计费。
	if svc.Source == "marketplace" {
		return s.callMarketplaceToolTested(svc, userID, req)
	}

	res, _ := callToolTested(svc, userID, req)
	recordManualTestLog(svc, userID, "tools/call", req.Name, res, nil)
	return res, nil
}

// manualEntryPreConsume 手动测试计费·预扣段(§6.2 与网关同口径):条目价解析 → 预扣。
// 返回 (sess, abort):abort 非 nil 时调用方直接返回该结果且不再调上游(sess 恒 nil,
// bill 已更新);sess 非 nil 表示已预扣,调用结束后必须传 manualEntryFinalize 结算。
// 免费(含计费总开关关闭/资源提示无条目价)/价格加载失败(FailOpen)返回 (nil, nil) 放行。
// 手动测试走会话鉴权无 API Key,ApiKeyID=0 仅受用户总额度约束;RequestID 进程内唯一,
// 每次点击都是新请求,不做幂等去重(同 tools/call 手动测试口径)。
func manualEntryPreConsume(svcID, itemID, userID int64, kind, entryName string, bill *manualTestBilling) (*billing.BillingSession, *dto.CallToolResult) {
	user, uErr := model.GetUserByID(userID)
	if uErr != nil {
		// 无法取用户信息:bill 保持 skipped(放行式失败,同网关 FailOpen 口径)
		return nil, &dto.CallToolResult{IsError: true, Error: "用户信息获取失败: " + uErr.Error()}
	}

	price, perr := billing.ResolveMarketplaceEntryPrice(itemID, kind, entryName, user.Group)
	bill.BillingType = price.BillingType
	bill.UnitPrice = price.UnitPriceDecimal
	bill.PriceScope = price.Scope

	switch {
	case perr == nil && (price.BillingType == billing.BillingTypeFree || price.UnitPriceQuota <= 0):
		// 免费(含 BillingEnabled=false):放行,不扣费
		return nil, nil
	case errors.Is(perr, billing.ErrPriceNotConfigured):
		bill.Status = "blocked"
		return nil, &dto.CallToolResult{IsError: true, Error: "市场服务价格未配置,无法测试"}
	case perr != nil:
		// 价格加载失败:FailOpen 放行(免费),同网关
		return nil, nil
	}

	// 预扣:RequestID 进程内唯一——手动测试每次点击都是新逻辑请求,不做幂等去重。
	sess, err := manualBillingService.PreConsume(billing.PreConsumeRequest{
		Price:     price,
		UserID:    userID,
		ApiKeyID:  0, // 会话鉴权无 API Key:仅用户额度约束
		UserRole:  user.Role,
		RequestID: fmt.Sprintf("manual:%d:%d", svcID, time.Now().UnixNano()),
	})
	if errors.Is(err, billing.ErrInsufficientQuota) {
		bill.Status = "blocked"
		return nil, &dto.CallToolResult{IsError: true, Error: "用户额度不足,剩余额度不足,请充值或兑换"}
	}
	if err != nil {
		// 计费 DB 异常且非 FailOpen:拒绝;FailOpen 时 PreConsume 返回 nil err + Debt
		bill.Status = "blocked"
		return nil, &dto.CallToolResult{IsError: true, Error: "计费服务暂时不可用,请稍后重试"}
	}
	if sess.Debt {
		bill.Status = "debt"
	} else {
		bill.Status = "pending"
	}
	return sess, nil
}

// manualEntryFinalize 手动测试计费·结算段(与网关 finalizeBilling 同口径):Debt 保持
// 欠账标记;成功按实扣记 charged(零消费如管理员豁免记 skipped);失败全额退款记
// refunded。sess 为 nil 时无操作。
func manualEntryFinalize(sess *billing.BillingSession, bill *manualTestBilling, callOK bool) {
	if sess == nil {
		return
	}
	switch {
	case sess.Debt:
		bill.Status = "debt"
		bill.Quota = 0
	case callOK:
		_ = manualBillingService.Confirm(sess)
		if sess.ConsumedQuota > 0 {
			bill.Status = "charged"
			bill.Quota = sess.ConsumedQuota
		} else {
			bill.Status = "skipped"
		}
	default:
		_ = manualBillingService.Refund(sess)
		bill.Status = "refunded"
	}
}

// callMarketplaceToolTested 市场引用服务(source=marketplace)的工具测试:注入平台上游
// 配置/凭证后直连上游执行 tools/call,计费与网关调用完全同口径(§6.2):
// 条目级定价解析(工具:条目→服务→全局默认,乘分组倍率)→ 预扣 → 成功 Confirm / 失败 Refund。
// 与网关 preConsumeBilling/finalizeBilling 的策略一致:
//   - 免费(含计费总开关关闭)/价格加载失败(FailOpen):放行不扣费;
//   - 未显式定价(非自用模式):拒绝,不调上游;
//   - 余额不足:拒绝本次调用,不调上游(不禁用、不影响已有余额);
//   - 成败判定=上游 tools/call 是否成功且结果无 isError(工具层失败默认退款,
//     ChargeOnClientError=true 才计费,同网关 §6.6);
//   - 手动测试走会话鉴权无 API Key,ApiKeyID=0:仅受用户总额度约束,不占任何 Key 预算;
//   - 管理员默认同样计费(ChargeAdmin=true),仅显式关闭时豁免。
//
// 计费结算结果随手动测试日志落库(billing_status=charged/refunded/blocked/...)。
func (s *McpServiceService) callMarketplaceToolTested(svc *model.McpService, userID int64, req *dto.CallToolReq) (*dto.CallToolResult, error) {
	bill := &manualTestBilling{Status: "skipped", ItemID: svc.MarketplaceItemID}

	if svc.MarketplaceItemID == nil {
		res := &dto.CallToolResult{IsError: true, Error: "市场服务缺少关联市场项,无法测试"}
		recordManualTestLog(svc, userID, "tools/call", req.Name, res, bill)
		return res, nil
	}
	// 注入平台上游配置/凭证(同网关 materializeMarketplaceConfig);市场项下架等失败直接终止。
	if mErr := s.materializeMarketplace(svc); mErr != nil {
		res := &dto.CallToolResult{IsError: true, Error: mErr.Error()}
		recordManualTestLog(svc, userID, "tools/call", req.Name, res, bill)
		return res, nil
	}
	// 工具名先用快照缓存校验:不存在的工具直接拒绝,避免无效预扣/退款往返。
	if !serviceHasTool(nil, svc, req.Name) {
		res := &dto.CallToolResult{IsError: true, Error: "工具不存在: " + req.Name}
		recordManualTestLog(svc, userID, "tools/call", req.Name, res, bill)
		return res, nil
	}

	sess, abort := manualEntryPreConsume(svc.ID, *svc.MarketplaceItemID, userID, billing.EntryKindTool, req.Name, bill)
	if abort != nil {
		recordManualTestLog(svc, userID, "tools/call", req.Name, abort, bill)
		return abort, nil
	}

	res, callOK := callToolTested(svc, userID, req)
	// 成败判定同网关(§6.6):传输层失败(callOK=false)退款;结果内 isError(工具层
	// 失败,如上游 key 错误/余额不足)默认退款,ChargeOnClientError=true 才计费。
	manualEntryFinalize(sess, bill, callOK && billing.ShouldChargeCall(nil, res.IsError))
	recordManualTestLog(svc, userID, "tools/call", req.Name, res, bill)
	return res, nil
}

// testMarketplaceEntry 市场服务资源/提示测试的同口径计费外壳:物化平台配置 → 条目价
// 解析+预扣 → call() → 结算 → 落手动测试日志(与工具测试/网关 resources/read、
// prompts/get 完全同口径;资源/提示无条目价缺省免费,显式继承按服务价)。call 的第二
// 返回值 callOK=传输层成功(资源/提示读取无结果内 isError 概念,调用成功即扣费,同网关
// err==nil 判定)。
func (s *McpServiceService) testMarketplaceEntry(svc *model.McpService, userID int64, kind, entryName, method, target string, call func() (*dto.CallToolResult, bool)) (*dto.CallToolResult, error) {
	bill := &manualTestBilling{Status: "skipped", ItemID: svc.MarketplaceItemID}

	if svc.MarketplaceItemID == nil {
		res := &dto.CallToolResult{IsError: true, Error: "市场服务缺少关联市场项,无法测试"}
		recordManualTestLog(svc, userID, method, target, res, bill)
		return res, nil
	}
	if mErr := s.materializeMarketplace(svc); mErr != nil {
		res := &dto.CallToolResult{IsError: true, Error: mErr.Error()}
		recordManualTestLog(svc, userID, method, target, res, bill)
		return res, nil
	}

	sess, abort := manualEntryPreConsume(svc.ID, *svc.MarketplaceItemID, userID, kind, entryName, bill)
	if abort != nil {
		recordManualTestLog(svc, userID, method, target, abort, bill)
		return abort, nil
	}

	res, callOK := call()
	manualEntryFinalize(sess, bill, callOK)
	recordManualTestLog(svc, userID, method, target, res, bill)
	return res, nil
}

// callToolTested 工具测试的实际调用:虚拟服务分发或上游 tools/call,全部出口
// (连接失败/工具不存在/调用失败/结果解析)统一返回 CallToolResult 供外层落日志。
// 第二返回值 callOK=上游调用在传输层成功(结果内 isError 的工具层错误由外层结合
// ChargeOnClientError 判定,传输失败恒退款,同网关 §6.6)。
func callToolTested(svc *model.McpService, userID int64, req *dto.CallToolReq) (*dto.CallToolResult, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var args json.RawMessage
	if req.Arguments != nil {
		args, _ = json.Marshal(req.Arguments)
	}
	start := time.Now()

	// 虚拟服务:按 serviceID 分发到虚拟工具处理器(vision/camera 等自有性质,免费)。
	if svc.TransportType == "virtual" {
		if VirtualRegistry == nil {
			return &dto.CallToolResult{IsError: true, Error: "虚拟服务未初始化"}, false
		}
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(svc.Config), &config)
		raw, vErr := VirtualRegistry.Handle(virtual.WithCallerUserID(ctx, userID), svc.ID, config, req.Name, args)
		if vErr != nil {
			return &dto.CallToolResult{IsError: true, Error: vErr.Error(), DurationMs: time.Since(start).Milliseconds()}, false
		}
		res, _ := parseTestResult(raw, start)
		return res, true
	}

	// 普通服务:获取测试用适配器(优先复用会话池,见 acquireTestAdapter)。
	adapter, closeAdapter, fail := acquireTestAdapter(ctx, svc)
	if fail != nil {
		fail.DurationMs = time.Since(start).Milliseconds()
		return fail, false
	}
	defer closeAdapter()

	// 工具名仅在服务自身工具列表中校验(单服务调试,不涉及网关命名空间路由)。
	if !serviceHasTool(adapter, svc, req.Name) {
		return &dto.CallToolResult{IsError: true, Error: "工具不存在: " + req.Name, DurationMs: time.Since(start).Milliseconds()}, false
	}

	raw, cErr := adapter.Call(ctx, "tools/call", map[string]interface{}{
		"name":      req.Name,
		"arguments": req.Arguments,
	})
	if cErr != nil {
		return &dto.CallToolResult{IsError: true, Error: cErr.Error(), DurationMs: time.Since(start).Milliseconds()}, false
	}
	res, _ := parseTestResult(raw, start)
	return res, true
}

// ReadResource 服务详情页资源测试:对指定 URI 执行 resources/read。
// 市场引用服务注入平台上游配置后同口径计费(与网关 resources/read 一致:按资源
// 条目价扣费,无条目价免费;2026-08 起资源/提示纳入条目级定价);虚拟服务无资源能力;
// 结果同样落调用日志(recordManualTestLog,计费结算结果随日志落库)。
func (s *McpServiceService) ReadResource(userID, serviceID int64, req *dto.ReadResourceReq) (*dto.CallToolResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.TransportType == "virtual" {
		return &dto.CallToolResult{IsError: true, Error: "虚拟服务不支持资源测试"}, nil
	}
	if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
		return s.testMarketplaceEntry(svc, userID, billing.EntryKindResource, req.URI, "resources/read", req.URI,
			func() (*dto.CallToolResult, bool) { return readResourceTested(svc, req) })
	}
	res, _ := readResourceTested(svc, req)
	recordManualTestLog(svc, userID, "resources/read", req.URI, res, nil)
	return res, nil
}

// readResourceTested 资源测试的实际调用,全部出口统一返回 CallToolResult 供外层落日志。
// 第二返回值 callOK=上游 resources/read 传输层成功(计费成败判定,同网关 err==nil)。
func readResourceTested(svc *model.McpService, req *dto.ReadResourceReq) (*dto.CallToolResult, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	adapter, closeAdapter, fail := acquireTestAdapter(ctx, svc)
	if fail != nil {
		fail.DurationMs = time.Since(start).Milliseconds()
		return fail, false
	}
	defer closeAdapter()
	raw, rErr := adapter.ReadResource(ctx, req.URI)
	if rErr != nil {
		return &dto.CallToolResult{IsError: true, Error: rErr.Error(), DurationMs: time.Since(start).Milliseconds()}, false
	}
	res, _ := parseTestResult(raw, start)
	return res, true
}

// GetPrompt 服务详情页提示测试:按传入参数渲染提示(prompts/get),市场服务同
// ReadResource 同口径计费(按提示条目价扣费,无条目价免费);虚拟服务无提示能力;
// 结果同样落调用日志(recordManualTestLog,计费结算结果随日志落库)。
func (s *McpServiceService) GetPrompt(userID, serviceID int64, req *dto.GetPromptReq) (*dto.CallToolResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.TransportType == "virtual" {
		return &dto.CallToolResult{IsError: true, Error: "虚拟服务不支持提示测试"}, nil
	}
	if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
		return s.testMarketplaceEntry(svc, userID, billing.EntryKindPrompt, req.Name, "prompts/get", req.Name,
			func() (*dto.CallToolResult, bool) { return getPromptTested(svc, req) })
	}
	res, _ := getPromptTested(svc, req)
	recordManualTestLog(svc, userID, "prompts/get", req.Name, res, nil)
	return res, nil
}

// getPromptTested 提示测试的实际调用,全部出口统一返回 CallToolResult 供外层落日志。
// 第二返回值 callOK=上游 prompts/get 传输层成功(计费成败判定,同网关 err==nil)。
func getPromptTested(svc *model.McpService, req *dto.GetPromptReq) (*dto.CallToolResult, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	adapter, closeAdapter, fail := acquireTestAdapter(ctx, svc)
	if fail != nil {
		fail.DurationMs = time.Since(start).Milliseconds()
		return fail, false
	}
	defer closeAdapter()
	raw, gErr := adapter.GetPrompt(ctx, req.Name, req.Arguments)
	if gErr != nil {
		return &dto.CallToolResult{IsError: true, Error: gErr.Error(), DurationMs: time.Since(start).Milliseconds()}, false
	}
	res, _ := parseTestResult(raw, start)
	return res, true
}

// acquireTestAdapter 获取测试用上游适配器:优先复用会话池连接(stdio 不必每次拉起子进程),
// 池未初始化时回退一次性连接(同 Test)。失败时返回可直接下发的 IsError 结果(DurationMs 由调用方补)。
func acquireTestAdapter(ctx context.Context, svc *model.McpService) (transport.TransportAdapter, func(), *dto.CallToolResult) {
	if SessionPool != nil {
		session, err := SessionPool.GetOrConnect(ctx, svc)
		if err != nil {
			return nil, nil, &dto.CallToolResult{IsError: true, Error: err.Error()}
		}
		return session.Adapter, func() {}, nil
	}
	adapter := bridge.CreateAdapter(svc)
	if adapter == nil {
		return nil, nil, &dto.CallToolResult{IsError: true, Error: "不支持的传输类型"}
	}
	if err := adapter.Connect(ctx); err != nil {
		return nil, nil, &dto.CallToolResult{IsError: true, Error: err.Error()}
	}
	return adapter, func() { adapter.Close() }, nil
}

// serviceHasTool 校验工具名是否属于该服务:优先用 adapter 实时工具列表,回退 tools_cache。
// adapter 可为 nil(市场服务计费前仅按快照缓存预检,未建立连接)。
func serviceHasTool(adapter transport.TransportAdapter, svc *model.McpService, name string) bool {
	if adapter != nil {
		for _, t := range adapter.GetTools() {
			if t.Name == name {
				return true
			}
		}
	}
	var cache []transport.Tool
	if json.Unmarshal([]byte(svc.ToolsCache), &cache) == nil {
		for _, t := range cache {
			if t.Name == name {
				return true
			}
		}
	}
	return false
}

// parseTestResult 解析上游测试调用原始结果(tools/call/resources/read/prompts/get),
// 提取其中的 isError 标记(tools/call 特有,其余调用无此字段保持 false)并记录耗时。
func parseTestResult(raw json.RawMessage, start time.Time) (*dto.CallToolResult, error) {
	result := &dto.CallToolResult{DurationMs: time.Since(start).Milliseconds()}
	if uErr := json.Unmarshal(raw, &result.Result); uErr != nil {
		return &dto.CallToolResult{IsError: true, Error: "结果解析失败: " + uErr.Error(), DurationMs: result.DurationMs}, nil
	}
	if m, ok := result.Result.(map[string]interface{}); ok {
		if flag, _ := m["isError"].(bool); flag {
			result.IsError = true
		}
	}
	return result, nil
}

// GetResources 返回服务缓存的资源与资源模板({"resources":[],"templates":[]} 形态)。
func (s *McpServiceService) GetResources(userID, serviceID int64) (map[string]interface{}, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	var cache map[string]interface{}
	_ = json.Unmarshal([]byte(svc.ResourcesCache), &cache)
	if cache == nil {
		cache = map[string]interface{}{"resources": []interface{}{}, "templates": []interface{}{}}
	}
	return cache, nil
}

// GetPrompts 返回服务缓存的提示列表。
func (s *McpServiceService) GetPrompts(userID, serviceID int64) ([]interface{}, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	var prompts []interface{}
	_ = json.Unmarshal([]byte(svc.PromptsCache), &prompts)
	if prompts == nil {
		prompts = []interface{}{}
	}
	return prompts, nil
}

func (s *McpServiceService) GetHealth(userID, serviceID int64) (map[string]interface{}, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"health_status":     svc.HealthStatus,
		"last_health_check": svc.LastHealthCheck,
	}, nil
}

// GetProcessStat 返回 stdio 服务子进程(整棵进程树)的资源占用快照。
// 非 stdio 服务或进程未运行(服务未被连接/已退出)时返回 Running:false,不报错,
// 供前端以固定形态渲染"未运行"状态。
func (s *McpServiceService) GetProcessStat(userID, serviceID int64) (*dto.ServiceProcessStat, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}

	notRunning := &dto.ServiceProcessStat{}
	if svc.TransportType != string(transport.TypeStdio) || SessionPool == nil {
		return notRunning, nil
	}
	session := SessionPool.Get(serviceID)
	if session == nil || !session.Adapter.IsConnected() {
		return notRunning, nil
	}
	proc := session.Adapter.GetStdioProcess()
	if proc == nil {
		return notRunning, nil
	}

	tree := transport.CollectProcessTreeStat(proc.PID, proc.Command)
	return &dto.ServiceProcessStat{
		Running:       tree.Running,
		PID:           tree.PID,
		Command:       tree.Command,
		ProcessCount:  tree.ProcessCount,
		MemoryRSS:     tree.RSSBytes,
		MemoryVMS:     tree.VMSBytes,
		CPUPercent:    tree.CPUPercent,
		UptimeSeconds: tree.UptimeSeconds,
	}, nil
}

// ControlProcess 对 stdio 服务子进程执行启动/停止/重启。启动(及重启的拉起)同步等待
// 握手完成后返回最新进程快照;停止走 Remove 的完整终止序列,并清掉过期的健康标记。
func (s *McpServiceService) ControlProcess(userID, serviceID int64, action string) (*dto.ServiceProcessStat, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.TransportType != string(transport.TypeStdio) {
		return nil, fmt.Errorf("仅 stdio 服务支持进程操作")
	}
	if svc.Status != common.StatusEnabled {
		return nil, fmt.Errorf("服务已禁用,请先启用再操作进程")
	}
	if SessionPool == nil {
		return nil, fmt.Errorf("会话池未初始化")
	}

	switch action {
	case "stop":
		SessionPool.Remove(serviceID)
		// 进程已停,库里的 healthy 是过期标记,还原为未知
		model.DB.Model(&model.McpService{}).Where("id = ?", serviceID).Update("health_status", common.HealthUnknown)
	case "start", "restart":
		if action == "restart" {
			SessionPool.Remove(serviceID)
		}
		// 市场引用服务:config 为空,先注入平台上游配置再拉起(与网关/测试口径一致)
		if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
			if mErr := s.materializeMarketplace(svc); mErr != nil {
				return nil, mErr
			}
		}
		// npx/uvx 首次拉起可能要下载依赖,放宽到 60s(与前端 65s 超时对齐)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, cErr := SessionPool.GetOrConnect(ctx, svc); cErr != nil {
			return nil, cErr
		}
	default:
		return nil, fmt.Errorf("不支持的操作: %s", action)
	}
	return s.GetProcessStat(userID, serviceID)
}

// GetServicesOverview 服务总览页数据:全部服务(不分页) + 统计摘要 + 各 stdio 服务
// 进程树资源快照 + 非 stdio 服务健康快照(调用日志聚合)。只读 SessionPool 现状,
// 绝不触发 GetOrConnect(查看总览不拉起进程);全部进程树合并为一次系统进程扫描。
// 普通用户(isAdmin=false)排除 stdio 服务(进程资源/启停操作属管理员运维视角),
// 页面呈现健康状态条视角;marketplace 等其余来源不排除。
func (s *McpServiceService) GetServicesOverview(userID int64, isAdmin bool) (*dto.ServicesOverview, error) {
	services, err := model.ListAllServicesByUser(userID)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		kept := make([]model.McpService, 0, len(services))
		for i := range services {
			if services[i].TransportType == string(transport.TypeStdio) {
				continue
			}
			kept = append(kept, services[i])
		}
		services = kept
	}

	// 池内会话快照(一次加锁),后续只读判断连接状态
	sessions := map[int64]*bridge.McpSession{}
	if SessionPool != nil {
		for _, sess := range SessionPool.GetAllSessions() {
			sessions[sess.ServiceID] = sess
		}
	}

	// 收集已连接 stdio 服务的进程根,一次扫描采集全部进程树
	var roots []transport.ProcessRoot
	for i := range services {
		svc := &services[i]
		if svc.TransportType != string(transport.TypeStdio) {
			continue
		}
		session := sessions[svc.ID]
		if session == nil || !session.Adapter.IsConnected() {
			continue
		}
		if proc := session.Adapter.GetStdioProcess(); proc != nil {
			roots = append(roots, transport.ProcessRoot{Key: svc.ID, RootPID: proc.PID, Command: proc.Command})
		}
	}
	treeStats := transport.CollectProcessTreesStat(roots)

	// 非 stdio 服务健康快照:mcp_call_logs 真实调用聚合(30s 缓存),出错降级为
	// 不带健康字段,同样不触碰 SessionPool。口径为当前用户自己的服务行;平台托管
	// 服务的全用户聚合健康在管理端市场页(GET /admin/marketplace/health)单独提供。
	var remoteIDs []int64
	for i := range services {
		if services[i].TransportType != string(transport.TypeStdio) {
			remoteIDs = append(remoteIDs, services[i].ID)
		}
	}
	healthSnaps := GetServiceHealthSnapshots(remoteIDs)

	resp := &dto.ServicesOverview{Services: make([]dto.ServicesOverviewItem, len(services))}
	for i, svc := range services {
		var tools []interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

		item := dto.ServicesOverviewItem{
			ID:            svc.ID,
			Name:          svc.Name,
			DisplayName:   svc.DisplayName,
			TransportType: svc.TransportType,
			Source:        svc.Source,
			HealthStatus:  svc.HealthStatus,
			ToolsCount:    len(tools),
			Status:        svc.Status,
			CreatedAt:     svc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}

		// running 口径:stdio = 池内会话已连接且进程树确实存活(连接标记可能滞后
		// 于进程退出,以采集结果为准);远程/虚拟无本机进程可测,取被动口径——当前
		// 有活跃连接或近 200 分钟窗口内有成功调用即算运行,禁用服务恒为未运行。
		// 健康状态同口径实时推导(derivePassiveHealth),不依赖库里仅在调用后回写的标记。
		session := sessions[svc.ID]
		connected := session != nil && session.Adapter.IsConnected()
		if svc.TransportType == string(transport.TypeStdio) {
			tree := treeStats[svc.ID]
			item.Running = connected && tree != nil && tree.Running
			if item.Running {
				item.PID = tree.PID
				item.ProcessCount = tree.ProcessCount
				item.MemoryRSS = tree.RSSBytes
				item.CPUPercent = tree.CPUPercent
				item.UptimeSeconds = tree.UptimeSeconds
			}
			// 健康率口径:stdio 以进程实测存活为准
			if item.Running {
				item.HealthStatus = common.HealthHealthy
				resp.Summary.HealthyCount++
			} else {
				item.HealthStatus = common.HealthUnknown
			}
		} else if snap := healthSnaps[svc.ID]; snap != nil {
			item.HealthBuckets = snap.Buckets
			item.LastErrorMessage = snap.LastErrorMessage
			item.LastErrorAt = snap.LastErrorUnix
			item.HealthStatus = derivePassiveHealth(svc.Status, connected, snap.Buckets)
			item.Running = svc.Status == common.StatusEnabled &&
				(connected || hasRecentSuccess(snap.Buckets))
			if item.HealthStatus == common.HealthHealthy {
				resp.Summary.HealthyCount++
			}
			// 窗口内有调用直接取快照时间;空窗服务点查全历史,给"暂无调用"一个时间锚点
			if snap.LastCallUnix > 0 {
				item.LastCallAt = snap.LastCallUnix
			} else if lt := model.GetLastConsumeTime(svc.ID); lt != nil {
				item.LastCallAt = lt.Unix()
			}
		} else {
			// 健康聚合失败降级:退回连接状态/未知,不报错
			item.HealthStatus = derivePassiveHealth(svc.Status, connected, nil)
			item.Running = svc.Status == common.StatusEnabled && connected
			if item.HealthStatus == common.HealthHealthy {
				resp.Summary.HealthyCount++
			}
		}

		resp.Summary.ToolsTotal += item.ToolsCount
		resp.Summary.ProcessTotal += item.ProcessCount
		resp.Summary.MemoryRSSTotal += item.MemoryRSS
		resp.Summary.CPUTotalPercent += item.CPUPercent
		if item.Running {
			resp.Summary.RunningServices++
		}
		resp.Services[i] = item
	}
	resp.Summary.TotalServices = len(services)

	// 主机物理内存总量:总览内存条与卡片的公共分母
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		resp.Summary.HostMemoryTotal = vm.Total
	}
	return resp, nil
}

// derivePassiveHealth 非 stdio 服务的实时被动健康推导(对齐 CLIProxyAPI:只看真实流量):
// 禁用/无数据 → 未知;当前有活跃连接 → 健康;否则看窗口内最近一个非空桶——
// 全失败 → 异常,有成功 → 健康。展示琥珀色由前端负责,红色只出现在色带插值里。
func derivePassiveHealth(status int, connected bool, buckets []dto.HealthBucket) string {
	if status != common.StatusEnabled {
		return common.HealthUnknown
	}
	if connected {
		return common.HealthHealthy
	}
	for i := len(buckets) - 1; i >= 0; i-- {
		b := buckets[i]
		if b.Success+b.Failed == 0 {
			continue
		}
		if b.Success > 0 {
			return common.HealthHealthy
		}
		return common.HealthUnhealthy
	}
	return common.HealthUnknown
}

// hasRecentSuccess 近 200 分钟窗口内是否有成功调用(与"有活跃连接"共同构成远程服务 running 判定)。
func hasRecentSuccess(buckets []dto.HealthBucket) bool {
	for _, b := range buckets {
		if b.Success > 0 {
			return true
		}
	}
	return false
}

// --- Admin service management ---

func (s *McpServiceService) CreateAdminService(adminID int64, req *dto.CreateServiceReq) (*dto.ServiceDetail, error) {
	configJSON, _ := json.Marshal(req.Config)
	authConfigJSON, _ := json.Marshal(req.AuthConfig)
	tags := strings.Join(req.Tags, ",")

	svc := &model.McpService{
		UserID:        adminID,
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		TransportType: req.TransportType,
		Config:        string(configJSON),
		AuthType:      req.AuthType,
		AuthConfig:    string(authConfigJSON),
		Tags:          tags,
		Source:        "admin",
		Status:        common.StatusEnabled,
		HealthStatus:  common.HealthUnknown,
	}
	if svc.AuthType == "" {
		svc.AuthType = "none"
	}
	if err := svc.Insert(); err != nil {
		return nil, err
	}

	// 异步加载工具
	if SessionPool != nil {
		go func() {
			SessionPool.GetOrConnect(context.Background(), svc)
		}()
	}

	return s.toDetail(svc), nil
}

func (s *McpServiceService) ListAdminServices(page, pageSize int) ([]dto.ServiceListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	services, total, err := model.ListServicesBySource("admin", offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return s.toServiceListItems(services), total, nil
}

// ListClonableServices 返回指定管理员(userID)可克隆上架的来源服务(其账户下 source=user/admin,
// 自动排除虚拟服务与市场引用),供"从自有服务克隆"下拉使用(§11)。
func (s *McpServiceService) ListClonableServices(userID int64, page, pageSize int) ([]dto.ServiceListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	services, total, err := model.ListClonableServices(userID, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return s.toServiceListItems(services), total, nil
}

// toServiceListItems 把 McpService 列表统一转为列表项 DTO(解析 tools_cache 统计工具数)。
func (s *McpServiceService) toServiceListItems(services []model.McpService) []dto.ServiceListItem {
	items := make([]dto.ServiceListItem, len(services))
	for i, svc := range services {
		var tools []interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
		items[i] = dto.ServiceListItem{
			ID:            svc.ID,
			Name:          svc.Name,
			DisplayName:   svc.DisplayName,
			Description:   svc.Description,
			TransportType: svc.TransportType,
			Source:        svc.Source,
			HealthStatus:  svc.HealthStatus,
			ToolsCount:    len(tools),
			Status:        svc.Status,
			CreatedAt:     svc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items
}

func (s *McpServiceService) toDetail(svc *model.McpService) *dto.ServiceDetail {
	var config map[string]interface{}
	_ = json.Unmarshal([]byte(svc.Config), &config)
	var toolsCache []interface{}
	_ = json.Unmarshal([]byte(svc.ToolsCache), &toolsCache)
	var serverInfo map[string]interface{}
	_ = json.Unmarshal([]byte(svc.ServerInfo), &serverInfo)

	var toolsUpdatedAt string
	if svc.ToolsUpdatedAt != nil {
		toolsUpdatedAt = svc.ToolsUpdatedAt.Format("2006-01-02T15:04:05Z")
	}
	var lastHealthCheck string
	if svc.LastHealthCheck != nil {
		lastHealthCheck = svc.LastHealthCheck.Format("2006-01-02T15:04:05Z")
	}

	var tags []string
	if svc.Tags != "" {
		tags = strings.Split(svc.Tags, ",")
	} else {
		tags = []string{}
	}

	d := &dto.ServiceDetail{
		ID:              svc.ID,
		Name:            svc.Name,
		DisplayName:     svc.DisplayName,
		Description:     svc.Description,
		TransportType:   svc.TransportType,
		Source:          svc.Source,
		Config:          config,
		AuthType:        svc.AuthType,
		HealthStatus:    svc.HealthStatus,
		LastHealthCheck: lastHealthCheck,
		ToolsCache:      toolsCache,
		ToolsUpdatedAt:  toolsUpdatedAt,
		ServerInfo:      serverInfo,
		ProtocolVersion: svc.ProtocolVersion,
		Tags:            tags,
		Status:          svc.Status,
		CreatedAt:       svc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		PassiveConnected: svc.PassiveConnected,
	}
	// 市场引用服务带上条目 ID(前端跳转市场详情用)
	if svc.Source == "marketplace" {
		d.MarketplaceItemID = svc.MarketplaceItemID
	}
	return d
}
