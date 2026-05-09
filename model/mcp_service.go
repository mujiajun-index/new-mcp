package model

import (
	"time"
)

type McpService struct {
	ID               int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID           int64          `json:"user_id" gorm:"not null;uniqueIndex:idx_svc_user_name"`
	Name             string         `json:"name" gorm:"size:128;not null;uniqueIndex:idx_svc_user_name"`
	DisplayName      string         `json:"display_name" gorm:"size:255"`
	Description      string         `json:"description" gorm:"type:text"`
	TransportType    string         `json:"transport_type" gorm:"size:32;not null;index"`
	Config           string         `json:"config" gorm:"type:text;default:'{}'"`
	PassiveToken     string         `json:"-" gorm:"column:passive_token;size:512"`
	PassiveConnected bool           `json:"passive_connected" gorm:"default:false"`
	AuthType         string         `json:"auth_type" gorm:"size:32;default:none"`
	AuthConfig       string         `json:"auth_config" gorm:"type:text;default:'{}'"`
	ToolsCache       string         `json:"tools_cache" gorm:"type:mediumtext;default:'[]'"`
	ToolsUpdatedAt   *time.Time     `json:"tools_updated_at"`
	HealthStatus     string         `json:"health_status" gorm:"size:16;default:unknown;index"`
	LastHealthCheck  *time.Time     `json:"last_health_check"`
	ServerInfo       string         `json:"server_info" gorm:"type:text;default:'{}'"`
	ProtocolVersion  string         `json:"protocol_version" gorm:"size:32"`
	IconURL          string         `json:"icon_url" gorm:"size:512"`
	Tags             string         `json:"tags" gorm:"size:512"`
	Visibility       string         `json:"visibility" gorm:"size:16;default:private;index"`
	Source           string         `json:"source" gorm:"size:16;default:user"`
	MarketplaceItemID *int64        `json:"marketplace_item_id"`
	SortOrder        int            `json:"sort_order" gorm:"default:0"`
	Status           int            `json:"status" gorm:"default:1"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (McpService) TableName() string { return "mcp_services" }

func ListServicesByUser(userID int64, offset, limit int, filters map[string]string) ([]McpService, int64, error) {
	var services []McpService
	query := DB.Where("user_id = ?", userID)
	if v, ok := filters["transport_type"]; ok && v != "" {
		query = query.Where("transport_type = ?", v)
	}
	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("status = ?", v)
	}
	if v, ok := filters["keyword"]; ok && v != "" {
		query = query.Where("name LIKE ? OR display_name LIKE ? OR description LIKE ?", "%"+v+"%", "%"+v+"%", "%"+v+"%")
	}
	var total int64
	if err := query.Model(&McpService{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("sort_order ASC, created_at DESC").Offset(offset).Limit(limit).Find(&services).Error
	return services, total, err
}

func GetServiceByID(userID, serviceID int64) (*McpService, error) {
	var svc McpService
	err := DB.Where("id = ? AND user_id = ?", serviceID, userID).First(&svc).Error
	return &svc, err
}

func (s *McpService) Insert() error {
	return DB.Create(s).Error
}

func (s *McpService) Update() error {
	return DB.Save(s).Error
}

func (s *McpService) Delete() error {
	return DB.Delete(s).Error
}

func GetServiceByIDWithoutUser(serviceID int64) (*McpService, error) {
	var svc McpService
	err := DB.Where("id = ?", serviceID).First(&svc).Error
	return &svc, err
}

func ListServicesBySource(source string, offset, limit int) ([]McpService, int64, error) {
	var services []McpService
	query := DB.Where("source = ?", source)
	var total int64
	if err := query.Model(&McpService{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&services).Error
	return services, total, err
}
