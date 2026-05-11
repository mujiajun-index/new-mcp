package dto

type CreateGroupReq struct {
	Name         string `json:"name" binding:"required,min=1,max=128"`
	DisplayName  string `json:"display_name" binding:"omitempty,max=255"`
	Description  string `json:"description"`
	Visibility   string `json:"visibility" binding:"omitempty,oneof=private public"`
	EndpointAuth string `json:"endpoint_auth" binding:"omitempty,oneof=api_key jwt none"`
	ExposeMode   string `json:"expose_mode" binding:"omitempty,oneof=smart direct"`
}

type UpdateGroupReq struct {
	Name        *string `json:"name"`
	DisplayName *string `json:"display_name"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility"`
	ExposeMode  *string `json:"expose_mode"`
	Status      *int    `json:"status"`
}

type AddGroupServicesReq struct {
	ServiceIDs []int64 `json:"service_ids" binding:"required,min=1"`
}

type UpdateToolReq struct {
	ServiceID           *int64  `json:"service_id"`
	Enabled             *bool   `json:"enabled"`
	NameOverride        *string `json:"name_override"`
	DescriptionOverride *string `json:"description_override"`
}

type BatchToolUpdate struct {
	ServiceID int64  `json:"service_id" binding:"required"`
	ToolName  string `json:"tool_name" binding:"required"`
	Enabled   bool   `json:"enabled"`
}

type BatchUpdateToolsReq struct {
	Tools []BatchToolUpdate `json:"tools" binding:"required,min=1"`
}

type GroupListItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	ExposeMode  string `json:"expose_mode"`
	ToolsCount  int    `json:"tools_count"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type GroupDetail struct {
	ID           int64              `json:"id"`
	Name         string             `json:"name"`
	DisplayName  string             `json:"display_name"`
	Description  string             `json:"description"`
	EndpointURL  string             `json:"endpoint_url"`
	Visibility   string             `json:"visibility"`
	ExposeMode   string             `json:"expose_mode"`
	Services     []GroupServiceItem `json:"services"`
	ToolsCount   int                `json:"tools_count"`
	Status       int                `json:"status"`
}

type GroupServiceItem struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	ToolsCount int    `json:"tools_count"`
}

type GroupToolItem struct {
	ServiceID    int64       `json:"service_id"`
	Name         string      `json:"name"`
	OriginalName string      `json:"original_name"`
	ServiceName  string      `json:"service_name"`
	Description  string      `json:"description"`
	Enabled      bool        `json:"enabled"`
	NameOverride string      `json:"name_override"`
	InputSchema  interface{} `json:"inputSchema"`
}

type EndpointInfo struct {
	StreamableHTTPURL string                 `json:"streamable_http_url"`
	WebSocketURL      string                 `json:"websocket_url"`
	AuthType          string                 `json:"auth_type"`
	ConnectionConfig  map[string]interface{} `json:"connection_config"`
	McpClientConfig   map[string]interface{} `json:"mcp_client_config"`
}
