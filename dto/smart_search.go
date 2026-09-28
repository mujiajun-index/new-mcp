package dto

type SmartSearchConfigInput struct {
	Enabled     bool     `json:"enabled"`
	Provider    string   `json:"provider"`
	EndpointURL string   `json:"endpoint_url"`
	ModelName   string   `json:"model_name"`
	APIKey      string   `json:"api_key"`
	AllGroups   bool     `json:"all_groups"`
	Groups      []string `json:"groups"`
	BatchSize   int      `json:"batch_size"`
	Concurrency int      `json:"concurrency"`
}

type SmartSearchConfigDetail struct {
	Enabled     bool     `json:"enabled"`
	Provider    string   `json:"provider"`
	EndpointURL string   `json:"endpoint_url"`
	ModelName   string   `json:"model_name"`
	HasAPIKey   bool     `json:"has_api_key"`
	AllGroups   bool     `json:"all_groups"`
	Groups      []string `json:"groups"`
	BatchSize   int      `json:"batch_size"`
	Concurrency int      `json:"concurrency"`
}
