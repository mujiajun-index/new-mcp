package transport

import (
	"context"
	"encoding/json"
)

type TransportType string

const (
	TypeStdio          TransportType = "stdio"
	TypeSSE            TransportType = "sse"
	TypeStreamableHTTP TransportType = "streamable-http"
	TypeWebSocket      TransportType = "websocket"
	TypePassiveWS      TransportType = "passive-ws"
)

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type TransportAdapter interface {
	Connect(ctx context.Context) error
	Close() error
	Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
	IsConnected() bool
	GetType() TransportType
	GetTools() []Tool

	// 握手信息(上游 initialize result):GetProtocolVersion 为协商出的协议版本,
	// GetServerInfo 为上游 serverInfo(name/version)。未完成握手时分别为空串/nil。
	GetProtocolVersion() string
	GetServerInfo() *ServerInfo

	// Resources / Prompts 透传(网关聚合用)。返回值为 MCP 规范形态的 result JSON:
	// ListResources/ListResourceTemplates/ListPrompts 返回 {"resources":[...]} 等,
	// ReadResource/GetPrompt 返回完整 result;上游未声明对应能力时 List* 返回空列表。
	ListResources(ctx context.Context) (json.RawMessage, error)
	ListResourceTemplates(ctx context.Context) (json.RawMessage, error)
	ReadResource(ctx context.Context, uri string) (json.RawMessage, error)
	ListPrompts(ctx context.Context) (json.RawMessage, error)
	GetPrompt(ctx context.Context, name string, arguments map[string]string) (json.RawMessage, error)
}
