# NewMCP 开发进度文档

> 最后更新: 2026-07-16 | 后端: 7,000+ 行 Go (60+ 源文件) | 前端: React 19 + TanStack 全家桶

---

## 1. 项目概况

NewMCP 是一个统一的 MCP（Model Context Protocol）网关平台，采用 Go 后端 + React 前端架构。前端对标 new-api Default 版，使用 React 19 + TanStack Router + TanStack Query + shadcn/ui + Rsbuild。

---

## 2. 总体进度

| 模块 | 状态 | 完成度 | 说明 |
|------|------|--------|------|
| 项目基础设施 | ✅ 已完成 | 100% | 入口、配置、工具函数、数据库 |
| 认证系统 | ✅ 已完成 | 100% | 注册/登录/JWT/API Key + 首次运行引导 |
| MCP 服务管理 | ✅ 已完成 | 95% | CRUD + RefreshTools 接入传输层 |
| MCP 分组管理 | ✅ 已完成 | 98% | CRUD + 工具聚合 + 工具选择管理 |
| API Key 管理 | ✅ 已完成 | 100% | 创建/编辑/删除/额度/有效期/IP白名单/分组绑定 |
| 市场功能 | ✅ 已完成 | 90% | 管理员上架/用户浏览/安装/评价 |
| **商业化(全栈)** | **✅ 已完成** | **100%** | **市场服务按次计费核心闭环 + 3级定价 + 引用式安装 + 额度管理 + 兑换码 + 管理员调额 + 凭证加密 + 钱包/价格/计费设置/市场定价前端;详见 [COMMERCIALIZATION.md](./COMMERCIALIZATION.md)** |
| 管理员接口 | ✅ 已完成 | 100% | 用户管理(CRUD+搜索+额度)/统计/日志/平台服务/市场管理 |
| MCP 协议网关 | ✅ 已完成 | 95% | 双端点 Direct/Smart + 用户隔离 + 共享 Resolver + 调用日志 |
| 云端连接 | ✅ 已完成 | 90% | XiaoZhi JWT 解析 + WSS 连接 + 自动重连 + 复用 GatewayHandler |
| **调用日志** | **✅ 已完成** | **90%** | **自动记录 + 多维筛选 + 统计 + 管理员/用户视图** |
| **视觉配置** | **✅ 已完成** | **95%** | **CRUD + 虚拟 MCP 服务注册 + 多供应商支持 + AI 分析工具** |
| **摄像头管理** | **✅ 已完成** | **95%** | **CRUD + WebRTC 预览 + WebSocket 推流 + capture/analyze 工具** |
| **前端界面** | **✅ 已完成** | **95%** | **架构 + 核心业务 + 日志 + Dashboard + 设置 + 视觉 + 摄像头** |
| **系统设置** | **✅ 已完成** | **90%** | **Option 键值模型 + 内存缓存 + 管理员设置页面 + 注册守卫 + 分组限流** |

**整体完成度: ~94%**

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
| 系统初始化 | `/setup` | ✅ | 首次运行引导 + 创建管理员 + 路由保护 |
| 控制台 | `/dashboard` | ✅ | 统计卡片 + 服务健康 + 最近日志 (对接真实 API) |
| 服务列表 | `/services` | ✅ | 表格 + 传输类型筛选 + 搜索 + 启用/禁用 |
| 注册服务 | `/services/create` | ✅ | 4 步分步表单 (基本信息→传输→认证→测试) |
| 服务详情 | `/services/:id` | ✅ | 信息卡 + 工具列表 + 测试连接 + 刷新工具 |
| 分组列表 | `/groups` | ✅ | 卡片视图 + Direct/Smart 标签 |
| 创建分组 | `/groups/create` | ✅ | 表单 + 暴露模式选择 |
| 分组详情 | `/groups/:id` | ✅ | 模式切换 + 端点复制 + 服务管理 + 工具管理面板(启用/禁用) |
| MCP 广场 | `/marketplace` | ✅ | 卡片网格 + 即用/源码筛选 + 搜索 |
| 市场详情 | `/marketplace/:id` | ✅ | 一键安装 + 部署指南 + 工具快照 |
| API 密钥 | `/api-keys` | ✅ | 完整 CRUD + 额度/有效期/IP/编辑/状态切换 |
| 连接列表 | `/connections` | ✅ | 表格 + 连接/断开操作 |
| 创建连接 | `/connections/create` | ✅ | 小智/自定义WSS/SSH + API Key 绑定 |
| 连接详情 | `/connections/:id` | ✅ | 状态 + 配置信息 |
| 个人设置 | `/settings` | ✅ | 账号信息 + 用量统计 + 编辑资料 + 修改密码 |
| 调用日志 | `/logs` | ✅ | 统计卡片 + 筛选 + 表格 + 详情展开 |
| 管理员-系统设置 | `/admin/system` | ✅ | Tabs 分区 (通用/认证/限流/SMTP/维护) + 逐字段自动保存 + 分组限流编辑器 |
| 管理员页面 | `/admin/*` | 🔲 | 部分完成 (用户管理/市场管理/系统设置已完成) |
| 视觉配置列表 | `/vision` | ✅ | 卡片视图 + 供应商筛选 + 启用/禁用 |
| 视觉配置详情 | `/vision/:id` | ✅ | 配置编辑 + 测试连接 + 工具自定义名称/描述 |
| 摄像头列表 | `/cameras` | ✅ | 卡片视图 + 启用/禁用 + 推流状态 |
| 摄像头详情 | `/cameras/:id` | ✅ | 配置编辑 + WebRTC 预览 + WebSocket 推流 + 工具自定义 |

### 3.3 前端核心模块

| 模块 | 文件 | 说明 |
|------|------|------|
| API 客户端 | `lib/api.ts` | Axios + JWT Bearer + 请求去重 + 统一错误处理 |
| Setup 检测 | `lib/setup-check.ts` | 首次运行状态检测 + 缓存 |
| 认证状态 | `stores/auth-store.ts` | Zustand + localStorage 持久化 |
| 系统配置 | `stores/system-config-store.ts` | Zustand + 持久化 |
| 主题 | `context/theme-provider.tsx` | 亮/暗/系统 + 跟随 OS |
| 侧边栏 | `components/layout/app-sidebar.tsx` | 可折叠 + 管理员导航 |
| 头部 | `components/layout/header.tsx` | 主题切换 + 用户菜单 |
| 国际化 | `i18n/locales/{zh,en}.json` | 中英文翻译 |
| 类型定义 | `types/index.ts` | 完整 TypeScript 接口 (对接后端所有 DTO) |
| API 模块 | `features/*/api.ts` | 8 个模块 API 层 (services/groups/marketplace/apikeys/connections/admin/setup/settings) |

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
 0. 系统首次启动 (空数据库) → 自动跳转 /setup → 创建管理员 → 跳转登录页 ✅
 1. 注册用户 POST /auth/register ✅

```
 1. 注册用户 POST /auth/register ✅
 2. 用户登录 POST /auth/login ✅ → 获取 JWT Token
 3. 获取资料 GET /auth/profile ✅ → JWT 鉴权通过
 4. 创建 MCP 服务 POST /services ✅
 5. 创建 MCP 分组 POST /groups ✅
 6. 创建 API Key POST /api-keys ✅ → sk- 前缀，仅创建时返回完整 key
 7. MCP 协议握手 POST /mcp (initialize) ✅
 8. 获取工具列表 POST /mcp (tools/list) ✅ → Direct 模式，全部聚合工具
 9. 获取 Smart 元工具 POST /smart/mcp (tools/list) ✅ → 3 个元工具
10. 创建小智连接 POST /connections ✅ → JWT 解析
11. MCP 搜索工具 POST /smart/mcp (tools/call mcp.search) ✅
```

### 4.2 市场 + 分组 + API Key 完整流程 ✅

```
 1. 管理员上架 MCP 服务到市场 POST /admin/marketplace ✅
 2. 用户浏览市场 GET /marketplace ✅
 3. 用户从市场引用式安装 POST /marketplace/:id/add ✅(空 config,平台托管)
 4. 用户创建分组 + 添加服务 POST /groups ✅ + POST /groups/:id/services ✅
 5. 用户创建 API Key 并绑定分组 POST /api-keys ✅
 6. AI Agent 通过 MCP 端点使用工具 POST /mcp/group/:slug ✅(source=marketplace 按市场价扣费 / source=user 免费)
```

### 4.3 前端流程 (新增) ✅

```
 0. 首次访问 (空数据库) → 自动跳转 /setup → 填写管理员信息 → 初始化成功 → 跳转登录 ✅
 1. 访问首页 → 查看产品介绍 → 点击注册 ✅
 2. 注册账号 → 自动登录 → 跳转 Dashboard ✅
 3. 注册新 MCP 服务 → 4 步表单 → 测试连接 → 创建 ✅
 4. 查看服务列表 → 筛选/搜索 → 查看详情 ✅
 5. 创建分组 → 切换暴露模式 → 添加服务 → 管理工具(启用/禁用) → 编辑分组信息(标识/名称/描述) → 复制端点 URL ✅
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

### 4.5 视觉配置流程 ✅

```
 1. 创建视觉配置 → 选择供应商/模型/端点/API Key ✅
 2. 测试连通性 → 发送 1x1 测试图片验证配置 ✅
 3. 启用视觉配置 → 自动创建虚拟 McpService + 注册 VirtualToolRegistry ✅
 4. 自定义工具名称/描述 → 同步更新 tools_cache ✅
 5. AI Agent 通过 MCP 端点调用 vision.analyze_image / vision.describe_scene ✅
 6. 禁用/删除 → 自动清理虚拟服务、分组关联、工具记录 ✅
```

### 4.6 摄像头流程 ✅

```
 1. 创建摄像头 → 绑定视觉配置 ✅
 2. 启用摄像头 → 自动创建虚拟 McpService + 注册 VirtualToolRegistry ✅
 3. 开启摄像头 → WebRTC getUserMedia 授权 → 预览画面 ✅
 4. WebSocket 推流 → canvas 截帧 → 发送到 /api/v1/cameras/:id/stream ✅
 5. AI Agent 调用 camera.capture → 返回最新帧 base64 图像 ✅
 6. AI Agent 调用 camera.analyze → 截帧 + 调用视觉模型 AI 识别 ✅
 7. 关闭摄像头 → 停止推流 + 清理 WebSocket + 释放摄像头设备 ✅
 8. 禁用/删除 → 自动清理虚拟服务、分组关联、工具记录 ✅
```

### 4.7 系统设置流程 ✅

```
 1. 管理员访问 /admin/system → 查看 5 个设置分类 (通用/认证/限流/SMTP/维护) ✅
 2. 修改系统名称 → onBlur 自动保存 → GET /settings/public 返回新名称 ✅
 3. 关闭用户注册 → RegisterEnabled=false → 注册接口返回 "注册功能已禁用" ✅
 4. 配置邮箱域名限制 → 开启 + 设置白名单 → 注册时校验邮箱域名 ✅
 5. 配置速率限制 → 设置全局参数 + 分组级限流规则 ✅
 6. 配置 SMTP → 保存服务器信息 (敏感字段掩码) ✅
 7. 查看系统维护 → 显示当前版本号 (只读) ✅
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
| 8 | ~~个人设置页面~~ | `features/settings/` | ~~中~~ ✅ |
| 9 | ~~管理员页面 (市场管理/系统设置)~~ | `features/admin/` | ~~中~~ ✅ |
| 10 | ~~视觉配置 CRUD 页面~~ | `features/vision/` | ~~低~~ ✅ |
| 11 | ~~摄像头管理页面~~ | `features/cameras/` | ~~低~~ ✅ |
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
│   ├── bridge/              # 会话池 + 工具路由 + ApiKeyResolver
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
│   │   │   ├── vision/      # 视觉配置 (列表/详情/测试)
│   │   │   ├── cameras/     # 摄像头 (列表/详情/推流预览)
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
│   ├── COMMERCIALIZATION.md # 商业化模块设计 (V1.7)
│   └── PROGRESS.md          # 本文档
└── data/                    # 数据目录 (SQLite)
```

---

## 8. 商业化模块进度 (V1,2026-07-16)

> 设计文档:[COMMERCIALIZATION.md](./COMMERCIALIZATION.md) V1.7。市场来源服务按次固定单价计费,用户自有服务免费,共用同一条分组调用路径按 `service.source` 门控。

### 8.1 后端 ✅ 已完成(build + vet + 运行时启动验证通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 数据模型 | ✅ | users 加 billing_preference/total_topup;marketplace_items 加 billing_type/price_per_call/subscription_only;mcp_call_logs 加计费6列;新表 mcp_tool_prices / redemptions;AutoMigrate 已注册 |
| 配置项 | ✅ | options 加齐 §15 计费/额度/日志/自有服务/自用模式默认项 + 公开键 + GetOptionInt64/Float/GetGroupRatio |
| 原子额度 | ✅ | DecreaseUserQuotaAtomic / SetUserQuota / Adjust*UsedQuota(符号无关) / Key 预算原子占用;兑换码 status 1→2 原子占领(三库通用) |
| 定价解析 `billing/pricing.go` | ✅ | 工具>服务>全局 3 级 + 分组倍率 + 自用模式门控 + 60s 缓存 + Invalidate* |
| 计费服务 `billing/billing.go` | ✅ | PreConsume/Confirm/Refund + 信任旁路 + 余额不足拒(不禁Key)+ request_id 幂等 + FailOpen 欠账 + 低额度提醒 |
| 网关接入 | ✅ | handleToolsCall/routeAndCall/handleExecute 接入计费插入点 A/B(仅 source=marketplace);materializeMarketplaceConfig 注入平台凭证(引用行 config 恒空) |
| 引用式安装 | ✅ | POST /marketplace/:id/add(空 config + marketplace 哨兵 transport + 去重) |
| 自有服务守卫 | ✅ | UserOwnedServicesEnabled=false 时禁 source=user 创建/调用 |
| 管理员定价/批量/克隆 | ✅ | 非自用模式显式定价门控;PUT /admin/marketplace/pricing/batch;POST /admin/marketplace/clone;config_template 加密落库 |
| 兑换码 | ✅ | admin CRUD + 用户 POST /redemptions/redeem(RedemptionEnabled 开关) |
| 管理员调额 | ✅ | POST /admin/users/:id/quota(add/sub/set + canManageTargetRole + 审计) |
| 钱包/用量 | ✅ | GET /wallet、/wallet/billing、/wallet/usage/stats |
| 日志 TTL 清理 | ✅ | LogRetentionDays 定时清理过期 mcp_call_logs |

> **架构要点**:计费代码在顶层 `billing/` 包(因 service→cloud→handler 链,handler 不能 import service);低额度邮件经 `billing.LowQuotaNotifier` 钩子由 service 注入解耦。市场 session 当前按引用行 ID 连接,跨用户共享平台 session 留作 V1.1 优化。

### 8.2 前端 ✅ 已完成(tsc -b + rsbuild build 双绿)

- **基础层**:`types` 补 wallet/redemption/批量定价/克隆/调额类型 + marketplace 定价字段;`lib/billing.ts` 价格与计费状态 helper;`system-config-store` 暴露 billingEnabled/displayCurrency/selfUseModeEnabled/redemptionEnabled/userOwnedServicesEnabled;zh/en 新增 billing/wallet/pricing/redemptionCodes 命名空间并扩展 nav/marketplace/logs/admin.users;侧边栏加 wallet/pricing(主导航)、adminBilling/adminRedemption(管理导航)。
- **新页面**:`/wallet`(额度概览 + 用量统计 + 兑换码卡片 + 消费明细表)、`/pricing`(公开价目表)、`/admin/redemption-codes`(兑换码 CRUD + 批量生成)、`/admin/billing`(计费设置 4 Tab,含分组倍率编辑器)。
- **既有页扩展**:市场 `install→POST /:id/add` 修正 + 价格展示 + 添加按钮;`/admin/marketplace`(原占位符)新建完整管理页(列表/创建/克隆含凭证替换提示/批量定价/删除);服务列表/详情 source=marketplace 徽标 + 只读横幅;调用日志计费列(状态徽标 + 消耗 + tooltip 单价/来源/市场项);管理员用户调额对话框(add/sub/set + 备注)。
- **展示约定**:额度统一按原始 quota 整数(QuotaPerUnit 未公开,与既有页一致);市场价格直接展示 price_per_call 货币值。

### 8.3 V2 占位

充值/在线支付、订阅套餐、用量看板(图表)、工具级精确定价 UI、市场引用 tools_cache 自动同步、余额变更流水表。

