package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// clientImpl 是上报给 MCP 服务端的客户端信息。
var clientImpl = &mcp.Implementation{Name: "newmcp", Version: "1.0.0"}

// SDKAdapter 用官方 Go SDK（github.com/modelcontextprotocol/go-sdk）的
// ClientSession 实现 TransportAdapter，统一支撑 streamable-http / stdio / sse
// 三种客户端传输。协议握手（initialize、notifications/initialized）、
// Mcp-Session-Id、SSE 解析、分页等全部交由 SDK 处理。
type SDKAdapter struct {
	typ       TransportType
	transport mcp.Transport
	sess      *mcp.ClientSession
	tools     []Tool
	connected bool
	mu        sync.Mutex
}

// NewStreamableHTTPAdapter 构造 Streamable HTTP 客户端传输。
// 自定义鉴权 header（X-API-Key / Authorization / 自定义头）通过专属 http.Client 注入。
func NewStreamableHTTPAdapter(serviceID int64, url string, headers map[string]string) *SDKAdapter {
	_ = serviceID
	return &SDKAdapter{
		typ:       TypeStreamableHTTP,
		transport: &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: httpClientWithHeaders(headers)},
	}
}

// NewSSEAdapter 构造 SSE（2024-11-05）客户端传输。
func NewSSEAdapter(serviceID int64, url string, headers map[string]string) *SDKAdapter {
	_ = serviceID
	return &SDKAdapter{
		typ:       TypeSSE,
		transport: &mcp.SSEClientTransport{Endpoint: url, HTTPClient: httpClientWithHeaders(headers)},
	}
}

// NewStdioAdapter 构造 stdio 客户端传输：以子进程方式运行命令，经 stdin/stdout 通信。
func NewStdioAdapter(serviceID int64, command string, args []string, env map[string]string) *SDKAdapter {
	_ = serviceID
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), envToSlice(env)...)
	return &SDKAdapter{
		typ:       TypeStdio,
		transport: &mcp.CommandTransport{Command: cmd},
	}
}

func (a *SDKAdapter) Connect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	client := mcp.NewClient(clientImpl, nil)
	sess, err := client.Connect(ctx, a.transport, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	a.sess = sess

	// SDK 的 Tools 迭代器自动翻页，收集全部工具。取工具失败不视为致命错误，
	// 与既有行为一致（连接成功但工具列表为空）。
	tools := []Tool{}
	for tool, err := range sess.Tools(ctx, nil) {
		if err != nil {
			break
		}
		tools = append(tools, sdkToolToTool(tool))
	}
	a.tools = tools
	a.connected = true
	return nil
}

func (a *SDKAdapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connected = false
	if a.sess != nil {
		err := a.sess.Close()
		a.sess = nil
		return err
	}
	return nil
}

func (a *SDKAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	a.mu.Lock()
	sess := a.sess
	a.mu.Unlock()
	if sess == nil {
		return nil, fmt.Errorf("not connected")
	}

	// 当前消费方（Gateway）仅调用 tools/call；其余方法显式拒绝，避免误用。
	if method != "tools/call" {
		return nil, fmt.Errorf("unsupported method via SDK adapter: %s", method)
	}

	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}

	var args any
	if len(p.Arguments) > 0 {
		args = json.RawMessage(p.Arguments)
	}
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: p.Name, Arguments: args})
	if err != nil {
		return nil, err
	}
	// CallToolResult 的 JSON 形态（{content, isError, ...}）即 MCP tools/call 的 result。
	return json.Marshal(res)
}

func (a *SDKAdapter) IsConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

func (a *SDKAdapter) GetType() TransportType { return a.typ }

func (a *SDKAdapter) GetTools() []Tool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.tools
}

// --- helpers ---

// sdkToolToTool 把 SDK 的 *mcp.Tool 转成本地 Tool。
func sdkToolToTool(t *mcp.Tool) Tool {
	var schema json.RawMessage
	if t.InputSchema != nil {
		if b, err := json.Marshal(t.InputSchema); err == nil {
			schema = b
		}
	}
	return Tool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: schema,
	}
}

func envToSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// httpClientWithHeaders 返回一个会为每个请求附加自定义 header 的 *http.Client，
// 用于在 Streamable HTTP / SSE 传输上注入鉴权信息（SDK 传输本身不暴露 header 入口）。
func httpClientWithHeaders(headers map[string]string) *http.Client {
	client := &http.Client{Timeout: 30 * time.Second}
	if len(headers) > 0 {
		client.Transport = &headerRoundTripper{base: http.DefaultTransport, headers: headers}
	}
	return client
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	return h.base.RoundTrip(clone)
}
