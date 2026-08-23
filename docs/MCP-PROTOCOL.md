# NewMCP 协议适配说明

> 版本: V1.2 | 状态: 草案 | 更新日期: 2026-08-21

## 1. 双模式网关架构

NewMCP 支持两种 MCP 工具暴露模式，**通过端点路由驱动**：

| 端点 | 模式 | 说明 |
|------|------|------|
| `POST /mcp` | 固定 Direct | 聚合 API Key 所有分组，去重后暴露全部工具（`serviceName__toolName`） |
| `POST /smart/mcp` | 固定 Smart | 聚合 API Key 所有分组，仅暴露 5 个元工具，渐进发现 |
| `POST /mcp/group/{slug}` | 由分组的 `expose_mode` 决定 | 端点驱动，每个分组独立配置 |
| `WS /mcp/ws` | 固定 Direct | 同 POST /mcp |
| `WS /smart/mcp/ws` | 固定 Smart | 同 POST /smart/mcp |
| `WS /mcp/ws/group/{slug}` | 由分组的 `expose_mode` 决定 | 端点驱动 |

> **Direct 主端点**: `/mcp` 暴露 API Key 绑定分组的全部工具（去重），适合 Claude Code、Cursor 等支持大量工具的 LLM 客户端。
> **Smart 主端点**: `/smart/mcp` 仅暴露 5 个元工具，适合小智等上下文受限设备或工具量特别大的场景。

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
| `mcp.search` | 搜索可用的 MCP 服务、工具、资源和提示 | `gateway.search` |
| `mcp.describe` | 查看服务详情（工具 Schema / 资源 URI / 提示定义） | `gateway.describe` |
| `mcp.execute` | 执行指定工具 | `gateway.invoke` |
| `mcp.execute_batch` | 批量并发执行多个**相互独立**的工具（见 3.5） | — |
| `mcp.read` | 读资源 / 取提示（V1.2 新增，见 3.6） | OpenAI `read_mcp_resource` 同构 |
| `mcp.execute_async` | 异步执行工具（可选） | `gateway.invoke_async` |
| `mcp.job_status` | 查询异步任务状态（可选） | `gateway.invoke_status` |

前 4 个 + `mcp.read` 为已实现（`mcp.read` 为 V1.2 新增，使纯 tools/call 客户端也能用满
Resources/Prompts 能力；`mcp.execute_batch` 复用单项执行路径，逐项计费/日志）；
`mcp.execute_async` / `mcp.job_status` 作为扩展未实现。

V1.2 起 `mcp.search` 的 scope 支持 `mcp/tool/resource/prompt/all`（resource 含资源模板），
`mcp.describe` 的服务视图在工具之外追加 Resources / Resource Templates / Prompts 小节
（读 resources_cache/prompts_cache，不连上游；条目命名空间形态与 resources/list、
prompts/list 一致，均可被分组勾选禁用过滤，禁用条目不出现在搜索与描述里）。

### 3.2 mcp.search - 搜索可用 MCP / 工具

**功能**: 在 API Key 绑定的分组范围内，使用 BM25 算法搜索 MCP 服务和工具。

**参数:**
```json
{
    "query": "网络工具 [web search]", // 可选，省略则浏览全部条目；非英文任务建议双语关键词（原词旁附英文翻译）
    "scope": "all",              // 可选: "mcp"/"tool"/"resource"/"prompt"/"all" (默认 "all"，resource 含资源模板；不确定时保持 all，明确只找某一类时再收窄)
    "group": "机器人控制",         // 可选，限定分组
    "limit": 20                  // 可选，最大 100，默认 20
}
```

**返回**（按类型分节，实现在 `smart.FormatSearchResult`）:
```json
{
    "content": [{
        "type": "text",
        "text": "Found 4 items:\n\n" +
            "Services — inspect with mcp.describe \"<name>\":\n" +
            "- exa (Exa 搜索) — Exa 网络搜索引擎 (3 tools, group: 联网工具)\n\n" +
            "Tools — call with mcp.execute \"service.tool\":\n" +
            "- exa.web_search — Search the web for any topic and get clean, ready-to-use content. Best for: Finding current information, news, facts…\n\n" +
            "Prompts — render with mcp.read (type \"prompt\"):\n" +
            "- exa__summarize — 汇总搜索结果"
    }]
}
```

格式要点:

- 按类型分节（服务/工具/资源/资源模板/提示），节标题携带该类条目的下一步动作
  （describe/execute/read），只输出非空节，节间空行分隔；
- 每行一条 `ID — 摘要`；描述折叠为单行并按 160 字符截断（上游多行/超长描述不再
  破坏列表格式，细节交给 mcp.describe）；
- 服务行附带工具数与分组名；同一服务挂多个分组时按条目 ID 去重（保留首分组）；
- 结果数达到 limit 上限时头部提示 "limit reached, more may exist"；0 结果时返回
  放宽条件的建议文案；query 仅含汉字（未附英文关键词）时改为点破语言原因，引导
  附上英文重试（目录名称/描述以英文为主，中文分词与英文索引零重叠）；query 已含
  英文关键词仍空结果则退回通用建议（语言已不是问题）。

  提示词层面同步引导"双语关键词"：`mcp.search` 的 description、`query` 参数描述
  与智能模式 instructions 均要求非英文任务在原词旁附上英文翻译（如
  网络工具 [web search]），不让模型在用户语言与英文之间二选一——BM25 按查询词
  OR 累计得分，双语 query 同时命中英文目录与中文描述的上游服务。

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

### 3.5 mcp.execute_batch - 批量并发执行

**功能**: 一次调用并发执行最多 10 个**相互独立**的工具调用，逐项返回结果——按入参顺序，
每项一个 `[index] tool_id — ok|failed` 头块，其后原样透传该项上游 content 块
（text/image 等类型与字节不变）。每项走与 mcp.execute 完全相同的执行路径
（分组作用域校验、虚拟工具分发、计费插入点 A/B、逐项超时），网关内并发度上限 5
（信号量钳制，避免单次批量瞬时打满上游限流）。

**适用与不适用**（工具描述里对模型作同样约束）:

- ✅ 参数全部已知、互不依赖：一次查 3 个城市天气；识别图片 + 网络搜索并行；
- ✅ 批量控制开关/设备：同一工具按不同设备重复调用（客厅灯、卧室灯、插座各一次，
  每设备参数独立已知），这正是小智等 IoT 场景的典型批量需求；
- ❌ 某项参数依赖另一项的返回值（先上传拿 URL、再识别）：该项参数在构造批量请求时
  根本不存在，硬打包会得到空引用或幻觉参数；
- ❌ **同一目标**需按序操作的组合（先设置设备再读回其状态、先创建后列表）：
  结果取决于执行顺序，并发下不可预期；不同目标的独立写操作（多设备开关）不受此限。

部分失败互不影响：失败项在结果里带原因（含上游 `isError: true` 的工具级错误，MCP
语义下工具错误进 result 不进协议 error），汇总块提示"修正后用 mcp.execute 单项重试"；
**全部**项失败才置 `isError: true`（部分失败是批量调用的正常结局）。上游结果缺
content 块（如仅 structuredContent）或不可解析时，退化为截断 2048 字符的原文文本块。

**参数**（两种形态二选一;`timeout_ms` 为整批统一超时）:

同工具扇出——推荐用于批量设备控制等"一个工具调 N 次"的场景（少一层嵌套、无需
逐项重复 tool_id,显著降低小参数量模型生成非法 JSON 的概率）:
```json
{
    "tool_id": "home.set_switch",
    "arguments_list": [
        {"device": "light.living_room", "on": true},
        {"device": "light.bedroom", "on": true},
        {"device": "socket.desk", "on": false}
    ],
    "timeout_ms": 30000
}
```

混合不同工具:
```json
{
    "calls": [
        {"tool_id": "weather.get_forecast", "arguments": {"city": "北京"}},
        {"tool_id": "exa.web_search_exa", "arguments": {"query": "AI news"}}
    ]
}
```

**返回:**
```json
{
    "content": [
        {"type": "text", "text": "Batch of 2 calls: 2 ok, 0 failed."},
        {"type": "text", "text": "[0] weather.get_forecast — ok"},
        {"type": "text", "text": "北京: 晴，温度 22°C"},
        {"type": "text", "text": "[1] exa.web_search_exa — ok"},
        {"type": "text", "text": "[搜索结果...]"}
    ]
}
```

**实现要点** (`internal/mcp/handler/gateway_handler.go`):
- 单项执行抽为 `executeOne`，单次 mcp.execute 与批量共用同一路径（含计费 A/B），
  两者行为完全一致；
- 双入参形态（`calls` 混合工具 / `tool_id`+`arguments_list` 同工具扇出）在入口
  归一化为同一 calls 切片，下游并发/计费/日志/聚合路径完全不变；两种形态同给、
  全缺、超限均为协议级 -32602；
- 计费幂等键 `request_id` 在工具名哈希部分带批内序号（`tool_id#i`）：批内两项
  (tool_id, arguments) 完全相同时若共键，第二项预扣会被幂等去重漏扣；
- 日志逐项一条（method=`mcp.execute_batch`，service/tool/分组/计费列按项归属；
  项级作用域/路由失败拿不到裸工具名时 tool_name 记完整 tool_id），请求数统计只按
  一次请求递增；
- 校验失败（形态缺失/互斥冲突/超限/缺 tool_id）为协议级 -32602 错误，走单条日志路径。

### 3.6 mcp.read - 读资源 / 取提示（V1.2）

**功能**: 让只走 tools/call 的智能模式客户端（OpenAI Agents SDK 风格托管集成、自研 Agent）
也能读取资源与提示，补齐 MCP 三大能力的工具入口。target 与原生方法同一套字符串——
资源为网关 URI `newmcp://{service}/{原始URI}`，提示为命名空间名 `{service}__{name}`，
即 mcp.search / mcp.describe 输出里直接可用的形态。

**参数:**
```json
{
    "type": "resource",                      // "resource" | "prompt"
    "target": "newmcp://weather/file:///alerts.csv",
    "arguments": {"lang": "go"}              // 仅 prompt，透传给上游 prompts/get
}
```

**返回**: tools/call 的 content 形态——资源 text 直传、图片 blob 转 image 内容项、
其余 blob 以占位说明返回；提示按消息转 text 并带 `[role]` 前缀。

**实现要点** (`internal/mcp/handler/resources_prompts.go` handleMetaRead):
- 复用原生 `resources/read` / `prompts/get` 的上游核心（readUpstreamResource / getUpstreamPrompt），
  API key 范围校验、分组禁用拒绝（读侧强制）、命名空间回写全部继承；
- 计费口径与原生一致：记 mcp_call_logs、市场服务不扣费（billing_status=skipped）；
- 日志 method 记 `mcp.read`，tool_name 记目标 URI/提示名——与 mcp.execute（method=mcp.execute）
  一样可在调用日志中区分智能模式入口（前端日志页据此显示「智能」徽标）。

**模式门控（V1.2）**: 智能模式下原生 resources/prompts 枚举收敛——`resources/list`、
`resources/templates/list`、`prompts/list` 返回空列表，`initialize` 不声明 resources/prompts
能力（防止连接时自动枚举的客户端绕过元工具全量拉取）；直连模式两者照常。
`resources/read`、`prompts/get` 在智能模式仍可用：单 URI 定点读取无枚举开销，且与
mcp.read 共用同一上游核心（鉴权/禁用/日志语义完全一致）。分组端点按分组 expose_mode 判定。

**arguments 空对象兜底（V1.2，传输层）**: `prompts/get`、`tools/call` 的 `arguments` 按
规范是可选字段，但 typescript-sdk 1.x 的 prompts 处理器会把缺失的 arguments 当 undefined
交给 zod 校验而拒绝（exa 零参数提示 `web_search_help` 即此例：exa-labs/exa-mcp-server#358、
typescript-sdk#1869）；go-sdk 的 `GetPromptParams.Arguments` 带 `omitempty`，nil/空 map 都
序列化不出该字段（v1.7.0 亦然，升级无效）。网关在 streamable-http/sse 适配器的 HTTP
RoundTripper 层把缺失的 `arguments` 补成显式 `{}`（TS 社区推荐的传输层规范化方案），
已有 arguments 的请求不改写。stdio 传输未覆盖此兜底（exa 类问题均在 HTTP 上游）。

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
    case "mcp.execute_batch":
        return h.smartHandler.HandleExecuteBatch(ctx, groupID, arguments)
    default:
        return nil, fmt.Errorf("unknown meta tool: %s", toolName)
    }
}

// getMetaTools 返回 Smart 模式的固定元工具列表
// 描述文案遵循三段式最佳实践(做什么/何时用/返回什么)+相似工具互相指路,
// 完整文案见 internal/mcp/smart/meta_tools.go,此处为节选。
func (h *GatewayHandler) getMetaTools() []Tool {
    return []Tool{
        {
            Name:        "mcp.search",
            Description: "Search the catalog of available MCP services, tools, resources, and prompts by keyword. ... Returns matching items with their exact IDs: tools as `service.toolName` (call via mcp.execute), resources as `newmcp://service/...` URIs and prompts as `service__promptName` (fetch via mcp.read). Use this FIRST when you don't yet know which service or tool fits a task ...",
            InputSchema: searchToolSchema,
        },
        {
            Name:        "mcp.describe",
            Description: "Inspect what a specific MCP service or tool offers, by exact name. Given a service name, returns its full inventory: tools with parameter schemas, resource URIs, and prompts with their arguments. Given `service.toolName`, returns that one tool's description and complete input schema. ...",
            InputSchema: describeToolSchema,
        },
        {
            Name:        "mcp.execute",
            Description: "Execute an MCP tool by ID with a JSON arguments object, returning the tool's execution result. The tool_id and the exact arguments it accepts come from mcp.search / mcp.describe — if unsure what arguments a tool takes, run mcp.describe on it first instead of guessing. ...",
            InputSchema: executeToolSchema,
        },
        {
            Name:        "mcp.execute_batch",
            Description: "Execute multiple independent MCP tools concurrently in one call, returning one result per item in input order, each under an \"[index] tool_id\" header. Use it when several calls are ready at once and none needs another's output — e.g. turning several devices on/off together (repeating the same tool with different arguments per device is fine). Do NOT use it when a call's arguments depend on an earlier call's result, or when calls must hit the SAME target in order: run those one at a time with mcp.execute instead. ...",
            InputSchema: executeBatchToolSchema,
        },
        {
            Name:        "mcp.read",
            Description: "Read an MCP resource by URI, or render an MCP prompt with arguments. Targets use the exact gateway forms returned by mcp.search / mcp.describe: resources as `newmcp://<service>/<upstream-uri>`, prompts as `<service>__<promptName>` ...",
            InputSchema: readToolSchema,
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
| `/smart/mcp` | Streamable HTTP | 固定 Smart | Smart 网关，仅暴露 5 个元工具 |
| `/mcp/group/{slug}` | Streamable HTTP | 按 group 配置 | 分组 MCP 端点 |
| `/mcp/ws` | WebSocket | 固定 Direct | 主网关 WebSocket |
| `/smart/mcp/ws` | WebSocket | 固定 Smart | Smart 网关 WebSocket |
| `/mcp/ws/group/{slug}` | WebSocket | 按 group 配置 | 分组 WebSocket 端点 |
| `/mcp/passive/` | WebSocket | 被动接入 | 外部 MCP 服务连入注册 (token 认证) |

Smart 模式下的 `tools/list` 永远返回 5 个元工具。
Direct 模式下的 `tools/list` 返回聚合后的完整工具列表。
被动接入端点 `/mcp/passive/` 供外部 MCP Server 连入，NewMCP 作为 MCP Client 发现和调用工具。

---

## 10. Resources / Prompts 聚合透传

> 参考 Cherry Studio `createMcpBridgeServer` 的桥接模式。网关在 tools 之外，
> 同样聚合透传 MCP 协议的另外两类能力：

| JSON-RPC 方法 | 说明 |
|---------------|------|
| `resources/list` | 聚合范围内全部上游服务的静态资源 |
| `resources/templates/list` | 聚合上游的资源模板（RFC 6570 URI Template） |
| `resources/read` | 按网关 URI 路由回源，透传读取结果 |
| `prompts/list` | 聚合上游提示词模板 |
| `prompts/get` | 按命名空间提示名路由回源，透传渲染结果 |

### 10.1 命名空间（多服务聚合的关键）

不同上游服务的 URI / 提示名可能冲突，聚合暴露时统一加命名空间：

```
资源 URI:   newmcp://{serviceName}/{上游原始URI}
资源模板:   uriTemplate 同样加 newmcp://{serviceName}/ 前缀（模板变量原样保留）
提示名:     {serviceName}__{promptName}   （与工具的 "__" 约定一致）
```

- `resources/read` 收到 `newmcp://weather/file:///alerts.csv` 时拆出服务名 `weather`，
  把原始 URI `file:///alerts.csv` 转发给对应上游；返回的 `contents[].uri` 会回写为网关 URI，
  保证客户端看到的所有 URI 形态一致。
- 模板展开出的 URI（如 `newmcp://memo/memo://items/42`）同样按前缀路由，无需预注册。
- 提示名经 `ParseNamespacedName` 拆解后转发 `prompts/get`，arguments 原样透传。

### 10.2 范围与容错

- 范围口径与 `tools/list` 一致：`/mcp`、`/smart/mcp` 聚合 API Key 绑定分组的全部服务
  （去重）；`/mcp/group/{slug}` 限定该分组并校验访问权。vision/camera 虚拟服务不参与。
- 聚合并发连上游（上限 8）；**单个服务连接/拉取失败只跳过该服务**，不影响整体响应；
  范围解析失败（分组不存在/无权限）返回 JSON-RPC error。
- 上游未声明 resources/prompts 能力时直接跳过（不发多余请求）；列表实时拉取不落库，
  上游变更在下一次 list 即可见（网关不声明 `listChanged`，客户端自行重新 list）。
- Smart 端点同样暴露资源/提示——资源枚举由客户端主动发起，不占用工具上下文。

### 10.3 分组内条目级启停（资源/提示勾选）

与工具过滤同一交互：分组详情页可按条目勾选启停资源/提示，服务详情页展示资源/提示缓存列表。

- 存储：`mcp_group_items`（group_id, service_id, item_kind, item_key, enabled），
  **无行 = 启用**（与 mcp_group_tools 语义一致，无回填）。item_kind 为
  `resource`（item_key=原始 URI）/ `template`（item_key=uriTemplate）/ `prompt`（item_key=提示名）。
- 枚举来源：`mcp_services.resources_cache`（`{"resources":[],"templates":[]}`）与
  `prompts_cache`（裸数组），连接时异步预热、`POST /services/:id/refresh-tools` 同步刷新。
- 网关强制：list 聚合剔除禁用条目；`resources/read`、`prompts/get` **拒绝禁用条目**
  （按 API key 范围内首个包含该服务的分组判定，与聚合去重取首分组一致）——否则
  隐藏但仍可直读，勾选形同虚设。模板禁用只隐藏 templates/list 条目，按模板展开出的
  URI 读取不做前缀匹配拦截（已知边界）。
- API：`GET /groups/:id/resources`、`PUT /groups/:id/resources/batch`、
  `GET /groups/:id/prompts`、`PUT /groups/:id/prompts/batch`、
  `GET /services/:id/resources`、`GET /services/:id/prompts`。

### 10.4 计费与日志

`resources/read`、`prompts/get` 会真实触达上游，与 `tools/call` 一样记入
`mcp_call_logs`（method = `resources/read` / `prompts/get`）；计费口径暂与
tools/call 不同——市场服务不扣费（`billing_status` 落默认 `skipped`）。
`resources/list`、`prompts/list` 等枚举方法不记日志（与 `tools/list` 一致）。

### 10.5 版本协商（initialize）

```json
// 请求
{"method": "initialize", "params": {"protocolVersion": "2025-06-18", ...}}

// 响应:请求版本在支持列表内则原样回显;否则回落最新支持版本(客户端不支持时自行断开)
{"result": {"protocolVersion": "2025-06-18", "capabilities": {"tools": {}, "resources": {}, "prompts": {}}, ...}}
```

- 支持版本集合：`2025-11-25` / `2025-06-18` / `2025-03-26` / `2024-11-05`
  （与 go-sdk v1.6.1 的 `supportedProtocolVersions` 保持同步），最新版本 `2025-11-25`。
- 协商行为与官方 TS/Go SDK 及 Cherry Studio 一致。
- `serverInfo.version` 取 `common.Version`（构建时由 VERSION 文件经 ldflags 注入）。
