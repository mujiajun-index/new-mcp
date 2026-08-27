package dto

type CreateServiceReq struct {
	Name          string                 `json:"name" binding:"required,min=1,max=128"`
	DisplayName   string                 `json:"display_name" binding:"omitempty,max=255"`
	Description   string                 `json:"description"`
	TransportType string                 `json:"transport_type" binding:"required,oneof=stdio sse streamable-http websocket passive-ws"`
	Config        map[string]interface{} `json:"config"`
	AuthType      string                 `json:"auth_type" binding:"omitempty,oneof=none api_key bearer custom"`
	AuthConfig    map[string]interface{} `json:"auth_config"`
	Tags          []string               `json:"tags"`
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
	HealthStatus  string `json:"health_status"`
	ToolsCount    int    `json:"tools_count"`
	Status        int    `json:"status"`
	CreatedAt     string `json:"created_at"`
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

// HealthBucket 健康 24 小时时间条的单个小时桶。Total==0 表示该小时无调用;
// StartUnix 为桶起点 epoch 秒,前端 dayjs 本地化展示。
type HealthBucket struct {
	StartUnix     int64 `json:"start_unix"`
	Total         int64 `json:"total"`
	Success       int64 `json:"success"`
	AvgDurationMs int64 `json:"avg_duration_ms"` // 0 表示该桶无数据
}

// ServicesOverviewItem 总览页单个服务的运行/资源快照。Running=false 时(未连接/
// 非 stdio/进程已退出)进程相关字段为零值被 omitempty 省略,口径同 ServiceProcessStat。
// 健康 5 字段仅非 stdio 服务填充(由 mcp_call_logs 真实调用聚合,见
// service/health_snapshot.go),stdio 行不带。
type ServicesOverviewItem struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	DisplayName   string  `json:"display_name"`
	TransportType string  `json:"transport_type"`
	Source        string  `json:"source"`
	HealthStatus  string  `json:"health_status"`
	Status        int     `json:"status"`
	ToolsCount    int     `json:"tools_count"`
	CreatedAt     string  `json:"created_at"`
	Running       bool    `json:"running"`
	PID           int     `json:"pid,omitempty"`
	ProcessCount  int     `json:"process_count,omitempty"`
	MemoryRSS     uint64  `json:"memory_rss_bytes,omitempty"` // 树 RSS;进度条分母用 summary.host_memory_total_bytes
	CPUPercent    float64 `json:"cpu_percent,omitempty"`      // 树累计 CPU/生存期,可 >100%(多核)
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`

	HealthScore       *int          `json:"health_score,omitempty"`        // 0-100,nil = 近 1h 无调用
	HealthState       string        `json:"health_state,omitempty"`        // healthy/ok/degraded/critical/no_data
	HealthBuckets     []HealthBucket `json:"health_buckets,omitempty"`     // 恒 24 项(近 24h,每小时一格)
	LastErrorMessage  string        `json:"last_error_message,omitempty"` // 24h 内最近一次失败信息
	LastErrorAt       int64         `json:"last_error_at,omitempty"`       // unix 秒,0 = 无
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
	HealthyCount    int     `json:"healthy_count"` // health_status==healthy 数,健康率由前端计算
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
