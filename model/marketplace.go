package model

import (
	"time"

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
	Name          string         `json:"name" gorm:"size:128;not null"`
	DisplayName   string         `json:"display_name" gorm:"size:255"`
	Description   string         `json:"description" gorm:"type:text"`
	Category      string         `json:"category" gorm:"size:32;not null"`
	Submission    string         `json:"submission" gorm:"type:mediumtext;not null"`
	ReviewStatus  string         `json:"review_status" gorm:"size:16;default:pending;index"`
	ReviewerID    *int64         `json:"reviewer_id"`
	ReviewComment string         `json:"review_comment" gorm:"type:text"`
	ReviewedAt    *time.Time     `json:"reviewed_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

func (MarketplaceReview) TableName() string { return "marketplace_reviews" }
