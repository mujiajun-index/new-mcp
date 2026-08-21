package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type GroupService struct{}

// serverBase returns the configured server address from system settings,
// with any trailing slash trimmed. Used to build MCP endpoint URLs.
func serverBase() string {
	return strings.TrimRight(model.GetOptionString("ServerAddress"), "/")
}

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
		EndpointSlug: req.Name,
		Visibility:   req.Visibility,
		AutoDiscover: true, // 显式写入:模型列已去掉 default 标签,防 GORM 零值省略
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
	if req.Name != nil && *req.Name != "" {
		group.Name = *req.Name
		group.EndpointSlug = *req.Name
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
	// 禁止删除被 API Key 引用的分组，避免产生悬空引用
	keys, err := model.GetApiKeysReferencingGroup(userID, group.Name)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		names := make([]string, 0, len(keys))
		for _, k := range keys {
			names = append(names, k.Name)
		}
		return fmt.Errorf("该分组已被 %d 个 API Key 关联（%s），请先在 API Key 中移除该分组或删除对应密钥", len(keys), strings.Join(names, "、"))
	}
	return group.Delete()
}

func (s *GroupService) CheckName(userID int64, name string, excludeID int64) (bool, error) {
	return model.CheckGroupNameExists(userID, name, excludeID)
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

// --- 资源/提示聚合与条目级启停(与工具过滤同一套交互) ---

// GetResources 聚合分组内各服务的资源与资源模板(读 services.resources_cache),标注条目级启停。
func (s *GroupService) GetResources(userID, groupID int64) ([]dto.GroupResourceItem, error) {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return nil, err
	}

	disabled := groupItemDisabledSet(groupID)

	groupServices, err := model.GetEnabledGroupServices(groupID)
	if err != nil {
		return nil, err
	}
	svcByID := servicesByMembership(groupServices)

	var result []dto.GroupResourceItem
	for _, gs := range groupServices {
		svc := svcByID[gs.ServiceID]
		if svc == nil {
			continue
		}
		var cache struct {
			Resources []struct {
				URI         string `json:"uri"`
				Name        string `json:"name"`
				Description string `json:"description"`
				MIMEType    string `json:"mimeType"`
			} `json:"resources"`
			Templates []struct {
				URITemplate string `json:"uriTemplate"`
				Name        string `json:"name"`
				Description string `json:"description"`
				MIMEType    string `json:"mimeType"`
			} `json:"templates"`
		}
		_ = json.Unmarshal([]byte(svc.ResourcesCache), &cache)
		for _, r := range cache.Resources {
			result = append(result, dto.GroupResourceItem{
				ServiceID:   gs.ServiceID,
				ServiceName: svc.Name,
				Kind:        "resource",
				URI:         r.URI,
				Name:        r.Name,
				Description: r.Description,
				MimeType:    r.MIMEType,
				Enabled:     !disabled[groupItemKey(gs.ServiceID, "resource", r.URI)],
			})
		}
		for _, t := range cache.Templates {
			result = append(result, dto.GroupResourceItem{
				ServiceID:   gs.ServiceID,
				ServiceName: svc.Name,
				Kind:        "template",
				URI:         t.URITemplate,
				Name:        t.Name,
				Description: t.Description,
				MimeType:    t.MIMEType,
				Enabled:     !disabled[groupItemKey(gs.ServiceID, "template", t.URITemplate)],
			})
		}
	}
	return result, nil
}

// GetPrompts 聚合分组内各服务的提示(读 services.prompts_cache),标注条目级启停。
func (s *GroupService) GetPrompts(userID, groupID int64) ([]dto.GroupPromptItem, error) {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return nil, err
	}

	disabled := groupItemDisabledSet(groupID)

	groupServices, err := model.GetEnabledGroupServices(groupID)
	if err != nil {
		return nil, err
	}
	svcByID := servicesByMembership(groupServices)

	var result []dto.GroupPromptItem
	for _, gs := range groupServices {
		svc := svcByID[gs.ServiceID]
		if svc == nil {
			continue
		}
		var prompts []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Arguments   []dto.GroupPromptArgument `json:"arguments"`
		}
		_ = json.Unmarshal([]byte(svc.PromptsCache), &prompts)
		for _, p := range prompts {
			result = append(result, dto.GroupPromptItem{
				ServiceID:   gs.ServiceID,
				ServiceName: svc.Name,
				Name:        p.Name,
				Description: p.Description,
				Arguments:   p.Arguments,
				Enabled:     !disabled[groupItemKey(gs.ServiceID, "prompt", p.Name)],
			})
		}
	}
	return result, nil
}

func (s *GroupService) BatchUpdateResources(userID, groupID int64, items []dto.UpdateResourceItemReq) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	for _, it := range items {
		if err := upsertGroupItem(groupID, it.ServiceID, it.Kind, it.URI, it.Enabled); err != nil {
			return err
		}
	}
	return nil
}

func (s *GroupService) BatchUpdatePrompts(userID, groupID int64, items []dto.UpdatePromptItemReq) error {
	if _, err := model.GetGroupByID(userID, groupID); err != nil {
		return err
	}
	for _, it := range items {
		if err := upsertGroupItem(groupID, it.ServiceID, "prompt", it.Name, it.Enabled); err != nil {
			return err
		}
	}
	return nil
}

func upsertGroupItem(groupID, serviceID int64, kind, key string, enabled bool) error {
	existing, err := model.GetGroupItem(groupID, serviceID, kind, key)
	if err != nil {
		existing = &model.McpGroupItem{
			GroupID:   groupID,
			ServiceID: serviceID,
			ItemKind:  kind,
			ItemKey:   key,
			Enabled:   enabled,
		}
	} else {
		existing.Enabled = enabled
	}
	return existing.Upsert()
}

// groupItemDisabledSet 返回该分组内 Enabled=false 的条目集合("serviceID:kind:key")。
func groupItemDisabledSet(groupID int64) map[string]bool {
	rows, err := model.GetGroupItems(groupID)
	if err != nil {
		return nil
	}
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		if !r.Enabled {
			m[groupItemKey(r.ServiceID, r.ItemKind, r.ItemKey)] = true
		}
	}
	return m
}

func groupItemKey(serviceID int64, kind, key string) string {
	return fmt.Sprintf("%d:%s:%s", serviceID, kind, key)
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
	svcByID := servicesByMembership(groupServices)

	toolFilters, _ := model.GetGroupTools(groupID)
	filterMap := make(map[string]*model.McpGroupTool)
	for i := range toolFilters {
		key := fmt.Sprintf("%d:%s", toolFilters[i].ServiceID, toolFilters[i].ToolName)
		filterMap[key] = &toolFilters[i]
	}

	var result []dto.GroupToolItem
	for _, gs := range groupServices {
		svc := svcByID[gs.ServiceID]
		if svc == nil {
			// 服务已被删除但分组关联未清理（孤儿行）：静默跳过，不再逐条 First 触发 record not found 日志
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
			var inputSchema interface{}
			if len(t.InputSchema) > 0 {
				inputSchema = t.InputSchema
			}
			result = append(result, dto.GroupToolItem{
				ServiceID:    gs.ServiceID,
				Name:         svc.Name + "__" + t.Name,
				OriginalName: t.Name,
				ServiceName:  svc.Name,
				Description:  t.Description,
				Enabled:      enabled,
				NameOverride: nameOverride,
				InputSchema:  inputSchema,
			})
		}
	}
	return result, nil
}

func (s *GroupService) toDetail(group *model.McpGroup) (*dto.GroupDetail, error) {
	groupServices, _ := model.GetGroupServices(group.ID)
	svcByID := servicesByMembership(groupServices)

	services := make([]dto.GroupServiceItem, 0, len(groupServices))
	for _, gs := range groupServices {
		svc := svcByID[gs.ServiceID]
		if svc == nil {
			// 服务已被删除但分组关联未清理（孤儿行）：静默跳过
			continue
		}
		var tools []interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
		services = append(services, dto.GroupServiceItem{
			ID:          svc.ID,
			Name:        svc.Name,
			DisplayName: svc.DisplayName,
			Enabled:     gs.Enabled,
			ToolsCount:  len(tools),
		})
	}

	toolsCount, _ := s.getToolsCount(group.ID)

	return &dto.GroupDetail{
		ID:          group.ID,
		Name:        group.Name,
		DisplayName: group.DisplayName,
		Description: group.Description,
		EndpointURL: serverBase() + "/mcp/group/" + group.Name,
		Visibility:   group.Visibility,
		ExposeMode:   group.ExposeMode,
		Services:     services,
		ToolsCount:   toolsCount,
		Status:       group.Status,
	}, nil
}

func (s *GroupService) buildEndpointInfo(group *model.McpGroup) (*dto.EndpointInfo, error) {
	httpURL := serverBase() + "/mcp/group/" + group.Name
	wsURL := serverBase() + "/mcp/ws/group/" + group.Name

	return &dto.EndpointInfo{
		StreamableHTTPURL: httpURL,
		WebSocketURL:      wsURL,
		AuthType:          group.EndpointAuth,
		ConnectionConfig: map[string]interface{}{
			"type":    "streamable-http",
			"url":     httpURL,
			"headers": map[string]string{"X-API-Key": "sk-xxxxxxxxxxxx"},
		},
		McpClientConfig: map[string]interface{}{
			"mcpServers": map[string]interface{}{
				group.Name: map[string]interface{}{
					"type":    "streamable-http",
					"url":     httpURL,
					"headers": map[string]string{"X-API-Key": "sk-xxxxxxxxxxxx"},
				},
			},
		},
	}, nil
}

// servicesByMembership 用单次 IN 查询批量解析成员关系对应的服务，返回 service_id -> *McpService。
// 相比逐条 GetServiceByIDWithoutUser（First）：消除 N+1；且当某成员关系指向已删除服务（孤儿行）
// 时，map 中无该键、返回 nil，由调用方静默跳过——First 在记录不存在时会打 record not found 日志，Find 不会。
func servicesByMembership(members []model.McpGroupService) map[int64]*model.McpService {
	m := make(map[int64]*model.McpService, len(members))
	if len(members) == 0 {
		return m
	}
	ids := make([]int64, 0, len(members))
	for _, gs := range members {
		ids = append(ids, gs.ServiceID)
	}
	svcs, err := model.GetServicesByIDs(ids)
	if err != nil {
		return m
	}
	for i := range svcs {
		m[svcs[i].ID] = &svcs[i]
	}
	return m
}
