package dto

type CreateConnectionReq struct {
	Name        string                 `json:"name" binding:"required,min=1,max=128"`
	CloudType   string                 `json:"cloud_type" binding:"required,oneof=xiaozhi custom ssh"`
	WssURL      string                 `json:"wss_url"`
	CloudConfig map[string]interface{} `json:"cloud_config"`
	ApiKeyID    *int64                 `json:"api_key_id"`
	AutoConnect *bool                  `json:"auto_connect"`
}

type UpdateConnectionReq struct {
	Name      *string `json:"name"`
	WssURL    *string `json:"wss_url"`
	ApiKeyID  *int64  `json:"api_key_id"`
	Status    *int    `json:"status"`
}

type ConnectionListItem struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	CloudType        string `json:"cloud_type"`
	RemoteID         string `json:"remote_id"`
	ConnectionStatus string `json:"connection_status"`
	AutoConnect      bool   `json:"auto_connect"`
	CreatedAt        string `json:"created_at"`
}

type ConnectionDetail struct {
	ID               int64                  `json:"id"`
	Name             string                 `json:"name"`
	CloudType        string                 `json:"cloud_type"`
	WssURL           string                 `json:"wss_url"`
	CloudConfig      map[string]interface{} `json:"cloud_config"`
	RemoteID         string                 `json:"remote_id"`
	TokenExpiresAt   string                 `json:"token_expires_at"`
	ApiKeyID         *int64                 `json:"api_key_id"`
	AutoConnect      bool                   `json:"auto_connect"`
	ConnectionStatus string                 `json:"connection_status"`
	LastConnectedAt  string                 `json:"last_connected_at"`
	LastError        string                 `json:"last_error"`
}
