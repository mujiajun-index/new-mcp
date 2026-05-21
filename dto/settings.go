package dto

type SettingItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SettingUpdateReq struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}
