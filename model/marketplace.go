package model

import (
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

type MarketplaceItem struct {
	ID                   int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	AdminID              int64          `json:"admin_id" gorm:"not null"`
	Name                 string         `json:"name" gorm:"size:128;not null;uniqueIndex"`
	DisplayName          string         `json:"display_name" gorm:"size:255"`
	Description          string         `json:"description" gorm:"type:text"`
	IconURL              string         `json:"icon_url" gorm:"size:512"`
	Category             string         `json:"category" gorm:"size:32;not null;index"`
	Tags                 string         `json:"tags" gorm:"size:512"`
	Version              string         `json:"version" gorm:"size:32;default:1.0.0"`
	TransportType        string         `json:"transport_type" gorm:"size:32"`
	ConfigTemplate       string         `json:"config_template" gorm:"type:text;default:'{}'"`
	AuthInstructions     string         `json:"auth_instructions" gorm:"type:text"`
	RepoURL              string         `json:"repo_url" gorm:"size:1024"`
	InstallGuide         string         `json:"install_guide" gorm:"type:text"`
	ConfigTemplateSource string         `json:"config_template_source" gorm:"type:text;default:'{}'"`
	RequiredEnv          string         `json:"required_env" gorm:"type:text;default:'[]'"`
	InstallCount         int            `json:"install_count" gorm:"default:0"`
	RatingAvg            float64        `json:"rating_avg" gorm:"type:decimal(2,1);default:0.0"`
	RatingCount          int            `json:"rating_count" gorm:"default:0"`
	ToolsSnapshot        string         `json:"tools_snapshot" gorm:"type:mediumtext;default:'[]'"`
	Status               int            `json:"status" gorm:"default:1;index"`
	SortOrder            int            `json:"sort_order" gorm:"default:0"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

func (MarketplaceItem) TableName() string { return "marketplace_items" }

type MarketplaceReview struct {
	ID            int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID        int64          `json:"user_id" gorm:"not null;index"`
	ItemID        int64          `json:"item_id" gorm:"not null;index"`
	Rating        int            `json:"rating" gorm:"default:0"`
	ReviewText    string         `json:"review_text" gorm:"type:text"`
	ReviewStatus  string         `json:"review_status" gorm:"size:16;default:pending;index"`
	ReviewerID    *int64         `json:"reviewer_id"`
	ReviewComment string         `json:"review_comment" gorm:"type:text"`
	ReviewedAt    *time.Time     `json:"reviewed_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

func (MarketplaceReview) TableName() string { return "marketplace_reviews" }

// --- MarketplaceItem queries ---

func GetMarketplaceItemByID(id int64) (*MarketplaceItem, error) {
	var item MarketplaceItem
	err := DB.Where("id = ?", id).First(&item).Error
	return &item, err
}

func ListPublishedMarketplaceItems(offset, limit int, category, keyword string) ([]MarketplaceItem, int64, error) {
	query := DB.Where("status = ?", common.StatusEnabled)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if keyword != "" {
		query = query.Where("name LIKE ? OR display_name LIKE ? OR description LIKE ?",
			"%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	var total int64
	if err := query.Model(&MarketplaceItem{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []MarketplaceItem
	err := query.Order("sort_order ASC, install_count DESC, created_at DESC").
		Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func ListAllMarketplaceItems(offset, limit int) ([]MarketplaceItem, int64, error) {
	var total int64
	if err := DB.Model(&MarketplaceItem{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []MarketplaceItem
	err := DB.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func IncrementInstallCount(itemID int64) error {
	return DB.Model(&MarketplaceItem{}).Where("id = ?", itemID).
		UpdateColumn("install_count", gorm.Expr("install_count + 1")).Error
}

func UpdateRating(itemID int64) error {
	var result struct {
		Avg   float64
		Count int
	}
	DB.Model(&MarketplaceReview{}).Where("item_id = ? AND review_status = ?",
		itemID, "approved").Select("AVG(rating) as avg, COUNT(*) as count").Scan(&result)
	return DB.Model(&MarketplaceItem{}).Where("id = ?", itemID).Updates(map[string]interface{}{
		"rating_avg":   result.Avg,
		"rating_count": result.Count,
	}).Error
}

func (i *MarketplaceItem) Insert() error {
	return DB.Create(i).Error
}

func (i *MarketplaceItem) Update() error {
	return DB.Save(i).Error
}

func (i *MarketplaceItem) Delete() error {
	return DB.Delete(i).Error
}

// --- MarketplaceReview queries ---

func GetUserReviewForItem(userID, itemID int64) (*MarketplaceReview, error) {
	var review MarketplaceReview
	err := DB.Where("user_id = ? AND item_id = ?", userID, itemID).First(&review).Error
	return &review, err
}

func (r *MarketplaceReview) Insert() error {
	return DB.Create(r).Error
}

func (r *MarketplaceReview) Update() error {
	return DB.Save(r).Error
}
