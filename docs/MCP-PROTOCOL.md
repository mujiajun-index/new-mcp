# NewMCP 协议适配说明

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

## 1. 双模式网关架构

NewMCP 支持两种 MCP 工具暴露模式，**通过端点路由驱动**：

| 端点 | 模式 | 说明 |
|------|------|------|
| `POST /mcp` | 固定 Direct | 聚合 API Key 所有分组，去重后暴露全部工具（`serviceName__toolName`） |
| `POST /smart/mcp` | 固定 Smart | 聚合 API Key 所有分组，仅暴露 3 个元工具，渐进发现 |
| `POST /mcp/group/{slug}` | 由分组的 `expose_mode` 决定 | 端点驱动，每个分组独立配置 |
| `WS /mcp/ws` | 固定 Direct | 同 POST /mcp |
| `WS /smart/mcp/ws` | 固定 Smart | 同 POST /smart/mcp |
| `WS /mcp/ws/group/{slug}` | 由分组的 `expose_mode` 决定 | 端点驱动 |

> **Direct 主端点**: `/mcp` 暴露 API Key 绑定分组的全部工具（去重），适合 Claude Code、Cursor 等支持大量工具的 LLM 客户端。
> **Smart 主端点**: `/smart/mcp` 仅暴露 3 个元工具，适合小智等上下文受限设备或工具量特别大的场景。

### 1.1 Direct 模式（直接模式）

将分组内所有上游 MCP 服务的工具聚合后直接暴露，添加命名空间前缀。

- 适合: 工具数量少（<20）的场景
- LLM 直接看到所有工具 schema
- 一步调用，延迟低

### 1.2 Smart 模式（智能模式）

只暴露 3~5 个元工具（Meta Tools），LLM 通过搜索→查看→执行渐进发现和调用工具。参考 [eznix86/mcp-gateway](https://github.com/eznix86/mcp-gateway)。

- 适合: 工具数量多（20+）的场景、小智等受限设备
- Token 消耗极低，永远只暴露几个元工具
- 无 MCP 规模上限

### 1.3 模式选择

| 场景 | 推荐模式 | 原因 |
|------|----------|------|
| Claude Code + 少量工具 | direct | 工具少，直接调用更快 |
| Cursor + 大量工具 | smart | 避免 context 爆炸 |
| 小智设备 | smart（可配置） | 设备上下文有限 |
| 机器人控制 | smart | 需要动态发现可用控制 MCP |

分组配置中的 `expose_mode` 字段控制模式（仅 `/mcp/group/{slug}` 端点生效）：
```json
{
    "expose_mode": "smart"  // "direct" 或 "smart"
}
```

---

## 2. 搜索范围收敛机制

NewMCP 的 MCP 工具搜索通过 **API Key → 分组 → MCP 服务** 的关联链路自然收敛搜索范围，从平台级万级规模降到百级：

```
平台 MCP 市场 (10,000+ 服务)
    │
    │  用户从市场选择服务加入分组
    ▼
┌─────────────────────────────────────────────────┐
│  用户分组:                                        │
│  分组A "机器人控制": [sea-bot, air-drone, arm]    │
│  分组B "数据分析": [exa-search, calculator, db]   │
│                                                  │
│  API Key-1 → 绑定 [分组A]                        │
│    mcp.search 搜索范围: 3 服务, ~15 工具          │
│                                                  │
│  API Key-2 → 绑定 [分组A, 分组B]                 │
│    mcp.search 搜索范围: 6 服务, ~30 工具          │
└─────────────────────────────────────────────────┘
```

**搜索范围**: API Key 认证 → 查询 `permissions.groups` → 收集分组内所有服务+工具 → 在此范围内搜索

**两种搜索场景**:

| 场景 | 触发时机 | 搜索范围 | 规模 | 方案 |
|------|----------|----------|------|------|
| mcp.search | Smart 模式元工具调用 | API Key 绑定分组内 | 5-200 工具 | 自实现 BM25（零依赖） |
| 市场浏览 | 前端 UI `/marketplace` | 全平台公开服务 | 10K+ 服务 | 数据库 LIKE/FTS 查询 |

---

## 3. Smart 模式元工具

### 3.1 工具列表

| 工具名 | 说明 | 对应 mcp-gateway |
|--------|------|------------------|
| `mcp.search` | 搜索可用的 MCP 服务和工具 | `gateway.search` |
| `mcp.describe` | 查看指定工具的完整 Schema | `gateway.describe` |
| `mcp.execute` | 执行指定工具 | `gateway.invoke` |
| `mcp.execute_async` | 异步执行工具（可选） | `gateway.invoke_async` |
| `mcp.job_status` | 查询异步任务状态（可选） | `gateway.invoke_status` |

V1 核心实现前 3 个，后 2 个作为 V1.1 扩展。

### 3.2 mcp.search - 搜索可用 MCP / 工具

**功能**: 在 API Key 绑定的分组范围内，使用 BM25 算法搜索 MCP 服务和工具。

**参数:**
```json
{
    "query": "搜索",              // 必填，搜索关键字
    "scope": "mcp",              // 可选: "mcp"=搜服务, "tool"=搜工具, "all"=都搜 (默认 "mcp")
    "group": "机器人控制",         // 可选，限定分组
    "limit": 10                  // 可选，最大 50，默认 10
}
```

**返回 (scope="mcp"):**
```json
{
    "content": [{
        "type": "text",
        "text": "找到 3 个匹配的 MCP 服务:\n\n" +
            "1. **exa-search** (分组: 联网工具)\n" +
            "   Exa 网络搜索引擎，支持关键词和语义搜索\n" +
            "   工具数: 3\n\n" +
            "2. **sea-bot** (分组: 机器人控制)\n" +
            "   水下机器人控制 MCP\n" +
            "   工具数: 5\n\n" +
            "3. **web-fetch** (分组: 联网工具)\n" +
            "   网页内容抓取\n" +
            "   工具数: 2"
    }]
}
```

**返回 (scope="tool"):**
```json
{
    "content": [{
        "type": "text",
        "text": "找到 5 个匹配的工具:\n\n" +
            "1. **exa-search.web_search** (exa-search)\n" +
            "   网页搜索，返回相关结果\n\n" +
            "2. **exa-search.get_contents** (exa-search)\n" +
            "   获取网页内容\n\n" +
            "..."
    }]
}
```

### 3.3 mcp.describe - 查看工具详细 Schema

**功能**: 获取指定 MCP 服务的工具列表，或指定工具的完整参数 Schema。

**参数:**
```json
{
    "targets": ["exa-search"],           // MCP 服务名列表，或 "serviceName.toolName" 形式
    "include_schema": true               // 可选，是否包含 inputSchema (默认 true)
}
```

**返回:**
```json
{
    "content": [{
        "type": "text",
        "text": "## exa-search 的工具列表 (3个)\n\n" +
            "### web_search\n" +
            "搜索网页内容\n" +
            "参数:\n" +
            "- query (string, 必填): 搜索关键词\n" +
            "- numResults (number, 可选): 返回结果数量，默认 10\n\n" +
            "### get_contents\n" +
            "获取指定 URL 的网页内容\n" +
            "参数:\n" +
            "- urls (array, 必填): 要获取的 URL 列表\n\n" +
            "### find_similar\n" +
            "查找相似网页\n" +
            "参数:\n" +
            "- url (string, 必填): 参考网页 URL"
    }]
}
```

**批量查询示例:**
```json
{
    "targets": ["exa-search", "calculator"]
}
```

返回两个服务的所有工具信息。

### 3.4 mcp.execute - 执行指定工具

**功能**: 根据工具 ID 和参数执行指定 MCP 工具。

**参数:**
```json
{
    "tool_id": "exa-search.web_search",    // 格式: "服务名.工具名"
    "arguments": {                          // 工具参数
        "query": "今天新闻"
    },
    "timeout_ms": 30000                     // 可选，超时毫秒，默认 30000
}
```

**返回:**
```json
{
    "content": [{
        "type": "text",
        "text": "[搜索结果...]"
    }]
}
```

**实现逻辑:**
```go
// internal/mcp/smart/executor.go

func (e *Executor) Execute(ctx context.Context, toolID string, arguments json.RawMessage, timeoutMs int) (interface{}, error) {
    // 1. 解析 toolID: "exa-search.web_search" → service="exa-search", tool="web_search"
    parts := strings.SplitN(toolID, ".", 2)
    if len(parts) != 2 {
        return nil, fmt.Errorf("invalid tool_id format, expected 'service.tool'")
    }
    serviceName, toolName := parts[0], parts[1]

    // 2. 路由到对应的上游 MCP 服务
    session := e.sessionPool.Get(serviceName)
    if session == nil {
        return nil, fmt.Errorf("MCP service '%s' not found or not connected", serviceName)
    }

    // 3. 设置超时
    if timeoutMs <= 0 {
        timeoutMs = 30000
    }
    ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
    defer cancel()

    // 4. 调用上游 MCP
    result, err := session.Adapter.Call(ctx, "tools/call", map[string]interface{}{
        "name":      toolName,
        "arguments": arguments,
    })

    return result, err
}
```

---

## 4. Direct 模式实现

### 4.1 工具聚合 (tools/list)

```go
func (g *GatewayHandler) HandleToolsList(ctx context.Context, groupID int64) ([]Tool, error) {
    services := groupService.GetEnabledServices(groupID)

    var allTools []Tool
    for _, svc := range services {
        tools := toolCache.Get(svc.ID)
        for _, tool := range tools {
            if groupToolFilter.IsEnabled(groupID, svc.ID, tool.Name) {
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

### 4.2 工具路由 (tools/call)

```go
func (g *GatewayHandler) HandleToolsCall(ctx context.Context, groupID int64, namespacedName string, arguments json.RawMessage) (json.RawMessage, error) {
    serviceName, toolName := parseNamespacedName(namespacedName)  // "__" 分隔
    session := sessionPool.Get(groupID, serviceName)
    result, err := session.Adapter.Call(ctx, "tools/call", map[string]interface{}{
        "name":      toolName,
        "arguments": arguments,
    })
    return result, err
}
```

---

## 5. 模式分发

Gateway Handler 根据分组的 `expose_mode` 配置分发请求：

```go
// internal/mcp/handler/gateway_handler.go

func (h *GatewayHandler) HandleToolsList(ctx context.Context, groupID int64) ([]Tool, error) {
    group, _ := h.groupService.GetByID(groupID)

    switch group.ExposeMode {
    case "direct":
        return h.handleDirectToolsList(ctx, groupID)
    case "smart":
        // Smart 模式只返回固定的元工具
        return h.getMetaTools(), nil
    default:
        return h.handleDirectToolsList(ctx, groupID)
    }
}

func (h *GatewayHandler) HandleToolsCall(ctx context.Context, groupID int64, toolName string, arguments json.RawMessage) (json.RawMessage, error) {
    group, _ := h.groupService.GetByID(groupID)

    switch group.ExposeMode {
    case "direct":
        return h.handleDirectToolsCall(ctx, groupID, toolName, arguments)
    case "smart":
        return h.handleSmartToolsCall(ctx, groupID, toolName, arguments)
    default:
        return h.handleDirectToolsCall(ctx, groupID, toolName, arguments)
    }
}

func (h *GatewayHandler) handleSmartToolsCall(ctx context.Context, groupID int64, toolName string, arguments json.RawMessage) (json.RawMessage, error) {
    switch toolName {
    case "mcp.search":
        return h.smartHandler.HandleSearch(ctx, groupID, arguments)
    case "mcp.describe":
        return h.smartHandler.HandleDescribe(ctx, groupID, arguments)
    case "mcp.execute":
        return h.smartHandler.HandleExecute(ctx, groupID, arguments)
    default:
        return nil, fmt.Errorf("unknown meta tool: %s", toolName)
    }
}

// getMetaTools 返回 Smart 模式的固定元工具列表
func (h *GatewayHandler) getMetaTools() []Tool {
    return []Tool{
        {
            Name:        "mcp.search",
            Description: "搜索可用的 MCP 服务和工具。支持按关键字、分组名、服务名搜索。",
            InputSchema: searchToolSchema,
        },
        {
            Name:        "mcp.describe",
            Description: "查看指定 MCP 服务的工具列表，或指定工具的完整参数 Schema。支持批量查询。",
            InputSchema: describeToolSchema,
        },
        {
            Name:        "mcp.execute",
            Description: "执行指定的 MCP 工具。参数: tool_id (格式: 服务名.工具名), arguments (工具参数)。",
            InputSchema: executeToolSchema,
        },
    }
}
```

---

## 6. Transport Adapter 实现

### 6.1 接口定义

```go
// internal/mcp/transport/transport.go

type TransportAdapter interface {
    Connect(ctx context.Context) error
    Close() error
    Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
    IsConnected() bool
    GetType() TransportType
}

type TransportType string
const (
    TypeStdio          TransportType = "stdio"           // 本地子进程
    TypeSSE            TransportType = "sse"             // 主动连接远程 SSE
    TypeStreamableHTTP TransportType = "streamable-http" // 主动连接远程 HTTP
    TypeWebSocket      TransportType = "websocket"       // 主动连接远程 WSS
    TypePassiveWS      TransportType = "passive-ws"      // 被动: 外部服务连入
)
```

### 6.2 连接方式

| 方式 | transport_type | 方向 | 说明 |
|------|---------------|------|------|
| Stdio | stdio | NewMCP → 本地子进程 | 本地命令行 MCP 服务 |
| SSE | sse | NewMCP → 远程 | 连接远程 SSE 端点 |
| Streamable HTTP | streamable-http | NewMCP → 远程 | 连接远程 HTTP 端点 |
| WebSocket | websocket | NewMCP → 远程 | 连接远程 WSS 端点 |
| 被动连接 | passive-ws | 外部 → NewMCP | NewMCP 生成接入 URL，外部服务主动连入 |

### 6.3 被动连接 (passive-ws) 实现

NewMCP 作为 MCP Client，接收外部 MCP Server 的连入:

```
时序图: 外部 MCP 服务通过被动连接注册到 NewMCP

┌──────────┐    ┌──────────┐    ┌──────────┐
│外部 MCP  │    │NewMCP    │    │LLM 客户端│
│Server    │    │Passive   │    │(Claude)  │
└────┬─────┘    └────┬─────┘    └────┬─────┘
     │               │               │
     │ ① 用户在 NewMCP 创建 passive-ws 服务
     │   获得 URL: wss://api.newmcp.pro/mcp/passive/?token=JWT
     │               │               │
     │ ② 外部服务连接 WSS 接入点      │
     │──────────────>│               │
     │               │               │
     │ ③ NewMCP 作为 MCP Client       │
     │   发送 initialize              │
     │<──────────────│               │
     │               │               │
     │ ④ 外部服务响应 capabilities     │
     │──────────────>│               │
     │               │               │
     │ ⑤ NewMCP 请求 tools/list      │
     │<──────────────│               │
     │               │               │
     │ ⑥ 返回工具列表，缓存到 mcp_services
     │──────────────>│               │
     │               │               │
     │               │ ⑦ LLM 客户端调用工具
     │               │──────────────>│ (via gateway)
     │               │               │
     │ ⑧ 路由 tools/call              │
     │<──────────────│               │
     │               │               │
     │ ⑨ 执行并返回   │               │
     │──────────────>│               │
     │               │──────────────>│
```

**被动接入 URL 生成:**

```go
// internal/mcp/transport/passive_ws.go

type PassiveWSListener struct {
    services    map[string]*PassiveSession  // key: service_name
    mu          sync.RWMutex
    jwtSecret   string
    baseURL     string
}

// GenerateConnectURL 为 passive-ws 类型的服务生成接入 URL
func (l *PassiveWSListener) GenerateConnectURL(serviceID int64, serviceName string, userID int64) string {
    token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
        "serviceId":  serviceID,
        "serviceName": serviceName,
        "userId":     userID,
        "purpose":    "mcp-endpoint",
        "iat":        time.Now().Unix(),
        "exp":        time.Now().Add(365 * 24 * time.Hour).Unix(),
    })
    tokenString, _ := token.SignedString(l.jwtSecret)
    return fmt.Sprintf("%s/mcp/passive/?token=%s", l.baseURL, tokenString)
}
```

**被动连接 WebSocket Handler:**

```go
// 外部服务连入 wss://api.newmcp.pro/mcp/passive/?token=JWT

func (l *PassiveWSListener) HandleConnection(wsConn *websocket.Conn, tokenClaims jwt.MapClaims) {
    serviceName := tokenClaims["serviceName"].(string)
    serviceID := int64(tokenClaims["serviceId"].(float64))

    session := &PassiveSession{
        ServiceID:   serviceID,
        ServiceName: serviceName,
        Conn:        wsConn,
    }

    l.mu.Lock()
    l.services[serviceName] = session
    l.mu.Unlock()

    // NewMCP 作为 MCP Client: 发送 initialize → 获取工具 → 缓存
    session.Initialize()
    tools := session.FetchTools()
    toolCache.Update(serviceID, tools)

    // 更新数据库状态
    db.Model(&McpService{}).Where("id = ?", serviceID).
        Updates(map[string]interface{}{
            "passive_connected": 1,
            "health_status":     "healthy",
        })

    // 进入消息循环 (等待 NewMCP 发起 tools/call)
    session.MessageLoop()
}
```

### 6.4 适配器实现

StreamableHTTPAdapter 和 WebSocketAdapter（主动连接远程）实现同前。

---

## 7. 会话池与工具目录缓存

```go
// internal/mcp/bridge/session_pool.go
type SessionPool struct {
    mu       sync.RWMutex
    sessions map[string]*McpSession  // key: "{serviceName}"
}

type McpSession struct {
    ServiceID   int64
    ServiceName string
    Adapter     transport.TransportAdapter
    Tools       []transport.Tool
    LastUsed    time.Time
}

// internal/service/registry/tool_cache.go
type ToolCache struct {
    cache map[int64][]Tool  // key: service_id
    mu    sync.RWMutex
    ttl   time.Duration     // 默认 5 分钟
}

// internal/service/registry/health_checker.go
type HealthChecker struct {
    interval time.Duration  // 默认 60s
    pool     *SessionPool
}
```

---

## 8. 搜索引擎实现 (Smart 模式核心)

### 8.1 搜索范围收敛

mcp.search 的搜索范围通过 API Key → 分组 → MCP 服务的关联链路自然收敛：

```
MCP 请求 (X-API-Key) → 认证中间件
    → 查询 api_keys.permissions.groups (绑定的分组列表)
    → 收集这些分组内所有服务+工具 (5-200 条)
    → BM25 搜索 → 排序返回
```

API Key 的 `permissions` 字段示例:
```json
{
    "groups": ["robot-control", "data-analysis"],
    "max_rate": 100
}
```

搜索范围通常只有几十到几百条，因此不需要外部搜索引擎库。

### 8.2 BM25Okapi 自实现 (参考 mcp-gateway MiniSearch)

参考 [eznix86/mcp-gateway](https://github.com/eznix86/mcp-gateway) 的 MiniSearch 实现，在 Go 中自实现轻量 BM25Okapi，零外部依赖。

**BM25Okapi 核心公式:**
```
score(D, Q) = Σ IDF(t) × (f(t,D) × (k1+1)) / (f(t,D) + k1 × (1 - b + b × |D|/avgLen))

IDF(t) = log((N - n(t) + 0.5) / (n(t) + 0.5) + 1)
k1 = 1.2,  b = 0.75  (标准参数)
```

**字段权重 (与 MiniSearch 一致):**

| 字段 | 权重 | 说明 |
|------|------|------|
| name | 3.0 | 服务名/工具名，最高权重 |
| server_name | 2.0 | 所属服务名（工具类型） |
| description | 1.0 | 描述文本 |

**Go 实现:**

```go
// internal/mcp/smart/search_engine.go

type SearchEngine struct {
    docs     []SearchDoc          // 当前范围的文档集合
    index    *bm25Index           // 内存 BM25 索引
    dirty    bool                 // 是否需要重建
    mu       sync.RWMutex
}

type SearchDoc struct {
    ID          string  // "svc:exa-search" 或 "tool:exa-search.web_search"
    Type        string  // "mcp" 或 "tool"
    Name        string  // 服务名或工具名
    Description string
    GroupName   string
    ServerName  string  // 所属 MCP 服务名 (工具类型)
    ToolCount   int     // 工具数 (服务类型)
}

type bm25Index struct {
    docs       []SearchDoc
    termFreqs  map[string]map[int]int       // term → {docIdx: freq}
    docLens    []int                          // 每个文档的词数
    avgDocLen  float64
    docCount   int
    fieldBoost map[string]float64            // 字段权重
}

// Search 在 API Key 绑定的分组范围内搜索
func (e *SearchEngine) Search(ctx context.Context, store Store, apiKeyID int64, query string, opts SearchOptions) ([]SearchResult, error) {
    e.mu.RLock()
    defer e.mu.RUnlock()

    // 1. 根据 API Key 获取绑定的分组
    groups, err := store.GetGroupsByAPIKey(apiKeyID)
    if err != nil {
        return nil, err
    }

    // 2. 收集分组内所有服务+工具（从缓存中获取，已在内存中）
    var docs []SearchDoc
    for _, g := range groups {
        if opts.Group != "" && g.Name != opts.Group {
            continue
        }
        services := store.GetGroupServices(g.ID)
        for _, svc := range services {
            if opts.Scope != "tool" {
                docs = append(docs, SearchDoc{
                    ID:          "svc:" + svc.Name,
                    Type:        "mcp",
                    Name:        svc.Name,
                    Description: svc.Description,
                    GroupName:   g.Name,
                    ToolCount:   len(svc.Tools),
                })
            }
            if opts.Scope != "mcp" {
                for _, tool := range svc.Tools {
                    docs = append(docs, SearchDoc{
                        ID:          "tool:" + svc.Name + "." + tool.Name,
                        Type:        "tool",
                        Name:        tool.Name,
                        Description: tool.Description,
                        ServerName:  svc.Name,
                        GroupName:   g.Name,
                    })
                }
            }
        }
    }

    // 3. BM25 搜索
    idx := buildIndex(docs)
    results := idx.search(query, opts.Limit)
    return results, nil
}
```

**BM25 索引与评分:**

```go
// internal/mcp/smart/bm25.go

const (
    k1       = 1.2
    b        = 0.75
    fuzzDist = 2  // Levenshtein 最大编辑距离 (模糊匹配)
)

func buildIndex(docs []SearchDoc) *bm25Index {
    idx := &bm25Index{
        docs:       docs,
        termFreqs:  make(map[string]map[int]int),
        fieldBoost: map[string]float64{"name": 3.0, "server_name": 2.0, "description": 1.0},
    }
    idx.docLens = make([]int, len(docs))
    totalLen := 0

    for i, doc := range docs {
        // 按字段分词，加权合并到文档词频中
        fields := map[string]string{
            "name":        doc.Name,
            "server_name": doc.ServerName,
            "description": doc.Description,
        }
        docTerms := 0
        for field, text := range fields {
            tokens := tokenize(text)
            boost := idx.fieldBoost[field]
            for _, tok := range tokens {
                if idx.termFreqs[tok] == nil {
                    idx.termFreqs[tok] = make(map[int]int)
                }
                // 字段权重体现为词频倍增
                idx.termFreqs[tok][i] += int(boost * 10) // 乘以 10 避免浮点精度问题
            }
            docTerms += len(tokens)
        }
        idx.docLens[i] = docTerms
        totalLen += docTerms
    }

    idx.docCount = len(docs)
    if idx.docCount > 0 {
        idx.avgDocLen = float64(totalLen) / float64(idx.docCount)
    }
    return idx
}

func (idx *bm25Index) search(query string, limit int) []SearchResult {
    terms := tokenize(query)
    if len(terms) == 0 {
        return nil
    }

    // 计算每个文档的 BM25 分数
    scores := make(map[int]float64)
    for _, term := range terms {
        // 模糊匹配: 查找编辑距离内的相似词
        matchingTerms := idx.fuzzyExpand(term)

        for _, mt := range matchingTerms {
            postings, ok := idx.termFreqs[mt]
            if !ok {
                continue
            }

            // IDF(t) = log((N - n(t) + 0.5) / (n(t) + 0.5) + 1)
            n := float64(len(postings))
            idf := math.Log((float64(idx.docCount)-n+0.5)/(n+0.5) + 1)

            for docIdx, freq := range postings {
                f := float64(freq)
                docLen := float64(idx.docLens[docIdx])
                // BM25 评分
                tf := (f * (k1 + 1)) / (f + k1*(1-b+b*docLen/idx.avgDocLen))
                scores[docIdx] += idf * tf
            }
        }
    }

    // 排序
    var results []SearchResult
    for docIdx, score := range scores {
        if score > 0 {
            results = append(results, SearchResult{
                Doc:   idx.docs[docIdx],
                Score: score,
            })
        }
    }
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })
    if len(results) > limit {
        results = results[:limit]
    }
    return results
}

// fuzzyExpand 模糊扩展: 查找索引中与 term 编辑距离 <= fuzzDist 的词
func (idx *bm25Index) fuzzyExpand(term string) []string {
    var matched []string
    for idxTerm := range idx.termFreqs {
        if levenshtein(term, idxTerm) <= fuzzDist {
            matched = append(matched, idxTerm)
        }
    }
    if len(matched) == 0 {
        // 无模糊匹配时，尝试前缀匹配
        for idxTerm := range idx.termFreqs {
            if strings.HasPrefix(idxTerm, term) {
                matched = append(matched, idxTerm)
            }
        }
    }
    if len(matched) == 0 {
        matched = []string{term} // 回退到精确匹配
    }
    return matched
}

// tokenize 分词: 小写 + 英文按空格拆分 + 中文逐字拆分
// 参考 smart_gateway.py 的 _tokenize 实现，支持中英文混合
func tokenize(text string) []string {
    text = strings.ToLower(text)
    var tokens []string
    var buf strings.Builder
    for _, r := range text {
        if r >= 0x4E00 && r <= 0x9FFF { // CJK 统一汉字
            if buf.Len() > 0 {
                tokens = append(tokens, buf.String())
                buf.Reset()
            }
            tokens = append(tokens, string(r))
        } else if unicode.IsLetter(r) || unicode.IsDigit(r) {
            buf.WriteRune(r)
        } else {
            if buf.Len() > 0 {
                tokens = append(tokens, buf.String())
                buf.Reset()
            }
        }
    }
    if buf.Len() > 0 {
        tokens = append(tokens, buf.String())
    }
    return tokens
}
```

**与 MiniSearch (mcp-gateway) 的对应关系:**

| 特性 | MiniSearch (JS) | NewMCP (Go) |
|------|-----------------|-------------|
| 算法 | BM25Okapi | BM25Okapi |
| 字段权重 | name 3x, title 2x, desc 1x | name 3x, server 2x, desc 1x |
| 模糊匹配 | threshold 0.2 | Levenshtein ≤ 2 |
| 前缀搜索 | 支持 | 支持 (fuzzyExpand 回退) |
| 索引重建 | 全量销毁重建 | 按请求范围即时构建 |
| 外部依赖 | 零 | 零 |

### 8.3 搜索流程

```
mcp.search 请求
    │
    ▼
认证中间件: 解析 X-API-Key → apiKeyID
    │
    ▼
查询 api_keys: permissions.groups → ["robot-control", "data-analysis"]
    │
    ▼
收集分组内服务+工具 (从缓存):
    - robot-control: sea-bot(5 tools), air-drone(3 tools), arm(4 tools)
    - data-analysis: exa-search(3 tools), calculator(1 tool)
    = 5 服务 + 16 工具 = 21 个可搜索文档
    │
    ▼
buildIndex(21 docs) → BM25 倒排索引
    │
    ▼
search("机器人") → 模糊扩展 → 评分 → 排序 → 返回 top 10
```

### 8.4 市场浏览搜索 (前端 UI)

市场浏览是独立的 REST API，搜索全平台公开服务，不经过 MCP 网关:

```
GET /api/v1/marketplace?q=search&category=&page=1&page_size=20

→ 数据库查询:
  WHERE visibility='public' AND status=1
  AND (name LIKE '%search%' OR description LIKE '%search%')
→ 分页返回
```

10K 行的 `LIKE` 查询在 SQLite 中 <50ms，完全够用。未来规模更大时可切换到 FTS5/FULLTEXT，上层 API 无感。

#### 未来扩展：SQLite FTS5

当市场服务超过 50K 时，可启用 FTS5 全文索引。FTS5 原生支持 BM25 字段权重：

```sql
-- FTS5 虚拟表 (content table 模式，避免数据冗余)
CREATE VIRTUAL TABLE mcp_search USING fts5(
    service_name,
    tool_name,
    description,
    content='mcp_search_content',
    content_rowid='id',
    prefix='2 3 4',
    tokenize='unicode61 categories "L* N* Co"'
);

-- 内容表 + 同步触发器
CREATE TABLE mcp_search_content (
    id INTEGER PRIMARY KEY,
    service_name TEXT,
    tool_name TEXT,
    description TEXT
);

CREATE TRIGGER mcp_search_ai AFTER INSERT ON mcp_search_content BEGIN
    INSERT INTO mcp_search(rowid, service_name, tool_name, description)
    VALUES (new.id, new.service_name, new.tool_name, new.description);
END;
CREATE TRIGGER mcp_search_ad AFTER DELETE ON mcp_search_content BEGIN
    INSERT INTO mcp_search(mcp_search, rowid, service_name, tool_name, description)
    VALUES('delete', old.id, old.service_name, old.tool_name, old.description);
END;
CREATE TRIGGER mcp_search_au AFTER UPDATE ON mcp_search_content BEGIN
    INSERT INTO mcp_search(mcp_search, rowid, service_name, tool_name, description)
    VALUES('delete', old.id, old.service_name, old.tool_name, old.description);
    INSERT INTO mcp_search(rowid, service_name, tool_name, description)
    VALUES (new.id, new.service_name, new.tool_name, new.description);
END;
```

BM25 字段权重查询（name=3.0, tool=2.0, desc=1.0）：

```sql
SELECT sc.*, bm25(mcp_search, 3.0, 2.0, 1.0) AS score
FROM mcp_search ms
JOIN mcp_search_content sc ON ms.rowid = sc.id
WHERE ms.mcp_search MATCH ?
ORDER BY score
LIMIT 20;
```

中文分词可集成 [wangfenjin/simple](https://github.com/wangfenjin/simple) C 扩展（支持 jieba 分词 + 拼音搜索），或用 unicode61 unigram + 应用层预处理。索引大小预估：100K 文档约 25-50MB。

#### 未来扩展：MySQL FULLTEXT + ngram

MySQL 5.7.6+ 内置 ngram 分词器，原生支持 CJK：

```sql
-- ngram_token_size=2，将中文按双字切分
CREATE FULLTEXT INDEX ft_idx
ON mcp_services(name, description) WITH PARSER ngram;

-- 前缀搜索
WHERE MATCH(name, description) AGAINST('搜索词*' IN BOOLEAN MODE)
```

模拟字段权重（需每列单独 FULLTEXT 索引）：

```sql
SELECT *,
    (MATCH(name) AGAINST(?) * 3.0 +
     MATCH(description) AGAINST(?)) AS weighted_score
FROM mcp_services
WHERE MATCH(name, description) AGAINST(? IN BOOLEAN MODE)
  AND visibility = 'public'
ORDER BY weighted_score DESC
LIMIT 20;
```

---

## 9. MCP 协议端点汇总

| 路径 | 传输 | 模式 | 说明 |
|------|------|------|------|
| `/mcp` | Streamable HTTP | 固定 Direct | 主网关，暴露 API Key 绑定分组全部工具 |
| `/smart/mcp` | Streamable HTTP | 固定 Smart | Smart 网关，仅暴露 3 个元工具 |
| `/mcp/group/{slug}` | Streamable HTTP | 按 group 配置 | 分组 MCP 端点 |
| `/mcp/ws` | WebSocket | 固定 Direct | 主网关 WebSocket |
| `/smart/mcp/ws` | WebSocket | 固定 Smart | Smart 网关 WebSocket |
| `/mcp/ws/group/{slug}` | WebSocket | 按 group 配置 | 分组 WebSocket 端点 |
| `/mcp/passive/` | WebSocket | 被动接入 | 外部 MCP 服务连入注册 (token 认证) |

Smart 模式下的 `tools/list` 永远返回 3 个元工具。
Direct 模式下的 `tools/list` 返回聚合后的完整工具列表。
被动接入端点 `/mcp/passive/` 供外部 MCP Server 连入，NewMCP 作为 MCP Client 发现和调用工具。
