# NewMCP API 接口文档

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

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

## 2. 认证接口

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

## 3. MCP 服务接口

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
        "headers": {}
    },
    "auth_type": "bearer",
    "auth_config": {
        "token": "exa-xxxxx"
    },
    "tags": ["search", "web"]
}
```

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
    "headers": {}
}
```

websocket:
```json
{
    "url": "wss://remote-server.com/mcp",
    "headers": {}
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

## 4. MCP 分组接口

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
    "endpoint_slug": "robot",
    "visibility": "private",
    "endpoint_auth": "api_key"
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
        "endpoint_slug": "robot",
        "endpoint_url": "http://localhost:3000/mcp/group/robot",
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
更新分组。

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
            "name": "sea_bot__navigate",
            "original_name": "navigate",
            "service_name": "sea-bot",
            "description": "Navigate to coordinates",
            "enabled": true,
            "name_override": "",
            "inputSchema": { ... }
        },
        {
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
    "enabled": false,
    "name_override": "custom_name",
    "description_override": "自定义描述"
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
                "X-API-Key": "nm-xxxxxxxxxxxx"
            }
        },
        "mcp_client_config": {
            "mcpServers": {
                "robot-control": {
                    "type": "streamable-http",
                    "url": "http://localhost:3000/mcp/group/robot",
                    "headers": {
                        "X-API-Key": "nm-xxxxxxxxxxxx"
                    }
                }
            }
        }
    }
}
```

---

## 5. 云端主动连接接口

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
手动触发连接。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "connection_status": "connected",
        "tools_registered": 5
    }
}
```

### POST /connections/:id/disconnect
手动断开连接。

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

## 6. 视觉配置接口

### GET /vision/configs
获取视觉配置列表。

### POST /vision/configs
创建视觉配置。

**Request Body:**
```json
{
    "name": "OpenAI Vision",
    "provider": "openai",
    "model_name": "gpt-4o",
    "endpoint_url": "https://api.openai.com/v1",
    "api_key": "sk-xxx",
    "system_prompt": "Describe this image in detail.",
    "max_tokens": 4096,
    "auto_register": true
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

### PUT /vision/configs/:id
更新视觉配置。

### DELETE /vision/configs/:id
删除视觉配置。

### POST /vision/configs/:id/test
测试视觉模型。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "connected": true,
        "model": "gpt-4o",
        "test_result": "A test image showing a white cat sitting on a table.",
        "latency_ms": 1200
    }
}
```

---

## 7. 摄像头接口

### GET /cameras
获取摄像头列表。

### POST /cameras
添加摄像头。

**Request Body:**
```json
{
    "name": "前门摄像头",
    "source_type": "rtsp",
    "source_url": "rtsp://admin:password@192.168.1.100:554/stream1",
    "fps": 1.0,
    "resolution_w": 640,
    "resolution_h": 480,
    "vision_config_id": 1,
    "auto_register": true
}
```

### GET /cameras/:id
获取摄像头详情。

### PUT /cameras/:id
更新摄像头配置。

### DELETE /cameras/:id
删除摄像头。

### POST /cameras/:id/capture
手动触发截图。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "image_url": "/api/v1/cameras/1/latest",
        "captured_at": "2026-05-03T12:00:00Z",
        "resolution": "640x480",
        "analysis": "An outdoor scene with a white door..."
    }
}
```

### GET /cameras/:id/latest
获取最新捕获的帧。

---

## 8. API Key 接口

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
        "key": "nm-a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
        "key_prefix": "nm-a1b2c3d4",
        "permissions": { ... },
        "expires_at": "2027-01-01T00:00:00Z"
    }
}
```

> **注意**: 完整的 key 只在创建时返回一次，之后无法再查看。

### DELETE /api-keys/:id
删除（吊销）API Key。

---

## 9. 管理员接口

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

## 10. MCP 协议端点

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
主网关端点 (Streamable HTTP)。聚合 API Key 绑定的所有分组，**固定 Smart 模式**（因跨分组工具量大，只能通过元工具渐进发现）。

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
WebSocket MCP 传输。聚合 API Key 所有分组，**固定 Smart 模式**。

### WebSocket /mcp/ws/group/{slug}
分组 WebSocket MCP 传输。按分组的 `expose_mode` 决定模式（端点驱动）。

### WebSocket /mcp/passive/
被动接入端点。外部 MCP Server 连入注册工具。

**连接参数:**
```
wss://api.newmcp.pro/mcp/passive/?token=<PASSIVE_JWT>
```

> 此 URL 由创建 `passive-ws` 类型服务时自动生成。外部 MCP Server 连入后，NewMCP 作为 MCP Client 发起 `initialize` → `tools/list`，自动发现并缓存工具。
