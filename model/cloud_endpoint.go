package model

import (
	"time"

	"gorm.io/gorm"
)

type CloudEndpoint struct {
	ID               int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID           int64          `json:"user_id" gorm:"not null;index"`
	Name             string         `json:"name" gorm:"size:128;not null"`
	CloudType        string         `json:"cloud_type" gorm:"size:32;not null;default:custom;index"`
	WssURL           string         `json:"wss_url" gorm:"size:1024"`
	CloudConfig      string         `json:"cloud_config" gorm:"type:varchar(4096);default:'{}'"`
	RemoteID         string         `json:"remote_id" gorm:"size:128"`
	TokenExpiresAt   *time.Time     `json:"token_expires_at"`
	ApiKeyID         *int64         `json:"api_key_id" gorm:"index"`
	GroupID          *int64         `json:"group_id" gorm:"index"`
	// 无 default 标签:default:true 时 GORM 在 INSERT 省略零值 false,「手动连接」
	// 落库变回自动连接,重启后 CloudManager 会违背用户意愿自动拉起。
	AutoConnect      bool           `json:"auto_connect"`
	ExposeMode       string         `json:"expose_mode" gorm:"size:16;default:smart"`
	ConnectionStatus string         `json:"connection_status" gorm:"size:16;default:disconnected;index"`
	LastConnectedAt  *time.Time     `json:"last_connected_at"`
	LastError        string         `json:"last_error" gorm:"type:text"`
	Status           int            `json:"status" gorm:"default:1"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}

func (CloudEndpoint) TableName() string { return "cloud_endpoints" }

func ListConnectionsByUser(userID int64) ([]CloudEndpoint, error) {
	var conns []CloudEndpoint
	err := DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&conns).Error
	return conns, err
}

func GetConnectionByID(userID, connID int64) (*CloudEndpoint, error) {
	var conn CloudEndpoint
	err := DB.Where("id = ? AND user_id = ?", connID, userID).First(&conn).Error
	return &conn, err
}

func (c *CloudEndpoint) Insert() error {
	return DB.Create(c).Error
}

func (c *CloudEndpoint) Update() error {
	return DB.Save(c).Error
}

func (c *CloudEndpoint) Delete() error {
	return DB.Delete(c).Error
}
