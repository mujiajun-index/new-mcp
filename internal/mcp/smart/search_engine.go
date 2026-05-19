package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mujkjk/newmcp/internal/mcp/bridge"
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

	info, err := bridge.ResolveApiKeyInfo(apiKeyID)
	if err != nil {
		return nil, err
	}

	docs := e.buildSearchDocs(info, opts.Group, opts.Scope)

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

func (e *SearchEngine) buildSearchDocs(info *bridge.ApiKeyInfo, scopeGroup string, scope string) []SearchDoc {
	groups, err := bridge.GetGroupsForApiKey(info)
	if err != nil || len(groups) == 0 {
		return nil
	}

	if scopeGroup != "" {
		var filtered []model.McpGroup
		for _, g := range groups {
			if g.Name == scopeGroup {
				filtered = append(filtered, g)
				break
			}
		}
		groups = filtered
	}

	var docs []SearchDoc
	for _, g := range groups {
		groupServices, _ := model.GetEnabledGroupServices(g.ID)

		for _, gs := range groupServices {
			svc, svcErr := model.GetServiceByIDWithoutUser(gs.ServiceID)
			if svcErr != nil {
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

// Describe returns details about specific services or tools within the APIKey's group scope.
func (e *SearchEngine) Describe(targets []string, apiKeyID int64) ([]map[string]interface{}, error) {
	info, err := bridge.ResolveApiKeyInfo(apiKeyID)
	if err != nil {
		return nil, err
	}

	// Collect all service IDs accessible by this APIKey
	groups, err := bridge.GetGroupsForApiKey(info)
	if err != nil {
		return nil, err
	}
	allowedSvcIDs := make(map[int64]bool)
	for _, g := range groups {
		gsList, _ := model.GetEnabledGroupServices(g.ID)
		for _, gs := range gsList {
			allowedSvcIDs[gs.ServiceID] = true
		}
	}

	results := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		if target == "" {
			continue
		}
		parts := strings.SplitN(target, ".", 2)
		serviceName := parts[0]
		if serviceName == "" {
			continue
		}

		var svc model.McpService
		if err := model.DB.Where("(name = ? OR display_name = ?) AND user_id = ?", serviceName, serviceName, info.UserID).First(&svc).Error; err != nil {
			continue
		}

		// Check if this service is within the APIKey's group scope
		if !allowedSvcIDs[svc.ID] {
			continue
		}

		if len(parts) == 1 {
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

// FormatDescribeResult formats describe results into readable text for LLM consumption.
// When includeSchema is false, only names and descriptions are shown (no parameter details).
func FormatDescribeResult(results []map[string]interface{}, includeSchema bool) string {
	if len(results) == 0 {
		return "未找到匹配的服务或工具。"
	}

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		rtype, _ := r["type"].(string)
		if rtype == "service" {
			name, _ := r["name"].(string)
			displayName, _ := r["display_name"].(string)
			tc, _ := r["tools_count"].(int)

			header := displayName
			if displayName != name && name != "" {
				header = fmt.Sprintf("%s (%s)", displayName, name)
			}
			fmt.Fprintf(&sb, "## %s 的工具列表 (%d个)\n", header, tc)

			if desc, ok := r["description"].(string); ok && desc != "" {
				fmt.Fprintf(&sb, "%s\n", desc)
			}

			if tools, ok := r["tools"].([]interface{}); ok {
				for _, t := range tools {
					if tm, ok := t.(map[string]interface{}); ok {
						tname, _ := tm["name"].(string)
						tdesc, _ := tm["description"].(string)
						fmt.Fprintf(&sb, "\n### %s\n", tname)
						if tdesc != "" {
							fmt.Fprintf(&sb, "%s\n", tdesc)
						}
						if includeSchema {
							if schema, ok := tm["inputSchema"]; ok && schema != nil {
								sb.WriteString("参数:\n")
								sb.WriteString(formatSchemaParams(schema))
							}
						}
					}
				}
			}
		} else if rtype == "tool" {
			service, _ := r["service"].(string)
			name, _ := r["name"].(string)
			fmt.Fprintf(&sb, "## %s.%s\n", service, name)
			if desc, ok := r["description"].(string); ok && desc != "" {
				fmt.Fprintf(&sb, "%s\n", desc)
			}
			if includeSchema {
				if schema, ok := r["inputSchema"]; ok && schema != nil {
					sb.WriteString("参数:\n")
					sb.WriteString(formatSchemaParams(schema))
				}
			}
		}
	}
	return sb.String()
}

// formatSchemaParams parses a JSON Schema object and formats properties as:
//   - paramName (type, 必填/可选): description
func formatSchemaParams(schema interface{}) string {
	raw, err := json.Marshal(schema)
	if err != nil {
		return ""
	}

	var s struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}

	requiredSet := make(map[string]bool, len(s.Required))
	for _, r := range s.Required {
		requiredSet[r] = true
	}

	var sb strings.Builder
	// Deterministic order via sorted keys
	keys := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		p := s.Properties[k]
		reqLabel := "可选"
		if requiredSet[k] {
			reqLabel = "必填"
		}
		typeStr := p.Type
		if typeStr == "" {
			typeStr = "any"
		}
		fmt.Fprintf(&sb, "- %s (%s, %s)", k, typeStr, reqLabel)
		if p.Description != "" {
			fmt.Fprintf(&sb, ": %s", p.Description)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
