package dto

// --- Admin: Update marketplace item ---
// 注:市场项创建仅支持"从自有服务克隆上架"(CloneMarketplaceReq),无手动创建。

type UpdateMarketplaceItemReq struct {
	DisplayName          *string                `json:"display_name"`
	Description          *string                `json:"description"`
	IconURL              *string                `json:"icon_url"`
	Category             *string                `json:"category"`
	GroupID              *int64                 `json:"group_id"`
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
	ResourcesSnapshot    *map[string]interface{} `json:"resources_snapshot"` // {"resources":[],"templates":[]}
	PromptsSnapshot      *[]interface{}         `json:"prompts_snapshot"`
	Status               *int                   `json:"status"`
	SortOrder            *int                   `json:"sort_order"`
	// 商业化定价:启用/上架时非自用模式须显式定价(§5.6)
	BillingType  *string  `json:"billing_type" binding:"omitempty,oneof=free per_call"`
	PricePerCall *float64 `json:"price_per_call" binding:"omitempty,gte=0"`
}

// BatchPricingReq 批量设置已上架市场服务价格(§5.5)。
type BatchPricingReq struct {
	Items []BatchPricingItem `json:"items" binding:"required,min=1,dive"`
}

type BatchPricingItem struct {
	ID           int64   `json:"id" binding:"required"`
	BillingType  string  `json:"billing_type" binding:"required,oneof=free per_call"`
	PricePerCall float64 `json:"price_per_call" binding:"gte=0"`
}

// CloneMarketplaceReq 从自有服务克隆上架(§11/D14):深拷贝 transport/config/auth/tools,与源服务无关联。
type CloneMarketplaceReq struct {
	FromServiceID int64   `json:"from_service_id" binding:"required"`
	Name          string  `json:"name" binding:"required,min=1,max=128"`
	DisplayName   string  `json:"display_name"`
	Description   string  `json:"description"`
	BillingType   string  `json:"billing_type" binding:"omitempty,oneof=free per_call"`
	PricePerCall  float64 `json:"price_per_call" binding:"gte=0"`
}

// MarketplaceRefreshResult 市场项快照手动刷新结果(各快照条目数)。
type MarketplaceRefreshResult struct {
	ToolsCount     int `json:"tools_count"`
	ResourcesCount int `json:"resources_count"`
	TemplatesCount int `json:"templates_count"`
	PromptsCount   int `json:"prompts_count"`
}

type MarketplaceListItem struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name"`
	Description   string   `json:"description"`
	IconURL       string   `json:"icon_url"`
	Category      string   `json:"category"`
	GroupID       *int64   `json:"group_id"`
	GroupName     string   `json:"group_name"`
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
	GroupID              *int64                 `json:"group_id"`
	GroupName            string                 `json:"group_name"`
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
	ResourcesSnapshot    map[string]interface{} `json:"resources_snapshot"` // {"resources":[],"templates":[]}
	PromptsSnapshot      []interface{}          `json:"prompts_snapshot"`
	// 上游握手信息(克隆/刷新时捕获):真实 serverInfo 与协商协议版本
	ServerInfo      map[string]interface{} `json:"server_info"`
	ProtocolVersion string                 `json:"protocol_version"`
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
