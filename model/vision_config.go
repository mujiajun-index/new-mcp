package model

import (
	"time"

	"gorm.io/gorm"
)

type VisionConfig struct {
	ID                 int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID             int64          `json:"user_id" gorm:"not null;index"`
	Name               string         `json:"name" gorm:"size:128;not null"`
	Description        string         `json:"description" gorm:"type:text"`
	Provider           string         `json:"provider" gorm:"size:32;not null;index"`
	ModelName          string         `json:"model_name" gorm:"size:128"`
	EndpointURL        string         `json:"endpoint_url" gorm:"size:512"`
	ApiKey             string         `json:"-" gorm:"column:api_key;size:512"`
	SystemPrompt       string         `json:"system_prompt" gorm:"type:text"`
	MaxTokens          int            `json:"max_tokens" gorm:"default:4096"`
	AutoRegister       bool           `json:"auto_register" gorm:"default:true"`
	RegisteredServiceID *int64        `json:"registered_service_id"`
	Status             int            `json:"status" gorm:"default:1"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

func (VisionConfig) TableName() string { return "vision_configs" }

func ListVisionConfigsByUser(userID int64) ([]VisionConfig, error) {
	var configs []VisionConfig
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error
	return configs, err
}

func (v *VisionConfig) Insert() error {
	return DB.Create(v).Error
}

func (v *VisionConfig) Update() error {
	return DB.Save(v).Error
}

func (v *VisionConfig) Delete() error {
	return DB.Delete(v).Error
}
