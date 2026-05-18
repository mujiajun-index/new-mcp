# NewMCP 技术架构文档

> 版本: V1.1 | 状态: 草案 | 更新日期: 2026-05-15

## 1. 架构概述

### 1.1 架构风格
**模块化单体 (Modular Monolith)**，与 NewAPI 同架构。

- 单个 Go 二进制文件，内嵌编译后的 React 前端
- 默认 SQLite 存储，可选 MySQL/PostgreSQL + Redis
- 模块间通过 Go interface 解耦，未来可拆分为微服务

### 1.2 技术栈

| 层 | 技术 | 说明 |
|----|------|------|
| 后端 | Go 1.22+ / Gin | HTTP 框架，高并发网关 |
| ORM | GORM | 支持 SQLite/MySQL/PostgreSQL |
| 缓存 | Redis (可选) / 内存缓存 | 工具目录缓存、会话存储 |
| 认证 | JWT + API Key | 双重认证机制 |
| WebSocket | gorilla/websocket | 设备长链接、MCP WS 传输 |
| MCP 协议 | go-sdk/mcp + 自定义适配器 | 官方 SDK + 协议桥接 |
| 前端 | React 19 + Vite + Radix UI + Tailwind CSS | 管理界面 |
| 路由 | TanStack Router | 类型安全路由 |
| 数据请求 | TanStack Query | 服务端状态管理 |
| 部署 | Docker / 单二进制 | 灵活部署选项 |

---

## 2. 系统组件图

```
┌─────────────────────────────────────────────────────────────────┐
│                     NewMCP Platform                             │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Gin HTTP Server                       │   │
│  │                                                          │   │
│  │  ┌──────────────┐  ┌──────────────────────────┐  ┌──────────────┐  │   │
│  │  │  REST API    │  │  MCP Gateway             │  │  WebSocket   │  │   │
│  │  │  /api/v1/*   │  │  /mcp        → Direct    │  │  /mcp/ws     → Direct│   │
│  │  │              │  │  /smart/mcp  → Smart     │  │  /smart/mcp/ws → Smart│   │
│  │  │              │  │  /mcp/group/* → 按配置    │  │  /mcp/ws/group/* → 按配置 │   │
│  │  └──────┬───────┘  └──────┬───────────────────┘  └──────┬───────┘  │   │
│  └─────────┼─────────────────┼─────────────────┼──────────┘   │
│            │                 │                 │               │
│  ┌─────────┴─────────────────┴─────────────────┴──────────┐   │
│  │                    Middleware Layer                      │   │
│  │     Auth │ CORS │ RateLimit │ Logger │ GroupResolver    │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │                    Controller Layer                      │   │
│  │  Auth │ Service │ Group │ Connection │ Vision │ Camera  │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │                    Service Layer                         │   │
│  │                                                          │   │
│  │  ┌────────────┐  ┌────────────┐  ┌──────────────────┐  │   │
│  │  │ Registry   │  │ Group      │  │ Bridge           │  │   │
│  │  │ - 注册     │  │ - 分组管理 │  │ - 协议转换       │  │   │
│  │  │ - 发现     │  │ - 工具聚合 │  │ - 工具路由       │  │   │
│  │  │ - 健康检查 │  │ - 过滤控制 │  │ - 会话池管理     │  │   │
│  │  └────────────┘  └────────────┘  └──────────────────┘  │   │
│  │  ┌────────────┐  ┌────────────┐  ┌──────────────────┐  │   │
│  │  │ Smart      │  │ Cloud      │  │ Virtual          │  │   │
│  │  │ - BM25搜索 │  │ - 主动连接 │  │ - VirtualToolReg │  │   │
│  │  │ - 元工具   │  │ - 多平台   │  │ - VisionClient   │  │   │
│  │  │ - 范围收敛 │  │ - 状态监控 │  │ - CameraStream   │  │   │
│  │  └────────────┘  └────────────┘  └──────────────────┘  │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │              MCP Protocol Layer                          │   │
│  │                                                          │   │
│  │  ┌──────────────────────────────────────────────────┐  │   │
│  │  │  Transport Adapters                               │  │   │
│  │  │  Stdio │ SSE │ HTTP │ WS │ PassiveWS │ Virtual                    │  │
│  │  └──────────────────────────────────────────────────┘  │   │
│  │                                                          │   │
│  │  ┌──────────────┐  ┌──────────────┐                     │   │
│  │  │ Session Pool │  │ Tool Router  │                     │   │
│  │  └──────────────┘  └──────────────┘                     │   │
│  └──────────────────────────────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │              Data Access Layer (GORM)                    │   │
│  └───────┬──────────────────┬──────────────────┬───────────┘   │
│      SQLite            MySQL/PG             Redis              │
└─────────────────────────────────────────────────────────────────┘
```

---

## 3. 核心数据流

### 3.1 LLM 客户端调用工具

```
时序图: LLM 客户端通过 NewMCP 调用工具

┌────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│LLM     │    │NewMCP    │    │Group     │    │Transport │    │上游 MCP  │
│Client  │    │Gateway   │    │Service   │    │Adapter   │    │Server    │
└───┬────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
    │              │               │               │               │
    │ POST /mcp/group/robot        │               │               │
    │ {method:     │               │               │               │
    │  "tools/list"}               │               │               │
    │─────────────>│               │               │               │
    │              │               │               │               │
    │              │ 解析分组 slug  │               │               │
    │              │──────────────>│               │               │
    │              │               │               │               │
    │              │               │ 查询分组下的服务列表          │
    │              │               │───┐           │               │
    │              │               │<──┘           │               │
    │              │               │               │               │
    │              │ 对每个服务:    │               │               │
    │              │ 获取缓存工具目录               │               │
    │              │<──────────────│               │               │
    │              │               │               │               │
    │              │ 聚合工具列表(命名空间前缀)      │               │
    │              │───┐           │               │               │
    │              │<──┘           │               │               │
    │              │               │               │               │
    │ 返回聚合工具列表              │               │               │
    │<─────────────│               │               │               │
    │              │               │               │               │
    │ POST /mcp/group/robot        │               │               │
    │ {method:     │               │               │               │
    │  "tools/call",│              │               │               │
    │  tool: "sea_bot__navigate"}  │               │               │
    │─────────────>│               │               │               │
    │              │               │               │               │
    │              │ 解析工具名:    │               │               │
    │              │ sea_bot → service_id=3         │               │
    │              │ navigate → tool                │               │
    │              │               │               │               │
    │              │ 获取 service 的 transport adapter              │
    │              │───────────────────────────────>│               │
    │              │               │               │               │
    │              │               │     转发 JSON-RPC              │
    │              │               │               │──────────────>│
    │              │               │               │               │
    │              │               │               │  执行工具     │
    │              │               │               │<──────────────│
    │              │               │               │               │
    │              │               │<──────────────│               │
    │              │<───────────────────────────────│               │
    │              │               │               │               │
    │ 返回结果     │               │               │               │
    │<─────────────│               │               │               │
```

### 3.2 云端主动连接（NewMCP 向远端平台暴露工具）

> **关键**: NewMCP **主动连接**远端云平台 WSS 端点（如小智云），作为 MCP Server 注册工具。远端设备通过云平台调用工具时，云平台转发到 NewMCP。

```
时序图: 远端设备通过 NewMCP 调用工具

┌────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│远端    │    │云平台    │    │NewMCP    │    │上游 MCP  │
│设备    │    │(如小智云)│    │Cloud     │    │服务      │
└───┬────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
    │              │               │               │
    │              │  ① NewMCP 主动连接 WSS 端点   │
    │              │<──────────────│               │
    │              │               │               │
    │              │  ② 云平台作为 MCP Client       │
    │              │  发送 initialize               │
    │              │──────────────>│               │
    │              │               │               │
    │              │  ③ NewMCP 响应 capabilities    │
    │              │  (声明提供 tools 能力)          │
    │              │<──────────────│               │
    │              │               │               │
    │              │  ④ 云平台请求 tools/list       │
    │              │──────────────>│               │
    │              │               │               │
    │              │  ⑤ 返回聚合工具列表             │
    │              │<──────────────│               │
    │              │               │               │
    │  ⑥ 设备调用工具               │               │
    │─────────────>│               │               │
    │              │               │               │
    │              │  ⑦ 转发 tools/call             │
    │              │──────────────>│               │
    │              │               │               │
    │              │               │  ⑧ 路由到上游  │
    │              │               │──────────────>│
    │              │               │               │
    │              │               │  ⑨ 执行并返回  │
    │              │               │<──────────────│
    │              │               │               │
    │              │  ⑩ 返回结果    │               │
    │              │<──────────────│               │
    │              │               │               │
    │  ⑪ 返回给设备 │               │               │
    │<─────────────│               │               │
```

### 3.3 NewMCP 连接模型

NewMCP 支持两种连接方向:

**被动连接（LLM 客户端连入）:**
- `http://` 或 `https://` → Streamable HTTP（主要方式）
- `ws://` 或 `wss://` → WebSocket

**主动连接（NewMCP 连出）:**
- NewMCP 主动连接远端云平台 WSS（如小智云等），作为 MCP Server 向远端注册工具
- 支持 `cloud_type` 区分不同平台（xiaozhi、custom）

**被动接入（外部 MCP 服务连入）:**
- NewMCP 生成 WSS 接入点 URL: `wss://api.newmcp.pro/mcp/passive/?token=JWT`
- 外部 MCP 服务（如远程 calculator、自定义服务）连接此 URL，向 NewMCP 注册工具
- 类似小智云的注册模式：服务提供方连接平台，而非平台连接服务
- 适合外部服务在 NAT/防火墙后的场景（服务主动连出）

### 3.4 虚拟工具调用流程（Vision/Camera）

```
时序图: MCP 客户端调用虚拟工具

┌────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│MCP     │    │Gateway   │    │Virtual   │    │Vision/   │    │外部 AI   │
│Client  │    │Handler   │    │Registry  │    │Camera    │    │API       │
│        │    │          │    │          │    │Handler   │    │(OpenAI等)│
└───┬────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
    │              │               │               │               │
    │ POST /mcp/group/xxx          │               │               │
    │ {method: "tools/call",       │               │               │
    │  name: "vision_1.analyze_image"}             │               │
    │─────────────>│               │               │               │
    │              │               │               │               │
    │              │ 解析: vision_1 = serviceName   │               │
    │              │ 查询 mcp_services              │               │
    │              │ (transport_type="virtual")     │               │
    │              │───┐           │               │               │
    │              │<──┘           │               │               │
    │              │               │               │               │
    │              │ VirtualRegistry.Handle()       │               │
    │              │──────────────>│               │               │
    │              │               │               │               │
    │              │               │ 查找 handler (serviceID=5)    │
    │              │               │──────────────>│               │
    │              │               │               │               │
    │              │               │               │ 查 VisionConfig│
    │              │               │               │───┐           │
    │              │               │               │<──┘           │
    │              │               │               │               │
    │              │               │               │ 调用 AI API   │
    │              │               │               │──────────────>│
    │              │               │               │               │
    │              │               │               │  返回识别结果  │
    │              │               │               │<──────────────│
    │              │               │               │               │
    │              │               │<──────────────│               │
    │              │<──────────────│               │               │
    │              │               │               │               │
    │ 返回结果     │               │               │               │
    │<─────────────│               │               │               │
```

### 3.5 摄像头帧推送与调用流程

```
时序图: 浏览器推流 + MCP 工具调用摄像头

┌────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│浏览器  │    │NewMCP    │    │Camera    │    │MCP       │    │AI API    │
│摄像头  │    │WebSocket │    │Stream    │    │Client    │    │(Vision)  │
└───┬────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
    │              │               │               │               │
    │ ① WebSocket 连接             │               │               │
    │ /api/v1/cameras/1/stream     │               │               │
    │─────────────>│               │               │               │
    │              │               │               │               │
    │ ② canvas 截帧 (JPEG)         │               │               │
    │ (每2秒)      │               │               │               │
    │─────────────>│               │               │               │
    │              │               │               │               │
    │              │ HandleFrame()  │               │               │
    │              │──────────────>│               │               │
    │              │               │               │               │
    │              │               │ 缓存最新帧    │               │
    │              │               │───┐           │               │
    │              │               │<──┘           │               │
    │              │               │               │               │
    │ ③ MCP Client 调用 camera.capture              │               │
    │              │               │               │               │
    │              │<──────────────────────────────│               │
    │              │               │               │               │
    │              │ GetLatestFrame()               │               │
    │              │──────────────>│               │               │
    │              │               │               │               │
    │              │<──────────────│               │               │
    │              │ (返回 base64)  │               │               │
    │              │──────────────────────────────>│               │
    │              │               │               │               │
    │ ④ MCP Client 调用 camera.analyze             │               │
    │              │               │               │               │
    │              │<──────────────────────────────│               │
    │              │               │               │               │
    │              │ GetLatestFrame() + VisionClient│               │
    │              │──────────────>│               │               │
    │              │               │──────────────────────────────>│
    │              │               │               │               │
    │              │               │<──────────────────────────────│
    │              │<──────────────│               │               │
    │              │──────────────────────────────>│               │
```

---

## 4. 模块详细设计

### 4.1 Transport Adapter 接口

```go
// internal/mcp/transport/transport.go

// TransportAdapter 定义 MCP 传输协议适配器接口
// 注意: 所有方法必须支持并发安全调用
type TransportAdapter interface {
    // Connect 建立到上游 MCP 服务器的连接
    Connect(ctx context.Context) error
    // Close 关闭连接
    Close() error
    // Call 发送 MCP JSON-RPC 请求并返回响应
    // method: MCP 方法名 (如 "tools/list", "tools/call")
    // params: 请求参数 (map 或 struct)
    Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
    // IsConnected 检查连接状态
    IsConnected() bool
    // GetType 返回传输类型
    GetType() TransportType
}

// TransportType 传输类型枚举
type TransportType string
const (
    TypeStdio          TransportType = "stdio"           // 本地子进程
    TypeSSE            TransportType = "sse"             // 主动连接远程 SSE
    TypeStreamableHTTP TransportType = "streamable-http" // 主动连接远程 HTTP
    TypeWebSocket      TransportType = "websocket"       // 主动连接远程 WSS
    TypePassiveWS      TransportType = "passive-ws"      // 被动: 外部服务连入
)
```

### 4.2 Session Pool 设计

```go
// internal/mcp/bridge/session_pool.go

// SessionPool 管理到上游 MCP 服务器的连接池
type SessionPool struct {
    mu          sync.RWMutex
    sessions    map[int64]*McpSession  // key: service_id
    idleTimeout time.Duration          // 空闲超时，默认 10 分钟
    maxRetries  int                    // 最大重连次数，默认 5
}

// McpSession 代表一个到上游 MCP 服务器的活跃连接
type McpSession struct {
    ServiceID   int64
    Adapter     TransportAdapter
    Tools       []Tool
    LastUsed    time.Time
    LastRefresh time.Time
    Health      HealthStatus
    failCount   int                   // 连续失败计数（用于熔断）
}

// GetOrConnect 获取已有 session 或按需创建新连接
func (p *SessionPool) GetOrConnect(ctx context.Context, serviceID int64) (*McpSession, error)
```

> **Session 生命周期管理**:
> - **空闲淘汰**: stdio 子进程 session 空闲超过 `idleTimeout` 后自动关闭，释放系统资源
> - **健康检查**: 每 60 秒 ping 所有活跃 session，失败标记为 unhealthy
> - **自动重连**: unhealthy session 按指数退避重连（1s, 2s, 4s, ..., 最大 30s），连续失败 5 次后标记为 dead
> - **熔断机制**: 连续 3 次 `Call()` 失败的 session 自动熔断，路由绕过该 session
> - **线程安全**: TransportAdapter 的 `Call()` 方法必须支持并发调用

### 4.3 Smart 模式搜索引擎

```go
// internal/mcp/smart/search_engine.go

// SearchEngine 自实现 BM25Okapi 搜索 (参考 mcp-gateway MiniSearch)
// 搜索范围通过 API Key → 分组 → MCP 服务 自然收敛 (5-200 条)
type SearchEngine struct {
    docs     []SearchDoc
    index    *bm25Index
    mu       sync.RWMutex
}

// Search 在 API Key 绑定的分组范围内 BM25 搜索
func (e *SearchEngine) Search(ctx context.Context, store Store, apiKeyID int64, query string, opts SearchOptions) ([]SearchResult, error)
```

> 零外部依赖，BM25Okapi + 字段权重 + Levenshtein 模糊匹配。详见 MCP-PROTOCOL.md 第 8 节。

### 4.4 双模式分发器

```go
// GatewayHandler 根据端点路由分发请求

// 端点驱动模式选择:
//   POST /mcp              → 固定 Direct 模式（聚合 API Key 所有分组，去重暴露全部工具）
//   POST /smart/mcp        → 固定 Smart 模式（仅暴露 3 个元工具）
//   POST /mcp/group/{slug} → 按分组的 expose_mode 决定:
//     Direct 模式: 聚合分组内所有工具，命名空间前缀 (serviceName__toolName，双下划线)
//     Smart  模式: 只暴露 3 个固定元工具 (mcp.search, mcp.describe, mcp.execute)
//                  mcp.execute 的 tool_id 使用 serviceName.toolName 格式（点号）

// 无 slug 时根据 logCtx.ExposeMode 决定模式
func (h *GatewayHandler) handleToolsList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
    if logCtx.GroupSlug == "" {
        if logCtx.ExposeMode == "direct" {
            tools, _ := h.getDirectToolsForApiKey(logCtx.ApiKeyID)
            return toolsResponse(tools)
        }
        return h.smartToolsResponse(req.ID)
    }
    // /mcp/group/:slug 按 group.ExposeMode 决定
}
```

> **工具 ID 格式约定**:
> - **Direct 模式**: `serviceName__toolName`（双下划线），出现在 `tools/list` 和 `tools/call` 中
> - **Smart 模式**: `serviceName.toolName`（点号），出现在 `mcp.execute` 的 `tool_id` 参数和搜索结果中
> - 原因: Direct 模式工具名需作为 MCP tool name，双下划线避免与 JSON path 冲突；Smart 模式通过元工具间接调用，点号更易读

### 4.5 工具路由器

```go
// internal/mcp/bridge/tool_router.go

// ToolRouter 根据 namespaced tool name 路由到正确的上游服务
type ToolRouter struct {
    pool *SessionPool
}

// Route 解析工具调用，自动识别两种命名空间格式:
//   Direct: "serviceName__toolName" (双下划线)
//   Smart:  "serviceName.toolName"  (点号)
// 返回目标 session 和原始 tool name
func (r *ToolRouter) Route(namespacedTool string) (*McpSession, string, error)
```

### 4.6 VirtualToolRegistry — 虚拟工具注册表

```go
// internal/mcp/virtual/registry.go

// VirtualToolRegistry 管理虚拟 MCP 工具的内存注册表
// 虚拟工具不走 SessionPool/TransportAdapter，直接在进程内处理
type VirtualToolRegistry struct {
    mu       sync.RWMutex
    handlers map[int64]VirtualToolHandler  // key: McpService.ID
}

// VirtualToolHandler 虚拟工具处理函数签名
type VirtualToolHandler func(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error)

// Register 注册虚拟工具处理器
func (r *VirtualToolRegistry) Register(serviceID int64, handler VirtualToolHandler)

// Unregister 注销虚拟工具处理器
func (r *VirtualToolRegistry) Unregister(serviceID int64)

// Handle 调用虚拟工具
func (r *VirtualToolRegistry) Handle(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error)

// IsVirtual 检查服务是否为虚拟服务
func (r *VirtualToolRegistry) IsVirtual(serviceID int64) bool
```

> **设计思路**: VirtualToolRegistry 绕过现有的 SessionPool/TransportAdapter 架构，为进程内虚拟工具提供直接调用路径。每个虚拟 McpService（Vision/Camera 启用后创建）注册一个 handler，Gateway 在 `tools/call` 时优先检查虚拟服务。

### 4.7 VisionClient — 视觉 API 客户端

```go
// internal/mcp/vision/client.go

// VisionClient 通用 OpenAI 兼容视觉 API 客户端
// 支持 OpenAI、GLM、Qwen、Ollama 等兼容端点
type VisionClient struct {
    EndpointURL string
    ApiKey      string
    ModelName   string
    MaxTokens   int
}

// Analyze 调用视觉模型分析图片
// 使用 OpenAI Chat Completions API 格式，image_url content part 传递 base64 图片
func (c *VisionClient) Analyze(ctx context.Context, systemPrompt, userPrompt, base64Image string) (string, error)
```

> **请求格式**: 标准 OpenAI Chat Completions API，content 包含 text 和 image_url 两个 part。`image_url` 使用 `data:image/jpeg;base64,{data}` 格式。

### 4.8 CameraStreamManager — 摄像头帧流管理

```go
// internal/mcp/camera/stream_manager.go

// CameraStreamManager 管理摄像头实时帧的内存缓存
// 纯内存结构，不持久化到数据库
type CameraStreamManager struct {
    mu      sync.RWMutex
    streams map[int64]*CameraStream  // key: camera ID
}

type CameraStream struct {
    LatestFrame []byte         // 最新帧 JPEG
    CapturedAt  time.Time      // 捕获时间
    Conn        *websocket.Conn // WebSocket 连接
}

// HandleFrame 缓存最新帧（由 WebSocket 端点调用）
func (m *CameraStreamManager) HandleFrame(cameraID int64, frame []byte)

// GetLatestFrame 获取缓存的最新帧
func (m *CameraStreamManager) GetLatestFrame(cameraID int64) ([]byte, time.Time, error)

// IsStreaming 检查是否有活跃的 WebSocket 推流连接
func (m *CameraStreamManager) IsStreaming(cameraID int64) bool

// Cleanup 关闭连接并清理缓存
func (m *CameraStreamManager) Cleanup(cameraID int64)
```

> **帧流程**: 浏览器通过 WebRTC `getUserMedia` 获取摄像头 → canvas 截取 JPEG → WebSocket 发送到 `/api/v1/cameras/:id/stream` → 后端缓存最新帧 → MCP 工具调用时返回缓存帧。

### 4.9 MCP Gateway Handler

```go
// internal/mcp/handler/gateway_handler.go

// GatewayHandler 处理 MCP 协议请求
// 同时作为 MCP Server 暴露给下游客户端
type GatewayHandler struct {
    pool            *SessionPool
    toolRouter      *ToolRouter
    searchEngine    *SearchEngine
    virtualRegistry *VirtualToolRegistry
}

// 处理 tools/list: 根据端点模式聚合工具
func (h *GatewayHandler) handleToolsList(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse

// 处理 tools/call: 优先检查虚拟服务，再路由到上游服务
func (h *GatewayHandler) handleToolsCall(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse
```

> **路由优先级**: `handleToolsCall` 先检查 Smart 模式元工具 (mcp.search/describe/execute)，再检查虚拟服务，最后走 SessionPool 路由到上游 MCP 服务。所有服务名查找均带 `user_id` 约束确保用户隔离。

### 4.10 ApiKeyResolver — 共享工具函数

```go
// internal/mcp/bridge/resolver.go

// ResolveApiKeyInfo 一次性获取 APIKey + User 信息
func ResolveApiKeyInfo(apiKeyID int64) (*ApiKeyInfo, error)

// GetGroupsForApiKey 返回 APIKey 可访问的分组列表（支持 groups: ["*"] 通配）
func GetGroupsForApiKey(info *ApiKeyInfo) ([]model.McpGroup, error)

// CollectToolsForGroups 聚合多个分组的工具（含去重、过滤、命名空间前缀）
func CollectToolsForGroups(groups []model.McpGroup, dedup bool) ([]ToolEntry, error)

// HasGroupAccess 检查 APIKey 是否有权限访问指定分组
func HasGroupAccess(info *ApiKeyInfo, groupName string) bool
```

> **设计目的**: 统一 APIKey 权限解析、分组查询、工具聚合逻辑，消除 gateway_handler、search_engine、xiaozhi_client 中的重复代码。

---

## 5. 分层架构

```
┌──────────────────────────────────────────────┐
│ HTTP Layer (Gin)                             │
│  router.go → controller → service → model    │
└──────────────────────────────────────────────┘
        │                    │
        ▼                    ▼
┌──────────────────┐ ┌──────────────────────────┐
│ REST API         │ │ MCP Protocol             │
│ /api/v1/*        │ │ /mcp/*                   │
│                  │ │ /mcp/ws/*                │
│ 管理操作         │ │ 协议级操作                │
└────────┬─────────┘ └────────┬─────────────────┘
         │                     │
         ▼                     ▼
┌──────────────────────────────────────────────┐
│ Service Layer (业务逻辑)                      │
│  Registry │ Group │ Connection │ Vision │ Camera │ Bridge │
└──────────────────────┬───────────────────────┘
                       │
         ┌─────────────┼─────────────┐
         ▼             ▼             ▼
┌──────────────┐ ┌──────────┐ ┌──────────────┐
│ MCP Protocol │ │ DAL      │ │ External     │
│ Transport    │ │ (GORM)   │ │ Clients      │
│ Bridge       │ │          │ │ (Redis, etc) │
└──────────────┘ └──────────┘ └──────────────┘
```

---

## 6. 关键设计决策

| 决策 | 选择 | 原因 |
|------|------|------|
| 架构风格 | 模块化单体 | 个人使用优先，单二进制部署最简单 |
| 存储默认 | SQLite | 零配置，适合个人使用 |
| HTTP 框架 | Gin | 性能好，生态成熟，NewAPI 验证过 |
| MCP SDK | go-sdk/mcp | 官方 Go SDK，Streamable HTTP 原生支持 |
| WebSocket | gorilla/websocket | 成熟稳定，广泛使用 |
| 前端框架 | React + Semi Design | Semi Design 功能丰富，中文生态好 |
| 前端构建 | Vite | 开发体验好，构建快 |
| 状态管理 | Zustand | 比 Redux 轻量，适合中小项目 |
| 工具暴露 | 双模式 (Direct + Smart) | Direct 适合工具少场景，Smart 适合大量工具/受限设备 |
| 搜索引擎 | 自实现 BM25Okapi | 零依赖，搜索范围通过 API Key→分组 自然收敛到百级 |
| 虚拟工具 | VirtualToolRegistry (进程内) | 绕过 SessionPool，支持 Vision/Camera 等非 MCP 协议工具 |
| 视觉 API | OpenAI 兼容格式 | 一套客户端适配 OpenAI/GLM/Qwen/Ollama 等 |
| 摄像头帧 | WebSocket 实时推流 + 内存缓存 | 浏览器端推流，后端仅缓存最新帧，被动响应 MCP 调用 |

---

## 7. 扩展点设计

### V2 扩展: IoT / 智能家居集成

#### 7.1 ProtocolBridge 抽象层

IoT 协议（MQTT、Zigbee、Z-Wave）与 MCP 协议的语义不同（发布/订阅 vs 请求/响应），需要增加 `ProtocolBridge` 接口层:

```
IoT 设备 <--协议桥接--> ProtocolBridge <--MCP 工具映射--> IoTAdapter(TransportAdapter) --> SessionPool
```

```go
// internal/mcp/transport/protocol_bridge.go

// ProtocolBridge 定义 IoT/智能家居协议桥接接口
type ProtocolBridge interface {
    Connect(ctx context.Context) error
    Close() error
    DiscoverDevices(ctx context.Context) ([]IoTDevice, error)
    ExecuteCommand(ctx context.Context, deviceID, command string, params map[string]any) (any, error)
    ReadState(ctx context.Context, deviceID string) (map[string]any, error)
    SubscribeStateChanges(callback DeviceStateCallback) error
}

type IoTDevice struct {
    ID           string
    Name         string
    Type         string                 // sensor, actuator, light, thermostat, camera
    Capabilities []DeviceCapability
    State        map[string]any
}

type DeviceCapability struct {
    Name       string
    Type       string          // "action" 或 "sensor"
    Parameters json.RawMessage // JSON Schema (action 参数)
    ReturnType json.RawMessage // JSON Schema (sensor 返回值)
}

type DeviceStateCallback func(deviceID string, state map[string]any)
```

#### 7.2 IoTAdapter — 将 IoT 设备映射为 MCP 工具

```go
// internal/mcp/transport/iot_adapter.go

// IoTAdapter 实现 TransportAdapter 接口，桥接 IoT 设备到 MCP
type IoTAdapter struct {
    bridge      ProtocolBridge
    tools       []Tool                    // 从 DeviceCapabilities 动态生成
    deviceState map[string]map[string]any // 设备状态缓存
    mu          sync.RWMutex
}

// GetTools 将 IoTDevice.Capabilities 转为 MCP Tool 定义
// 例如: 温度传感器 → tool "sensor.read_temperature"
//       智能灯泡   → tool "light.set_brightness", "light.set_color"
func (a *IoTAdapter) GetTools() []Tool

// Call 路由到 ExecuteCommand (action) 或 ReadState (sensor)
func (a *IoTAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
```

#### 7.3 桥接优先级

| 桥接目标 | 优先级 | 覆盖范围 | 说明 |
|---------|--------|---------|------|
| Home Assistant REST/WS API | P0 | 2000+ 设备集成 | 最广泛的智能家居平台 |
| MQTT (通用) | P0 | Zigbee2MQTT, ESPHome, Tasmota | 通用 IoT 协议 |
| Zigbee2MQTT (via MQTT) | P1 | 直接 Zigbee 设备 | 通过 MQTT 桥接 |
| Philips Hue Bridge | P2 | 流行照明系统 | REST API |
| Apple HomeKit | P2 | Apple 生态 | 需通过 Home Assistant 桥接 |

> **关键策略**: 不在 Go 中实现底层无线协议栈（Zigbee、Z-Wave、Thread/Matter），而是桥接到现有的 Hub/网关（Home Assistant、Zigbee2MQTT 等），这些平台已处理了设备配对、OTA、mesh 网络等复杂逻辑。

#### 7.4 数据库扩展

```sql
-- IoT 网关配置 (V2)
CREATE TABLE iot_configs (
    id                BIGINT UNSIGNED AUTO_INCREMENT,
    user_id           BIGINT UNSIGNED NOT NULL,
    service_id        BIGINT UNSIGNED NOT NULL COMMENT '关联的 MCP 服务',
    bridge_type       VARCHAR(32) NOT NULL COMMENT 'mqtt, homeassistant, zigbee2mqtt',
    bridge_config     TEXT DEFAULT '{}' COMMENT 'Bridge 连接配置 JSON',
    auto_discover     TINYINT DEFAULT 1,
    state_ttl         INT DEFAULT 300 COMMENT '设备状态缓存 TTL (秒)',
    PRIMARY KEY (id)
);

-- IoT 设备状态 (V2)
CREATE TABLE iot_device_state (
    id                BIGINT UNSIGNED AUTO_INCREMENT,
    device_id         VARCHAR(128) NOT NULL COMMENT '设备唯一标识',
    service_id        BIGINT UNSIGNED NOT NULL,
    state             MEDIUMTEXT DEFAULT '{}' COMMENT '设备状态 JSON',
    last_updated      DATETIME,
    PRIMARY KEY (id),
    UNIQUE KEY (device_id)
);
```

#### 7.5 实时事件推送

IoT 设备状态变化需推送到 LLM 客户端，利用已有的 WebSocket/SSE 传输层:
- GatewayHandler 增加 notification dispatcher
- 客户端可订阅特定设备组的事件
- 状态变化通过 MCP notifications/tools_changed 通知客户端

### V3 扩展: 商业化
- 新增 `BillingService` 处理用量计量
- 新增 `plans` / `subscriptions` / `invoices` 表
- 中间件层新增配额检查
- 支付网关抽象接口（微信/支付宝/Stripe）
