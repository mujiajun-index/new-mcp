package model

import (
	"errors"
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

// MarketplaceItemKey 是市场条目秘钥池(仅 HTTP 类 instant 条目)中的一把秘钥。
// 一份池全局共享:全部安装该条目的用户经同一选择器轮换,坏 key 一次禁光。
// SortOrder 语义与 mcp_service_keys 一致:从 1 起 = 轮询次序 = 日志 key_index
// (0 保留给「未使用多秘钥」);状态挂行上,删 key 重排不串状态。
type MarketplaceItemKey struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	ItemID         int64      `json:"item_id" gorm:"not null;uniqueIndex:idx_item_key_pos"`
	SortOrder      int        `json:"sort_order" gorm:"not null;uniqueIndex:idx_item_key_pos"`
	Value          string     `json:"value" gorm:"type:text"`
	Status         int        `json:"status" gorm:"not null"`
	DisabledReason string     `json:"disabled_reason" gorm:"size:255"`
	DisabledAt     *time.Time `json:"disabled_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (MarketplaceItemKey) TableName() string { return "marketplace_item_keys" }

// ListKeysByItem 返回池内全部秘钥,按 SortOrder ASC(轮询次序)。
func ListKeysByItem(itemID int64) ([]MarketplaceItemKey, error) {
	var keys []MarketplaceItemKey
	err := DB.Where("item_id = ?", itemID).Order("sort_order ASC").Find(&keys).Error
	return keys, err
}

// AppendItemKeys 追加秘钥:对池内已有值去重,SortOrder 接在当前最大值之后;
// 既有行与状态一律不动(追加语义)。返回实际新增的行数。
func AppendItemKeys(itemID int64, values []string) (int64, error) {
	clean, err := CleanKeyValues(values)
	if err != nil {
		return 0, err
	}
	if len(clean) == 0 {
		return 0, nil
	}
	var existing []MarketplaceItemKey
	if err := DB.Where("item_id = ?", itemID).Find(&existing).Error; err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(existing))
	for _, k := range existing {
		seen[k.Value] = true
	}
	now := time.Now()
	rows := make([]MarketplaceItemKey, 0, len(clean))
	next := len(existing) + 1
	for _, v := range clean {
		if seen[v] {
			continue
		}
		seen[v] = true
		rows = append(rows, MarketplaceItemKey{
			ItemID:    itemID,
			SortOrder: next,
			Value:     v,
			Status:    common.StatusEnabled,
			CreatedAt: now,
			UpdatedAt: now,
		})
		next++
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if len(existing)+len(rows) > MaxServiceKeys {
		return 0, errors.New("秘钥数量超出上限(100)")
	}
	if err := DB.Create(&rows).Error; err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

// ReplaceItemKeys 替换全部秘钥:事务内删全量、按入参顺序从 1 重排插入,状态清零。
func ReplaceItemKeys(itemID int64, values []string) error {
	clean, err := CleanKeyValues(values)
	if err != nil {
		return err
	}
	if len(clean) == 0 || len(clean) > MaxServiceKeys {
		return errors.New("秘钥列表为空或超出上限(100)")
	}
	now := time.Now()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("item_id = ?", itemID).Delete(&MarketplaceItemKey{}).Error; err != nil {
			return err
		}
		rows := make([]MarketplaceItemKey, len(clean))
		for i, v := range clean {
			rows[i] = MarketplaceItemKey{
				ItemID:    itemID,
				SortOrder: i + 1,
				Value:     v,
				Status:    common.StatusEnabled,
				CreatedAt: now,
				UpdatedAt: now,
			}
		}
		return tx.Create(&rows).Error
	})
}

// SetItemKeyStatus 更新单把秘钥状态;启用时清空禁用原因/时间。
// 校验行属于指定条目,返回是否命中。
func SetItemKeyStatus(itemID, keyID int64, status int, reason string) (bool, error) {
	updates := map[string]interface{}{"status": status, "updated_at": time.Now()}
	if status == common.StatusEnabled {
		updates["disabled_reason"] = ""
		updates["disabled_at"] = nil
	} else {
		updates["disabled_reason"] = reason
		updates["disabled_at"] = time.Now()
	}
	res := DB.Model(&MarketplaceItemKey{}).
		Where("id = ? AND item_id = ?", keyID, itemID).
		Updates(updates)
	return res.Error == nil && res.RowsAffected > 0, res.Error
}

// DeleteItemKey 删除单把秘钥(校验归属),返回是否命中。
func DeleteItemKey(itemID, keyID int64) (bool, error) {
	res := DB.Where("id = ? AND item_id = ?", keyID, itemID).Delete(&MarketplaceItemKey{})
	return res.Error == nil && res.RowsAffected > 0, res.Error
}

// BatchEnableAllItemKeys 全部启用(清禁用原因/时间)。
func BatchEnableAllItemKeys(itemID int64) (int64, error) {
	res := DB.Model(&MarketplaceItemKey{}).Where("item_id = ?", itemID).Updates(map[string]interface{}{
		"status":          common.StatusEnabled,
		"disabled_reason": "",
		"disabled_at":     nil,
		"updated_at":      time.Now(),
	})
	return res.RowsAffected, res.Error
}

// DeleteDisabledItemKeys 删除所有已禁用(手动+自动)的秘钥。
func DeleteDisabledItemKeys(itemID int64) (int64, error) {
	res := DB.Where("item_id = ? AND status <> ?", itemID, common.StatusEnabled).Delete(&MarketplaceItemKey{})
	return res.RowsAffected, res.Error
}

// DeleteKeysByItem 删条目时一并清池。
func DeleteKeysByItem(itemID int64) error {
	return DB.Where("item_id = ?", itemID).Delete(&MarketplaceItemKey{}).Error
}

// CountItemKeys 点查池内秘钥总数与启用数(管理详情徽章用)。
func CountItemKeys(itemID int64) (total, enabled int64) {
	DB.Model(&MarketplaceItemKey{}).Where("item_id = ?", itemID).Count(&total)
	DB.Model(&MarketplaceItemKey{}).Where("item_id = ? AND status = ?", itemID, common.StatusEnabled).Count(&enabled)
	return total, enabled
}
