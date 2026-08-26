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

// StdioProcessInfo 标识 stdio 传输拉起的本地子进程,供进程资源监控定位进程树。
type StdioProcessInfo struct {
	PID     int
	Command string // 完整命令行(空格拼接)
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

	// GetStdioProcess 返回 stdio 传输拉起的子进程信息(PID + 完整命令行),
	// 供服务详情页展示进程资源占用。非 stdio 类型或进程尚未 Start 时返回 nil;
	// 进程已退出时仍可能非 nil,存活判定交给采集层(CollectProcessTreeStat)。
	GetStdioProcess() *StdioProcessInfo

	// Resources / Prompts 透传(网关聚合用)。返回值为 MCP 规范形态的 result JSON:
	// ListResources/ListResourceTemplates/ListPrompts 返回 {"resources":[...]} 等,
	// ReadResource/GetPrompt 返回完整 result;上游未声明对应能力时 List* 返回空列表。
	ListResources(ctx context.Context) (json.RawMessage, error)
	ListResourceTemplates(ctx context.Context) (json.RawMessage, error)
	ReadResource(ctx context.Context, uri string) (json.RawMessage, error)
	ListPrompts(ctx context.Context) (json.RawMessage, error)
	GetPrompt(ctx context.Context, name string, arguments map[string]string) (json.RawMessage, error)
}
