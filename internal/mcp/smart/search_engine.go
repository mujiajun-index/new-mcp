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

// resourceMeta/templateMeta/promptMeta 是 resources_cache/prompts_cache 里条目的
// 索引字段(缓存形态见 bridge.fetchResourcesCache/fetchPromptsCache)。
type resourceMeta struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

type templateMeta struct {
	URITemplate string `json:"uriTemplate"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

type promptArgumentMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type promptMeta struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Arguments   []promptArgumentMeta `json:"arguments"`
}

type resourcesCacheDoc struct {
	Resources []resourceMeta `json:"resources"`
	Templates []templateMeta `json:"templates"`
}

// 条目种类,与 model.McpGroupItem.ItemKind 对应(与 handler 包同名常量一致)。
const (
	kindResource = "resource"
	kindTemplate = "template"
	kindPrompt   = "prompt"
)

// gatewayURIScheme 网关资源 URI 前缀,与 handler 包的 gatewayURIScheme 保持一致
// (smart 被 handler 依赖,不能反向 import;改前缀时两处同步)。
const gatewayURIScheme = "newmcp"

func gatewayResourceURI(serviceName, upstreamURI string) string {
	return gatewayURIScheme + "://" + serviceName + "/" + upstreamURI
}

// disabledItemSet 加载分组条目级禁用集合(key "groupID:serviceID:kind:itemKey",
// 与 handler.itemFilter 同构;无行=启用;加载失败返回空集合 fail-open)。
func disabledItemSet(groupIDs []int64, kinds ...string) map[string]bool {
	rows, err := model.GetGroupItemsByGroupIDsAndKinds(groupIDs, kinds)
	if err != nil {
		return nil
	}
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		if !r.Enabled {
			m[itemDisableKey(r.GroupID, r.ServiceID, r.ItemKind, r.ItemKey)] = true
		}
	}
	return m
}

func itemDisableKey(groupID, serviceID int64, kind, key string) string {
	return fmt.Sprintf("%d:%d:%s:%s", groupID, serviceID, kind, key)
}

func itemDisabled(m map[string]bool, groupID, serviceID int64, kind, key string) bool {
	return m[itemDisableKey(groupID, serviceID, kind, key)]
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

	// Parse each service's item caches at most once (a service may appear in several groups).
	resCache := make(map[int64]resourcesCacheDoc, len(resolved))
	parseResources := func(svcID int64, raw string) resourcesCacheDoc {
		if rc, ok := resCache[svcID]; ok {
			return rc
		}
		var rc resourcesCacheDoc
		_ = json.Unmarshal([]byte(raw), &rc)
		resCache[svcID] = rc
		return rc
	}
	prmCache := make(map[int64][]promptMeta, len(resolved))
	parsePrompts := func(svcID int64, raw string) []promptMeta {
		if ps, ok := prmCache[svcID]; ok {
			return ps
		}
		var ps []promptMeta
		_ = json.Unmarshal([]byte(raw), &ps)
		prmCache[svcID] = ps
		return ps
	}

	emitMcp := scope == "" || scope == ScopeService || scope == ScopeAll
	emitTool := scope == ScopeTool || scope == ScopeAll
	emitResource := scope == ScopeResource || scope == ScopeAll
	emitPrompt := scope == ScopePrompt || scope == ScopeAll

	// 条目级禁用过滤与网关 list 同口径,避免勾选掉的条目从搜索侧绕过暴露。
	var disabled map[string]bool
	if emitResource || emitPrompt {
		groupIDs := make([]int64, 0, len(groups))
		for _, g := range groups {
			groupIDs = append(groupIDs, g.ID)
		}
		disabled = disabledItemSet(groupIDs, kindResource, kindTemplate, kindPrompt)
	}

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

		if emitResource {
			rc := parseResources(svc.ID, svc.ResourcesCache)
			for _, r := range rc.Resources {
				// 无 URI 的条目既无法过滤也无法读取,不进搜索索引。
				if r.URI == "" || itemDisabled(disabled, gs.Group.ID, svc.ID, kindResource, r.URI) {
					continue
				}
				docs = append(docs, SearchDoc{
					ID:          "res:" + svc.Name + "/" + r.URI,
					Type:        "resource",
					Name:        r.URI,
					Description: r.Description,
					GroupName:   gs.Group.Name,
					ServiceName: svc.Name,
				})
			}
			for _, tp := range rc.Templates {
				if tp.URITemplate == "" || itemDisabled(disabled, gs.Group.ID, svc.ID, kindTemplate, tp.URITemplate) {
					continue
				}
				docs = append(docs, SearchDoc{
					ID:          "tpl:" + svc.Name + "/" + tp.URITemplate,
					Type:        "template",
					Name:        tp.URITemplate,
					Description: tp.Description,
					GroupName:   gs.Group.Name,
					ServiceName: svc.Name,
				})
			}
		}

		if emitPrompt {
			for _, p := range parsePrompts(svc.ID, svc.PromptsCache) {
				if p.Name == "" || itemDisabled(disabled, gs.Group.ID, svc.ID, kindPrompt, p.Name) {
					continue
				}
				docs = append(docs, SearchDoc{
					ID:          "prompt:" + svc.Name + "." + p.Name,
					Type:        "prompt",
					Name:        p.Name,
					Description: p.Description,
					GroupName:   gs.Group.Name,
					ServiceName: svc.Name,
				})
			}
		}
	}

	// 同一服务挂多个分组时,ResolveEnabledServicesForGroups 按(分组,服务)对返回,
	// 该服务的全部条目会重复入索引;按条目 ID 去重(保留首分组),避免搜索结果出现重复行。
	seen := make(map[string]struct{}, len(docs))
	unique := docs[:0]
	for _, d := range docs {
		if _, ok := seen[d.ID]; ok {
			continue
		}
		seen[d.ID] = struct{}{}
		unique = append(unique, d)
	}
	return unique
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

	// 条目级禁用过滤:归属分组取该 API key 范围内首次包含此服务的分组,
	// 与网关读侧 resolveGroupForService 的"取首分组"口径一致。
	firstGroupBySvc := make(map[int64]int64, len(resolved))
	groupIDSet := make(map[int64]bool, len(resolved))
	var groupIDs []int64
	for _, gs := range resolved {
		if _, ok := firstGroupBySvc[gs.Service.ID]; !ok {
			firstGroupBySvc[gs.Service.ID] = gs.Group.ID
		}
		if !groupIDSet[gs.Group.ID] {
			groupIDSet[gs.Group.ID] = true
			groupIDs = append(groupIDs, gs.Group.ID)
		}
	}
	disabled := disabledItemSet(groupIDs, kindResource, kindTemplate, kindPrompt)

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
			owningGroup := firstGroupBySvc[svc.ID]
			resources, templates := filterResourceItems(svc, owningGroup, disabled)
			prompts := filterPromptItems(svc, owningGroup, disabled)
			results = append(results, map[string]interface{}{
				"type":               "service",
				"name":               svc.Name,
				"display_name":       svc.DisplayName,
				"description":        svc.Description,
				"tools_count":        len(tools),
				"tools":              tools,
				"resources":          resources,
				"resource_templates": templates,
				"prompts":            prompts,
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

// filterResourceItems/filterPromptItems 从缓存取出条目,套用条目级禁用过滤,并把
// URI/名称换成网关命名空间形态(与 resources/list、prompts/list 暴露给客户端的一致,
// mcp.read 直接以此形态回读)。
func filterResourceItems(svc *model.McpService, groupID int64, disabled map[string]bool) (resources, templates []map[string]interface{}) {
	var rc resourcesCacheDoc
	_ = json.Unmarshal([]byte(svc.ResourcesCache), &rc)
	for _, r := range rc.Resources {
		if r.URI == "" || itemDisabled(disabled, groupID, svc.ID, kindResource, r.URI) {
			continue
		}
		resources = append(resources, map[string]interface{}{
			"uri":         gatewayResourceURI(svc.Name, r.URI),
			"name":        r.Name,
			"description": r.Description,
			"mimeType":    r.MimeType,
		})
	}
	for _, t := range rc.Templates {
		if t.URITemplate == "" || itemDisabled(disabled, groupID, svc.ID, kindTemplate, t.URITemplate) {
			continue
		}
		templates = append(templates, map[string]interface{}{
			"uriTemplate": gatewayResourceURI(svc.Name, t.URITemplate),
			"name":        t.Name,
			"description": t.Description,
			"mimeType":    t.MimeType,
		})
	}
	return resources, templates
}

func filterPromptItems(svc *model.McpService, groupID int64, disabled map[string]bool) []map[string]interface{} {
	var prompts []promptMeta
	_ = json.Unmarshal([]byte(svc.PromptsCache), &prompts)
	var out []map[string]interface{}
	for _, p := range prompts {
		if p.Name == "" || itemDisabled(disabled, groupID, svc.ID, kindPrompt, p.Name) {
			continue
		}
		out = append(out, map[string]interface{}{
			"name":        svc.Name + "__" + p.Name,
			"description": p.Description,
			"arguments":   p.Arguments,
		})
	}
	return out
}

// FormatDescribeResult formats describe results into readable text for LLM consumption.
// When includeSchema is false, only names and descriptions are shown (no parameter details).
func FormatDescribeResult(results []map[string]interface{}, includeSchema bool) string {
	if len(results) == 0 {
		return "No matching services or tools found. Targets must be exact service names or \"service.toolName\" entries as returned by mcp.search — call mcp.search to find valid names."
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

			if resources, ok := r["resources"].([]map[string]interface{}); ok && len(resources) > 0 {
				fmt.Fprintf(&sb, "\n## Resources (%d)\n", len(resources))
				for _, item := range resources {
					uri, _ := item["uri"].(string)
					name, _ := item["name"].(string)
					desc, _ := item["description"].(string)
					mime, _ := item["mimeType"].(string)
					label := uri
					if name != "" && name != uri {
						label = fmt.Sprintf("%s (`%s`)", name, uri)
					}
					if mime != "" {
						label += fmt.Sprintf(" [%s]", mime)
					}
					fmt.Fprintf(&sb, "- %s\n", label)
					if desc != "" {
						fmt.Fprintf(&sb, "  %s\n", desc)
					}
				}
			}

			if templates, ok := r["resource_templates"].([]map[string]interface{}); ok && len(templates) > 0 {
				fmt.Fprintf(&sb, "\n## Resource Templates (%d)\n", len(templates))
				for _, item := range templates {
					tpl, _ := item["uriTemplate"].(string)
					name, _ := item["name"].(string)
					desc, _ := item["description"].(string)
					mime, _ := item["mimeType"].(string)
					label := tpl
					if name != "" {
						label = fmt.Sprintf("%s (`%s`)", name, tpl)
					}
					if mime != "" {
						label += fmt.Sprintf(" [%s]", mime)
					}
					fmt.Fprintf(&sb, "- %s\n", label)
					if desc != "" {
						fmt.Fprintf(&sb, "  %s\n", desc)
					}
				}
			}

			if prompts, ok := r["prompts"].([]map[string]interface{}); ok && len(prompts) > 0 {
				fmt.Fprintf(&sb, "\n## Prompts (%d)\n", len(prompts))
				for _, p := range prompts {
					name, _ := p["name"].(string)
					fmt.Fprintf(&sb, "\n### %s\n", name)
					if desc, _ := p["description"].(string); desc != "" {
						fmt.Fprintf(&sb, "%s\n", desc)
					}
					if includeSchema {
						if args, ok := p["arguments"].([]promptArgumentMeta); ok && len(args) > 0 {
							sb.WriteString("Arguments:\n")
							for _, a := range args {
								reqLabel := "optional"
								if a.Required {
									reqLabel = "required"
								}
								fmt.Fprintf(&sb, "- %s (%s)", a.Name, reqLabel)
								if a.Description != "" {
									fmt.Fprintf(&sb, ": %s", a.Description)
								}
								sb.WriteString("\n")
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

// FormatSearchResult 把搜索结果格式化为按类型分节的紧凑文本(mcp.search 的 text
// content)。每节标题一次性给出该类条目的下一步动作(describe/execute/read),行内
// 只保留"ID — 摘要":描述折叠成单行并按 searchDescMaxRunes 截断,防止上游多行/
// 超长描述破坏"一行一条"的列表(细节本就由 mcp.describe 提供)。capped 表示结果数
// 达到 limit 上限,此时在头部提示可能还有更多。
func FormatSearchResult(results []SearchResult, capped bool) string {
	if len(results) == 0 {
		// 空结果不给线索会让模型放弃或反复瞎猜;按 AWS MCP tool design 的建议,
		// 在失败点直接告诉它下一步怎么改("helpful errors can steer the next attempt")。
		return "No results. Try broader, plainer keywords (matched against names and descriptions), set scope to \"all\", or remove the group filter. Call mcp.describe if you already know a service or tool name."
	}

	var sb strings.Builder
	if capped {
		fmt.Fprintf(&sb, "Found %d items (limit reached, more may exist — raise limit or narrow scope):\n", len(results))
	} else {
		fmt.Fprintf(&sb, "Found %d items:\n", len(results))
	}

	type resultSection struct {
		kind   string
		header string
	}
	sections := []resultSection{
		{"mcp", "Services — inspect with mcp.describe \"<name>\":"},
		{"tool", "Tools — call with mcp.execute \"service.tool\":"},
		{"resource", "Resources — fetch with mcp.read (type \"resource\"):"},
		{"template", "Resource templates — fill in the {placeholders} to build a URI, then fetch with mcp.read (type \"resource\"):"},
		{"prompt", "Prompts — render with mcp.read (type \"prompt\"):"},
	}
	// bodies[i] 与 sections[i] 一一对应;末位多留一个给未知类型条目的兜底节。
	bodies := make([]strings.Builder, len(sections)+1)
	fallbackIdx := len(sections)
	sectionIdx := make(map[string]int, len(sections))
	for i, s := range sections {
		sectionIdx[s.kind] = i
	}

	for _, r := range results {
		d := r.Doc
		i, ok := sectionIdx[d.Type]
		if !ok {
			i = fallbackIdx
		}
		body := &bodies[i]

		var id, meta string
		switch d.Type {
		case "mcp":
			// 描述/执行路由按服务名走,名称放最前;display name 不同时以括号补充。
			id = d.ServiceName
			if id == "" {
				id = d.Name
			} else if d.Name != "" && d.Name != d.ServiceName {
				id = d.ServiceName + " (" + d.Name + ")"
			}
			meta = fmt.Sprintf("%d tools", d.ToolCount)
			if d.GroupName != "" {
				meta += ", group: " + d.GroupName
			}
		case "tool":
			id = d.ServiceName + "." + d.Name
		case "resource", "template":
			id = gatewayResourceURI(d.ServiceName, d.Name)
		case "prompt":
			id = d.ServiceName + "__" + d.Name
		default:
			id = d.ID
		}

		desc := singleLineDesc(d.Description, searchDescMaxRunes)
		switch {
		case desc != "" && meta != "":
			fmt.Fprintf(body, "- %s — %s (%s)\n", id, desc, meta)
		case desc != "":
			fmt.Fprintf(body, "- %s — %s\n", id, desc)
		case meta != "":
			fmt.Fprintf(body, "- %s (%s)\n", id, meta)
		default:
			fmt.Fprintf(body, "- %s\n", id)
		}
	}

	for i, s := range sections {
		if bodies[i].Len() == 0 {
			continue
		}
		sb.WriteString("\n")
		sb.WriteString(s.header)
		sb.WriteString("\n")
		sb.WriteString(bodies[i].String())
	}
	if bodies[fallbackIdx].Len() > 0 {
		sb.WriteString("\nItems — use mcp.describe to inspect:\n")
		sb.WriteString(bodies[fallbackIdx].String())
	}
	return sb.String()
}

// searchDescMaxRunes 搜索结果里单条描述的截断上限:描述在搜索阶段只是筛选线索,
// 细节由 mcp.describe 提供,避免单条超长描述(如整段 Query tips)撑爆列表。
const searchDescMaxRunes = 160

// singleLineDesc 把描述里的所有空白(含换行)折叠为单个空格,并按 rune 数截断加省略号,
// 保证"一行一条"的列表格式不被上游多行描述破坏。
func singleLineDesc(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return strings.TrimRight(string(runes[:max-1]), " ,.;:、，。；：") + "…"
}
