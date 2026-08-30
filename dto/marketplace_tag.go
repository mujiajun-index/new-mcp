package dto

// --- Marketplace tag dictionary (预设标签) ---

type CreateMarketplaceTagReq struct {
	Name        string `json:"name" binding:"required,min=1,max=64"`
	Description string `json:"description"`
	Color       string `json:"color" binding:"omitempty,max=16"` // #RRGGBB;空=默认灰
	SortOrder   int    `json:"sort_order"`
	Status      *int   `json:"status"`
}

type UpdateMarketplaceTagReq struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=64"`
	Description *string `json:"description"`
	Color       *string `json:"color" binding:"omitempty,max=16"` // #RRGGBB;空=清除颜色
	SortOrder   *int    `json:"sort_order"`
	Status      *int    `json:"status"`
}

type MarketplaceTagItem struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	SortOrder   int    `json:"sort_order"`
	Status      int    `json:"status"`
	CreatedAt   string `json:"created_at"`
}
