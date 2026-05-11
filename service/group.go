package service

import (
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type GroupService struct{}

func (s *GroupService) List(userID int64, page, pageSize int) ([]dto.GroupListItem, int64, error) {
	offset := common.GetOffset(page, pageSize)
	groups, total, err := model.ListGroupsByUser(userID, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.GroupListItem, len(groups))
	for i, g := range groups {
		toolsCount, _ := s.getToolsCount(g.ID)
		items[i] = dto.GroupListItem{
			ID:          g.ID,
			Name:        g.Name,
			DisplayName: g.DisplayName,
			Description: g.Description,
			ExposeMode:  g.ExposeMode,
			ToolsCount:  toolsCount,
			Status:      g.Status,
			CreatedAt:   g.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, total, nil
}

func (s *GroupService) Create(userID int64, req *dto.CreateGroupReq) (*dto.GroupDetail, error) {
	group := &model.McpGroup{
		UserID:       userID,
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Description:  req.Description,
		EndpointSlug: req.EndpointSlug,
		Visibility:   req.Visibility,
		EndpointAuth: req.EndpointAuth,
		ExposeMode:   req.ExposeMode,
		Status:       common.StatusEnabled,
	}
	if group.Visibility == "" {
		group.Visibility = "private"
	}
	if group.EndpointAuth == "" {
		group.EndpointAuth = "api_key"
	}
	if group.ExposeMode == "" {
		group.ExposeMode = "smart"
	}

	if err := group.Insert(); err != nil {
		return nil, err
	}
	return s.toDetail(group)
}

func (s *GroupService) GetByID(userID, groupID int64) (*dto.GroupDetail, error) {
	group, err := model.GetGroupByID(userID, groupID)
	if err != nil {
		return nil, err
	}
	return s.toDetail(group)
}

func (s *GroupService) Update(userID, groupID int64, req *dto.UpdateGroupReq) error {
	group, err := model.GetGroupByID(userID, groupID)
	if err != nil {
		return err
	}
	if req.DisplayName != nil {
		group.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		group.Description = *req.Description
	}
	if req.Visibility != nil {
		group.Visibility = *req.Visibility
	}
	if req.ExposeMode != nil {
		group.ExposeMode = *req.ExposeMode
	}
	if req.Status != nil {
		group.Status = *req.Status
	}
	return group.Update()
}

func (s *GroupService) Delete(userID, groupID int64) error {
	group, err := model.GetGroupByID(userID, groupID)
	if err != nil {
		return err
	}
	return group.Delete()
}

func (s *GroupService) AddServices(userID, groupID int64, serviceIDs []int64) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	return model.AddServicesToGroup(groupID, serviceIDs)
}

func (s *GroupService) RemoveService(userID, groupID, serviceID int64) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	return model.RemoveServiceFromGroup(groupID, serviceID)
}

func (s *GroupService) GetTools(userID, groupID int64) ([]dto.GroupToolItem, error) {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return nil, err
	}
	return s.getAggregatedTools(groupID)
}

func (s *GroupService) UpdateTool(userID, groupID int64, toolName string, req *dto.UpdateToolReq) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	serviceID := int64(0)
	if req.ServiceID != nil {
		serviceID = *req.ServiceID
	}
	existing, err := model.GetGroupTool(groupID, serviceID, toolName)
	if err != nil {
		existing = &model.McpGroupTool{
			GroupID:   groupID,
			ServiceID: serviceID,
			ToolName:  toolName,
			Enabled:   true,
		}
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.NameOverride != nil {
		existing.NameOverride = *req.NameOverride
	}
	if req.DescriptionOverride != nil {
		existing.DescriptionOverride = *req.DescriptionOverride
	}
	return existing.Upsert()
}

func (s *GroupService) BatchUpdateTools(userID, groupID int64, tools []dto.BatchToolUpdate) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	for _, t := range tools {
		existing, err := model.GetGroupTool(groupID, t.ServiceID, t.ToolName)
		if err != nil {
			existing = &model.McpGroupTool{
				GroupID:   groupID,
				ServiceID: t.ServiceID,
				ToolName:  t.ToolName,
				Enabled:   t.Enabled,
			}
		} else {
			existing.Enabled = t.Enabled
		}
		if err := existing.Upsert(); err != nil {
			return err
		}
	}
	return nil
}

func (s *GroupService) RefreshAll(userID, groupID int64) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	// Actual refresh via transport adapters in Phase 3
	return nil
}

func (s *GroupService) GetEndpoint(userID, groupID int64) (*dto.EndpointInfo, error) {
	group, err := model.GetGroupByID(userID, groupID)
	if err != nil {
		return nil, err
	}
	return s.buildEndpointInfo(group)
}

func (s *GroupService) getToolsCount(groupID int64) (int, error) {
	tools, err := s.getAggregatedTools(groupID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, t := range tools {
		if t.Enabled {
			count++
		}
	}
	return count, nil
}

func (s *GroupService) getAggregatedTools(groupID int64) ([]dto.GroupToolItem, error) {
	groupServices, err := model.GetEnabledGroupServices(groupID)
	if err != nil {
		return nil, err
	}

	toolFilters, _ := model.GetGroupTools(groupID)
	filterMap := make(map[string]*model.McpGroupTool)
	for i := range toolFilters {
		key := fmt.Sprintf("%d:%s", toolFilters[i].ServiceID, toolFilters[i].ToolName)
		filterMap[key] = &toolFilters[i]
	}

	var result []dto.GroupToolItem
	for _, gs := range groupServices {
		svc, err := model.GetServiceByIDWithoutUser(gs.ServiceID)
		if err != nil {
			continue
		}
		var tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

		for _, t := range tools {
			key := fmt.Sprintf("%d:%s", gs.ServiceID, t.Name)
			enabled := true
			nameOverride := ""
			if f, ok := filterMap[key]; ok {
				enabled = f.Enabled
				nameOverride = f.NameOverride
			}
			result = append(result, dto.GroupToolItem{
				ServiceID:    gs.ServiceID,
				Name:         svc.Name + "__" + t.Name,
				OriginalName: t.Name,
				ServiceName:  svc.Name,
				Description:  t.Description,
				Enabled:      enabled,
				NameOverride: nameOverride,
			})
		}
	}
	return result, nil
}

func (s *GroupService) toDetail(group *model.McpGroup) (*dto.GroupDetail, error) {
	groupServices, _ := model.GetGroupServices(group.ID)

	services := make([]dto.GroupServiceItem, 0)
	for _, gs := range groupServices {
		svc, err := model.GetServiceByIDWithoutUser(gs.ServiceID)
		if err != nil {
			continue
		}
		var tools []interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
		services = append(services, dto.GroupServiceItem{
			ID:         svc.ID,
			Name:       svc.Name,
			Enabled:    gs.Enabled,
			ToolsCount: len(tools),
		})
	}

	toolsCount, _ := s.getToolsCount(group.ID)

	return &dto.GroupDetail{
		ID:           group.ID,
		Name:         group.Name,
		DisplayName:  group.DisplayName,
		Description:  group.Description,
		EndpointSlug: group.EndpointSlug,
		EndpointURL:  common.BaseURL + "/mcp/group/" + group.EndpointSlug,
		Visibility:   group.Visibility,
		ExposeMode:   group.ExposeMode,
		Services:     services,
		ToolsCount:   toolsCount,
		Status:       group.Status,
	}, nil
}

func (s *GroupService) buildEndpointInfo(group *model.McpGroup) (*dto.EndpointInfo, error) {
	httpURL := common.BaseURL + "/mcp/group/" + group.EndpointSlug
	wsURL := common.BaseURL + "/mcp/ws/group/" + group.EndpointSlug

	return &dto.EndpointInfo{
		StreamableHTTPURL: httpURL,
		WebSocketURL:      wsURL,
		AuthType:          group.EndpointAuth,
		ConnectionConfig: map[string]interface{}{
			"type":    "streamable-http",
			"url":     httpURL,
			"headers": map[string]string{"X-API-Key": "nm-xxxxxxxxxxxx"},
		},
		McpClientConfig: map[string]interface{}{
			"mcpServers": map[string]interface{}{
				group.Name: map[string]interface{}{
					"type":    "streamable-http",
					"url":     httpURL,
					"headers": map[string]string{"X-API-Key": "nm-xxxxxxxxxxxx"},
				},
			},
		},
	}, nil
}
