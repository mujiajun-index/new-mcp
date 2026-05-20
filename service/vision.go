package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
)

type VisionService struct{}

func (s *VisionService) List(userID int64) ([]dto.VisionConfigListItem, error) {
	configs, err := model.ListVisionConfigsByUser(userID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.VisionConfigListItem, len(configs))
	for i, c := range configs {
		items[i] = dto.VisionConfigListItem{
			ID:                  c.ID,
			Name:                c.Name,
			Provider:            c.Provider,
			ModelName:           c.ModelName,
			EndpointURL:         c.EndpointURL,
			AutoRegister:        c.AutoRegister,
			RegisteredServiceID: c.RegisteredServiceID,
			Status:              c.Status,
			CreatedAt:           c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, nil
}

func (s *VisionService) Create(userID int64, req *dto.CreateVisionConfigReq) (*dto.VisionConfigDetail, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	vc := &model.VisionConfig{
		UserID:            userID,
		Name:              req.Name,
		Description:       req.Description,
		Provider:          req.Provider,
		ModelName:         req.ModelName,
		EndpointURL:       req.EndpointURL,
		ApiKey:            req.ApiKey,
		SystemPrompt:      req.SystemPrompt,
		MaxTokens:         maxTokens,
		AutoRegister:      false,
		Status:            common.StatusEnabled,
		AnalyzeImageName:  "vision.analyze_image",
		AnalyzeImageDesc:  "分析图片内容，识别其中的物体、文字、场景等",
		DescribeSceneName: "vision.describe_scene",
		DescribeSceneDesc: "描述图片中的场景和整体内容",
		ExtraConfig:       "{}",
	}

	if err := vc.Insert(); err != nil {
		return nil, err
	}

	return s.toDetail(vc), nil
}

func (s *VisionService) GetByID(userID, id int64) (*dto.VisionConfigDetail, error) {
	vc, err := model.GetVisionConfigByID(userID, id)
	if err != nil {
		return nil, err
	}
	return s.toDetail(vc), nil
}

func (s *VisionService) Update(userID, id int64, req *dto.UpdateVisionConfigReq) error {
	vc, err := model.GetVisionConfigByID(userID, id)
	if err != nil {
		return err
	}

	if req.Name != nil {
		vc.Name = *req.Name
	}
	if req.Description != nil {
		vc.Description = *req.Description
	}
	if req.Provider != nil {
		vc.Provider = *req.Provider
	}
	if req.ModelName != nil {
		vc.ModelName = *req.ModelName
	}
	if req.EndpointURL != nil {
		vc.EndpointURL = *req.EndpointURL
	}
	if req.ApiKey != nil {
		vc.ApiKey = *req.ApiKey
	}
	if req.SystemPrompt != nil {
		vc.SystemPrompt = *req.SystemPrompt
	}
	if req.MaxTokens != nil {
		vc.MaxTokens = *req.MaxTokens
	}
	if req.AnalyzeImageName != nil {
		vc.AnalyzeImageName = *req.AnalyzeImageName
	}
	if req.AnalyzeImageDesc != nil {
		vc.AnalyzeImageDesc = *req.AnalyzeImageDesc
	}
	if req.DescribeSceneName != nil {
		vc.DescribeSceneName = *req.DescribeSceneName
	}
	if req.DescribeSceneDesc != nil {
		vc.DescribeSceneDesc = *req.DescribeSceneDesc
	}
	if req.Status != nil {
		vc.Status = *req.Status
	}

	if err := vc.Update(); err != nil {
		return err
	}

	// If registered, update the virtual service tools_cache
	if vc.AutoRegister && vc.RegisteredServiceID != nil {
		s.syncVirtualService(vc)
	}

	return nil
}

func (s *VisionService) Enable(userID, id int64) error {
	vc, err := model.GetVisionConfigByID(userID, id)
	if err != nil {
		return err
	}
	if vc.AutoRegister {
		return nil
	}

	// Create virtual McpService
	serviceName := fmt.Sprintf("vision_%d", vc.ID)
	svc := &model.McpService{
		UserID:        vc.UserID,
		Name:          serviceName,
		DisplayName:   vc.Name,
		Description:   vc.Description,
		TransportType: "virtual",
		Source:        "vision",
		Config:        fmt.Sprintf(`{"virtual_type":"vision","ref_id":%d}`, vc.ID),
		HealthStatus:  "healthy",
		Status:        common.StatusEnabled,
	}

	now := time.Now()
	svc.ToolsUpdatedAt = &now

	tools := s.buildToolsCache(vc)
	toolsJSON, _ := json.Marshal(tools)
	svc.ToolsCache = string(toolsJSON)

	if err := svc.Insert(); err != nil {
		return fmt.Errorf("failed to create virtual service: %w", err)
	}

	vc.AutoRegister = true
	vc.RegisteredServiceID = &svc.ID
	_ = vc.Update()

	// Register handler
	if VirtualRegistry != nil {
		VirtualRegistry.Register(svc.ID, vc.UserID, serviceName, virtual.ParseConfig(svc.Config), virtual.VisionHandler)
	}

	return nil
}

func (s *VisionService) Disable(userID, id int64) error {
	vc, err := model.GetVisionConfigByID(userID, id)
	if err != nil {
		return err
	}
	if !vc.AutoRegister || vc.RegisteredServiceID == nil {
		return nil
	}

	serviceID := *vc.RegisteredServiceID

	// Unregister handler
	if VirtualRegistry != nil {
		VirtualRegistry.Unregister(serviceID)
	}

	// Clean up McpGroupTool and McpGroupService references
	model.DB.Where("service_id = ?", serviceID).Delete(&model.McpGroupTool{})
	model.DB.Where("service_id = ?", serviceID).Delete(&model.McpGroupService{})
	model.DB.Delete(&model.McpService{}, serviceID)

	vc.AutoRegister = false
	vc.RegisteredServiceID = nil
	_ = vc.Update()

	return nil
}

func (s *VisionService) Delete(userID, id int64) error {
	// Disable first to clean up virtual service
	_ = s.Disable(userID, id)

	vc, err := model.GetVisionConfigByID(userID, id)
	if err != nil {
		return err
	}
	return vc.Delete()
}

func (s *VisionService) TestVision(req *dto.TestVisionReq) *dto.TestVisionResult {
	client := &vision.VisionClient{
		Provider:    req.Provider,
		EndpointURL: req.EndpointURL,
		ApiKey:      req.ApiKey,
		ModelName:   req.ModelName,
		MaxTokens:   100,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use a tiny 1x1 white pixel as test image
	testImage := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="

	result, err := client.Analyze(ctx, "You are a test assistant.", "Describe this image in one word.", testImage)
	if err != nil {
		return &dto.TestVisionResult{Success: false, Error: err.Error()}
	}
	return &dto.TestVisionResult{Success: true, Result: result}
}

func (s *VisionService) ListModels(userID int64, req *dto.ListModelsReq) ([]dto.ModelInfo, error) {
	apiKey := req.ApiKey
	endpointURL := req.EndpointURL
	if apiKey == "" && req.ConfigID > 0 {
		vc, err := model.GetVisionConfigByID(userID, req.ConfigID)
		if err != nil {
			return nil, fmt.Errorf("配置不存在")
		}
		apiKey = vc.ApiKey
		if endpointURL == "" {
			endpointURL = vc.EndpointURL
		}
	}
	if apiKey == "" {
		return nil, fmt.Errorf("缺少 API Key")
	}

	client := &vision.VisionClient{
		Provider:    req.Provider,
		EndpointURL: endpointURL,
		ApiKey:      apiKey,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]dto.ModelInfo, len(models))
	for i, m := range models {
		result[i] = dto.ModelInfo{ID: m.ID, Name: m.Name}
	}
	return result, nil
}

func (s *VisionService) syncVirtualService(vc *model.VisionConfig) {
	if vc.RegisteredServiceID == nil {
		return
	}

	var svc model.McpService
	if err := model.DB.First(&svc, *vc.RegisteredServiceID).Error; err != nil {
		return
	}

	svc.DisplayName = vc.Name
	svc.Description = vc.Description
	tools := s.buildToolsCache(vc)
	toolsJSON, _ := json.Marshal(tools)
	svc.ToolsCache = string(toolsJSON)
	_ = svc.Update()
}

func (s *VisionService) buildToolsCache(vc *model.VisionConfig) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        vc.AnalyzeImageName,
			"description": vc.AnalyzeImageDesc,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image":  map[string]string{"type": "string", "description": "Base64 encoded image"},
					"prompt": map[string]string{"type": "string", "description": "Custom analysis prompt (optional)"},
				},
				"required": []string{"image"},
			},
		},
		{
			"name":        vc.DescribeSceneName,
			"description": vc.DescribeSceneDesc,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image": map[string]string{"type": "string", "description": "Base64 encoded image"},
				},
				"required": []string{"image"},
			},
		},
	}
}

func (s *VisionService) toDetail(vc *model.VisionConfig) *dto.VisionConfigDetail {
	return &dto.VisionConfigDetail{
		ID:                  vc.ID,
		Name:                vc.Name,
		Description:         vc.Description,
		Provider:            vc.Provider,
		ModelName:           vc.ModelName,
		EndpointURL:         vc.EndpointURL,
		SystemPrompt:        vc.SystemPrompt,
		MaxTokens:           vc.MaxTokens,
		AutoRegister:        vc.AutoRegister,
		RegisteredServiceID: vc.RegisteredServiceID,
		AnalyzeImageName:    vc.AnalyzeImageName,
		AnalyzeImageDesc:    vc.AnalyzeImageDesc,
		DescribeSceneName:   vc.DescribeSceneName,
		DescribeSceneDesc:   vc.DescribeSceneDesc,
		ExtraConfig:         vc.ExtraConfig,
		Status:              vc.Status,
		CreatedAt:           vc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           vc.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
