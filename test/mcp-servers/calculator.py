"""
最小 MCP 服务 - 计算器
测试目标: 验证 agent 可发现并调用此工具
"""
from mcp.server.fastmcp import FastMCP

mcp = FastMCP("calculator")


@mcp.tool()
def add(a: float, b: float) -> str:
    """两数相加。输入两个数字，返回它们的和。"""
    return f"{a} + {b} = {a + b}"


@mcp.tool()
def multiply(a: float, b: float) -> str:
    """两数相乘。输入两个数字，返回它们的积。"""
    return f"{a} × {b} = {a * b}"


@mcp.tool()
def power(base: float, exponent: float) -> str:
    """计算幂运算。输入底数和指数，返回 base^exponent 的结果。"""
    return f"{base}^{exponent} = {base ** exponent}"


if __name__ == "__main__":
    mcp.run(transport="stdio")
