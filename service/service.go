package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	}
	// 配置（command/args/env/registry/url/headers）变更后，运行中的连接/子进程仍带旧配置。
	// 踢掉旧 session 并按新配置异步重连（与 Create 一致）：让新 env 立即生效，同时刷新
	// tools_cache 并预热连接。AuthConfig 不喂给 adapter、DisplayName/Description/Tags 为展示字段，均无需重连。
	if req.Config != nil && SessionPool != nil {
		SessionPool.Remove(serviceID)
		go SessionPool.GetOrConnect(context.Background(), svc)
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

// CallTool 服务详情页工具测试:直连该服务的上游会话执行 tools/call(调试用途,不计费)。
// 虚拟服务(vision/camera)走 VirtualRegistry 分发;市场引用服务注入平台上游配置后调用。
// 本地错误(连接失败/工具不存在)以 IsError+Error 返回,由前端在结果区展示。
func (s *McpServiceService) CallTool(userID, serviceID int64, req *dto.CallToolReq) (*dto.CallToolResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}

	// 平台托管(市场)服务:工具测试不计费,禁止免费消耗平台上游,只能测试自有服务。
	// 连通性测试 Test() 不受此限制(仍走 materializeMarketplace 物化后连接)。
	if svc.Source == "marketplace" {
		return &dto.CallToolResult{IsError: true, Error: "平台托管(市场)服务不支持工具测试"}, nil
	}

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
			return &dto.CallToolResult{IsError: true, Error: "虚拟服务未初始化"}, nil
		}
		var config map[string]interface{}
		_ = json.Unmarshal([]byte(svc.Config), &config)
		raw, vErr := VirtualRegistry.Handle(virtual.WithCallerUserID(ctx, userID), svc.ID, config, req.Name, args)
		if vErr != nil {
			return &dto.CallToolResult{IsError: true, Error: vErr.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
		}
		return parseTestResult(raw, start)
	}

	// 普通服务:获取测试用适配器(优先复用会话池,见 acquireTestAdapter)。
	adapter, closeAdapter, fail := acquireTestAdapter(ctx, svc)
	if fail != nil {
		fail.DurationMs = time.Since(start).Milliseconds()
		return fail, nil
	}
	defer closeAdapter()

	// 工具名仅在服务自身工具列表中校验(单服务调试,不涉及网关命名空间路由)。
	if !serviceHasTool(adapter, svc, req.Name) {
		return &dto.CallToolResult{IsError: true, Error: "工具不存在: " + req.Name}, nil
	}

	raw, cErr := adapter.Call(ctx, "tools/call", map[string]interface{}{
		"name":      req.Name,
		"arguments": req.Arguments,
	})
	if cErr != nil {
		return &dto.CallToolResult{IsError: true, Error: cErr.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	return parseTestResult(raw, start)
}

// ReadResource 服务详情页资源测试:对指定 URI 执行 resources/read。
// 与工具测试同策略:不计费的调试用途,市场(平台托管)服务禁止;虚拟服务无资源能力。
func (s *McpServiceService) ReadResource(userID, serviceID int64, req *dto.ReadResourceReq) (*dto.CallToolResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.Source == "marketplace" {
		return &dto.CallToolResult{IsError: true, Error: "平台托管(市场)服务不支持资源测试"}, nil
	}
	if svc.TransportType == "virtual" {
		return &dto.CallToolResult{IsError: true, Error: "虚拟服务不支持资源测试"}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	adapter, closeAdapter, fail := acquireTestAdapter(ctx, svc)
	if fail != nil {
		fail.DurationMs = time.Since(start).Milliseconds()
		return fail, nil
	}
	defer closeAdapter()
	raw, rErr := adapter.ReadResource(ctx, req.URI)
	if rErr != nil {
		return &dto.CallToolResult{IsError: true, Error: rErr.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	return parseTestResult(raw, start)
}

// GetPrompt 服务详情页提示测试:按传入参数渲染提示(prompts/get),限制策略同 ReadResource。
func (s *McpServiceService) GetPrompt(userID, serviceID int64, req *dto.GetPromptReq) (*dto.CallToolResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.Source == "marketplace" {
		return &dto.CallToolResult{IsError: true, Error: "平台托管(市场)服务不支持提示测试"}, nil
	}
	if svc.TransportType == "virtual" {
		return &dto.CallToolResult{IsError: true, Error: "虚拟服务不支持提示测试"}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	adapter, closeAdapter, fail := acquireTestAdapter(ctx, svc)
	if fail != nil {
		fail.DurationMs = time.Since(start).Milliseconds()
		return fail, nil
	}
	defer closeAdapter()
	raw, gErr := adapter.GetPrompt(ctx, req.Name, req.Arguments)
	if gErr != nil {
		return &dto.CallToolResult{IsError: true, Error: gErr.Error(), DurationMs: time.Since(start).Milliseconds()}, nil
	}
	return parseTestResult(raw, start)
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
func serviceHasTool(adapter transport.TransportAdapter, svc *model.McpService, name string) bool {
	for _, t := range adapter.GetTools() {
		if t.Name == name {
			return true
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

// GetServicesOverview 服务总览页数据:全部服务(不分页) + 统计摘要 + 各 stdio 服务
// 进程树资源快照。只读 SessionPool 现状,绝不触发 GetOrConnect(查看总览不拉起
// 进程);全部进程树合并为一次系统进程扫描。
func (s *McpServiceService) GetServicesOverview(userID int64) (*dto.ServicesOverview, error) {
	services, err := model.ListAllServicesByUser(userID)
	if err != nil {
		return nil, err
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
		// 于进程退出,以采集结果为准);远程/虚拟无本机进程可测,取最近一次健康
		// 检测结果(当前有活跃连接同样算),禁用服务恒为未运行。
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
		} else {
			item.Running = svc.Status == common.StatusEnabled &&
				(connected || svc.HealthStatus == common.HealthHealthy)
		}

		resp.Summary.ToolsTotal += item.ToolsCount
		resp.Summary.ProcessTotal += item.ProcessCount
		resp.Summary.MemoryRSSTotal += item.MemoryRSS
		resp.Summary.CPUTotalPercent += item.CPUPercent
		if item.Running {
			resp.Summary.RunningServices++
		}
		if svc.HealthStatus == common.HealthHealthy {
			resp.Summary.HealthyCount++
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

	return &dto.ServiceDetail{
		ID:               svc.ID,
		Name:             svc.Name,
		DisplayName:      svc.DisplayName,
		Description:      svc.Description,
		TransportType:    svc.TransportType,
		Source:           svc.Source,
		Config:           config,
		AuthType:         svc.AuthType,
		HealthStatus:     svc.HealthStatus,
		LastHealthCheck:  lastHealthCheck,
		ToolsCache:       toolsCache,
		ToolsUpdatedAt:   toolsUpdatedAt,
		ServerInfo:       serverInfo,
		ProtocolVersion:  svc.ProtocolVersion,
		Tags:             tags,
		Status:           svc.Status,
		CreatedAt:        svc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		PassiveConnected: svc.PassiveConnected,
	}
}
