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
	Status *int    `json:"status"`
	Role   *string `json:"role"`
	Email  *string `json:"email"`
}

type UserListItem struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}

type LogItem struct {
	ID             int64  `json:"id"`
	UserID         *int64 `json:"user_id"`
	ServiceID      *int64 `json:"service_id"`
	GroupID        *int64 `json:"group_id"`
	ToolName       string `json:"tool_name"`
	ResponseStatus string `json:"response_status"`
	DurationMs     int    `json:"duration_ms"`
	ClientIP       string `json:"client_ip"`
	CreatedAt      string `json:"created_at"`
}
