package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
)

type CameraService struct{}

func (s *CameraService) List(userID int64) ([]dto.CameraListItem, error) {
	cameras, err := model.ListCamerasByUser(userID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.CameraListItem, len(cameras))
	for i, c := range cameras {
		var visionConfigName string
		if c.VisionConfigID != nil {
			var vc model.VisionConfig
			if err := model.DB.Select("name").First(&vc, *c.VisionConfigID).Error; err == nil {
				visionConfigName = vc.Name
			}
		}

		streaming := false
		if CameraStreamMgr != nil {
			streaming = CameraStreamMgr.IsStreaming(c.ID)
		}

		items[i] = dto.CameraListItem{
			ID:                  c.ID,
			Name:                c.Name,
			VisionConfigID:      c.VisionConfigID,
			VisionConfigName:    visionConfigName,
			AutoRegister:        c.AutoRegister,
			RegisteredServiceID: c.RegisteredServiceID,
			Streaming:           streaming,
			Status:              c.Status,
			CreatedAt:           c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, nil
}

func (s *CameraService) Create(userID int64, req *dto.CreateCameraReq) (*dto.CameraDetail, error) {
	// Verify vision config exists
	var vc model.VisionConfig
	if err := model.DB.Where("id = ? AND user_id = ?", req.VisionConfigID, userID).First(&vc).Error; err != nil {
		return nil, fmt.Errorf("视觉配置不存在")
	}

	cam := &model.Camera{
		UserID:        userID,
		Name:          req.Name,
		Description:   req.Description,
		SourceType:    "webrtc",
		SourceURL:     "browser",
		VisionConfigID: &req.VisionConfigID,
		AutoRegister:  false,
		Status:        common.StatusEnabled,
		CaptureName:   "camera.capture",
		CaptureDesc:   "截取当前摄像头画面并返回图像",
		AnalyzeName:   "camera.analyze",
		AnalyzeDesc:   "截取当前摄像头画面并识别分析",
		ExtraConfig:   "{}",
	}

	if err := cam.Insert(); err != nil {
		return nil, err
	}

	return s.toDetail(cam), nil
}

func (s *CameraService) GetByID(userID, id int64) (*dto.CameraDetail, error) {
	cam, err := model.GetCameraByID(userID, id)
	if err != nil {
		return nil, err
	}
	return s.toDetail(cam), nil
}

func (s *CameraService) Update(userID, id int64, req *dto.UpdateCameraReq) error {
	cam, err := model.GetCameraByID(userID, id)
	if err != nil {
		return err
	}

	if req.Name != nil {
		cam.Name = *req.Name
	}
	if req.Description != nil {
		cam.Description = *req.Description
	}
	if req.VisionConfigID != nil {
		cam.VisionConfigID = req.VisionConfigID
	}
	if req.CaptureName != nil {
		cam.CaptureName = *req.CaptureName
	}
	if req.CaptureDesc != nil {
		cam.CaptureDesc = *req.CaptureDesc
	}
	if req.AnalyzeName != nil {
		cam.AnalyzeName = *req.AnalyzeName
	}
	if req.AnalyzeDesc != nil {
		cam.AnalyzeDesc = *req.AnalyzeDesc
	}
	if req.Status != nil {
		cam.Status = *req.Status
	}

	if err := cam.Update(); err != nil {
		return err
	}

	if cam.AutoRegister && cam.RegisteredServiceID != nil {
		s.syncVirtualService(cam)
	}

	return nil
}

func (s *CameraService) Enable(userID, id int64) error {
	cam, err := model.GetCameraByID(userID, id)
	if err != nil {
		return err
	}
	if cam.AutoRegister {
		return nil
	}

	serviceName := fmt.Sprintf("camera_%d", cam.ID)
	svc := &model.McpService{
		UserID:        cam.UserID,
		Name:          serviceName,
		DisplayName:   cam.Name,
		Description:   cam.Description,
		TransportType: "virtual",
		Source:        "camera",
		Config:        fmt.Sprintf(`{"virtual_type":"camera","ref_id":%d}`, cam.ID),
		HealthStatus:  "healthy",
		Status:        common.StatusEnabled,
	}

	now := time.Now()
	svc.ToolsUpdatedAt = &now

	tools := s.buildToolsCache(cam)
	toolsJSON, _ := json.Marshal(tools)
	svc.ToolsCache = string(toolsJSON)

	if err := svc.Insert(); err != nil {
		return fmt.Errorf("failed to create virtual service: %w", err)
	}

	cam.AutoRegister = true
	cam.RegisteredServiceID = &svc.ID
	_ = cam.Update()

	if VirtualRegistry != nil {
		VirtualRegistry.Register(svc.ID, virtual.CameraHandler)
	}

	return nil
}

func (s *CameraService) Disable(userID, id int64) error {
	cam, err := model.GetCameraByID(userID, id)
	if err != nil {
		return err
	}
	if !cam.AutoRegister || cam.RegisteredServiceID == nil {
		return nil
	}

	serviceID := *cam.RegisteredServiceID

	if VirtualRegistry != nil {
		VirtualRegistry.Unregister(serviceID)
	}

	model.DB.Where("service_id = ?", serviceID).Delete(&model.McpGroupTool{})
	model.DB.Where("service_id = ?", serviceID).Delete(&model.McpGroupService{})
	model.DB.Delete(&model.McpService{}, serviceID)

	cam.AutoRegister = false
	cam.RegisteredServiceID = nil
	_ = cam.Update()

	return nil
}

func (s *CameraService) Delete(userID, id int64) error {
	_ = s.Disable(userID, id)

	if CameraStreamMgr != nil {
		CameraStreamMgr.Cleanup(id)
	}

	cam, err := model.GetCameraByID(userID, id)
	if err != nil {
		return err
	}
	return cam.Delete()
}

func (s *CameraService) syncVirtualService(cam *model.Camera) {
	if cam.RegisteredServiceID == nil {
		return
	}

	var svc model.McpService
	if err := model.DB.First(&svc, *cam.RegisteredServiceID).Error; err != nil {
		return
	}

	svc.DisplayName = cam.Name
	svc.Description = cam.Description
	tools := s.buildToolsCache(cam)
	toolsJSON, _ := json.Marshal(tools)
	svc.ToolsCache = string(toolsJSON)
	_ = svc.Update()
}

func (s *CameraService) buildToolsCache(cam *model.Camera) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        cam.CaptureName,
			"description": cam.CaptureDesc,
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        cam.AnalyzeName,
			"description": cam.AnalyzeDesc,
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"prompt": map[string]string{"type": "string", "description": "Custom analysis prompt (optional)"},
				},
			},
		},
	}
}

func (s *CameraService) toDetail(cam *model.Camera) *dto.CameraDetail {
	var visionConfigName string
	if cam.VisionConfigID != nil {
		var vc model.VisionConfig
		if err := model.DB.Select("name").First(&vc, *cam.VisionConfigID).Error; err == nil {
			visionConfigName = vc.Name
		}
	}

	streaming := false
	if CameraStreamMgr != nil {
		streaming = CameraStreamMgr.IsStreaming(cam.ID)
	}

	return &dto.CameraDetail{
		ID:                  cam.ID,
		Name:                cam.Name,
		Description:         cam.Description,
		VisionConfigID:      cam.VisionConfigID,
		VisionConfigName:    visionConfigName,
		AutoRegister:        cam.AutoRegister,
		RegisteredServiceID: cam.RegisteredServiceID,
		CaptureName:         cam.CaptureName,
		CaptureDesc:         cam.CaptureDesc,
		AnalyzeName:         cam.AnalyzeName,
		AnalyzeDesc:         cam.AnalyzeDesc,
		ExtraConfig:         cam.ExtraConfig,
		Streaming:           streaming,
		Status:              cam.Status,
		CreatedAt:           cam.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           cam.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
