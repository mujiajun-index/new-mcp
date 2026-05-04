# NewMCP 开发进度文档

> 最后更新: 2026-05-04 | 代码行数: 4,534 行 Go | 源文件: 51 个

---

## 1. 项目概况

NewMCP 是一个统一的 MCP（Model Context Protocol）网关平台，采用 Go 模块化单体架构。本文档追踪各模块的开发状态、已完成功能与待开发项。

---

## 2. 总体进度

| 模块 | 状态 | 完成度 | 说明 |
|------|------|--------|------|
| 项目基础设施 | ✅ 已完成 | 100% | 入口、配置、工具函数、数据库 |
| 认证系统 | ✅ 已完成 | 100% | 注册/登录/JWT/API Key |
| MCP 服务管理 | ✅ 已完成 | 90% | CRUD 完整，测试/刷新工具需接入传输层 |
| MCP 分组管理 | ✅ 已完成 | 95% | CRUD + 工具聚合完整，刷新需接入传输层 |
| API Key 管理 | ✅ 已完成 | 100% | 创建/列表/删除/权限 |
| 云端连接 | 🔧 部分完成 | 40% | CRUD 完整，实际 WSS 连接未实现 |
| 管理员接口 | ✅ 已完成 | 100% | 用户管理/统计/日志 |
| MCP 协议网关 | 🔧 部分完成 | 65% | Smart 模式完整，Direct 模式完整，传输层不全 |
| 视觉配置 | ❌ 未开始 | 5% | 仅 Model 层 |
| 摄像头管理 | ❌ 未开始 | 5% | 仅 Model 层 |
| 前端界面 | ❌ 未开始 | 0% | 计划 React + Semi Design |

**整体完成度: ~65%**

---

## 3. 详细模块状态

### 3.1 项目基础设施 ✅

| 文件 | 功能 | 状态 |
|------|------|------|
| `cmd/server/main.go` | 应用入口，加载配置、初始化数据库、启动 HTTP 服务 | ✅ |
| `go.mod` | Go 模块定义，依赖管理 | ✅ |
| `Makefile` | 构建/运行/清理命令 | ✅ |
| `.env.example` | 环境变量示例 | ✅ |

**配置与工具函数:**

| 文件 | 功能 | 状态 |
|------|------|------|
| `common/env.go` | 环境变量加载（PORT, DB_TYPE, BASE_URL 等） | ✅ |
| `common/constants.go` | 全局常量（角色、状态、传输类型、暴露模式） | ✅ |
| `common/json.go` | JSON 序列化/反序列化封装 | ✅ |
| `common/crypto.go` | bcrypt 密码哈希 + AES-GCM 加密 | ✅ |
| `common/response.go` | 统一 API 响应格式（Success/Created/Error/PageOf） | ✅ |
| `common/page.go` | 分页参数解析 | ✅ |

### 3.2 数据模型层 ✅

所有 12 个数据库表已创建并通过 GORM AutoMigrate 自动建表：

| 模型文件 | 表名 | 状态 |
|----------|------|------|
| `model/user.go` | users | ✅ 含 CRUD 方法 |
| `model/api_key.go` | api_keys | ✅ 含按 hash 查询 |
| `model/mcp_service.go` | mcp_services | ✅ 含列表过滤 |
| `model/mcp_group.go` | mcp_groups | ✅ 含 slug 查询 |
| `model/mcp_group_service.go` | mcp_group_services | ✅ 含批量添加 |
| `model/mcp_group_tool.go` | mcp_group_tools | ✅ 含 Upsert |
| `model/cloud_endpoint.go` | cloud_endpoints | ✅ |
| `model/vision_config.go` | vision_configs | ✅ 仅模型定义 |
| `model/camera.go` | cameras | ✅ 仅模型定义 |
| `model/mcp_call_log.go` | mcp_call_logs | ✅ |
| `model/marketplace.go` | marketplace_items + marketplace_reviews | ✅ |
| `model/main.go` | 数据库初始化与迁移 | ✅ |

### 3.3 认证与中间件 ✅

| 文件 | 功能 | 状态 |
|------|------|------|
| `middleware/auth.go` | JWT 生成/解析 + UserAuth/AdminAuth 中间件 | ✅ |
| `middleware/api_key_auth.go` | X-API-Key 头认证，SHA256 哈希校验 | ✅ |
| `middleware/cors.go` | CORS 跨域配置 | ✅ |
| `middleware/logger.go` | 请求日志（方法/路径/延迟/状态码） | ✅ |

### 3.4 REST API 控制器与服务

#### 认证模块 ✅ 100%

| 端点 | 控制器 | 服务 | 状态 |
|------|--------|------|------|
| `POST /api/v1/auth/register` | ✅ | ✅ | 已测试通过 |
| `POST /api/v1/auth/login` | ✅ | ✅ | 已测试通过 |
| `GET /api/v1/auth/profile` | ✅ | ✅ | 已测试通过 |
| `PUT /api/v1/auth/profile` | ✅ | ✅ | |
| `PUT /api/v1/auth/password` | ✅ | ✅ | |

#### MCP 服务管理 ✅ 90%

| 端点 | 控制器 | 服务 | 状态 |
|------|--------|------|------|
| `GET /api/v1/services` | ✅ | ✅ 列表+过滤+分页 | 已测试通过 |
| `POST /api/v1/services` | ✅ | ✅ | 已测试通过 |
| `GET /api/v1/services/:id` | ✅ | ✅ | 已测试通过 |
| `PUT /api/v1/services/:id` | ✅ | ✅ | |
| `DELETE /api/v1/services/:id` | ✅ | ✅ 软删除 | |
| `POST /api/v1/services/:id/test` | ✅ | ⚠️ 返回 stub，需接入传输层 | 待完善 |
| `POST /api/v1/services/:id/refresh-tools` | ✅ | ⚠️ 返回空结果，需接入传输层 | 待完善 |
| `GET /api/v1/services/:id/tools` | ✅ | ✅ 从 tools_cache 读取 | |
| `GET /api/v1/services/:id/health` | ✅ | ✅ | |

#### MCP 分组管理 ✅ 95%

| 端点 | 控制器 | 服务 | 状态 |
|------|--------|------|------|
| `GET /api/v1/groups` | ✅ | ✅ | 已测试通过 |
| `POST /api/v1/groups` | ✅ | ✅ 含 slug 唯一约束 | 已测试通过 |
| `GET /api/v1/groups/:id` | ✅ | ✅ 含服务列表和工具计数 | |
| `PUT /api/v1/groups/:id` | ✅ | ✅ | |
| `DELETE /api/v1/groups/:id` | ✅ | ✅ | |
| `POST /api/v1/groups/:id/services` | ✅ | ✅ 批量添加服务 | |
| `DELETE /api/v1/groups/:id/services/:serviceId` | ✅ | ✅ | |
| `GET /api/v1/groups/:id/tools` | ✅ | ✅ 聚合+命名空间前缀 | |
| `PUT /api/v1/groups/:id/tools/:toolName` | ✅ | ✅ 启用/禁用/重命名 | |
| `POST /api/v1/groups/:id/refresh` | ✅ | ⚠️ 待接入传输层 | 待完善 |
| `GET /api/v1/groups/:id/endpoint` | ✅ | ✅ 含 MCP 客户端配置 | |

#### API Key 管理 ✅ 100%

| 端点 | 控制器 | 服务 | 状态 |
|------|--------|------|------|
| `GET /api/v1/api-keys` | ✅ | ✅ | |
| `POST /api/v1/api-keys` | ✅ | ✅ nm- 前缀，SHA256 存储 | 已测试通过 |
| `DELETE /api/v1/api-keys/:id` | ✅ | ✅ | |

#### 云端连接管理 🔧 40%

| 端点 | 控制器 | 服务 | 状态 |
|------|--------|------|------|
| `GET /api/v1/connections` | ✅ | ✅ | |
| `POST /api/v1/connections` | ✅ | ⚠️ CRUD 完整，XiaoZhi JWT 解析未实现 | 待完善 |
| `GET /api/v1/connections/:id` | ✅ | ✅ | |
| `PUT /api/v1/connections/:id` | ✅ | ✅ | |
| `DELETE /api/v1/connections/:id` | ✅ | ✅ | |
| `POST /api/v1/connections/:id/connect` | ✅ | ⚠️ 仅更新状态，未建立实际连接 | 待完善 |
| `POST /api/v1/connections/:id/disconnect` | ✅ | ⚠️ 仅更新状态 | 待完善 |
| `PUT /api/v1/connections/:id/bind-apikey` | ✅ | ✅ | |

#### 管理员接口 ✅ 100%

| 端点 | 控制器 | 服务 | 状态 |
|------|--------|------|------|
| `GET /api/v1/admin/users` | ✅ | ✅ | |
| `PUT /api/v1/admin/users/:id` | ✅ | ✅ | |
| `GET /api/v1/admin/stats` | ✅ | ✅ | |
| `GET /api/v1/admin/logs` | ✅ | ✅ | |

#### 视觉配置 ❌ 未实现

缺失文件：`controller/vision.go`、`service/vision.go`、`dto/vision.go`

缺失端点：CRUD + test（共 5 个）

#### 摄像头管理 ❌ 未实现

缺失文件：`controller/camera.go`、`service/camera.go`、`dto/camera.go`

缺失端点：CRUD + capture + latest（共 7 个）

### 3.5 MCP 协议网关 🔧 65%

#### 传输适配器

| 文件 | 传输类型 | 状态 | 说明 |
|------|----------|------|------|
| `internal/mcp/transport/transport.go` | 接口定义 | ✅ | TransportAdapter 接口 + Tool/ServerInfo 类型 |
| `internal/mcp/transport/stdio.go` | stdio | ✅ | 子进程启动、stdin/stdout JSON-RPC、工具发现 |
| `internal/mcp/transport/streamable_http.go` | streamable-http | ✅ | HTTP POST JSON-RPC、工具发现 |
| `internal/mcp/transport/sse.go` | sse | ❌ 未创建 | |
| `internal/mcp/transport/websocket.go` | websocket | ❌ 未创建 | |
| `internal/mcp/transport/passive_ws.go` | passive-ws | ❌ 未创建 | |

#### 连接池与路由

| 文件 | 功能 | 状态 |
|------|------|------|
| `internal/mcp/bridge/session_pool.go` | 会话池管理（懒加载、工具缓存、自动更新 DB） | ✅ |
| `internal/mcp/bridge/tool_router.go` | 工具路由（解析 `svc__tool` / `svc.tool`） | ✅ |

**SessionPool 待完善:**
- ⚠️ 空闲连接淘汰（idle eviction）未实现
- ⚠️ 健康检查定时器未实现
- ⚠️ 熔断机制未实现
- ⚠️ 自动重连未实现

#### Smart 模式搜索引擎

| 文件 | 功能 | 状态 |
|------|------|------|
| `internal/mcp/smart/bm25.go` | BM25Okapi 算法（零依赖，CJK 分词，Levenshtein 模糊匹配） | ✅ |
| `internal/mcp/smart/search_engine.go` | 搜索引擎（API Key → 分组 → 服务范围收敛）+ Describe | ✅ |
| `internal/mcp/smart/meta_tools.go` | 3 个元工具 Schema 定义 | ✅ |
| `internal/mcp/smart/executor.go` | 独立执行器 | ❌ 逻辑内联在 handler 中 |

#### 网关处理器

| 文件 | 功能 | 状态 |
|------|------|------|
| `internal/mcp/handler/gateway_handler.go` | 双模式分发器（Smart/Direct）、JSON-RPC 处理 | ✅ |

**已实现的 JSON-RPC 方法:**
- ✅ `initialize` — 协议握手
- ✅ `notifications/initialized` — 通知（无响应）
- ✅ `tools/list` — 双模式返回
- ✅ `tools/call` — Smart 元工具 + Direct 路由

**元工具实现:**
- ✅ `mcp.search` — BM25 搜索
- ✅ `mcp.describe` — 服务/工具描述
- ✅ `mcp.execute` — 工具执行

#### MCP 协议端点

| 端点 | 传输 | 状态 |
|------|------|------|
| `POST /mcp` | Streamable HTTP（主端点，固定 Smart） | ✅ 已测试通过 |
| `POST /mcp/group/:slug` | Streamable HTTP（分组端点，按 expose_mode） | ✅ 已测试通过 |
| `GET /mcp/group/:slug` | SSE 流 | ❌ 未实现 |
| `GET /mcp/ws` | WebSocket（主端点） | ❌ 返回 501 |
| `GET /mcp/ws/group/:slug` | WebSocket（分组端点） | ❌ 返回 501 |
| `GET /mcp/passive/` | 被动 WebSocket 接入 | ❌ 未创建 |

---

## 4. 已验证的端到端流程

以下流程已通过 curl 手动测试验证：

```
1. 注册用户 POST /auth/register ✅
2. 用户登录 POST /auth/login ✅ → 获取 JWT Token
3. 获取资料 GET /auth/profile ✅ → JWT 鉴权通过
4. 创建 MCP 服务 POST /services ✅ → streamable-http 类型
5. 创建 MCP 分组 POST /groups ✅ → 含 endpoint_slug
6. 创建 API Key POST /api-keys ✅ → nm- 前缀，仅创建时返回完整 key
7. MCP 协议握手 POST /mcp (initialize) ✅ → JSON-RPC 响应
8. 获取工具列表 POST /mcp (tools/list) ✅ → 返回 3 个元工具（Smart 模式）
9. 分组工具列表 POST /mcp/group/search (tools/list) ✅ → 按 expose_mode 返回
```

---

## 5. 待开发事项

### P0 — 核心功能完善

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 1 | 服务测试/刷新工具接入实际传输层 | `service/service.go` | 高 |
| 2 | SSE 传输适配器 | `internal/mcp/transport/sse.go` | 高 |
| 3 | WebSocket 传输适配器 | `internal/mcp/transport/websocket.go` | 高 |
| 4 | MCP WebSocket 端点（gorilla/websocket 升级） | `router/mcp_router.go` | 高 |
| 5 | 被动 WebSocket 接入端点 | `internal/mcp/transport/passive_ws.go` | 高 |
| 6 | SessionPool 空闲淘汰 + 健康检查 + 熔断 | `internal/mcp/bridge/session_pool.go` | 中 |

### P1 — 云端连接

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 7 | XiaoZhi JWT 解析（提取 Agent ID + 过期时间） | `service/connection.go` | 中 |
| 8 | WSS 主动连接管理器 | `internal/mcp/cloud/` (新建) | 中 |
| 9 | 云端连接：NewMCP 作为 MCP Server 向远端注册工具 | 新文件 | 中 |
| 10 | SSH 隧道连接支持 | 新文件 | 低 |

### P2 — 视觉与摄像头

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 11 | 视觉模型配置 CRUD + 测试 | `controller/vision.go`, `service/vision.go` | 中 |
| 12 | 摄像头管理 CRUD + 截图 | `controller/camera.go`, `service/camera.go` | 中 |
| 13 | RTSP/HTTP MJPEG 帧捕获 | 新文件 | 低 |
| 14 | 视觉分析 + 自动注册 MCP 工具 | 新文件 | 低 |

### P3 — 前端

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 15 | React 项目初始化（Vite + Semi Design） | `web/` | 中 |
| 16 | 登录/注册页面 | `web/` | 中 |
| 17 | 仪表盘（服务/分组/连接概览） | `web/` | 中 |
| 18 | MCP 服务管理页面 | `web/` | 中 |
| 19 | 分组管理 + 端点配置页面 | `web/` | 中 |
| 20 | API Key 管理页面 | `web/` | 低 |

### P4 — 优化与运维

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 21 | 调用日志记录中间件（MCP 端点） | `middleware/` | 低 |
| 22 | Redis 缓存集成 | `common/` | 低 |
| 23 | Docker 部署配置 | `Dockerfile`, `docker-compose.yml` | 低 |
| 24 | 市场功能（管理员上架/用户安装） | `controller/marketplace.go` | 低 |
| 25 | i18n 国际化 | `i18n/` | 低 |

---

## 6. 技术栈与依赖

```
github.com/gin-gonic/gin          — HTTP 框架
github.com/gin-contrib/cors        — CORS 中间件
github.com/golang-jwt/jwt/v5       — JWT 认证
github.com/joho/godotenv           — .env 加载
golang.org/x/crypto                — bcrypt 密码哈希
gorm.io/gorm                       — ORM
gorm.io/driver/sqlite              — SQLite 驱动
gorm.io/driver/mysql               — MySQL 驱动
gorm.io/driver/postgres            — PostgreSQL 驱动
```

**待引入依赖:**
- `github.com/gorilla/websocket` — WebSocket 支持
- `github.com/modelcontextprotocol/go-sdk` — MCP 官方 Go SDK

---

## 7. 目录结构

```
newmcp/
├── cmd/server/              # 应用入口
├── common/                  # 工具函数（配置/加密/响应/分页）
├── constant/                # 常量定义
├── dto/                     # 数据传输对象（请求/响应结构体）
├── model/                   # 数据模型层（12 个 GORM 模型）
├── middleware/               # 中间件（JWT/API Key/CORS/日志）
├── controller/              # 控制器层（HTTP 处理）
├── service/                 # 业务逻辑层
├── router/                  # 路由配置（REST API + MCP 端点）
├── internal/mcp/
│   ├── transport/           # 传输适配器（stdio, streamable-http）
│   ├── bridge/              # 会话池 + 工具路由
│   ├── smart/               # Smart 模式（BM25 搜索 + 元工具）
│   └── handler/             # 网关处理器（双模式分发）
├── docs/                    # 项目文档
├── test/                    # 测试环境（Python MCP 服务器）
└── reference/new-api/       # 参考项目
```
