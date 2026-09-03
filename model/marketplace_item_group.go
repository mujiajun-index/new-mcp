package model

import (
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

// MarketplaceItemGroup 市场项↔市场分组的多对多绑定(显式关联表,替代 marketplace_items.group_id 单列)。
// 复合唯一 (item_id, group_id):同一分组对同一市场项仅一行;复合索引 item_id 前缀即覆盖
// "按项查全部分组"路径,group_id 另建单列索引覆盖分组禁用/删除摘除与广场筛选路径。
// 仅 CreatedAt:绑定行随两端行增删,无独立生命周期(同 mcp_group_services 先例)。
type MarketplaceItemGroup struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	ItemID    int64     `json:"item_id" gorm:"not null;uniqueIndex:idx_item_group"`
	GroupID   int64     `json:"group_id" gorm:"not null;uniqueIndex:idx_item_group;index"`
	CreatedAt time.Time `json:"created_at"`
}

func (MarketplaceItemGroup) TableName() string { return "marketplace_item_groups" }

// GetMarketplaceItemGroupsByItemIDs 批量取多个市场项的分组绑定(列表/详情填充用,避免 N+1)。
func GetMarketplaceItemGroupsByItemIDs(itemIDs []int64) ([]MarketplaceItemGroup, error) {
	var rows []MarketplaceItemGroup
	if len(itemIDs) == 0 {
		return rows, nil
	}
	err := DB.Where("item_id IN ?", itemIDs).Order("item_id ASC, id ASC").Find(&rows).Error
	return rows, err
}

// ReplaceMarketplaceItemGroups 全量替换某市场项的分组绑定(删旧插新;须在事务内调用)。
// groupIDs 须为调用方清洗后的干净列表(去重/去非正)。
func ReplaceMarketplaceItemGroups(tx *gorm.DB, itemID int64, groupIDs []int64) error {
	if err := tx.Where("item_id = ?", itemID).Delete(&MarketplaceItemGroup{}).Error; err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]MarketplaceItemGroup, len(groupIDs))
	for i, gid := range groupIDs {
		rows[i] = MarketplaceItemGroup{ItemID: itemID, GroupID: gid}
	}
	return tx.Create(&rows).Error
}

// BatchReplaceMarketplaceItemGroups 批量全量替换多个市场项的分组绑定(删旧插新;
// 须在事务内调用)。groupIDs 须为调用方清洗后的干净列表(去重/去非正);空列表=清空。
func BatchReplaceMarketplaceItemGroups(tx *gorm.DB, itemIDs []int64, groupIDs []int64) error {
	if len(itemIDs) == 0 {
		return nil
	}
	if err := tx.Where("item_id IN ?", itemIDs).Delete(&MarketplaceItemGroup{}).Error; err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]MarketplaceItemGroup, 0, len(itemIDs)*len(groupIDs))
	for _, itemID := range itemIDs {
		for _, gid := range groupIDs {
			rows = append(rows, MarketplaceItemGroup{ItemID: itemID, GroupID: gid})
		}
	}
	return tx.CreateInBatches(rows, 500).Error
}

// DeleteMarketplaceItemGroupsByGroupID 摘除某分组下的全部绑定(分组禁用/删除时同步,
// 维持"市场项绑定 ⊆ 启用分组"不变量;须在事务内调用)。
func DeleteMarketplaceItemGroupsByGroupID(tx *gorm.DB, groupID int64) error {
	return tx.Where("group_id = ?", groupID).Delete(&MarketplaceItemGroup{}).Error
}

// DeleteMarketplaceItemGroupsByItemID 摘除某市场项的全部绑定(市场项删除时清理悬空引用)。
func DeleteMarketplaceItemGroupsByItemID(tx *gorm.DB, itemID int64) error {
	return tx.Where("item_id = ?", itemID).Delete(&MarketplaceItemGroup{}).Error
}

// CountPublishedItemsPerGroup 统计每个分组下已上架(status=启用且未软删)市场项的数量,
// 返回 group_id → 数量(不在 map 的分组计 0)。口径与 ListPublishedMarketplaceItems 的
// 组筛选一致(同一"已上架项 id"子查询),保证计数=点击该分组筛出的条数。
func CountPublishedItemsPerGroup() (map[int64]int64, error) {
	type row struct {
		GroupID int64
		Cnt     int64
	}
	var rows []row
	sub := DB.Model(&MarketplaceItem{}).Select("id").Where("status = ?", common.StatusEnabled)
	err := DB.Model(&MarketplaceItemGroup{}).
		Select("group_id, COUNT(*) AS cnt").
		Where("item_id IN (?)", sub).
		Group("group_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[int64]int64, len(rows))
	for _, r := range rows {
		counts[r.GroupID] = r.Cnt
	}
	return counts, nil
}
