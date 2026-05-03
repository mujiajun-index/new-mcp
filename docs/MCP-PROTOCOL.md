# NewMCP 协议适配说明

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

## 1. 双模式网关架构

NewMCP 支持两种 MCP 工具暴露模式，按分组粒度配置：

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

分组配置中的 `expose_mode` 字段控制模式：
```json
{
    "expose_mode": "smart"  // "direct" 或 "smart"
}
```

---

## 2. Smart 模式元工具

### 2.1 工具列表

| 工具名 | 说明 | 对应 mcp-gateway |
|--------|------|------------------|
| `mcp.search` | 搜索可用的 MCP 服务和工具 | `gateway.search` |
| `mcp.describe` | 查看指定工具的完整 Schema | `gateway.describe` |
| `mcp.execute` | 执行指定工具 | `gateway.invoke` |
| `mcp.execute_async` | 异步执行工具（可选） | `gateway.invoke_async` |
| `mcp.job_status` | 查询异步任务状态（可选） | `gateway.invoke_status` |

V1 核心实现前 3 个，后 2 个作为 V1.1 扩展。

### 2.2 mcp.search - 搜索可用 MCP / 工具

**功能**: 支持关键字搜索 MCP 服务名、分组名、工具名、描述。使用 BM25 算法 + 模糊匹配。

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

**搜索算法 (参考 mcp-gateway):**
- **BM25 评分**: 标准文本相关性算法
- **字段权重**: 服务名 3x / 工具名 2x / 描述 1x
- **模糊匹配**: 0.2 阈值容忍拼写错误
- **前缀匹配**: 支持 "搜" 匹配 "搜索"

**Go 实现:**
```go
// internal/mcp/smart/search_engine.go

type SearchEngine struct {
    index  *bleve.Index  // 或自定义 BM25 实现
    mu     sync.RWMutex
}

type SearchDocument struct {
    ID          string  // "serviceName" 或 "serviceName.toolName"
    Type        string  // "mcp" 或 "tool"
    Name        string
    Description string
    Group       string
    ServerName  string
    ToolCount   int     // 仅 mcp 类型
}

func (e *SearchEngine) Search(query string, scope string, group string, limit int) ([]SearchResult, error) {
    // 1. 构建搜索请求 (BM25 + 字段权重)
    // 2. 可选: 添加 group 过滤
    // 3. 可选: 添加 type 过滤 (mcp/tool)
    // 4. 执行搜索，返回排序结果
}

// Rebuild 重建索引 (当 MCP 服务变更时调用)
func (e *SearchEngine) Rebuild(tools []ToolDocument) {
    // 清空索引
    // 重新索引所有文档
}
```

### 2.3 mcp.describe - 查看工具详细 Schema

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

### 2.4 mcp.execute - 执行指定工具

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

## 3. Direct 模式实现

### 3.1 工具聚合 (tools/list)

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

### 3.2 工具路由 (tools/call)

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

## 4. 模式分发

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

## 5. Transport Adapter 实现

### 5.1 接口定义

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
    TypeStdio          TransportType = "stdio"
    TypeSSE            TransportType = "sse"
    TypeStreamableHTTP  TransportType = "streamable-http"
    TypeWebSocket      TransportType = "websocket"
)
```

### 5.2 适配器实现

各传输协议适配器（StdioAdapter, StreamableHTTPAdapter, WebSocketAdapter, SSEAdapter）实现同前，此处不重复。

---

## 6. 会话池与工具目录缓存

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

## 7. 搜索引擎实现 (Smart 模式核心)

参考 mcp-gateway 的 MiniSearch + BM25 实现：

```go
// internal/mcp/smart/search_engine.go

import "github.com/blevesearch/bleve/v2"

type SearchEngine struct {
    index    bleve.Index
    mu       sync.RWMutex
    ready    bool
}

type SearchDoc struct {
    ID          string  `json:"id"`            // "exa-search" 或 "exa-search.web_search"
    Type        string  `json:"type"`          // "mcp" 或 "tool"
    Name        string  `json:"name"`          // 服务名或工具名
    Description string  `json:"description"`   // 描述
    Group       string  `json:"group"`         // 分组名
    ServerName  string  `json:"server_name"`   // 所属 MCP 服务名 (工具类型)
    ToolCount   int     `json:"tool_count"`    // 工具数 (服务类型)
}

func NewSearchEngine() (*SearchEngine, error) {
    mapping := bleve.NewIndexMapping()
    // 配置字段权重
    docMapping := bleve.NewDocumentMapping()
    nameField := bleve.NewTextFieldMapping()
    nameField.Boost = 3.0  // 名字匹配 3x 权重
    docMapping.AddFieldMappingsAt("name", nameField)

    descField := bleve.NewTextFieldMapping()
    descField.Boost = 1.0
    docMapping.AddFieldMappingsAt("description", descField)

    mapping.AddDocumentMapping("default", docMapping)

    index, err := bleve.NewMemOnly(mapping)
    if err != nil {
        return nil, err
    }
    return &SearchEngine{index: index}, nil
}

func (e *SearchEngine) Search(query string, scope string, group string, limit int) ([]SearchResult, error) {
    e.mu.RLock()
    defer e.mu.RUnlock()

    if !e.ready {
        return nil, fmt.Errorf("search index not ready")
    }

    // 构建查询
    bq := bleve.NewBooleanQuery()

    // 文本搜索
    textQuery := bleve.NewMatchQuery(query)
    textQuery.SetBoost(1.0)
    bq.AddMust(textQuery)

    // 可选: scope 过滤
    if scope != "" && scope != "all" {
        scopeQuery := bleve.NewMatchPhraseQuery(scope)
        scopeQuery.SetField("type")
        bq.AddMust(scopeQuery)
    }

    // 可选: group 过滤
    if group != "" {
        groupQuery := bleve.NewMatchPhraseQuery(group)
        groupQuery.SetField("group")
        bq.AddMust(groupQuery)
    }

    search := bleve.NewSearchRequest(bq)
    search.Size = limit
    search.IncludeLocations = true

    results, err := e.index.Search(search)
    if err != nil {
        return nil, err
    }

    // 转换结果
    var searchResults []SearchResult
    for _, hit := range results.Hits {
        doc, _ := e.index.GetDocument(hit.ID)
        searchResults = append(searchResults, SearchResult{
            ID:          hit.ID,
            Score:       hit.Score,
            Name:        getStringField(doc, "name"),
            Description: getStringField(doc, "description"),
            Type:        getStringField(doc, "type"),
            Group:       getStringField(doc, "group"),
        })
    }

    return searchResults, nil
}

// Rebuild 重建搜索索引 (MCP 服务变更时调用)
func (e *SearchEngine) Rebuild(services []McpServiceInfo, tools []ToolDocument) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    // 创建新索引
    newIndex, _ := bleve.NewMemOnly(e.index.Mapping())

    // 索引 MCP 服务文档
    for _, svc := range services {
        doc := SearchDoc{
            ID:          svc.Name,
            Type:        "mcp",
            Name:        svc.Name,
            Description: svc.Description,
            Group:       svc.GroupName,
            ToolCount:   svc.ToolCount,
        }
        newIndex.Index(doc.ID, doc)
    }

    // 索引工具文档
    for _, tool := range tools {
        doc := SearchDoc{
            ID:          tool.ServiceName + "." + tool.Name,
            Type:        "tool",
            Name:        tool.Name,
            Description: tool.Description,
            Group:       tool.GroupName,
            ServerName:  tool.ServiceName,
        }
        newIndex.Index(doc.ID, doc)
    }

    // 替换索引
    oldIndex := e.index
    e.index = newIndex
    e.ready = true
    oldIndex.Close()

    return nil
}
```

---

## 8. MCP 协议端点汇总

| 路径 | 传输 | 模式 | 说明 |
|------|------|------|------|
| `/mcp/group/{slug}` | Streamable HTTP | 按 group 配置 | 分组 MCP 端点 |
| `/mcp/ws/group/{slug}` | WebSocket | 按 group 配置 | 分组 WebSocket 端点 |
| `/mcp` | Streamable HTTP | 暴露所有工具 | 主网关 (direct 模式) |
| `/mcp/ws` | WebSocket | 暴露所有工具 | 主网关 WebSocket |

Smart 模式下的 `tools/list` 永远返回 3 个元工具。
Direct 模式下的 `tools/list` 返回聚合后的完整工具列表。
