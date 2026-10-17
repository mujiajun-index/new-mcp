# NewMCP 前端设计文档

> 版本: V2.0 | 状态: 草案 | 更新日期: 2026-10-17（视频配置支持 Gemini 原生协议）
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
| `/videogen` | `_authenticated/videogen/index.lazy.tsx` | 视频生成配置列表与任务历史 | 登录 |
| `/videogen/create` | `_authenticated/videogen/create.lazy.tsx` | 新建视频配置与草稿测试 | 登录 |
| `/videogen/:id` | `_authenticated/videogen/$id.lazy.tsx` | 视频配置详情与测试 | 登录 |
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
- 最近调用日志 (过去 24 小时内的最近 6 条)
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
  - Smart 模式说明: 仅暴露 5 个元工具（搜索/查看/执行/批量执行/读资源提示），适合工具多或设备上下文受限的场景
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

### 4.17 管理员 - 系统设置 `/admin/system`
- **Tabs 分区布局**：通用设置 / 身份认证 / 速率限制 / SMTP 邮件 / 系统维护
- **逐字段自动保存**：每个字段 onBlur 时触发 `PUT /admin/settings`，无需手动点保存
- **Switch 组件**：布尔型设置使用 Radix Switch 开关
- **分组限流可视化编辑器**：RateLimitGroupConfig 以表格形式展示，每行一个分组规则（分组名/最大请求数/时间窗口），支持添加/删除
- **敏感字段掩码**：SMTPToken 等字段以 `***` 显示，输入新值时才更新
- **版本信息只读展示**：系统维护 tab 显示当前版本号
- **组件文件**: `features/admin/components/admin-settings-page.tsx`
- **UI 组件**: 复用 `components/ui/switch.tsx`、`components/ui/tabs.tsx`

---

### 4.18 视频生成 `/videogen`

- 列表、创建及详情沿用图片生成页面交互，加入侧栏；状态为“已启用/已禁用”，按钮为“启用/禁用”。启用时自动注册 `videogen_<id>` 内置服务，再在分组中选用。服务和分组显示视频来源标识；虚拟服务编辑或删除入口引导到配置页。
- 配置列表使用与图片生成一致的表格，展示名称、协议、模型、生成能力、启用状态和操作；支持即时搜索名称、协议与模型。移动端沿用紧凑列表行，删除收纳在更多菜单。
- 配置包含协议、模型搜索与手填、能力、图生格式、默认参数 JSON、超时和工具描述。协议默认 `openai_video`，可选 `gemini_video`、`openrouter_video`；New API 的 Wan、豆包图生使用 JSON，Gemini / OpenRouter 图生固定 JSON。Gemini 默认地址为 `https://generativelanguage.googleapis.com/v1beta`，提示原生驼峰参数、时长及分辨率映射，实际请求预览包含模型和 `:predictLongRunning`。API Key 输入留空保留原值，读取通过 `has_api_key` 展示配置情况。
- 获取模型列表只提交协议、地址、凭证等连接字段，不依赖名称、模型或默认参数是否填完。列表、添加/编辑和详情标题统一显示 OpenAI、Gemini、OpenRouter；地址可留空使用对应官方地址，切换协议时替换上一协议的默认地址并保留自定义地址。OpenRouter 查询专用视频模型目录；Gemini 分页查询原生模型目录并筛选视频生成能力。页面仅消费 `id` / `name`，不限制模型参数枚举；默认搜索 `video`，可修改或清空，Gemini 默认词匹配全部视频目录条目，也可直接手填模型 ID。协议、参数和地址示例见[视频生成说明](./VIDEO-GENERATION.md#上游协议与格式)。
- 详情页与图片生成一致，依次展示基础配置、注册工具及测试生成卡片。新建页使用较窄的居中布局，保存后回到列表；详情页使用完整内容宽度。注册工具按已保存的能力展示，工具描述分别保存、恢复默认；`upload_image` 标为系统内置且不可修改。
- 草稿测试无需保存，提供文生/图生切换、单图上传及提示词。测试提交取得任务 ID 后每 5 秒查询，显示进度、生成状态和保存状态；保存失败提供“重试保存”。
- 保存成功显示 `<video controls>`、下载与复制链接。公共 MCP 测试结果组件也能将视频 `resource_link` 渲染为播放器；视频到期和历史保留按后端数据展示。系统设置将“图片存储”统一命名为“文件储存”，图片和视频保留时间允许 0（永久保存）；图片管理和视频详情在 `expires_at=null` 时显示“永久保存”。
- 任务历史使用表格，汇总模型、任务 ID、生成能力、提示词、生成状态/进度、保存状态、创建时间、耗时、过期时间及操作。“生成能力”位于“提示词”左侧，展示文生视频/图生视频及下方调用来源，与图片任务历史一致。“耗时”位于“过期时间”左侧，沿用详情数据，以秒显示并保留一位小数，暂无耗时时显示“—”。过期时间按任务接口返回的实际期限展示，永久保存的视频显示“永久保存”，尚未保存的视频显示“—”；文件过期后的历史记录仍展示原到期时间。列表提示词使用摘要，详情弹窗完整展示提示词并保留换行，长文本可滚动阅读，长连续字符自动换行；保留播放器、完整错误、重试保存及继续跟踪。支持配置过滤和分页，进行中的任务不提供删除操作。窄屏在表格内部横向滚动。管理员设置新增视频大小、文件保留、历史保留及每用户数量上限。
- 生成失败的表格行和详情提供“重试生成”，窗口支持配置选择、提示词、参考图上传及 JSON 参数修正。确认后关闭窗口，刷新原任务详情和历史行，任务 ID、历史数量及当前分页保持不变；下次重试重新获取最新参数快照。旧记录提示补充未保存的输入。重试使用独立请求 ID 去重，保存失败仍使用“重试保存”，提交结果不确定不提供新的生成重试。
- 默认、测试和重试参数共用 JSON 对象解析：空文本或仅空白按 `{}` 处理；不完整 JSON 或数组、`null`、标量会阻止提交，并以当前语言提示具体字段。`npm run test:videogen-params` 覆盖原报错场景及中英文错误提示。
- 任务详情对齐图片任务详情：顶部为模型、生成/保存状态及文生/图生标签，双列信息区展示配置、调用来源（网页测试/MCP 调用）、创建时间、耗时，有重试时展示重试次数。耗时取后端 `duration_ms`，表示创建至停止跟踪的总时长，包含排队、保存及重试等待；进行中或旧记录缺少停止时间时显示“—”。提示词在独立浅色区完整展示并可滚动，视频结果使用边框容器，过期视频显示占位提示；详情底部不展示任务操作栏，复制任务 ID、删除、重试生成、重试保存及继续跟踪统一在历史列表操作，视频资源的播放、下载及复制链接保持可用。弹窗去除内层重复卡片边框，与图片详情采用相同宽度及滚动布局。
- 页面代码位于 `features/videogen/components/`，接口在 `features/videogen/api.ts`；中文、英文文案由现有 i18n 管理。

图片与视频任务详情均在“提示词”标题旁提供“复制提示词”按钮，一键复制完整原始提示词（含换行），不受预览区域滚动范围影响；提示词为空时禁用按钮，复制成功或失败通过通知提示。

图生图与图生视频详情在提示词下方共用 `TaskReferenceImages` 展示“输入图片”，默认仅按输入顺序展示 80×80 的小缩略图，自动换行；点击缩略图弹出详情，展示大图、尺寸/格式/大小、完整链接、复制链接、查看原图及保留时间，图生图 mask 在弹窗中单独标注。接口使用 `reference_images` 数组，图片过期或删除显示占位，旧记录无保存数据时显示说明。视频生成重试更新原行的输入图；进行中的详情随现有轮询刷新。

使用流程、MCP 参数及真实验收限制见[视频生成说明](./VIDEO-GENERATION.md)。

---

### 4.19 管理员 - 图片管理 `/admin/uploads`

- 顶部提供「上传图片」按钮，弹窗以虚线框展示文件选择区域，支持拖拽图片和点击选择，悬停或拖入时高亮，并在选图后预览。点击上传后显示图片地址及复制按钮；上传中禁止重复提交和关闭弹窗，失败可重试，成功后可继续选择下一张图片。
- 上传复用 `POST /vision/upload`（登录 JWT），遵循现有文件大小、配额、去重和图片保留设置，支持 local/S3。图片归属当前管理员账号，界面提示使用该账号的 API Key 调用内置图生图、图生视频工具。
- 上传成功后清空用户筛选、回到第一页并刷新图片列表，现有预览、复制链接、删除和调优预览操作保持可用。保留桌面表格与移动端列表。
- 上传弹窗位于 `features/uploads/components/upload-image-dialog.tsx`；图片地址可用于 `edit_image.images`、`generate_video_from_image.image` 和图片分析，Agent 无需执行上传命令。

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
│   │   │   └── admin/
│   │   │       ├── route.tsx
│   │   │       ├── users.lazy.tsx
│   │   │       ├── marketplace/
│   │   │       └── system.lazy.tsx        ← 系统设置 (Tabs: 通用/认证/限流/SMTP/维护)
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
