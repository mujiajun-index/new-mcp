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
