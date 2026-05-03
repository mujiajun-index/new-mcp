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
| `/devices` | 设备列表 | 登录 |
| `/devices/create` | 注册设备 | 登录 |
| `/devices/:id` | 设备详情 | 登录 |
| `/vision` | 视觉配置列表 | 登录 |
| `/vision/create` | 新建视觉配置 | 登录 |
| `/vision/:id` | 编辑视觉配置 | 登录 |
| `/cameras` | 摄像头列表 | 登录 |
| `/cameras/create` | 添加摄像头 | 登录 |
| `/cameras/:id` | 摄像头详情 | 登录 |
| `/api-keys` | API 密钥管理 | 登录 |
| `/settings` | 个人设置 | 登录 |
| `/admin/users` | 用户管理 | admin |
| `/admin/logs` | 调用日志 | admin |
| `/admin/system` | 系统设置 | admin |

---

## 3. 页面设计

### 3.1 登录页 `/auth/login`
- 居中卡片式登录表单
- 用户名 + 密码输入
- "记住我" 复选框
- 登录按钮 + 注册链接

### 3.2 Dashboard `/dashboard`
- 4 个统计卡片: 服务数 / 分组数 / 设备数 / 今日调用量
- 服务健康状态列表 (实时显示各 MCP 服务健康/不健康)
- 最近调用日志 (最近 10 条)
- 快捷操作: 注册服务 / 创建分组 / 添加设备

### 3.3 MCP 服务列表 `/services`
- 表格视图: 名称 / 类型 / 状态 / 健康状态 / 工具数 / 操作
- 筛选器: 传输类型 / 状态 / 关键词搜索
- 批量操作: 启用 / 禁用
- "注册新服务" 按钮

### 3.4 注册新服务 `/services/create`
- 分步表单 (Step Form):
  1. 基本信息: 名称、显示名、描述、标签
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
- 端点信息卡片: Streamable HTTP URL / WebSocket URL / 连接配置 JSON (一键复制)
- 已添加服务列表 (可拖拽排序、启用/禁用、移除)
- 聚合工具列表 (带命名空间前缀、可单独启用/禁用/重命名)
- "添加服务" 按钮 (弹出服务选择器)

### 3.8 设备列表 `/devices`
- 表格: 名称 / 类型 / 绑定分组 / 状态(在线/离线) / 最后连接时间
- 设备类型图标区分

### 3.9 设备详情 `/devices/:id`
- 设备信息卡片
- 连接状态 (实时)
- 绑定的分组信息 (可切换)
- 连接配置: WebSocket URL + Token (一键复制)
- 最近调用日志

### 3.10 视觉配置 `/vision`
- 配置列表卡片: 模型提供商 / 模型名 / 状态
- 添加配置: 选择提供商 → 填写 API Key / 端点 / 模型名 → 测试 → 保存

### 3.11 摄像头管理 `/cameras`
- 摄像头列表: 名称 / 源类型 / 帧率 / 绑定视觉模型 / 状态
- 添加摄像头: 配置源 URL / 帧率 / 分辨率 / 选择视觉配置
- 摄像头详情: 实时预览 + 最新分析结果

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
| `DeviceStatusBadge` | 设备在线/离线状态标识 |
| `CopyButton` | 一键复制按钮 |

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

// stores/deviceStore.ts
interface DeviceState {
  devices: Device[];
  fetchDevices: () => Promise<void>;
  createDevice: (data: CreateDeviceDTO) => Promise<Device>;
  bindGroup: (deviceId: number, groupId: number) => Promise<void>;
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
│   │   ├── DeviceStatusBadge.tsx
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
│   │   ├── devices/
│   │   │   ├── DeviceListPage.tsx
│   │   │   ├── DeviceCreatePage.tsx
│   │   │   └── DeviceDetailPage.tsx
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
│   │   └── admin/
│   │       ├── UserManagePage.tsx
│   │       ├── LogViewPage.tsx
│   │       └── SystemPage.tsx
│   ├── stores/                  # Zustand 状态
│   │   ├── authStore.ts
│   │   ├── serviceStore.ts
│   │   ├── groupStore.ts
│   │   └── deviceStore.ts
│   ├── api/                     # API 客户端
│   │   ├── client.ts            # Axios 实例 + 拦截器
│   │   ├── auth.ts
│   │   ├── services.ts
│   │   ├── groups.ts
│   │   ├── devices.ts
│   │   ├── vision.ts
│   │   ├── cameras.ts
│   │   └── apiKeys.ts
│   ├── hooks/                   # 自定义 Hooks
│   │   ├── useMcpService.ts
│   │   ├── useGroup.ts
│   │   └── useDevice.ts
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
