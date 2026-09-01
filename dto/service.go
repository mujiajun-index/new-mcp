package dto

type CreateServiceReq struct {
	Name          string                 `json:"name" binding:"required,min=1,max=128"`
	DisplayName   string                 `json:"display_name" binding:"omitempty,max=255"`
	Description   string                 `json:"description"`
	TransportType string                 `json:"transport_type" binding:"required,oneof=stdio sse streamable-http websocket passive-ws"`
	Config        map[string]interface{} `json:"config"`
	AuthType      string                 `json:"auth_type" binding:"omitempty,oneof=none api_key bearer custom"`
	AuthConfig    map[string]interface{} `json:"auth_config"`
	// 多秘钥(仅 streamable-http/sse):KeyMode 为 random/polling 时 AuthKeys 必填,
	// 认证头值不再写入 config.headers,由秘钥池按策略注入。
	KeyMode  string   `json:"key_mode" binding:"omitempty,oneof=random polling"`
	AuthKeys []string `json:"auth_keys" binding:"omitempty,max=100"`
	Tags     []string `json:"tags"`
}

type UpdateServiceReq struct {
	DisplayName *string                `json:"display_name"`
	Description *string                `json:"description"`
	Config      map[string]interface{} `json:"config"`
	AuthType    *string                `json:"auth_type"`
	AuthConfig  map[string]interface{} `json:"auth_config"`
	Tags        []string               `json:"tags"`
	Status      *int                   `json:"status"`
}

type ServiceListItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	TransportType string `json:"transport_type"`
	Source        string `json:"source"`
	// 多秘钥模式(random/polling);空 = 单秘钥。列表徽章用。
	KeyMode      string `json:"key_mode,omitempty"`
	HealthStatus string `json:"health_status"`
	ToolsCount   int    `json:"tools_count"`
	Status       int    `json:"status"`
	CreatedAt    string `json:"created_at"`
}

type ServiceDetail struct {
	ID               int64                  `json:"id"`
	Name             string                 `json:"name"`
	DisplayName      string                 `json:"display_name"`
	Description      string                 `json:"description"`
	TransportType    string                 `json:"transport_type"`
	Source           string                 `json:"source"`
	Config           map[string]interface{} `json:"config"`
	AuthType         string                 `json:"auth_type"`
	// 多秘钥:key_mode 为 random/polling 时认证头由秘钥池按策略注入,
	// KeyCount/KeyEnabled 为池内总数与启用数。
	KeyMode    string `json:"key_mode"`            // ""=单秘钥;random|polling
	KeyCount   int    `json:"key_count,omitempty"` // 多秘钥模式下的池内秘钥总数
	KeyEnabled int    `json:"key_enabled,omitempty"`
	HealthStatus     string                 `json:"health_status"`
	LastHealthCheck  string                 `json:"last_health_check"`
	ToolsCache       []interface{}          `json:"tools_cache"`
	ToolsUpdatedAt   string                 `json:"tools_updated_at"`
	ServerInfo       map[string]interface{} `json:"server_info"`
	ProtocolVersion  string                 `json:"protocol_version"`
	Tags             []string               `json:"tags"`
	Status           int                    `json:"status"`
	CreatedAt        string                 `json:"created_at"`
	PassiveURL       string                 `json:"passive_url,omitempty"`
	PassiveConnected bool                   `json:"passive_connected,omitempty"`
	// 市场引用服务(source=marketplace)的条目 ID,前端跳转市场详情页用;其余来源不返回
	MarketplaceItemID *int64 `json:"marketplace_item_id,omitempty"`
}

// --- 多秘钥管理(/services/:id/keys) ---

// ServiceKeyItem 秘钥池单行(列表永不回明文,仅掩码值)。
type ServiceKeyItem struct {
	ID             int64  `json:"id"`
	SortOrder      int    `json:"sort_order"` // 池内序号(1 起)= 调用日志 key_index
	MaskedValue    string `json:"masked_value"`
	Status         int    `json:"status"` // 1启用 2手动禁用 3自动禁用
	DisabledReason string `json:"disabled_reason,omitempty"`
	DisabledAt     string `json:"disabled_at,omitempty"`
}

// ServiceKeysResp 秘钥池视图:配置 + 池列表 + 启用统计。
type ServiceKeysResp struct {
	KeyMode        string           `json:"key_mode"`    // ""=单秘钥;random|polling
	HeaderName     string           `json:"header_name"` // 多秘钥注入目标头
	AuthType       string           `json:"auth_type"`
	TransportType  string           `json:"transport_type"`
	Total          int              `json:"total"`
	Enabled        int              `json:"enabled"`
	Keys           []ServiceKeyItem `json:"keys"`
}

// UpdateServiceKeysReq 更新秘钥:追加(去重保状态)/ 替换全部(状态清零)。
type UpdateServiceKeysReq struct {
	Mode   string   `json:"mode" binding:"required,oneof=append replace"`
	Values []string `json:"values" binding:"required,min=1,max=100"`
}

// UpdateServiceKeysResult 更新结果:新增数与去重跳过数。
type UpdateServiceKeysResult struct {
	Added   int `json:"added"`
	Skipped int `json:"skipped"`
}

// SetServiceKeyStatusReq 启用/禁用单把秘钥。
type SetServiceKeyStatusReq struct {
	Status string `json:"status" binding:"required,oneof=enabled disabled"`
}

// BatchServiceKeysReq 批量操作:全部启用 / 删除已禁用。
type BatchServiceKeysReq struct {
	Action string `json:"action" binding:"required,oneof=enable_all delete_disabled"`
}

// UpdateServiceKeyConfigReq 模式切换:单↔多、随机↔轮询。
// single→multi:HeaderName 为注入目标头(api_key/bearer 自动推导,custom 必填),
// 现有 config.headers 中的认证值收编为首把秘钥;multi→single:首选启用秘钥写回
// config.headers 并清空秘钥池。
type UpdateServiceKeyConfigReq struct {
	KeyMode    string `json:"key_mode" binding:"required,oneof=single random polling"`
	HeaderName string `json:"header_name" binding:"omitempty,max=255"`
}

// ServiceProcessStat 是 stdio 服务子进程(整棵进程树)的资源占用快照。
// Running 为 false 时(未连接/进程已退出/非 stdio 服务)其余字段为零值。
type ServiceProcessStat struct {
	Running       bool    `json:"running"`
	PID           int     `json:"pid,omitempty"`
	Command       string  `json:"command,omitempty"`
	ProcessCount  int     `json:"process_count,omitempty"` // 树内进程总数(含 wrapper)
	MemoryRSS     uint64  `json:"memory_rss_bytes,omitempty"`
	MemoryVMS     uint64  `json:"memory_vms_bytes,omitempty"`
	CPUPercent    float64 `json:"cpu_percent,omitempty"` // 树累计 CPU/生存期,可 >100%
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`
}

// ProcessControlReq stdio 服务进程操作请求。action: start 拉起子进程并完成握手 /
// stop 终止整棵进程树并释放内存 / restart = stop + start。
type ProcessControlReq struct {
	Action string `json:"action" binding:"required,oneof=start stop restart"`
}

// HealthBucket 近期调用色带的单个 10 分钟桶(对齐 CLIProxyAPI recent_requests 口径:
// 20 桶 × 10 分钟 = 近 200 分钟滚动窗口)。Success+Failed==0 表示该桶无调用;
// StartUnix 为桶起点 epoch 秒(绝对 10 分钟边界),前端本地化展示。
type HealthBucket struct {
	StartUnix int64 `json:"start_unix"`
	Success   int64 `json:"success"`
	Failed    int64 `json:"failed"`
}

// ServicesOverviewItem 总览页单个服务的运行/资源快照。Running=false 时(未连接/
// 非 stdio/进程已退出)进程相关字段为零值被 omitempty 省略,口径同 ServiceProcessStat。
// 健康字段仅非 stdio 服务填充(由 mcp_call_logs 真实调用聚合,见
// service/health_snapshot.go),stdio 行不带。
type ServicesOverviewItem struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	DisplayName   string  `json:"display_name"`
	TransportType string  `json:"transport_type"`
	Source        string  `json:"source"`
	// HealthStatus:stdio 为进程实测口径,非 stdio 为实时被动推导(连接>窗口成败>未知),
	// 不依赖库里仅在调用后回写的标记。
	HealthStatus string  `json:"health_status"`
	Status       int     `json:"status"`
	ToolsCount   int     `json:"tools_count"`
	CreatedAt    string  `json:"created_at"`
	Running      bool    `json:"running"`
	PID          int     `json:"pid,omitempty"`
	ProcessCount int     `json:"process_count,omitempty"`
	MemoryRSS    uint64  `json:"memory_rss_bytes,omitempty"` // 树 RSS;进度条分母用 summary.host_memory_total_bytes
	CPUPercent   float64 `json:"cpu_percent,omitempty"`      // 树累计 CPU/生存期,可 >100%(多核)
	UptimeSeconds int64  `json:"uptime_seconds,omitempty"`

	// 非 stdio 服务:近 200 分钟(20 桶 × 10 分钟,旧→新)真实调用成败 + 窗口内最近
	// 一次调用时间 + 窗口内最近一次失败(悬停可见,不占布局)
	HealthBuckets    []HealthBucket `json:"health_buckets,omitempty"`    // 恒 20 项
	LastCallAt       int64          `json:"last_call_at,omitempty"`      // unix 秒,0 = 从未调用
	LastErrorMessage string         `json:"last_error_message,omitempty"`
	LastErrorAt      int64          `json:"last_error_at,omitempty"`     // unix 秒,0 = 无
}

// ServicesOverviewSummary 总览页顶部统计卡数据。CPUTotalPercent 为各 stdio 进程树
// 求和,多核下可超过 100%,由前端如实展示并提示口径。
type ServicesOverviewSummary struct {
	TotalServices   int     `json:"total_services"`
	RunningServices int     `json:"running_services"`
	ToolsTotal      int     `json:"tools_total"`
	ProcessTotal    int     `json:"process_total"` // stdio 运行树的进程数总和
	MemoryRSSTotal  uint64  `json:"memory_rss_bytes_total"`
	CPUTotalPercent float64 `json:"cpu_percent_total"`
	HealthyCount    int     `json:"healthy_count"` // 实时健康数(stdio=进程存活;非stdio=连接中或窗口内有成功),健康率由前端计算
	HostMemoryTotal uint64  `json:"host_memory_total_bytes"` // 主机物理内存总量(gopsutil),内存条分母
}

// ServicesOverview 是总览接口响应:统计摘要 + 全量服务快照(不分页,前端本地筛选)。
type ServicesOverview struct {
	Summary  ServicesOverviewSummary `json:"summary"`
	Services []ServicesOverviewItem  `json:"services"`
}

type TestConnectionReq struct {
	TransportType string                 `json:"transport_type" binding:"required,oneof=stdio sse streamable-http websocket passive-ws"`
	Config        map[string]interface{} `json:"config"`
}

// PrepareStdioReq drives the pre-flight detect/install step for a stdio service.
type PrepareStdioReq struct {
	Command  string            `json:"command" binding:"required"`
	Args     []string          `json:"args"`
	Env      map[string]string `json:"env"`
	Registry string            `json:"registry"` // mirror URL; "" = system default
}

// PrepareStdioResult is the outcome of the detect/install step. Installed is the
// single gate the UI uses to enable Next / Create.
type PrepareStdioResult struct {
	Branch       string            `json:"branch"` // npx | uvx | plain
	RuntimeFound bool              `json:"runtime_found"`
	RuntimePath  string            `json:"runtime_path,omitempty"`
	DidInstall   bool              `json:"did_install"`
	Installed    bool              `json:"installed"`
	PackageName  string            `json:"package_name,omitempty"`
	RegistryEnv  map[string]string `json:"registry_env"`
	Stdout       string            `json:"stdout,omitempty"`
	Stderr       string            `json:"stderr,omitempty"`
	DurationMs   int64             `json:"duration_ms"`
	Message      string            `json:"message"`
}

type TestResult struct {
	Connected       bool                   `json:"connected"`
	Error           string                 `json:"error,omitempty"`
	ServerInfo      map[string]interface{} `json:"server_info"`
	ProtocolVersion string                 `json:"protocol_version,omitempty"`
	ToolsCount      int                    `json:"tools_count"`
	LatencyMs       int64                  `json:"latency_ms"`
}

type RefreshToolsResult struct {
	ToolsCount int           `json:"tools_count"`
	Tools      []interface{} `json:"tools"`
}

// CallToolReq 服务详情页工具测试请求:工具名放 body(避免路径特殊字符问题)。
type CallToolReq struct {
	Name      string                 `json:"name" binding:"required"`
	Arguments map[string]interface{} `json:"arguments"`
}

// CallToolResult 服务详情页测试调用(工具/资源/提示)共用结果:
// Result 为上游完整 result JSON(tools/call 为 {content, isError, ...} 等);
// 连接失败/条目不存在等本地错误通过 IsError+Error 返回(前端在结果区展示,而非仅弹 toast)。
type CallToolResult struct {
	Result     interface{} `json:"result"`
	IsError    bool        `json:"is_error"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

// ReadResourceReq 资源测试请求:对指定 URI 执行 resources/read。
type ReadResourceReq struct {
	URI string `json:"uri" binding:"required"`
}

// GetPromptReq 提示测试请求:按传入参数渲染提示(prompts/get,参数均为字符串)。
type GetPromptReq struct {
	Name      string            `json:"name" binding:"required"`
	Arguments map[string]string `json:"arguments"`
}
