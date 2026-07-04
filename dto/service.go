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
	Connected  bool                   `json:"connected"`
	Error      string                 `json:"error,omitempty"`
	ServerInfo map[string]interface{} `json:"server_info"`
	ToolsCount int                    `json:"tools_count"`
	LatencyMs  int64                  `json:"latency_ms"`
}

type RefreshToolsResult struct {
	ToolsCount int           `json:"tools_count"`
	Tools      []interface{} `json:"tools"`
}
