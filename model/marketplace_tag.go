package model

import (
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

// MarketplaceTag 市场标签字典(管理员维护预设标签)。市场项 tags 字段(逗号字符串)的每个值
// 须存在于本表启用记录(service 层校验),避免自由乱填(§11)。
type MarketplaceTag struct {
	ID          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string         `json:"name" gorm:"size:64;not null;uniqueIndex"`
	Description string         `json:"description" gorm:"type:text"`
	SortOrder   int            `json:"sort_order" gorm:"default:0"`
	Status      int            `json:"status" gorm:"default:1;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (MarketplaceTag) TableName() string { return "marketplace_tags" }

// ListAllMarketplaceTags 分页返回标签(admin;status<=0 表示不过滤)。
func ListAllMarketplaceTags(status, offset, limit int) ([]MarketplaceTag, int64, error) {
	query := DB.Model(&MarketplaceTag{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tags []MarketplaceTag
	err := query.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&tags).Error
	return tags, total, err
}

// ListEnabledMarketplaceTags 返回启用标签(供管理员编辑市场项时多选)。
func ListEnabledMarketplaceTags() ([]MarketplaceTag, error) {
	var tags []MarketplaceTag
	err := DB.Where("status = ?", common.StatusEnabled).
		Order("sort_order ASC, created_at DESC").Find(&tags).Error
	return tags, err
}

func GetMarketplaceTagByID(id int64) (*MarketplaceTag, error) {
	var tag MarketplaceTag
	err := DB.Where("id = ?", id).First(&tag).Error
	return &tag, err
}

// CountEnabledTagsByNames 返回 names 中存在于启用标签库的数量(供 validateTags 校验)。
func CountEnabledTagsByNames(names []string) (int64, error) {
	if len(names) == 0 {
		return 0, nil
	}
	var count int64
	err := DB.Model(&MarketplaceTag{}).
		Where("name IN ? AND status = ?", names, common.StatusEnabled).
		Count(&count).Error
	return count, err
}

func CheckMarketplaceTagNameExists(name string, excludeID int64) (bool, error) {
	var count int64
	query := DB.Model(&MarketplaceTag{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (t *MarketplaceTag) Insert() error { return DB.Create(t).Error }

func (t *MarketplaceTag) Update() error { return DB.Save(t).Error }

func (t *MarketplaceTag) Delete() error { return DB.Delete(t).Error }
