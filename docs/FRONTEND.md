# NewMCP 前端设计文档

> 版本: V2.0 | 状态: 草案 | 更新日期: 2026-05-05
> 架构对标: new-api Default 版 (React 19 + TanStack 全家桶)

## 1. 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| React | 19.x | UI 框架 |
| TypeScript | ~5.9.x | 类型安全 |
| Rsbuild | 2.x | 构建工具 (基于 Rspack/Rust) |
| TanStack Router | 1.x | 文件路由 + 类型安全导航 |
| TanStack Query | 5.x | 服务端状态管理 (缓存/去重/重试) |
| Zustand | 5.x | 客户端状态管理 (仅 auth/config 等) |
| Radix UI | latest | 无样式可访问组件原语 |
| shadcn/ui | 3.x | Radix UI + Tailwind 组件集合 |
| Tailwind CSS | 4.x | 原子化样式 |
| Axios | 1.x | HTTP 客户端 |
| i18next | 25.x | 国际化 |
| React Hook Form + Zod | 7.x / 4.x | 表单验证 |
| Lucide React | latest | 图标库 |
| Sonner | latest | Toast 通知 |
| Motion | 12.x | 动画 |
| Monaco Editor | latest | JSON 配置编辑 |
| Bun | latest | 包管理器 (首选) |

---

## 2. 架构概览

### 2.1 状态管理策略

**双轨制**：服务端状态和客户端状态分离管理。

```
┌─────────────────────────────────────────────────────┐
│                    React 组件树                       │
│                                                     │
│   TanStack Query          Zustand                   │
│   ┌──────────────┐       ┌──────────────┐          │
│   │ 服务列表      │       │ 当前用户      │          │
│   │ 分组数据      │       │ 系统配置      │          │
│   │ 市场服务      │       │ UI 偏好       │          │
│   │ 调用日志      │       │ 主题/语言     │          │
│   │ ...          │       │              │          │
│   └──────────────┘       └──────────────┘          │
│   自动缓存/去重/重试      手动管理，持久化            │
└─────────────────────────────────────────────────────┘
```

- **TanStack Query**：所有从后端获取的数据。自动处理缓存、窗口聚焦重新获取、请求去重、错误重试、乐观更新。
- **Zustand**：仅管理纯客户端状态（认证信息、系统配置缓存、UI 偏好）。持久化到 localStorage。

### 2.2 路由架构

使用 TanStack Router 文件路由，路由定义在 `src/routes/` 目录结构中，`routeTree.gen.ts` 自动生成。

```
routes/
├── __root.tsx                          ← 根布局 (Provider 层)
├── _public/                            ← 公开路由组
│   ├── route.tsx                       ← 公开布局
│   ├── sign-in.lazy.tsx                ← 登录
│   └── sign-up.lazy.tsx                ← 注册
├── _authenticated/                     ← 需认证路由组
│   ├── route.tsx                       ← 认证守卫 + AuthenticatedLayout
│   ├── dashboard.lazy.tsx              ← 仪表盘
│   ├── services/                       ← 服务管理
│   │   ├── index.lazy.tsx
│   │   ├── create.lazy.tsx
│   │   └── $id.lazy.tsx
│   ├── groups/                         ← 分组管理
│   │   ├── index.lazy.tsx
│   │   ├── create.lazy.tsx
│   │   └── $id.lazy.tsx
│   ├── connections/                    ← 云端连接
│   │   ├── index.lazy.tsx
│   │   ├── create.lazy.tsx
│   │   └── $id.lazy.tsx
│   ├── vision/                         ← 视觉配置
│   │   ├── index.lazy.tsx
│   │   ├── create.lazy.tsx
│   │   └── $id.lazy.tsx
│   ├── cameras/                        ← 摄像头管理
│   │   ├── index.lazy.tsx
│   │   ├── create.lazy.tsx
│   │   └── $id.lazy.tsx
│   ├── api-keys.lazy.tsx               ← API 密钥
│   ├── marketplace/                    ← 平台市场
│   │   ├── index.lazy.tsx
│   │   └── $id.lazy.tsx
│   ├── settings.lazy.tsx               ← 个人设置
│   └── admin/                          ← 管理员路由 (/admin/*, 含权限守卫)
│       ├── route.tsx                   ← 管理员权限守卫
│       ├── users.lazy.tsx
│       ├── logs.lazy.tsx
│       ├── marketplace/
│       │   ├── index.lazy.tsx
│       │   ├── create.lazy.tsx
│       │   └── $id.lazy.tsx
│       ├── reviews.lazy.tsx
│       └── system.lazy.tsx
└── 500.lazy.tsx                        ← 错误页面
```

**路由守卫模式** (参考 new-api `_authenticated/route.tsx`):

```typescript
// routes/_authenticated/route.tsx
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { getSelf } from '@/lib/api'

let sessionVerified = false

export const Route = createFileRoute('/_authenticated')({
  beforeLoad: async ({ location }) => {
    const { auth } = useAuthStore.getState()

    // 1. 检查 localStorage 缓存
    if (!auth.user) {
      throw redirect({ to: '/sign-in', search: { redirect: location.href } })
    }

    // 2. 首次验证服务端 session
    if (!sessionVerified) {
      const res = await getSelf()
      if (!res?.success) {
        auth.reset()
        sessionVerified = false
        throw redirect({ to: '/sign-in', search: { redirect: location.href } })
      }
      sessionVerified = true
    }
  },
  component: AuthenticatedLayout,
})
```

**管理员路由守卫**:

```typescript
// routes/_authenticated/admin/route.tsx
export const Route = createFileRoute('/_authenticated/admin')({
  beforeLoad: async () => {
    const { auth } = useAuthStore.getState()
    if (auth.user?.role !== 'admin') {
      throw redirect({ to: '/dashboard' })
    }
  },
})
```

### 2.3 Provider 层级

```
<StrictMode>
  <QueryClientProvider client={queryClient}>
    <ThemeProvider>
      <I18nextProvider>
        <RouterProvider router={router} />
      </I18nextProvider>
    </ThemeProvider>
  </QueryClientProvider>
</StrictMode>
```

---

## 3. 页面结构

```
┌─────────────────────────────────────────────┐
│  Header (Logo + 导航 + 用户菜单)             │
├──────┬──────────────────────────────────────┤
│      │                                      │
│ 侧边 │           主内容区域                  │
│ 导航 │                                      │
│      │                                      │
│      │                                      │
│      │                                      │
│      │                                      │
├──────┴──────────────────────────────────────┤
│  Footer (可选)                              │
└─────────────────────────────────────────────┘
```

### 路由表

| 路径 | 路由文件 | 页面 | 权限 |
|------|----------|------|------|
| `/sign-in` | `_public/sign-in.lazy.tsx` | 登录页 | 公开 |
| `/sign-up` | `_public/sign-up.lazy.tsx` | 注册页 | 公开 |
| `/dashboard` | `_authenticated/dashboard.lazy.tsx` | 总览仪表盘 | 登录 |
| `/services` | `_authenticated/services/index.lazy.tsx` | MCP 服务列表 | 登录 |
| `/services/create` | `_authenticated/services/create.lazy.tsx` | 注册新服务 | 登录 |
| `/services/:id` | `_authenticated/services/$id.lazy.tsx` | 服务详情 | 登录 |
| `/groups` | `_authenticated/groups/index.lazy.tsx` | MCP 分组列表 | 登录 |
| `/groups/create` | `_authenticated/groups/create.lazy.tsx` | 创建分组 | 登录 |
| `/groups/:id` | `_authenticated/groups/$id.lazy.tsx` | 分组详情 | 登录 |
| `/connections` | `_authenticated/connections/index.lazy.tsx` | 云端连接列表 | 登录 |
| `/connections/create` | `_authenticated/connections/create.lazy.tsx` | 添加连接 | 登录 |
| `/connections/:id` | `_authenticated/connections/$id.lazy.tsx` | 连接详情 | 登录 |
| `/vision` | `_authenticated/vision/index.lazy.tsx` | 视觉配置列表 | 登录 |
| `/vision/create` | `_authenticated/vision/create.lazy.tsx` | 新建视觉配置 | 登录 |
| `/vision/:id` | `_authenticated/vision/$id.lazy.tsx` | 编辑视觉配置 | 登录 |
| `/cameras` | `_authenticated/cameras/index.lazy.tsx` | 摄像头列表 | 登录 |
| `/cameras/create` | `_authenticated/cameras/create.lazy.tsx` | 添加摄像头 | 登录 |
| `/cameras/:id` | `_authenticated/cameras/$id.lazy.tsx` | 摄像头详情 | 登录 |
| `/api-keys` | `_authenticated/api-keys.lazy.tsx` | API 密钥管理 | 登录 |
| `/settings` | `_authenticated/settings.lazy.tsx` | 个人设置 | 登录 |
| `/marketplace` | `_authenticated/marketplace/index.lazy.tsx` | 平台市场 | 登录 |
| `/marketplace/:id` | `_authenticated/marketplace/$id.lazy.tsx` | 市场服务详情 | 登录 |
| `/admin/users` | `_authenticated/admin/users.lazy.tsx` | 用户管理 | admin |
| `/admin/logs` | `_authenticated/admin/logs.lazy.tsx` | 调用日志 | admin |
| `/admin/marketplace` | `_authenticated/admin/marketplace/index.lazy.tsx` | 市场管理 | admin |
| `/admin/marketplace/create` | `_authenticated/admin/marketplace/create.lazy.tsx` | 上架服务 | admin |
| `/admin/marketplace/:id` | `_authenticated/admin/marketplace/$id.lazy.tsx` | 编辑服务 | admin |
| `/admin/reviews` | `_authenticated/admin/reviews.lazy.tsx` | 审核列表 | admin |
| `/admin/system` | `_authenticated/admin/system.lazy.tsx` | 系统设置 | admin |

---

## 4. 页面设计

### 4.1 登录页 `/sign-in`
- 居中卡片式登录表单
- 用户名 + 密码输入
- "记住我" 复选框
- 登录按钮 + 注册链接

### 4.2 Dashboard `/dashboard`
- 4 个统计卡片: 服务数 / 分组数 / 主动连接数 / 今日调用量
- 服务健康状态列表 (实时显示各 MCP 服务健康/不健康)
- 最近调用日志 (最近 10 条)
- 快捷操作: 注册服务 / 创建分组 / 添加连接

### 4.3 MCP 服务列表 `/services`
- 表格视图: 名称 / 类型 / 服务分类 / 状态 / 健康状态 / 工具数 / 操作
- 筛选器: 服务分类（即时使用 / 自建部署 / 被动接入）/ 传输类型 / 状态 / 关键词搜索
- 批量操作: 启用 / 禁用
- "注册新服务" 按钮

### 4.4 注册新服务 `/services/create`
- 分步表单 (Step Form):
  1. 基本信息: 名称、显示名、描述、**服务分类**
     - 选择服务分类后自动过滤可选传输类型:
       - 即时使用 → SSE / Streamable HTTP / WebSocket
       - 自建部署 → stdio
       - 被动接入 → passive-ws
  2. 传输配置: 选择传输类型 → 动态显示对应配置表单
     - stdio: 命令、参数、环境变量
     - sse/http: URL
     - websocket: URL
  3. 认证配置: 认证类型 → 对应的凭证输入，自动生成请求头
     - 无需认证: 不设置请求头
     - API Key: 输入 key → 自动设置 `X-API-Key` 请求头
     - Bearer Token: 输入 token → 自动设置 `Authorization: Bearer <token>` 请求头
     - 自定义配置: 输入 Key/Value → 自动设置自定义请求头
  4. 连接测试: 显示测试结果，确认后提交

### 4.5 服务详情 `/services/:id`
- 服务信息卡片 (名称、类型、状态、健康)
- 连接配置 (可编辑，JSON 编辑器)
- 工具列表表格 (名称、描述、参数 Schema)
- 健康检查历史
- 操作: 测试连接 / 刷新工具 / 编辑 / 删除

### 4.6 MCP 分组列表 `/groups`
- 卡片视图: 每个分组一张卡片，显示名称、服务数、工具数、端点 URL
- 点击卡片进入详情

### 4.7 分组详情 `/groups/:id`
- 分组信息 (可编辑)
- **暴露模式切换**: Direct 模式 / Smart 模式 (Radio 或 Switch 组件)
  - Direct 模式说明: 直接暴露所有工具，适合工具少的场景
  - Smart 模式说明: 仅暴露 3 个元工具（搜索/查看/执行），适合工具多或设备上下文受限的场景
- 端点信息卡片: Streamable HTTP URL / WebSocket URL / 连接配置 JSON (一键复制)
- 已添加服务列表 (可拖拽排序、启用/禁用、移除)
- 聚合工具列表 (带命名空间前缀、可单独启用/禁用/重命名)
  - "管理工具" 按钮: 点击展开工具管理面板
  - 工具管理面板: 按服务分组展示所有工具，支持搜索过滤
  - 每个工具可独立启用/禁用（勾选框），支持按服务批量启用/禁用
  - 批量保存变更，调用 `PUT /groups/:id/tools/batch` 接口
- Smart 模式下额外显示: 搜索引擎状态、已索引文档数
- "添加服务" 按钮 (弹出服务选择器)

### 4.8 云端连接列表 `/connections`
- 表格: 名称 / 云平台类型 / 绑定 API Key / 连接状态(已连接/断开/错误) / 最后连接时间
- 操作: 连接/断开/编辑/删除
- "添加连接" 按钮

### 4.9 添加云端连接 `/connections/create`
- 选择云平台类型: 小智 / 自定义 WSS / SSH
- 根据平台类型动态显示配置表单:
  - 小智: 粘贴 WSS URL（自动解析 JWT 获取 Agent ID 和过期时间）
  - 自定义 WSS: 输入 URL + 自定义请求头
  - SSH: 输入主机 / 端口 / 用户名 / 认证方式（密码或密钥）
- **绑定 API Key**: 选择一个已有 API Key（下拉列表，显示 Key 名称和关联分组）
  - API Key 的 `permissions.groups` 决定此连接可暴露哪些 MCP 分组/服务
  - 展示该 API Key 关联的分组列表预览
- 自动连接开关
- 测试连接

### 4.10 视觉配置 `/vision`
- 配置列表卡片: 模型提供商 / 模型名 / 状态
- 添加配置: 选择提供商 → 填写 API Key / 端点 / 模型名 → 测试 → 保存

### 4.11 摄像头管理 `/cameras`
- 摄像头列表: 名称 / 源类型 / 帧率 / 绑定视觉模型 / 状态
- 添加摄像头: 配置源 URL / 帧率 / 分辨率 / 选择视觉配置
- 摄像头详情: 实时预览 + 最新分析结果

### 4.12 平台市场 `/marketplace`
- 卡片网格视图: 每个市场服务一张卡片
  - 卡片信息: 名称 / 描述 / 类型标签（即用型/源码型）/ 工具数 / 使用人数 / 评分
  - 即用型卡片: 显示"一键添加"按钮
  - 源码型卡片: 显示"查看部署指南"按钮
- 筛选器: 服务类型（即用型/源码型）/ 分类标签 / 关键词搜索
- 排序: 最热 / 最新 / 评分最高

### 4.13 市场服务详情 `/marketplace/:id`
- 服务信息卡片: 名称、描述、图标、类型（即用型/源码型）、作者、版本
- 工具列表: 该服务提供的所有 MCP 工具
- **即用型**: "添加到我的服务"按钮 → 弹窗填写 API Key / 认证信息 → 一键创建
- **源码型**: 部署指南区域（仓库地址、安装命令、配置模板、环境变量说明）→ "我已部署，去注册"按钮跳转到注册页
- 评论区: 用户评分和评论（后期）
- 收藏按钮（后期）

### 4.14 管理员 - 市场服务管理 `/admin/marketplace`
- 表格: 名称 / 类型 / 状态（已上架/已下架）/ 使用人数 / 创建时间 / 操作
- "上架新服务"按钮
- 操作: 编辑 / 上架/下架 / 删除

### 4.15 管理员 - 上架市场服务 `/admin/marketplace/create`
- 选择服务类型: 即用型 / 源码型
- **即用型配置**: 关联已有的 MCP 服务配置（URL、传输类型）+ 认证说明 + 使用文档
- **源码型配置**: 仓库地址 + 安装说明 + 配置模板 + 环境变量说明 + 部署文档
- 通用信息: 名称、描述、图标、分类标签、版本号
- 上架后所有用户可在市场看到

### 4.16 管理员 - 上架审核列表 `/admin/reviews`（后期）
- 表格: 申请人 / 服务名称 / 提交时间 / 审核状态 / 操作
- 审核操作: 查看详情 / 通过 / 拒绝（填写理由）

---

## 5. 前端项目结构

```
web/
├── public/
│   └── favicon.svg
├── src/
│   ├── components/                  # 通用组件
│   │   ├── ui/                      # shadcn/ui 组件 (Radix UI + Tailwind)
│   │   │   ├── button.tsx
│   │   │   ├── card.tsx
│   │   │   ├── dialog.tsx
│   │   │   ├── dropdown-menu.tsx
│   │   │   ├── form.tsx
│   │   │   ├── input.tsx
│   │   │   ├── select.tsx
│   │   │   ├── table.tsx
│   │   │   ├── tabs.tsx
│   │   │   ├── toast.tsx
│   │   │   └── ...
│   │   └── layout/                  # 布局组件
│   │       ├── authenticated-layout.tsx
│   │       ├── app-sidebar.tsx
│   │       ├── header.tsx
│   │       └── navigation-progress.tsx
│   ├── features/                    # 功能模块 (组件+API+hooks 一体)
│   │   ├── auth/
│   │   │   ├── components/
│   │   │   │   ├── sign-in-form.tsx
│   │   │   │   └── sign-up-form.tsx
│   │   │   └── api.ts
│   │   ├── services/
│   │   │   ├── components/
│   │   │   │   ├── service-card.tsx
│   │   │   │   ├── service-table.tsx
│   │   │   │   ├── transport-config-form.tsx
│   │   │   │   ├── auth-config-form.tsx
│   │   │   │   ├── connection-tester.tsx
│   │   │   │   └── tool-table.tsx
│   │   │   └── api.ts
│   │   ├── groups/
│   │   │   ├── components/
│   │   │   │   ├── group-card.tsx
│   │   │   │   ├── expose-mode-switch.tsx
│   │   │   │   ├── endpoint-info.tsx
│   │   │   │   ├── service-selector.tsx
│   │   │   │   └── aggregated-tool-table.tsx
│   │   │   └── api.ts
│   │   ├── connections/
│   │   │   ├── components/
│   │   │   │   ├── connection-table.tsx
│   │   │   │   ├── platform-config-form.tsx
│   │   │   │   └── api-key-binding.tsx
│   │   │   └── api.ts
│   │   ├── vision/
│   │   │   ├── components/
│   │   │   │   └── vision-provider-form.tsx
│   │   │   └── api.ts
│   │   ├── cameras/
│   │   │   ├── components/
│   │   │   │   ├── camera-table.tsx
│   │   │   │   ├── camera-source-form.tsx
│   │   │   │   └── camera-preview.tsx
│   │   │   └── api.ts
│   │   ├── marketplace/
│   │   │   ├── components/
│   │   │   │   ├── marketplace-card.tsx
│   │   │   │   ├── market-type-tag.tsx
│   │   │   │   └── install-dialog.tsx
│   │   │   └── api.ts
│   │   ├── api-keys/
│   │   │   ├── components/
│   │   │   │   └── api-key-table.tsx
│   │   │   └── api.ts
│   │   ├── dashboard/
│   │   │   ├── components/
│   │   │   │   ├── stats-cards.tsx
│   │   │   │   ├── health-status-list.tsx
│   │   │   │   └── recent-logs.tsx
│   │   │   └── api.ts
│   │   └── admin/
│   │       ├── components/
│   │       │   ├── user-table.tsx
│   │       │   ├── log-viewer.tsx
│   │       │   └── review-table.tsx
│   │       └── api.ts
│   ├── routes/                      # TanStack Router 文件路由
│   │   ├── __root.tsx
│   │   ├── _public/
│   │   │   ├── route.tsx
│   │   │   ├── sign-in.lazy.tsx
│   │   │   └── sign-up.lazy.tsx
│   │   ├── _authenticated/
│   │   │   ├── route.tsx
│   │   │   ├── dashboard.lazy.tsx
│   │   │   ├── services/
│   │   │   ├── groups/
│   │   │   ├── connections/
│   │   │   ├── vision/
│   │   │   ├── cameras/
│   │   │   ├── api-keys.lazy.tsx
│   │   │   ├── marketplace/
│   │   │   ├── settings.lazy.tsx
│   │   │   └── _admin/
│   │   └── 500.lazy.tsx
│   ├── lib/                         # 核心工具
│   │   ├── api.ts                   # Axios 实例 + 拦截器 + 请求去重
│   │   ├── handle-server-error.ts   # 统一错误处理
│   │   └── utils.ts                 # cn() 等工具函数
│   ├── stores/                      # Zustand (仅客户端状态)
│   │   ├── auth-store.ts            # 认证状态 (localStorage 持久化)
│   │   └── system-config-store.ts   # 系统配置缓存
│   ├── hooks/                       # 通用自定义 Hooks
│   │   └── use-mcp-service.ts       # MCP 相关复用逻辑
│   ├── i18n/                        # 国际化
│   │   ├── config.ts
│   │   └── locales/
│   │       ├── zh.json
│   │       └── en.json
│   ├── types/                       # TypeScript 类型
│   │   └── index.ts
│   ├── context/                     # React Context
│   │   └── theme-provider.tsx       # 主题 (亮/暗/系统)
│   ├── styles/
│   │   └── index.css                # Tailwind 入口 + CSS 变量
│   ├── main.tsx                     # 入口
│   └── routeTree.gen.ts             # 自动生成，勿手动编辑
├── index.html
├── rsbuild.config.ts
├── tsconfig.json
├── components.json                  # shadcn/ui 配置
├── .prettierrc
└── package.json
```

---

## 6. API 层设计

### 6.1 Axios 实例 (`lib/api.ts`)

```typescript
import axios from 'axios'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'

// 请求去重：防止并发重复 GET
const pendingRequests = new Map<string, AbortController>()

function getRequestKey(config: any): string {
  return `${config.method}:${config.url}?${JSON.stringify(config.params)}`
}

export const api = axios.create({
  baseURL: '',
  withCredentials: true,
  headers: { 'Cache-Control': 'no-store' },
})

// 请求拦截器：去重 + 用户标识
api.interceptors.request.use((config) => {
  if (config.method === 'get' && !config.disableDuplicate) {
    const key = getRequestKey(config)
    if (pendingRequests.has(key)) {
      const controller = new AbortController()
      controller.abort()
      return { ...config, signal: controller.signal }
    }
    const controller = new AbortController()
    config.signal = controller.signal
    pendingRequests.set(key, controller)
  }
  return config
})

// 响应拦截器：统一错误处理
api.interceptors.response.use(
  (response) => {
    const key = getRequestKey(response.config)
    pendingRequests.delete(key)

    if (response.config.skipErrorHandler) return response

    const { success, message, data } = response.data
    if (success === false) {
      toast.error(message || '请求失败')
      return Promise.reject(new Error(message))
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      useAuthStore.getState().auth.reset()
      toast.error('会话已过期')
      window.location.href = '/sign-in'
    }
    return Promise.reject(error)
  }
)
```

### 6.2 功能模块 API 示例 (`features/services/api.ts`)

```typescript
import { api } from '@/lib/api'

export async function getServices(params?: ListParams) {
  const res = await api.get('/api/services', { params })
  return res.data
}

export async function getService(id: number) {
  const res = await api.get(`/api/services/${id}`)
  return res.data
}

export async function createService(data: CreateServiceDTO) {
  const res = await api.post('/api/services', data)
  return res.data
}

export async function testConnection(id: number) {
  const res = await api.post(`/api/services/${id}/test`)
  return res.data
}

export async function refreshTools(id: number) {
  const res = await api.post(`/api/services/${id}/refresh-tools`)
  return res.data
}
```

### 6.3 组件中使用 TanStack Query

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getServices, createService } from '@/features/services/api'

// 查询
function ServiceListPage() {
  const { data, isLoading } = useQuery({
    queryKey: ['services'],
    queryFn: getServices,
  })
  // ...
}

// 变更 + 自动刷新缓存
function CreateServicePage() {
  const queryClient = useQueryClient()
  const mutation = useMutation({
    mutationFn: createService,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['services'] })
      toast.success('服务创建成功')
    },
  })
  // ...
}
```

---

## 7. 状态管理 (Zustand)

Zustand 仅管理纯客户端状态，不存放服务端数据。

```typescript
// stores/auth-store.ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface AuthUser {
  id: number
  username: string
  role: 'user' | 'admin'
}

interface AuthState {
  auth: {
    user: AuthUser | null
    setUser: (user: AuthUser) => void
    reset: () => void
  }
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      auth: {
        user: null,
        setUser: (user) => set({ auth: { user } }),
        reset: () => set({ auth: { user: null } }),
      },
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({ auth: { user: state.auth.user } }),
    }
  )
)
```

```typescript
// stores/system-config-store.ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface SystemConfig {
  systemName: string
  theme: 'light' | 'dark' | 'system'
}

interface SystemConfigState {
  config: SystemConfig
  setConfig: (config: Partial<SystemConfig>) => void
}

export const useSystemConfigStore = create<SystemConfigState>()(
  persist(
    (set) => ({
      config: { systemName: 'NewMCP', theme: 'system' },
      setConfig: (partial) =>
        set((state) => ({ config: { ...state.config, ...partial } })),
    }),
    { name: 'system-config' }
  )
)
```

---

## 8. 构建配置

### 8.1 Rsbuild (`rsbuild.config.ts`)

```typescript
import path from 'path'
import { fileURLToPath } from 'url'
import { defineConfig } from '@rsbuild/core'
import { pluginReact } from '@rsbuild/plugin-react'
import { tanstackRouter } from '@tanstack/router-plugin/rspack'

const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig(({ envMode }) => {
  const isProd = envMode === 'production'

  return {
    plugins: [pluginReact()],
    splitChunks: {
      preset: 'default',
      cacheGroups: {
        'vendor-react': {
          test: /node_modules[\\/](react|react-dom)[\\/]/,
          name: 'vendor-react',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-radix': {
          test: /node_modules[\\/]@radix-ui[\\/]/,
          name: 'vendor-radix',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
        'vendor-tanstack': {
          test: /node_modules[\\/]@tanstack[\\/]/,
          name: 'vendor-tanstack',
          chunks: 'all',
          priority: 0,
          enforce: true,
        },
      },
    },
    source: {
      entry: { index: './src/main.tsx' },
    },
    resolve: {
      alias: { '@': path.resolve(__dirname, './src') },
    },
    html: { template: './index.html' },
    server: {
      host: '0.0.0.0',
      proxy: {
        '/api': { target: 'http://localhost:8080', changeOrigin: true },
        '/mcp': { target: 'http://localhost:8080', changeOrigin: true },
      },
    },
    output: {
      distPath: { root: 'dist' },
    },
    performance: {
      removeConsole: isProd ? ['log'] : false,
    },
    tools: {
      rspack: {
        plugins: [
          tanstackRouter({
            target: 'react',
            autoCodeSplitting: isProd,
          }),
        ],
      },
    },
  }
})
```

### 8.2 Go 后端嵌入 (`cmd/server/main.go`)

```go
//go:embed web/dist
var buildFS embed.FS
//go:embed web/dist/index.html
var indexPage []byte
```

---

## 9. 共享组件

| 组件 | 说明 |
|------|------|
| `AuthenticatedLayout` | 认证后布局 (Sidebar + Header + Content) |
| `NavigationProgress` | 路由切换顶部进度条 |
| `ServiceStatusBadge` | 服务健康状态 (绿/红/灰) |
| `TransportTypeTag` | 传输类型标签 (stdio/SSE/WS/HTTP) |
| `ToolTable` | MCP 工具列表 (带启用/禁用开关) |
| `JsonEditor` | JSON 配置编辑器 (Monaco) |
| `ConnectionTester` | MCP 连接测试按钮 + 结果展示 |
| `EndpointInfo` | MCP 端点信息卡片 (URL + 复制按钮) |
| `ServiceSelector` | 服务选择器 (弹窗/穿梭框) |
| `ExposeModeSwitch` | 暴露模式切换 (Direct / Smart) |
| `ConnectionStatusBadge` | 连接状态 (已连接/断开/错误) |
| `CopyButton` | 一键复制按钮 |
| `MarketTypeTag` | 市场服务类型标签 (即用型/源码型) |
