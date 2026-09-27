package model

import (
	"time"

	"gorm.io/gorm"
)

type SystemOneConfig struct {
	ID                  int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID              int64          `json:"user_id" gorm:"not null;index"`
	Name                string         `json:"name" gorm:"size:128;not null"`
	Description         string         `json:"description" gorm:"type:text"`
	Provider            string         `json:"provider" gorm:"size:32;not null"`
	EndpointURL         string         `json:"endpoint_url" gorm:"size:512;not null"`
	ModelName           string         `json:"model_name" gorm:"size:128;not null"`
	APIKey              string         `json:"-" gorm:"column:api_key;type:text;not null"`
	AutoRegister        bool           `json:"auto_register" gorm:"default:false"`
	RegisteredServiceID *int64         `json:"registered_service_id"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `json:"-" gorm:"index"`
}

func (SystemOneConfig) TableName() string { return "systemone_configs" }

func ListSystemOneConfigs(userID int64) ([]SystemOneConfig, error) {
	var configs []SystemOneConfig
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&configs).Error
	return configs, err
}

func GetSystemOneConfig(userID, id int64) (*SystemOneConfig, error) {
	var config SystemOneConfig
	err := DB.Where("id = ? AND user_id = ?", id, userID).First(&config).Error
	return &config, err
}

func GetSystemOneConfigByServiceID(serviceID int64) (*SystemOneConfig, error) {
	var config SystemOneConfig
	err := DB.Where("registered_service_id = ?", serviceID).First(&config).Error
	return &config, err
}
