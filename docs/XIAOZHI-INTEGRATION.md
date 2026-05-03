# NewMCP 小智对接文档

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

> **注意**: 小智云是 NewMCP 云端主动连接（`cloud_endpoints`）支持的一种 `cloud_type`。本文档描述小智云的对接细节，通用连接管理请参考 API.md 和 ARCHITECTURE.md。

## 1. 概述

小智 (XiaoZhi) 是一个 AI 硬件设备平台。小智云提供 WebSocket MCP 端点，**外部 MCP 服务需要主动连接到这个端点**，向小智云注册工具后，小智设备才能使用。

在 NewMCP 中，小智连接通过通用「云端主动连接」功能管理，`cloud_type` 设为 `xiaozhi`。

### 核心理解

小智云的 WSS 端点 (`wss://api.xiaozhi.me/mcp/?token=...`) 是**注册端点**，不是调用端点。正确的交互方向是：

```
NewMCP ──主动连接──> 小智云 WSS ──注册工具──> 小智设备可调用

小智设备 ──调用工具──> 小智云 ──转发──> NewMCP ──路由──> 上游 MCP 服务 (calculator 等)
```

---

## 2. 小智 MCP 端点

### 2.1 端点格式

```
wss://api.xiaozhi.me/mcp/?token=<JWT_TOKEN>
```

这个端点的含义是：**外部 MCP 服务连接此端点后，小智云会将该服务提供的工具注册到对应的 Agent 下，小智设备即可调用这些工具。**

### 2.2 JWT Token 结构

```json
{
    "userId": 166769,
    "agentId": 104304,
    "endpointId": "agent_104304",
    "purpose": "mcp-endpoint",
    "iat": 1777785320,
    "exp": 1809342920
}
```

**关键字段:**
- `userId`: 小智平台用户 ID
- `agentId`: 小智 Agent ID（决定工具注册到哪个 Agent）
- `endpointId`: 端点标识
- `purpose`: 固定为 `mcp-endpoint`
- `exp`: 过期时间（约 1 年有效期）

### 2.3 MCP 协议交互

小智云通过 WebSocket 传递标准 MCP JSON-RPC 消息。**NewMCP 作为 MCP Server 角色响应小智云的请求：**

```json
// 1. 小智云发来 initialize
→ {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "xiaozhi-cloud", "version": "1.0"}}}

// NewMCP 响应（声明自己是 MCP Server，提供 tools 能力）
← {"jsonrpc": "2.0", "id": 1, "result": {"protocolVersion": "2025-03-26", "capabilities": {"tools": {}}, "serverInfo": {"name": "newmcp-gateway", "version": "1.0.0"}}}

// 2. 小智云请求工具列表
→ {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}

// NewMCP 返回聚合后的工具列表
← {"jsonrpc": "2.0", "id": 2, "result": {"tools": [{"name": "calculator", "description": "数学计算", ...}, {"name": "web_search", "description": "网络搜索", ...}]}}

// 3. 小智设备调用工具（小智云转发）
→ {"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "calculator", "arguments": {"python_expression": "3**5"}}}

// NewMCP 路由到上游 calculator 服务，返回结果
← {"jsonrpc": "2.0", "id": 3, "result": {"content": [{"type": "text", "text": "{\"success\": true, \"result\": 243}"}]}}
```

---

## 3. 核心模式: NewMCP 向小智云提供工具

### 3.1 架构

```
用户操作流程:
  1. 用户创建 MCP 服务 (如 calculator.py, stdio)
  2. 在 NewMCP 平台注册这些 MCP 服务
  3. 在 NewMCP 平台配置小智 WSS 链接 (粘贴从平台复制的链接)
  4. NewMCP 主动连接小智云 WSS，注册聚合工具
  5. 小智设备通过小智云调用工具

数据流:
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│小智设备   │    │小智云     │    │NewMCP    │    │上游 MCP  │
│(ESP32)   │    │api.xiaozhi│   │XiaoZhi   │    │服务      │
│          │    │.me       │    │Client    │    │          │
└────┬─────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │               │
     │               │   ① NewMCP 主动连接 WSS       │
     │               │<──────────────│               │
     │               │               │               │
     │               │   ② initialize (小智云作为客户端)│
     │               │──────────────>│               │
     │               │               │               │
     │               │   ③ 响应 capabilities + tools  │
     │               │<──────────────│               │
     │               │               │               │
     │  ④ 设备语音指令│               │               │
     │──────────────>│               │               │
     │               │               │               │
     │               │  ⑤ 转发 tools/call             │
     │               │──────────────>│               │
     │               │               │               │
     │               │               │  ⑥ 路由到上游  │
     │               │               │──────────────>│
     │               │               │               │
     │               │               │  ⑦ 执行并返回  │
     │               │               │<──────────────│
     │               │               │               │
     │               │  ⑧ 返回结果    │               │
     │               │<──────────────│               │
     │               │               │               │
     │  ⑨ 返回给设备  │               │               │
     │<──────────────│               │               │
```

### 3.2 配置步骤

**暴露模式选择:**

小智端点绑定的 MCP 分组可以选择暴露模式：

| 模式 | 说明 | 适合场景 |
|------|------|----------|
| direct | 直接暴露所有工具（带命名空间前缀） | 工具少（<10），小智设备需要直接调用 |
| smart | 仅暴露 3 个元工具: search/describe/execute | 工具多（10+），或小智设备上下文有限 |

> 建议小智设备默认使用 smart 模式，因为设备上下文有限，不适合一次性加载大量工具 schema。通过 3 个元工具渐进发现和调用更合理。

在创建分组时设置:
```json
{
    "name": "xiaozhi-tools",
    "expose_mode": "smart",  // "direct" 或 "smart"
    ...
}
```

或在分组详情页随时切换暴露模式。

**步骤 1: 用户创建自己的 MCP 服务**

```python
# server.py - 用户自建的 MCP 服务
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("Calculator")

@mcp.tool()
def calculator(python_expression: str) -> dict:
    """数学计算工具"""
    result = eval(python_expression)
    return {"success": True, "result": result}

if __name__ == "__main__":
    mcp.run(transport="stdio")
```

**步骤 2: 在 NewMCP 平台注册 MCP 服务**

```json
POST /api/v1/services
{
    "name": "calculator",
    "display_name": "数学计算器",
    "description": "Python 表达式计算工具",
    "transport_type": "stdio",
    "config": {
        "command": "python",
        "args": ["server.py"]
    }
}
```

NewMCP 启动子进程 → 连接 stdio → 发现 `calculator` 工具 → 缓存工具目录。

**步骤 3: 在 NewMCP 平台配置小智云端连接**

用户从小智云平台复制 WSS 注册链接，在 NewMCP 添加云端连接：

```json
POST /api/v1/connections
{
    "name": "我的小智 Agent",
    "cloud_type": "xiaozhi",
    "wss_url": "wss://api.xiaozhi.me/mcp/?token=eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9...",
    "group_id": 1,
    "auto_connect": true
}
```

- `wss_url`: 从小智云平台复制的完整 WSS 链接（含 token）
- `group_id`: 要向小智设备暴露的 MCP 分组（分组内包含 calculator 等服务）
- `auto_connect`: 是否自动连接并保持长链接

**步骤 4: NewMCP 主动连接小智云**

```go
// NewMCP 启动后自动（或手动触发）连接小智云
func (c *XiaoZhiClient) Connect(ctx context.Context) error {
    // 建立 WebSocket 连接到 wss://api.xiaozhi.me/mcp/?token=JWT
    conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.wssURL, nil)

    // 等待小智云发来 initialize
    // 响应 capabilities (声明提供 tools 能力)
    // 等待小智云发来 tools/list
    // 返回分组内聚合的工具列表
    // 进入消息处理循环，等待 tools/call 请求
}
```

**步骤 5: 小智设备调用工具**

```
小智设备 (语音: "帮我算一下 3 的 5 次方")
    → 小智云 (识别意图 → 调用 MCP 工具 "calculator")
    → NewMCP (接收 tools/call → 路由到 calculator 上游服务)
    → calculator 子进程 (stdio: 执行 3**5 = 243)
    → NewMCP → 小智云 → 小智设备 ("结果是 243")
```

### 3.3 XiaoZhi Client 实现

NewMCP 作为 MCP Server 角色通过 WebSocket 连接到小智云，响应小智云的 MCP 请求：

```go
// internal/mcp/xiaozhi/client.go

// XiaoZhiClient 主动连接小智云 WSS，作为 MCP Server 提供工具
type XiaoZhiClient struct {
    endpointID   int64
    wssURL       string
    groupID      int64
    conn         *websocket.Conn
    sessionPool  *bridge.SessionPool
    toolRouter   *bridge.ToolRouter
    groupService *group.GroupService
    mu           sync.Mutex
    connected    bool
    reconnectCh  chan struct{}
}

// Connect 主动连接小智云 WSS 端点
func (c *XiaoZhiClient) Connect(ctx context.Context) error {
    conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.wssURL, nil)
    if err != nil {
        return fmt.Errorf("connect to xiaozhi cloud failed: %w", err)
    }

    c.conn = conn
    c.connected = true

    // 启动消息处理循环
    go c.messageLoop(ctx)

    return nil
}

// messageLoop 处理小智云发来的 MCP 请求
func (c *XiaoZhiClient) messageLoop(ctx context.Context) {
    defer func() {
        c.connected = false
        c.scheduleReconnect()  // 断线自动重连
    }()

    for {
        select {
        case <-ctx.Done():
            return
        default:
        }

        _, message, err := c.conn.ReadMessage()
        if err != nil {
            return
        }

        var request JSONRPCRequest
        json.Unmarshal(message, &request)

        var response interface{}
        switch request.Method {
        case "initialize":
            response = c.handleInitialize(request)
        case "notifications/initialized":
            continue  // 通知，无需响应
        case "tools/list":
            response = c.handleToolsList(ctx)
        case "tools/call":
            response = c.handleToolsCall(ctx, request)
        default:
            response = c.handleError(request.ID, "method not found")
        }

        if response != nil {
            responseBytes, _ := json.Marshal(response)
            c.mu.Lock()
            c.conn.WriteMessage(websocket.TextMessage, responseBytes)
            c.mu.Unlock()
        }
    }
}

// handleInitialize 响应小智云的 initialize 请求
func (c *XiaoZhiClient) handleInitialize(req JSONRPCRequest) JSONRPCResponse {
    return JSONRPCResponse{
        JSONRPC: "2.0",
        ID:      req.ID,
        Result: map[string]interface{}{
            "protocolVersion": "2025-03-26",
            "capabilities": map[string]interface{}{
                "tools": map[string]interface{}{},
            },
            "serverInfo": map[string]interface{}{
                "name":    "newmcp-gateway",
                "version": "1.0.0",
            },
        },
    }
}

// handleToolsList 根据分组的 expose_mode 返回工具列表
func (c *XiaoZhiClient) handleToolsList(ctx context.Context) JSONRPCResponse {
    group, _ := c.groupService.GetByID(c.groupID)

    switch group.ExposeMode {
    case "smart":
        // Smart 模式: 返回固定的 3 个元工具
        return JSONRPCResponse{
            JSONRPC: "2.0",
            Result: map[string]interface{}{
                "tools": getMetaTools(),
            },
        }
    default:
        // Direct 模式: 返回分组内所有聚合工具
        tools, err := c.groupService.GetAggregatedTools(ctx, c.groupID)
        if err != nil {
            return c.errorResponse(0, err.Error())
        }

        toolList := make([]map[string]interface{}, len(tools))
        for i, t := range tools {
            toolList[i] = map[string]interface{}{
                "name":        t.Name,
                "description": t.Description,
                "inputSchema": t.InputSchema,
            }
        }

        return JSONRPCResponse{
            JSONRPC: "2.0",
            Result: map[string]interface{}{
                "tools": toolList,
            },
        }
    }
}

// handleToolsCall 路由工具调用到上游 MCP 服务
func (c *XiaoZhiClient) handleToolsCall(ctx context.Context, req JSONRPCRequest) JSONRPCResponse {
    params := req.Params.(map[string]interface{})
    toolName := params["name"].(string)
    arguments := params["arguments"]

    // Smart 模式: 处理元工具
    if isMetaTool(toolName) {
        return c.handleSmartTool(ctx, req.ID, toolName, arguments)
    }

    // Direct 模式: 通过 ToolRouter 路由到正确的上游服务
    result, err := c.toolRouter.Call(ctx, c.groupID, toolName, arguments)
    if err != nil {
        return c.errorResponse(req.ID, err.Error())
    }

    return JSONRPCResponse{
        JSONRPC: "2.0",
        ID:      req.ID,
        Result:  result,
    }
}

// handleSmartTool 处理 Smart 模式的元工具
func (c *XiaoZhiClient) handleSmartTool(ctx context.Context, id interface{}, toolName string, arguments interface{}) JSONRPCResponse {
    args := arguments.(map[string]interface{})
    var result interface{}
    var err error

    switch toolName {
    case "mcp.search":
        result, err = c.searchEngine.Search(
            args["query"].(string),
            getString(args, "scope", "mcp"),
            getString(args, "group", ""),
            int(getString(args, "limit", 10).(float64)),
        )
    case "mcp.describe":
        result, err = c.smartHandler.Describe(ctx, c.groupID, args)
    case "mcp.execute":
        result, err = c.smartHandler.Execute(ctx, c.groupID, args)
    default:
        return c.errorResponse(id, "unknown meta tool: "+toolName)
    }

    if err != nil {
        return c.errorResponse(id, err.Error())
    }

    return JSONRPCResponse{
        JSONRPC: "2.0",
        ID:      id,
        Result:  result,
    }
}

// scheduleReconnect 断线自动重连 (指数退避)
func (c *XiaoZhiClient) scheduleReconnect() {
    backoff := time.Second
    maxBackoff := 30 * time.Second

    for {
        time.Sleep(backoff)
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        err := c.Connect(ctx)
        cancel()

        if err == nil {
            return  // 重连成功
        }

        backoff *= 2
        if backoff > maxBackoff {
            backoff = maxBackoff
        }
    }
}
```

### 3.4 生命周期管理

```go
// internal/service/xiaozhi/endpoint_manager.go

// EndpointManager 管理所有小智 WSS 连接
type EndpointManager struct {
    clients map[int64]*XiaoZhiClient  // key: endpoint_id
    mu      sync.RWMutex
}

// StartAll 启动所有已配置的小智 WSS 连接
func (m *EndpointManager) StartAll(ctx context.Context) {
    endpoints := m.loadAutoConnectEndpoints()
    for _, ep := range endpoints {
        client := NewXiaoZhiClient(ep)
        go client.Connect(ctx)
        m.mu.Lock()
        m.clients[ep.ID] = client
        m.mu.Unlock()
    }
}

// StopAll 关闭所有连接
func (m *EndpointManager) StopAll() { ... }
```

---

## 4. 辅助模式: NewMCP 消费小智云的工具

如果小智云本身也提供了一些 MCP 工具，NewMCP 也可以把它们接入平台，供 LLM 客户端使用。

### 4.1 配置方式

将小智 WSS 链接注册为**普通的上游 MCP 服务**：

```json
POST /api/v1/services
{
    "name": "xiaozhi-tools",
    "display_name": "小智云工具",
    "description": "小智云平台自带工具",
    "transport_type": "websocket",
    "config": {
        "url": "wss://api.xiaozhi.me/mcp/?token=eyJhbGciOiJFUzI1NiIs..."
    }
}
```

> 注意: 这种模式下，NewMCP 作为 MCP Client 连接小智云，调用小智云提供的工具。与核心模式（NewMCP 作为 MCP Server 向小智提供工具）方向相反。

### 4.2 两种模式共存

同一个 WSS 链接可以同时配置两种用途：
- **核心模式**: 通过 `POST /api/v1/xiaozhi/endpoints` 配置，NewMCP 向小智暴露工具
- **辅助模式**: 通过 `POST /api/v1/services` 注册，NewMCP 消费小智的工具

实际使用中，核心模式是主要用途。

---

## 5. 注意事项

### 5.1 暴露模式选择

小智端点绑定的分组 `expose_mode` 决定向小智设备暴露工具的方式：

- **smart** (推荐): 小智设备上下文有限，仅暴露 3 个元工具，通过搜索→查看→执行渐进发现
- **direct**: 工具少时可考虑，所有工具直接暴露，调用更快但消耗更多上下文

分组设置中的 `expose_mode` 字段控制此行为，可在前端随时切换。

### 5.2 Token 管理
- 小智 JWT Token 有效期约 1 年，过期后需从小智云平台重新复制
- NewMCP 前端应显示 token 过期时间，提前提醒用户更新
- Token 更新后 NewMCP 自动重连

### 5.3 连接稳定性
- WebSocket 长链接需要心跳保活（每 30s ping）
- 断线自动重连（指数退避: 1s → 2s → 4s → ... → 最大 30s）
- 重连成功后自动重新 initialize，无需人工干预
- 连接状态实时显示在 NewMCP 前端

### 5.4 工具命名策略
- 向小智云暴露工具时，可选择是否带命名空间前缀
- 如果分组内所有服务的工具名无冲突，可以不带前缀（对用户更友好）
- 如果有冲突，自动添加 `服务名__` 前缀

### 5.5 多 Agent 支持
- 用户可能在小智云有多个 Agent（对应不同的 WSS 链接/token）
- NewMCP 可以为每个 Agent 配置不同的分组，暴露不同的工具集

---

## 6. 数据库说明

小智连接使用通用的 `cloud_endpoints` 表，`cloud_type` 设为 `xiaozhi`：

```sql
-- 在 cloud_endpoints 表中，小智连接的字段映射:
-- cloud_type = 'xiaozhi'
-- wss_url = 'wss://api.xiaozhi.me/mcp/?token=...'
-- remote_id = 小智 Agent ID (从 JWT 解析)
-- token_expires_at = JWT 过期时间 (从 JWT 解析)
-- cloud_config = {} (小智无额外配置)
```

完整的 `cloud_endpoints` 表定义见 DATABASE.md。

---

## 7. 测试方案

### 7.1 手动测试

1. 创建一个简单的 MCP 服务 (calculator.py)
2. 在 NewMCP 注册此服务
3. 配置小智 WSS 链接
4. 观察 NewMCP 前端连接状态变为 `connected`
5. 对小智设备说 "帮我算一下 2 加 3"
6. 验证小智设备返回正确结果

### 7.2 集成测试

```go
func TestXiaoZhiClientToolsList(t *testing.T) {
    // 1. 启动 mock WebSocket server (模拟小智云)
    // 2. mock server 发送 initialize
    // 3. 验证 NewMCP 返回正确的 capabilities
    // 4. mock server 发送 tools/list
    // 5. 验证 NewMCP 返回分组内的聚合工具列表
}

func TestXiaoZhiClientToolsCall(t *testing.T) {
    // 1. 启动 mock calculator MCP 服务 (stdio)
    // 2. 注册到 NewMCP
    // 3. 启动 mock 小智云 WS server
    // 4. mock server 发送 tools/call {name: "calculator", ...}
    // 5. 验证 NewMCP 正确路由并返回结果
}

func TestXiaoZhiClientReconnect(t *testing.T) {
    // 1. 建立 WS 连接
    // 2. 强制关闭连接
    // 3. 验证自动重连
    // 4. 验证重连后工具仍可调用
}
```

### 7.3 E2E 测试

1. 部署 NewMCP 实例
2. 配置真实小智 MCP 端点
3. 通过小智设备发起语音指令调用工具
4. 验证完整链路: 设备 → 小智云 → NewMCP → 上游 MCP → 返回
