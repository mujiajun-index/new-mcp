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
}

export interface McpTool {
  name: string
  description: string
  inputSchema: Record<string, unknown>
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
  tags: string[]
  version: string
  transport_type: TransportType
  config_template_source: Record<string, unknown>
  auth_instructions: string
  repo_url: string
  install_guide: string
  required_env: string[]
  install_count: number
  rating_avg: number
  rating_count: number
  tools_snapshot: McpTool[]
  status: number
  created_at: string
  updated_at: string
  // 商业化定价
  billing_type: string
  price_per_call: number
}

export interface MarketplaceListParams extends ListParams {
  category?: 'instant' | 'source'
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
  keyword?: string
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

export interface WalletBillingItem {
  id: number
  tool_name: string
  method: string
  service_name: string
  group_name: string
  billing_status: string // charged / refunded / blocked / debt
  billing_type: string   // free / per_call
  unit_price: number     // 展示货币单价快照
  quota_consumed: number // 本次实扣 quota
  price_scope: string    // tool/service/marketplace/global
  marketplace_item_id: number | null
  created_at: string
}

export interface WalletUsageStats {
  consumed_today: number
  consumed_week: number
  consumed_total: number
}

// --- 商业化:Redemption(兑换码) ---
export interface RedemptionItem {
  id: number
  code: string
  name: string
  quota: number
  status: number // 1=可用 2=已兑换 3=已禁用
  user_id: number | null
  expired_at: number // Unix 秒,0=永不过期
  created_at: string
  redeemed_at: string
}

export interface RedemptionCreateReq {
  name?: string
  quota: number
  count?: number
  expired_at?: number
}

export interface RedeemReq {
  code: string
}

export interface RedeemResp {
  quota: number
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
