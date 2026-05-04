package model

import (
	"time"

	"gorm.io/gorm"
)

type Camera struct {
	ID                 int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID             int64          `json:"user_id" gorm:"not null;index"`
	Name               string         `json:"name" gorm:"size:128;not null"`
	Description        string         `json:"description" gorm:"type:text"`
	SourceType         string         `json:"source_type" gorm:"size:32;not null"`
	SourceURL          string         `json:"source_url" gorm:"size:512;not null"`
	FPS                float64        `json:"fps" gorm:"type:decimal(4,1);default:1.0"`
	ResolutionW        int            `json:"resolution_w" gorm:"default:640"`
	ResolutionH        int            `json:"resolution_h" gorm:"default:480"`
	VisionConfigID     *int64         `json:"vision_config_id" gorm:"index"`
	AutoRegister       bool           `json:"auto_register" gorm:"default:true"`
	RegisteredServiceID *int64        `json:"registered_service_id"`
	Status             int            `json:"status" gorm:"default:1"`
	LastCaptureAt      *time.Time     `json:"last_capture_at"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Camera) TableName() string { return "cameras" }
