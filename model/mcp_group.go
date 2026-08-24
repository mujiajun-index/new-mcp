package model

import (
	"time"

	"gorm.io/gorm"
)

type McpGroup struct {
	ID               int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	// UserID 同时参与 (user_id, name) 与 (user_id, endpoint_slug) 两个复合唯一索引:
	// 分组名与端点标识都只在同一用户内唯一,不同用户可重名(用户隔离)。
	UserID           int64          `json:"user_id" gorm:"not null;uniqueIndex:idx_grp_user_name;uniqueIndex:idx_grp_user_slug"`
	Name             string         `json:"name" gorm:"size:128;not null;uniqueIndex:idx_grp_user_name"`
	DisplayName      string         `json:"display_name" gorm:"size:255"`
	Description      string         `json:"description" gorm:"type:text"`
	IconURL          string         `json:"icon_url" gorm:"size:512"`
	Visibility       string         `json:"visibility" gorm:"size:16;default:private;index"`
	// 无 default 标签(GORM 对带 default 的 bool 零值 INSERT 时省略该列);创建时
	// service 层显式置 true,与原 DB 默认语义一致。
	AutoDiscover     bool           `json:"auto_discover"`
	EndpointSlug     string         `json:"endpoint_slug" gorm:"size:128;not null;uniqueIndex:idx_grp_user_slug"`
	EndpointAuth     string         `json:"endpoint_auth" gorm:"size:32;default:api_key"`
	ExposeMode       string         `json:"expose_mode" gorm:"size:16;default:smart"`
	MiddlewareConfig string         `json:"middleware_config" gorm:"type:varchar(4096);default:'{}'"`
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

// GetGroupBySlug 按端点标识解析分组,限定在请求者(经 API Key 认证的用户)名下:
// endpoint_slug 只保证用户内唯一,不同用户可重名,必须在解析时就带上 user_id 隔离,
// 否则 /mcp/group/:slug 会命中他人的同名分组。
func GetGroupBySlug(userID int64, slug string) (*McpGroup, error) {
	var group McpGroup
	err := DB.Where("user_id = ? AND endpoint_slug = ?", userID, slug).First(&group).Error
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

// Delete 真删除分组:事务内级联清理成员关系/工具过滤/资源提示条目三个子表后,
// 物理删除分组行(Unscoped)。不做软删除——(user_id, name/endpoint_slug) 复合唯一
// 索引不排除已删行,软删除会让同用户永远建不回同名分组。调用日志按 group_name
// 快照保留,不依赖分组行存在。
func (g *McpGroup) Delete() error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", g.ID).Delete(&McpGroupService{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", g.ID).Delete(&McpGroupTool{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", g.ID).Delete(&McpGroupItem{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(g).Error
	})
}
