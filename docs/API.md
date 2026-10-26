# NewMCP API 接口文档

> 版本: V1.1 | 更新日期: 2026-10-17 (新增 Gemini 原生视频协议；正式 MCP `2026-07-28`、订阅与实验 Events)

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
    "email": "string (可选；开启邮箱验证时必填)",
    "verification_code": "string (开启邮箱验证时必填，6位)"
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

> 当管理员开启「邮箱验证」(`EmailVerificationEnabled`) 时，`email` 与 `verification_code` 为必填；验证码通过 `GET /verification` 获取，10 分钟内有效。

### GET /verification
发送邮箱验证码（用于注册）。公开接口，按客户端 IP 限流（默认每 60 秒最多 1 次）。

**Query:**

| 参数 | 说明 |
|------|------|
| `email` | 接收验证码的邮箱地址（必填） |

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "success"
}
```

> 校验顺序：邮箱格式 → 邮箱域名白名单 → 邮箱未被占用 → 生成 6 位验证码并发送。任一环节失败返回 `success: false` 并附带 `message`；触发限流时返回 `429`。

### POST /auth/login
用户登录。

**Request Body:**
```json
{
    "username": "string (必填，可为用户名或邮箱)",
    "password": "string (必填)"
}
```

> `username` 字段同时接受用户名或邮箱地址，后端按 `username` 或 `email` 匹配账号。

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
    "display_name": "string (可选)",
    "email": "new@example.com",
    "avatar_url": "https://...",
    "email_verification_code": "string (更换/绑定邮箱且已配置 SMTP 时必填，6 位)"
}
```

> 绑定或更换邮箱时：
> - 新邮箱需通过域名白名单校验，且未被其他账号占用；
> - **若已配置 SMTP**（`SMTPServer` + `SMTPAccount`），必须先通过 `GET /auth/profile/email-code` 向新邮箱获取验证码，并在 `email_verification_code` 中提交，校验通过后才更新；
> - **若未配置 SMTP**，可直接绑定/更换，无需验证码。

### GET /auth/profile/email-code
向指定新邮箱发送绑定/换绑验证码（需登录）。复用注册验证码的限流策略（按 IP，默认每 60 秒最多 1 次）。

**Query:**

| 参数 | 说明 |
|------|------|
| `email` | 欲绑定/更换的新邮箱地址（必填） |

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "success"
}
```

> 校验顺序：邮箱格式 → 域名白名单 → 邮箱未被占用 → 生成 6 位验证码并发送。任一环节失败返回 `success: false` 并附带 `message`；触发限流返回 `429`。

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
| query_param | URL 参数认证 | `auth_config.query_param_name` 指定参数名；单秘钥值位于 `config.url` |

> Header 认证写入 `config.headers`；URL 参数认证按指定参数名写入每次上游请求。单秘钥 URL 参数值在详情接口中掩码回显。

**多秘钥（可选，仅 streamable-http / sse）:** 传 `key_mode` + `auth_keys` 直接建池,
此时认证值不进 `config.headers`,由秘钥池按策略逐请求注入:

```json
{
    "transport_type": "streamable-http",
    "config": { "url": "https://mcp.exa.ai/mcp" },
    "auth_type": "bearer",
    "auth_config": { "header_name": "Authorization" },
    "key_mode": "polling",
    "auth_keys": ["key-a", "key-b", "key-c"]
}
```

> `key_mode`: `random` | `polling`。上限 100 把、单把 ≤8KB。上游 401/403 自动禁用对应
> key 并落系统日志;调用日志记录所用序号 `key_index`。详见 `docs/MULTI-KEY.md`。

URL 参数多秘钥示例（池内只填参数值，共用基础 URL）：

```json
{
  "transport_type": "streamable-http",
  "config": { "url": "https://example.com/mcp/?region=cn" },
  "auth_type": "query_param",
  "auth_config": { "query_param_name": "tavilyApiKey" },
  "key_mode": "polling",
  "auth_keys": ["key-a", "key-b"]
}
```

创建时若 URL 已含目标参数，该参数值会先收编为第 1 把秘钥，再与 `auth_keys` 去重；保存的基础 URL 不再包含该参数。

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
    "url": "wss://remote-server.com/mcp",
    "headers": {}
}
```
> 主动连接支持 `ws://` / `wss://`，上游须通过 WebSocket 文本帧传递原始 MCP JSON-RPC 消息。

passive-ws (被动 WebSocket 接入):
```json
{
}
```
> 无需提供 URL、命令或上游认证配置。NewMCP 根据系统设置 `ServerAddress` 生成接入地址：HTTPS 对应 WSS，HTTP 对应 WS。创建前须配置有效的服务器地址。

**Response (passive-ws):** `201 Created`
```json
{
    "success": true,
    "data": {
        "id": 1,
        "name": "remote-calculator",
        "transport_type": "passive-ws",
        "passive_url": "wss://api.newmcp.pro/mcp/passive/?token=1.<random-secret>",
        "passive_connected": false,
        ...
    }
}
```

> 将 `passive_url` 配置到本地 MCP 服务的 WebSocket 桥接程序。每个接入点对应一个 MCP 服务；握手和工具发现成功后，平台自动同步目录。多个本地 MCP 服务应分别创建接入点。
>
> `token` 格式为 `<serviceID>.<random-secret>`，secret 为独立生成的 256 位随机凭证，在数据库中加密保存，与登录 JWT、网关 API Key 无关。完整接入地址即访问凭证，仅向服务所有者返回。

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

### POST /services/:id/passive-token/reset
重置被动 WebSocket 服务的接入凭证。完整路径为 `/api/v1/services/:id/passive-token/reset`，使用服务所有者的用户认证，无需请求体。

**Response:** `200 OK`，`data` 为更新后的 `ServiceDetail`，包含新的 `passive_url` 与 `passive_connected: false`。旧地址立即失效，当前连接断开，工具目录保留；本地桥接程序须改用新地址重新连接。非 `passive-ws` 服务或无效服务器地址返回 `400`。

### POST /services/:id/test
测试服务连接。

`passive-ws` 复用已接入的实时连接执行 MCP ping，不向接入地址反向拨号。尚未接入时返回 `connected: false` 和 `error: "等待服务接入"`；已接入时返回工具数量、协商协议版本与服务器信息。

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

被动 WebSocket 服务通过当前连接重新执行 `tools/list` 并同步目录；离线时无法刷新。上游发送 `notifications/tools/list_changed` 时也会自动重新同步。

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

### GET /services/:id/keys
获取多秘钥池（值恒为掩码，永不回明文）。仅 HTTP 类传输的自有服务可用。

**Response:** `200 OK`
```json
{
    "success": true,
    "data": {
        "key_mode": "polling",
        "header_name": "Authorization",
        "auth_type": "bearer",
        "transport_type": "streamable-http",
        "total": 2,
        "enabled": 1,
        "keys": [
            { "id": 11, "sort_order": 1, "masked_value": "exa-8f2…c91a", "status": 3, "disabled_reason": "upstream 401/403", "disabled_at": "2026-09-01T10:00:00Z" },
            { "id": 12, "sort_order": 2, "masked_value": "exa-3ad…77b0", "status": 1 }
        ]
    }
}
```

> `status`: 1=启用 2=手动禁用 3=自动禁用(上游 401/403)。`sort_order` 即调用日志
> `key_index`。
> URL 参数模式返回 `auth_type: "query_param"`、`query_param_name`，此时 `header_name` 为空。

### PUT /services/:id/keys
批量更新秘钥（追加 / 替换）。

**Request Body:**
```json
{ "mode": "append", "values": ["key-d", "key-e"] }
```

> `mode`: `append`(对池内已有值去重,保留既有行与状态) | `replace`(整池替换,序号从 1
> 重排,状态清零)。返回 `{added, skipped}`(skipped=追加时重复跳过的数量)。

### PUT /services/:id/keys/:keyID
启/禁单把秘钥。**Request Body:** `{ "status": 1 }`(1=启用,2=禁用;重新启用自动禁用
的 key 会清空 `disabled_reason`/`disabled_at`)。

### DELETE /services/:id/keys/:keyID
删除单把秘钥。

### POST /services/:id/keys/batch
批量操作。**Request Body:** `{ "action": "enable_all" }` 或 `{ "action": "delete_disabled" }`。

### PUT /services/:id/keys/config
模式与策略切换。

**Request Body:**
```json
{ "key_mode": "random", "header_name": "Authorization" }
```

> `key_mode`: `single`(多→单:首选启用 key 写回 `config.headers` 并清空池,替换语义)
> | `random` | `polling`。单→多时 `config.headers` 里须已有目标头的值(收编为首把秘钥);
> `header_name` 缺省按 auth_type 推导(api_key→`X-API-Key`、bearer→`Authorization`,
> custom 必填);多秘钥模式下不可更换注入头。切换即失效运行时选择器并踢会话重建。
> URL 参数认证传 `{"key_mode":"polling","query_param_name":"tavilyApiKey"}`；
> 现有 URL 须恰含一个非空目标参数，切换时收编为首把秘钥并从基础 URL 移除。
> 多→单写回 URL 参数。服务即使原先标为 `none`，也可指定该字段转换。

### 市场条目级秘钥池(admin)

`/admin/marketplace/:id/keys*` 与服务级六端点同构(路径前缀换成
`/admin/marketplace/:id/keys`,AdminAuth 把关,请求/响应 DTO 完全一致)。差异:

- 仅 `category=instant` 的 sse/streamable-http 条目可用;
- 一份池对**全部安装该条目的用户**全局轮换,坏 key 一次禁光;调用日志 `key_index`
  与服务级同口径;
- 单→多时收编的是**平台上游模板**(`config_template`)里的认证头,多→单写回模板;
  `header_name` 缺省按模板 headers 反推(`Authorization`/`X-API-Key`/首个自定义头);
- URL 参数模式同样使用 `query_param_name`，从模板 URL 收编首把秘钥，多→单写回模板 URL；
- 池/模式变更与删除条目会踢掉该条目全部引用会话。

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
获取连接详情（含连接状态）。普通用户仅可查看自己的配置；管理员和超级管理员也可只读查看其他用户的配置。

响应包含 `user_id` 和 `read_only`。管理员查看其他用户时，`read_only=true`，另返回所属用户 `username`、绑定令牌名称 `api_key_name`，WSS URL 仅保留协议、主机和端口，其余敏感部分显示为 `****`（例如 `wss://api.xiaozhi.me/****?****`）。SSH 等配置仅保留主机、端口、用户名和认证类型，其余字段及原始错误信息遮罩，避免凭据泄漏。配置所有者的详情和编辑行为保持原样。

此只读权限不开放修改、删除、连接/断开或密钥重绑；这些操作仍严格校验配置归属。

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
        "analyze_image_desc": "Analyze an image with a vision model. Covers all image understanding: identify objects, people and text, describe the scene and overall content, extract structured info, or answer any custom question. Pass the prompt parameter to steer the analysis, e.g. describe the scene, transcribe all text, or list defects. Returns: the analysis result as text.",
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
    "analyze_image_desc": "自定义分析描述"
}
```

> 工具名称固定不可改（`analyze_image` / `upload_image`），仅描述可自定义。

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
启用视觉配置。创建虚拟 McpService（`transport_type="virtual"`, `source="vision"`），注册到 VirtualToolRegistry，生成包含 analyze_image 与内置 upload_image 的 tools_cache（describe_scene 已彻底下线，其能力由 analyze_image 的 prompt 参数覆盖；旧名调用返回 unknown vision tool 错误）。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "视觉配置已启用",
    "data": {
        "service_id": 5,
        "service_name": "vision_1",
        "tools": ["analyze_image", "upload_image"]
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

### POST /vision/upload

使用登录 JWT 上传单张本机图片，供网页及管理员「图片管理」页使用；无需执行 `upload_image` 返回的命令。请求使用 `multipart/form-data`，文件字段为 `image`。服务端复用 local/S3 存储、图片格式校验、大小限制、当前用户配额和去重规则。

成功返回 `201`，`data` 包含 `url`、`expires_at`、`mime`、`size`、`key`、`backend`、`deduped`。`url` 是可复制的签名图片地址（`/u/:sid?s=...`），`expires_at=null` 表示永久保存。将地址作为 `edit_image.images` 中的一项、`generate_video_from_image.image` 或视觉工具的 `image_url` 使用；图生图、图生视频支持 PNG/JPEG/WebP，上传端点另外支持 GIF。

图片归属当前登录账号，内置图生工具使用该账号的 API Key 校验归属；管理员的列表用户筛选不会改变上传归属，也不会代其他用户上传。应用的 `ServerAddress` 应设置为调用方可访问的地址。

---

## 8. 摄像头接口

本节描述当前已实现的摄像头 API。Agent 的 Direct／Smart 调用、推流密钥、拟新增的采集间隔、YOLO 工具与摄像头事件见 [摄像头与 Agent 调用指南](./CAMERA-AGENT-CALLING.md)。新指南中的 YOLO 和监控接口目前为设计约定，尚未上线。

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
创建摄像头。`vision_config_id` 为必填字段，指定关联的视觉配置用于 `analyze_frame` 工具。

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
    "message": "created",
    "data": {
        "id": 1,
        "name": "前门摄像头",
        "description": "前门监控",
        "vision_config_id": 1,
        "auto_register": false,
        "registered_service_id": null,
        "streaming": false,
        "has_stream_key": false
    }
}
```

上例展示响应核心字段；实际 data 还包含工具描述、默认描述、状态和时间等 CameraDetail 字段。

### GET /cameras/:id
获取摄像头详情（含工具描述、默认描述、推流密钥是否存在和流状态）。推流密钥原文只通过专用 stream-key 接口向属主提供。

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
        "capture_desc": "Capture a single still frame from the live camera feed and return it as an image. Best for: taking snapshots or capturing the current view. Returns: the captured frame as an image.",
        "analyze_desc": "Capture the current camera frame and run visual analysis on it. Best for: detecting objects, people, or events in the live feed. Returns: the analysis result for the current frame.",
        "extra_config": "{}",
        "auto_register": true,
        "registered_service_id": 6,
        "streaming": true,
        "has_stream_key": true,
        "stream_key_expires_at": "",
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
    "capture_desc": "自定义截取描述"
}
```

> 工具名称固定不可改（`capture_frame` / `analyze_frame`），仅描述可自定义。

### DELETE /cameras/:id
删除摄像头。自动禁用、停止流、清理关联的虚拟 McpService、McpGroupService、McpGroupTool。

### POST /cameras/:id/enable
启用摄像头。创建虚拟 McpService（`transport_type="virtual"`, `source="camera"`），注册到 VirtualToolRegistry，生成包含 2 个工具（capture_frame、analyze_frame）的 tools_cache。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "success"
}
```

启用响应不包含服务详情。随后 GET `/cameras/:id` 读取 registered_service_id；服务名按 `camera_<摄像头ID>` 生成，具体工具从 MCP tools/list 发现。

### POST /cameras/:id/disable
禁用摄像头。从 VirtualToolRegistry 注销，删除关联的 McpService、McpGroupService、McpGroupTool 记录，并拒绝新的推流连接。当前 Disable 实现不会主动关闭已升级的 WebSocket；需要立即停止已有推流时，关闭采集页面或撤销推流密钥。

**Response:** `200 OK`
```json
{
    "success": true,
    "message": "success"
}
```

### GET /cameras/:id/stream-key
登录用户查询自己摄像头的密钥与推流页面链接。未生成密钥时 data.has_key=false；此接口不自动生成密钥。

**Response:** `200 OK`（有密钥时）
```json
{
    "success": true,
    "message": "success",
    "data": {
        "stream_key": "<该摄像头的推流密钥>",
        "stream_url": "https://newmcp.example.com/camera-live/1?k=<推流密钥>",
        "expires_at": "",
        "has_key": true
    }
}
```

### POST /cameras/:id/stream-key
生成或重新生成推流密钥，旧密钥立即失效，已有推流连接会断开。请求 `expires_in` 为有效秒数，0 或空 body 表示永久有效，负数返回 400。响应与 GET stream-key 相同；有限期 expires_at 返回时间字符串。

```json
{"expires_in":3600}
```

### DELETE /cameras/:id/stream-key
撤销密钥并关闭已有推流连接，返回 `{"success":true,"message":"success"}`。以上三种管理接口均需要登录 JWT，并校验摄像头属主；密钥和完整 stream_url 都应作为凭证保存。

### WebSocket GET /cameras/:id/stream
摄像头帧推流端点。浏览器通过此 WebSocket 连接推送摄像头实时画面。

**认证方式:** 由于浏览器 WebSocket API 不支持自定义 HTTP 头，认证通过 URL query 参数传递：

```
ws://localhost:3000/api/v1/cameras/1/stream?k=<stream_key>
```

> 此端点位于 auth 路由组之外，handler 将 k 与摄像头 StreamKey 做恒定时间比较并校验有效期。不是 JWT 认证；只有已启用的摄像头允许推流，每路同时只允许一个连接。缺少／无效／过期密钥返回 401，未启用返回 403，已有推流返回 409。线上使用 wss。

**协议:**
- 浏览器连接后，当前固定每 2 秒通过 canvas 截取 JPEG 帧，以二进制消息发送；后台可配置间隔属于拟新增能力
- 后端通过 `CameraStreamManager` 缓存最新帧，供 MCP 工具 `capture_frame` 和 `analyze_frame` 调用
- 连接关闭后自动清理缓存

**连接示例 (浏览器端):**
```javascript
// streamKey 由属主从专用 stream-key 接口取得，或来自推流页面的 k 参数。
const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
const ws = new WebSocket(`${protocol}//${location.host}/api/v1/cameras/${cameraId}/stream?k=${encodeURIComponent(streamKey)}`);

// canvas 截取 JPEG 帧并发送
canvas.toBlob(blob => {
    if (blob && ws.readyState === WebSocket.OPEN) ws.send(blob);
}, 'image/jpeg', 0.8);
```

> **帧缓存**: 后端仅缓存每个摄像头的最新一帧，不存储历史帧。MCP 客户端调用 `capture_frame` 时返回缓存的最新帧（base64），调用 `analyze_frame` 时获取最新帧并调用关联的 VisionConfig 进行 AI 识别。

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
        "ServerAddress": "http://localhost:3000",
        "Footer": "",
        "RegisterEnabled": "true",
        "EmailVerificationEnabled": "false",
        "SMTPConfigured": "false"
    }
}
```

> 仅返回公开设置项（系统名称、页脚、注册/邮箱验证开关等），不包含敏感信息。`SMTPConfigured` 为派生布尔值（`SMTPServer` 与 `SMTPAccount` 均已配置时为 `true`），用于前端判断绑定/更换邮箱是否需要验证码。

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
        { "key": "SMTPPort", "value": "587" },
        { "key": "SMTPAccount", "value": "" },
        { "key": "SMTPToken", "value": "***" },
        { "key": "SMTPFrom", "value": "" },
        { "key": "SMTPSSLEnabled", "value": "false" }
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
| 视频 | `VideoGenMaxBytes` | int | 单文件上限，默认 268435456 字节（256 MiB） |
| 文件储存 | `UploadRetentionHours` | int | 上传及参考图片保留小时数，默认 24；0 表示永久保存，不自动过期或清理 |
| 图片生成 | `ImageGenRetentionHours` | int | 已保存生成图片保留小时数，默认 24；0 表示永久保存，与上传图片独立 |
| 图片生成 | `ImageGenHistoryDays` | int | 任务结束后的历史保留天数，默认 30；0 表示永久保留；仍有有效生成图片的记录保留 |
| 图片生成 | `MaxImagesPerUser` | int | 每用户已保存生成图片数量上限，默认 0（不限）；过期图片不占用配额 |
| 视频 | `VideoGenRetentionHours` | int | 保存成功后的文件保留小时数，默认 24；0 表示永久保存 |
| 视频 | `VideoGenHistoryDays` | int | 停止跟踪后的历史保留天数，默认 30；0 表示永久保留；未过期视频的记录保留 |
| 视频 | `MaxVideosPerUser` | int | 每用户保存数量上限，默认 0（不限） |
| 认证 | `RegisterEnabled` | bool | 是否允许用户注册 |
| 认证 | `EmailVerificationEnabled` | bool | 是否开启邮箱验证 |
| 认证 | `EmailDomainRestrictionEnabled` | bool | 是否开启邮箱域名限制 |
| 认证 | `EmailDomainWhitelist` | string | 允许的邮箱域名（逗号分隔） |
| 限流 | `RateLimitEnabled` | bool | 是否启用速率限制 |
| 限流 | `RateLimitMaxRequests` | int | 时间窗口内最大请求数 |
| 限流 | `RateLimitWindowMinutes` | int | 时间窗口（分钟） |
| 限流 | `RateLimitGroupConfig` | string | 分组级限流配置 JSON，如 `{"vip":{"max":120,"window":1}}` |
| 市场 Stdio | `SharedStdioIdleTimeoutMinutes` | int | 平台托管 stdio 进程空闲自动释放时长（分钟），0=关闭自动释放；修改立即生效 |
| 市场 Stdio | `SharedStdioMaxConcurrency` | int | 共享市场 stdio 条目并发调用上限（每条目独立、全体用户合计），超出立即返回 `-32000 "服务名: 当前服务负载较高，请稍后重试"`；0=不限；修改立即生效 |
| SMTP | `SMTPServer` | string | SMTP 服务器地址 |
| SMTP | `SMTPPort` | int | SMTP 端口（默认 587，STARTTLS） |
| SMTP | `SMTPAccount` | string | SMTP 账号 |
| SMTP | `SMTPToken` | string | SMTP 访问凭证（敏感） |
| SMTP | `SMTPFrom` | string | 发件人邮箱 |
| SMTP | `SMTPSSLEnabled` | bool | 是否启用 SSL（默认关闭；端口 465 时应开启） |

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

这些端点使用 MCP JSON-RPC，不使用上述 REST 响应格式，也不位于 `/api/v1` 下。客户端网关使用 API Key 认证（`X-API-Key` 或 `Authorization: Bearer <API_KEY>`）。本节描述代码支持的接口；部署中的实例需运行包含这些改动的构建。

支持正式 `2026-07-28` 和 legacy `2025-11-25`、`2025-06-18`、`2025-03-26`、`2024-11-05`。MCP 版本按日期命名；消息中的 `jsonrpc: "2.0"` 不代表“MCP 2.0”。[完整 curl 示例](./MCP-PROTOCOL.md#114-完整-curl-示例)

### 双模式说明

每个分组通过 `expose_mode` 配置选择工具暴露模式：

| 模式 | tools/list 返回 | tools/call 行为 |
|------|-----------------|-----------------|
| direct | 所有聚合工具（带 `serviceName__toolName` 前缀） | 直接路由到上游服务 |
| smart | 固定 5 个元工具: `mcp.search`, `mcp.describe`, `mcp.execute`, `mcp.execute_batch`, `mcp.read` | 先解析元工具，再路由 |

### Smart 模式元工具 Schema

**mcp.search:**
```json
{
    "name": "mcp.search",
    "description": "搜索可用的 MCP 服务、工具、资源和提示。关键词匹配名称、服务名与描述；目录以英文为主，非英文任务建议双语关键词（原词旁附英文翻译）。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "搜索关键字，可选；省略则浏览全部条目"},
            "scope": {"type": "string", "enum": ["mcp", "tool", "resource", "prompt", "all"], "default": "all", "description": "搜索范围（resource 含资源模板；不确定时保持 all）"},
            "group": {"type": "string", "description": "限定分组"},
            "limit": {"type": "number", "default": 20, "maximum": 100},
            "offset": {"type": "number", "minimum": 0, "default": 0}
        },
        "required": []
    }
}
```

**mcp.describe:**
```json
{
    "name": "mcp.describe",
    "description": "查看指定 MCP 服务的完整清单（工具、资源、资源模板、提示），或指定工具的完整参数 Schema。",
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

**mcp.execute_batch:**
```json
{
    "name": "mcp.execute_batch",
    "description": "并发执行多个相互独立的工具调用（最多 10 项、网关内并发上限 5），逐项返回结果。同工具批量调用（如批量控制开关/设备）用 tool_id + arguments_list 短形态;混合不同工具用 calls;某项参数依赖另一项返回值、或同一目标需按序操作（先设置再读回）时请逐个用 mcp.execute。",
    "inputSchema": {
        "type": "object",
        "properties": {
            "tool_id": {"type": "string", "description": "同工具扇出: 服务名.工具名,配合 arguments_list"},
            "arguments_list": {
                "type": "array",
                "minItems": 1,
                "maxItems": 10,
                "items": {"type": "object", "description": "每次调用的参数对象"},
                "description": "与 tool_id 配对,同工具每调用一次给一个参数对象"
            },
            "calls": {
                "type": "array",
                "minItems": 1,
                "maxItems": 10,
                "items": {
                    "type": "object",
                    "properties": {
                        "tool_id": {"type": "string", "description": "格式: 服务名.工具名"},
                        "arguments": {"type": "object", "description": "工具参数"}
                    },
                    "required": ["tool_id"]
                },
                "description": "混合不同工具时使用,与 tool_id/arguments_list 互斥"
            },
            "timeout_ms": {"type": "number", "default": 30000, "description": "整批统一超时毫秒"}
        },
        "required": []
    }
}
```

### POST /mcp
主网关端点 (Streamable HTTP)。聚合 API Key 绑定的所有分组，**固定 Direct 模式**，去重后暴露全部工具（`serviceName__toolName` 前缀）。

**现代请求 Headers:**
```text
Content-Type: application/json
Accept: application/json, text/event-stream
MCP-Protocol-Version: 2026-07-28
Mcp-Method: tools/list
X-API-Key: <key>
```

**Request Body:**
```json
{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list",
    "params": {
        "_meta": {
            "io.modelcontextprotocol/protocolVersion": "2026-07-28",
            "io.modelcontextprotocol/clientCapabilities": {},
            "io.modelcontextprotocol/clientInfo": {"name": "api-example", "version": "1"}
        }
    }
}
```

**Response:** `.result.tools` 返回 API Key 绑定分组的全部工具（去重后），工具名格式 `serviceName__toolName`；现代结果包含 `resultType: "complete"`、`ttlMs: 0`、`cacheScope: "private"` 和 `_meta["io.modelcontextprotocol/serverInfo"]`。

每个现代请求都必须有 `_meta.protocolVersion` 和 `_meta.clientCapabilities` 的完整命名空间键；`clientInfo` 为建议字段。`MCP-Protocol-Version`、`Mcp-Method` 必须匹配请求体；`tools/call` 和 `prompts/get` 还需要匹配 `params.name` 的 `Mcp-Name`，`resources/read` 的 `Mcp-Name` 匹配 `params.uri`。名称不能直接表示为安全 ASCII 时使用 `=?base64?<UTF-8 Base64>?=`。工具的 `x-mcp-header` 参数注解对应 `Mcp-Param-*` 镜像头。

现代请求无需 initialize 或 session。旧客户端继续走 `initialize` → `notifications/initialized`，只协商 legacy 版本，HTTP 初始化响应以 `Mcp-Session-Id` 返回会话，后续请求携带同一会话头。

### POST /smart/mcp
Smart 网关端点 (Streamable HTTP)。聚合 API Key 绑定的所有分组，**固定 Smart 模式**。请求元数据与 headers 同 `/mcp`。`tools/list` 返回 5 个核心元工具 (`mcp.search`, `mcp.describe`, `mcp.execute`, `mcp.execute_batch`, `mcp.read`)，获准使用语义搜索时增加 `mcp.smart_search`。搜索范围覆盖该 API Key 所有绑定分组。

### POST /mcp/group/{slug}
分组端点 (Streamable HTTP)。按分组的 `expose_mode` 决定模式（端点驱动）：`direct` 返回聚合工具，`smart` 返回元工具。

### `server/discover`

向上述任一 POST 端点发送现代 `server/discover` 请求，参数只需标准 `_meta`。响应包括 `supportedVersions`、该端点的 `capabilities`、`resultType` 与 `_meta["io.modelcontextprotocol/serverInfo"]`；可以在首次调用前发现能力。实验事件能力位于 `capabilities.extensions["io.newmcp/events"] = {"experimental": true}`，这不是正式核心能力。

### `subscriptions/listen`（POST SSE）

向上述 POST 端点发送 `subscriptions/listen`，参数包含标准 `_meta` 和 `notifications`：

| 过滤项 | 类型 | 通知 |
|--------|------|------|
| `toolsListChanged` | boolean | `notifications/tools/list_changed` |
| `resourcesListChanged` | boolean | `notifications/resources/list_changed` |
| `promptsListChanged` | boolean | `notifications/prompts/list_changed` |
| `resourceSubscriptions` | string[] | 所列网关 URI 的 `notifications/resources/updated` |

HTTP 响应为 `text/event-stream`。首条 `notifications/subscriptions/acknowledged` 返回实际支持的过滤项子集，每条通知通过 `_meta["io.modelcontextprotocol/subscriptionId"]` 关联原请求 ID。Smart 模式不声明原生资源/提示能力，不在 ack 中承诺相应过滤项；其元工具目录不跟随上游工具列表变化。HTTP 客户端关闭流即取消；服务器主动结束时发送成功 completion 再关闭。现代标准订阅不支持 `Last-Event-ID` 回放。

### GET /mcp、GET /smart/mcp、GET /mcp/group/{slug}

旧版 SSE 通知流，仅供已 initialize 的 legacy 客户端使用。需要 API Key、`MCP-Protocol-Version`（legacy 日期）、`Mcp-Session-Id`（initialize 响应头中的会话）和 `Accept: text/event-stream`。旧版指定资源仍通过 `resources/subscribe` / `resources/unsubscribe` 管理。现代客户端使用 POST `subscriptions/listen`。

### WebSocket /mcp/ws
客户端 Direct 网关，通过 WebSocket 文本帧传递 JSON-RPC 请求、响应及订阅通知。连接握手使用 API Key 认证；现代消息包含标准 `_meta`，旧客户端可用 initialize。HTTP POST 镜像头不适用于 WebSocket 消息。

### WebSocket /smart/mcp/ws
客户端 Smart 网关，能力和授权范围与 `/smart/mcp` 对应。

### WebSocket /mcp/ws/group/{slug}
客户端分组网关，模式由分组 `expose_mode` 决定。`subscriptions/listen` 和 `events/stream` 的通知通过连接发送；`notifications/cancelled` 的 `requestId` 指向原流请求，连接关闭时也会清理所属流。

### 实验 `events/*`

现代客户端需在 `_meta["io.modelcontextprotocol/clientCapabilities"].extensions` 中声明 `"io.newmcp/events": {}`；未显式启用时返回 HTTP `400` / `-32021`。

此实现参考尚在 Draft 阶段的 [Events proposal](https://github.com/modelcontextprotocol/experimental-ext-triggers-events/blob/main/docs/design-sketch-proposal.md)。事件只覆盖 NewMCP 网关目录与 MCP 资源更新，不代理任意上游业务事件。授权基于当前 API Key、端点分组、服务及条目启停；Smart 客户端也可在授权服务范围内使用。

| 事件 | 参数 |
|------|------|
| `mcp.tools.list_changed` | `arguments.service` 可选 |
| `mcp.resources.list_changed` | `arguments.service` 可选 |
| `mcp.prompts.list_changed` | `arguments.service` 可选 |
| `mcp.resources.updated` | `arguments.uri` 必填，`newmcp://service/upstream-uri`，可自动推导 service |

| 方法 | 参数/结果 |
|------|-----------|
| `events/list` | 返回事件类型与 input/payload Schema、delivery 模式 |
| `events/poll` | `name`、`arguments`、可选 `cursor`、`maxAgeMs`、`maxEvents`；结果含 `events`、`cursor`、`truncated`、`hasMore`、`nextPollMs` |
| `events/stream` | `name`、`arguments`、可选 `cursor`、`maxAgeMs`；HTTP SSE 或 WebSocket active/event/heartbeat 通知 |
| `events/subscribe` | `name`、`arguments`、`delivery: {mode:"webhook",url,secret}`、可选 `cursor`、`maxAgeMs`、`ttlMs`；返回服务端 `id`、`refreshBefore`、`cursor`、`truncated` |
| `events/unsubscribe` | 原 `name`、`arguments`、`delivery: {url}`；按当前身份和原订阅键清理 |

`events/subscribe` 仅用于 webhook；poll 直接请求，push 直接打开 stream。`cursor: null`（或省略）从现在开始，非空 cursor 请求进程内有界回放。历史缺失时 `truncated` / `gap` 表明缺口，重启会丢失历史和订阅。客户端须保存 cursor、按 eventId 去重并重建订阅。

Webhook 要求公网 HTTPS callback 和客户端生成的 `whsec_` + Base64(24–64 随机字节) secret。激活前发送已签名 `verification` nonce，接收端验签后回 `2xx` JSON `{challenge:"<原nonce>"}`。每条投递都含 Standard Webhooks HMAC-SHA256 头及 `X-MCP-Subscription-Id`；签名覆盖 ID、Unix 秒时间戳和原始 body bytes。接收端在处理前验签、检查五分钟时间窗口、去重，保存或转发成功再返回 `2xx`。失败使用有限退避重试，`410` / `413` 不重试该事件。

订阅状态在内存中，有限 TTL 最长五分钟；不授予无期限订阅。`refreshBefore` 是实际到期时间，客户端须在此前用相同键重发 subscribe 续期；重启后重新订阅。唯一键为 `(认证身份, url, name, canonical arguments)`，返回 ID 只用于路由，unsubscribe 不接受 ID 替代原键。签名格式、控制载荷及完整命令见[协议文档](./MCP-PROTOCOL.md#113-webhook-生命周期与验签)。

### 现代 HTTP 协议错误

| HTTP | JSON-RPC code | 场景 |
|------|---------------|------|
| `400` | `-32602` | 必需现代元数据缺失、参数无效 |
| `400` | `-32020` | 缺失或不匹配的 HTTP 镜像头 |
| `400` | `-32021` | 客户端能力不足，`data.requiredCapabilities` |
| `400` | `-32022` | 版本不支持，`data.requested` / `data.supported` |
| `404` | `-32601` | 未实现的方法 |

实验事件错误使用 `-32011` NotFound、`-32012` Forbidden、`-32013` ResourceExhausted、`-32014` Unsupported、`-32015` CallbackEndpointError；callback 错误只返回分类（如 `timeout`、`challenge_failed`），不返回接收端响应正文。

### WebSocket /mcp/passive/
被动 WebSocket 接入端点。本地或外部 MCP Server 主动连入，NewMCP 作为 MCP Client 发现和调用工具。

**连接参数:**
```
wss://api.newmcp.pro/mcp/passive/?token=<serviceID>.<random-secret>
```

> URL 来自服务详情的 `passive_url`；`ServerAddress` 为 HTTP 时使用 `ws://`。只接受独立接入凭证，缺失或错误凭证返回 `401`，服务或所属用户禁用时返回 `403`。
>
> 通过 WebSocket 文本帧双向传递原始 MCP JSON-RPC 消息。官方 SDK 完成版本协商（兼容 `initialize` → `notifications/initialized`）和工具发现；接入成功后复用长连接执行 `tools/call`。同一服务的新连接成功初始化后替换旧连接，握手失败不会踢掉旧连接。停用、删除服务或重置凭证会关闭当前连接；重连后自动同步工具目录。

---

## 12. 商业化接口(计费 / 额度 / 兑换 / 市场定价)

> 商业化只对**市场来源服务**(`mcp_services.source='marketplace'`)按"每个工具调用收一个固定价"计费;**用户自有服务**(`source='user'`)免费。两者共用同一条分组调用路径(`/mcp`、`/smart/mcp`、`/mcp/group/:slug`),仅按 `service.source` 决定是否计费。详见 [COMMERCIALIZATION.md](./COMMERCIALIZATION.md)。
>
> **额度单位**:统一整数 `quota`,1 展示货币单位 = `QuotaPerUnit` quota(默认 500000,即 1 quota = 0.000002 元)。前端展示:quota ÷ QuotaPerUnit = 元。

### 12.1 计费行为(MCP 网关侧)

| 方法 | 端点 | 计费 |
|------|------|------|
| POST | `/mcp`、`/smart/mcp`、`/mcp/group/:slug` | 调用 `source=user` 服务**免费**;调用 `source=marketplace` 服务按 3 级定价**按次计费**,余额不足拒绝本次调用(不禁用 Key) |

**计费口径**:仅 `tools/call`(Direct)与 `mcp.execute` / `mcp.execute_batch`(Smart,批量**逐项**计费:每项独立预扣/确认/退款,幂等键带批内序号)扣费;`initialize`/`tools/list`/`mcp.search`/`mcp.describe` 免费。

**余额不足错误体**(MCP JSON-RPC,不禁用 Key,充值后立即可用):
```json
{ "jsonrpc":"2.0", "id":1, "error":{ "code":-32603, "message":"用户额度不足,剩余额度不足,请充值或兑换", "data":{ "code":"QUOTA_INSUFFICIENT" } } }
```

### 12.2 钱包与用量(用户侧,UserAuth)

#### GET /wallet
我的额度概览。
```json
{ "success": true, "data": { "quota": 5000000, "used_quota": 250000, "request_count": 42, "total_topup": 5000000, "group": "default" } }
```

#### GET /wallet/billing
消费明细分页(基于 `mcp_call_logs` 计费列,仅计费相关行,排除 skipped)。支持 `page` / `page_size`。
```json
{ "success": true, "data": [ { "id": 1, "tool_name": "search", "billing_status": "charged", "billing_type": "per_call", "unit_price": 0.05, "quota_consumed": 25000, "price_scope": "service", "marketplace_item_id": 3, "created_at": "..." } ], "pagination": {...} }
```

#### GET /wallet/usage/stats
用量统计(今日/本周/累计消费 quota)。
```json
{ "success": true, "data": { "consumed_today": 50000, "consumed_week": 200000, "consumed_total": 250000 } }
```

#### POST /redemptions/redeem
兑换码兑换。`RedemptionEnabled=false` 时 403。
- 请求:`{ "code": "..." }`
- 响应:`{ "success": true, "data": { "quota": 5000000 } }`(入账额度)
- 业务错误 400:已使用 / 已过期 / 已禁用 / 无效。

### 12.3 服务市场(用户侧,UserAuth + 公开浏览)

#### GET /marketplace `(公开)`
浏览市场(含价格)。列表项含 `billing_type` / `price_per_call`。支持 `page` / `page_size` / `category` / `keyword`。

#### GET /marketplace/:id `(公开)`
市场项详情(含价格)。

#### POST /marketplace/:id/add `(UserAuth)`
**引用式安装**:把市场项添加为用户的引用服务(`source='marketplace'`,config 留空、不复制凭证、复制 tools_cache)。
- 响应:`{ "success": true, "data": { "service_id": 12, "name": "exa" } }`
- 去重:同一用户对同一市场项仅一份引用,重复添加返回已有引用。
- 添加后可加入分组,经 `/mcp/group/:slug` 调用,按市场价扣费。

#### POST /marketplace/:id/review `(UserAuth)`
市场项评分/评价(1-5 分)。

### 12.4 管理员侧(AdminAuth)

#### POST /admin/users/:id/quota
**管理员调额**(D13)。`value` 单位 quota。
- 请求:`{ "mode": "add|sub|set", "value": 500000, "remark": "补偿6月故障" }`
- 响应:`{ "success": true, "data": { "new_quota": 5500000 } }`
- 权限:普通管理员不可操作超级管理员或同级管理员(403)。审计写入服务器日志。

#### PUT /admin/marketplace/pricing/batch
**批量设置已上架市场服务价格**(§5.5)。非自用模式逐条校验显式定价。
- 请求:
```json
{ "items": [ { "id": 1, "billing_type": "per_call", "price_per_call": 0.05 }, { "id": 2, "billing_type": "free" } ] }
```
- 响应:`{ "success": true, "data": { "affected": 2 } }`

#### PUT /admin/marketplace/:id/entry-prices `(条目级定价,全量替换)`
**市场项条目级定价**(§5.2/§5.5):对工具/资源/提示逐条设价。载荷为该市场项期望的**完整**条目价列表(全量替换,事务删旧插新);不在载荷中的条目按缺省回退——**工具→服务统一价,资源/提示→免费**。
- 请求:
```json
{ "prices": [
    { "kind": "tool",     "name": "search",          "billing_type": "free",     "price_per_call": 0 },
    { "kind": "tool",     "name": "fetch",           "billing_type": "per_call", "price_per_call": 0.05 },
    { "kind": "resource", "name": "memo://overview", "billing_type": "inherit",  "price_per_call": 0 },
    { "kind": "prompt",   "name": "review",          "billing_type": "per_call", "price_per_call": 0.01 }
] }
```
- `kind`: `tool` / `resource` / `prompt`;`name` 为条目键(工具名 / 资源**上游 URI** / 上游提示名,与计费解析同口径)。
- `billing_type`: `free` / `per_call` / `inherit`(显式继承服务统一价,`price_per_call` 强制归零存储)。工具缺省即继承无需落 `inherit` 行;资源/提示缺省是免费,要继承服务价须显式落 `inherit` 行。
- 校验(400):条目不存在于服务快照(资源模板不可定价——模板读取按展开后的具体 URI 走资源条目价)、`per_call` 须 `price_per_call > 0`、`(kind, name)` 重复、价格负数。
- 响应:`{ "success": true, "data": null }`。保存后价格缓存即时失效。
- 详情暴露:管理端与公开市场详情(`GET /admin/marketplace/:id` / `GET /marketplace/:id`)均返回 `entry_prices` 数组(同上结构),用户侧市场详情页据此在条目右侧展示价格。
- 计费口径:三种 kind 命中条目价即用(工具日志 `price_scope='tool'`,资源/提示 `price_scope='entry'`);未命中或缺省——工具(及显式 `inherit` 的资源/提示)走"服务→全局默认"链,**缺省资源/提示免费**(不回退服务价)。

#### POST /admin/marketplace/clone
**从自有服务克隆上架**(D14,唯一上架入口,已移除手动创建):深拷贝 transport/config/auth/tools,与源服务无关联。克隆保留源凭证并加密落库,前端提示替换为平台凭证。非自用模式须显式定价。前端选择服务后自动回显 `name`/`display_name`/`description`,一般无需修改直接提交。
- 请求:`{ "from_service_id": 5, "name": "exa-market", "display_name": "...", "billing_type": "per_call", "price_per_call": 0.05, "isolated_process": false }`
- 响应:市场项详情(含价格)。
- `name` 全局唯一,重复返回 400(提示修改服务标识)。
- `isolated_process`(D17,仅源服务为 stdio 时生效,其余传输忽略置 false):`false`=共享进程(默认,全部安装用户共用平台一个子进程)/`true`=独占(每安装用户各一进程);上架后可在管理详情页切换。

#### PUT /admin/marketplace/:id `(更新市场项)`
扩展支持 `billing_type` / `price_per_call` / `isolated_process`(指针,可选更新;`isolated_process` 仅 stdio 条目生效,共享↔独占切换会踢掉该条目全部池内会话按新模式重建)。启用状态下非自用模式校验显式定价。`config_template` 平台凭证**加密落库**。管理端详情(GET /admin/marketplace/:id)回传 `config_template` 供编辑:url/command/args 等结构明文,headers/env 的**凭证值替换为首尾掩码**(如 `sk-A...x9z`);保存时掩码值原样传回则回填库内明文(未改动),输入新值则替换。

#### GET /admin/marketplace/:id/process `(stdio 条目进程视图,独占模式服务端分页)`
仅 stdio 条目有意义(其余传输返回空视图)。共享/独占模式返回不同形态,管理详情页 5s 轮询,只读现状不拉起进程:
- 共享(`isolated:false`):`{ "isolated": false, "shared": { "running": true, "pid": ..., "command": "...", "process_count": ..., "memory_rss_bytes": ..., "cpu_percent": ..., "uptime_seconds": ... } }`(未运行时 `shared.running=false`,其余字段缺省)
- 独占(`isolated:true`)**服务端分页**:查询参数 `page`(默认 1)/ `page_size`(默认 18,上限 100)/ `username`(LIKE 匹配用户名或服务名/显示名,匹配在服务端执行——万级安装时一次只回一页,用户名只对当前页反查,不构造全量用户 IN 列表)。响应:
```json
{ "isolated": true,
  "running_instances": 3, "total_processes": 9, "memory_bytes": 524288000, "cpu_percent_total": 12.4,
  "total": 10000, "page": 1, "page_size": 18, "total_pages": 556,
  "instances": [ { "service_id": 12, "user_id": 3, "username": "alice", "name": "mem-market", "status": 1, "stat": { "running": false } } ] }
```
  `instances` 为当前页(含从未连接的安装);`running_instances/total_processes/memory_bytes/cpu_percent_total` 为**全部运行实例**的资源概述(来自会话池现状 + 一次进程树扫描,不随分页/筛选变化,CPU 为各实例之和、多核可超 100%);`total` 为筛选后的安装总数。

#### POST /admin/marketplace/:id/process/control `(stdio 条目进程启停)`
- 请求:`{ "action": "start|stop|restart", "service_id": 0 }`
- 共享模式忽略 `service_id`:`start`=**预热**(不等首个用户调用即拉起平台唯一进程),`stop`/`restart` 操作该进程。
- 独占模式 `service_id` 必填且须为该条目的安装引用行;用户已禁用的引用行拒绝拉起(400)。
- 响应:操作后的最新进程视图(同 GET)。停止后下一个调用会自动重拉进程(与自有 stdio 服务同语义);真停需下架条目。

#### GET /admin/redemptions `(列表)` / POST /admin/redemptions `(批量生成)`
- POST 批量生成:`{ "name": "活动码", "quota": 5000000, "count": 10, "expired_at": 0 }`(`count` 上限 100,`expired_at` 0=永久)。返回含明文 `code` 的条目(仅此一次可见)。
- GET 列表:支持 `page` / `page_size` / `keyword` / `status`。

#### PUT /admin/redemptions/:id `(启停)` / DELETE /admin/redemptions/:id `(删除)`
- PUT:`{ "status": 1 }`(可用)或 `{ "status": 3 }`(禁用);已兑换(2)不可改。

### 12.5 计费相关系统设置(经 PUT /admin/settings 配置)

| Key | 默认 | 说明 |
|-----|------|------|
| `BillingEnabled` | false | 计费总开关 |
| `QuotaPerUnit` | 500000 | 1 货币单位 = 多少 quota |
| `DisplayCurrency` | CNY | 展示货币 |
| `BillingDefaultType` | per_call | 全局默认计费类型(仅自用模式生效) |
| `BillingDefaultPricePerCall` | 0 | 全局默认按次单价 |
| `GroupRatio` | {"default":1,"vip":1,"svip":1} | 分组倍率 JSON |
| `TrustQuota` | 5000000 | 信任额度旁路阈值(默认 10 元) |
| `ChargeAdmin` | false | 是否对管理员计费 |
| `BillingFailOpen` | true | 计费 DB 异常时是否放行(记欠账) |
| `QuotaForNewUser` | 0 | 新用户赠送额度 |
| `QuotaRemindThreshold` | 0 | 低额度邮件提醒阈值(0=不提醒) |
| `LogPayloadEnabled` | true | 是否落 request/response_payload |
| `UserOwnedServicesEnabled` | true | 是否允许用户添加/调用自有服务(false=纯市场模式) |
| `SelfUseModeEnabled` | false | 自用模式可用全局默认;非自用上架须显式定价 |
| `RedemptionEnabled` | true | 是否开放兑换 |

> 公开设置(`GET /settings/public`)暴露:`BillingEnabled` / `DisplayCurrency` / `SelfUseModeEnabled` / `RedemptionEnabled` / `UserOwnedServicesEnabled`(供 `/pricing`、市场上架页按模式适配)。

---

## 13. 视频生成接口

管理接口使用用户 JWT，路径相对于 `/api/v1`。MCP 工具使用 API Key 及分组授权；视频文件通过独立签名链接读取。完整操作流程与兼容性说明见 [视频生成](./VIDEO-GENERATION.md)。

### 13.1 配置 CRUD 与注册

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET / POST | `/videogen` | 当前用户的配置列表 / 创建配置 |
| GET / PUT / DELETE | `/videogen/:id` | 详情 / 部分字段更新 / 删除配置 |
| POST | `/videogen/:id/enable` | 注册虚拟 MCP 服务，重复启用不会创建第二个服务 |
| POST | `/videogen/:id/disable` | 取消注册并清理分组关联 |

创建请求示例（密钥和模型 ID 为占位值）：

```json
{
  "name": "视频生成",
  "protocol": "openai_video",
  "model_name": "your-video-model",
  "endpoint_url": "https://gateway.example.com",
  "api_key": "<UPSTREAM_API_KEY>",
  "operation": "both",
  "image_request_format": "multipart",
  "default_params": {
    "seconds": 4,
    "size": "1280x720"
  },
  "request_timeout_seconds": 30,
  "task_timeout_seconds": 3600
}
```

| 字段 | 类型 | 约束 / 默认 |
| --- | --- | --- |
| `name` / `description` | string | 配置名称必填；说明可选 |
| `protocol` | string | `openai_video`（默认）、`gemini_video`（Gemini 原生 Veo）或 `openrouter_video`；页面显示 OpenAI、Gemini、OpenRouter |
| `model_name` | string | 必填，允许手填；不设厂商模型白名单 |
| `endpoint_url` | string | 可选；留空时 OpenAI 默认 `https://api.openai.com/v1`，Gemini 默认 `https://generativelanguage.googleapis.com/v1beta`，OpenRouter 默认 `https://openrouter.ai/api/v1`；更新时省略字段保留原地址，传空字符串恢复当前协议官方地址。保留代理前缀，Gemini 统一使用 `/v1beta`；OpenRouter 的官方根地址补 `/api/v1` |
| `api_key` | string | 写入字段；更新为空保留原密钥，读取只返回 `has_api_key` |
| `operation` | string | `generate`（文生）、`image`（图生）、`both`，默认 `both` |
| `image_request_format` | string | New API 为 `multipart` / `json`，默认 `multipart`；OpenRouter 图生使用 JSON |
| `default_params` | object | 默认 `{}`，调用参数按图片生成规则深度合并 |
| `request_timeout_seconds` | int | 1–300 秒，默认 30 |
| `task_timeout_seconds` | int | 60–86400 秒，默认 3600；到期暂停跟踪，查询可恢复 |
| `generate_video_desc` / `generate_video_from_image_desc` / `get_video_status_desc` | string | 可编辑工具描述；上传工具描述固定 |

返回详情包含 `auto_register`、`registered_service_id` 和默认工具描述；注册状态由 enable/disable 接口控制。新建配置不自动注册。注册后服务名称为 `videogen_<id>`，`transport_type=virtual`、`source=videogen`；服务编辑和删除应从视频配置页操作。配置变更、取消注册或删除不会取消已有任务，任务继续使用提交时的连接及协议快照。

OpenRouter 配置与参数示例见[视频生成说明](./VIDEO-GENERATION.md#openrouter-视频协议openrouter_video)。同一协议覆盖 `heygen/heygen-video-1`、`minimax/hailuo-3-max`、`google/veo-3.1-lite` 等任意上游支持的模型 ID，不根据模型名称静默改变协议。

Gemini 原生配置选择 `gemini_video`，参考[Gemini 接入说明](./VIDEO-GENERATION.md#gemini-原生视频协议gemini_video)。`seconds` 映射为整数 `parameters.durationSeconds`，`size` 映射为 `parameters.resolution`；`extra_body` 直接承载其他原生驼峰 `parameters` 字段，不嵌套 `instances` / `parameters`，不使用 `metadata`。固定 JSON 单图首帧和单个输出视频，不包含额外参考帧、延长、Vertex AI 或 Interactions API。

### 13.2 模型目录与草稿测试

`POST /videogen/models` 接受连接字段 `endpoint_url`、`api_key`、可选 `protocol`（默认 `openai_video`）及可选 `config_id`。提供 ID 时先载入当前用户的配置，再覆盖请求中的连接字段，API Key 留空保留已保存值；省略 ID 时只需密钥，地址可省略或留空使用当前协议官方地址，不要求填写 `name` 或 `model_name`，也不校验生成参数。OpenAI 使用 `/v1/models`，Gemini 分页查询 `/v1beta/models` 并筛选支持 `predictLongRunning` 的模型，OpenRouter 使用专用 `/api/v1/videos/models`（实际路径保留配置的代理前缀）。统一返回 `id` / `name`，不限制具体模型的时长、分辨率枚举。页面默认用 `video` 搜索，可修改为 `veo`、`hailuo` 或清空；Gemini 目录已按视频能力过滤，默认词匹配全部返回条目。查询目录成功不代表模型具有视频生成权限。

`POST /videogen/test` 同样支持草稿覆盖，另接受 `tool`（默认 `generate_video`）及 `arguments` 对象。已保存配置示例：

```json
{
  "config_id": 1,
  "tool": "generate_video",
  "arguments": {
    "prompt": "山间日出，镜头缓慢向前移动",
    "seconds": 4,
    "wait_seconds": 0,
    "request_id": "preview-001"
  }
}
```

测试响应遵循通用外层 `{success, data}`，`data` 为 `{result, is_error, error?, duration_ms}`；任务位于 `data.result.structuredContent`。草稿测试任务也持久化，未保存配置的 `config_id=0`。测试生成会实际调用上游，可能产生费用。

### 13.3 任务历史与结果

`GET /imagegen/tasks` 和 `GET /videogen/tasks` 支持可选 `status` 查询参数，省略或 `all` 不筛选；筛选与用户归属和 `config_id` 共同生效，先筛选再统计总数和分页。图片可选 `completed/running/partial/failed/canceled/interrupted`；视频可选 `completed/failed/submitting/queued/in_progress/unknown/submission_unknown`，非法值返回 400。“成功”对应 `completed`，视频保存状态不受此筛选定义影响。

图片任务（`/imagegen/tasks`、`/imagegen/tasks/:task_id`）和视频任务的视图均新增 `reference_images` 数组。每项包含 `role`（`reference` 或图片编辑的 `mask`）、`status`（`ready/expired`）、`mime`、`size`、`width`、`height`、`expires_at`，有效时提供可预览/复制的 `url`。链接按任务用户与当前图片保留规则生成，`expires_at=null` 表示永久保存；过期或删除保留元数据且不返回链接，旧记录无法恢复输入时为空数组。新任务保存经校验的图片内容及资产引用，不把 base64 或原始私有参数放进此字段；视频重试更新原记录的参考图。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| GET | `/videogen/tasks` | `page=1`、`page_size=10`（最大 100），可选 `config_id`；返回 `{items,total,page,page_size}` |
| GET | `/videogen/tasks/:task_id` | 当前用户的本地任务详情；恢复已暂停的自动跟踪 |
| POST | `/videogen/tasks/:task_id/retry-download` | 保存失败时重试下载及存储，不重新提交生成 |
| GET | `/videogen/tasks/:task_id/retry-input` | 获取当前用户失败任务的调用参数及 `has_input_snapshot`，不返回连接和凭证 |
| POST | `/videogen/tasks/:task_id/retry` | 使用当前已保存配置重新提交原任务，仅允许失败任务接受新的重试 |
| DELETE | `/videogen/tasks/:task_id` | 删除停止处理的任务及其视频；进行中或被其他处理者领取时返回 409 |

详情含 `task_id`、`config_id`、`config_name`、`source`、`operation`、`protocol`、`model`、`prompt`、`status`、`progress`、`artifact_status`、`tracking_paused`、`poll_after_seconds`、`created_at`、`stopped_at`、`duration_ms`、错误信息及 `videos`。列表与 MCP 状态结果使用相同任务视图。`stopped_at` 为本次停止跟踪时间，处理中为 `null`；`duration_ms` 为任务创建至停止跟踪的总毫秒数，包含排队、生成、保存及重试等待时间。进行中、缺少停止时间的旧记录或无效时间范围返回 `null`，不使用可能因清理而变化的 `updated_at` 计算耗时。不会返回上游 ID、凭证或连接快照。`protocol` 是本次提交使用的协议，旧任务按 `openai_video` 兼容。`status=completed` 表示生成成功，只有 `artifact_status=ready` 且 `videos` 非空才可下载。

生成重试请求为 `{config_id?, request_id, arguments?}`：`request_id` 必填且最长 128 字节，同一次重试重复提交返回原任务的当前状态；改变输入时必须更换该 ID。返回的 `task_id` 与 URL 中的任务 ID 相同，不新增历史记录。`config_id` 默认使用原配置，草稿或原配置已删除时需选择其他属于当前用户的已保存配置。使用所选配置当前的模型、连接和默认参数，更新原记录的配置关联、模型、提示词及参数；保留创建时间、初始来源及原提交幂等映射，`retry_count` 加一。`arguments` 可替换原调用参数，省略时恢复私有输入快照；其中不能填写 `request_id` 或 `wait_seconds`。旧记录没有输入快照时仅恢复提示词，图生须补充参考图。进行中、已生成和 `submission_unknown` 任务不能接受新的重试；已经受理的同一次重试仍可重复查询当前结果。保存失败使用 `retry-download`。重试生成会重新调用上游，可能产生费用。

MCP 生成参数为必填 `prompt`，可选 `seconds`、`size`、`extra_body`、`wait_seconds`、`request_id`；`metadata` 仅在 `openai_video` 工具 schema 中提供，图生另需 `image`。等待上限 30 秒，生成默认 10 秒，状态查询默认 0 秒。进行中或短等待结束返回正常结果及任务 ID。已保存结果同时提供 `structuredContent.videos` 与 `resource_link`。MCP 状态工具校验用户和配置归属，不接受任意上游任务 ID。

OpenRouter 将 `seconds` 转为整数 `duration`；未提供 `seconds` 时可传 `extra_body.duration`，不可同时提供。`extra_body` 可传 `resolution`、`aspect_ratio`、`generate_audio`、`seed` 和 `provider.options` 等扩展。非空 `metadata` 在本地拒绝，提示使用 `extra_body`。图生固定使用 JSON `frame_images` 的首帧结构；URL 经现有图片加载和归属校验，base64/data URL 可直接提交而无需先保存为图片链接。上游查询及下载固定为配置前缀下的 `/videos/{id}`、`/videos/{id}/content?index=0`，不使用返回的任意 URL；`expired` 终态映射为 `failed`。精确字段参考 [OpenRouter 提交接口](https://openrouter.ai/docs/api/api-reference/video-generation/submit-a-video-generation-request)。

相同用户、配置、`request_id` 的相同输入返回已有任务；输入不同拒绝。提交受理情况不明确时记录 `submission_unknown`，禁止自动重新提交。查询临时失败保留最近状态；保存失败仅重试保存。各状态与调用示例见 [MCP 工具说明](./VIDEO-GENERATION.md#mcp-工具)。

提交 HTTP 503 的结构化错误码为 `video_queue_full` 时属于明确拒绝，记录为 `failed`，可通过生成重试接口重新提交原任务；普通 5xx 或响应不完整仍为 `submission_unknown`。启动时会条件修正旧版保存有完整队列拒绝错误且无上游 ID、无租约持有者的误分类记录，不会自动调用上游。已有任务查询返回 503 时保留其最近状态。

错误码识别兼容 New API 将完整上游错误 JSON 放入 `message`、`error.message` 或字符串 `error` 的包装形式，即使外层为通用错误码。完整内层 JSON 中必须存在确切的 `video_queue_full`，不使用文字匹配。重试再次被队列拒绝时，原记录再次显示 `failed`，历史数量保持不变。

### 13.4 视频读取

`GET` / `HEAD /v/:sid?s=<signature>` 位于应用根路径，不带 `/api/v1`。返回 MP4/WebM，支持 Range（206、416）和浏览器拖动播放；附加 `download=1` 时返回 `Content-Disposition: attachment` 和带正确扩展名的文件名，供浏览器直接下载，仍使用相同签名、过期检查和 Range 能力。文件到期返回 410。链接使用独立于图片的签名域，持有有效链接即可读取文件，无需额外用户 JWT。过期检查以任务保存的 `expires_at` 为准。

图片、视频和音频接口中的 `expires_at=null` 表示永久保存。通过系统设置修改 `UploadRetentionHours`、`ImageGenRetentionHours`、`VideoGenRetentionHours` 或 `AudioGenRetentionHours` 时，已有且未清理的对应类型文件统一从修改时刻重新计算期限，原创建时间不变。比如从永久改为 24 小时，已有文件均在修改时刻加 24 小时到期；之后新增文件按自身创建/保存时间加 24 小时到期。改为 0 后清除有效期限，重复保存相同值不会再次续期。设置与已有文件的更新在同一事务中提交，待上传槽位和已清理的文件不会被恢复。永久视频计入每用户数量限制，其任务历史保留至手动删除。

服务端仅将上游凭证发送至配置来源，跨来源下载重定向移除凭证并校验目标地址；参考图需通过现有图片加载及归属校验。New API 的通义万相、豆包图生选择 JSON；multipart 上传实际图片字节，`metadata` 编码为 JSON 字符串，其他复杂扩展参数使用 JSON。OpenRouter 使用 JSON 首帧，内容由固定接口携带凭证流式获取后保存。`extra_body` 不可覆盖模型、提示词、参考图（包括 `frame_images`、`input_references`）、鉴权和本地任务控制字段。

## 14. 图片异步任务

完整配置和三种图片协议见[图片生成](./IMAGE-GENERATION.md)。路径基于 `/api/v1`，管理接口使用用户 JWT。

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| POST | `/imagegen/test` | 支持保存配置或草稿；`tool` 指定生成、编辑或查询，`arguments` 使用对应 MCP schema；生成默认短等 10 秒，网页传 0 |
| GET | `/imagegen/tasks` | 当前用户列表，`config_id` 可选，`status` 可选，`page` 从 1 开始，`page_size` 默认 10，最多 100 |
| GET | `/imagegen/tasks/:task_id` | 本地任务详情、参考图、图片结果及新签名 URL |
| POST | `/imagegen/tasks/:task_id/retry-download` | 仅重试已有生成输出的保存；不调用生成上游 |
| DELETE | `/imagegen/tasks/:task_id` | 删除结束任务的历史及私有输出缓存；活跃任务返回 409，共享图片资产继续按正常保留规则清理 |

`/imagegen/test` 外层保持 `{data:{result,is_error,duration_ms}}`，内部 `result.structuredContent` 为任务视图。排队、生成、保存及短等待结束属于正常结果；Agent 使用 `get_image_status` 继续查询。列表与详情返回 `artifact_status`、`result_count`、`poll_after_seconds`、`can_retry_save`，不暴露密钥、连接快照、内部存储地址或租约。

状态筛选支持 `queued/running/saving/completed/partial/failed/submission_unknown/canceled/interrupted`，省略或 `all` 返回全部。用户与配置筛选先于总数和分页，保存失败重试同样校验用户。跨配置 MCP 查询拒绝；禁用/删除配置不会取消已受理任务。

生成 `arguments` 新增 `wait_seconds`（整数 0–30）与 `request_id`（1–128 字节）。相同用户、配置、ID 和输入返回已有任务，输入不同拒绝；不要以重新调用生成为轮询。`timeout_seconds` 控制后台生成，默认 180 秒，0 不设生成期限，客户端断开不再取消已入库任务。
