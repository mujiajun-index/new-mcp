package smart

import "encoding/json"

var MetaTools = []struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}{
	{
		Name:        "mcp.search",
		Description: "Search available MCP services and tools by keyword, group name, or service name. Best for: discovering which MCP server or tool can fulfill a task before calling it. Returns: a list of matching services and/or tools.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "Search keyword"},
				"scope": {"type": "string", "enum": ["mcp", "tool", "all"], "default": "mcp", "description": "Search scope: mcp (services), tool, or all"},
				"group": {"type": "string", "description": "Restrict results to a specific group"},
				"limit": {"type": "number", "default": 10, "maximum": 50}
			},
			"required": []
		}`),
	},
	{
		Name:        "mcp.describe",
		Description: "List the tools of a given MCP service, or fetch the full parameter schema of a specific tool. Best for: inspecting what a service offers or learning a tool's exact arguments before calling it. Returns: tool lists and/or full input schemas.",
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
}
