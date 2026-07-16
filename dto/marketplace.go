package dto

// --- Admin: Create/Update marketplace item ---

type CreateMarketplaceItemReq struct {
	Name                 string                 `json:"name" binding:"required,min=1,max=128"`
	DisplayName          string                 `json:"display_name" binding:"omitempty,max=255"`
	Description          string                 `json:"description"`
	IconURL              string                 `json:"icon_url" binding:"omitempty,max=512"`
	Category             string                 `json:"category" binding:"required,oneof=instant source"`
	Tags                 []string               `json:"tags"`
	Version              string                 `json:"version" binding:"omitempty,max=32"`
	TransportType        string                 `json:"transport_type" binding:"required,oneof=stdio sse streamable-http websocket passive-ws"`
	ConfigTemplate       map[string]interface{} `json:"config_template"`
	AuthInstructions     string                 `json:"auth_instructions"`
	RepoURL              string                 `json:"repo_url" binding:"omitempty,max=1024"`
	InstallGuide         string                 `json:"install_guide"`
	ConfigTemplateSource map[string]interface{} `json:"config_template_source"`
	RequiredEnv          []string               `json:"required_env"`
	ToolsSnapshot        []interface{}          `json:"tools_snapshot"`
	Status               *int                   `json:"status"`
	// 商业化定价(§5):非自用模式上架必须显式定价(§5.6)
	BillingType   string  `json:"billing_type"`     // free / per_call(默认 per_call)
	PricePerCall  float64 `json:"price_per_call"`   // 展示货币单价(per_call 时需 >0,非自用模式)
}

type UpdateMarketplaceItemReq struct {
	DisplayName          *string                `json:"display_name"`
	Description          *string                `json:"description"`
	IconURL              *string                `json:"icon_url"`
	Category             *string                `json:"category"`
	Tags                 []string               `json:"tags"`
	Version              *string                `json:"version"`
	TransportType        *string                `json:"transport_type"`
	ConfigTemplate       map[string]interface{} `json:"config_template"`
	AuthInstructions     *string                `json:"auth_instructions"`
	RepoURL              *string                `json:"repo_url"`
	InstallGuide         *string                `json:"install_guide"`
	ConfigTemplateSource map[string]interface{} `json:"config_template_source"`
	RequiredEnv          []string               `json:"required_env"`
	ToolsSnapshot        []interface{}          `json:"tools_snapshot"`
	Status               *int                   `json:"status"`
	SortOrder            *int                   `json:"sort_order"`
	// 商业化定价:启用/上架时非自用模式须显式定价(§5.6)
	BillingType  *string  `json:"billing_type"`
	PricePerCall *float64 `json:"price_per_call"`
}

// BatchPricingReq 批量设置已上架市场服务价格(§5.5)。
type BatchPricingReq struct {
	Items []BatchPricingItem `json:"items" binding:"required,min=1,dive"`
}

type BatchPricingItem struct {
	ID           int64   `json:"id" binding:"required"`
	BillingType  string  `json:"billing_type" binding:"required,oneof=free per_call"`
	PricePerCall float64 `json:"price_per_call"`
}

// CloneMarketplaceReq 从自有服务克隆上架(§11/D14):深拷贝 transport/config/auth/tools,与源服务无关联。
type CloneMarketplaceReq struct {
	FromServiceID int64   `json:"from_service_id" binding:"required"`
	Name          string  `json:"name" binding:"required,min=1,max=128"`
	DisplayName   string  `json:"display_name"`
	Description   string  `json:"description"`
	BillingType   string  `json:"billing_type"`
	PricePerCall  float64 `json:"price_per_call"`
}

type MarketplaceListItem struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	IconURL       string   `json:"icon_url"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	Version       string   `json:"version"`
	TransportType string   `json:"transport_type"`
	InstallCount  int      `json:"install_count"`
	RatingAvg     float64  `json:"rating_avg"`
	RatingCount   int      `json:"rating_count"`
	Status        int      `json:"status"`
	SortOrder     int      `json:"sort_order"`
	CreatedAt     string   `json:"created_at"`
	// 商业化定价(供市场列表展示价格/免费标记)
	BillingType  string  `json:"billing_type"`
	PricePerCall float64 `json:"price_per_call"`
}

type MarketplaceDetail struct {
	ID                   int64                  `json:"id"`
	Name                 string                 `json:"name"`
	DisplayName          string                 `json:"display_name"`
	Description          string                 `json:"description"`
	IconURL              string                 `json:"icon_url"`
	Category             string                 `json:"category"`
	Tags                 []string               `json:"tags"`
	Version              string                 `json:"version"`
	TransportType        string                 `json:"transport_type"`
	ConfigTemplateSource map[string]interface{} `json:"config_template_source"`
	AuthInstructions     string                 `json:"auth_instructions"`
	RepoURL              string                 `json:"repo_url"`
	InstallGuide         string                 `json:"install_guide"`
	RequiredEnv          []string               `json:"required_env"`
	InstallCount         int                    `json:"install_count"`
	RatingAvg            float64                `json:"rating_avg"`
	RatingCount          int                    `json:"rating_count"`
	ToolsSnapshot        []interface{}          `json:"tools_snapshot"`
	Status               int                    `json:"status"`
	CreatedAt            string                 `json:"created_at"`
	UpdatedAt            string                 `json:"updated_at"`
	// 商业化定价
	BillingType  string  `json:"billing_type"`
	PricePerCall float64 `json:"price_per_call"`
}

// --- User: Install from marketplace ---

type InstallFromMarketplaceReq struct {
	ItemID      int64  `json:"item_id" binding:"required"`
	NameOverride string `json:"name_override"`
}

type InstallResult struct {
	ServiceID int64  `json:"service_id"`
	Name      string `json:"name"`
}

// --- User: Rate/Review ---

type CreateReviewReq struct {
	ItemID     int64  `json:"item_id" binding:"required"`
	Rating     int    `json:"rating" binding:"required,min=1,max=5"`
	ReviewText string `json:"review_text" binding:"max=1000"`
}
