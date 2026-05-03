# NewMCP 协议适配说明

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

## 1. MCP 协议概述

Model Context Protocol (MCP) 是一个标准化的协议，用于 LLM 应用与外部工具/数据源之间的通信。核心概念：

- **Transport (传输层)**: 客户端和服务器之间的通信方式
- **JSON-RPC 2.0**: MCP 基于 JSON-RPC 2.0 消息格式
- **Primitives (原语)**: Tools (工具)、Resources (资源)、Prompts (提示)

### MCP 协议版本
当前支持: `2025-03-26` 及以上

---

## 2. 支持的传输协议

### 2.1 传输协议对比

| 特性 | stdio | SSE (旧版) | Streamable HTTP | WebSocket |
|------|-------|-----------|-----------------|-----------|
| 方向 | 双向 (stdin/stdout) | 单向推送 + HTTP POST | 双向 (HTTP + SSE) | 双向 |
| 连接 | 本地进程 | HTTP 长连接 | HTTP | TCP 长连接 |
| 会话 | 进程生命周期 | 连接生命周期 | 可选会话 | 连接生命周期 |
| 适用场景 | 本地 MCP 服务 | 旧版兼容 | **推荐** | 设备/实时场景 |
| NewMCP 角色 | 仅上游客户端 | 仅上游客户端 | 服务端 + 客户端 | 服务端 + 客户端 |

### 2.2 Streamable HTTP (主传输协议)

**NewMCP 作为服务端 (暴露给 LLM 客户端):**

```
客户端 → POST /mcp/group/{slug}
请求头:
  Content-Type: application/json
  Accept: application/json, text/event-stream
  MCP-Protocol-Version: 2025-03-26
  X-API-Key: nm-xxx

请求体 (JSON-RPC):
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
        "protocolVersion": "2025-03-26",
        "capabilities": {},
        "clientInfo": {"name": "claude-code", "version": "1.0.0"}
    }
}

响应 (JSON):
{
    "jsonrpc": "2.0",
    "id": 1,
    "result": {
        "protocolVersion": "2025-03-26",
        "capabilities": {"tools": {}},
        "serverInfo": {"name": "newmcp-gateway", "version": "1.0.0"}
    }
}
```

**关键端点行为:**

| 操作 | 方法 | 路径 | 说明 |
|------|------|------|------|
| 初始化 | POST | `/mcp/group/{slug}` | 发送 `initialize` 方法 |
| 工具列表 | POST | `/mcp/group/{slug}` | 发送 `tools/list` 方法 |
| 工具调用 | POST | `/mcp/group/{slug}` | 发送 `tools/call` 方法 |
| SSE 流 | GET | `/mcp/group/{slug}` | 打开服务端推送流 |
| 关闭会话 | DELETE | `/mcp/group/{slug}` | 终止会话 |

**会话管理:**
- 响应头包含 `Mcp-Session-Id: <session-id>`
- 后续请求需携带此 header
- 无状态模式 (`--stateless`): 每个请求独立，不维护会话

### 2.3 WebSocket (设备/实时场景)

**连接建立:**
```
ws://localhost:3000/mcp/ws/group/{slug}
或
wss://your-newmcp.com/mcp/xiaozhi?token=DEVICE_TOKEN
```

**消息格式 (与 Streamable HTTP 相同的 JSON-RPC):**
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
}
```

**NewMCP 实现:**
- 使用 `gorilla/websocket` 库
- WebSocket 连接升级由 Gin 中间件处理
- 消息读写通过 goroutine 并发处理
- 心跳机制: 每 30s 发送 ping，60s 无响应关闭连接

### 2.4 stdio (本地进程)

**仅用于连接上游 MCP 服务 (作为客户端):**

```go
// NewMCP 启动上游 MCP 进程
cmd := exec.Command("npx", "-y", "@anthropic/mcp-server-fetch")
stdin, _ := cmd.StdinPipe()
stdout, _ := cmd.StdoutPipe()
cmd.Start()

// 通过 stdin/stdout 交换 JSON-RPC 消息
```

**生命周期管理:**
- 按需启动: 第一次 `tools/list` 或 `tools/call` 时启动进程
- 空闲超时: 5 分钟无请求则关闭进程
- 进程崩溃: 自动重启，最多 3 次

### 2.5 SSE (旧版兼容)

**仅用于连接上游旧版 MCP 服务:**

```
GET /sse  →  接收 SSE 事件流 (endpoint 事件获取消息 URL)
POST /message  →  发送消息
```

NewMCP 内部实现 SSE 客户端适配器，将旧版 SSE 协议转换为标准 JSON-RPC 调用。

---

## 3. Transport Adapter 实现设计

### 3.1 接口定义

```go
// internal/mcp/transport/transport.go

package transport

type TransportAdapter interface {
    // Connect 建立连接
    Connect(ctx context.Context) error

    // Close 关闭连接
    Close() error

    // Call 发送 JSON-RPC 请求并等待响应
    Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)

    // IsConnected 检查连接状态
    IsConnected() bool

    // GetType 返回传输类型
    GetType() TransportType
}

type TransportType string

const (
    TypeStdio         TransportType = "stdio"
    TypeSSE           TransportType = "sse"
    TypeStreamableHTTP TransportType = "streamable-http"
    TypeWebSocket     TransportType = "websocket"
)
```

### 3.2 StdioAdapter

```go
// internal/mcp/transport/stdio_adapter.go

type StdioAdapter struct {
    config   StdioConfig
    cmd      *exec.Cmd
    stdin    io.WriteCloser
    stdout   io.ReadCloser
    decoder  *json.Decoder
    mu       sync.Mutex
}

type StdioConfig struct {
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env"`
}

func (a *StdioAdapter) Connect(ctx context.Context) error {
    // 启动子进程
    // 建立 stdin/stdout 管道
    // 发送 initialize 请求
}

func (a *StdioAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
    // 序列化 JSON-RPC 请求
    // 写入 stdin
    // 从 stdout 读取响应
}
```

### 3.3 StreamableHTTPAdapter

```go
// internal/mcp/transport/http_adapter.go

type StreamableHTTPAdapter struct {
    config    HTTPConfig
    client    *http.Client
    sessionID string
    baseURL   string
}

type HTTPConfig struct {
    URL     string            `json:"url"`
    Headers map[string]string `json:"headers"`
}

func (a *StreamableHTTPAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
    // 构建 JSON-RPC 请求体
    // 发送 POST 请求到 config.URL
    // 处理 JSON 或 SSE 响应
    // 保存 sessionID
}
```

### 3.4 WebSocketAdapter

```go
// internal/mcp/transport/ws_adapter.go

type WebSocketAdapter struct {
    config  WSConfig
    conn    *websocket.Conn
    mu      sync.Mutex
}

type WSConfig struct {
    URL     string            `json:"url"`
    Headers map[string]string `json:"headers"`
}

func (a *WebSocketAdapter) Connect(ctx context.Context) error {
    // 建立 WebSocket 连接
    // 可选: 添加 headers (通过 websocket.DefaultDialer)
}

func (a *WebSocketAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
    // 序列化 JSON-RPC
    // conn.WriteJSON()
    // conn.ReadJSON() 等待响应 (需要请求-响应匹配)
}
```

---

## 4. 协议桥接机制

### 4.1 桥接流程

NewMCP 在 MCP 客户端和上游 MCP 服务器之间做透明桥接：

```
LLM Client (Streamable HTTP)
    │
    ▼
NewMCP Gateway (MCP Server)
    │ 解析 tools/call 中的工具名
    │ "sea_bot__navigate" → service=sea_bot, tool=navigate
    │
    ▼
ToolRouter → 查找 sea_bot 的 TransportAdapter
    │
    ▼
StdioAdapter.Call("tools/call", {name: "navigate", arguments: {...}})
    │
    ▼
上游 MCP 服务器 (stdio 进程)
```

### 4.2 工具聚合 (tools/list)

```go
// 收到客户端的 tools/list 请求后
func (g *GatewayHandler) HandleToolsList(ctx context.Context, groupID int64) ([]Tool, error) {
    // 1. 查询分组下的所有启用的服务
    services := groupService.GetEnabledServices(groupID)

    var allTools []Tool
    for _, svc := range services {
        // 2. 从缓存获取每个服务的工具列表
        tools := toolCache.Get(svc.ID)

        // 3. 检查工具是否在分组内被禁用
        for _, tool := range tools {
            if groupToolFilter.IsEnabled(groupID, svc.ID, tool.Name) {
                // 4. 添加命名空间前缀
                namespacedTool := Tool{
                    Name:        svc.Name + "__" + tool.Name,
                    Description: tool.Description,
                    InputSchema: tool.InputSchema,
                }
                allTools = append(allTools, namespacedTool)
            }
        }
    }
    return allTools, nil
}
```

### 4.3 工具路由 (tools/call)

```go
func (g *GatewayHandler) HandleToolsCall(ctx context.Context, groupID int64, namespacedName string, arguments json.RawMessage) (json.RawMessage, error) {
    // 1. 解析命名空间: "sea_bot__navigate" → service="sea_bot", tool="navigate"
    serviceName, toolName := parseNamespacedName(namespacedName)

    // 2. 查找服务和对应的 TransportAdapter
    session := sessionPool.Get(groupID, serviceName)
    if session == nil {
        return nil, fmt.Errorf("service %s not found or not connected", serviceName)
    }

    // 3. 通过 TransportAdapter 调用上游
    result, err := session.Adapter.Call(ctx, "tools/call", map[string]interface{}{
        "name":      toolName,
        "arguments": arguments,
    })

    return result, err
}
```

---

## 5. 会话池管理

```go
// internal/mcp/bridge/session_pool.go

type SessionPool struct {
    mu       sync.RWMutex
    sessions map[string]*McpSession  // key: "{groupID}_{serviceName}"
}

type McpSession struct {
    ServiceID   int64
    ServiceName string
    Adapter     transport.TransportAdapter
    Tools       []transport.Tool
    LastUsed    time.Time
    CreatedAt   time.Time
}

// Get 获取或创建到指定服务 (在指定分组内) 的连接
func (p *SessionPool) Get(groupID int64, serviceName string) (*McpSession, error) {
    key := fmt.Sprintf("%d_%s", groupID, serviceName)

    p.mu.RLock()
    session, exists := p.sessions[key]
    p.mu.RUnlock()

    if exists && session.Adapter.IsConnected() {
        session.LastUsed = time.Now()
        return session, nil
    }

    // 需要创建新连接
    return p.createSession(groupID, serviceName)
}
```

---

## 6. 工具目录缓存

```go
// internal/service/registry/tool_cache.go

type ToolCache struct {
    cache map[int64][]Tool  // key: service_id
    mu    sync.RWMutex
    ttl   time.Duration     // 默认 5 分钟
}

// Get 获取缓存，过期则异步刷新
func (c *ToolCache) Get(serviceID int64) []Tool

// ForceRefresh 强制刷新指定服务的工具目录
func (c *ToolCache) ForceRefresh(ctx context.Context, serviceID int64) error
```

---

## 7. 健康检查

```go
// internal/service/registry/health_checker.go

type HealthChecker struct {
    interval time.Duration  // 默认 60s
    pool     *SessionPool
}

func (h *HealthChecker) Start(ctx context.Context) {
    ticker := time.NewTicker(h.interval)
    for {
        select {
        case <-ticker.C:
            h.checkAll(ctx)
        case <-ctx.Done():
            ticker.Stop()
            return
        }
    }
}

func (h *HealthChecker) checkAll(ctx context.Context) {
    // 遍历所有注册的服务
    // 对每个服务调用 initialize 或 tools/list
    // 更新 health_status 字段
}
```
