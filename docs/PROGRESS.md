# NewMCP 开发进度文档

> 最后更新: 2026-05-05 | 后端: 5,800+ 行 Go (56 源文件) | 前端: React 19 + TanStack 全家桶

---

## 1. 项目概况

NewMCP 是一个统一的 MCP（Model Context Protocol）网关平台，采用 Go 后端 + React 前端架构。前端对标 new-api Default 版，使用 React 19 + TanStack Router + TanStack Query + shadcn/ui + Rsbuild。

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
| MCP 协议网关 | 🔧 部分完成 | 80% | Smart+Direct 模式 + API Key 校验 + 调用日志自动记录 |
| 云端连接 | ✅ 已完成 | 85% | XiaoZhi JWT 解析 + WSS 连接 + 自动重连 |
| **调用日志** | **✅ 已完成** | **90%** | **自动记录 + 多维筛选 + 统计 + 管理员/用户视图** |
| 视觉配置 | ❌ 未开始 | 5% | 仅 Model 层 |
| 摄像头管理 | ❌ 未开始 | 5% | 仅 Model 层 |
| **前端界面** | **✅ 已完成** | **85%** | **架构 + 核心业务 + 日志 + Dashboard 实时数据** |

**整体完成度: ~85%**

---

## 3. 前端开发进度 (新增)

### 3.1 前端架构 ✅ 已完成

| 技术 | 版本 | 用途 |
|------|------|------|
| React | 19.x | UI 框架 |
| TypeScript | ~5.9.x | 类型安全 |
| Rsbuild | 2.x | 构建工具 (基于 Rspack/Rust) |
| TanStack Router | 1.x | 文件路由 + 类型安全导航 |
| TanStack Query | 5.x | 服务端状态管理 (缓存/去重/重试) |
| Zustand | 5.x | 客户端状态 (auth/config) |
| Radix UI + shadcn/ui | latest | 无样式组件 + Tailwind 样式 |
| Tailwind CSS | 4.x | 原子化样式 |
| Axios | 1.x | HTTP 客户端 |
| i18next | 25.x | 中英文国际化 |
| Sonner | latest | Toast 通知 |
| Lucide React | latest | 图标库 |

### 3.2 前端页面状态

| 页面 | 路由 | 状态 | 说明 |
|------|------|------|------|
| 首页/Landing | `/` | ✅ | Hero + 功能卡片 + 品牌视觉 |
| 登录 | `/sign-in` | ✅ | 左右分栏 + JWT 认证 |
| 注册 | `/sign-up` | ✅ | 居中表单 + 错误提示 |
| 控制台 | `/dashboard` | ✅ | 统计卡片 + 服务健康 + 最近日志 (对接真实 API) |
| 服务列表 | `/services` | ✅ | 表格 + 传输类型筛选 + 搜索 + 启用/禁用 |
| 注册服务 | `/services/create` | ✅ | 4 步分步表单 (基本信息→传输→认证→测试) |
| 服务详情 | `/services/:id` | ✅ | 信息卡 + 工具列表 + 测试连接 + 刷新工具 |
| 分组列表 | `/groups` | ✅ | 卡片视图 + Direct/Smart 标签 |
| 创建分组 | `/groups/create` | ✅ | 表单 + 暴露模式选择 |
| 分组详情 | `/groups/:id` | ✅ | 模式切换 + 端点复制 + 服务管理 + 工具列表 |
| MCP 广场 | `/marketplace` | ✅ | 卡片网格 + 即用/源码筛选 + 搜索 |
| 市场详情 | `/marketplace/:id` | ✅ | 一键安装 + 部署指南 + 工具快照 |
| API 密钥 | `/api-keys` | ✅ | 列表 + 创建 + Key 仅显示一次 |
| 连接列表 | `/connections` | ✅ | 表格 + 连接/断开操作 |
| 创建连接 | `/connections/create` | ✅ | 小智/自定义WSS/SSH + API Key 绑定 |
| 连接详情 | `/connections/:id` | ✅ | 状态 + 配置信息 |
| 个人设置 | `/settings` | 🔲 | 占位页 |
| 调用日志 | `/logs` | ✅ | 统计卡片 + 筛选 + 表格 + 详情展开 |
| 管理员页面 | `/admin/*` | 🔲 | 占位页 |
| 视觉配置 | `/vision/*` | 🔲 | 占位页 |
| 摄像头 | `/cameras/*` | 🔲 | 占位页 |

### 3.3 前端核心模块

| 模块 | 文件 | 说明 |
|------|------|------|
| API 客户端 | `lib/api.ts` | Axios + JWT Bearer + 请求去重 + 统一错误处理 |
| 认证状态 | `stores/auth-store.ts` | Zustand + localStorage 持久化 |
| 系统配置 | `stores/system-config-store.ts` | Zustand + 持久化 |
| 主题 | `context/theme-provider.tsx` | 亮/暗/系统 + 跟随 OS |
| 侧边栏 | `components/layout/app-sidebar.tsx` | 可折叠 + 管理员导航 |
| 头部 | `components/layout/header.tsx` | 主题切换 + 用户菜单 |
| 国际化 | `i18n/locales/{zh,en}.json` | 中英文翻译 |
| 类型定义 | `types/index.ts` | 完整 TypeScript 接口 (对接后端所有 DTO) |
| API 模块 | `features/*/api.ts` | 6 个模块 API 层 (services/groups/marketplace/apikeys/connections/admin) |

### 3.4 前端构建

- 构建工具: Rsbuild (Rspack/Rust 驱动)
- 构建输出: ~931 KB (gzip ~303 KB)
- 代码分割: vendor-react / vendor-radix / vendor-tanstack 独立 chunk
- 开发模式: `npm run dev` → http://localhost:5173
- 代理配置: `/api` → `http://localhost:3000` (Go 后端)

---

## 4. 已验证的端到端流程

### 4.1 基础管理流程 ✅

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

### 4.2 市场 + 分组 + API Key 完整流程 ✅

```
 1. 管理员上架 MCP 服务到市场 POST /admin/marketplace ✅
 2. 用户浏览市场 GET /marketplace ✅
 3. 用户从市场安装 POST /marketplace/install ✅
 4. 用户创建分组 + 添加服务 POST /groups ✅ + POST /groups/:id/services ✅
 5. 用户创建 API Key 并绑定分组 POST /api-keys ✅
 6. AI Agent 通过 MCP 端点使用工具 POST /mcp/group/search ✅
```

### 4.3 前端流程 (新增) ✅

```
 1. 访问首页 → 查看产品介绍 → 点击注册 ✅
 2. 注册账号 → 自动登录 → 跳转 Dashboard ✅
 3. 注册新 MCP 服务 → 4 步表单 → 测试连接 → 创建 ✅
 4. 查看服务列表 → 筛选/搜索 → 查看详情 ✅
 5. 创建分组 → 切换暴露模式 → 添加服务 → 复制端点 URL ✅
 6. 浏览 MCP 广场 → 按类型筛选 → 查看详情 → 一键安装 ✅
 7. 创建 API Key → 绑定分组 → 查看 key (仅一次) ✅
 8. 添加云端连接 → 选择平台 → 绑定 API Key → 连接/断开 ✅
```

### 4.4 调用日志流程 ✅

```
 1. MCP 网关自动记录所有 tools/call 请求 ✅
 2. 记录用户/API Key/分组/服务/工具/状态/耗时/IP ✅
 3. 用户访问 /logs → 查看自己的调用日志 + 统计 ✅
 4. 管理员访问 /admin/logs → 查看全局日志 + 多维筛选 + 统计 ✅
 5. Dashboard 展示实时统计数据 + 最近调用日志 ✅
```

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

### P1 — 前端待完善

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 7 | ~~Dashboard 实时数据 (对接 admin/stats API)~~ | `features/dashboard/` | ~~高~~ ✅ |
| 8 | 个人设置页面 | `features/settings/` | 中 |
| 9 | 管理员页面 (用户管理/市场管理/系统设置) | `features/admin/` | 中 |
| 10 | 视觉配置 CRUD 页面 | `features/vision/` | 低 |
| 11 | 摄像头管理页面 | `features/cameras/` | 低 |
| 12 | Go 后端 go:embed 嵌入前端 | `cmd/server/main.go` | 中 |
| 13 | Docker 构建配置 (多阶段: Bun→Go) | `Dockerfile` | 中 |

### P2 — 优化与运维

| # | 事项 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| 14 | SSH 隧道连接支持 | 新文件 | 低 |
| 15 | ~~调用日志记录中间件~~ | `internal/mcp/handler/` | ~~低~~ ✅ |

---

## 6. 技术栈

### 后端

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

### 前端

```
react@19 + react-dom@19            — UI 框架
typescript@5.9                      — 类型安全
@rsbuild/core@2                     — 构建工具 (Rspack)
@tanstack/react-router@1            — 文件路由
@tanstack/react-query@5             — 服务端状态
zustand@5                           — 客户端状态
@radix-ui/*                         — 无样式组件原语
tailwindcss@4                       — 原子化样式
axios@1                             — HTTP 客户端
i18next@25 + react-i18next          — 国际化
sonner@2                            — Toast 通知
lucide-react                        — 图标库
```

---

## 7. 目录结构

```
newmcp/
├── cmd/server/              # 应用入口
├── common/                  # 工具函数（配置/加密/响应/分页）
├── dto/                     # 数据传输对象
├── model/                   # 数据模型层
├── service/                 # 业务逻辑层
├── controller/              # 控制器层
├── router/                  # 路由配置
│   ├── api_router.go        # REST API
│   └── mcp_router.go        # MCP 端点
├── internal/mcp/
│   ├── transport/           # 传输适配器
│   ├── bridge/              # 会话池 + 工具路由
│   ├── smart/               # Smart 模式搜索引擎
│   └── handler/             # 网关处理器
├── web/                     # 前端项目 (React 19)
│   ├── src/
│   │   ├── components/ui/   # shadcn/ui 基础组件
│   │   ├── components/layout/ # 布局组件 (Sidebar/Header)
│   │   ├── features/        # 功能模块 (API + 组件)
│   │   │   ├── auth/        # 登录/注册
│   │   │   ├── services/    # 服务管理 (列表/创建/详情)
│   │   │   ├── groups/      # 分组管理
│   │   │   ├── marketplace/ # MCP 广场
│   │   │   ├── api-keys/    # API 密钥
│   │   │   ├── connections/ # 云端连接
│   │   │   ├── dashboard/   # 控制台
│   │   │   └── admin/       # 管理员
│   │   ├── routes/          # TanStack Router 文件路由
│   │   ├── lib/             # API client + 工具函数
│   │   ├── stores/          # Zustand 状态
│   │   ├── context/         # 主题 Provider
│   │   ├── i18n/            # 中英文翻译
│   │   └── types/           # TypeScript 类型定义
│   ├── rsbuild.config.ts    # Rsbuild 构建配置
│   ├── postcss.config.mjs   # PostCSS (Tailwind v4)
│   └── package.json
├── docs/                    # 项目文档
│   ├── PRD.md               # 产品需求文档
│   ├── FRONTEND.md          # 前端设计文档 V2.0
│   ├── ARCHITECTURE.md      # 架构设计
│   ├── API.md               # API 文档
│   ├── DATABASE.md          # 数据库设计
│   └── PROGRESS.md          # 本文档
└── data/                    # 数据目录 (SQLite)
```
