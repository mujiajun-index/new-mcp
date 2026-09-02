package model

import (
	"encoding/json"
	"strings"
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
	// 独占进程(仅 stdio 条目有意义):false=共享——全部安装用户共用平台侧一个 stdio
	// 子进程;true=独占——每个安装用户的引用行各一个进程(记忆存储等有状态服务)。
	// bool 不设 default(GORM 规范),CloneFromService 显式赋值,存量行零值 false=共享;
	// 非 stdio 条目读写两侧均忽略该字段。
	IsolatedProcess      bool           `json:"isolated_process"`
	ConfigTemplate       string         `json:"config_template" gorm:"type:varchar(4096);default:'{}'"`
	// 条目级多秘钥配置段({"key_mode","header_name","bearer"}),与 mcp_services.AuthConfig
	// 同语义;空串=单秘钥(模板 headers 存凭证)。type:text:本表 varchar 预算已贴近
	// MySQL 行宽上限,新字符串大列一律 TEXT(TEXT 不可带 default)。
	AuthConfig           string         `json:"auth_config" gorm:"type:text"`
	AuthInstructions     string         `json:"auth_instructions" gorm:"type:text"`
	RepoURL              string         `json:"repo_url" gorm:"size:1024"`
	InstallGuide         string         `json:"install_guide" gorm:"type:text"`
	ConfigTemplateSource string         `json:"config_template_source" gorm:"type:varchar(4096);default:'{}'"`
	RequiredEnv          string         `json:"required_env" gorm:"type:varchar(4096);default:'[]'"`
	InstallCount         int            `json:"install_count" gorm:"default:0"`
	RatingAvg            float64        `json:"rating_avg" gorm:"type:decimal(2,1);default:0.0"`
	RatingCount          int            `json:"rating_count" gorm:"default:0"`
	ToolsSnapshot        string         `json:"tools_snapshot" gorm:"type:text"`
	// 资源/提示快照:形态与 mcp_services 的 resources_cache({"resources":[],"templates":[]})、prompts_cache(裸数组)一致,
	// 克隆上架时从源服务拷贝,市场详情页展示、安装/同步时回填引用行
	ResourcesSnapshot    string         `json:"resources_snapshot" gorm:"type:text"`
	PromptsSnapshot      string         `json:"prompts_snapshot" gorm:"type:text"`
	// 上游握手信息:克隆上架时从源服务拷贝、手动刷新快照时从临时直连捕获。
	// server_info 为 JSON({"name":...,"version":...}),protocol_version 为协商出的协议版本。
	// text 而非 varchar(4096):本表 varchar 预算已贴近 MySQL 65535 行上限,再加大 varchar 会迁移失败
	ServerInfo      string `json:"server_info" gorm:"type:text"`
	ProtocolVersion string `json:"protocol_version" gorm:"size:32"`
	// 商业化:服务级计费类型(free / per_call)
	BillingType string `json:"billing_type" gorm:"size:16;default:per_call"`
	// 商业化:服务级按次单价(展示货币,如 CNY 元);free 时忽略
	PricePerCall float64 `json:"price_per_call" gorm:"type:decimal(10,4);default:0"`
	// 商业化:0=按次计费,1=仅订阅用户可用(V2);V1 固定 false
	SubscriptionOnly bool `json:"subscription_only" gorm:"default:false"`
	// 市场分组归属见 marketplace_item_groups 关联表(多对多);category 为 instant/source 部署形态,两概念独立
	Status               int            `json:"status" gorm:"default:1;index"`
	SortOrder            int            `json:"sort_order" gorm:"default:0"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

func (MarketplaceItem) TableName() string { return "marketplace_items" }

// ItemAuthKeyConfig 是条目 AuthConfig 里的多秘钥配置段。比服务版 AuthKeyConfig
// 多一个 Bearer 位:条目没有 AuthType,值补/剥 "Bearer " 前缀的依据显式落库。
type ItemAuthKeyConfig struct {
	KeyMode    string `json:"key_mode,omitempty"`    // ""=单秘钥;random | polling
	HeaderName string `json:"header_name,omitempty"` // 多秘钥注入的目标头名
	Bearer     bool   `json:"bearer,omitempty"`      // true=注入值加 "Bearer " 前缀
}

// ParseAuthKeyConfig 解析条目 AuthConfig 的多秘钥配置;空/坏 JSON 返回零值。
func (i *MarketplaceItem) ParseAuthKeyConfig() ItemAuthKeyConfig {
	var c ItemAuthKeyConfig
	if i.AuthConfig == "" {
		return c
	}
	_ = json.Unmarshal([]byte(i.AuthConfig), &c)
	return c
}

// IsMultiKey 报告条目是否处于多秘钥模式(仅 HTTP 类传输由调用方保证)。
func (i *MarketplaceItem) IsMultiKey() bool {
	m := i.ParseAuthKeyConfig().KeyMode
	return m == common.KeyModeRandom || m == common.KeyModePolling
}

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

func ListPublishedMarketplaceItems(offset, limit int, category, keyword string, groupID int64, tag string) ([]MarketplaceItem, int64, error) {
	query := DB.Where("status = ?", common.StatusEnabled)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if groupID > 0 {
		// 多对多绑定:子查询而非 JOIN——同一条 query 链同时喂 Count 与 Find,
		// IN 子查询无行翻倍、无 GORM Count-after-Joins 坑,三方言同构
		sub := DB.Model(&MarketplaceItemGroup{}).Select("item_id").Where("group_id = ?", groupID)
		query = query.Where("id IN (?)", sub)
	}
	if tag != "" {
		// tags 是逗号串,四段 LIKE 做精确词元匹配(整串= / 首词 / 尾词 / 中间词):
		// 只用标准 LIKE 无方言拼接函数,且避免 '%tag%' 把 "web" 误配 "webgl"
		query = query.Where("tags = ? OR tags LIKE ? OR tags LIKE ? OR tags LIKE ?",
			tag, tag+",%", "%,"+tag, "%,"+tag+",%")
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

// ListMarketplaceItemStatuses 全部条目的 id+status 窄行(管理页平台级健康聚合用,
// 不携带 tools/resources 快照等 text 大列)。
func ListMarketplaceItemStatuses() ([]MarketplaceItem, error) {
	var items []MarketplaceItem
	err := DB.Model(&MarketplaceItem{}).Select("id, status").Find(&items).Error
	return items, err
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

// MarketplaceItemNameExists 判断市场项标识(name 全局唯一索引)是否已被占用。
func MarketplaceItemNameExists(name string) (bool, error) {
	var count int64
	err := DB.Model(&MarketplaceItem{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (i *MarketplaceItem) Update() error {
	return DB.Save(i).Error
}

func (i *MarketplaceItem) Delete() error {
	return DB.Delete(i).Error
}

// IsExplicitlyPriced 判断市场项是否"已显式定价"(§5.6)。
// 显式定价 = billing_type='free'(显式免费) 或 (billing_type='per_call' 且 price_per_call>0)。
// 默认行(per_call + price=0)视为**未定价**:非自用模式下拒绝上架/启用。
func (i *MarketplaceItem) IsExplicitlyPriced() bool {
	if i.BillingType == "free" {
		return true
	}
	return i.BillingType == "per_call" && i.PricePerCall > 0
}

// MarketplacePricingUpdate 是批量定价的单条更新项(§5.5)。
type MarketplacePricingUpdate struct {
	ID           int64
	BillingType  string  // free / per_call
	PricePerCall float64 // 展示货币单价
}

// UpdateMarketplacePricing 批量更新市场项服务级定价。事务内逐条更新,任一失败回滚。
func UpdateMarketplacePricing(items []MarketplacePricingUpdate) (int64, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx := DB.Begin()
	var affected int64
	for _, it := range items {
		res := tx.Model(&MarketplaceItem{}).Where("id = ?", it.ID).
			Updates(map[string]interface{}{
				"billing_type":   it.BillingType,
				"price_per_call": it.PricePerCall,
			})
		if res.Error != nil {
			tx.Rollback()
			return affected, res.Error
		}
		affected += res.RowsAffected
	}
	return affected, tx.Commit().Error
}

// GetMarketplaceItemsByIDs 一次查询取回多个市场项。
func GetMarketplaceItemsByIDs(ids []int64) ([]MarketplaceItem, error) {
	var items []MarketplaceItem
	if len(ids) == 0 {
		return items, nil
	}
	err := DB.Where("id IN ?", ids).Find(&items).Error
	return items, err
}

// --- 市场项 tags 串同步(标签字典改名/禁用/删除时维护引用) ---
// 市场项 Tags 存的是逗号拼接的标签名:LIKE 仅作候选预筛(通配符只会放宽不会漏),
// Go 侧按逗号整词精确匹配,避免"AI"误伤"AI助手"。

func marketplaceItemsContainingTag(tx *gorm.DB, name string) ([]MarketplaceItem, error) {
	var items []MarketplaceItem
	err := tx.Where("tags LIKE ?", "%"+name+"%").Find(&items).Error
	return items, err
}

// ReplaceMarketplaceTagName 标签字典改名后同步市场项 tags(整词替换 old→new;
// 同行去重,避免项上原有新名时替换后出现 "new,new")。
func ReplaceMarketplaceTagName(tx *gorm.DB, oldName, newName string) error {
	items, err := marketplaceItemsContainingTag(tx, oldName)
	if err != nil {
		return err
	}
	for _, it := range items {
		parts := strings.Split(it.Tags, ",")
		kept := make([]string, 0, len(parts))
		seen := make(map[string]bool, len(parts))
		replaced := false
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == oldName {
				p = newName
				replaced = true
			}
			if p != "" && !seen[p] {
				seen[p] = true
				kept = append(kept, p)
			}
		}
		if !replaced {
			continue
		}
		if err := tx.Model(&MarketplaceItem{}).Where("id = ?", it.ID).
			Update("tags", strings.Join(kept, ",")).Error; err != nil {
			return err
		}
	}
	return nil
}

// RemoveMarketplaceTagName 标签禁用/删除后从市场项 tags 摘除该词,维持
// "市场项 tags ⊆ 启用标签字典"不变量(否则存量引用项再次保存会因校验失败)。
func RemoveMarketplaceTagName(tx *gorm.DB, name string) error {
	items, err := marketplaceItemsContainingTag(tx, name)
	if err != nil {
		return err
	}
	for _, it := range items {
		parts := strings.Split(it.Tags, ",")
		kept := make([]string, 0, len(parts))
		removed := false
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == name {
				removed = true
				continue
			}
			if p != "" {
				kept = append(kept, p)
			}
		}
		if !removed {
			continue
		}
		if err := tx.Model(&MarketplaceItem{}).Where("id = ?", it.ID).
			Update("tags", strings.Join(kept, ",")).Error; err != nil {
			return err
		}
	}
	return nil
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
