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
}
