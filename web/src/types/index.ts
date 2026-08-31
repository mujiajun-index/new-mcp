export interface User {
  id: number
  username: string
  display_name: string
  email: string
  role: 'super_admin' | 'admin' | 'user'
  status: number
  quota: number
  used_quota: number
  request_count: number
  group: string
  created_at: string
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message: string
  data: T
}

export interface PaginatedResponse<T> {
  success: boolean
  data: T[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
}

export interface ListParams {
  page?: number
  page_size?: number
  keyword?: string
}

// --- Auth ---
export interface AuthResp {
  id: number
  username: string
  role: string
  token: string
}

export interface ProfileResp {
  id: number
  username: string
  display_name: string
  email: string
  role: string
  avatar_url: string
  status: number
  quota: number
  used_quota: number
  request_count: number
  group: string
  created_at: string
}

// --- Services ---
export type TransportType = 'stdio' | 'sse' | 'streamable-http' | 'websocket' | 'passive-ws' | 'virtual'
export type AuthType = 'none' | 'api_key' | 'bearer' | 'custom'

export interface ServiceListItem {
  id: number
  name: string
  display_name: string
  description: string
  transport_type: TransportType
  source: string
  health_status: string
  tools_count: number
  status: number
  created_at: string
}

export interface ServiceDetail {
  id: number
  name: string
  display_name: string
  description: string
  transport_type: TransportType
  source: string
  config: Record<string, unknown>
  auth_type: AuthType
  health_status: string
  last_health_check: string
  tools_cache: McpTool[]
  tools_updated_at: string
  server_info: Record<string, unknown>
  protocol_version: string
  tags: string[]
  status: number
  created_at: string
  passive_url: string
  passive_connected: boolean
  /** 市场引用服务的条目 ID(跳转市场详情用;其余来源不返回) */
  marketplace_item_id?: number
}

export interface McpTool {
  name: string
  description: string
  inputSchema: Record<string, unknown>
}

// stdio 进程操作动作(总览卡片/详情页进程信息)
export type ProcessControlAction = 'start' | 'stop' | 'restart'

// stdio 服务子进程(整棵进程树)的资源占用快照;running 为 false 时其余字段缺省
export interface ServiceProcessStat {
  running: boolean
  pid?: number
  command?: string
  process_count?: number
  memory_rss_bytes?: number
  memory_vms_bytes?: number
  cpu_percent?: number
  uptime_seconds?: number
}

// 近期调用色带的单个 10 分钟桶(近 200 分钟 = 20 桶,旧→新):success+failed===0
// 表示该桶无调用;start_unix 为桶起点 epoch 秒(绝对 10 分钟边界),本地化展示
export interface HealthBucket {
  start_unix: number
  success: number
  failed: number
}

// 服务总览:单个服务的运行/资源快照,running=false 时进程字段缺省;
// 健康字段仅非 stdio 服务返回(真实调用日志聚合,近 200 分钟被动口径)
export interface ServicesOverviewItem {
  id: number
  name: string
  display_name: string
  transport_type: TransportType
  source: string
  health_status: string
  status: number
  tools_count: number
  created_at: string
  running: boolean
  pid?: number
  process_count?: number
  memory_rss_bytes?: number
  cpu_percent?: number
  uptime_seconds?: number
  /** 近 200 分钟调用成败色带(20 桶 × 10 分钟,旧→新) */
  health_buckets?: HealthBucket[]
  /** 最近一次消费调用时间(unix 秒;窗口内或全历史点查) */
  last_call_at?: number
  last_error_message?: string
  last_error_at?: number
}

// 管理端市场页:单个市场条目的平台级健康(同条目下全部用户引用行的真实调用
// 聚合,口径同总览被动健康);字段与 ServicesOverviewItem 健康字段同名,共用健康色带组件
export interface MarketplaceItemHealth {
  health_status: string
  health_buckets?: HealthBucket[]
  last_call_at?: number
  last_error_message?: string
  last_error_at?: number
}

// 服务总览:顶部统计卡数据;cpu_percent_total 多核下可超 100%
export interface ServicesOverviewSummary {
  total_services: number
  running_services: number
  tools_total: number
  process_total: number
  memory_rss_bytes_total: number
  cpu_percent_total: number
  healthy_count: number
  host_memory_total_bytes: number
}

// 服务总览接口响应:统计摘要 + 全量服务快照(不分页,前端本地筛选)
export interface ServicesOverviewData {
  summary: ServicesOverviewSummary
  services: ServicesOverviewItem[]
}

// MCP 内容块:text/image/resource 等,工具调用 content 与提示消息 content 共用
export interface McpContentBlock {
  type: string
  text?: string
  data?: string
  mimeType?: string
  url?: string
  resource?: { uri?: string; text?: string; blob?: string; mimeType?: string }
  [key: string]: unknown
}

// 服务详情页测试调用(tools/call / resources/read / prompts/get)共用返回:
// result 为上游完整结果 JSON;连接失败等本地错误走 is_error + error
export interface ToolCallResult {
  result: {
    content?: McpContentBlock[]
    // resources/read:{contents:[{uri, mimeType, text|blob}]}
    contents?: Array<{ uri?: string; mimeType?: string; text?: string; blob?: string; [key: string]: unknown }>
    // prompts/get:{messages:[{role, content}]}
    messages?: Array<{ role?: string; content?: McpContentBlock; [key: string]: unknown }>
    isError?: boolean
    [key: string]: unknown
  }
  is_error: boolean
  error?: string
  duration_ms: number
}

export interface CreateServiceReq {
  name: string
  display_name?: string
  description?: string
  transport_type: TransportType
  config: Record<string, unknown>
  auth_type?: AuthType
  auth_config?: Record<string, unknown>
  tags?: string[]
}

export interface UpdateServiceReq {
  display_name?: string
  description?: string
  config?: Record<string, unknown>
  auth_type?: AuthType
  auth_config?: Record<string, unknown>
  tags?: string[]
  status?: number
}

export interface TestResult {
  connected: boolean
  server_info: Record<string, unknown>
  protocol_version?: string
  tools_count: number
  latency_ms: number
  error?: string
}

export interface PrepareStdioReq {
  command: string
  args: string[]
  env: Record<string, string>
  registry: string
}

export interface PrepareStdioResult {
  branch: string
  runtime_found: boolean
  runtime_path?: string
  did_install: boolean
  installed: boolean
  package_name?: string
  registry_env: Record<string, string>
  stdout?: string
  stderr?: string
  duration_ms: number
  message: string
}

export interface RefreshToolsResult {
  tools_count: number
  tools: McpTool[]
}

export interface ServiceListParams extends ListParams {
  transport_type?: TransportType
  status?: number
}

// --- Groups ---
export interface GroupListItem {
  id: number
  name: string
  display_name: string
  description: string
  expose_mode: 'direct' | 'smart'
  tools_count: number
  status: number
  created_at: string
}

export interface GroupDetail {
  id: number
  name: string
  display_name: string
  description: string
  endpoint_url: string
  visibility: string
  expose_mode: 'direct' | 'smart'
  services: GroupServiceItem[]
  tools_count: number
  status: number
}

export interface GroupServiceItem {
  id: number
  name: string
  display_name: string
  source: string
  enabled: boolean
  tools_count: number
}

export interface GroupToolItem {
  service_id: number
  name: string
  original_name: string
  service_name: string
  description: string
  enabled: boolean
  name_override: string
  inputSchema: Record<string, unknown>
}

export interface BatchToolUpdate {
  service_id: number
  tool_name: string
  enabled: boolean
}

export interface GroupResourceItem {
  service_id: number
  service_name: string
  kind: 'resource' | 'template'
  uri: string
  name?: string
  description?: string
  mime_type?: string
  enabled: boolean
}

export interface GroupPromptArgument {
  name: string
  description?: string
  required: boolean
}

export interface GroupPromptItem {
  service_id: number
  service_name: string
  name: string
  description?: string
  arguments?: GroupPromptArgument[]
  enabled: boolean
}

export interface BatchResourceUpdate {
  service_id: number
  kind: 'resource' | 'template'
  uri: string
  enabled: boolean
}

export interface BatchPromptUpdate {
  service_id: number
  name: string
  enabled: boolean
}

export interface McpResource {
  uri: string
  name?: string
  description?: string
  mimeType?: string
  size?: number
}

export interface McpResourceTemplate {
  uriTemplate: string
  name?: string
  description?: string
  mimeType?: string
}

export interface McpPrompt {
  name: string
  description?: string
  arguments?: GroupPromptArgument[]
}

export interface CreateGroupReq {
  name: string
  display_name?: string
  description?: string
  visibility?: 'private' | 'public'
  endpoint_auth?: 'api_key' | 'jwt' | 'none'
  expose_mode?: 'direct' | 'smart'
}

export interface UpdateGroupReq {
  name?: string
  display_name?: string
  description?: string
  visibility?: string
  expose_mode?: 'direct' | 'smart'
  status?: number
}

export interface EndpointInfo {
  streamable_http_url: string
  websocket_url: string
  auth_type: string
  connection_config: Record<string, unknown>
  mcp_client_config: Record<string, unknown>
}

// --- API Keys ---
export interface ApiKeyListItem {
  id: number
  name: string
  key_prefix: string
  status: number
  groups: string[]
  quota: number
  used_quota: number
  unlimited_quota: boolean
  allow_ips: string
  expires_at: string
  last_used_at: string
  created_at: string
}

export interface CreateApiKeyReq {
  name: string
  groups: string[]
  expires_at?: string
  quota?: number
  unlimited_quota?: boolean
  allow_ips?: string
}

export interface UpdateApiKeyReq {
  name?: string
  groups?: string[]
  status?: number
  quota?: number
  unlimited_quota?: boolean
  allow_ips?: string
  expires_at?: string
}

export interface CreateApiKeyResp {
  id: number
  name: string
  key: string
  key_prefix: string
  groups: string[]
  quota: number
  unlimited_quota: boolean
  expires_at: string
}

// --- Connections ---
export type CloudType = 'xiaozhi' | 'custom' | 'ssh'

export interface ConnectionListItem {
  id: number
  name: string
  cloud_type: CloudType
  remote_id: string
  connection_status: string
  expose_mode: 'direct' | 'smart'
  auto_connect: boolean
  status: number
  created_at: string
}

export interface ConnectionDetail {
  id: number
  name: string
  cloud_type: CloudType
  wss_url: string
  cloud_config: Record<string, unknown>
  remote_id: string
  token_expires_at: string
  api_key_id: number
  auto_connect: boolean
  connection_status: string
  expose_mode: 'direct' | 'smart'
  last_connected_at: string
  last_error: string
  status: number
}

export interface CreateConnectionReq {
  name: string
  cloud_type: CloudType
  wss_url?: string
  cloud_config?: Record<string, unknown>
  api_key_id: number
  auto_connect?: boolean
  expose_mode?: 'direct' | 'smart'
}

export interface UpdateConnectionReq {
  name?: string
  wss_url?: string
  api_key_id?: number
  status?: number
  expose_mode?: 'direct' | 'smart'
}

// --- Marketplace ---
export interface MarketplaceListItem {
  id: number
  name: string
  display_name: string
  description: string
  icon_url: string
  category: 'instant' | 'source'
  group_ids: number[]
  group_names: string[]
  tags: string[]
  version: string
  transport_type: TransportType
  install_count: number
  rating_avg: number
  rating_count: number
  status: number
  sort_order: number
  created_at: string
  // 商业化定价(§5):供市场列表展示价格/免费标记
  billing_type: string   // free / per_call
  price_per_call: number // 展示货币单价(per_call)
}

export interface MarketplaceDetail {
  id: number
  name: string
  display_name: string
  description: string
  icon_url: string
  category: 'instant' | 'source'
  group_ids: number[]
  group_names: string[]
  tags: string[]
  version: string
  transport_type: TransportType
  // 独占进程(仅 stdio 条目有意义):false=共享(全部安装用户共用平台子进程)
  isolated_process?: boolean
  // 平台上游连接配置(解密;headers/env 的凭证值为首尾掩码,明文不出服务端);仅 admin 详情接口回传
  config_template?: Record<string, unknown>
  config_template_source: Record<string, unknown>
  auth_instructions: string
  repo_url: string
  install_guide: string
  required_env: string[]
  install_count: number
  rating_avg: number
  rating_count: number
  tools_snapshot: McpTool[]
  // 形态同 services 的资源/提示缓存;旧市场项无快照时为 null
  resources_snapshot: { resources: McpResource[]; templates: McpResourceTemplate[] } | null
  prompts_snapshot: McpPrompt[] | null
  // 上游握手信息(克隆上架/手动刷新时捕获);未刷新过的市场项为空
  server_info: { name?: string; version?: string } | null
  protocol_version: string
  status: number
  created_at: string
  updated_at: string
  // 商业化定价
  billing_type: string
  price_per_call: number
  // 条目级定价(工具/资源/提示);无条目价时按缺省:工具回退服务价,资源/提示免费
  entry_prices?: MarketplaceEntryPrice[]
}

// 条目级定价(§5.2 条目维度):工具/资源/提示单独设价。
// name 为条目键,与网关计费同口径:工具=工具名 / 资源=上游原始 URI / 提示=上游提示名。
export type MarketplaceEntryKind = 'tool' | 'resource' | 'prompt'

export interface MarketplaceEntryPrice {
  kind: MarketplaceEntryKind
  name: string
  billing_type: string   // free / per_call / inherit(显式继承服务价)
  price_per_call: number // 展示货币单价(per_call)
}

export interface MarketplaceListParams extends ListParams {
  category?: 'instant' | 'source'
  group_id?: number
  tag?: string
}

export interface MarketplaceGroup {
  id: number
  name: string
  display_name: string
  description: string
  icon_url: string
  sort_order: number
  status: number
  /** 该分组下已上架市场项数量(与广场按组筛选口径一致) */
  item_count: number
  created_at: string
}

export interface MarketplaceTag {
  id: number
  name: string
  description: string
  sort_order: number
  status: number
  created_at: string
}

export interface InstallReq {
  item_id: number
  name_override?: string
}

export interface InstallResult {
  service_id: number
  name: string
}

// --- Admin ---
export interface AdminStats {
  users_count: number
  services_count: number
  groups_count: number
  connections_count: number
  calls_today: number
  calls_success_rate: number
  avg_latency_ms: number
}

export interface AdminUserItem {
  id: number
  username: string
  display_name: string
  email: string
  role: string
  status: number
  quota: number
  used_quota: number
  request_count: number
  group: string
  remark: string
  created_at: string
}

export interface AdminUserDetail extends AdminUserItem {
  register_ip: string
  last_login_at: string
  last_login_ip: string
}

export interface AdminCreateUserReq {
  username: string
  password: string
  email?: string
  display_name?: string
  role?: string
  quota?: number
  group?: string
}

export interface AdminUpdateUserReq {
  display_name?: string
  status?: number
  role?: string
  email?: string
  quota?: number
  group?: string
  remark?: string
  password?: string
}

export interface LogStats {
  total_calls: number
  success_calls: number
  failed_calls: number
  avg_duration_ms: number
  calls_today: number
}

export interface LogFilter {
  start_date?: string
  end_date?: string
  status?: string
  tool_name?: string
  group_name?: string
  username?: string
  service_name?: string
  api_key_name?: string
  keyword?: string
  type?: number // 0=全部(哨兵),否则按日志类型(LogType)过滤
  page?: number
  page_size?: number
}

// --- 商业化:Wallet(我的额度/消费明细/用量统计) ---
export interface WalletOverview {
  quota: number          // 可用余额(quota)
  used_quota: number     // 累计已用(quota)
  request_count: number  // 累计请求数
  total_topup: number    // 累计充值(quota)
  group: string          // 用户套餐分组
}

export interface WalletUsageStats {
  consumed_today: number
  consumed_week: number
  consumed_total: number
}

// --- 商业化:Invite(邀请码/邀请奖励,对齐 new-api aff_code) ---
export interface InviteOverview {
  aff_code: string // 我的邀请码
  invite_url: string // 邀请链接
  aff_count: number // 已邀请人数
  aff_quota: number // 邀请奖励待提取余额(quota)
  aff_history_quota: number // 邀请奖励累计(quota)
  quota_for_inviter: number // 当前邀请者奖励配置
  quota_for_invitee: number // 当前受邀者奖励配置
}

export interface TransferAffQuotaReq {
  quota: number
}

export interface TransferAffQuotaResp {
  quota: number // 转账后钱包余额
  aff_quota: number // 转账后待提取余额
}

// --- 商业化:Redemption(兑换码) ---
export interface RedemptionItem {
  id: number
  code: string
  name: string
  amount: number // 面值(货币单位)
  status: number // 1=可用 2=已兑换 3=已禁用
  user_id: number | null
  username: string // 兑换者用户名(未兑换时为空)
  expired_at: number // Unix 秒,0=永不过期
  created_at: string
  redeemed_at: string
}

export interface RedemptionCreateReq {
  name?: string
  amount: number // 面值(货币单位)
  count?: number
  expired_at?: number
}

export interface RedeemReq {
  code: string
}

export interface RedeemResp {
  amount: number
}

// --- 商业化:Marketplace 批量定价 / 克隆 ---
export interface BatchPricingItem {
  id: number
  billing_type: string // free / per_call
  price_per_call?: number
}

export interface BatchPricingReq {
  items: BatchPricingItem[]
}

export interface CloneMarketplaceReq {
  from_service_id: number
  name: string
  display_name?: string
  description?: string
  billing_type?: string
  price_per_call?: number
  // 独占进程(仅源服务为 stdio 时生效);默认 false=共享
  isolated_process?: boolean
}

// 市场条目(stdio)进程视图:共享=平台唯一进程;独占=安装引用行服务端分页枚举
// (instances 为当前页)+ 全量运行实例的资源概述(不随分页/筛选变化)
export interface MarketplaceItemProcess {
  isolated: boolean
  shared?: ServiceProcessStat
  instances?: MarketplaceItemProcessInstance[]
  running_instances?: number
  total_processes?: number
  memory_bytes?: number
  cpu_percent_total?: number
  total?: number
  page?: number
  page_size?: number
  total_pages?: number
}

// 独占条目下某个安装用户的进程实例(引用行粒度);stat.running=false 为未运行固定形态
export interface MarketplaceItemProcessInstance {
  service_id: number
  user_id: number
  username: string
  name: string
  status: number
  stat: ServiceProcessStat
}

// --- 商业化:管理员调额(POST /admin/users/:id/quota) ---
export interface AdminAdjustQuotaReq {
  mode: 'add' | 'sub' | 'set'
  value: number
  remark?: string
}

export interface AdminAdjustQuotaResp {
  new_quota: number
}
