package model

import (
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

// MarketplaceGroup 市场分组(业务分类,管理员全局范围)。市场项通过 group_id 归属。
// 区别于 marketplace_items.category(instant/source 部署形态),本表是可管理的业务分类(§11)。
type MarketplaceGroup struct {
	ID          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string         `json:"name" gorm:"size:128;not null;uniqueIndex"`
	Description string         `json:"description" gorm:"type:text"`
	IconURL     string         `json:"icon_url" gorm:"size:512"`
	SortOrder   int            `json:"sort_order" gorm:"default:0"`
	Status      int            `json:"status" gorm:"default:1;index"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (MarketplaceGroup) TableName() string { return "marketplace_groups" }

// ListEnabledMarketplaceGroups 返回所有启用分组(按 sort_order),供广场左侧筛选(公开端点)。
func ListEnabledMarketplaceGroups() ([]MarketplaceGroup, error) {
	var groups []MarketplaceGroup
	err := DB.Where("status = ?", common.StatusEnabled).
		Order("sort_order ASC, created_at DESC").Find(&groups).Error
	return groups, err
}

// ListAllMarketplaceGroups 分页返回分组(admin;status<=0 表示不过滤)。
func ListAllMarketplaceGroups(status, offset, limit int) ([]MarketplaceGroup, int64, error) {
	query := DB.Model(&MarketplaceGroup{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var groups []MarketplaceGroup
	err := query.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&groups).Error
	return groups, total, err
}

// ListAllMarketplaceGroupsForAdmin 返回所有分组(不分页,管理 UI 用)。
func ListAllMarketplaceGroupsForAdmin() ([]MarketplaceGroup, error) {
	var groups []MarketplaceGroup
	err := DB.Order("sort_order ASC, created_at DESC").Find(&groups).Error
	return groups, err
}

func GetMarketplaceGroupByID(id int64) (*MarketplaceGroup, error) {
	var group MarketplaceGroup
	err := DB.Where("id = ?", id).First(&group).Error
	return &group, err
}

// GetEnabledMarketplaceGroupByID 取启用分组;未启用/不存在返回错误(供市场项 group_id 校验)。
func GetEnabledMarketplaceGroupByID(id int64) (*MarketplaceGroup, error) {
	var group MarketplaceGroup
	err := DB.Where("id = ? AND status = ?", id, common.StatusEnabled).First(&group).Error
	return &group, err
}

// GetMarketplaceGroupsByIDs 批量取分组(供市场项列表填充 group_name,避免 N+1)。
func GetMarketplaceGroupsByIDs(ids []int64) ([]MarketplaceGroup, error) {
	var groups []MarketplaceGroup
	if len(ids) == 0 {
		return groups, nil
	}
	err := DB.Where("id IN ?", ids).Find(&groups).Error
	return groups, err
}

func CheckMarketplaceGroupNameExists(name string, excludeID int64) (bool, error) {
	var count int64
	query := DB.Model(&MarketplaceGroup{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (g *MarketplaceGroup) Insert() error { return DB.Create(g).Error }

func (g *MarketplaceGroup) Update() error { return DB.Save(g).Error }

func (g *MarketplaceGroup) Delete() error { return DB.Delete(g).Error }
