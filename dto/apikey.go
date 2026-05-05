package dto

type CreateApiKeyReq struct {
	Name           string   `json:"name" binding:"required,min=1,max=128"`
	Groups         []string `json:"groups"`
	ExpiresAt      *string  `json:"expires_at"`
	Quota          *int64   `json:"quota"`
	UnlimitedQuota *bool    `json:"unlimited_quota"`
	AllowIPs       string   `json:"allow_ips"`
}

type UpdateApiKeyReq struct {
	Name           *string  `json:"name"`
	Groups         []string `json:"groups"`
	Status         *int     `json:"status"`
	Quota          *int64   `json:"quota"`
	UnlimitedQuota *bool    `json:"unlimited_quota"`
	AllowIPs       *string  `json:"allow_ips"`
	ExpiresAt      *string  `json:"expires_at"`
}

type ApiKeyListItem struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	KeyPrefix      string   `json:"key_prefix"`
	Status         int      `json:"status"`
	Groups         []string `json:"groups"`
	Quota          int64    `json:"quota"`
	UsedQuota      int64    `json:"used_quota"`
	UnlimitedQuota bool     `json:"unlimited_quota"`
	AllowIPs       string   `json:"allow_ips"`
	ExpiresAt      string   `json:"expires_at"`
	LastUsedAt     string   `json:"last_used_at"`
	CreatedAt      string   `json:"created_at"`
}

type ApiKeyCreateResult struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Key            string   `json:"key"`
	KeyPrefix      string   `json:"key_prefix"`
	Groups         []string `json:"groups"`
	Quota          int64    `json:"quota"`
	UnlimitedQuota bool     `json:"unlimited_quota"`
	ExpiresAt      string   `json:"expires_at"`
}
