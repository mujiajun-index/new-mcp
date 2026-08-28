package model

import (
	"time"
)

type McpService struct {
	ID                int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID            int64      `json:"user_id" gorm:"not null;uniqueIndex:idx_svc_user_name"`
	Name              string     `json:"name" gorm:"size:128;not null;uniqueIndex:idx_svc_user_name"`
	DisplayName       string     `json:"display_name" gorm:"size:255"`
	Description       string     `json:"description" gorm:"type:text"`
	TransportType     string     `json:"transport_type" gorm:"size:32;not null;index"`
	Config            string     `json:"config" gorm:"type:varchar(4096);default:'{}'"`
	PassiveToken      string     `json:"-" gorm:"column:passive_token;size:512"`
	PassiveConnected  bool       `json:"passive_connected" gorm:"default:false"`
	AuthType          string     `json:"auth_type" gorm:"size:32;default:none"`
	AuthConfig        string     `json:"auth_config" gorm:"type:varchar(4096);default:'{}'"`
	ToolsCache        string     `json:"tools_cache" gorm:"type:text"`
	ToolsUpdatedAt    *time.Time `json:"tools_updated_at"`
	ResourcesCache    string     `json:"resources_cache" gorm:"type:text"`
	PromptsCache      string     `json:"prompts_cache" gorm:"type:text"`
	HealthStatus      string     `json:"health_status" gorm:"size:16;default:unknown;index"`
	LastHealthCheck   *time.Time `json:"last_health_check"`
	ServerInfo        string     `json:"server_info" gorm:"type:text"`
	ProtocolVersion   string     `json:"protocol_version" gorm:"size:32"`
	IconURL           string     `json:"icon_url" gorm:"size:512"`
	Tags              string     `json:"tags" gorm:"size:512"`
	Visibility        string     `json:"visibility" gorm:"size:16;default:private;index"`
	Source            string     `json:"source" gorm:"size:16;default:user"`
	// 平台健康按日志表 marketplace_item_id 聚合,不枚举本表引用行;此索引供
	// 按 item 定位引用行/回填 item 归属的点查(如按条目踢会话、启停市场服务)
	MarketplaceItemID *int64     `json:"marketplace_item_id" gorm:"index"`
	SortOrder         int        `json:"sort_order" gorm:"default:0"`
	Status            int        `json:"status" gorm:"default:1"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
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

// ListAllServicesByUser 返回用户全部服务(不分页),供总览页一次拉全量、前端本地筛选。
func ListAllServicesByUser(userID int64) ([]McpService, error) {
	var services []McpService
	err := DB.Where("user_id = ?", userID).
		Order("sort_order ASC, created_at DESC").Find(&services).Error
	return services, err
}

// GetServiceMarketplaceItemID 点查服务行的市场条目归属。resources/read、
// prompts/get、mcp.read 等不走计费路径的调用日志用它回填 marketplace_item_id
// (tools/call 由 applyBillingToLog 写入),市场条目健康/日志筛选口径才完整;
// 非市场服务返回 nil。
func GetServiceMarketplaceItemID(serviceID int64) *int64 {
	var svc struct {
		MarketplaceItemID *int64
	}
	if err := DB.Model(&McpService{}).
		Where("id = ?", serviceID).
		Select("marketplace_item_id").
		Take(&svc).Error; err != nil {
		return nil
	}
	return svc.MarketplaceItemID
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

// UpdateHealthStatusIfChanged 幂等回写健康状态:已是目标值则不写(常态下零
// RowsAffected),返回是否实际发生变更(调用方据此决定是否记系统日志)。
func UpdateHealthStatusIfChanged(serviceID int64, status string) bool {
	res := DB.Model(&McpService{}).
		Where("id = ? AND health_status <> ?", serviceID, status).
		Update("health_status", status)
	return res.Error == nil && res.RowsAffected > 0
}

func GetServiceByIDWithoutUser(serviceID int64) (*McpService, error) {
	var svc McpService
	err := DB.Where("id = ?", serviceID).First(&svc).Error
	return &svc, err
}

// GetServicesByIDs returns the services with the given IDs in a single query.
func GetServicesByIDs(ids []int64) ([]McpService, error) {
	var services []McpService
	err := DB.Where("id IN ?", ids).Find(&services).Error
	return services, err
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

// ListClonableServices 返回指定管理员(userID)可用于"从自有服务克隆上架"的来源服务:该管理员账户下的真实
// 自有服务(source=user/admin),自动排除 marketplace 引用与 vision/camera 虚拟服务。管理员只能上架自己
// 配置的服务,不触碰其他用户的服务(§11)。
func ListClonableServices(userID int64, offset, limit int) ([]McpService, int64, error) {
	var services []McpService
	query := DB.Where("user_id = ? AND source IN ?", userID, []string{"user"})
	var total int64
	if err := query.Model(&McpService{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&services).Error
	return services, total, err
}

// GetMarketplaceReferenceByUser 返回用户对某市场项已建立的引用(source=marketplace);不存在返回 nil。
// 用于引用式安装去重(§11):同一用户对同一市场项仅一份引用。
func GetMarketplaceReferenceByUser(userID, itemID int64) (*McpService, error) {
	var svc McpService
	err := DB.Where("user_id = ? AND source = ? AND marketplace_item_id = ?", userID, "marketplace", itemID).
		First(&svc).Error
	if err != nil {
		return nil, err
	}
	return &svc, nil
}

// ServiceNameExists 报告该用户下是否已存在同名服务。
// mcp_services 上 (user_id,name) 为唯一索引(idx_svc_user_name),引用式安装需据此避开命名冲突。
func ServiceNameExists(userID int64, name string) (bool, error) {
	var count int64
	err := DB.Model(&McpService{}).Where("user_id = ? AND name = ?", userID, name).Count(&count).Error
	return count > 0, err
}
