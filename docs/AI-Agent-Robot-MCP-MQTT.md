# AI Agent 驱动小机器人 — MCP + MQTT 云端接入方案

> 调研日期: 2026-05-04 | 版本: V2.0 | 状态: 调研完成

---

## 1. 概述

### 1.1 目标

让 AI Agent（如 Claude、小智等）通过自然语言驱动小机器人执行动作（前进、后退、跳舞等），通过云平台统一管理 MCP 服务，实现 **自然语言 → LLM → MCP 命令 → MQTT → 机器人执行** 的完整链路。

### 1.2 核心架构

```
┌─────────────┐     MCP tools/call     ┌──────────────┐
│  AI Agent   │ ──────────────────────→ │  NewMCP 网关  │
│ (Claude/    │                        │ (Streamable   │
│  小智/GPT)  │ ←────────────────────  │  HTTP)        │
└─────────────┘     MCP response       └──────┬───────┘
                                               │ 工具路由
                                               ▼
                                      ┌─────────────────┐
                                      │ Robot MCP Server │
                                      │ (Python/FastMCP) │
                                      │                  │
                                      │ MCP 工具翻译为    │
                                      │ MQTT 消息发布     │
                                      └────────┬─────────┘
                                               │ MQTT publish
                                               ▼
                                      ┌─────────────────┐
                                      │  MQTT Broker     │
                                      │ (EMQX/Mosquitto) │
                                      └────────┬─────────┘
                                               │ MQTT subscribe
                                               ▼
                                      ┌─────────────────┐
                                      │   小机器人        │
                                      │  (ESP32/Arduino) │
                                      │                  │
                                      │ WiFi → MQTT 连接 │
                                      │ 接收指令 → 执行   │
                                      └─────────────────┘
```

### 1.3 为什么选择 MQTT

| 特性 | 说明 |
|------|------|
| **轻量** | 最小报文仅 2 字节，ESP32 原生支持 |
| **可靠** | QoS 0/1/2 三级消息送达保障 |
| **双向** | 发布/订阅模式，天然支持指令下发 + 状态回传 |
| **断线重连** | 自动重连 + 遗嘱消息（LWT），设备离线可感知 |
| **生态成熟** | EMQX、Mosquitto 等开源 Broker 稳定可靠 |
| **低功耗** | 适合电池供电的小机器人长期运行 |

---

## 2. 全链路数据流

### 2.1 指令下发（Agent → 机器人）

```
用户: "让机器人向前走两秒"
  → AI Agent 解析意图
  → 调用 MCP 工具: robot.move_forward(speed=50, duration=2000)
  → NewMCP 网关路由到 Robot MCP Server
  → MCP Server 发布 MQTT 消息:
      Topic: robot/bot-001/command
      Payload: {"action":"forward","speed":50,"duration":2000,"cmd_id":"a1b2c3"}
  → ESP32 收到指令，驱动电机前进
  → ESP32 发布状态:
      Topic: robot/bot-001/status
      Payload: {"cmd_id":"a1b2c3","status":"completed","action":"forward"}
  → MCP Server 收到状态，返回 MCP 响应
  → AI Agent 回复用户: "机器人已向前走了两秒"
```

### 2.2 状态查询

```
用户: "机器人现在什么状态"
  → AI Agent 调用: robot.get_status()
  → MCP Server 发布 MQTT:
      Topic: robot/bot-001/query
      Payload: {"type":"status","query_id":"q1"}
  → ESP32 回复:
      Topic: robot/bot-001/status
      Payload: {"battery":78,"position":{"x":1.2,"y":0.5},"state":"idle"}
  → MCP Server 返回状态信息给 Agent
```

---

## 3. 各层技术实现

### 3.1 机器人固件层（ESP32/Arduino）

**硬件选型参考：**

| 方案 | 芯片 | 适合场景 | 参考价格 |
|------|------|----------|----------|
| ESP32 + 电机驱动板 | ESP32-WROOM | 履带/轮式小车 | ¥30-50 |
| ESP32 + 舵机 | ESP32-S3 | 多自由度机器人 | ¥20-40 |
| Arduino + ESP8266 | ATmega328P + ESP8266 | 简单小车 | ¥20-30 |

**固件核心逻辑：**

```
1. WiFi 连接
2. MQTT 连接 Broker（mqtt://broker-ip:1883）
3. 订阅 robot/{device_id}/command
4. 主循环:
   - 收到命令 → 解析 JSON → 执行动作
   - 定时上报状态（电池/位置/传感器）
   - 心跳保活
```

**MQTT 主题设计：**

| 主题 | 方向 | 说明 |
|------|------|------|
| `robot/{device_id}/command` | 下行 | AI Agent 下发的指令 |
| `robot/{device_id}/status` | 上行 | 机器人状态上报 |
| `robot/{device_id}/query` | 下行 | 查询请求 |
| `robot/{device_id}/response` | 上行 | 查询响应 |
| `robot/{device_id}/heartbeat` | 上行 | 心跳（每 30s） |
| `robot/online` | 上行 | 遗嘱消息（LWT），设备上下线通知 |

**固件伪代码（Arduino/ESP32）：**

```cpp
#include <WiFi.h>
#include <PubSubClient.h>
#include <ArduinoJson.h>

WiFiClient espClient;
PubSubClient mqtt(espClient);

const char* DEVICE_ID = "bot-001";
const char* MQTT_BROKER = "broker.emqx.io";
const int MQTT_PORT = 1883;

void onCommand(char* topic, byte* payload, unsigned int len) {
    StaticJsonDocument<256> doc;
    deserializeJson(doc, payload, len);

    String action = doc["action"];
    int speed = doc["speed"] | 50;
    int duration = doc["duration"] | 1000;

    if (action == "forward")  moveForward(speed, duration);
    if (action == "backward") moveBackward(speed, duration);
    if (action == "left")     turnLeft(speed, duration);
    if (action == "right")    turnRight(speed, duration);
    if (action == "dance")    dance(doc["pattern"] | "spin");
    if (action == "stop")     stopMotors();

    // 发布执行状态
    publishStatus(action, "completed");
}

void setup() {
    WiFi.begin("SSID", "PASSWORD");
    mqtt.setServer(MQTT_BROKER, MQTT_PORT);
    mqtt.setCallback(onCommand);
    connectMQTT();
}

void connectMQTT() {
    String willTopic = "robot/online";
    String willMsg = "{\"device\":\"bot-001\",\"status\":\"offline\"}";

    mqtt.connect("bot-001", NULL, NULL,
                 willTopic.c_str(), 1, false, willMsg.c_str());
    mqtt.subscribe("robot/bot-001/command");

    // 上线通知
    mqtt.publish("robot/online",
                 "{\"device\":\"bot-001\",\"status\":\"online\"}");
}

void loop() {
    mqtt.loop();
    // 定时心跳
    if (millis() - lastHeartbeat > 30000) {
        publishHeartbeat();
        lastHeartbeat = millis();
    }
}
```

### 3.2 MQTT Broker 层

**推荐方案：EMQX（开源版）**

```bash
# Docker 一键部署
docker run -d --name emqx \
  -p 1883:1883 \
  -p 8083:8083 \
  -p 18083:18083 \
  emqx/emqx:latest

# 管理面板: http://localhost:18083
# 默认账号: admin / public
```

**备选：Mosquitto（更轻量）**

```bash
docker run -d --name mosquitto \
  -p 1883:1883 \
  -v ./mosquitto.conf:/mosquitto/config/mosquitto.conf \
  eclipse-mosquitto:latest
```

**Broker 配置要点：**

| 配置项 | 建议值 | 说明 |
|--------|--------|------|
| 认证 | 用户名/密码 或 Token | 防止未授权设备接入 |
| ACL | 按设备 ID 限制主题 | 设备只能订阅自己的 command 主题 |
| TLS | 生产环境建议启用 | MQTT over TLS (8883端口) |
| QoS | 指令用 QoS 1，心跳用 QoS 0 | 平衡可靠性和开销 |
| 遗嘱消息 (LWT) | 必须配置 | 设备异常断线时自动通知 |

### 3.3 Robot MCP Server 层（Python）

**技术栈：** Python 3.10+ / FastMCP 2.0 / paho-mqtt

**工具定义：**

| 工具名 | 参数 | 返回 | 说明 |
|--------|------|------|------|
| `robot.move_forward` | speed(0-100), duration(ms) | 执行结果 | 前进 |
| `robot.move_backward` | speed(0-100), duration(ms) | 执行结果 | 后退 |
| `robot.turn_left` | angle(0-360) | 执行结果 | 左转 |
| `robot.turn_right` | angle(0-360) | 执行结果 | 右转 |
| `robot.dance` | pattern(spin/wave/wiggle) | 执行结果 | 跳舞 |
| `robot.stop` | 无 | 执行结果 | 急停 |
| `robot.get_status` | 无 | 电池/位置/状态 | 查询状态 |
| `robot.set_led` | color(hex), mode(blink/breathe) | 执行结果 | 控制 LED |
| `robot.beep` | frequency(hz), duration(ms) | 执行结果 | 蜂鸣器 |

**MCP Server 伪代码（Python）：**

```python
from fastmcp import FastMCP
import paho.mqtt.client as mqtt
import json, uuid, time

mcp = FastMCP("robot-control")

# MQTT 连接
MQTT_BROKER = "broker.emqx.io"
MQTT_PORT = 1883
DEVICE_ID = "bot-001"

client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2)
client.connect(MQTT_BROKER, MQTT_PORT)
client.loop_start()

# 等待 MQTT 响应的同步机制
pending_requests = {}

def on_response(client, userdata, msg):
    data = json.loads(msg.payload)
    if data.get("cmd_id") in pending_requests:
        pending_requests[data["cmd_id"]].append(data)

client.on_message = on_response
client.subscribe(f"robot/{DEVICE_ID}/status")

def send_command(action: str, **kwargs) -> dict:
    """发送 MQTT 指令并等待响应"""
    cmd_id = str(uuid.uuid4())[:8]
    payload = {"action": action, "cmd_id": cmd_id, **kwargs}

    pending_requests[cmd_id] = []
    client.publish(f"robot/{DEVICE_ID}/command", json.dumps(payload))

    # 等待机器人响应（超时 5 秒）
    for _ in range(50):
        if pending_requests[cmd_id]:
            result = pending_requests.pop(cmd_id)[0]
            return result
        time.sleep(0.1)

    pending_requests.pop(cmd_id, None)
    return {"status": "timeout", "message": "机器人未响应"}

# MCP 工具定义
@mcp.tool()
def move_forward(speed: int = 50, duration: int = 2000) -> str:
    """控制机器人前进。speed: 速度(0-100), duration: 持续时间(毫秒)"""
    result = send_command("forward", speed=speed, duration=duration)
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def move_backward(speed: int = 50, duration: int = 2000) -> str:
    """控制机器人后退。speed: 速度(0-100), duration: 持续时间(毫秒)"""
    result = send_command("backward", speed=speed, duration=duration)
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def turn_left(angle: int = 90) -> str:
    """控制机器人左转。angle: 转角(0-360度)"""
    result = send_command("left", angle=angle)
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def turn_right(angle: int = 90) -> str:
    """控制机器人右转。angle: 转角(0-360度)"""
    result = send_command("right", angle=angle)
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def dance(pattern: str = "spin") -> str:
    """让机器人跳舞。pattern: 舞蹈模式(spin/wave/wiggle)"""
    result = send_command("dance", pattern=pattern)
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def stop() -> str:
    """紧急停止机器人"""
    result = send_command("stop")
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def get_status() -> str:
    """获取机器人当前状态（电池、位置、运行状态等）"""
    result = send_command("query", type="status")
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def set_led(color: str = "#FF0000", mode: str = "breathe") -> str:
    """控制机器人 LED 灯。color: 颜色(十六进制), mode: 模式(blink/breathe)"""
    result = send_command("led", color=color, mode=mode)
    return json.dumps(result, ensure_ascii=False)

@mcp.tool()
def beep(frequency: int = 1000, duration: int = 500) -> str:
    """控制机器人蜂鸣器。frequency: 频率(Hz), duration: 持续时间(ms)"""
    result = send_command("beep", frequency=frequency, duration=duration)
    return json.dumps(result, ensure_ascii=False)

if __name__ == "__main__":
    mcp.run(transport="streamable-http", port=8000)
```

### 3.4 NewMCP 平台配置

在 NewMCP 平台上的配置步骤：

**Step 1 — 注册 MCP 服务**

| 配置项 | 值 |
|--------|------|
| 服务名称 | robot-control |
| 传输类型 | Streamable HTTP |
| URL | http://localhost:8000/mcp |
| 描述 | 小机器人控制 MCP 服务 |

**Step 2 — 创建分组**

| 配置项 | 值 |
|--------|------|
| 分组名称 | 机器人控制 |
| 关联服务 | robot-control |
| 可见性 | private（或 public） |

**Step 3 — 配置 API Key**

| 配置项 | 值 |
|--------|------|
| 名称 | robot-agent-key |
| 权限分组 | ["机器人控制"] |
| 用途 | 绑定给 AI Agent 或云端连接使用 |

**Step 4 —（可选）配置云端连接**

如果需要通过小智等平台暴露：

| 配置项 | 值 |
|--------|------|
| 连接类型 | xiaozhi / custom(WSS) |
| 端点 | wss://xiaozhi-cloud.example.com/ws |
| 绑定 API Key | robot-agent-key |

---

## 4. 安全设计

### 4.1 传输安全

| 层 | 安全措施 |
|----|----------|
| Agent → NewMCP | HTTPS + API Key 认证 |
| NewMCP → MCP Server | 内网通信 / TLS |
| MCP Server → MQTT Broker | MQTT 用户名/密码 + TLS |
| MQTT Broker → 机器人 | TLS + 设备证书 + ACL |

### 4.2 指令安全

- **指令超时**: 每条指令设置 5 秒超时，避免机器人卡在某个状态
- **急停优先**: stop 指令使用最高 QoS，确保送达
- **速度限制**: MCP Server 端校验 speed 参数范围，防止意外高速
- **指令 ID 追踪**: 每条指令带 cmd_id，可追踪执行链路

### 4.3 设备认证

```bash
# EMQX 配置设备认证
# 每个设备使用独立的 用户名/密码
# ACL 限制每个设备只能订阅自己的主题
```

---

## 5. 多机器人扩展

当有多台机器人时，架构无需大改：

```
AI Agent
  → NewMCP 网关
    → Robot MCP Server
      → MQTT Broker
        ├── robot/bot-001/command  →  机器人 A（履带车）
        ├── robot/bot-002/command  →  机器人 B（机械臂）
        └── robot/bot-003/command  →  机器人 C（四足）
```

**扩展方式：**

| 方式 | 说明 |
|------|------|
| 同一 MCP Server | 在 MCP Server 中支持 `device_id` 参数，路由到不同机器人 |
| 多个 MCP Server | 每种机器人一个 MCP Server，在 NewMCP 中归入同一分组 |
| MCP 工具参数 | 工具增加 `robot_id` 参数，Agent 选择操作哪台机器人 |

**多机器人工具定义示例：**

```python
@mcp.tool()
def move_forward(robot_id: str, speed: int = 50, duration: int = 2000) -> str:
    """控制指定机器人前进"""
    result = send_command(robot_id, "forward", speed=speed, duration=duration)
    return json.dumps(result, ensure_ascii=False)
```

---

## 6. 开源参考项目

| 项目 | 地址 | 说明 |
|------|------|------|
| mqtt-mcp | https://github.com/ezhuk/mqtt-mcp | 轻量 MCP↔MQTT 桥接，FastMCP 2.0 |
| mcp2mqtt | https://github.com/echoxor/mcp2mqtt | MCP 命令转 MQTT 中间件 |
| EMQX MCP Bridge | https://docs.emqx.com/en/emqx/latest/emqx-ai/mcp-bridge/ | 企业级 MCP over MQTT |
| IoT-Edge-MCP-Server | https://github.com/poly-mcp/iot-edge-mcp-server | 工业 IoT MCP 服务器 |
| ROS-MCP-Server | https://github.com/robotmcp/ros-mcp-server | ROS 机器人 MCP 集成 |

---

## 7. 技术栈汇总

| 层 | 技术 | 版本 | 用途 |
|----|------|------|------|
| AI Agent | Claude / 小智 / GPT | — | 自然语言理解 + 工具调用 |
| MCP 网关 | NewMCP | V1 | 统一管理、路由、协议桥接 |
| MCP Server | Python + FastMCP | 3.10+ / 2.0 | MCP 工具定义 + MQTT 翻译 |
| MQTT Client | paho-mqtt | ≥1.6 | Python MQTT 客户端 |
| MQTT Broker | EMQX / Mosquitto | 5.x / 2.x | 消息中间件 |
| 机器人固件 | Arduino / ESP-IDF | — | MQTT 订阅 + 电机控制 |
| 机器人芯片 | ESP32-WROOM / S3 | — | WiFi + MQTT + GPIO 控制 |

---

## 8. 实施步骤建议

| 阶段 | 内容 | 预估时间 |
|------|------|----------|
| **Phase 1** | 搭建 MQTT Broker + ESP32 固件（先用串口调试） | 1-2 天 |
| **Phase 2** | 编写 Robot MCP Server（Python），验证工具调用 | 1-2 天 |
| **Phase 3** | 接入 NewMCP 平台，配置服务和分组 | 0.5 天 |
| **Phase 4** | 对接 AI Agent，端到端联调 | 1 天 |
| **Phase 5** | 安全加固（TLS、认证、ACL）+ 多机器人扩展 | 1-2 天 |

---

## 9. 备选方案对比

| 方案 | 优势 | 劣势 | 适合场景 |
|------|------|------|----------|
| **MCP + MQTT 桥接** | 固件简单，生态成熟 | 多一层桥接 | ESP32 小机器人（快速验证） |
| **MCP over MQTT** | 标准化，服务发现，可扩展 | 固件需实现 MCP 协议栈 | **规模化部署（推荐演进方向）** |
| WebSocket 直连 | 架构简单 | ESP32 实现复杂 | 树莓派级别机器人 |
| ROS + MCP | 功能强大 | 依赖 ROS，重 | 工业机器人、科研平台 |

---

## 10. 方案 2 详解：MCP over MQTT（EMQX 标准）

> 参考规范: [MCP over MQTT Specification 2025-03-26](https://mqtt.ai/docs/mcp-over-mqtt/specification/2025-03-26/basic/architecture.html)
>
> 参考实现: [EMQX MCP Bridge Plugin](https://docs.emqx.com/en/emqx/latest/emqx-ai/mcp-bridge/overview.html)

### 10.1 核心架构

方案 1 中，MCP Server 和机器人之间是**自定义的 MQTT 主题 + 自定义 JSON 格式**，扩展靠改代码。

方案 2 中，机器人**直接作为 MCP Server**，通过标准化的 MQTT 主题和 JSON-RPC 2.0 协议与 Broker 通信，Broker 负责**服务发现、路由、认证、负载均衡**。

```
┌─────────────┐       MCP (HTTP/SSE)       ┌──────────────────────┐
│  AI Agent   │ ──────────────────────────→ │  EMQX MCP Bridge     │
│ (Claude/    │                             │  Plugin               │
│  小智/GPT)  │ ←────────────────────────── │  (协议转换 HTTP↔MQTT)  │
└─────────────┘       MCP response          └──────────┬───────────┘
                                                          │ MCP over MQTT
                                                          │ (JSON-RPC 2.0)
                                                          ▼
                                                 ┌──────────────────┐
                                                 │  EMQX Broker     │
                                                 │  (MQTT 5.0)      │
                                                 │                  │
                                                 │  · 服务发现       │
                                                 │  · 负载均衡       │
                                                 │  · ACL 鉴权       │
                                                 │  · 工具聚合       │
                                                 └────────┬─────────┘
                                                          │ MQTT 5.0
                                           ┌──────────────┼──────────────┐
                                           ▼              ▼              ▼
                                    ┌────────────┐ ┌────────────┐ ┌────────────┐
                                    │ 机器人 A    │ │ 机器人 B    │ │ 机器人 C    │
                                    │ (MCP Server)│ │ (MCP Server)│ │ (MCP Server)│
                                    │ bot-001     │ │ bot-002     │ │ bot-003     │
                                    │ 前进/后退    │ │ 抓取/放下    │ │ 跑/跳       │
                                    └────────────┘ └────────────┘ └────────────┘
```

### 10.2 与方案 1 的关键区别

| 维度 | 方案 1：MCP + MQTT 桥接 | 方案 2：MCP over MQTT |
|------|-------------------------|----------------------|
| **设备角色** | 纯 MQTT 客户端，被动接收自定义命令 | MCP Server，主动注册工具，自描述能力 |
| **协议格式** | 自定义 JSON（`{"action":"forward",...}`） | 标准 JSON-RPC 2.0（`tools/call`、`tools/list`） |
| **主题规范** | 自定义（`robot/{id}/command`） | 标准化（`$mcp-rpc/...`、`$mcp-server/...`） |
| **工具发现** | 在桥接层硬编码 | 设备上线自动注册，Agent 动态发现 |
| **多设备** | 桥接层手动路由 | Broker 自动聚合同类设备工具 |
| **负载均衡** | 无 | Broker 层多实例自动路由 |
| **认证鉴权** | MCP Server 自行实现 | Broker 统一 ACL |
| **MQTT 版本** | 3.1.1 或 5.0 均可 | **必须 MQTT 5.0** |
| **扩展性** | 1-10 台设备 | 10 - 10 万台设备 |

### 10.3 MQTT 主题规范

MCP over MQTT 定义了一套标准化的 MQTT 5.0 主题结构：

| 主题 | 用途 | 方向 |
|------|------|------|
| `$mcp-server/presence/{server-id}/{server-name}` | 设备上下线通知（保留消息） | 设备 → Broker |
| `$mcp-server/{server-id}/{server-name}` | 初始化、控制消息 | 双向 |
| `$mcp-server/capability/{server-id}/{server-name}` | 工具列表变更通知 | 设备 → Broker |
| `$mcp-rpc/{client-id}/{server-id}/{server-name}` | RPC 请求/响应（tools/call 等） | 双向 |
| `$mcp-client/presence/{client-id}` | 客户端上下线通知 | 客户端 → Broker |

**以机器人控制为例：**

```
# 机器人 bot-001 上线，注册为 MCP Server
发布 → $mcp-server/presence/bot-001/robot-wheeled
Payload: {"status":"online","tools":["move_forward","move_backward","turn_left","turn_right","dance","stop"]}

# AI Agent 发起工具调用
发布 → $mcp-rpc/agent-001/bot-001/robot-wheeled
Payload: {"jsonrpc":"2.0","method":"tools/call","params":{"name":"move_forward","arguments":{"speed":50,"duration":2000}},"id":1}

# 机器人返回执行结果
发布 → $mcp-rpc/agent-001/bot-001/robot-wheeled
Payload: {"jsonrpc":"2.0","result":{"status":"completed","action":"forward"},"id":1}
```

### 10.4 设备端实现：机器人作为 MCP Server

在方案 2 中，每台机器人内嵌 MCP over MQTT 协议栈，**直接暴露 MCP 工具**。

**两种实现路径：**

| 路径 | 说明 | 适合设备 |
|------|------|----------|
| **A. 设备原生实现** | 机器人固件直接实现 MCP over MQTT 协议 | 有 SDK 支持的设备 |
| **B. 代理模式（推荐起步）** | 外部代理进程实现 MCP 协议，通过标准 MQTT 与设备通信 | ESP32 等资源受限设备 |

#### 路径 A：设备原生实现（SDK 嵌入）

设备使用 MCP over MQTT SDK 注册工具：

```python
# 运行在机器人 onboard 电脑（如树莓派）上的 MCP Server
from mcp_over_mqtt import MCPOverMQTTServer

server = MCPOverMQTTServer(
    server_name="robot-wheeled",
    server_id="bot-001",
    broker_host="broker.emqx.io",
    broker_port=1883,
)

@server.tool()
def move_forward(speed: int = 50, duration: int = 2000) -> dict:
    """控制机器人前进"""
    # 调用底层硬件驱动
    motor_driver.forward(speed, duration)
    return {"status": "completed", "action": "forward"}

@server.tool()
def move_backward(speed: int = 50, duration: int = 2000) -> dict:
    """控制机器人后退"""
    motor_driver.backward(speed, duration)
    return {"status": "completed", "action": "backward"}

@server.tool()
def dance(pattern: str = "spin") -> dict:
    """让机器人跳舞"""
    choreography.execute(pattern)
    return {"status": "completed", "action": "dance", "pattern": pattern}

@server.tool()
def stop() -> dict:
    """紧急停止"""
    motor_driver.stop()
    return {"status": "completed", "action": "stop"}

@server.tool()
def get_status() -> dict:
    """获取机器人状态"""
    return {
        "battery": battery.level(),
        "position": odometry.position(),
        "state": motor_driver.state(),
    }

server.run()
```

#### 路径 B：代理模式（ESP32 友好）

```
┌─────────────┐    标准 MQTT     ┌───────────────┐   MCP over MQTT   ┌──────────┐
│  ESP32      │ ←──────────────→ │  代理进程      │ ←────────────────→ │  EMQX    │
│  (原始MQTT) │  robot/cmd       │  (Python)      │  $mcp-rpc/...      │  Broker  │
│             │  robot/status    │  MCP Server    │                    │          │
└─────────────┘                  └───────────────┘                    └──────────┘
```

代理进程相当于方案 1 中 MCP Server 的升级版：对外走 MCP over MQTT 标准协议，对内仍用简单 MQTT 与 ESP32 通信。ESP32 固件**不需要改动**。

```python
# 代理进程：同时作为 MCP over MQTT Server 和 ESP32 的 MQTT 桥接
import paho.mqtt.client as mqtt
from mcp_over_mqtt import MCPOverMQTTServer

# 连接 ESP32 的 MQTT（内部协议，简单 JSON）
esp_mqtt = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2)
esp_mqtt.connect("localhost", 1883)

# 对外暴露为标准 MCP Server
server = MCPOverMQTTServer(
    server_name="robot-wheeled",
    server_id="bot-001",
    broker_host="broker.emqx.io",
    broker_port=1883,
)

pending = {}

def on_esp_response(client, userdata, msg):
    data = json.loads(msg.payload)
    if data.get("cmd_id") in pending:
        pending[data["cmd_id"]].set_result(data)

esp_mqtt.on_message = on_esp_response
esp_mqtt.subscribe("robot/bot-001/status")

def send_to_esp(action: str, **kwargs) -> dict:
    cmd_id = uuid.uuid4().hex[:8]
    fut = asyncio.get_event_loop().create_future()
    pending[cmd_id] = fut
    esp_mqtt.publish("robot/bot-001/command",
                     json.dumps({"action": action, "cmd_id": cmd_id, **kwargs}))
    return asyncio.wait_for(fut, timeout=5.0)

@server.tool()
def move_forward(speed: int = 50, duration: int = 2000) -> dict:
    """控制机器人前进"""
    return send_to_esp("forward", speed=speed, duration=duration)

@server.tool()
def dance(pattern: str = "spin") -> dict:
    """让机器人跳舞"""
    return send_to_esp("dance", pattern=pattern)

# ... 其他工具

server.run()
```

### 10.5 EMQX Broker 配置

#### 安装 EMQX（需要企业版才支持 MCP Bridge Plugin）

```bash
# EMQX 企业版
docker run -d --name emqx-ee \
  -p 1883:1883 \
  -p 8883:8883 \
  -p 18083:18083 \
  -p 8083:8083 \
  emqx/emqx-enterprise:latest

# 管理面板: http://localhost:18083
```

#### 启用 MCP Bridge Plugin

在 EMQX Dashboard 或配置文件中启用 MCP Bridge Plugin：

```hocon
# emqx.conf
emqx_ai {
  mcp_bridge {
    enable = true
    http_endpoint = "0.0.0.0:8083"
    # 工具聚合策略：按 server-name 聚合同类设备
    aggregation = "server_name"
  }
}
```

启用后，EMQX 暴露标准 MCP HTTP 端点，AI Agent 直接连接：

```
MCP 端点: http://emqx-host:8083/mcp
传输协议: Streamable HTTP / SSE
```

#### 配置 ACL 鉴权

```hocon
# 每个 AI Agent 只能访问授权的设备
authorization {
  rules = [
    {action = "subscribe", topic = "$mcp-rpc/agent-001/+/robot-wheeled", permission = "allow"},
    {action = "publish", topic = "$mcp-rpc/agent-001/+/robot-wheeled", permission = "allow"},
  ]
}
```

### 10.6 服务发现与工具聚合

#### 自动服务发现

机器人上线后自动注册，AI Agent 通过 Broker 的保留消息自动发现可用设备：

```
1. 机器人 bot-001 上线
   → 发布保留消息到 $mcp-server/presence/bot-001/robot-wheeled
   → Payload: {"status":"online"}

2. 机器人 bot-002 上线
   → 发布保留消息到 $mcp-server/presence/bot-002/robot-arm
   → Payload: {"status":"online"}

3. AI Agent 订阅 $mcp-server/presence/+/*
   → 自动收到所有在线设备列表
   → 调用 tools/list 获取每台设备的工具清单
```

#### 同类设备工具聚合

EMQX MCP Bridge 自动将同类型设备（相同 `server-name`）的工具聚合为一个逻辑工具，并注入 `target-mqtt-client-id` 参数用于路由：

```
机器人 A (bot-001, robot-wheeled) 注册工具: move_forward, dance, stop
机器人 B (bot-002, robot-wheeled) 注册工具: move_forward, dance, stop

Bridge 自动聚合为:
  工具: move_forward(target-mqtt-client-id, speed, duration)
  工具: dance(target-mqtt-client-id, pattern)
  工具: stop(target-mqtt-client-id)

AI Agent 调用:
  move_forward(target-mqtt-client-id="bot-001", speed=50, duration=2000)
  → Bridge 自动路由到 bot-001
```

### 10.7 负载均衡与水平扩展

```
                 ┌── MCP Server 实例 1 (bot-proxy-001)
Agent → Broker ──┤── MCP Server 实例 2 (bot-proxy-002)   ← 共享 server-name
                 └── MCP Server 实例 3 (bot-proxy-003)

Broker 基于共享订阅自动路由:
  - 新请求随机分配到空闲实例
  - 实例宕机，请求自动转移到其他实例
  - 加实例无需改配置
```

适用场景：代理模式中，启动多个代理进程处理大量机器人的请求。

### 10.8 NewMCP 平台配置（方案 2）

方案 2 中 NewMCP 的配置更简单，因为 EMQX Bridge 已经承担了协议转换和服务发现：

**Step 1 — 注册 MCP 服务**

| 配置项 | 值 |
|--------|------|
| 服务名称 | emqx-robot-mcp |
| 传输类型 | Streamable HTTP |
| URL | http://emqx-host:8083/mcp |
| 描述 | 通过 EMQX MCP Bridge 暴露的机器人控制服务 |

**Step 2 — 创建分组 + API Key**（同方案 1）

**Step 3 — 无需单独的 Python MCP Server**

方案 2 中，EMQX Bridge Plugin 已经充当了 MCP Server 的角色。NewMCP 直接连接 EMQX 的 HTTP 端点即可。

### 10.9 方案 1 → 方案 2 迁移路径

从方案 1 平滑升级到方案 2：

```
阶段 1（当前）: 方案 1
  ESP32 ←简单MQTT→ Python MCP Server ←MCP HTTP→ NewMCP

阶段 2（过渡）: 代理模式
  ESP32 ←简单MQTT→ Python 代理进程 ←MCP over MQTT→ EMQX ←MCP HTTP→ NewMCP
  (ESP32 固件不变，Python 代理进程升级为 MCP over MQTT Server)

阶段 3（最终）: 原生模式
  机器人(树莓派) ←MCP over MQTT→ EMQX ←MCP HTTP→ NewMCP
  (设备能力足够时，去掉代理，设备直接跑 MCP Server)
```

迁移要点：
- 阶段 1→2：ESP32 固件**零改动**，只升级代理进程 + 引入 EMQX
- MQTT 主题设计在阶段 1 就预留 `$mcp-server/` 前缀，便于后续迁移
- 阶段 2→3：当设备升级为树莓派等更强硬件时，直接嵌入 SDK

### 10.10 方案 2 技术栈

| 层 | 技术 | 版本 | 用途 |
|----|------|------|------|
| AI Agent | Claude / 小智 / GPT | — | 自然语言理解 + 工具调用 |
| MCP 网关 | NewMCP | V1 | 统一管理、路由 |
| 协议转换 | EMQX MCP Bridge Plugin | 企业版 5.8+ | HTTP ↔ MQTT 协议桥接 |
| MQTT Broker | EMQX Enterprise | 5.8+ / 6.x | 服务发现、负载均衡、ACL |
| MQTT 版本 | MQTT 5.0 | — | 必须使用 5.0 |
| 代理进程 | Python + MCP over MQTT SDK | — | ESP32 场景的协议翻译 |
| 机器人固件 | Arduino / ESP-IDF | — | MQTT 3.1.1 + JSON（代理模式）|
| 或原生 MCP | MCP over MQTT SDK | — | 树莓派场景直接嵌入 |

### 10.11 方案 2 实施步骤

| 阶段 | 内容 | 预估时间 |
|------|------|----------|
| **Phase 1** | 部署 EMQX 企业版 + 启用 MCP Bridge Plugin | 0.5 天 |
| **Phase 2** | 编写 Python 代理进程（MCP over MQTT Server + ESP32 桥接） | 1-2 天 |
| **Phase 3** | 配置 EMQX ACL、设备认证、工具聚合规则 | 0.5 天 |
| **Phase 4** | ESP32 连接测试（复用方案 1 的固件） | 0.5 天 |
| **Phase 5** | NewMCP 注册 EMQX Bridge 端点 + AI Agent 联调 | 0.5 天 |
| **Phase 6** | 多设备接入验证、负载测试 | 1 天 |

### 10.12 方案 2 注意事项

| 事项 | 说明 |
|------|------|
| **EMQX 企业版** | MCP Bridge Plugin 仅企业版支持，开源版不支持 |
| **MQTT 5.0 必须** | 规范要求必须使用 MQTT 5.0，不支持 3.1.1 |
| **SDK 生态** | MCP over MQTT SDK 目前以 Python/Node.js 为主，C/Arduino SDK 尚不成熟 |
| **ESP32 限制** | 直接实现 MCP 协议栈对 ESP32 偏重，建议走代理模式 |
| **长连接开销** | MQTT 5.0 维持长连接比 3.1.1 占用更多内存 |
| **网络要求** | 设备需持续联网，断网期间无法接收指令 |
