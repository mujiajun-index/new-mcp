package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
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
	typ             TransportType
	transport       mcp.Transport
	sess            *mcp.ClientSession
	tools           []Tool
	protocolVersion string
	serverInfo      *ServerInfo
	connected       bool
	// dyn 多秘钥动态注入槽位(nil = 静态 headers 行为,单秘钥/stdio)。
	dyn *dynamicSlot
	mu  sync.Mutex
}

// dynamicSlot 描述多秘钥注入位置与供值来源。
type dynamicSlot struct {
	target string // 目标头名(Authorization / X-API-Key / 自定义)
	dyn    DynamicAuth
}

// AdapterOption 是 adapter 构造的可选配置。
type AdapterOption func(*SDKAdapter)

// WithDynamicAuth 为 HTTP 类传输启用多秘钥动态注入:每个上游请求按策略取头值,
// 上游 401/403 时熔断对应秘钥。动态值在静态 headers 之后 Set,覆盖同名静态头。
func WithDynamicAuth(targetHeader string, dyn DynamicAuth) AdapterOption {
	return func(a *SDKAdapter) {
		a.dyn = &dynamicSlot{target: targetHeader, dyn: dyn}
	}
}

// NewStreamableHTTPAdapter 构造 Streamable HTTP 客户端传输。
// 自定义鉴权 header（X-API-Key / Authorization / 自定义头）通过专属 http.Client 注入。
func NewStreamableHTTPAdapter(serviceID int64, url string, headers map[string]string, opts ...AdapterOption) *SDKAdapter {
	_ = serviceID
	a := &SDKAdapter{typ: TypeStreamableHTTP}
	for _, opt := range opts {
		opt(a)
	}
	a.transport = &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: httpClientWithHeaders(headers, a.dyn)}
	return a
}

// NewSSEAdapter 构造 SSE（2024-11-05）客户端传输。
func NewSSEAdapter(serviceID int64, url string, headers map[string]string, opts ...AdapterOption) *SDKAdapter {
	_ = serviceID
	a := &SDKAdapter{typ: TypeSSE}
	for _, opt := range opts {
		opt(a)
	}
	a.transport = &mcp.SSEClientTransport{Endpoint: url, HTTPClient: httpClientWithHeaders(headers, a.dyn)}
	return a
}

// NewStdioAdapter 构造 stdio 客户端传输：以子进程方式运行命令，经 stdin/stdout 通信。
// stdout 上的非 JSON 输出（部分社区服务的启动横幅/日志）会被过滤层丢弃并记日志，
// 避免官方 SDK 的严格 JSON 解析直接断连（如 bazi-mcp 的启动横幅）。
func NewStdioAdapter(serviceID int64, command string, args []string, env map[string]string) *SDKAdapter {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), envToSlice(env)...)
	return &SDKAdapter{
		typ:       TypeStdio,
		transport: NewStdioFilterTransport(cmd, stdioLogf(serviceID, command)),
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

	// 记录握手结果:协商出的协议版本与 serverInfo,供服务详情/测试结果展示真实值。
	if ir := sess.InitializeResult(); ir != nil {
		a.protocolVersion = ir.ProtocolVersion
		if ir.ServerInfo != nil {
			a.serverInfo = &ServerInfo{Name: ir.ServerInfo.Name, Version: ir.ServerInfo.Version}
		}
	}

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
	raw, _, err := a.CallWithMeta(ctx, method, params)
	return raw, err
}

// pickAuthCtx 多秘钥服务在发起一次上游逻辑调用前按策略选 key:放入 ctx 供
// RoundTripper 落到请求头(同一次调用的 POST/GET 全程同一把 key),返回所选
// 序号;无动态槽位(单秘钥/stdio)原样返回 ctx 与 0。
func (a *SDKAdapter) pickAuthCtx(ctx context.Context) (context.Context, int, error) {
	a.mu.Lock()
	dyn := a.dyn
	a.mu.Unlock()
	if dyn == nil {
		return ctx, 0, nil
	}
	idx, value, err := dyn.dyn.Pick()
	if err != nil {
		return ctx, 0, fmt.Errorf("secret pool: %w", err)
	}
	return WithAuthChoice(ctx, idx, value), idx, nil
}

// CallWithMeta 在 Call 基础上带回本次调用所用秘钥的池内序号(MetaCaller)。
// 多秘钥服务在发起 tools/call 前先按策略选好 key 并放入 ctx,由 RoundTripper
// 落到请求头上——同一次调用的所有 HTTP 请求(POST/GET 流)全程同一把 key。
func (a *SDKAdapter) CallWithMeta(ctx context.Context, method string, params interface{}) (json.RawMessage, CallMeta, error) {
	meta := CallMeta{}
	a.mu.Lock()
	sess := a.sess
	a.mu.Unlock()
	if sess == nil {
		return nil, meta, fmt.Errorf("not connected")
	}

	// 当前消费方(网关与服务详情工具测试)仅调用 tools/call;其余方法显式拒绝,避免误用。
	if method != "tools/call" {
		return nil, meta, fmt.Errorf("unsupported method via SDK adapter: %s", method)
	}

	raw, err := json.Marshal(params)
	if err != nil {
		return nil, meta, err
	}
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, meta, err
	}

	var args any
	if len(p.Arguments) > 0 {
		args = json.RawMessage(p.Arguments)
	}
	picked, idx, pErr := a.pickAuthCtx(ctx)
	if pErr != nil {
		return nil, meta, pErr
	}
	ctx = picked
	meta.KeyIndex = idx
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: p.Name, Arguments: args})
	if err != nil {
		return nil, meta, err
	}
	// CallToolResult 的 JSON 形态({content, isError, ...})即 MCP tools/call 的 result。
	out, err := json.Marshal(res)
	return out, meta, err
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

func (a *SDKAdapter) GetProtocolVersion() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.protocolVersion
}

func (a *SDKAdapter) GetServerInfo() *ServerInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.serverInfo
}

// GetStdioProcess 返回 stdio 传输拉起的子进程信息;其余传输类型/未启动返回 nil。
func (a *SDKAdapter) GetStdioProcess() *StdioProcessInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.transport.(*StdioFilterTransport)
	if !ok || st.Command == nil || st.Command.Process == nil {
		return nil
	}
	return &StdioProcessInfo{PID: st.Command.Process.Pid, Command: strings.Join(st.Command.Args, " ")}
}

// --- Resources / Prompts 透传 ---

// session 返回已握手的 ClientSession;未连接时报错。
func (a *SDKAdapter) session() (*mcp.ClientSession, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sess == nil {
		return nil, fmt.Errorf("not connected")
	}
	return a.sess, nil
}

// upstreamCapability 返回上游 initialize 声明的 ServerCapabilities;未拿到时为 nil。
func upstreamCapability(sess *mcp.ClientSession) *mcp.ServerCapabilities {
	if ir := sess.InitializeResult(); ir != nil {
		return ir.Capabilities
	}
	return nil
}

// ListResources 拉取上游 resources/list(迭代器自动翻页)。
// 上游未声明 resources 能力时直接返回空列表,不发多余请求。
func (a *SDKAdapter) ListResources(ctx context.Context) (json.RawMessage, error) {
	sess, err := a.session()
	if err != nil {
		return nil, err
	}
	if caps := upstreamCapability(sess); caps == nil || caps.Resources == nil {
		return json.RawMessage(`{"resources":[]}`), nil
	}
	resources := []*mcp.Resource{}
	for r, err := range sess.Resources(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list resources: %w", err)
		}
		resources = append(resources, r)
	}
	return json.Marshal(map[string]interface{}{"resources": resources})
}

// ListResourceTemplates 拉取上游 resources/templates/list(迭代器自动翻页)。
func (a *SDKAdapter) ListResourceTemplates(ctx context.Context) (json.RawMessage, error) {
	sess, err := a.session()
	if err != nil {
		return nil, err
	}
	if caps := upstreamCapability(sess); caps == nil || caps.Resources == nil {
		return json.RawMessage(`{"resourceTemplates":[]}`), nil
	}
	templates := []*mcp.ResourceTemplate{}
	for t, err := range sess.ResourceTemplates(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list resource templates: %w", err)
		}
		templates = append(templates, t)
	}
	return json.Marshal(map[string]interface{}{"resourceTemplates": templates})
}

// ReadResource 转发 resources/read 到上游,返回完整 result(含 contents)。
func (a *SDKAdapter) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	raw, _, err := a.ReadResourceWithMeta(ctx, uri)
	return raw, err
}

// ReadResourceWithMeta 在 ReadResource 基础上带回本次调用所用秘钥的池内序号
// (ResourceMetaCaller,服务详情页资源测试落日志 key_index 用)。
func (a *SDKAdapter) ReadResourceWithMeta(ctx context.Context, uri string) (json.RawMessage, CallMeta, error) {
	meta := CallMeta{}
	sess, err := a.session()
	if err != nil {
		return nil, meta, err
	}
	picked, idx, pErr := a.pickAuthCtx(ctx)
	if pErr != nil {
		return nil, meta, pErr
	}
	meta.KeyIndex = idx
	res, err := sess.ReadResource(picked, &mcp.ReadResourceParams{URI: uri})
	if err != nil {
		return nil, meta, fmt.Errorf("read resource %s: %w", uri, err)
	}
	out, err := json.Marshal(res)
	return out, meta, err
}

// ListPrompts 拉取上游 prompts/list(迭代器自动翻页)。
func (a *SDKAdapter) ListPrompts(ctx context.Context) (json.RawMessage, error) {
	sess, err := a.session()
	if err != nil {
		return nil, err
	}
	if caps := upstreamCapability(sess); caps == nil || caps.Prompts == nil {
		return json.RawMessage(`{"prompts":[]}`), nil
	}
	prompts := []*mcp.Prompt{}
	for p, err := range sess.Prompts(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list prompts: %w", err)
		}
		prompts = append(prompts, p)
	}
	return json.Marshal(map[string]interface{}{"prompts": prompts})
}

// GetPrompt 转发 prompts/get 到上游,返回完整 result(含 messages)。
func (a *SDKAdapter) GetPrompt(ctx context.Context, name string, arguments map[string]string) (json.RawMessage, error) {
	raw, _, err := a.GetPromptWithMeta(ctx, name, arguments)
	return raw, err
}

// GetPromptWithMeta 在 GetPrompt 基础上带回本次调用所用秘钥的池内序号
// (PromptMetaCaller,服务详情页提示测试落日志 key_index 用)。
func (a *SDKAdapter) GetPromptWithMeta(ctx context.Context, name string, arguments map[string]string) (json.RawMessage, CallMeta, error) {
	meta := CallMeta{}
	sess, err := a.session()
	if err != nil {
		return nil, meta, err
	}
	picked, idx, pErr := a.pickAuthCtx(ctx)
	if pErr != nil {
		return nil, meta, pErr
	}
	meta.KeyIndex = idx
	res, err := sess.GetPrompt(picked, &mcp.GetPromptParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, meta, fmt.Errorf("get prompt %s: %w", name, err)
	}
	out, err := json.Marshal(res)
	return out, meta, err
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
// dyn 非 nil 时启用多秘钥动态注入(见 headerRoundTripper)。
func httpClientWithHeaders(headers map[string]string, dyn *dynamicSlot) *http.Client {
	client := &http.Client{Timeout: 30 * time.Second}
	// 空对象 arguments 兜底对所有上游生效(见 emptyObjectArgsRoundTripper),
	// 自定义 header 再包在外层。
	var rt http.RoundTripper = &emptyObjectArgsRoundTripper{base: http.DefaultTransport}
	if len(headers) > 0 || dyn != nil {
		rt = &headerRoundTripper{base: rt, headers: headers, dyn: dyn}
	}
	client.Transport = rt
	return client
}

// emptyObjectArgsRoundTripper 兜底 TS SDK 系上游的严格校验:prompts/get 与
// tools/call 的 arguments 按 MCP 规范是可选字段,但 typescript-sdk 1.x 的 prompts
// 处理器把缺失的 arguments 当 undefined 交给 zod 校验而拒绝(如 exa 零参数提示
// web_search_help,exa-labs/exa-mcp-server#358、typescript-sdk#1869);而 go-sdk 的
// GetPromptParams.Arguments 带 omitempty,nil/空 map 都序列化不出该字段(v1.7.0 亦然)。
// 在请求体层把缺失的 arguments 补成显式空对象 {}——TS 社区推荐的传输层规范化方案。
type emptyObjectArgsRoundTripper struct {
	base http.RoundTripper
}

func (t *emptyObjectArgsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.Method == http.MethodPost {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			req.Body = io.NopCloser(bytes.NewReader(nil))
			return t.base.RoundTrip(req)
		}
		if patched := withEmptyArgumentsIfMissing(body); patched != nil {
			body = patched
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}
	return t.base.RoundTrip(req)
}

// withEmptyArgumentsIfMissing 仅对 params.arguments 缺失的 prompts/get、tools/call
// 请求体补 "arguments":{};其余情况返回 nil 表示不改。key 顺序变化对 JSON-RPC 无影响。
func withEmptyArgumentsIfMissing(body []byte) []byte {
	var env struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if json.Unmarshal(body, &env) != nil || len(env.Params) == 0 {
		return nil
	}
	if env.Method != "prompts/get" && env.Method != "tools/call" {
		return nil
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	var params map[string]json.RawMessage
	if json.Unmarshal(env.Params, &params) != nil {
		return nil
	}
	if _, has := params["arguments"]; has {
		return nil
	}
	params["arguments"] = json.RawMessage(`{}`)
	patchedParams, err := json.Marshal(params)
	if err != nil {
		return nil
	}
	envelope["params"] = patchedParams
	patched, err := json.Marshal(envelope)
	if err != nil {
		return nil
	}
	return patched
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
	// dyn 多秘钥动态注入槽位(nil = 纯静态 headers)。
	dyn *dynamicSlot
	// 无 ctx 指定的请求(后台 GET 流、会话 DELETE 等)沿用最近一次 POST 选的
	// key,保证无归因请求与上一操作用同一把 key。
	lastMu  sync.Mutex
	lastIdx int
	lastVal string
	lastSet bool
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	if h.dyn == nil {
		return h.base.RoundTrip(clone)
	}

	var idx int
	var val string
	if c, ok := authChoiceFrom(req.Context()); ok {
		// 逻辑调用已选定:key 全程一致(精确归因到 mcp_call_logs.key_index)。
		idx, val = c.index, c.value
	} else if req.Method == http.MethodPost {
		// 后台 POST(initialize/通知/缓存刷新):现选一把。
		i, v, err := h.dyn.dyn.Pick()
		if err != nil {
			return nil, fmt.Errorf("secret pool: %w", err)
		}
		idx, val = i, v
	} else {
		// GET/SSE 流/DELETE:沿用最近值;从未选过(如 SSE 首 GET)则现选。
		h.lastMu.Lock()
		if h.lastSet {
			idx, val = h.lastIdx, h.lastVal
		}
		h.lastMu.Unlock()
		if val == "" {
			i, v, err := h.dyn.dyn.Pick()
			if err != nil {
				return nil, fmt.Errorf("secret pool: %w", err)
			}
			idx, val = i, v
		}
	}
	// 动态值在静态 headers 之后 Set:覆盖同名静态头,防御两处并存。
	clone.Header.Set(h.dyn.target, val)
	if req.Method == http.MethodPost {
		h.lastMu.Lock()
		h.lastIdx, h.lastVal, h.lastSet = idx, val, true
		h.lastMu.Unlock()
	}
	resp, err := h.base.RoundTrip(clone)
	if err == nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		h.dyn.dyn.OnAuthFailure(idx)
	}
	return resp, err
}
