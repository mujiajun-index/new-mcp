# NewMCP 协议适配说明

> 版本: V1.4 | 更新日期: 2026-10-02 | 正式 MCP `2026-07-28` + NewMCP 实验 Events 扩展

本文描述代码的协议能力。部署中的实例需要运行包含这些改动的构建；本文不表示现有实例已更新或重启。MCP 的正式版本使用日期标识，JSON-RPC `2.0` 与 SDK v2 不等同于“MCP 2.0”。

## 1. 双模式网关架构

NewMCP 支持两种 MCP 工具暴露模式，**通过端点路由驱动**：

| 端点 | 模式 | 说明 |
|------|------|------|
| `POST /mcp` | 固定 Direct | 聚合 API Key 所有分组，去重后暴露全部工具（`serviceName__toolName`） |
| `POST /smart/mcp` | 固定 Smart | 聚合 API Key 所有分组，仅暴露 5 个元工具，渐进发现 |
| `POST /mcp/group/{slug}` | 由分组的 `expose_mode` 决定 | 端点驱动，每个分组独立配置 |

客户端 WebSocket 网关路由 `/mcp/ws`、`/smart/mcp/ws`、`/mcp/ws/group/{slug}` 已支持 MCP JSON-RPC 请求、响应及订阅通知。下文的 `websocket` / `passive-ws` 是独立的上游服务接入能力。

> **Direct 主端点**: `/mcp` 暴露 API Key 绑定分组的全部工具（去重），适合 Claude Code、Cursor 等支持大量工具的 LLM 客户端。
> **Smart 主端点**: `/smart/mcp` 仅暴露 5 个元工具，适合小智等上下文受限设备或工具量特别大的场景。

### 1.1 Direct 模式（直接模式）

将分组内所有上游 MCP 服务的工具聚合后直接暴露，添加命名空间前缀。

- 适合: 工具数量少（<20）的场景
- LLM 直接看到所有工具 schema
- 一步调用，延迟低

### 1.2 Smart 模式（智能模式）

通常暴露 5 个元工具（Meta Tools）；管理员启用智能搜索且用户分组获准时，还会暴露 `mcp.smart_search`。LLM 通过搜索→查看→执行渐进发现和调用工具。参考 [eznix86/mcp-gateway](https://github.com/eznix86/mcp-gateway)。

- 适合: 工具数量多（20+）的场景、小智等受限设备
- Token 消耗极低，永远只暴露几个元工具
- 无 MCP 规模上限

`mcp.smart_search` 接受 `query`（自然语言需求，必填）、`limit`（1–10，默认 3）和可选的 MCP `group`。它只比较当前 API Key 与端点可调用的工具，排除被禁用的工具。用 System One Choice 对工具做语义排序：默认每批 200 个工具并附加“都不合适”选项；超过一批时复排各批候选。返回的 `probability` 是最终 Choice 全部选项间的相对概率，并非工具能力的绝对概率；概率为 0 的工具不会列出。`no_match_probability` 给出最终“都不合适”选项的相对概率，`choice_confidence` 是上游报告的决策置信度，`evaluated_tool_count` 和 `finalist_count` 分别表示可用工具总数与最终复排数。模型选中“都不合适”时返回空结果；上游失败时返回错误。工具只负责推荐，调用前仍应使用 `mcp.describe` 查看参数。

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
    "limit": 20,                 // 可选，每页最大条数，最大 100，默认 20
    "offset": 0                  // 可选，跳过前 N 条匹配（SQL OFFSET 语义，V1.3），与 limit 组合翻页，默认 0
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
- 分页（V1.3）：引擎返回切片前统计的匹配总数，跨页时头部输出
  `Found <总数> matches — showing X-Y (pass offset=<下一页偏移> for the next page)`，
  末页标 `(end of results)` 终止翻页；结果一页放得下且 offset=0 时维持
  `Found N items`；offset 越过总数时提示调低 offset。BM25 同分条目按文档序号
  破平（排序稳定），保证翻页时条目不在两页间漂移；
- 0 结果时返回放宽条件的建议文案；query 仅含汉字（未附英文关键词）时改为点破
  语言原因，引导附上英文重试（分词已带拼音桥，仍空结果多为拼音兜不住的专有名词
  缩写/非常规罗马化）；query 已含英文关键词仍空结果则退回通用建议（语言已不是问题）。

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
| 被动 WebSocket 接入 | passive-ws | 外部 → NewMCP | NewMCP 生成接入 URL，外部服务主动连入 |

### 6.3 被动 WebSocket 接入 (passive-ws)

创建服务时选择 `passive-ws`，平台生成独立的接入地址。外部 MCP Server 或其本地桥接程序主动连接，NewMCP 作为 MCP Client 发现和调用工具：

```
本地 MCP Server/桥接程序 ──主动连接──> NewMCP /mcp/passive/
                         <── MCP 版本协商、tools/list ──
                         ── 服务器信息与工具目录 ──>

LLM 客户端 ──HTTP 网关调用──> NewMCP ──同一 WS 连接 tools/call──> 本地 MCP Server
```

**接入地址与凭证：**

```
wss://api.newmcp.pro/mcp/passive/?token=<serviceID>.<random-secret>
```

- 接入 URL 根据系统设置 `ServerAddress` 生成，HTTPS → WSS、HTTP → WS；部署公网接入时须配置真实的 HTTPS 地址及 TLS 终止代理。
- secret 是 32 字节（256 位）随机值，以 URL-safe Base64 表示并加密存入 `passive_token`，与登录 JWT 和网关 API Key 独立。接入地址仅向服务所有者返回。
- 创建时无需 URL、命令或上游认证配置；固定 `config={}`、`auth_type=none`、`auth_config={}`。每个端点对应一个 MCP 服务，多个本地服务分别注册。被动接入服务不能克隆上架到市场，接入后的工具通过现有分组、API Key、Direct 和 Smart 流程使用。
- `POST /api/v1/services/:id/passive-token/reset` 返回更新后的服务详情。旧凭证立即失效并关闭当前连接，随后用新地址重新接入。

**连接、调用与目录同步：**

- WebSocket 文本帧直接携带 MCP JSON-RPC，没有额外消息包装或工具注册消息。官方 Go SDK 处理版本协商（包括旧版 `initialize` / `notifications/initialized`）、分页、请求 ID 和并发响应。
- 新连接完成握手和工具发现后加入会话池，更新工具目录、协议版本及服务器信息。同一服务最近成功初始化的连接替换旧连接；新连接失败时旧连接仍可调用。
- `tools/call`、资源与提示词调用复用当前连接。收到 `notifications/tools/list_changed` 或用户点击重新同步时，通过当前连接刷新工具目录。
- 尚未接入时显示“等待服务接入”；测试接口不会主动拨号，已在线时通过当前连接执行 MCP ping。断线后目录保留，调用返回离线错误，重新接入后重新同步。
- 停用、删除服务或重置凭证会关闭连接。平台重启将在线标记重置，保留接入凭证和目录，等待本地桥接程序重新连接。

### 6.4 适配器实现

stdio、SSE、Streamable HTTP、主动 WebSocket 和被动 WebSocket 都复用官方 SDK 的 `ClientSession`。WebSocket 传输适配器将文本帧映射到 SDK I/O 流，并维持 ping/pong 心跳。主动 WebSocket 支持 `ws://` / `wss://` 和静态认证请求头；SSE 使用旧版 MCP HTTP+SSE 的端点事件与消息 POST 流程。

### 6.5 Stdio stdout 容忍过滤

MCP stdio 传输约定 stdout 只能出现 JSON-RPC 消息，但不少社区服务会在启动时往
stdout 打横幅/日志（如 bazi-mcp 的 `Bazi MCP server is running on stdio.`）。
官方 go-sdk 的 `CommandTransport` 严格按 JSON 流解析 stdout，遇到垃圾字节会在
initialize 阶段直接断连（`invalid character 'B' ...`），表现为「创建成功但工具
拉取不到」。

`StdioFilterTransport`（`internal/mcp/transport/stdio_filter.go`）为此替代了
`CommandTransport`：

- 自己拉起子进程，stdout 经行过滤器净化后交给官方 `mcp.IOTransport`（连接
  解析、JSON-RPC batch、关闭语义全部复用官方实现）；
- 行过滤器只放行「能构成完整 JSON 对象/数组」的内容（JSON-RPC 消息的合法
  形态），横幅/日志行丢弃并记入平台日志（带 `[stdio service <id> <命令>]`
  前缀，最多逐条记 100 行）；跨行 pretty JSON、`\r\n`、UTF-8 BOM 均可容忍；
- 子进程 stderr 持续排空并逐行记日志（npx 下载报错、上游崩溃栈都走这里，
  最多记 200 行），停读会卡住子进程，因此永远只丢弃日志不停读；
- 进程终止沿用 go-sdk 语义：Close 时关 stdin 优雅退出 → 限时 5s 等待 →
  SIGTERM → SIGKILL → Wait 回收。

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

参考 [eznix86/mcp-gateway](https://github.com/eznix86/mcp-gateway) 的 MiniSearch 实现，在 Go 中自实现轻量 BM25Okapi。评分/索引零外部库，分词层为打通中英检索引入纯 Go 的 [go-pinyin](https://github.com/mozillazg/go-pinyin)（无 cgo）做拼音桥。

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

// tokenize 分词: 小写 + 拉丁/数字按连续段一个 token + 汉字 run 三层展开。
// 建索引与查询两侧共用同一分词，保证「八字」(→ 拼音 bazi) 与英文目录 bazi-mcp
// 互相命中。汉字 run 的三层展开（以「八字排盘」为例）:
//   1. 单字 unigram: 八 字 排 盘        —— 单字查询仍可命中
//   2. 相邻二元 bigram: 八字 字排 排盘   —— 中文查询精度(优先命中词组而非散字)
//   3. 拼音形式(go-pinyin,无声调小写): 每字音节 ba/zi/pai/pan、二元连写
//      bazi/zipai/paipan、整段连写 bazipaipan(仅 ≤8 字 run) —— 打通中英鸿沟
func tokenize(text string) []string {
    text = strings.ToLower(text)
    // 按 run 扫描: 汉字 run 交给 hanRunTokens 三层展开,其余逻辑与旧版一致
    // (短 token 过滤: <2 字节且不含汉字的丢弃,单字母/数字被滤掉)
    ...
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
| 外部依赖 | 零 | 评分零依赖;分词用 go-pinyin(拼音桥) |
| 中文检索 | — | 单字+二元词组+拼音双向匹配 |

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

中文分词与拼音检索已在应用层解决（汉字 run 单字 + 二元词组 + go-pinyin 拼音桥，见 8.2 节 tokenize）；切换 FTS5 只为规模化，分词可继续走应用层预处理，或集成 [wangfenjin/simple](https://github.com/wangfenjin/simple) C 扩展（jieba 分词 + 拼音搜索）。索引大小预估：100K 文档约 25-50MB。

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
| `/mcp/ws`、`/smart/mcp/ws`、`/mcp/ws/group/{slug}` | WebSocket | 与 HTTP 路由对应 | 客户端网关请求、响应及订阅通知 |
| `/mcp/passive/` | WebSocket | 被动 WebSocket 接入 | 外部 MCP Server 连入，独立接入凭证认证 |

Smart 模式下的 `tools/list` 返回 5 个核心元工具；获准使用语义搜索时还可返回 `mcp.smart_search`。
Direct 模式下的 `tools/list` 返回聚合后的完整工具列表。
被动接入端点 `/mcp/passive/` 供外部 MCP Server 连入，NewMCP 作为 MCP Client 发现和调用工具。

三个 HTTP 端点均支持 POST 请求与 POST 订阅流；GET SSE 仅供携带 legacy session 的旧版客户端使用。现代协议用 `subscriptions/listen`，不使用 GET 或协议会话。WebSocket 是 NewMCP 的传输适配方式，通过文本帧传递 JSON-RPC，现代请求仍需 `_meta`，没有 HTTP POST 的镜像头要求。

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
- 上游未声明 resources/prompts 能力时直接跳过（不发多余请求）；目录变化可通过订阅得知，
  再重新请求相应列表。Smart 搜索使用的上游目录缓存由每会话后台任务合并刷新，通知转发与缓存落库之间可能存在短暂窗口；原生列表请求直接拉取上游。订阅只覆盖当前 API Key 与端点允许的范围。
- Smart 端点通过 `mcp.search` / `mcp.describe` / `mcp.read` 渐进访问资源与提示，
  不声明原生资源/提示列表能力，也不会在标准订阅 acknowledgment 中承诺这些过滤项。

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

### 10.5 正式版本与旧版兼容

网关支持 `2026-07-28`，同时保留 legacy `2025-11-25`、`2025-06-18`、`2025-03-26`、`2024-11-05`。现代请求可以直接调用，无需 `initialize` / `notifications/initialized`。`server/discover` 返回 `supportedVersions` 和当前端点的 `capabilities`，身份放在结果的 `_meta["io.modelcontextprotocol/serverInfo"]` 中。

每个现代请求必须包含 `params._meta` 下的 `io.modelcontextprotocol/protocolVersion` 和 `io.modelcontextprotocol/clientCapabilities`；后者可为 `{}`。`clientInfo` 建议包含 `{name,version}`。成功结果包含 `resultType: "complete"`。发现、目录列表和资源读取结果包含必填缓存提示 `ttlMs: 0`、`cacheScope: "private"`，客户端不能跨 API Key 共享这些响应。服务器版本来自构建时注入的 `common.Version`。

HTTP POST 的 `MCP-Protocol-Version`、`Mcp-Method` 必须分别匹配请求体协议版本、方法；`tools/call`、`prompts/get` 的 `Mcp-Name` 镜像 `params.name`，`resources/read` 镜像 `params.uri`。非 ASCII、控制字符、首尾空白或匹配编码标记的名称使用 `=?base64?<UTF-8 Base64>?=`。已有 `x-mcp-header` 工具参数注解时，对应参数还须镜像为 `Mcp-Param-*`。

| 情况 | HTTP 状态 | JSON-RPC 错误 |
|------|-----------|----------------|
| 缺少必需 `_meta` 字段或参数无效 | `400` | `-32602` Invalid params |
| 必需镜像头缺失、无效或与请求体不一致 | `400` | `-32020` HeaderMismatch |
| 请求版本不支持 | `400` | `-32022`，`data: {requested, supported}` |
| 缺少当前请求所需的客户端能力 | `400` | `-32021`，`data: {requiredCapabilities}` |
| 方法不存在 | `404` | `-32601` Method not found |

旧客户端继续使用 `initialize`，只协商 legacy 版本；请求现代日期也不会把 initialize 升为现代握手。HTTP initialize 响应通过 `Mcp-Session-Id` 返回旧会话句柄，后续 POST 与 GET SSE 应携带相同句柄。现代请求不创建会话。WebSocket 旧客户端仍可使用旧版握手。

规范参考：[正式版本](https://github.com/modelcontextprotocol/modelcontextprotocol/releases/tag/2026-07-28)、[版本兼容](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/versioning.mdx)、[Streamable HTTP](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/transports/streamable-http.mdx)。

---

## 11. 订阅与实验 Events

旧会话最长保留 30 分钟；过期后客户端需重新 initialize。浏览器 MCP Origin 默认允许配置的 `ServerAddress` 和当前回环地址；其他前端来源通过 `MCPAllowedOrigins`（逗号分隔的完整 Origin）配置。CORS 支持现代镜像头和 schema 声明的 `Mcp-Param-*` 预检。

Go SDK 锁定到官方修复提交 `53effc04ea258b9ee618886702e03abc4306a160`（`v1.8.1-0.20261001080146-53effc04ea25`），覆盖资源退订误删列表通知订阅的回归。使用旧版 Go SDK 的外部上游服务仍需自行升级服务端实现。

### 11.1 标准 `subscriptions/listen`

现代客户端向现有 MCP 端点 POST `subscriptions/listen`，响应为持续打开的 SSE 流。`params.notifications` 可选择 `toolsListChanged`、`resourcesListChanged`、`promptsListChanged` 和 `resourceSubscriptions`（网关资源 URI 数组）。

首条消息是 `notifications/subscriptions/acknowledged`，其中 `notifications` 为服务端实际承诺的过滤项子集。每条订阅通知的 `params._meta["io.modelcontextprotocol/subscriptionId"]` 等于原请求的 JSON-RPC `id`。Direct 模式可订阅授权范围的目录和资源；Smart 的标准目录是元工具列表，ack 不承诺原生资源/提示过滤项，也不会把上游工具变化当作元工具列表变化。始终检查 ack，不能假定请求的过滤项全部获准。

```text
data: {"jsonrpc":"2.0","method":"notifications/subscriptions/acknowledged","params":{"notifications":{"toolsListChanged":true},"_meta":{"io.modelcontextprotocol/subscriptionId":"listen-1"}}}

data: {"jsonrpc":"2.0","method":"notifications/tools/list_changed","params":{"_meta":{"io.modelcontextprotocol/subscriptionId":"listen-1"}}}

```

关闭 HTTP 流即取消，服务端释放相关订阅。WebSocket 客户端可发送 `notifications/cancelled`，`params.requestId` 使用原请求 ID；断开连接也会清理连接所属订阅。上游断线、服务配置、分组范围或端点模式发生变化时，服务端结束当前流，客户端需重新订阅以获取新的 ack；已停用资源的更新在每次转发前过滤。订阅建立总超时为 20 秒，每流最多连接 64 个上游。服务端主动结束标准流时发送 `resultType: "complete"` 的最终结果。核心订阅的 keepalive 可以使用 SSE 注释；`Last-Event-ID` 不用于恢复现代标准订阅。[正式订阅规范](https://github.com/modelcontextprotocol/modelcontextprotocol/blob/main/docs/specification/2026-07-28/basic/patterns/subscriptions.mdx)

### 11.2 NewMCP 实验 Events 范围

`events/*` 参考官方工作组的 [Draft design sketch](https://github.com/modelcontextprotocol/experimental-ext-triggers-events/blob/main/docs/design-sketch-proposal.md)。它尚未成为正式核心协议。NewMCP 用自定义能力 `capabilities.extensions["io.newmcp/events"] = {"experimental": true}` 标识此实现，后续草案变动可能要求迁移。

现代客户端调用 `events/*` 时，必须在每次请求的 `clientCapabilities.extensions` 中声明 `"io.newmcp/events": {}`，显式启用该实验扩展；未声明返回 `-32021`。

当前事件来自网关的授权目录和上游 MCP 变更通知：

| 事件名 | 含义 | `arguments` |
|--------|------|-------------|
| `mcp.tools.list_changed` | 授权工具目录变化 | 可选 `service`，省略为当前端点全部授权服务 |
| `mcp.resources.list_changed` | 授权资源目录变化 | 可选 `service` |
| `mcp.prompts.list_changed` | 授权提示词目录变化 | 可选 `service` |
| `mcp.resources.updated` | 指定授权资源更新 | 必填 `uri`，使用 `newmcp://service/upstream-uri`；可从 URI 推导 `service` |

Smart 客户端可通过实验 Events 订阅授权服务范围的变更，然后使用元工具重新发现或读取。API Key、端点分组、服务与条目启停共同约束订阅范围。当前实现不发现或转发任意上游 `events/*` 业务事件，例如 Slack 新消息或 GitHub PR 事件；这些需要单独增加事件源。

| 方法 | 用途 |
|------|------|
| `events/list` | 获取当前四种事件描述、参数 Schema、载荷 Schema 和 delivery 模式 |
| `events/poll` | 按事件与参数轮询；返回 `events`、`cursor`、`truncated`、`hasMore`、`nextPollMs` |
| `events/stream` | 每个请求一条推送订阅；HTTP SSE 或 WebSocket 通知 |
| `events/subscribe` | 注册或刷新 webhook 订阅 |
| `events/unsubscribe` | 按原订阅键取消 webhook |

Poll 和 push 无需先调用 `events/subscribe`。Push 用 `notifications/events/active` 确认订阅，随后发送 `notifications/events/event` 和带 cursor 的 `notifications/events/heartbeat`，取消方式与连接传输一致。事件包含 `{eventId,name,timestamp,data,cursor}`；客户端按 `eventId` 去重，并保存最近 cursor 用于后续 poll、重连或刷新。

回放仅限进程内有界历史，受容量、保存时间及 `maxAgeMs` 限制；无法恢复的区间以 `truncated: true` 或 `gap` 通知表示。`cursor: null` 或省略 cursor 表示从现在开始；重启后旧历史不可恢复，客户端需重新建立订阅。它不是持久事件队列。

### 11.3 Webhook 生命周期与验签

Webhook callback 必须是公网 HTTPS URL。服务端在发送时重新校验地址、限制非公网目标并禁止跳转；订阅 API 使用当前 API Key 身份。初次激活前发送带签名的 `{type: "verification", challenge: "<nonce>"}`，接收端验证签名后用 `2xx` JSON `{challenge: "<same nonce>"}` 回应。

客户端提供 `delivery.secret`，格式为 `whsec_` 加 24–64 字节随机值的 Base64。接收端提前保存 secret，以便验证首条 challenge。每条 POST 的头包含 `webhook-id`、`webhook-timestamp`（Unix 秒）、`webhook-signature`、`X-MCP-Subscription-Id`：

```text
key = Base64Decode(secret 去掉 whsec_ 前缀)
signedBytes = UTF8(webhook-id + "." + webhook-timestamp + ".") + 原始 HTTP body bytes
webhook-signature = "v1," + Base64(HMAC-SHA256(key, signedBytes))
```

在解析或处理载荷前验签，不要用重新序列化的 JSON 计算签名；检查五分钟时间窗口并按 `webhook-id` / `eventId` 去重。接收端接受并保存或转发事件后再返回 `2xx`。失败投递使用有限次数与时间窗口的退避重试；`410` 与 `413` 不重试该条事件。

本实现的 webhook 订阅为内存状态，授予的有限 TTL 不超过五分钟，不提供无期限订阅。请求 `ttlMs` 是建议值，响应 `refreshBefore` 为实际到期时间；客户端应在其前重复同一 `events/subscribe` 续期，停止续期后过期。进程重启后需重新订阅，进程内回放不能弥补重启期间的事件。

唯一订阅键为 `(API Key 身份, delivery.url, name, canonical JSON arguments)`。相同键的 subscribe 刷新 TTL 和可变字段；活跃订阅沿用服务端已确认的投递 cursor，不按刷新请求的旧 cursor 回退在途批次。已过期或重启后重建时，才按请求的 cursor 和 maxAgeMs 尝试回放；返回 `id` 为服务端生成的路由句柄。取消请求使用原来的 `name`、`arguments`、`delivery.url`，不能只提交返回 ID。

控制载荷也使用相同签名：`verification` 用于首次验证；`{type:"gap",cursor:...}` 表示回放缺口；`{type:"terminated",error:...}` 表示订阅已结束。控制消息的 `webhook-id` 为 `msg_<type>_<random>`。

### 11.4 完整 curl 示例

以下示例使用 Bash、curl、jq；示例密钥通过环境变量提供。选择 `/mcp`、`/smart/mcp` 或 `/mcp/group/<slug>`，后续调用使用同一端点。资源 URI 与服务名应从当前授权目录取得。

```bash
export MCP_URL='http://localhost:3000/mcp'
export MCP_API_KEY='<你的 API Key>'

# 自动添加现代请求元数据和 HTTP 镜像头；第三个参数为需要镜像的名称/URI。
mcp_rpc() {
  local mcp_method="$1" mcp_params="$2"
  local -a mcp_name_header=()
  if [ -n "${3:-}" ]; then
    mcp_name_header=(-H "Mcp-Name: $3")
  fi
  curl --no-buffer --silent --show-error "$MCP_URL" \
    -H "X-API-Key: $MCP_API_KEY" \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    -H 'MCP-Protocol-Version: 2026-07-28' \
    -H "Mcp-Method: $mcp_method" \
    "${mcp_name_header[@]}" \
    --data "$(jq -cn --arg method "$mcp_method" --argjson params "$mcp_params" '
      {jsonrpc:"2.0",id:1,method:$method,params:($params + {
        _meta:{
          "io.modelcontextprotocol/protocolVersion":"2026-07-28",
          "io.modelcontextprotocol/clientCapabilities":{extensions:{"io.newmcp/events":{}}},
          "io.modelcontextprotocol/clientInfo":{name:"curl-example",version:"1"}
        }
      })}')"
}

mcp_rpc server/discover '{}'
mcp_rpc tools/list '{}'
```

打开标准订阅后，curl 持续输出 SSE；按 Ctrl+C 关闭并取消：

```bash
mcp_rpc subscriptions/listen \
  '{"notifications":{"toolsListChanged":true,"resourcesListChanged":true,"promptsListChanged":true}}'
```

列举、轮询和推送实验事件。保存 poll 响应的 `.result.cursor`，下次 poll 带回；push 重连时带最近收到的 cursor：

```bash
mcp_rpc events/list '{}'
mcp_rpc events/poll \
  '{"name":"mcp.tools.list_changed","arguments":{},"cursor":null,"maxAgeMs":300000,"maxEvents":50}'

export MCP_EVENT_CURSOR='<上次收到的 cursor>'
mcp_rpc events/poll "$(jq -cn --arg cursor "$MCP_EVENT_CURSOR" \
  '{name:"mcp.tools.list_changed",arguments:{},cursor:$cursor,maxAgeMs:300000,maxEvents:50}')"

# 长连接，按 Ctrl+C 取消。
mcp_rpc events/stream \
  '{"name":"mcp.tools.list_changed","arguments":{},"cursor":null,"maxAgeMs":300000}'

# 仅关注目录中指定资源；该URI需在当前端点的授权范围内。
export MCP_RESOURCE_URI='<newmcp://服务名/上游资源URI>'
mcp_rpc events/poll "$(jq -cn --arg uri "$MCP_RESOURCE_URI" \
  '{name:"mcp.resources.updated",arguments:{uri:$uri},cursor:null}')"
```

Webhook 接收端应先准备好 HTTPS callback、secret 存储、验签和 challenge 回应。生成本地随机 secret，不把它写入共享文件或日志：

```bash
export MCP_CALLBACK_URL='https://receiver.example.com/hooks/newmcp'
export MCP_WEBHOOK_SECRET="whsec_$(openssl rand -base64 32)"

mcp_rpc events/subscribe "$(jq -cn \
  --arg url "$MCP_CALLBACK_URL" --arg secret "$MCP_WEBHOOK_SECRET" '
  {name:"mcp.tools.list_changed",arguments:{},
   delivery:{mode:"webhook",url:$url,secret:$secret},
   cursor:null,ttlMs:300000,maxAgeMs:300000}')"

# 在响应 refreshBefore 到期前重复 subscribe；如已收到 cursor，在刷新时带回。
mcp_rpc events/subscribe "$(jq -cn \
  --arg url "$MCP_CALLBACK_URL" --arg secret "$MCP_WEBHOOK_SECRET" \
  --arg cursor "$MCP_EVENT_CURSOR" '
  {name:"mcp.tools.list_changed",arguments:{},
   delivery:{mode:"webhook",url:$url,secret:$secret},
   cursor:$cursor,ttlMs:300000,maxAgeMs:300000}')"

mcp_rpc events/unsubscribe "$(jq -cn --arg url "$MCP_CALLBACK_URL" \
  '{name:"mcp.tools.list_changed",arguments:{},delivery:{url:$url}}')"
```

旧客户端先初始化，捕获响应头中的 session，再通过 GET 订阅旧版通知流。此示例的 API Key 和端点与上文相同：

```bash
MCP_LEGACY_HEADERS=$(mktemp)
curl --silent --show-error -D "$MCP_LEGACY_HEADERS" "$MCP_URL" \
  -H "X-API-Key: $MCP_API_KEY" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2025-11-25' \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-curl-example","version":"1"}}}'

MCP_SESSION_ID=$(awk 'tolower($1)=="mcp-session-id:" {gsub("\r", "", $2); print $2}' "$MCP_LEGACY_HEADERS")
rm -f "$MCP_LEGACY_HEADERS"

curl --silent --show-error "$MCP_URL" \
  -H "X-API-Key: $MCP_API_KEY" \
  -H "Mcp-Session-Id: $MCP_SESSION_ID" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2025-11-25' \
  --data '{"jsonrpc":"2.0","method":"notifications/initialized"}'

curl --no-buffer --silent --show-error "$MCP_URL" \
  -H "X-API-Key: $MCP_API_KEY" \
  -H "Mcp-Session-Id: $MCP_SESSION_ID" \
  -H 'Accept: text/event-stream' \
  -H 'MCP-Protocol-Version: 2025-11-25'
```
