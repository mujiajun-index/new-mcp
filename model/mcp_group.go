package model

import (
	"time"

	"gorm.io/gorm"
)

type McpGroup struct {
	ID               int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID           int64          `json:"user_id" gorm:"not null;uniqueIndex:idx_grp_user_name"`
	Name             string         `json:"name" gorm:"size:128;not null;uniqueIndex:idx_grp_user_name"`
	DisplayName      string         `json:"display_name" gorm:"size:255"`
	Description      string         `json:"description" gorm:"type:text"`
	IconURL          string         `json:"icon_url" gorm:"size:512"`
	Visibility       string         `json:"visibility" gorm:"size:16;default:private;index"`
	AutoDiscover     bool           `json:"auto_discover" gorm:"default:true"`
	EndpointSlug     string         `json:"endpoint_slug" gorm:"size:128;uniqueIndex"`
	EndpointAuth     string         `json:"endpoint_auth" gorm:"size:32;default:api_key"`
	ExposeMode       string         `json:"expose_mode" gorm:"size:16;default:smart"`
	MiddlewareConfig string         `json:"middleware_config" gorm:"type:text;default:'{}'"`
	Status           int            `json:"status" gorm:"default:1"`
	SortOrder        int            `json:"sort_order" gorm:"default:0"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

func (McpGroup) TableName() string { return "mcp_groups" }

func ListGroupsByUser(userID int64, offset, limit int) ([]McpGroup, int64, error) {
	var groups []McpGroup
	var total int64
	query := DB.Where("user_id = ?", userID)
	if err := query.Model(&McpGroup{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&groups).Error
	return groups, total, err
}

func GetGroupByID(userID, groupID int64) (*McpGroup, error) {
	var group McpGroup
	err := DB.Where("id = ? AND user_id = ?", groupID, userID).First(&group).Error
	return &group, err
}

func GetGroupBySlug(slug string) (*McpGroup, error) {
	var group McpGroup
	err := DB.Where("endpoint_slug = ?", slug).First(&group).Error
	return &group, err
}

func (g *McpGroup) Insert() error {
	return DB.Create(g).Error
}

func (g *McpGroup) Update() error {
	return DB.Save(g).Error
}

func CheckGroupNameExists(userID int64, name string, excludeID int64) (bool, error) {
	var count int64
	query := DB.Model(&McpGroup{}).Where("user_id = ? AND name = ?", userID, name)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

func (g *McpGroup) Delete() error {
	return DB.Delete(g).Error
}
