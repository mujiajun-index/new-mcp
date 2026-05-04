package dto

type PaginationReq struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

func (p *PaginationReq) GetPage() int {
	if p.Page <= 0 {
		return 1
	}
	return p.Page
}

func (p *PaginationReq) GetPageSize() int {
	if p.PageSize <= 0 {
		return 20
	}
	if p.PageSize > 100 {
		return 100
	}
	return p.PageSize
}
