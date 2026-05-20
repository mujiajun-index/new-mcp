package dto

type CreateVisionConfigReq struct {
	Name         string `json:"name" binding:"required,min=1,max=128"`
	Description  string `json:"description"`
	Provider     string `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	ModelName    string `json:"model_name" binding:"required"`
	EndpointURL  string `json:"endpoint_url" binding:"required"`
	ApiKey       string `json:"api_key" binding:"required"`
	SystemPrompt string `json:"system_prompt"`
	MaxTokens    int    `json:"max_tokens"`
}

type UpdateVisionConfigReq struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Provider         *string `json:"provider"`
	ModelName        *string `json:"model_name"`
	EndpointURL      *string `json:"endpoint_url"`
	ApiKey           *string `json:"api_key"`
	SystemPrompt     *string `json:"system_prompt"`
	MaxTokens        *int    `json:"max_tokens"`
	AnalyzeImageName *string `json:"analyze_image_name"`
	AnalyzeImageDesc *string `json:"analyze_image_desc"`
	DescribeSceneName *string `json:"describe_scene_name"`
	DescribeSceneDesc *string `json:"describe_scene_desc"`
	Status           *int    `json:"status"`
}

type VisionConfigListItem struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	ModelName    string `json:"model_name"`
	EndpointURL  string `json:"endpoint_url"`
	AutoRegister bool   `json:"auto_register"`
	RegisteredServiceID *int64 `json:"registered_service_id"`
	Status       int    `json:"status"`
	CreatedAt    string `json:"created_at"`
}

type VisionConfigDetail struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	Provider            string `json:"provider"`
	ModelName           string `json:"model_name"`
	EndpointURL         string `json:"endpoint_url"`
	SystemPrompt        string `json:"system_prompt"`
	MaxTokens           int    `json:"max_tokens"`
	AutoRegister        bool   `json:"auto_register"`
	RegisteredServiceID *int64 `json:"registered_service_id"`
	AnalyzeImageName    string `json:"analyze_image_name"`
	AnalyzeImageDesc    string `json:"analyze_image_desc"`
	DescribeSceneName   string `json:"describe_scene_name"`
	DescribeSceneDesc   string `json:"describe_scene_desc"`
	ExtraConfig         string `json:"extra_config"`
	Status              int    `json:"status"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type TestVisionReq struct {
	Provider    string `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	EndpointURL string `json:"endpoint_url" binding:"required"`
	ApiKey      string `json:"api_key" binding:"required"`
	ModelName   string `json:"model_name" binding:"required"`
}

type ListModelsReq struct {
	Provider    string `json:"provider" binding:"required,oneof=openai anthropic gemini"`
	EndpointURL string `json:"endpoint_url" binding:"required"`
	ApiKey      string `json:"api_key"`
	ConfigID    int64  `json:"config_id"`
}

type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TestVisionResult struct {
	Success bool   `json:"success"`
	Result  string `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}
