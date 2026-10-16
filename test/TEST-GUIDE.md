# NewMCP Agent 测试指南

测试大模型 agent 是否能正确通过 MCP 搜索和调用工具。

内置视频生成的自动检查、Agent 流程和真实网关测试见下文[视频生成测试](#视频生成测试)；历史结果见[视频验收记录](../docs/VIDEO-GENERATION-ACCEPTANCE.md)。

## 测试模式

### 模式 1: Direct 模式 (工具直接暴露)

**配置文件**: `mcp-config-direct.json`

将 calculator 和 weather 两个 MCP 服务直接暴露给 agent，agent 可直接看到所有工具。

**使用方法**: 将 `mcp-config-direct.json` 内容复制到 Claude Code 的 MCP 配置中:
```bash
# 方式 1: 项目级配置
cp mcp-config-direct.json .claude/settings.json  # 合并到 mcpServers 字段

# 方式 2: 手动添加到 ~/.claude/settings.json 的 mcpServers 中
```

**测试用例**:

| # | 测试提示 | 期望行为 |
|---|---------|---------|
| 1 | "帮我算一下 3 加 5" | 调用 calculator.add(a=3, b=5) → "3 + 5 = 8" |
| 2 | "2 的 10 次方是多少" | 调用 calculator.power(base=2, exponent=10) → "1024" |
| 3 | "北京今天天气怎么样" | 调用 weather.get_weather(city="北京") → "晴, 22°C" |
| 4 | "有哪些城市可以查天气" | 调用 weather.list_cities() → "北京、上海、深圳" |

**验证点**:
- [ ] agent 能看到所有工具 (add, multiply, power, get_weather, list_cities)
- [ ] 参数传递正确 (数字型参数不传成字符串)
- [ ] 返回结果被正确理解和使用

---

### 模式 2: Smart 模式 (元工具搜索调用)

**配置文件**: `mcp-config-smart.json`

agent 只看到 5 个元工具 (mcp_search, mcp_describe, mcp_execute, mcp_execute_batch, mcp_read)，需要通过搜索发现工具再调用。

**测试用例**:

| # | 测试提示 | 期望调用链 |
|---|---------|-----------|
| 1 | "帮我算一下 3 加 5" | mcp_search("计算") → mcp_describe(["calculator"]) → mcp_execute("calculator.add", {"a":3,"b":5}) |
| 2 | "北京天气怎么样" | mcp_search("天气") → mcp_execute("weather.get_weather", {"city":"北京"}) |
| 3 | "有哪些计算工具可以用" | mcp_search("计算", scope="tool") |
| 4 | "2 的 10 次方" | mcp_execute("calculator.power", {"base":2,"exponent":10}) |

**验证点**:
- [ ] agent 知道先搜索，而不是猜测工具名
- [ ] mcp_execute 的 tool_id 格式正确 ("服务名.工具名")
- [ ] arguments 为有效 JSON 字符串
- [ ] agent 能理解搜索结果并选择正确的工具

---

## 快速开始

```bash
# 安装 MCP Python SDK
pip install mcp

# 验证 MCP 服务可启动
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | python test/mcp-servers/calculator.py

# 在 Claude Code 中测试 Direct 模式
# 将 mcp-config-direct.json 内容添加到 MCP 配置

# 在 Claude Code 中测试 Smart 模式
# 将 mcp-config-smart.json 内容添加到 MCP 配置
```

## Claude Code 配置方法

编辑 `~/.claude/settings.json` 或项目 `.claude/settings.json`，在 `mcpServers` 中添加配置:

```json
{
  "mcpServers": {
    "calculator": {
      "type": "stdio",
      "command": "python",
      "args": ["test/mcp-servers/calculator.py"]
    }
  }
}
```

配置完成后重启 Claude Code，agent 将自动发现 MCP 工具。

---

### 模式 3: XiaoZhi Cloud (WSS 桥接)

通过 WSS 端点将 Smart Gateway 注册到小智云，用小智设备测试。

**启动方式**:

```bash
# 1. 安装依赖
test/.venv/bin/pip install websockets python-dotenv python-socks[asyncio]

# 2. 配置 WSS 端点 (编辑 test/.env)
# XIAOZHI_WSS_URL=wss://api.xiaozhi.me/mcp/?token=YOUR_TOKEN

# 3. 启动连接器
test/.venv/bin/python test/mcp-servers/xiaozhi_connector.py
```

**工作原理**:

```
小智设备 → 小智云(WSS) → xiaozhi_connector.py → smart_gateway.py (子进程)
                                         ↑ stdio↔WSS 双向桥接
```

connector 启动 `smart_gateway.py` 作为子进程，将小智云 WSS 的 JSON-RPC 消息管道对接到子进程的 stdin/stdout。MCP 协议由 FastMCP 处理，connector 只做传输层桥接 + 自动重连。

**测试用例**:

| # | 设备端提示 | 期望行为 |
|---|----------|---------|
| 1 | "帮我算一下 3 加 5" | mcp_search → mcp_execute("calculator.add", {"a":3,"b":5}) → "3 + 5 = 8" |
| 2 | "北京天气怎么样" | mcp_search("天气") → mcp_execute("weather.get_weather", {"city":"北京"}) |
| 3 | "有什么工具可以用" | mcp_search("工具") → 列出所有服务和工具 |

**验证点**:
- [ ] 连接器成功连接并保持长连接
- [ ] 元工具正确注册到小智云(含 mcp_execute_batch)
- [ ] 小智设备能通过搜索发现工具
- [ ] 工具调用参数传递正确，结果正确返回
- [ ] 断线自动重连正常工作

---

### 模式 4: 外部 HTTP MCP (Exa 搜索引擎)

Smart Gateway 已集成 Exa 网络搜索 MCP 服务 (`https://mcp.exa.ai/mcp`)，支持通过元工具搜索和调用。

**配置**: 编辑 `test/.env` 设置 API Key（可选，留空使用免费版）:
```
EXA_API_KEY=your_key_here
```

**可调用工具**:

| 工具 ID | 说明 |
|--------|------|
| `exa.web_search_exa` | 网页搜索，返回干净的结果 |
| `exa.web_fetch_exa` | 读取网页内容，返回 Markdown |

**测试用例**:

| # | 测试提示 | 期望调用链 |
|---|---------|-----------|
| 1 | "搜索一下最新的 AI 新闻" | mcp_search("搜索") → mcp_execute("exa.web_search_exa", {"query":"latest AI news"}) |
| 2 | "帮我读一下某个网页的内容" | mcp_search("fetch") → mcp_execute("exa.web_fetch_exa", {"urls":["https://example.com"]}) |
| 3 | "有什么搜索工具" | mcp_search("search", scope="tool") |
| 4 | "3加5等于多少，再搜一下MCP协议" | mcp_execute("calculator.add",...) + mcp_execute("exa.web_search_exa",...) |

**验证点**:
- [ ] mcp_search("搜索") 能找到 exa 服务
- [ ] HTTP 上游连接成功（initialize → call_tool → 结果返回）
- [ ] 免费版和 API Key 版均可工作
- [ ] 本地服务 (calculator/weather) 不受影响
- [ ] 通过小智设备也能调用 Exa 工具

## 视频生成测试

先在独立测试环境创建视频配置，测试后注册 `videogen_<id>`，加入分组并绑定测试 API Key。使用模拟网关运行应用流程；真实网关生成需显式提供授权凭证，会产生上游费用。配置、请求格式及参数见[视频生成 MCP](../docs/VIDEO-GENERATION.md)。

| 场景 | 期望行为 |
| --- | --- |
| 提示词生成视频 | Direct 调用 generate_video；Smart 先搜索、查看 schema，再执行生成工具 |
| 本地单张参考图生成视频 | 调用 upload_image，执行返回的上传命令，再用 image_url 调用 generate_video_from_image |
| 生成工具短等待结束 | 识别正常任务结果，使用本地 task_id 查询状态，不重复调用生成 |
| 状态为 completed，保存尚未 ready | 继续查询；只有 videos 链接就绪才提供下载 |
| 保存失败 | 对原任务设置 retry_download=true，不重新生成 |
| 同输入重复 request_id | 返回同一任务；改变输入应被拒绝 |
| submission_unknown | 告知提交结果不明确，不自动重提 |
| 提交 HTTP 503 且 code=video_queue_full | 生成失败，允许手动重试更新原任务；原请求不自动重提 |
| 多次重试或两个实例同时重试 | task_id 和历史数量不变；同一次重试只提交一次，进行中拒绝新的重试，重启后仍去重 |
| New API 将队列拒绝 JSON 包装在 message/error.message/error 中 | 识别内层明确拒绝；首次生成和重试再次被拒绝均为 failed，同一次重试重复发送不新增任务 |
| 查询已有任务时返回临时 503 | 保留上游 ID 和最近生成状态，延后查询，不重新生成 |
| 旧版误标的完整队列拒绝记录 | 重启后修正为失败；已有上游 ID、租约持有者或不完整错误记录不变 |
| 页面参数框留空，另一个填写 `{"extra_body":{"mode":"text"}}` | 空参数按 `{}` 提交，合法扩展参数保留，不出现 `Unexpected end of JSON input` |
| 页面 JSON 不完整、数组或标量 | 提示对应输入框错误，阻止配置保存、测试或生成重试 |
| 分组禁用生成工具或跨用户/配置查询 | 请求拒绝，不能绕过工具权限或使用任意上游 ID |

有依赖关系的上传、生成和状态查询分步执行，不放进同一次 execute_batch。图生 JSON 的图片链接应能被上游访问；Wan、豆包使用 JSON。

从项目根目录运行：

```powershell
go test ./internal/mcp/videogen ./internal/videoasset ./internal/storage ./model ./router -count=1
go test ./internal/mcp/videogen -run TestNewAPIGatewayPluginCompatibility -v
```

插件解码检查需要 Node 和 `reference/new-api-main`。MySQL/PostgreSQL 检查另设 `VIDEOGEN_TEST_MYSQL_DSN`、`VIDEOGEN_TEST_POSTGRES_DSN` 指向专用空测试库，测试会创建和删除表；不设置时跳过。

前端安装依赖后，在 `web` 目录运行 `npm run test:videogen-params` 检查参数解析回归，运行 `npm run build:check` 完成类型检查和构建。该参数测试不调用网关或生成视频。

真实生成另设 `VIDEOGEN_SMOKE_ENDPOINT`、`VIDEOGEN_SMOKE_API_KEY`、`VIDEOGEN_SMOKE_TEXT_MODEL`、`VIDEOGEN_SMOKE_IMAGE_MODEL`，运行 `go test ./internal/mcp/videogen -run TestLiveGatewayVideoSmoke -v -timeout 35m`。该测试使用临时数据库，提交一次文生和一次图生；JSON 格式另需 `VIDEOGEN_SMOKE_IMAGE_URL`。可选时长、尺寸和格式详见[验证说明](../docs/VIDEO-GENERATION.md#验证)。缺少环境变量时跳过；记录实际模型和输出结果后才标记真实验收完成。
