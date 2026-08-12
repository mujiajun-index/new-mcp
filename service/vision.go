package service

import (
	"context"
	"encoding/base64"
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
	// MaxTokens == 0 means "unlimited"; the OpenAI/Gemini paths omit the field
	// (omitempty) and the Anthropic path substitutes a high safe cap.
	vc := &model.VisionConfig{
		UserID:            userID,
		Name:              req.Name,
		Description:       req.Description,
		Provider:          req.Provider,
		ModelName:         req.ModelName,
		EndpointURL:       req.EndpointURL,
		ApiKey:            req.ApiKey,
		SystemPrompt:      req.SystemPrompt,
		MaxTokens:         req.MaxTokens,
		AutoRegister:      false,
		Status:            common.StatusEnabled,
		AnalyzeImageName:  "vision.analyze_image",
		AnalyzeImageDesc:  "Analyze image content and identify the objects, text, and scenes it contains. Best for: extracting structured info, detecting items, or reading text. Returns: a detailed breakdown of recognized elements.",
		DescribeSceneName: "vision.describe_scene",
		DescribeSceneDesc: "Describe the scene and overall content of an image in natural language. Best for: getting a high-level summary of what is happening. Returns: a natural-language description of the scene.",
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

	// Use a tiny 1x1 white pixel PNG as test image.
	testImageB64 := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="
	testImage, err := base64.StdEncoding.DecodeString(testImageB64)
	if err != nil {
		return &dto.TestVisionResult{Success: false, Error: fmt.Sprintf("decode test image: %v", err)}
	}

	result, err := client.Analyze(ctx, "You are a test assistant.", "Describe this image in one word.", vision.ImageInput{Bytes: testImage, MediaType: "image/png"})
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
	// Both tools accept either an inline base64 image or an image_url. The URL
	// is preferred: the bytes stay out of the calling LLM's context (upload once
	// via /api/v1/vision/upload or /api/v1/vision/mcp-upload, pass the returned
	// signed URL here) and the upstream model fetches it directly. Neither field
	// is "required" in the schema because exactly one must be present; the
	// handler enforces the either/or with a clear error.
	//
	// V1.1: the choice is size-based (§16). Small images may inline base64; for
	// larger ones the model should upload first (vision.upload_image → curl →
	// image_url) so the bytes stay out of its context.
	const imageURLDesc = "Public https URL of the image. Use for LARGER images: obtain file_url via the vision.upload_image tool (returns a ready curl command + file_url, no API key in the curl), run the curl, then pass file_url here. The upstream model fetches it directly, so image bytes never enter the LLM context. For small images you may inline base64 via the image parameter instead."
	const imageB64Desc = "Base64-encoded image. Use for SMALL images only (at most ~VisionInlineMaxBytes, default 10KB). Larger images bloat the LLM context (~400 token/KB, generated as output) — for those, use vision.upload_image → image_url instead."

	return []map[string]interface{}{
		{
			"name":        vc.AnalyzeImageName,
			"description": vc.AnalyzeImageDesc,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image_url": map[string]string{"type": "string", "description": imageURLDesc},
					"image":     map[string]string{"type": "string", "description": imageB64Desc},
					"prompt":    map[string]string{"type": "string", "description": "Custom analysis prompt (optional)"},
				},
			},
		},
		{
			"name":        vc.DescribeSceneName,
			"description": vc.DescribeSceneDesc,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"image_url": map[string]string{"type": "string", "description": imageURLDesc},
					"image":     map[string]string{"type": "string", "description": imageB64Desc},
				},
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
