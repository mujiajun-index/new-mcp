package dto

type SystemOneConfigReq struct {
	Name        string `json:"name" binding:"required,min=1,max=128"`
	Description string `json:"description"`
	Provider    string `json:"provider" binding:"required,oneof=typesafe openrouter"`
	EndpointURL string `json:"endpoint_url"`
	ModelName   string `json:"model_name"`
	APIKey      string `json:"api_key"`
}

type SystemOneConfigDetail struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	Provider            string `json:"provider"`
	EndpointURL         string `json:"endpoint_url"`
	ResolvedURL         string `json:"resolved_url"`
	ModelName           string `json:"model_name"`
	HasAPIKey           bool   `json:"has_api_key"`
	AutoRegister        bool   `json:"auto_register"`
	RegisteredServiceID *int64 `json:"registered_service_id"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type TestSystemOneReq struct {
	ConfigID    int64  `json:"config_id"`
	Provider    string `json:"provider"`
	EndpointURL string `json:"endpoint_url"`
	ModelName   string `json:"model_name"`
	APIKey      string `json:"api_key"`
}

type TestSystemOneResult struct {
	Success bool   `json:"success"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}
