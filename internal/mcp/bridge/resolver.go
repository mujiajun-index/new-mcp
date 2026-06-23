package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/model"
)

// ApiKeyInfo holds resolved APIKey metadata.
type ApiKeyInfo struct {
	ApiKeyID   int64
	UserID     int64
	Username   string
	ApiKeyName string
	GroupNames []string // ["*"] means all groups for this user
}

// ResolveApiKeyInfo fetches APIKey + User info in one pass.
func ResolveApiKeyInfo(apiKeyID int64) (*ApiKeyInfo, error) {
	var apiKey model.ApiKey
	if err := model.DB.First(&apiKey, apiKeyID).Error; err != nil {
		return nil, err
	}

	var user model.User
	if err := model.DB.Select("username").First(&user, apiKey.UserID).Error; err != nil {
		return nil, err
	}

	var perms struct {
		Groups []string `json:"groups"`
	}
	_ = json.Unmarshal([]byte(apiKey.Permissions), &perms)

	return &ApiKeyInfo{
		ApiKeyID:   apiKey.ID,
		UserID:     apiKey.UserID,
		Username:   user.Username,
		ApiKeyName: apiKey.Name,
		GroupNames: perms.Groups,
	}, nil
}

// GetGroupsForApiKey returns the McpGroup list accessible by the given APIKey.
// Handles groups: ["*"] by returning all groups for the user.
func GetGroupsForApiKey(info *ApiKeyInfo) ([]model.McpGroup, error) {
	if len(info.GroupNames) == 0 {
		return nil, nil
	}

	query := model.DB.Where("user_id = ?", info.UserID)
	if info.GroupNames[0] != "*" {
		query = query.Where("name IN ?", info.GroupNames)
	}

	var groups []model.McpGroup
	if err := query.Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// ToolEntry is a namespaced tool ready for exposure.
type ToolEntry struct {
	ServiceID   int64
	ServiceName string
	ToolName    string
	Tool        map[string]interface{}
}

// CollectToolsForGroups aggregates all enabled tools from the given groups.
// If dedup is true, the same service appearing in multiple groups is only counted once.
//
// It batches all DB access: group->service memberships and tool-level filters are each
// fetched in a single query, and services in a single query (via ResolveEnabledServicesForGroups).
func CollectToolsForGroups(groups []model.McpGroup, dedup bool) ([]ToolEntry, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	// Batched: all (group, service) pairs via two queries.
	pairs, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil || len(pairs) == 0 {
		// Best-effort: return empty rather than propagate, matching prior behavior.
		return nil, nil
	}

	// Batched: tool-level filters for all groups in one query, indexed by group.
	groupIDs := make([]int64, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.ID
	}
	allFilters, _ := model.GetGroupToolsByGroupIDs(groupIDs)
	filterByGroup := make(map[int64]map[string]bool, len(groups))
	for _, f := range allFilters {
		fm := filterByGroup[f.GroupID]
		if fm == nil {
			fm = make(map[string]bool)
			filterByGroup[f.GroupID] = fm
		}
		fm[fmt.Sprintf("%d:%s", f.ServiceID, f.ToolName)] = f.Enabled
	}

	seen := make(map[int64]bool)
	var allTools []ToolEntry

	for _, p := range pairs {
		svc := p.Service
		if dedup && seen[svc.ID] {
			continue
		}
		seen[svc.ID] = true

		filterMap := filterByGroup[p.Group.ID]

		var tools []map[string]interface{}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

		for _, t := range tools {
			name, _ := t["name"].(string)
			if filterMap != nil {
				if enabled, ok := filterMap[fmt.Sprintf("%d:%s", svc.ID, name)]; ok && !enabled {
					continue
				}
			}
			t["name"] = svc.Name + "__" + name
			allTools = append(allTools, ToolEntry{
				ServiceID:   svc.ID,
				ServiceName: svc.Name,
				ToolName:    name,
				Tool:        t,
			})
		}
	}

	return allTools, nil
}

// HasGroupAccess checks if the APIKey has access to a specific group by name.
func HasGroupAccess(info *ApiKeyInfo, groupName string) bool {
	for _, g := range info.GroupNames {
		if g == "*" || g == groupName {
			return true
		}
	}
	return false
}

// ToolsToMaps extracts the tool map entries from ToolEntry slice.
func ToolsToMaps(entries []ToolEntry) []map[string]interface{} {
	maps := make([]map[string]interface{}, len(entries))
	for i, e := range entries {
		maps[i] = e.Tool
	}
	return maps
}
