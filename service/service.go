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
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
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
		Connected:  true,
		ToolsCount: len(tools),
		LatencyMs:  time.Since(start).Milliseconds(),
	}, nil
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

	// Re-read the service to get updated tools_cache
	svc, _ = model.GetServiceByID(userID, serviceID)
	var tools []interface{}
	_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

	return &dto.RefreshToolsResult{
		ToolsCount: len(tools),
		Tools:      tools,
	}, nil
}

func (s *McpServiceService) Test(userID, serviceID int64) (*dto.TestResult, error) {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
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
		Connected:  true,
		ToolsCount: len(tools),
		LatencyMs:  time.Since(start).Milliseconds(),
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
