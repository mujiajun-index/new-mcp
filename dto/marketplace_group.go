package dto

// --- Marketplace group (业务分类) ---

type CreateMarketplaceGroupReq struct {
	Name        string  `json:"name" binding:"required,min=1,max=128"`
	Description string  `json:"description"`
	IconURL     string  `json:"icon_url" binding:"omitempty,max=512"`
	SortOrder   int     `json:"sort_order"`
	Status      *int    `json:"status"`
}

type UpdateMarketplaceGroupReq struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=128"`
	Description *string `json:"description"`
	IconURL     *string `json:"icon_url" binding:"omitempty,max=512"`
	SortOrder   *int    `json:"sort_order"`
	Status      *int    `json:"status"`
}

type MarketplaceGroupItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
	SortOrder   int    `json:"sort_order"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
}
