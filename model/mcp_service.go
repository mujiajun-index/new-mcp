package model

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
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
	// SharedProcess 内存态标记(gorm:"-"):市场共享 stdio 条目(isolated_process=false)
	// 物化时置 true,会话池据此把会话键从服务行 ID 换成条目 ID——全部安装用户共用
	// 同一个平台子进程。不落库;引用行落库值恒为 false,真实配置在 marketplace_items。
	SharedProcess bool       `json:"-" gorm:"-"`
	SortOrder     int        `json:"sort_order" gorm:"default:0"`
	Status            int        `json:"status" gorm:"default:1"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (McpService) TableName() string { return "mcp_services" }

// AuthKeyConfig 是 AuthConfig 列中与多秘钥相关的配置段(该列还承载展示性字段
// key/token 等,解析时只取这两个字段,写回由 service 层整体保留其余键)。
type AuthKeyConfig struct {
	KeyMode    string `json:"key_mode,omitempty"`    // ""=单秘钥;random | polling
	HeaderName string `json:"header_name,omitempty"` // 多秘钥注入的目标头名
}

// ParseAuthKeyConfig 解析 AuthConfig 里的多秘钥配置;空/坏 JSON 返回零值。
func (s *McpService) ParseAuthKeyConfig() AuthKeyConfig {
	var c AuthKeyConfig
	if s.AuthConfig == "" {
		return c
	}
	_ = json.Unmarshal([]byte(s.AuthConfig), &c)
	return c
}

// IsMultiKey 报告服务是否处于多秘钥模式(仅 HTTP 类传输由调用方保证)。
func (s *McpService) IsMultiKey() bool {
	m := s.ParseAuthKeyConfig().KeyMode
	return m == common.KeyModeRandom || m == common.KeyModePolling
}

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

// ServiceRowRef 市场条目安装引用行的窄视图(条目进程枚举用):不取 config 等大列。
type ServiceRowRef struct {
	ID          int64
	UserID      int64
	Name        string
	DisplayName string
	Status      int
}

// QueryServiceRowsByMarketplaceItem 某市场条目安装引用行的分页查询(独占进程模式
// 逐行枚举用):keyword 匹配用户名(users 子查询)或服务名/显示名,返回当前页窄行
// 与筛选后总数。分页 + 用户名只对当前页(≤page_size 个 ID)反查,万级安装时不
// 一次性带回全部行,也不构造全量用户 IN 列表(SQLite 32766 参数悬崖)。
func QueryServiceRowsByMarketplaceItem(itemID int64, keyword string, offset, limit int) ([]ServiceRowRef, int64, error) {
	q := DB.Model(&McpService{}).
		Where("marketplace_item_id = ? AND source = ?", itemID, "marketplace")
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("(user_id IN (SELECT id FROM users WHERE username LIKE ?) OR name LIKE ? OR display_name LIKE ?)", like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []ServiceRowRef
	err := q.Select("id, user_id, name, display_name, status").
		Order("id ASC").Offset(offset).Limit(limit).Find(&rows).Error
	return rows, total, err
}

// UpdateMarketplaceRefsByItems 把市场条目的展示/快照字段变更批量同步到全部
// 安装引用行(source=marketplace 且 marketplace_item_id 命中所列条目)。
// updates 为列名→新值 map(display_name/description/icon_url/tags/tools_cache/
// tools_updated_at/resources_cache/prompts_cache/server_info/protocol_version),
// 与 GORM Updates(map) 同语义,零值字符串照写(支持清空)。
// 须在事务内调用,与条目本体更新保持原子;空 ids/空 updates 直接返回(幂等)。
// 注意:管理端同步会覆盖用户对引用行 DisplayName/Description/Tags 的自定义编辑
// (产品明确要求:管理员修改市场服务信息须同步所有已安装用户)。
// 不同步项:name(引用行可能带 -2 冲突后缀)、transport_type(哨兵值 marketplace,
// 真实 transport 调用时由 materializeMarketplace 注入)、config(平台托管不下发)、
// status(用户自管,下架门控在调用侧按条目状态判断)。
func UpdateMarketplaceRefsByItems(tx *gorm.DB, itemIDs []int64, updates map[string]interface{}) error {
	if len(itemIDs) == 0 || len(updates) == 0 {
		return nil
	}
	return tx.Model(&McpService{}).
		Where("marketplace_item_id IN ? AND source = ?", itemIDs, "marketplace").
		Updates(updates).Error
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
