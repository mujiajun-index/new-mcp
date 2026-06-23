# NewMCP API 接口文档

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-11 (更新分组接口)

## 1. 概述

### Base URL
```
http://localhost:3000/api/v1
```

### 认证方式

所有接口（除 `/auth/register` 和 `/auth/login` 外）需要在请求头中携带认证信息：

```
Authorization: Bearer <JWT_TOKEN>
```

或

```
X-API-Key: <API_KEY>
```

### 通用响应格式

```json
{
    "success": true,
    "message": "操作成功",
    "data": { ... }
}
```

错误响应：
```json
{
    "success": false,
    "message": "错误描述",
    "code": "ERROR_CODE"
}
```

### 分页参数

```
GET /api/v1/services?page=1&page_size=20&sort=created_at&order=desc
```

响应包含分页信息：
```json
{
    "success": true,
    "data": [...],
    "pagination": {
        "page": 1,
        "page_size": 20,
        "total": 100,
        "total_pages": 5
    }
}
```

---

## 2. 系统初始化接口

> 以下接口为公开接口，无需认证。系统初始化完成后（Setup 记录存在），两个接口均返回 `403 Forbidden`。

### GET /setup
查询系统初始化状态。

**Response (未初始化):** `200 OK`
```json
{
    "success": true,
    "data": {
        "status": false,
        "admin_init": false,
        "database_type": "sqlite"
    }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| status | bool | 是否已初始化 |
| admin_init | bool | 是否已存在管理员账号 |
| database_type | string | 数据库类型: sqlite / mysql / postgres |

**Response (已初始化):** `403 Forbidden`
```json
{
    "success": false,
    "message": "系统已经初始化完成"
}
```

### POST /setup
执行系统初始化，创建管理员账号。

**Request Body:**
```json
{
    "username": "string (1-64字符, 必填)",
    "password": "string (至少8字符, 必填)",
    "confirm_password": "string (必须与password一致, 必填)"
}
```

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "success"
}
```

**错误响应:**
| HTTP 状态码 | message | 说明 |
|-------------|---------|------|
| 403 | 系统已经初始化完成 | 重复初始化 |
| 400 | 用户名长度应在1-64个字符之间 | 用户名校验失败 |
| 400 | 密码长度至少为8个字符 | 密码过短 |
| 400 | 两次输入的密码不一致 | 密码确认不匹配 |
| 500 | 创建管理员账号失败 | 数据库写入异常 |

---

## 3. 认证接口

### POST /auth/register
注册新用户。

**Request Body:**
```json
{
    "username": "string (3-64字符, 必填)",
    "password": "string (6-128字符, 必填)",
    "email": "string (可选)"
}
```

**Response:** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "username": "testuser",
        "token": "eyJhbGciOiJIUzI1NiIs..."
    }
}
```

### POST /auth/login
用户登录。

**Request Body:**
```json
{
    "username": "string (必填)",
    "password": "string (必填)"
}
```

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "username": "testuser",
        "role": "user",
        "token": "eyJhbGciOiJIUzI1NiIs..."
    }
}
```

### GET /auth/profile
获取当前用户信息。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "username": "testuser",
        "email": "test@example.com",
        "role": "user",
        "avatar_url": "",
        "status": 1,
        "created_at": "2026-05-03T00:00:00Z"
    }
}
```

### PUT /auth/profile
更新当前用户信息。

**Request Body:**
```json
{
    "email": "new@example.com",
    "avatar_url": "https://..."
}
```

### PUT /auth/password
修改密码。

**Request Body:**
```json
{
    "old_password": "string (必填)",
    "new_password": "string (必填)"
}
```

---

## 4. MCP 服务接口

### GET /services
获取当前用户的 MCP 服务列表。

**Query Parameters:**
| 参数 | 类型 | 说明 |
|------|------|------|
| page | int | 页码 (默认 1) |
| page_size | int | 每页数量 (默认 20) |
| transport_type | string | 按传输类型过滤 |
| status | int | 按状态过滤 |
| keyword | string | 按名称/描述搜索 |

**Response:** `200 OK`
```json
{
    "success": true,
    "data": [
        {
            "id": 1,
            "name": "exa-search",
            "display_name": "Exa Web Search",
            "description": "Web search powered by Exa",
            "transport_type": "streamable-http",
            "config": {
                "url": "https://mcp.exa.ai/mcp"
            },
            "health_status": "healthy",
            "tools_count": 3,
            "status": 1,
            "created_at": "2026-05-03T00:00:00Z"
        }
    ],
    "pagination": { ... }
}
```

### POST /services
注册新的 MCP 服务。

**Request Body:**
```json
{
    "name": "exa-search",
    "display_name": "Exa Web Search",
    "description": "Web search powered by Exa",
    "transport_type": "streamable-http",
    "config": {
        "url": "https://mcp.exa.ai/mcp",
        "headers": {
            "Authorization": "Bearer exa-xxxxx"
        }
    },
    "auth_type": "bearer",
    "auth_config": {
        "token": "exa-xxxxx"
    },
    "tags": ["search", "web"]
}
```

**auth_type 可选值:**

| auth_type | 说明 | headers 自动生成规则 |
|-----------|------|---------------------|
| none | 无需认证 | `{}` |
| api_key | API Key 认证 | `{"X-API-Key": "<用户输入>"}` |
| bearer | Bearer Token 认证 | `{"Authorization": "Bearer <用户输入>"}` |
| custom | 自定义请求头 | `{"<用户输入Key>": "<用户输入Value>"}` |

> 认证信息会自动写入 `config.headers`，由后端 Transport Adapter 在每次 HTTP 请求时携带。

**config 格式按 transport_type 不同:**

stdio:
```json
{
    "command": "npx",
    "args": ["-y", "@anthropic/mcp-server-fetch"],
    "env": {}
}
```

sse:
```json
{
    "url": "http://localhost:3000/sse",
    "headers": {}
}
```

streamable-http:
```json
{
    "url": "http://localhost:3000/mcp",
    "headers": {
        "Authorization": "Bearer exa-xxxxx"
    }
}
```

websocket:
```json
{
    "url": "wss://remote-server.com/mcp"
}
```

passive-ws (被动连接):
```json
{
}
```
> passive-ws 无需提供 URL，NewMCP 自动生成 WSS 接入点。返回结果中包含生成的接入 URL。

**Response (passive-ws):** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "remote-calculator",
        "transport_type": "passive-ws",
        "passive_url": "wss://api.newmcp.pro/mcp/passive/?token=eyJhbGciOiJFUzI1NiIs...",
        "passive_connected": false,
        ...
    }
}
```

> 将 `passive_url` 复制到外部 MCP 服务配置中，外部服务连接后 NewMCP 自动发现工具。

**Response:** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "exa-search",
        ...
    }
}
```

### GET /services/:id
获取单个服务详情。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "exa-search",
        "display_name": "Exa Web Search",
        "description": "...",
        "transport_type": "streamable-http",
        "config": { ... },
        "auth_type": "bearer",
        "health_status": "healthy",
        "last_health_check": "2026-05-03T12:00:00Z",
        "tools_cache": [
            {
                "name": "web_search",
                "description": "Search the web using Exa",
                "inputSchema": { ... }
            }
        ],
        "tools_updated_at": "2026-05-03T12:00:00Z",
        "server_info": {
            "name": "exa-mcp-server",
            "version": "1.0.0"
        },
        "protocol_version": "2025-03-26",
        "tags": ["search", "web"],
        "status": 1,
        "created_at": "2026-05-03T00:00:00Z"
    }
}
```

### PUT /services/:id
更新服务配置。

### DELETE /services/:id
删除服务（软删除）。

### POST /services/:id/test
测试服务连接。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "connected": true,
        "server_info": {
            "name": "exa-mcp-server",
            "version": "1.0.0"
        },
        "tools_count": 3,
        "latency_ms": 156
    }
}
```

### POST /services/:id/refresh-tools
手动刷新工具目录。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "tools_count": 3,
        "tools": [ ... ]
    }
}
```

### GET /services/:id/tools
获取服务的工具列表。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": [
        {
            "name": "web_search",
            "description": "Search the web using Exa",
            "inputSchema": {
                "type": "object",
                "properties": { ... },
                "required": ["query"]
            }
        }
    ]
}
```

### GET /services/:id/health
获取服务健康状态。

---

## 5. MCP 分组接口

### GET /groups
获取分组列表。

### POST /groups
创建新分组。

**Request Body:**
```json
{
    "name": "robot-control",
    "display_name": "机器人控制",
    "description": "海陆空机器人统一控制分组",
    "visibility": "private",
    "endpoint_auth": "api_key"
}
```

### GET /groups/check-name
检查分组标识是否重复。

**Query Parameters:**
| 参数 | 类型 | 说明 |
|------|------|------|
| name | string | 要检查的分组标识（必填） |
| exclude_id | int | 排除的分组 ID，用于编辑场景（可选） |

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "exists": false
    }
}
```

### GET /groups/:id
获取分组详情（含服务列表和聚合工具列表）。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "robot-control",
        "display_name": "机器人控制",
        "endpoint_url": "http://localhost:3000/mcp/group/robot-control",
        "services": [
            {
                "id": 1,
                "name": "sea-bot",
                "enabled": true,
                "tools_count": 5
            },
            {
                "id": 2,
                "name": "air-drone",
                "enabled": true,
                "tools_count": 3
            }
        ],
        "tools_count": 8,
        "status": 1
    }
}
```

### PUT /groups/:id
更新分组（可修改分组标识、显示名称、描述、暴露模式等）。

**Request Body:**
```json
{
    "name": "robot-control-v2",
    "display_name": "机器人控制 V2",
    "description": "更新后的描述",
    "expose_mode": "direct"
}
```

> 修改分组标识（name）后会同步更新端点 URL 路径。

### DELETE /groups/:id
删除分组。

### POST /groups/:id/services
向分组添加服务。

**Request Body:**
```json
{
    "service_ids": [1, 2, 3]
}
```

### DELETE /groups/:id/services/:serviceId
从分组移除服务。

### GET /groups/:id/tools
获取分组聚合工具列表。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": [
        {
            "service_id": 1,
            "name": "sea_bot__navigate",
            "original_name": "navigate",
            "service_name": "sea-bot",
            "description": "Navigate to coordinates",
            "enabled": true,
            "name_override": "",
            "inputSchema": { ... }
        },
        {
            "service_id": 2,
            "name": "air_drone__takeoff",
            "original_name": "takeoff",
            "service_name": "air-drone",
            "description": "Take off the drone",
            "enabled": true,
            "inputSchema": { ... }
        }
    ]
}
```

### PUT /groups/:id/tools/:toolName
更新工具配置（启用/禁用/重命名）。

**Request Body:**
```json
{
    "service_id": 1,
    "enabled": false,
    "name_override": "custom_name",
    "description_override": "自定义描述"
}
```

### PUT /groups/:id/tools/batch
批量更新工具启用/禁用状态。

**Request Body:**
```json
{
    "tools": [
        {
            "service_id": 1,
            "tool_name": "navigate",
            "enabled": false
        },
        {
            "service_id": 2,
            "tool_name": "takeoff",
            "enabled": true
        }
    ]
}
```

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "success"
}
```

### POST /groups/:id/refresh
刷新分组内所有服务的工具目录。

### GET /groups/:id/endpoint
获取分组的 MCP 端点信息和连接配置。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "streamable_http_url": "http://localhost:3000/mcp/group/robot",
        "websocket_url": "ws://localhost:3000/mcp/ws/group/robot",
        "auth_type": "api_key",
        "connection_config": {
            "type": "streamable-http",
            "url": "http://localhost:3000/mcp/group/robot",
            "headers": {
                "X-API-Key": "sk-xxxxxxxxxxxx"
            }
        },
        "mcp_client_config": {
            "mcpServers": {
                "robot-control": {
                    "type": "streamable-http",
                    "url": "http://localhost:3000/mcp/group/robot",
                    "headers": {
                        "X-API-Key": "sk-xxxxxxxxxxxx"
                    }
                }
            }
        }
    }
}
```

---

## 6. 云端主动连接接口

### GET /connections
获取云端主动连接列表。

### POST /connections
添加新的云端主动连接。

**Request Body (小智云):**
```json
{
    "name": "客厅小智 Agent",
    "cloud_type": "xiaozhi",
    "wss_url": "wss://api.xiaozhi.me/mcp/?token=eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9...",
    "api_key_id": 1,
    "auto_connect": true
}
```

**Request Body (自定义 WSS):**
```json
{
    "name": "我的云平台",
    "cloud_type": "custom",
    "wss_url": "wss://my-cloud.example.com/mcp/?token=xxx",
    "cloud_config": {
        "headers": {}
    },
    "api_key_id": 2,
    "auto_connect": true
}
```

**支持的 cloud_type:**
| cloud_type | 说明 | 特殊处理 |
|------------|------|----------|
| xiaozhi | 小智云平台 | 自动解析 JWT 获取 Agent ID 和过期时间 |
| custom | 自定义 WSS 端点 | 无特殊处理 |
| ssh | SSH 远程连接 | 配置主机/端口/认证方式 |

**Request Body (SSH):**
```json
{
    "name": "远程服务器",
    "cloud_type": "ssh",
    "cloud_config": {
        "host": "192.168.1.100",
        "port": 22,
        "user": "ubuntu",
        "auth_type": "key",
        "private_key": "-----BEGIN OPENSSH PRIVATE KEY-----\n..."
    },
    "api_key_id": 1,
    "auto_connect": true
}
```

**Response:** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "客厅小智 Agent",
        "cloud_type": "xiaozhi",
        "wss_url": "wss://api.xiaozhi.me/mcp/?token=eyJ...",
        "remote_id": "104304",
        "token_expires_at": "2027-05-03T00:00:00Z",
        "api_key_id": 1,
        "connection_status": "connecting",
        "auto_connect": true
    }
}
```

> **说明**: cloud_type 为 xiaozhi 时，创建后自动解析 JWT 获取 remote_id (Agent ID) 和 token_expires_at，并尝试连接。
> **API Key 绑定**: `api_key_id` 关联一个已有 API Key，该 Key 的 `permissions.groups` 决定此连接可暴露的 MCP 分组和工具范围。

### GET /connections/:id
获取连接详情（含连接状态）。

### PUT /connections/:id
更新连接配置（如更换 URL、切换分组）。

### DELETE /connections/:id
删除连接（同时断开 WSS 连接）。

### POST /connections/:id/connect
手动触发连接。**同步操作**：等待 WebSocket 握手完成后才返回响应（超时 15 秒）。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "connection_status": "connected"
    }
}
```

**错误响应:**
| HTTP 状态码 | message | 说明 |
|-------------|---------|------|
| 400 | 连接超时 | WebSocket 握手超过 15 秒 |
| 400 | ... | WebSocket 连接失败（具体错误信息） |

### POST /connections/:id/disconnect
手动断开连接。**同步操作**：连接关闭并更新数据库状态后返回。

### PUT /connections/:id/bind-apikey
更换绑定的 API Key。

**Request Body:**
```json
{
    "api_key_id": 2
}
```

> 更换 API Key 后自动重新向远端平台注册工具列表（按新 Key 的权限范围）。

---

## 7. 视觉配置接口

### GET /vision
获取当前用户的视觉配置列表。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": [
        {
            "id": 1,
            "name": "OpenAI Vision",
            "provider": "openai",
            "model_name": "gpt-4o",
            "auto_register": false,
            "registered_service_id": null,
            "status": 1,
            "created_at": "2026-05-15T00:00:00Z"
        }
    ]
}
```

### POST /vision
创建视觉配置。创建后默认未启用（`auto_register=false`），需手动调用 enable 接口启用。

**Request Body:**
```json
{
    "name": "OpenAI Vision",
    "description": "OpenAI 视觉模型",
    "provider": "openai",
    "model_name": "gpt-4o",
    "endpoint_url": "https://api.openai.com/v1",
    "api_key": "sk-xxx",
    "system_prompt": "You are a helpful vision assistant.",
    "max_tokens": 4096
}
```

**支持的 provider:**
| Provider | 说明 | 默认 model_name |
|----------|------|-----------------|
| openai | OpenAI API | gpt-4o |
| glm | 智谱 GLM | glm-4v |
| qwen | 通义千问 | qwen-vl-max |
| ollama | Ollama 本地模型 | llava |
| custom | 自定义 OpenAI 兼容端点 | - |

**Response:** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "OpenAI Vision",
        ...
    }
}
```

### GET /vision/:id
获取视觉配置详情（含工具名称/描述）。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "OpenAI Vision",
        "description": "OpenAI 视觉模型",
        "provider": "openai",
        "model_name": "gpt-4o",
        "endpoint_url": "https://api.openai.com/v1",
        "api_key": "sk-***",
        "system_prompt": "You are a helpful vision assistant.",
        "max_tokens": 4096,
        "analyze_image_name": "vision.analyze_image",
        "analyze_image_desc": "Analyze image content and identify the objects, text, and scenes it contains. Best for: extracting structured info, detecting items, or reading text. Returns: a detailed breakdown of recognized elements.",
        "describe_scene_name": "vision.describe_scene",
        "describe_scene_desc": "Describe the scene and overall content of an image in natural language. Best for: getting a high-level summary of what is happening. Returns: a natural-language description of the scene.",
        "extra_config": "{}",
        "auto_register": true,
        "registered_service_id": 5,
        "status": 1,
        "created_at": "2026-05-15T00:00:00Z",
        "updated_at": "2026-05-15T00:00:00Z"
    }
}
```

### PUT /vision/:id
更新视觉配置。所有字段可选，仅传需要更新的字段。如已启用，自动同步更新关联虚拟 McpService 的 tools_cache。

**Request Body:**
```json
{
    "name": "OpenAI Vision V2",
    "model_name": "gpt-4o-mini",
    "analyze_image_name": "custom_analyze",
    "analyze_image_desc": "自定义分析描述"
}
```

### DELETE /vision/:id
删除视觉配置。自动禁用并清理关联的虚拟 McpService、McpGroupService、McpGroupTool。

### POST /vision/test
测试视觉模型 API 连通性。发送一个 1x1 测试像素图片验证端点、密钥和模型是否可用。

**Request Body:**
```json
{
    "endpoint_url": "https://api.openai.com/v1",
    "api_key": "sk-xxx",
    "model_name": "gpt-4o",
    "system_prompt": "Describe this image."
}
```

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "connected": true,
        "model": "gpt-4o",
        "test_result": "This is a very small, nearly empty image...",
        "latency_ms": 1200
    }
}
```

**错误响应:**
| HTTP 状态码 | message | 说明 |
|-------------|---------|------|
| 400 | 连接失败 | API 端点不可达 |
| 401 | 认证失败 | API Key 无效 |
| 400 | 模型不存在 | model_name 错误 |

### POST /vision/:id/enable
启用视觉配置。创建虚拟 McpService（`transport_type="virtual"`, `source="vision"`），注册到 VirtualToolRegistry，生成包含 2 个工具（analyze_image、describe_scene）的 tools_cache。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "视觉配置已启用",
    "data": {
        "service_id": 5,
        "service_name": "vision_1",
        "tools": ["vision.analyze_image", "vision.describe_scene"]
    }
}
```

### POST /vision/:id/disable
禁用视觉配置。从 VirtualToolRegistry 注销，删除关联的 McpService、McpGroupService、McpGroupTool 记录。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "视觉配置已禁用"
}
```

---

## 8. 摄像头接口

### GET /cameras
获取当前用户的摄像头列表。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": [
        {
            "id": 1,
            "name": "前门摄像头",
            "vision_config_id": 1,
            "vision_config_name": "OpenAI Vision",
            "auto_register": false,
            "registered_service_id": null,
            "streaming": false,
            "status": 1,
            "created_at": "2026-05-15T00:00:00Z"
        }
    ]
}
```

### POST /cameras
创建摄像头。`vision_config_id` 为必填字段，指定关联的视觉配置用于 `camera.analyze` 工具。

**Request Body:**
```json
{
    "name": "前门摄像头",
    "description": "前门监控",
    "vision_config_id": 1
}
```

**Response:** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "前门摄像头",
        ...
    }
}
```

### GET /cameras/:id
获取摄像头详情（含工具名称/描述和流状态）。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "前门摄像头",
        "description": "前门监控",
        "vision_config_id": 1,
        "vision_config_name": "OpenAI Vision",
        "capture_name": "camera.capture",
        "capture_desc": "Capture a single still frame from the live camera feed and return it as an image. Best for: taking snapshots or capturing the current view. Returns: the captured frame as an image.",
        "analyze_name": "camera.analyze",
        "analyze_desc": "Capture the current camera frame and run visual analysis on it. Best for: detecting objects, people, or events in the live feed. Returns: the analysis result for the current frame.",
        "extra_config": "{}",
        "auto_register": true,
        "registered_service_id": 6,
        "streaming": true,
        "status": 1,
        "created_at": "2026-05-15T00:00:00Z",
        "updated_at": "2026-05-15T00:00:00Z"
    }
}
```

### PUT /cameras/:id
更新摄像头配置。所有字段可选，仅传需要更新的字段。如已启用，自动同步更新关联虚拟 McpService 的 tools_cache。

**Request Body:**
```json
{
    "name": "后门摄像头",
    "vision_config_id": 2,
    "capture_name": "custom_capture",
    "capture_desc": "自定义截取描述"
}
```

### DELETE /cameras/:id
删除摄像头。自动禁用、停止流、清理关联的虚拟 McpService、McpGroupService、McpGroupTool。

### POST /cameras/:id/enable
启用摄像头。创建虚拟 McpService（`transport_type="virtual"`, `source="camera"`），注册到 VirtualToolRegistry，生成包含 2 个工具（capture、analyze）的 tools_cache。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "摄像头已启用",
    "data": {
        "service_id": 6,
        "service_name": "camera_1",
        "tools": ["camera.capture", "camera.analyze"]
    }
}
```

### POST /cameras/:id/disable
禁用摄像头。从 VirtualToolRegistry 注销，停止流连接，删除关联的 McpService、McpGroupService、McpGroupTool 记录。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "摄像头已禁用"
}
```

### WebSocket GET /cameras/:id/stream
摄像头帧推流端点。浏览器通过此 WebSocket 连接推送摄像头实时画面。

**认证方式:** 由于浏览器 WebSocket API 不支持自定义 HTTP 头，认证通过 URL query 参数传递：

```
ws://localhost:3000/api/v1/cameras/1/stream?token=<JWT_TOKEN>
```

> 此端点位于 auth 路由组之外，由 handler 内部通过 `middleware.ParseToken()` 校验 query 中的 JWT token。

**协议:**
- 浏览器连接后，定时（默认 2 秒间隔）通过 canvas 截取 JPEG 帧，以二进制消息发送
- 后端通过 `CameraStreamManager` 缓存最新帧，供 MCP 工具 `camera.capture` 和 `camera.analyze` 调用
- 连接关闭后自动清理缓存

**连接示例 (浏览器端):**
```javascript
const token = localStorage.getItem('token');
const ws = new WebSocket(`ws://localhost:3000/api/v1/cameras/${cameraId}/stream?token=${encodeURIComponent(token)}`);

// canvas 截取 JPEG 帧并发送
canvas.toBlob(blob => {
    ws.send(blob);
}, 'image/jpeg', 0.8);
```

> **帧缓存**: 后端仅缓存每个摄像头的最新一帧，不存储历史帧。MCP 客户端调用 `camera.capture` 时返回缓存的最新帧（base64），调用 `camera.analyze` 时获取最新帧并调用关联的 VisionConfig 进行 AI 识别。

---

## 9. API Key 接口

### GET /api-keys
获取 API Key 列表。

### POST /api-keys
创建 API Key。

**Request Body:**
```json
{
    "name": "Claude Code Key",
    "permissions": {
        "groups": ["robot-control", "search"],
        "max_rate": 100
    },
    "expires_at": "2027-01-01T00:00:00Z"
}
```

**Response:** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "Claude Code Key",
        "key": "sk-a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
        "key_prefix": "sk-a1b2c3d4",
        "permissions": { ... },
        "expires_at": "2027-01-01T00:00:00Z"
    }
}
```

> **注意**: 完整的 key 只在创建时返回一次，之后无法再查看。

### DELETE /api-keys/:id
删除（吊销）API Key。

---

## 10. 系统设置接口

### GET /settings/public
获取公开系统设置（无需认证），用于登录页面展示系统品牌信息。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "SystemName": "NewMCP",
        "Footer": ""
    }
}
```

> 仅返回公开设置项（如系统名称、页脚），不包含敏感信息。

### GET /admin/settings
获取所有系统设置（需要 admin 角色）。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": [
        { "key": "SystemName", "value": "NewMCP" },
        { "key": "ServerAddress", "value": "" },
        { "key": "Footer", "value": "" },
        { "key": "RegisterEnabled", "value": "true" },
        { "key": "EmailVerificationEnabled", "value": "false" },
        { "key": "EmailDomainRestrictionEnabled", "value": "false" },
        { "key": "EmailDomainWhitelist", "value": "" },
        { "key": "RateLimitEnabled", "value": "false" },
        { "key": "RateLimitMaxRequests", "value": "60" },
        { "key": "RateLimitWindowMinutes", "value": "1" },
        { "key": "RateLimitGroupConfig", "value": "{}" },
        { "key": "SMTPServer", "value": "" },
        { "key": "SMTPPort", "value": "465" },
        { "key": "SMTPAccount", "value": "" },
        { "key": "SMTPToken", "value": "***" },
        { "key": "SMTPFrom", "value": "" },
        { "key": "SMTPSSLEnabled", "value": "true" }
    ]
}
```

> 敏感字段（如 `SMTPToken`）返回 `"***"` 掩码。

### PUT /admin/settings
更新单个系统设置（需要 admin 角色）。

**Request Body:**
```json
{
    "key": "SystemName",
    "value": "My MCP Platform"
}
```

**Response:** `200 OK`
```json
{
    "success": true,
    "message": ""
}
```

> 设置立即生效（写入数据库 + 更新内存缓存）。敏感字段传入 `"***"` 时跳过更新（不改变原值）。

**设置项说明:**

| 分类 | Key | 类型 | 说明 |
|------|-----|------|------|
| 通用 | `SystemName` | string | 系统名称，显示在登录页和标题栏 |
| 通用 | `ServerAddress` | string | 服务器地址 |
| 通用 | `Footer` | string | 页脚内容 |
| 认证 | `RegisterEnabled` | bool | 是否允许用户注册 |
| 认证 | `EmailVerificationEnabled` | bool | 是否开启邮箱验证 |
| 认证 | `EmailDomainRestrictionEnabled` | bool | 是否开启邮箱域名限制 |
| 认证 | `EmailDomainWhitelist` | string | 允许的邮箱域名（逗号分隔） |
| 限流 | `RateLimitEnabled` | bool | 是否启用速率限制 |
| 限流 | `RateLimitMaxRequests` | int | 时间窗口内最大请求数 |
| 限流 | `RateLimitWindowMinutes` | int | 时间窗口（分钟） |
| 限流 | `RateLimitGroupConfig` | string | 分组级限流配置 JSON，如 `{"vip":{"max":120,"window":1}}` |
| SMTP | `SMTPServer` | string | SMTP 服务器地址 |
| SMTP | `SMTPPort` | int | SMTP 端口 |
| SMTP | `SMTPAccount` | string | SMTP 账号 |
| SMTP | `SMTPToken` | string | SMTP 授权码（敏感） |
| SMTP | `SMTPFrom` | string | 发件人地址 |
| SMTP | `SMTPSSLEnabled` | bool | 是否启用 SSL |

---

## 11. 管理员接口

> 以下接口需要 admin 角色。

### GET /admin/users
获取所有用户列表。

### PUT /admin/users/:id
更新用户信息（状态、角色等）。

### GET /admin/stats
获取平台统计数据。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "users_count": 10,
        "services_count": 25,
        "groups_count": 8,
        "devices_count": 15,
        "calls_today": 1234,
        "calls_success_rate": 0.98,
        "avg_latency_ms": 156
    }
}
```

### GET /admin/logs
获取调用日志。

**Query Parameters:**
| 参数 | 类型 | 说明 |
|------|------|------|
| user_id | int | 按用户过滤 |
| service_id | int | 按服务过滤 |
| group_id | int | 按分组过滤 |
| tool_name | string | 按工具名过滤 |
| status | string | success/error |
| start_date | string | 开始日期 |
| end_date | string | 结束日期 |
| page | int | 页码 |
| page_size | int | 每页数量 |

---

## 11. MCP 协议端点

这些端点遵循 MCP 协议规范，不使用上述 REST 响应格式。

### 双模式说明

每个分组通过 `expose_mode` 配置选择工具暴露模式：

| 模式 | tools/list 返回 | tools/call 行为 |
|------|-----------------|-----------------|
| direct | 所有聚合工具（带 `serviceName__toolName` 前缀） | 直接路由到上游服务 |
| smart | 固定 3 个元工具: `mcp.search`, `mcp.describe`, `mcp.execute` | 先解析元工具，再路由 |

### Smart 模式元工具 Schema

**mcp.search:**
```json
{
    "name": "mcp.search",
    "description": "搜索可用的 MCP 服务和工具。支持按关键字、分组名、服务名搜索。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "搜索关键字"},
            "scope": {"type": "string", "enum": ["mcp", "tool", "all"], "default": "mcp", "description": "搜索范围"},
            "group": {"type": "string", "description": "限定分组"},
            "limit": {"type": "number", "default": 10, "maximum": 50}
        },
        "required": ["query"]
    }
}
```

**mcp.describe:**
```json
{
    "name": "mcp.describe",
    "description": "查看指定 MCP 服务的工具列表，或指定工具的完整参数 Schema。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "targets": {"type": "array", "items": {"type": "string"}, "description": "服务名或 serviceName.toolName"},
            "include_schema": {"type": "boolean", "default": true}
        },
        "required": ["targets"]
    }
}
```

**mcp.execute:**
```json
{
    "name": "mcp.execute",
    "description": "执行指定的 MCP 工具。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "tool_id": {"type": "string", "description": "格式: 服务名.工具名"},
            "arguments": {"type": "object", "description": "工具参数"},
            "timeout_ms": {"type": "number", "default": 30000}
        },
        "required": ["tool_id"]
    }
}
```

### POST /mcp
主网关端点 (Streamable HTTP)。聚合 API Key 绑定的所有分组，**固定 Direct 模式**，去重后暴露全部工具（`serviceName__toolName` 前缀）。

**Headers:**
```
Content-Type: application/json
Accept: application/json, text/event-stream
MCP-Protocol-Version: 2025-03-26
X-API-Key: <key>
```

**Request Body (JSON-RPC):**
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
}
```

**Response:** 返回 API Key 绑定分组的全部工具（去重后），工具名格式 `serviceName__toolName`。

### POST /smart/mcp
Smart 网关端点 (Streamable HTTP)。聚合 API Key 绑定的所有分组，**固定 Smart 模式**，仅暴露 3 个元工具。

**Headers:**
```
Content-Type: application/json
Accept: application/json, text/event-stream
MCP-Protocol-Version: 2025-03-26
X-API-Key: <key>
```

**Request Body (JSON-RPC):**
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {}
}
```

**Response:** 固定返回 3 个元工具 (`mcp.search`, `mcp.describe`, `mcp.execute`)，搜索范围覆盖该 API Key 所有绑定分组。

### POST /mcp/group/{slug}
分组端点 (Streamable HTTP)。按分组的 `expose_mode` 决定模式（端点驱动）：`direct` 返回聚合工具，`smart` 返回元工具。

### GET /mcp/group/{slug}
SSE 流 (服务端推送)。

### WebSocket /mcp/ws
WebSocket MCP 传输。聚合 API Key 所有分组，**固定 Direct 模式**（同 POST /mcp）。

### WebSocket /smart/mcp/ws
Smart 模式 WebSocket MCP 传输。聚合 API Key 所有分组，**固定 Smart 模式**。

### WebSocket /mcp/ws/group/{slug}
分组 WebSocket MCP 传输。按分组的 `expose_mode` 决定模式（端点驱动）。

### WebSocket /mcp/passive/
被动接入端点。外部 MCP Server 连入注册工具。

**连接参数:**
```
wss://api.newmcp.pro/mcp/passive/?token=<PASSIVE_JWT>
```

> 此 URL 由创建 `passive-ws` 类型服务时自动生成。外部 MCP Server 连入后，NewMCP 作为 MCP Client 发起 `initialize` → `tools/list`，自动发现并缓存工具。
