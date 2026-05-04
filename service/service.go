package service

import (
	"encoding/json"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

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
			HealthStatus:  svc.HealthStatus,
			ToolsCount:    len(tools),
			Status:        svc.Status,
			CreatedAt:     svc.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}

func (s *McpServiceService) Create(userID int64, req *dto.CreateServiceReq) (*dto.ServiceDetail, error) {
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

	return s.toDetail(svc), nil
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
	return svc.Update()
}

func (s *McpServiceService) Delete(userID, serviceID int64) error {
	svc, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return err
	}
	return svc.Delete()
}

func (s *McpServiceService) RefreshTools(userID, serviceID int64) (*dto.RefreshToolsResult, error) {
	_, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	// Actual tool refresh will use transport adapters in Phase 3
	return &dto.RefreshToolsResult{ToolsCount: 0, Tools: []interface{}{}}, nil
}

func (s *McpServiceService) Test(userID, serviceID int64) (*dto.TestResult, error) {
	_, err := model.GetServiceByID(userID, serviceID)
	if err != nil {
		return nil, err
	}
	// Actual connection test will use transport adapters in Phase 3
	return &dto.TestResult{Connected: false}, nil
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
		ID:              svc.ID,
		Name:            svc.Name,
		DisplayName:     svc.DisplayName,
		Description:     svc.Description,
		TransportType:   svc.TransportType,
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
}
