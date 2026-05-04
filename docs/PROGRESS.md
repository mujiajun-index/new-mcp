# NewMCP 开发进度文档

> 最后更新: 2026-05-04 | 代码行数: 5,800+ 行 Go | 源文件: 56 个

---

## 1. 项目概况

NewMCP 是一个统一的 MCP（Model Context Protocol）网关平台，采用 Go 模块化单体架构。本文档追踪各模块的开发状态、已完成功能与待开发项。

---

## 2. 总体进度

| 模块 | 状态 | 完成度 | 说明 |
|------|------|--------|------|
| 项目基础设施 | ✅ 已完成 | 100% | 入口、配置、工具函数、数据库 |
| 认证系统 | ✅ 已完成 | 100% | 注册/登录/JWT/API Key |
| MCP 服务管理 | ✅ 已完成 | 95% | CRUD + RefreshTools 接入传输层 |
| MCP 分组管理 | ✅ 已完成 | 95% | CRUD + 工具聚合完整 |
| API Key 管理 | ✅ 已完成 | 100% | 创建/列表/删除/分组绑定 |
| 市场功能 | ✅ 已完成 | 90% | 管理员上架/用户浏览/安装/评价 |
| 管理员接口 | ✅ 已完成 | 100% | 用户管理/统计/日志/平台服务/市场管理 |
| MCP 协议网关 | 🔧 部分完成 | 75% | Smart+Direct 模式 + API Key 权限校验 |
| 云端连接 | ✅ 已完成 | 85% | XiaoZhi JWT 解析 + WSS 连接 + 自动重连 |
| 视觉配置 | ❌ 未开始 | 5% | 仅 Model 层 |
| 摄像头管理 | ❌ 未开始 | 5% | 仅 Model 层 |
| 前端界面 | ❌ 未开始 | 0% | 计划 React + Semi Design |

**整体完成度: ~72%**

---

## 3. 已验证的端到端流程

### 3.1 基础管理流程 ✅

```
 1. 注册用户 POST /auth/register ✅
 2. 用户登录 POST /auth/login ✅ → 获取 JWT Token
 3. 获取资料 GET /auth/profile ✅ → JWT 鉴权通过
 4. 创建 MCP 服务 POST /services ✅
 5. 创建 MCP 分组 POST /groups ✅
 6. 创建 API Key POST /api-keys ✅ → nm- 前缀，仅创建时返回完整 key
 7. MCP 协议握手 POST /mcp (initialize) ✅
 8. 获取工具列表 POST /mcp (tools/list) ✅ → Smart 模式 3 个元工具
 9. 创建小智连接 POST /connections ✅ → JWT 解析
10. MCP 搜索工具 POST /mcp (tools/call mcp.search) ✅
```

### 3.2 市场 + 分组 + API Key 完整流程 ✅ (新增)

```
 1. 管理员上架 MCP 服务到市场
    POST /admin/marketplace ✅
    → 创建 MarketplaceItem (Exa 网络搜索, category=instant)

 2. 用户浏览市场
    GET /marketplace ✅
    → 返回已上架的 MCP 服务列表

 3. 用户从市场安装
    POST /marketplace/install ✅
    → 创建 McpService (source=marketplace, 复制 config_template + tools_snapshot)
    → marketplace install_count +1

 4. 用户创建分组 + 添加服务
    POST /groups ✅ (expose_mode=direct)
    POST /groups/:id/services ✅

 5. 用户创建 API Key 并绑定分组
    POST /api-keys ✅ (groups=["search-tools"])
    → permissions = {"groups":["search-tools"]}

 6. AI Agent 通过 MCP 端点使用工具
    POST /mcp/group/search (X-API-Key) ✅
    → initialize → tools/list 返回 exa-search__web_search_exa 等工具
    → 权限校验：未绑定分组返回 -32602 错误 ✅
```

---

## 4. 详细模块状态

### 4.1 市场功能 ✅ 90%

**新增文件:**

| 文件 | 功能 | 状态 |
|------|------|------|
| `dto/marketplace.go` | 市场 DTO (创建/更新/列表/详情/安装/评价) | ✅ |
| `service/marketplace.go` | 市场业务逻辑 (管理员CRUD + 浏览 + 安装 + 评价) | ✅ |
| `controller/marketplace.go` | 市场 HTTP 处理 (Admin/Browse/Install/Review) | ✅ |

**修改文件:**

| 文件 | 变更 | 状态 |
|------|------|------|
| `model/marketplace.go` | 添加查询方法 + Review 字段 (ItemID, Rating, ReviewText) | ✅ |
| `model/mcp_service.go` | 添加 ListServicesBySource, GetServiceByIDWithoutUser | ✅ |
| `router/api_router.go` | 市场路由 (公开浏览 + 用户安装/评价 + 管理员管理) | ✅ |

**API 端点:**

| 端点 | 说明 | 状态 |
|------|------|------|
| `GET /marketplace` | 公开浏览市场 | ✅ 已测试 |
| `GET /marketplace/:id` | 查看市场项详情 | ✅ |
| `POST /marketplace/install` | 用户安装 (需登录) | ✅ 已测试 |
| `POST /marketplace/:id/review` | 用户评价 | ✅ |
| `GET /admin/marketplace` | 管理员市场列表 | ✅ |
| `POST /admin/marketplace` | 管理员上架 | ✅ 已测试 |
| `GET /admin/marketplace/:id` | 管理员查看 | ✅ |
| `PUT /admin/marketplace/:id` | 管理员更新 | ✅ |
| `DELETE /admin/marketplace/:id` | 管理员下架 | ✅ |
| `GET /admin/services` | 管理员平台服务列表 | ✅ |
| `POST /admin/services` | 管理员创建平台服务 (source=admin) | ✅ |

### 4.2 API Key 分组绑定 ✅

| 变更 | 说明 |
|------|------|
| `dto/apikey.go` CreateApiKeyReq 添加 `Groups []string` | 创建时直接绑定分组 |
| `service/apikey.go` 添加 validateGroups | 验证分组归属 + 构建 permissions JSON |
| API Key permissions 格式 | `{"groups":["group1","group2"]}` 或 `{"groups":["*"]}` |

### 4.3 网关权限控制 ✅

| 变更 | 说明 |
|------|------|
| `gateway_handler.go` 添加 hasGroupAccess | 读取 API Key permissions 验证分组访问权限 |
| handleToolsList 权限校验 | groupSlug 非空时验证 API Key 有权访问 |
| handleToolsCall 权限校验 | Direct 模式路由前验证分组权限 |

### 4.4 RefreshTools 实现 ✅

| 变更 | 说明 |
|------|------|
| `service/service.go` 实现 RefreshTools | 通过 SessionPool.GetOrConnect 连接上游获取 tools |
| `service/service.go` 实现 Test | 通过 SessionPool 测试连接 |
| `service/service.go` 添加 SessionPool 变量 | 依赖注入 |
| `router/main.go` 注入 SessionPool | InitGateway 中设置 |

---

## 5. 待开发事项

### P0 — 核心功能完善

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 1 | SSE 传输适配器 | `internal/mcp/transport/sse.go` | 高 |
| 2 | WebSocket 传输适配器 | `internal/mcp/transport/websocket.go` | 高 |
| 3 | MCP WebSocket 端点 | `router/mcp_router.go` | 高 |
| 4 | 被动 WebSocket 接入 | `internal/mcp/transport/passive_ws.go` | 高 |
| 5 | SessionPool 空闲淘汰 + 健康检查 | `internal/mcp/bridge/session_pool.go` | 中 |
| 6 | BM25 搜索优化 (中文分词改进) | `internal/mcp/smart/bm25.go` | 中 |

### P1 — 云端连接

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 7 | SSH 隧道连接支持 | 新文件 | 低 |

### P2 — 视觉与摄像头

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 8 | 视觉模型配置 CRUD + 测试 | `controller/vision.go`, `service/vision.go` | 中 |
| 9 | 摄像头管理 CRUD + 截图 | `controller/camera.go`, `service/camera.go` | 中 |

### P3 — 前端

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 10 | React 项目初始化 | `web/` | 中 |
| 11 | 仪表盘 + 服务/分组管理页面 | `web/` | 中 |
| 12 | 市场浏览 + 安装页面 | `web/` | 中 |
| 13 | API Key 管理 + MCP 端点配置页面 | `web/` | 低 |

### P4 — 优化与运维

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 14 | 调用日志记录中间件 | `middleware/` | 低 |
| 15 | Docker 部署配置 | `Dockerfile`, `docker-compose.yml` | 低 |

---

## 6. 技术栈与依赖

```
github.com/gin-gonic/gin          — HTTP 框架
github.com/golang-jwt/jwt/v5       — JWT 认证
github.com/joho/godotenv           — .env 加载
github.com/gorilla/websocket       — WebSocket 支持
golang.org/x/crypto                — bcrypt 密码哈希
gorm.io/gorm                       — ORM
gorm.io/driver/sqlite              — SQLite 驱动
gorm.io/driver/mysql               — MySQL 驱动
gorm.io/driver/postgres            — PostgreSQL 驱动
```

---

## 7. 目录结构

```
newmcp/
├── cmd/server/              # 应用入口
├── common/                  # 工具函数（配置/加密/响应/分页）
├── dto/                     # 数据传输对象
│   ├── auth.go              # 认证 DTO
│   ├── apikey.go            # API Key DTO (含 Groups 字段)
│   ├── marketplace.go       # 市场 DTO (新增)
│   └── ...
├── model/                   # 数据模型层
│   ├── marketplace.go       # 市场模型 + 查询方法
│   ├── mcp_service.go       # 服务模型 (含 ListServicesBySource)
│   └── ...
├── service/                 # 业务逻辑层
│   ├── marketplace.go       # 市场服务 (新增)
│   ├── service.go           # 服务管理 (含 RefreshTools + AdminService)
│   ├── apikey.go            # API Key (含分组绑定验证)
│   └── ...
├── controller/              # 控制器层
│   ├── marketplace.go       # 市场控制器 (新增)
│   ├── admin.go             # 管理员 (含平台服务管理)
│   └── ...
├── router/                  # 路由配置
│   ├── api_router.go        # REST API (含市场路由)
│   └── mcp_router.go        # MCP 端点
├── internal/mcp/
│   ├── transport/           # 传输适配器
│   ├── bridge/              # 会话池 + 工具路由
│   ├── smart/               # Smart 模式搜索引擎
│   └── handler/             # 网关处理器 (含权限校验)
├── docs/                    # 项目文档
└── data/                    # 数据目录 (SQLite)
```
