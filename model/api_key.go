package model

import (
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

type ApiKey struct {
	ID          int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      int64          `json:"user_id" gorm:"not null;index"`
	Name        string         `json:"name" gorm:"size:128;not null"`
	KeyHash     string         `json:"-" gorm:"column:key_hash;size:255;not null;uniqueIndex"`
	KeyPrefix   string         `json:"key_prefix" gorm:"column:key_prefix;size:16;not null;index"`
	Permissions string         `json:"permissions" gorm:"type:text;default:'{}'"`
	Status      int            `json:"status" gorm:"default:1"`
	ExpiresAt   *time.Time     `json:"expires_at"`
	LastUsedAt  *time.Time     `json:"last_used_at"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

func (ApiKey) TableName() string { return "api_keys" }

func GetApiKeyByHash(keyHash string) (*ApiKey, error) {
	var key ApiKey
	err := DB.Where("key_hash = ? AND status = ?", keyHash, common.StatusEnabled).First(&key).Error
	return &key, err
}

func ListApiKeysByUser(userID int64) ([]ApiKey, error) {
	var keys []ApiKey
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

func (k *ApiKey) Insert() error {
	return DB.Create(k).Error
}

func (k *ApiKey) Delete() error {
	return DB.Delete(k).Error
}
