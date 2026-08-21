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

var MetaTools = []struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}{
	{
		Name:        "mcp.search",
		Description: "Search available MCP services, tools, resources, and prompts by keyword, group name, or service name. Best for: discovering which MCP server, tool, resource, or prompt can fulfill a task before calling it. Returns: a list of matching items.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search keyword"},
				"scope": {"type": "string", "enum": ["mcp", "tool", "resource", "prompt", "all"], "default": "all", "description": "Search scope: mcp (services), tool, resource (includes templates), prompt, or all"},
				"group": {"type": "string", "description": "Restrict results to a specific group"},
				"limit": {"type": "number", "default": 10, "maximum": 50}
			},
			"required": []
		}`),
	},
	{
		Name:        "mcp.describe",
		Description: "List the tools, resources, and prompts of a given MCP service, or fetch the full parameter schema of a specific tool. Best for: inspecting what a service offers or learning an item's exact shape before calling it. Returns: tool lists and/or full input schemas, plus resource URIs and prompt definitions.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"targets": {"type": "array", "items": {"type": "string"}, "description": "Service name(s) or serviceName.toolName entries"},
				"include_schema": {"type": "boolean", "default": true}
			},
			"required": ["targets"]
		}`),
	},
	{
		Name:        "mcp.execute",
		Description: "Execute a specified MCP tool. Best for: actually calling a tool by id with the right arguments. Returns: the tool's execution result.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"tool_id": {"type": "string", "description": "Format: serviceName.toolName"},
				"arguments": {"type": "object", "description": "Tool arguments"},
				"timeout_ms": {"type": "number", "default": 30000}
			},
			"required": ["tool_id"]
		}`),
	},
	{
		Name:        "mcp.read",
		Description: "Read an MCP resource or get an MCP prompt. Best for: fetching resource content by URI (as shown by mcp.search/mcp.describe, format newmcp://serviceName/...) or rendering a prompt (format serviceName__promptName) with arguments. Returns: the resource contents or prompt messages.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"type": {"type": "string", "enum": ["resource", "prompt"], "description": "Whether target is a resource URI or a prompt name"},
				"target": {"type": "string", "description": "Resource: gateway URI newmcp://<service>/<upstream-uri>; prompt: <service>__<promptName>"},
				"arguments": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Prompt arguments (prompt only)"}
			},
			"required": ["type", "target"]
		}`),
	},
}
