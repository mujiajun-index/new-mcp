package model

import "time"

// McpGroupItem 分组内资源/提示的条目级启停覆盖。
// 语义与 mcp_group_tools 一致:无行 = 启用;仅 Enabled=false 的行会生效。
// ItemKind: "resource"(静态资源,ItemKey=原始 URI) / "template"(资源模板,ItemKey=uriTemplate)
//
//	/ "prompt"(提示,ItemKey=提示名)。
type McpGroupItem struct {
	ID        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	GroupID   int64  `json:"group_id" gorm:"not null;uniqueIndex:idx_group_service_kind_key;index"`
	ServiceID int64  `json:"service_id" gorm:"not null;uniqueIndex:idx_group_service_kind_key"`
	ItemKind  string `json:"item_kind" gorm:"size:16;not null;uniqueIndex:idx_group_service_kind_key"`
	ItemKey   string `json:"item_key" gorm:"size:512;not null;uniqueIndex:idx_group_service_kind_key"`
	// 无 default 标签:GORM 对带 default 的 bool 零值(false)会在 INSERT 时省略该列,
	// 落库变回默认 true,"首次勾选禁用"会静默失效;这里显式写入每个值。
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func (McpGroupItem) TableName() string { return "mcp_group_items" }

// GetGroupItems 返回该分组的全部条目级覆盖(所有 kind)。
func GetGroupItems(groupID int64) ([]McpGroupItem, error) {
	var items []McpGroupItem
	err := DB.Where("group_id = ?", groupID).Find(&items).Error
	return items, err
}

// GetGroupItemsByGroupIDsAndKinds 批量返回多个分组在指定 kind 集合内的覆盖(网关聚合单次查询)。
func GetGroupItemsByGroupIDsAndKinds(groupIDs []int64, kinds []string) ([]McpGroupItem, error) {
	if len(groupIDs) == 0 || len(kinds) == 0 {
		return nil, nil
	}
	var items []McpGroupItem
	err := DB.Where("group_id IN ? AND item_kind IN ?", groupIDs, kinds).Find(&items).Error
	return items, err
}

func GetGroupItem(groupID, serviceID int64, kind, key string) (*McpGroupItem, error) {
	var item McpGroupItem
	err := DB.Where("group_id = ? AND service_id = ? AND item_kind = ? AND item_key = ?",
		groupID, serviceID, kind, key).First(&item).Error
	return &item, err
}

func (i *McpGroupItem) Upsert() error {
	return DB.Save(i).Error
}

// DeleteGroupItemsByServiceID 删除该服务在所有分组中的资源/提示级覆盖。
// 与 DeleteGroupToolsByServiceID 配合，在删除服务时一并清理。
func DeleteGroupItemsByServiceID(serviceID int64) error {
	return DB.Where("service_id = ?", serviceID).Delete(&McpGroupItem{}).Error
}
