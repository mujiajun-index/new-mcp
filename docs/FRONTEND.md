# NewMCP 前端设计文档

> 版本: V1.0 | 状态: 草案 | 更新日期: 2026-05-03

## 1. 技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| React | 18.x | UI 框架 |
| Vite | 5.x | 构建工具 |
| Semi Design | 2.x | UI 组件库 (字节跳动出品，中文友好) |
| React Router | 6.x | 路由 |
| Zustand | 4.x | 状态管理 |
| Axios | 1.x | HTTP 客户端 |
| i18next | 23.x | 国际化 |
| Monaco Editor | - | JSON 配置编辑 |

---

## 2. 页面结构

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

| 路径 | 页面 | 权限 |
|------|------|------|
| `/auth/login` | 登录页 | 公开 |
| `/auth/register` | 注册页 | 公开 |
| `/dashboard` | 总览仪表盘 | 登录 |
| `/services` | MCP 服务列表 | 登录 |
| `/services/create` | 注册新服务 | 登录 |
| `/services/:id` | 服务详情 | 登录 |
| `/groups` | MCP 分组列表 | 登录 |
| `/groups/create` | 创建分组 | 登录 |
| `/groups/:id` | 分组详情 | 登录 |
| `/connections` | 云端主动连接列表 | 登录 |
| `/connections/create` | 添加云端连接 | 登录 |
| `/connections/:id` | 连接详情 | 登录 |
| `/vision` | 视觉配置列表 | 登录 |
| `/vision/create` | 新建视觉配置 | 登录 |
| `/vision/:id` | 编辑视觉配置 | 登录 |
| `/cameras` | 摄像头列表 | 登录 |
| `/cameras/create` | 添加摄像头 | 登录 |
| `/cameras/:id` | 摄像头详情 | 登录 |
| `/api-keys` | API 密钥管理 | 登录 |
| `/settings` | 个人设置 | 登录 |
| `/marketplace` | 平台市场 | 登录 |
| `/marketplace/:id` | 市场服务详情 | 登录 |
| `/admin/users` | 用户管理 | admin |
| `/admin/logs` | 调用日志 | admin |
| `/admin/marketplace` | 市场服务管理 | admin |
| `/admin/marketplace/create` | 上架市场服务 | admin |
| `/admin/marketplace/:id` | 编辑市场服务 | admin |
| `/admin/reviews` | 上架审核列表 | admin |
| `/admin/system` | 系统设置 | admin |

---

## 3. 页面设计

### 3.1 登录页 `/auth/login`
- 居中卡片式登录表单
- 用户名 + 密码输入
- "记住我" 复选框
- 登录按钮 + 注册链接

### 3.2 Dashboard `/dashboard`
- 4 个统计卡片: 服务数 / 分组数 / 主动连接数 / 今日调用量
- 服务健康状态列表 (实时显示各 MCP 服务健康/不健康)
- 最近调用日志 (最近 10 条)
- 快捷操作: 注册服务 / 创建分组 / 添加连接

### 3.3 MCP 服务列表 `/services`
- 表格视图: 名称 / 类型 / 服务分类 / 状态 / 健康状态 / 工具数 / 操作
- 筛选器: 服务分类（即时使用 / 自建部署 / 被动接入）/ 传输类型 / 状态 / 关键词搜索
- 批量操作: 启用 / 禁用
- "注册新服务" 按钮

### 3.4 注册新服务 `/services/create`
- 分步表单 (Step Form):
  1. 基本信息: 名称、显示名、描述、**服务分类**
     - 选择服务分类后自动过滤可选传输类型:
       - 即时使用 → SSE / Streamable HTTP / WebSocket
       - 自建部署 → stdio
       - 被动接入 → passive-ws
  2. 传输配置: 选择传输类型 → 动态显示对应配置表单
     - stdio: 命令、参数、环境变量
     - sse/http: URL、请求头
     - websocket: URL
  3. 认证配置: 认证类型 → 对应的凭证输入
  4. 连接测试: 显示测试结果，确认后提交

### 3.5 服务详情 `/services/:id`
- 服务信息卡片 (名称、类型、状态、健康)
- 连接配置 (可编辑，JSON 编辑器)
- 工具列表表格 (名称、描述、参数 Schema)
- 健康检查历史
- 操作: 测试连接 / 刷新工具 / 编辑 / 删除

### 3.6 MCP 分组列表 `/groups`
- 卡片视图: 每个分组一张卡片，显示名称、服务数、工具数、端点 URL
- 点击卡片进入详情

### 3.7 分组详情 `/groups/:id`
- 分组信息 (可编辑)
- **暴露模式切换**: Direct 模式 / Smart 模式 (Radio 或 Switch 组件)
  - Direct 模式说明: 直接暴露所有工具，适合工具少的场景
  - Smart 模式说明: 仅暴露 3 个元工具（搜索/查看/执行），适合工具多或设备上下文受限的场景
- 端点信息卡片: Streamable HTTP URL / WebSocket URL / 连接配置 JSON (一键复制)
- 已添加服务列表 (可拖拽排序、启用/禁用、移除)
- 聚合工具列表 (带命名空间前缀、可单独启用/禁用/重命名)
- Smart 模式下额外显示: 搜索引擎状态、已索引文档数
- "添加服务" 按钮 (弹出服务选择器)

### 3.8 云端连接列表 `/connections`
- 表格: 名称 / 云平台类型 / 绑定 API Key / 连接状态(已连接/断开/错误) / 最后连接时间
- 操作: 连接/断开/编辑/删除
- "添加连接" 按钮

### 3.9 添加云端连接 `/connections/create`
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

### 3.10 视觉配置 `/vision`
- 配置列表卡片: 模型提供商 / 模型名 / 状态
- 添加配置: 选择提供商 → 填写 API Key / 端点 / 模型名 → 测试 → 保存

### 3.11 摄像头管理 `/cameras`
- 摄像头列表: 名称 / 源类型 / 帧率 / 绑定视觉模型 / 状态
- 添加摄像头: 配置源 URL / 帧率 / 分辨率 / 选择视觉配置
- 摄像头详情: 实时预览 + 最新分析结果

### 3.12 平台市场 `/marketplace`
- 卡片网格视图: 每个市场服务一张卡片
  - 卡片信息: 名称 / 描述 / 类型标签（即用型/源码型）/ 工具数 / 使用人数 / 评分
  - 即用型卡片: 显示"一键添加"按钮
  - 源码型卡片: 显示"查看部署指南"按钮
- 筛选器: 服务类型（即用型/源码型）/ 分类标签 / 关键词搜索
- 排序: 最热 / 最新 / 评分最高

### 3.13 市场服务详情 `/marketplace/:id`
- 服务信息卡片: 名称、描述、图标、类型（即用型/源码型）、作者、版本
- 工具列表: 该服务提供的所有 MCP 工具
- **即用型**: "添加到我的服务"按钮 → 弹窗填写 API Key / 认证信息 → 一键创建
- **源码型**: 部署指南区域（仓库地址、安装命令、配置模板、环境变量说明）→ "我已部署，去注册"按钮跳转到注册页
- 评论区: 用户评分和评论（后期）
- 收藏按钮（后期）

### 3.14 管理员 - 市场服务管理 `/admin/marketplace`
- 表格: 名称 / 类型 / 状态（已上架/已下架）/ 使用人数 / 创建时间 / 操作
- "上架新服务"按钮
- 操作: 编辑 / 上架/下架 / 删除

### 3.15 管理员 - 上架市场服务 `/admin/marketplace/create`
- 选择服务类型: 即用型 / 源码型
- **即用型配置**: 关联已有的 MCP 服务配置（URL、传输类型）+ 认证说明 + 使用文档
- **源码型配置**: 仓库地址 + 安装说明 + 配置模板 + 环境变量说明 + 部署文档
- 通用信息: 名称、描述、图标、分类标签、版本号
- 上架后所有用户可在市场看到

### 3.16 管理员 - 上架审核列表 `/admin/reviews`（后期）
- 表格: 申请人 / 服务名称 / 提交时间 / 审核状态 / 操作
- 审核操作: 查看详情 / 通过 / 拒绝（填写理由）

---

## 4. 组件设计

### 4.1 共享组件

| 组件 | 说明 |
|------|------|
| `AppLayout` | 应用布局 (Header + Sidebar + Content) |
| `ServiceStatusBadge` | 服务健康状态标识 (绿/红/灰) |
| `TransportTypeTag` | 传输类型标签 (stdio/SSE/WS/HTTP) |
| `ToolTable` | MCP 工具列表 (带启用/禁用开关) |
| `JsonEditor` | JSON 配置编辑器 (Monaco) |
| `ConnectionTester` | MCP 连接测试按钮 + 结果展示 |
| `EndpointInfo` | MCP 端点信息卡片 (URL + 复制按钮) |
| `GroupSelector` | 分组选择器 (下拉/穿梭框) |
| `ExposeModeSwitch` | 暴露模式切换 (Direct / Smart) |
| `ConnectionStatusBadge` | 连接状态标识 (已连接/断开/错误) |
| `CopyButton` | 一键复制按钮 |
| `MarketTypeTag` | 市场服务类型标签 (即用型/源码型) |

### 4.2 表单组件

| 组件 | 说明 |
|------|------|
| `TransportConfigForm` | 动态传输配置表单 (根据类型切换) |
| `AuthConfigForm` | 认证配置表单 |
| `VisionProviderForm` | 视觉模型配置表单 |
| `CameraSourceForm` | 摄像头源配置表单 |

---

## 5. 状态管理 (Zustand)

```typescript
// stores/authStore.ts
interface AuthState {
  token: string | null;
  user: User | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  isAuthenticated: () => boolean;
}

// stores/serviceStore.ts
interface ServiceState {
  services: McpService[];
  loading: boolean;
  fetchServices: (params?: ListParams) => Promise<void>;
  createService: (data: CreateServiceDTO) => Promise<McpService>;
  testConnection: (id: number) => Promise<TestResult>;
  refreshTools: (id: number) => Promise<void>;
}

// stores/groupStore.ts
interface GroupState {
  groups: McpGroup[];
  currentGroup: McpGroup | null;
  fetchGroups: () => Promise<void>;
  fetchGroup: (id: number) => Promise<McpGroup>;
  createGroup: (data: CreateGroupDTO) => Promise<McpGroup>;
  addServiceToGroup: (groupId: number, serviceIds: number[]) => Promise<void>;
  removeServiceFromGroup: (groupId: number, serviceId: number) => Promise<void>;
  updateToolConfig: (groupId: number, toolName: string, config: ToolConfig) => Promise<void>;
}

// stores/connectionStore.ts
interface ConnectionState {
  connections: CloudConnection[];
  fetchConnections: () => Promise<void>;
  createConnection: (data: CreateConnectionDTO) => Promise<CloudConnection>;
  connect: (id: number) => Promise<void>;
  disconnect: (id: number) => Promise<void>;
  bindGroup: (connectionId: number, groupId: number) => Promise<void>;
}
```

---

## 6. 前端项目结构

```
web/
├── public/
│   └── favicon.svg
├── src/
│   ├── components/              # 共享组件
│   │   ├── Layout/
│   │   │   ├── AppLayout.tsx
│   │   │   ├── Header.tsx
│   │   │   ├── Sidebar.tsx
│   │   │   └── Footer.tsx
│   │   ├── McpServiceCard.tsx
│   │   ├── McpToolTable.tsx
│   │   ├── GroupSelector.tsx
│   │   ├── ExposeModeSwitch.tsx
│   │   ├── ConnectionStatusBadge.tsx
│   │   ├── ServiceStatusBadge.tsx
│   │   ├── TransportTypeTag.tsx
│   │   ├── JsonEditor.tsx
│   │   ├── ConnectionTester.tsx
│   │   ├── EndpointInfo.tsx
│   │   └── CopyButton.tsx
│   ├── pages/                   # 页面组件
│   │   ├── auth/
│   │   │   ├── LoginPage.tsx
│   │   │   └── RegisterPage.tsx
│   │   ├── dashboard/
│   │   │   └── DashboardPage.tsx
│   │   ├── services/
│   │   │   ├── ServiceListPage.tsx
│   │   │   ├── ServiceCreatePage.tsx
│   │   │   └── ServiceDetailPage.tsx
│   │   ├── groups/
│   │   │   ├── GroupListPage.tsx
│   │   │   ├── GroupCreatePage.tsx
│   │   │   └── GroupDetailPage.tsx
│   │   ├── connections/
│   │   │   ├── ConnectionListPage.tsx
│   │   │   ├── ConnectionCreatePage.tsx
│   │   │   └── ConnectionDetailPage.tsx
│   │   ├── vision/
│   │   │   ├── VisionConfigList.tsx
│   │   │   ├── VisionConfigCreate.tsx
│   │   │   └── VisionConfigEdit.tsx
│   │   ├── cameras/
│   │   │   ├── CameraListPage.tsx
│   │   │   ├── CameraCreatePage.tsx
│   │   │   └── CameraDetailPage.tsx
│   │   ├── api-keys/
│   │   │   └── ApiKeyPage.tsx
│   │   ├── settings/
│   │   │   └── SettingsPage.tsx
│   │   ├── marketplace/
│   │   │   ├── MarketplaceListPage.tsx
│   │   │   └── MarketplaceDetailPage.tsx
│   │   └── admin/
│   │       ├── UserManagePage.tsx
│   │       ├── LogViewPage.tsx
│   │       ├── MarketplaceManagePage.tsx
│   │       ├── MarketplaceCreatePage.tsx
│   │       ├── MarketplaceEditPage.tsx
│   │       ├── ReviewListPage.tsx
│   │       └── SystemPage.tsx
│   ├── stores/                  # Zustand 状态
│   │   ├── authStore.ts
│   │   ├── serviceStore.ts
│   │   ├── groupStore.ts
│   │   └── connectionStore.ts
│   ├── api/                     # API 客户端
│   │   ├── client.ts            # Axios 实例 + 拦截器
│   │   ├── auth.ts
│   │   ├── services.ts
│   │   ├── groups.ts
│   │   ├── connections.ts
│   │   ├── vision.ts
│   │   ├── cameras.ts
│   │   └── apiKeys.ts
│   ├── hooks/                   # 自定义 Hooks
│   │   ├── useMcpService.ts
│   │   ├── useGroup.ts
│   │   └── useConnection.ts
│   ├── i18n/                    # 国际化
│   │   ├── index.ts
│   │   └── locales/
│   │       ├── zh.json
│   │       └── en.json
│   ├── types/                   # TypeScript 类型
│   │   └── index.ts
│   ├── utils/                   # 工具函数
│   │   └── index.ts
│   ├── App.tsx                  # 根组件
│   ├── main.tsx                 # 入口
│   └── index.css                # 全局样式
├── index.html
├── vite.config.ts
├── tsconfig.json
├── tsconfig.node.json
├── .eslintrc.cjs
├── .prettierrc
└── package.json
```
