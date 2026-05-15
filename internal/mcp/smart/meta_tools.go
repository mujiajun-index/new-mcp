package smart

import "encoding/json"

var MetaTools = []struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}{
	{
		Name:        "mcp.search",
		Description: "搜索可用的 MCP 服务和工具。支持按关键字、分组名、服务名搜索。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {"type": "string", "description": "搜索关键字"},
				"scope": {"type": "string", "enum": ["mcp", "tool", "all"], "default": "mcp", "description": "搜索范围"},
				"group": {"type": "string", "description": "限定分组"},
				"limit": {"type": "number", "default": 10, "maximum": 50}
			},
			"required": []
		}`),
	},
	{
		Name:        "mcp.describe",
		Description: "查看指定 MCP 服务的工具列表，或指定工具的完整参数 Schema。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"targets": {"type": "array", "items": {"type": "string"}, "description": "服务名或 serviceName.toolName"},
				"include_schema": {"type": "boolean", "default": true}
			},
			"required": ["targets"]
		}`),
	},
	{
		Name:        "mcp.execute",
		Description: "执行指定的 MCP 工具。",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"tool_id": {"type": "string", "description": "格式: 服务名.工具名"},
				"arguments": {"type": "object", "description": "工具参数"},
				"timeout_ms": {"type": "number", "default": 30000}
			},
			"required": ["tool_id"]
		}`),
	},
}
