# NewMCP Agent 测试指南

测试大模型 agent 是否能正确通过 MCP 搜索和调用工具。

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

agent 只看到 3 个元工具 (mcp_search, mcp_describe, mcp_execute)，需要通过搜索发现工具再调用。

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
- [ ] 3 个元工具正确注册到小智云
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
