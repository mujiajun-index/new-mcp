"""
NewMCP XiaoZhi Cloud Connector (stdio <-> WSS pipe)

桥接模式: 启动 smart_gateway.py 子进程 (stdio)，
将 stdin/stdout 双向管道对接到小智云 WSS 端点。

MCP 协议由 smart_gateway.py 的 FastMCP 处理，
本脚本只负责传输层桥接 + 自动重连。

用法:
  python test/mcp-servers/xiaozhi_connector.py

依赖: websockets, python-dotenv
"""

import asyncio
import os
import signal
import subprocess
import sys

import websockets
from dotenv import load_dotenv

load_dotenv(os.path.join(os.path.dirname(__file__), "..", ".env"))

WSS_URL = os.environ.get("XIAOZHI_WSS_URL", "")

# smart_gateway.py 子进程命令
PYTHON = sys.executable
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
GATEWAY_SCRIPT = os.path.join(SCRIPT_DIR, "smart_gateway.py")

INITIAL_BACKOFF = 1
MAX_BACKOFF = 600


async def pipe_ws_to_proc(ws, proc):
    """WebSocket → 子进程 stdin"""
    try:
        async for msg in ws:
            text = msg if isinstance(msg, str) else msg.decode("utf-8")
            print(f"[WSS→STDIN] {text[:120]}...")
            proc.stdin.write(text + "\n")
            proc.stdin.flush()
    except Exception as e:
        print(f"[WSS→STDIN 错误] {e}")
        raise
    finally:
        if not proc.stdin.closed:
            proc.stdin.close()


async def pipe_proc_to_ws(proc, ws):
    """子进程 stdout → WebSocket (自动截断超大消息)"""
    MAX_MSG_SIZE = 50000  # 小智云 WSS 消息大小限制
    try:
        while True:
            data = await asyncio.to_thread(proc.stdout.readline)
            if not data:
                print("[STDOUT] 子进程输出结束")
                break
            # 截断超大响应避免 1009 message too big
            if len(data) > MAX_MSG_SIZE:
                # 尝试截断 JSON-RPC result 中的 text 字段
                try:
                    import json
                    msg = json.loads(data)
                    for item in msg.get("result", {}).get("content", []):
                        if isinstance(item, dict) and len(item.get("text", "")) > MAX_MSG_SIZE // 2:
                            item["text"] = item["text"][:MAX_MSG_SIZE // 2] + "\n\n...(结果已截断)"
                    data = json.dumps(msg, ensure_ascii=False)
                except (json.JSONDecodeError, KeyError):
                    data = data[:MAX_MSG_SIZE]
                print(f"[STDOUT→WSS] 截断: {len(data)} chars")
            else:
                print(f"[STDOUT→WSS] {data[:120]}...")
            await ws.send(data)
    except Exception as e:
        print(f"[STDOUT→WSS 错误] {e}")
        raise


async def pipe_proc_stderr(proc):
    """子进程 stderr → 本地终端"""
    try:
        while True:
            data = await asyncio.to_thread(proc.stderr.readline)
            if not data:
                break
            sys.stderr.write(data)
            sys.stderr.flush()
    except Exception:
        pass


async def run():
    if not WSS_URL:
        print("错误: 未设置 XIAOZHI_WSS_URL")
        print("请在 test/.env 中设置")
        sys.exit(1)

    print(f"[连接] WSS: {WSS_URL[:60]}...")
    backoff = INITIAL_BACKOFF

    while True:
        proc = None
        try:
            async with websockets.connect(WSS_URL) as ws:
                print("[连接] 已连接到小智云")
                backoff = INITIAL_BACKOFF

                proc = subprocess.Popen(
                    [PYTHON, GATEWAY_SCRIPT],
                    stdin=subprocess.PIPE,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                    text=True,
                )
                print(f"[启动] 子进程 PID={proc.pid}: {PYTHON} {GATEWAY_SCRIPT}")

                await asyncio.gather(
                    pipe_ws_to_proc(ws, proc),
                    pipe_proc_to_ws(proc, ws),
                    pipe_proc_stderr(proc),
                )

        except websockets.exceptions.ConnectionClosed as e:
            print(f"[断开] {e.code} {e.reason}")
        except Exception as e:
            print(f"[错误] {type(e).__name__}: {e}")
        finally:
            if proc and proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    proc.kill()

        print(f"[重连] {backoff}s 后重试...")
        await asyncio.sleep(backoff)
        backoff = min(backoff * 2, MAX_BACKOFF)


if __name__ == "__main__":
    print("=" * 50)
    print("NewMCP Smart Gateway - XiaoZhi Cloud Pipe")
    print("=" * 50)
    signal.signal(signal.SIGINT, lambda *_: sys.exit(0))
    asyncio.run(run())
