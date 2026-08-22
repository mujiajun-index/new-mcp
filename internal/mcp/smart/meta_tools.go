package smart

import "encoding/json"

// mcp.search 的 scope 枚举值。resource 同时覆盖静态资源与资源模板两类条目。
const (
	ScopeService  = "mcp"
	ScopeTool     = "tool"
	ScopeResource = "resource"
	ScopePrompt   = "prompt"
	ScopeAll      = "all"
)

// 描述文案遵循的工具提示词最佳实践(Anthropic《Writing effective tools for AI
// agents》/Tool Search Tool 文档、AWS MCP tool design 等):三段式(做什么/何时用/
// 返回什么)、相似工具互相指路(when-NOT-to-use)、参数描述带示例、约束进 schema
// 而非散文、只读元工具标 readOnlyHint。智能模式下这 4 个工具是仅有的常驻上下文
// 的工具定义,是 token 投放回报最高的位置。
var MetaTools = []struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema json.RawMessage        `json:"inputSchema"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
}{
	{
		Name:        "mcp.search",
		Description: "Search the catalog of available MCP services, tools, resources, and prompts by keyword. Plain-language keywords are matched against item names, service names, and descriptions. Returns matching items with their exact IDs: tools as `service.toolName` (call via mcp.execute), resources as `newmcp://service/...` URIs and prompts as `service__promptName` (fetch via mcp.read), each with a short description and group. Use this FIRST when you don't yet know which service or tool fits a task, or to see what exists (omit `query`). If you already know a service or tool name, use mcp.describe instead.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search keywords in plain English, e.g. \"weather forecast\" or \"create issue\". Omit to browse available items."},
				"scope": {"type": "string", "enum": ["mcp", "tool", "resource", "prompt", "all"], "default": "all", "description": "Restrict results to: mcp (services), tool, resource (includes templates), prompt, or all"},
				"group": {"type": "string", "description": "Restrict results to one group by its exact group name"},
				"limit": {"type": "number", "minimum": 1, "maximum": 50, "default": 10, "description": "Max results to return"}
			},
			"required": []
		}`),
		Annotations: map[string]interface{}{"readOnlyHint": true},
	},
	{
		Name:        "mcp.describe",
		Description: "Inspect what a specific MCP service or tool offers, by exact name. Given a service name, returns its full inventory: tools with parameter schemas, resource URIs, and prompts with their arguments. Given `service.toolName`, returns that one tool's description and complete input schema. Use it after mcp.search to drill into a candidate, or right before mcp.execute when you need a tool's exact parameters. For keyword discovery use mcp.search instead.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"targets": {"type": "array", "items": {"type": "string"}, "description": "Exact service names (e.g. \"weather\") or \"service.toolName\" entries (e.g. \"weather.get_forecast\"), as returned by mcp.search"},
				"include_schema": {"type": "boolean", "default": true, "description": "true: include full parameter schemas; false: names and descriptions only (leaner)"}
			},
			"required": ["targets"]
		}`),
		Annotations: map[string]interface{}{"readOnlyHint": true},
	},
	{
		Name:        "mcp.execute",
		Description: "Execute an MCP tool by ID with a JSON arguments object, returning the tool's execution result. The tool_id and the exact arguments it accepts come from mcp.search / mcp.describe — if unsure what arguments a tool takes, run mcp.describe on it first instead of guessing. To fetch a resource or render a prompt, use mcp.read instead.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"tool_id": {"type": "string", "description": "Tool ID in the form service.toolName, e.g. \"weather.get_forecast\""},
				"arguments": {"type": "object", "description": "Arguments object matching the tool's input schema from mcp.describe"},
				"timeout_ms": {"type": "number", "default": 30000, "description": "Execution timeout in milliseconds"}
			},
			"required": ["tool_id"]
		}`),
	},
	{
		Name:        "mcp.read",
		Description: "Read an MCP resource by URI, or render an MCP prompt with arguments. Targets use the exact gateway forms returned by mcp.search / mcp.describe: resources as `newmcp://<service>/<upstream-uri>`, prompts as `<service>__<promptName>` with the arguments the prompt declares. Returns the resource contents or the prompt's rendered messages. For calling tools, use mcp.execute instead.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"type": {"type": "string", "enum": ["resource", "prompt"], "description": "\"resource\" when target is a newmcp:// URI; \"prompt\" when target is a service__promptName"},
				"target": {"type": "string", "description": "Resource example: \"newmcp://weather/docs/cities.json\"; prompt example: \"weather__summarize_forecast\""},
				"arguments": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Prompt arguments as strings, e.g. {\"city\": \"Beijing\"} (prompt only)"}
			},
			"required": ["type", "target"]
		}`),
		Annotations: map[string]interface{}{"readOnlyHint": true},
	},
}
