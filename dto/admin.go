package dto

type AdminStats struct {
	UsersCount       int64   `json:"users_count"`
	ServicesCount    int64   `json:"services_count"`
	GroupsCount      int64   `json:"groups_count"`
	ConnectionsCount int64   `json:"connections_count"`
	CallsToday       int64   `json:"calls_today"`
	CallsSuccessRate float64 `json:"calls_success_rate"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
}

type AdminUpdateUserReq struct {
	DisplayName *string `json:"display_name"`
	Status      *int    `json:"status"`
	Role        *string `json:"role"`
	Email       *string `json:"email"`
	Quota       *int64  `json:"quota"`
	Group       *string `json:"group"`
	Remark      *string `json:"remark"`
	Password    *string `json:"password"`
}

type AdminCreateUserReq struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Password    string `json:"password" binding:"required,min=6,max=128"`
	Email       string `json:"email" binding:"omitempty,email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Quota       int64  `json:"quota"`
	Group       string `json:"group"`
}

type UserListItem struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Status       int    `json:"status"`
	Quota        int64  `json:"quota"`
	UsedQuota    int64  `json:"used_quota"`
	RequestCount int64  `json:"request_count"`
	Group        string `json:"group"`
	Remark       string `json:"remark"`
	CreatedAt    string `json:"created_at"`
}

// UserDetailResp 为管理员查看单个用户的详情，额外暴露审计字段（注册 IP、最后登录时间/IP）。
// LastLoginAt 为空字符串表示从未登录。
type UserDetailResp struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Status       int    `json:"status"`
	Quota        int64  `json:"quota"`
	UsedQuota    int64  `json:"used_quota"`
	RequestCount int64  `json:"request_count"`
	Group        string `json:"group"`
	Remark       string `json:"remark"`
	CreatedAt    string `json:"created_at"` // 注册时间
	RegisterIP   string `json:"register_ip"`
	LastLoginAt  string `json:"last_login_at"`
	LastLoginIP  string `json:"last_login_ip"`
}

type LogItem struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"user_id"`
	Username       string `json:"username"`
	ApiKeyID       int64  `json:"api_key_id"`
	ApiKeyName     string `json:"api_key_name"`
	GroupID        int64  `json:"group_id"`
	GroupName      string `json:"group_name"`
	ServiceID      int64  `json:"service_id"`
	ServiceName    string `json:"service_name"`
	ToolName       string `json:"tool_name"`
	Method         string `json:"method"`
	ResponseStatus string `json:"response_status"`
	DurationMs     int    `json:"duration_ms"`
	ErrorMessage   string `json:"error_message"`
	ClientIP       string `json:"client_ip"`
	CreatedAt      string `json:"created_at"`
}

type LogStats struct {
	TotalCalls    int64   `json:"total_calls"`
	SuccessCalls  int64   `json:"success_calls"`
	FailedCalls   int64   `json:"failed_calls"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	CallsToday    int64   `json:"calls_today"`
}

type LogFilter struct {
	StartDate   string `form:"start_date"`
	EndDate     string `form:"end_date"`
	Status      string `form:"status"`
	ToolName    string `form:"tool_name"`
	GroupName   string `form:"group_name"`
	Username    string `form:"username"`
	ServiceName string `form:"service_name"`
	Keyword     string `form:"keyword"`
}
