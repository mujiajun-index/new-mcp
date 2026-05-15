package smart

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/mujkjk/newmcp/model"
)

type SearchOptions struct {
	Scope string // "mcp", "tool", "all"
	Group string
	Limit int
}

type SearchEngine struct{}

func NewSearchEngine() *SearchEngine {
	return &SearchEngine{}
}

func (e *SearchEngine) Search(ctx context.Context, apiKeyID int64, query string, opts SearchOptions) ([]SearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 10
	}
	if opts.Limit > 50 {
		opts.Limit = 50
	}

	// Get groups bound to this API key
	var apiKey model.ApiKey
	if err := model.DB.First(&apiKey, apiKeyID).Error; err != nil {
		return nil, err
	}

	var permissions struct {
		Groups []string `json:"groups"`
	}
	_ = json.Unmarshal([]byte(apiKey.Permissions), &permissions)

	// Get services from bound groups
	docs := e.buildSearchDocs(apiKey.UserID, permissions.Groups, opts.Group, opts.Scope)

	if query == "" {
		limit := opts.Limit
		if limit > len(docs) {
			limit = len(docs)
		}
		results := make([]SearchResult, limit)
		for i := 0; i < limit; i++ {
			results[i] = SearchResult{Doc: docs[i], Score: 1.0}
		}
		return results, nil
	}

	idx := buildIndex(docs)
	return idx.search(query, opts.Limit), nil
}

func (e *SearchEngine) buildSearchDocs(userID int64, groups []string, scopeGroup string, scope string) []SearchDoc {
	var docs []SearchDoc

	query := model.DB.Where("user_id = ?", userID)
	if scopeGroup != "" {
		query = query.Where("name = ?", scopeGroup)
	} else if len(groups) > 0 && groups[0] != "*" {
		query = query.Where("name IN ?", groups)
	}

	var mcpGroups []model.McpGroup
	query.Find(&mcpGroups)

	for _, g := range mcpGroups {
		groupServices, _ := model.GetEnabledGroupServices(g.ID)

		for _, gs := range groupServices {
			svc, err := model.GetServiceByIDWithoutUser(gs.ServiceID)
			if err != nil {
				continue
			}

			var tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)

			if scope == "" || scope == "mcp" || scope == "all" {
				docs = append(docs, SearchDoc{
					ID:          "svc:" + svc.Name,
					Type:        "mcp",
					Name:        svc.DisplayName,
					Description: svc.Description,
					GroupName:   g.Name,
					ServiceName: svc.Name,
					ToolCount:   len(tools),
				})
			}

			if scope == "tool" || scope == "all" {
				for _, t := range tools {
					docs = append(docs, SearchDoc{
						ID:          "tool:" + svc.Name + "." + t.Name,
						Type:        "tool",
						Name:        t.Name,
						Description: t.Description,
						GroupName:   g.Name,
						ServiceName: svc.Name,
					})
				}
			}
		}
	}

	return docs
}

// Describe returns details about specific services or tools
func (e *SearchEngine) Describe(targets []string, apiKeyID int64) ([]map[string]interface{}, error) {
	var apiKey model.ApiKey
	if err := model.DB.First(&apiKey, apiKeyID).Error; err != nil {
		return nil, err
	}

	var permissions struct {
		Groups []string `json:"groups"`
	}
	_ = json.Unmarshal([]byte(apiKey.Permissions), &permissions)

	results := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		parts := strings.SplitN(target, ".", 2)
		serviceName := parts[0]

		var svc model.McpService
		if err := model.DB.Where("user_id = ? AND name = ?", apiKey.UserID, serviceName).First(&svc).Error; err != nil {
			continue
		}

		if len(parts) == 1 {
			// Describe the whole service
			var tools []interface{}
			_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
			results = append(results, map[string]interface{}{
				"type":         "service",
				"name":         svc.Name,
				"display_name": svc.DisplayName,
				"description":  svc.Description,
				"tools_count":  len(tools),
				"tools":        tools,
			})
		} else {
			// Describe a specific tool
			toolName := parts[1]
			var tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"inputSchema"`
			}
			_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
			for _, t := range tools {
				if t.Name == toolName {
					results = append(results, map[string]interface{}{
						"type":        "tool",
						"service":     svc.Name,
						"name":        t.Name,
						"description": t.Description,
						"inputSchema": t.InputSchema,
					})
					break
				}
			}
		}
	}
	return results, nil
}
