# NewMCP 技术架构文档

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

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
| 前端 | React 18 + Vite + Semi Design | 管理界面 |
| 状态管理 | Zustand | 轻量级前端状态 |
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
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │   │
│  │  │  REST API    │  │  MCP Gateway │  │  WebSocket   │  │   │
│  │  │  /api/v1/*   │  │  /mcp/*      │  │  /mcp/ws/*   │  │   │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  │   │
│  └─────────┼─────────────────┼─────────────────┼──────────┘   │
│            │                 │                 │               │
│  ┌─────────┴─────────────────┴─────────────────┴──────────┐   │
│  │                    Middleware Layer                      │   │
│  │     Auth │ CORS │ RateLimit │ Logger │ GroupResolver    │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │                    Controller Layer                      │   │
│  │  Auth │ Service │ Group │ Device │ Vision │ Camera      │   │
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
│  │                                                          │   │
│  │  ┌────────────┐  ┌────────────┐                         │   │
│  │  │ Device     │  │ Vision     │                         │   │
│  │  │ - 设备管理 │  │ - 模型配置 │                         │   │
│  │  │ - 小智桥接 │  │ - 摄像头   │                         │   │
│  │  │ - 状态监控 │  │ - 帧处理   │                         │   │
│  │  └────────────┘  └────────────┘                         │   │
│  └──────────────────────┬──────────────────────────────────┘   │
│                         │                                      │
│  ┌──────────────────────┴──────────────────────────────────┐   │
│  │              MCP Protocol Layer                          │   │
│  │                                                          │   │
│  │  ┌──────────────────────────────────────────────────┐  │   │
│  │  │  Transport Adapters                               │  │   │
│  │  │  StdioAdapter │ SSEAdapter │ HTTPAdapter │ WSAdapter │  │
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

### 3.2 小智设备通过 NewMCP 消费工具（核心模式）

> **关键**: NewMCP **主动连接**小智云 WSS 端点，作为 MCP Server 向小智注册工具。小智设备通过小智云调用工具时，小智云转发到 NewMCP。

```
时序图: 小智设备通过 NewMCP 调用工具

┌────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│小智    │    │小智云    │    │NewMCP    │    │上游 MCP  │
│设备    │    │api.xiaozhi│   │XiaoZhi   │    │服务      │
│(ESP32) │    │.me       │    │Client    │    │(calculator│
└───┬────┘    └────┬─────┘    └────┬─────┘    └────┬─────┘
    │              │               │               │
    │              │  ① NewMCP 主动连接 WSS 端点   │
    │              │<──────────────│               │
    │              │               │               │
    │              │  ② 小智云作为 MCP Client       │
    │              │  发送 initialize               │
    │              │──────────────>│               │
    │              │               │               │
    │              │  ③ NewMCP 响应 capabilities    │
    │              │  (声明提供 tools 能力)          │
    │              │<──────────────│               │
    │              │               │               │
    │              │  ④ 小智云请求 tools/list       │
    │              │──────────────>│               │
    │              │               │               │
    │              │  ⑤ 返回聚合工具列表             │
    │              │  (calculator, web_search, ...) │
    │              │<──────────────│               │
    │              │               │               │
    │  ⑥ 设备语音指令               │               │
    │─────────────>│               │               │
    │              │               │               │
    │              │  ⑦ 小智云转发 tools/call       │
    │              │  {name:"calculator", args:...} │
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

### 3.3 LLM 客户端通过 NewMCP 消费小智工具（辅助模式）

> 可选模式: 将小智 WSS 注册为上游 MCP 服务，LLM 客户端也能调用小智的工具。

```
时序图: Claude Code 通过 NewMCP 调用小智工具

┌──────────┐    ┌──────────┐    ┌──────────┐
│LLM Client│    │NewMCP    │    │小智云 MCP│
│(Claude)  │    │Gateway   │    │(wss://)  │
└────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │
     │               │ NewMCP 作为 MCP Client 连接小智云
     │               │──────────────>│
     │               │               │
     │               │ 获取小智工具列表
     │               │<──────────────│
     │               │               │
     │               │ 注册为虚拟 MCP 服务
     │               │───┐           │
     │               │<──┘           │
     │               │               │
     │ POST /mcp (tools/list)        │
     │──────────────>│               │
     │               │               │
     │ 返回包含小智工具的聚合列表     │
     │<──────────────│               │
     │               │               │
     │ POST /mcp (tools/call)        │
     │ tool: xiaozhi__xxx            │
     │──────────────>│               │
     │               │               │
     │               │ 转发到小智云   │
     │               │──────────────>│
     │               │               │
     │               │<──────────────│
     │<──────────────│               │
```

---

## 4. 模块详细设计

### 4.1 Transport Adapter 接口

```go
// internal/mcp/transport/transport.go

// TransportAdapter 定义 MCP 传输协议适配器接口
type TransportAdapter interface {
    // Connect 建立到上游 MCP 服务器的连接
    Connect(ctx context.Context) error
    // Close 关闭连接
    Close() error
    // Send 发送 JSON-RPC 消息
    Send(ctx context.Context, message jsonrpc.Message) (jsonrpc.Message, error)
    // IsConnected 检查连接状态
    IsConnected() bool
    // GetTools 获取工具列表（缓存）
    GetTools(ctx context.Context) ([]Tool, error)
}

// TransportType 传输类型枚举
type TransportType string
const (
    TransportStdio         TransportType = "stdio"
    TransportSSE           TransportType = "sse"
    TransportStreamableHTTP TransportType = "streamable-http"
    TransportWebSocket     TransportType = "websocket"
)
```

### 4.2 Session Pool 设计

```go
// internal/mcp/bridge/session_pool.go

// SessionPool 管理到上游 MCP 服务器的连接池
type SessionPool struct {
    mu       sync.RWMutex
    sessions map[int64]*McpSession  // key: service_id
}

// McpSession 代表一个到上游 MCP 服务器的活跃连接
type McpSession struct {
    ServiceID   int64
    Adapter     TransportAdapter
    Tools       []Tool
    LastRefresh time.Time
    Health      HealthStatus
}
```

### 4.3 工具路由器

```go
// internal/mcp/bridge/tool_router.go

// ToolRouter 根据 namespaced tool name 路由到正确的上游服务
type ToolRouter struct {
    pool *SessionPool
}

// Route 解析 "serviceName__toolName" 格式的工具调用
// 返回目标 session 和原始 tool name
func (r *ToolRouter) Route(namespacedTool string) (*McpSession, string, error)
```

### 4.4 MCP Gateway Handler

```go
// internal/mcp/handler/gateway_handler.go

// GatewayHandler 处理 MCP 协议请求
// 同时作为 MCP Server 暴露给下游客户端
type GatewayHandler struct {
    toolRouter *ToolRouter
    pool       *SessionPool
}

// 处理 tools/list: 聚合所有活跃服务的工具
func (h *GatewayHandler) HandleToolsList(ctx context.Context, req *Request) (*Response, error)

// 处理 tools/call: 路由到目标上游服务
func (h *GatewayHandler) HandleToolsCall(ctx context.Context, req *Request) (*Response, error)
```

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
│  Registry │ Group │ Device │ Vision │ Bridge │
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

---

## 7. 扩展点设计

### V2 扩展: IoT 集成
- 新增 `IoTService` 实现 `TransportAdapter` 接口
- 新增 `iot_configs` 表存储 IoT 设备参数
- MQTT 客户端作为新的 transport adapter

### V3 扩展: 商业化
- 新增 `BillingService` 处理用量计量
- 新增 `plans` / `subscriptions` / `invoices` 表
- 中间件层新增配额检查
- 支付网关抽象接口（微信/支付宝/Stripe）
