"""
最小 MCP 服务 - 天气查询 (模拟)
测试目标: 验证 agent 可通过 smart 模式搜索发现并调用此工具
"""
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("weather")

# 模拟天气数据
MOCK_DATA = {
    "北京": {"temp": 22, "condition": "晴", "humidity": 45},
    "上海": {"temp": 26, "condition": "多云", "humidity": 70},
    "深圳": {"temp": 30, "condition": "阵雨", "humidity": 85},
}


@mcp.tool()
def get_weather(city: str) -> str:
    """查询指定城市的当前天气信息。输入城市名，返回温度、天气状况和湿度。"""
    data = MOCK_DATA.get(city)
    if data:
        return f"{city}: {data['condition']}，温度 {data['temp']}°C，湿度 {data['humidity']}%"
    return f"{city}: 暂无天气数据"


@mcp.tool()
def list_cities() -> str:
    """列出所有可查询天气的城市。"""
    return "支持的城市: " + "、".join(MOCK_DATA.keys())


if __name__ == "__main__":
    mcp.run(transport="stdio")
