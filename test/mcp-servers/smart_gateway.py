"""
NewMCP Smart 模式网关 (最小测试实例)

模拟 NewMCP 的 Smart 模式行为:
- 暴露 3 个元工具: mcp.search, mcp.describe, mcp.execute
- 聚合下游 MCP 服务的工具 (本地 + HTTP)
- 通过 search → describe → execute 渐进发现

测试目标: 验证 LLM agent 能否通过元工具正确搜索和调用工具
"""
import json
import os
import subprocess
import sys

import httpx
from dotenv import load_dotenv
from mcp.client.session import ClientSession
from mcp.client.streamable_http import streamablehttp_client
from mcp.server.fastmcp import FastMCP

load_dotenv(os.path.join(os.path.dirname(__file__), "..", ".env"))

mcp = FastMCP("newmcp-smart-gateway")

# ── 注册的下游 MCP 服务目录 (模拟 NewMCP 注册表) ──

REGISTRY = {
    "calculator": {
        "display_name": "数学计算器",
        "description": "基础数学运算工具，支持加减乘除和幂运算",
        "command": "python",
        "args": ["test/mcp-servers/calculator.py"],
        "tools": [
            {"name": "add", "description": "两数相加。输入两个数字，返回它们的和。"},
            {"name": "multiply", "description": "两数相乘。输入两个数字，返回它们的积。"},
            {"name": "power", "description": "计算幂运算。输入底数和指数，返回 base^exponent 的结果。"},
        ],
    },
    "weather": {
        "display_name": "天气查询",
        "description": "查询城市天气信息，支持温度、天气状况和湿度",
        "command": "python",
        "args": ["test/mcp-servers/weather.py"],
        "tools": [
            {"name": "get_weather", "description": "查询指定城市的当前天气信息。输入城市名，返回温度、天气状况和湿度。"},
            {"name": "list_cities", "description": "列出所有可查询天气的城市。"},
        ],
    },
    "exa": {
        "display_name": "Exa 网络搜索",
        "description": "Exa 网络搜索引擎，支持关键词搜索和语义搜索，可获取网页内容",
        "transport": "streamable-http",
        "url": "https://mcp.exa.ai/mcp",
        "tools": [
            {"name": "web_search_exa", "description": "Search the web for any topic and get clean, ready-to-use content. Best for finding current information, news, facts. Query tips: describe the ideal page, not keywords."},
            {"name": "web_fetch_exa", "description": "Read a webpage's full content as clean markdown. Best for extracting full content from known URLs. Batch multiple URLs in one call."},
        ],
    },
}


# ── 简易 BM25 搜索 ──

def _tokenize(text: str) -> list[str]:
    """支持中英文混合分词: 英文按空格拆分，中文按单字拆分"""
    text = text.lower()
    tokens = []
    buf = ""
    for ch in text:
        if '一' <= ch <= '鿿':
            if buf:
                tokens.append(buf)
                buf = ""
            tokens.append(ch)
        elif ch.isalnum():
            buf += ch
        else:
            if buf:
                tokens.append(buf)
                buf = ""
    if buf:
        tokens.append(buf)
    return tokens


def _bm25_search(query: str, docs: list[dict], limit: int = 10) -> list[dict]:
    """极简 BM25 搜索，用于验证 smart 模式搜索是否可用"""
    q_tokens = set(_tokenize(query))
    scored = []
    for doc in docs:
        text = f"{doc.get('name', '')} {doc.get('description', '')} {doc.get('server_name', '')}"
        d_tokens = set(_tokenize(text))
        overlap = len(q_tokens & d_tokens)
        if overlap > 0:
            scored.append((overlap, doc))
    scored.sort(key=lambda x: -x[0])
    return [d for _, d in scored[:limit]]


# ── 元工具实现 ──

@mcp.tool()
def mcp_search(query: str = "", scope: str = "all", limit: int = 10) -> str:
    """搜索可用的 MCP 服务和工具。支持按关键字搜索服务名或工具名。query 为空时返回所有可用项。
    参数:
      query: 搜索关键字，为空则返回所有
      scope: 搜索范围 "mcp"(服务) "tool"(工具) "all"(全部)
      limit: 最大返回数量
    """
    docs = []
    for svc_name, svc in REGISTRY.items():
        if scope in ("mcp", "all"):
            docs.append({
                "type": "mcp",
                "name": svc_name,
                "description": svc["description"],
                "server_name": svc_name,
                "tool_count": len(svc["tools"]),
            })
        if scope in ("tool", "all"):
            for tool in svc["tools"]:
                docs.append({
                    "type": "tool",
                    "name": f"{svc_name}.{tool['name']}",
                    "description": tool["description"],
                    "server_name": svc_name,
                })

    if query:
        results = _bm25_search(query, docs, limit)
    else:
        results = docs[:limit]

    if not results:
        return f"未找到与 '{query}' 匹配的结果" if query else "当前没有可用的服务或工具"

    lines = [f"找到 {len(results)} 个结果:\n"]
    for i, r in enumerate(results, 1):
        if r["type"] == "mcp":
            lines.append(f"{i}. **{r['name']}** (服务)")
            lines.append(f"   {r['description']}")
            lines.append(f"   工具数: {r['tool_count']}\n")
        else:
            lines.append(f"{i}. **{r['name']}** (工具, 属于 {r['server_name']})")
            lines.append(f"   {r['description']}\n")
    return "\n".join(lines)


@mcp.tool()
def mcp_describe(targets: list[str]) -> str:
    """查看指定 MCP 服务的工具列表，或指定工具的详细信息。
    参数:
      targets: 服务名列表或 "服务名.工具名" 形式的标识符
    """
    lines = []
    for target in targets:
        if "." in target:
            svc_name, tool_name = target.split(".", 1)
            svc = REGISTRY.get(svc_name)
            if not svc:
                lines.append(f"服务 '{svc_name}' 不存在\n")
                continue
            tool = next((t for t in svc["tools"] if t["name"] == tool_name), None)
            if not tool:
                lines.append(f"工具 '{target}' 不存在\n")
                continue
            lines.append(f"## {target}")
            lines.append(f"{tool['description']}")
            lines.append(f"所属服务: {svc_name} ({svc['display_name']})\n")
        else:
            svc = REGISTRY.get(target)
            if not svc:
                lines.append(f"服务 '{target}' 不存在\n")
                continue
            lines.append(f"## {target} ({svc['display_name']})")
            lines.append(f"{svc['description']}")
            lines.append(f"工具数: {len(svc['tools'])}\n")
            for t in svc["tools"]:
                lines.append(f"  - {t['name']}: {t['description']}")
            lines.append("")
    return "\n".join(lines)


@mcp.tool()
async def mcp_execute(tool_id: str, arguments: str = "{}") -> str:
    """执行指定的 MCP 工具。
    参数:
      tool_id: 格式 "服务名.工具名"，如 "calculator.add"
      arguments: JSON 格式的工具参数，如 '{"a": 3, "b": 5}'
    """
    if "." not in tool_id:
        return f"错误: tool_id 格式应为 '服务名.工具名'，收到 '{tool_id}'"

    svc_name, tool_name = tool_id.split(".", 1)
    svc = REGISTRY.get(svc_name)
    if not svc:
        return f"错误: 服务 '{svc_name}' 不存在"

    tool = next((t for t in svc["tools"] if t["name"] == tool_name), None)
    if not tool:
        return f"错误: 工具 '{tool_id}' 不存在"

    try:
        args = json.loads(arguments) if isinstance(arguments, str) else arguments
    except json.JSONDecodeError:
        return f"错误: arguments 不是有效的 JSON"

    transport = svc.get("transport", "local")
    if transport == "local":
        return _execute_local(svc_name, tool_name, args)
    elif transport == "streamable-http":
        return await _execute_http(svc_name, tool_name, args)
    else:
        return f"错误: 不支持的传输类型 '{transport}'"


# ── HTTP 执行 (Streamable HTTP MCP 上游) ──

MAX_RESPONSE_CHARS = 8000

async def _execute_http(svc_name: str, tool_name: str, args: dict) -> str:
    """通过 Streamable HTTP 连接上游 MCP 服务并执行工具"""
    svc = REGISTRY.get(svc_name)
    url = svc.get("url", "")

    headers = {}
    api_key = os.environ.get("EXA_API_KEY", "") if svc_name == "exa" else ""
    if api_key:
        headers["x-api-key"] = api_key

    # Exa 搜索限制返回数据量，避免超出 WSS 消息限制
    if svc_name == "exa":
        if tool_name == "web_search_exa":
            args.setdefault("numResults", 3)
        if tool_name in ("web_search_exa", "web_fetch_exa"):
            args.setdefault("maxCharacters", 3000)

    max_retries = 2
    for attempt in range(max_retries + 1):
        try:
            async with streamablehttp_client(url=url, headers=headers or None) as (
                read_stream,
                write_stream,
                _,
            ):
                async with ClientSession(read_stream, write_stream) as session:
                    await session.initialize()
                    result = await session.call_tool(tool_name, args)

                    if result.isError:
                        err_text = str(result.content)
                        if len(err_text) > 500:
                            err_text = err_text[:500] + "...(已截断)"
                        return f"上游错误: {err_text}"

                    text_parts = []
                    for item in result.content:
                        if hasattr(item, "text"):
                            text_parts.append(item.text)
                    text = "\n".join(text_parts) if text_parts else str(result.content)

                    if len(text) > MAX_RESPONSE_CHARS:
                        text = text[:MAX_RESPONSE_CHARS] + f"\n\n...(结果已截断，共 {len(text)} 字符)"
                    return text
        except Exception as e:
            if attempt < max_retries:
                import asyncio as _asyncio
                await _asyncio.sleep(1)
                continue
            return f"错误: 连接上游 MCP '{svc_name}' 失败 - {type(e).__name__}: {e}"


# ── 本地执行 (模拟上游调用) ──

def _execute_local(svc_name: str, tool_name: str, args: dict) -> str:
    """通过启动子进程调用上游 MCP 服务 (简化版，直接调用函数)"""
    # calculator 服务
    if svc_name == "calculator":
        if tool_name == "add":
            a, b = args.get("a", 0), args.get("b", 0)
            return f"{a} + {b} = {a + b}"
        elif tool_name == "multiply":
            a, b = args.get("a", 0), args.get("b", 0)
            return f"{a} × {b} = {a * b}"
        elif tool_name == "power":
            base, exp = args.get("base", 0), args.get("exponent", 0)
            return f"{base}^{exp} = {base ** exp}"
    # weather 服务
    elif svc_name == "weather":
        MOCK = {
            "北京": {"temp": 22, "condition": "晴", "humidity": 45},
            "上海": {"temp": 26, "condition": "多云", "humidity": 70},
            "深圳": {"temp": 30, "condition": "阵雨", "humidity": 85},
        }
        if tool_name == "get_weather":
            city = args.get("city", "")
            data = MOCK.get(city)
            if data:
                return f"{city}: {data['condition']}，温度 {data['temp']}°C，湿度 {data['humidity']}%"
            return f"{city}: 暂无天气数据"
        elif tool_name == "list_cities":
            return "支持的城市: " + "、".join(MOCK.keys())

    return f"错误: 无法执行 {svc_name}.{tool_name}"


if __name__ == "__main__":
    mcp.run(transport="stdio")
