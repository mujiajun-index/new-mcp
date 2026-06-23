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

// toolMeta is the subset of a cached tool needed for search indexing.
type toolMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
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

	// Best-effort: swallow resolve errors and return no docs, matching prior behavior.
	resolved, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil || len(resolved) == 0 {
		return nil
	}

	// Parse each service's ToolsCache at most once (a service may appear in several groups).
	toolsCache := make(map[int64][]toolMeta, len(resolved))
	parseTools := func(svcID int64, raw string) []toolMeta {
		if tools, ok := toolsCache[svcID]; ok {
			return tools
		}
		var tools []toolMeta
		_ = json.Unmarshal([]byte(raw), &tools)
		toolsCache[svcID] = tools
		return tools
	}

	emitMcp := scope == "" || scope == "mcp" || scope == "all"
	emitTool := scope == "tool" || scope == "all"

	var docs []SearchDoc
	for _, gs := range resolved {
		svc := gs.Service
		tools := parseTools(svc.ID, svc.ToolsCache)

		if emitMcp {
			docs = append(docs, SearchDoc{
				ID:          "svc:" + svc.Name,
				Type:        "mcp",
				Name:        svc.DisplayName,
				Description: svc.Description,
				GroupName:   gs.Group.Name,
				ServiceName: svc.Name,
				ToolCount:   len(tools),
			})
		}

		if emitTool {
			for _, t := range tools {
				docs = append(docs, SearchDoc{
					ID:          "tool:" + svc.Name + "." + t.Name,
					Type:        "tool",
					Name:        t.Name,
					Description: t.Description,
					GroupName:   gs.Group.Name,
					ServiceName: svc.Name,
				})
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

	groups, err := bridge.GetGroupsForApiKey(info)
	if err != nil {
		return nil, err
	}

	// Batch-resolve all services reachable by this APIKey (two queries total).
	resolved, err := model.ResolveEnabledServicesForGroups(groups)
	if err != nil {
		// Best-effort: return empty rather than propagate, matching prior behavior.
		return nil, nil
	}

	// Index allowed services by name and display_name for O(1) target lookup,
	// avoiding one DB query per target. Exact name matches take precedence.
	svcByName := make(map[string]*model.McpService, len(resolved))
	for i := range resolved {
		if name := resolved[i].Service.Name; name != "" {
			svcByName[name] = &resolved[i].Service
		}
	}
	for i := range resolved {
		if dn := resolved[i].Service.DisplayName; dn != "" {
			if _, exists := svcByName[dn]; !exists {
				svcByName[dn] = &resolved[i].Service
			}
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

		svc := svcByName[serviceName]
		if svc == nil {
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
		return "No matching services or tools found."
	}

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		rtype, _ := r["type"].(string)
		switch rtype {
		case "service":
			name, _ := r["name"].(string)
			displayName, _ := r["display_name"].(string)
			tc, _ := r["tools_count"].(int)

			header := displayName
			if displayName != name && name != "" {
				header = fmt.Sprintf("%s (%s)", displayName, name)
			}
			fmt.Fprintf(&sb, "## %s — Tools (%d)\n", header, tc)

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
								sb.WriteString("Parameters:\n")
								sb.WriteString(formatSchemaParams(schema))
							}
						}
					}
				}
			}
		case "tool":
			service, _ := r["service"].(string)
			name, _ := r["name"].(string)
			fmt.Fprintf(&sb, "## %s.%s\n", service, name)
			if desc, ok := r["description"].(string); ok && desc != "" {
				fmt.Fprintf(&sb, "%s\n", desc)
			}
			if includeSchema {
				if schema, ok := r["inputSchema"]; ok && schema != nil {
					sb.WriteString("Parameters:\n")
					sb.WriteString(formatSchemaParams(schema))
				}
			}
		}
	}
	return sb.String()
}

// formatSchemaParams parses a JSON Schema object and formats properties as:
//   - paramName (type, required/optional): description
func formatSchemaParams(schema interface{}) string {
	// Most callers pass a json.RawMessage ([]byte) straight from the service cache;
	// skip the marshal round-trip in that common case.
	var raw []byte
	switch s := schema.(type) {
	case json.RawMessage:
		raw = s
	case []byte:
		raw = s
	case string:
		raw = []byte(s)
	default:
		b, err := json.Marshal(schema)
		if err != nil {
			return ""
		}
		raw = b
	}

	var parsed struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}

	requiredSet := make(map[string]bool, len(parsed.Required))
	for _, r := range parsed.Required {
		requiredSet[r] = true
	}

	var sb strings.Builder
	// Deterministic order via sorted keys
	keys := make([]string, 0, len(parsed.Properties))
	for k := range parsed.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		p := parsed.Properties[k]
		reqLabel := "optional"
		if requiredSet[k] {
			reqLabel = "required"
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
