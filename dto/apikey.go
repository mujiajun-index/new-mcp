package dto

type CreateApiKeyReq struct {
	Name      string   `json:"name" binding:"required,min=1,max=128"`
	Groups    []string `json:"groups"`
	ExpiresAt *string  `json:"expires_at"`
}

type ApiKeyListItem struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	KeyPrefix  string `json:"key_prefix"`
	Status     int    `json:"status"`
	Groups     []string `json:"groups"`
	ExpiresAt  string `json:"expires_at"`
	LastUsedAt string `json:"last_used_at"`
	CreatedAt  string `json:"created_at"`
}

type ApiKeyCreateResult struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Key       string   `json:"key"`
	KeyPrefix string   `json:"key_prefix"`
	Groups    []string `json:"groups"`
	ExpiresAt string   `json:"expires_at"`
}
