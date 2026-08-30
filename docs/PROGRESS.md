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
 1. 管理员上架 MCP 服务到市场 POST /admin/marketplace/clone ✅(从自有服务克隆,唯一上架入口)
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
 5. AI Agent 通过 MCP 端点调用 vision.analyze_image ✅（describe_scene 已彻底下线，2026-08-15：单一通用工具 + prompt 参数，旧名调用返回错误）
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
| 详情页工具测试计费 | ✅ | 市场服务工具测试放开(原一刀切拒绝),复用 BillingService 同口径:定价解析→预扣→成功确认/失败退款;ApiKeyID=0(无 Key,仅用户额度约束,Key 预算操作全跳过);管理员同计费(ChargeAdmin);余额不足/未定价拒;结算写手动测试日志计费列;资源/提示测试同放开且不计费(网关 read/get 免费口径);`marketplace_call_test.go` 覆盖 charged/refunded/blocked/admin/free/开关关闭/资源/提示 |
| 引用式安装 | ✅ | POST /marketplace/:id/add(空 config + marketplace 哨兵 transport + 去重) |
| 自有服务守卫 | ✅ | UserOwnedServicesEnabled=false 时禁 source=user 创建/调用 |
| 管理员定价/批量/克隆 | ✅ | 非自用模式显式定价门控;PUT /admin/marketplace/pricing/batch;POST /admin/marketplace/clone;config_template 加密落库 |
| 兑换码 | ✅ | admin CRUD + 用户 POST /redemptions/redeem(RedemptionEnabled 开关) |
| 管理员调额 | ✅ | POST /admin/users/:id/quota(add/sub/set + canManageTargetRole + 审计) |
| 钱包/用量 | ✅ | GET /wallet、/wallet/billing、/wallet/usage/stats |
| 日志 TTL 清理 | 已移除 | 对齐 reference/new-api,移除 LogRetentionDays 设置与定时清理任务;日志持久保留,需手动清理。视觉图片请求参数落库前脱敏 |

> **架构要点**:计费代码在顶层 `billing/` 包(因 service→cloud→handler 链,handler 不能 import service);低额度邮件经 `billing.LowQuotaNotifier` 钩子由 service 注入解耦。市场 session 当前按引用行 ID 连接,跨用户共享平台 session 留作 V1.1 优化。

### 8.2 前端 ✅ 已完成(tsc -b + rsbuild build 双绿)

- **基础层**:`types` 补 wallet/redemption/批量定价/克隆/调额类型 + marketplace 定价字段;`lib/billing.ts` 价格与计费状态 helper;`system-config-store` 暴露 billingEnabled/displayCurrency/selfUseModeEnabled/redemptionEnabled/userOwnedServicesEnabled;zh/en 新增 billing/wallet/pricing/redemptionCodes 命名空间并扩展 nav/marketplace/logs/admin.users;侧边栏加 wallet/pricing(主导航)、adminBilling/adminRedemption(管理导航)。
- **新页面**:`/wallet`(额度概览 + 用量统计 + 兑换码卡片 + 消费明细表)、`/pricing`(公开价目表)、`/admin/redemption-codes`(兑换码 CRUD + 批量生成)、`/admin/billing`(计费设置 4 Tab,含分组倍率编辑器)。
- **既有页扩展**:市场 `install→POST /:id/add` 修正 + 价格展示 + 添加按钮;`/admin/marketplace`(原占位符)新建完整管理页(列表/创建/克隆含凭证替换提示/批量定价/删除);服务列表/详情 source=marketplace 徽标 + 只读横幅;调用日志计费列(状态徽标 + 消耗 + tooltip 单价/来源/市场项);管理员用户调额对话框(add/sub/set + 备注)。
- **展示约定**:额度统一按原始 quota 整数(QuotaPerUnit 未公开,与既有页一致);市场价格直接展示 price_per_call 货币值。

### 8.3 V2 占位

充值/在线支付、订阅套餐、用量看板(图表)、工具级精确定价 UI、市场引用 tools_cache 自动同步、余额变更流水表。


---

## 9. Smart 模式批量执行 mcp.execute_batch(2026-08-22)

> 设计动机:串行工具循环的客户端(小智等)执行 N 个独立调用时,时延为各项之和、
> 轮次为 N;批量元工具把并发决策移到网关内。Anthropic Cookbook 官方亦推荐 batch
> 元工具模式诱导模型并行调用。

### 9.1 后端 ✅ 已完成(build + handler 单测通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 元工具定义 `smart/meta_tools.go` | ✅ | mcp.execute_batch 双形态入参:`calls`(混合工具)/`tool_id`+`arguments_list`(同工具扇出短形态,面向 flash 级模型:少一层嵌套、不逐项重复 tool_id);`timeout_ms` 整批统一;maxItems=10;mcp.execute 描述加互指;instructions 同步 |
| 执行路径复用 | ✅ | 单项执行抽为 `executeOne`,单次/批量共用(作用域校验/虚拟工具/计费 A+B/超时完全一致) |
| 并发控制 | ✅ | WaitGroup + 信号量,并发上限 5;批量上限 10(schema+服务端双兜底) |
| 结果聚合 | ✅ | 首块汇总 + 每项 `[index] tool_id` 头块 + 上游 content 原样透传(image 等类型不变);上游 isError 计为该项失败;全失败才 isError:true;缺 content/不可解析退化为截断原文 |
| 计费 | ✅ | 逐项预扣/确认/退款;幂等键哈希部分带批内序号(`tool_id#i`)防批内相同两项漏扣 |
| 日志 | ✅ | 每项一条(method=mcp.execute_batch,service/tool/分组/计费列按项归属);请求数只递增一次;校验失败回落单条日志(-32602) |
| 测试 | ✅ | `execute_batch_test.go`:聚合格式(全成/部分败/全败/上游 isError/无 content 兜底)+ 校验路径 |

### 9.2 文档与测试设施 ✅

MCP-PROTOCOL.md(§3.5 新小节,原 3.5 mcp.read 顺延 3.6)、API.md、README 中英、
PRD、ARCHITECTURE、COMMERCIALIZATION(计费口径表)、DATABASE(逐项日志)、
XIAOZHI-INTEGRATION(分发 switch)、TEST-GUIDE 同步;smart_gateway.py 测试实例
补 mcp_execute_batch(asyncio.gather 并发)。

## 10. 市场 stdio 共享/独占进程(2026-08-29)

> 设计动机:市场 stdio 条目原实现按引用行键控会话,N 个安装用户即 N 个平台子
> 进程,内存随安装数线性增长。拆出共享/独占二分(D17):无状态工具默认共享
> (全平台一个进程),记忆存储类有状态服务选独占。

### 10.1 后端 ✅ 已完成(build + vet + test 通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 数据模型 | ✅ | `marketplace_items.isolated_process` 反向命名(false=共享/true=独占),无 DB default(bool 规范),存量行零值=共享零回填;仅 stdio 条目消费,非 stdio 克隆强制 false |
| 会话池复合键 | ✅ | `sessions map[sessionKey]*McpSession`(行键/条目键);`svc.SharedProcess` 内存态标记(gorm:"-")由两处 materialize 从条目反算,市场行必先物化再入池;卸载 Remove(行键)不误杀共享进程;并发首连沿用双检锁 |
| 模式切换 | ✅ | UpdateItem 的 isolated_process 变更并入 ConfigTemplate 同路径 `RemoveByMarketplaceItem` 踢会话,按新模式重建 |
| 进程视图/启停 | ✅ | `service/marketplace_process.go` + `GET/POST /admin/marketplace/:id/process(/control)`:共享=条目键控单快照+预热(start 用 ID=0 内存行,不落库);独占=**服务端分页**枚举(默认每页 18 条/上限 100,`username` 参数 LIKE 匹配用户名/服务名)+ 全量运行实例资源概述(一次进程树扫描)+用户名仅对当前页反查——万级安装不整表返回、不构造全量用户 IN 列表(SQLite 32766 参数悬崖);按行启停(itemRefService 校验归属,用户禁用行拒绝拉起) |
| 归属不受影响 | ✅ | 日志/计费恒按调用者引用行 + marketplace_item_id 落账;条目平台健康(connectedItems 按会话 item 归属)两种模式同口径 |

### 10.2 前端 ✅ 已完成(tsc -b + rsbuild build 双绿)

| 模块 | 状态 | 说明 |
|------|------|------|
| 克隆上架对话框 | ✅ | 源服务为 stdio 才显示「进程模式」段选(默认共享)+ 场景提示文案 |
| 市场管理详情页 | ✅ | stdio 平台托管项新增「进程管理」卡:共享=服务详情同款 4 格资源快照+电源弹框启停;独占=「查看安装实例」弹窗(总览风格:总进程/内存/CPU 三概述卡(服务端全量运行实例合计)+ 总览同款工具栏(左标题/右用户名筛选+条数)+ 总览 stdio 同款实例卡网格,**服务端分页**每页 18 条,弹窗开着时随 5s 轮询刷新、页码越界自动夹紧);段选切换模式(confirm 确认) |
| 总览视图分页 | ✅ | /services/overview 卡片网格客户端本地分页(每页 18 条,LocalPager 共享控件):筛选/搜索变化回第 1 页,轮询刷新夹紧当前页;仅一页时不显示翻页条 |
| 用户侧市场详情 | ✅ | 平台托管卡内展示进程模式一行(stdio 才显示),安装记忆类服务前可知是否独占 |
| i18n | ✅ | marketplace.processMode/modeShared/modeIsolated 等中英;启停文案复用 services.process* |

### 10.3 文档 ✅

COMMERCIALIZATION(D17 决策行、§4.3 ALTER+进程模式条目、§6.1 会话键控、§17 风险表)、
API.md(clone/update 请求体 + 两个 process 端点)、DATABASE(§2.12 列)、
ARCHITECTURE(§4.2 会话池复合键)同步。

## 11. 市场服务条目级定价(2026-08-29)

> 设计动机:市场服务此前仅服务级统一价;`mcp_tool_prices` 表与三级解析虽已存在
> (V1 落地)但无管理入口;资源/提示读取完全免费。本次把定价粒度扩展为**条目级**
> (工具/资源/提示逐条设价),管理端可视化设价,资源/提示**默认免费**、有条目价
> 才计费(与用户确认:测试按钮同口径扣费;用户侧市场详情页展示条目价)。

### 11.1 后端 ✅ 已完成(build + vet + test 通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 数据模型 | ✅ | `mcp_tool_prices` 加 `kind`(tool/resource/prompt,default:'tool' 为 DDL 必需——Postgres 非空表加 NOT NULL 列需默认值);`tool_name` 255→512(资源上游 URI);唯一索引 `idx_item_tool`→`idx_item_kind_name`(同名工具与提示可并存),旧索引启动迁移 DROP(model/main.go GORM Migrator 三方言);`GetToolPrice`→`GetEntryPrice(itemID,kind,name)` |
| 定价解析 | ✅ | `billing.ResolveMarketplaceEntryPrice(itemID,kind,name,group)`:`kind=tool` 走原三级链(命中 scope 仍 'tool');`resource/prompt` 仅条目一级,**未命中→免费不回退服务价不报错**,命中 scope='entry'(日志新值);缓存键改 `kind+"\x00"+条目名` |
| 网关计费 | ✅ | `preConsumeBilling` 加 kind 参数;resources/read(条目键=上游 URI)、prompts/get(条目键=裸提示名)、Smart mcp.read(幂等键复用外层)接入预扣→Confirm/Refund 两段式;**顺手修顺序 bug**:免费判断原遮蔽 ErrPriceNotConfigured 判断(未定价 blocked 为死代码);`recordConsumeLog` 写计费列 + **RequestID 列改写计费幂等键**(HasChargedRequest 防重扣依赖该列) + 归属回填以 nil 守卫 |
| 管理 API | ✅ | `PUT /admin/marketplace/:id/entry-prices` **全量替换**(SetItemEntryPrices):条目须在快照内(模板拒收)、per_call 须 price>0、(kind,name) 去重、事务删旧插新、失效缓存;admin/公开/克隆详情均暴露 `entry_prices` |
| 手动测试 | ✅ | service.go 抽出 `manualEntryPreConsume`/`manualEntryFinalize` 公共外壳(callMarketplaceToolTested 重构为调用 helper,行为不变);新增 `testMarketplaceEntry`:ReadResource/GetPrompt 市场分支同口径计费,无条目价免费 |
| 测试 | ✅ | `billing/pricing_entry_test.go`(包首个测试:工具三级链回归/资源提示命中 scope=entry/未命中免费/服务未定价仅工具报错/同 item 同名工具与提示并存/缓存失效即时生效);`service/marketplace_entry_test.go`(真实 MCP 上游:资源/提示 charged·entry、无价免费、失败退款、余额不足 blocked、工具条目价覆盖服务价、SetItemEntryPrices 校验与全量替换/清空/详情暴露);既有 marketplace_call_test 全量回归通过 |

### 11.2 前端 ✅ 已完成(tsc -b + rsbuild build 双绿)

| 模块 | 状态 | 说明 |
|------|------|------|
| 类型与 API | ✅ | `MarketplaceEntryKind/MarketplaceEntryPrice` + `MarketplaceDetail.entry_prices`;`adminSetEntryPrices`(PUT) |
| 组件插槽 | ✅ | `ResourceItemCard`/`PromptItemCard` 加 `action` prop(名称行右侧,与 ToolItem 同款) |
| 管理详情页 | ✅ | 新组件 `entry-pricing.tsx`:`useEntryPricingDraft`(本地覆盖仅记改过的条目,未改随服务端数据走)+`EntryPriceControl`(工具三态:继承服务价/免费/自定义价;资源提示两态)+`EntryPricingBar`(待保存 N 项/取消/保存);三列表逐行注入右侧控件,**templates 不给控件**;定价说明 hint |
| 用户市场详情 | ✅ | 工具行右侧:条目价高亮徽标/未设弱化显示服务统一价(title 提示);资源/提示行:条目价徽标/未设"免费";模板无 |
| i18n | ✅ | marketplace.entryPricingHint/entryModeInherit/entryModeCustom/entrySave/entryPending/entryPriceRequired/servicePriceHint + billing.scopeEntry 中英 |

### 11.3 文档 ✅

COMMERCIALIZATION V1.9(变更摘要⑱、§4.4 表结构、§5.2 条目级解析、§5.5 入口表、
§6.7 计费口径扩展、手动测试口径、V2 任务17 标记已提前实现)、
API.md(entry-prices 端点)、DATABASE(§2.16/ER/索引)同步。

## 12. 条目定价三态统一:资源/提示支持继承服务价(2026-08-30)

> 设计动机:§11 落地时资源/提示只有"免费/自定义价"两态,与工具三态不一致;
> 市场管理详情页资源/提示卡片的展开箭头也在名称行末尾(工具在名称前)。本次
> 全部与工具统一——三态(继承服务价/免费/自定义价)、卡片样式(箭头名称前+
> 固定宽占位+action 右侧),仅**缺省不同:工具缺省继承服务价,资源/提示缺省
> 免费**(与用户确认)。

### 12.1 后端 ✅ 已完成(build + vet + test 通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 定价解析 | ✅ | `mcp_tool_prices.billing_type` 新值 `inherit`(价格恒 0,无 DDL 变更,size:16 容纳):条目命中 inherit 行 → 三种 kind 都回退服务级链(与缺省工具同款);资源/提示**缺省**(无行)仍免费不回退;`ResolveMarketplaceEntryPrice` 显式继承后非自用模式服务级未定价同报 ErrPriceNotConfigured |
| 管理 API | ✅ | dto `MarketplaceEntryPrice.BillingType` 放开 `oneof=free per_call inherit`;`SetItemEntryPrices` 接受 inherit 行(价格强制归零存储),其余校验不变 |
| 测试 | ✅ | `billing/pricing_entry_test.go` 新增 `TestResolveEntryInherit`(三 kind 显式继承→服务价 scope=service/免费服务继承→免费/未定价服务继承→报错·缺省资源放行);`service/marketplace_entry_test.go` 新增 `resource_inherits_service_price` 全链路(真实上游 charged·service·5000 quota)+ inherit 行归零存储校验 |

### 12.2 前端 ✅ 已完成(tsc -b 通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 条目卡片 | ✅ | `mcp-items.tsx` ResourceItemCard/PromptItemCard 重排为 ToolItem 同款:展开箭头名称前(固定宽占位保持无描述条目对齐)、action 名称行右侧、描述点击展开 |
| 定价控件 | ✅ | `EntryPriceControl` 三 kind 统一三态下拉(去掉工具才显示"继承服务价"的条件,`kind` prop 移除);`buildServerDraft` 识别 inherit 行回填;`buildPayload`:工具继承不落行(缺省即继承),资源/提示继承落 `inherit` 行(其缺省是免费,须持久化) |
| 用户市场详情 | ✅ | `entryPriceBadge`:inherit 行与未设价同款弱化展示**服务统一价**(资源/提示缺省仍弱化"免费") |
| i18n | ✅ | `entryPricingHint` 中英更新(说明缺省规则+显式继承) |

### 12.3 文档 ✅

COMMERCIALIZATION(§4.4/§5.2/§5.5/§6.7/手动测试口径)、API.md(entry-prices 端点)、
DATABASE(§2.16/定价解析 ER)同步。


## 13. 失败计费策略:客户端错误计费开关接线修复(2026-08-30)

> Bug:计费设置内「客户端错误计费」开关关闭时,调用失败仍被计费(上游 key
> 错误、上游余额不足等)。根因:结算成败判定是 `err == nil`——这类失败经
> MCP 工具层结果上报(`isError=true`,传输层成功),被当成成功 Confirm 扣费;
> 且 `ChargeOnClientError`/`ChargeOnTimeout` 两个选项定义了却从未接线。

### 13.1 后端 ✅ 已完成(build + vet + test 通过)

| 模块 | 状态 | 说明 |
|------|------|------|
| 失败计费策略 | ✅ | 新增 `billing/charge_policy.go`:`CallFailure{Failed, ClientError, Timeout}` 分类(err / 结果内 isError / context 截止 / JSON-RPC 客户端码 -32700/-32600/-32601/-32602)+ `Charge()` 应用选项 + `ShouldChargeCall`/`ToolResultIsError` 便捷入口 |
| 结算判定 | ✅ | 网关 4 处(tools/call 直连与 Smart execute、resources/read、prompts/get)+ 手动测试工具路径,`err == nil` 判定全部改为 `ShouldChargeCall`(经 priceKind* 同款包名遮蔽别名 `shouldChargeCall`/`toolResultIsError`);`finalizeBilling` 形参更名 `charge` |
| 语义 | ✅ | 成功恒计费;失败默认退款;`ChargeOnClientError=true` → 客户端侧错误(JSON-RPC 客户端码 + 结果内 isError 工具层失败)计费;`ChargeOnTimeout=true` → 超时计费;平台侧失败(上游内部错误/密钥失效/上游余额不足/传输故障)恒退款 |
| 测试 | ✅ | `billing/charge_policy_test.go`(4 个:isError 默认退款·开关计费、JSON-RPC 客户端码/内部错误、超时开关、isError 解析);`service/marketplace_call_test.go` 新增 `startFailingToolMCPServer`(上游 echo 恒返回 isError=true)+ `refunded_on_tool_level_error` / `charged_on_tool_level_error_when_option_on` 全链路子测试 |

### 13.2 文档 ✅

COMMERCIALIZATION §6.6 失败边界表(新增"工具层失败 isError"行,明确两类开关的
实际分类口径)、§6.2 手动测试同口径说明、选项表 ChargeOnClientError/ChargeOnTimeout
说明更新。
