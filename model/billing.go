package model

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// --- 兑换码错误哨兵(供 controller 层映射 HTTP 状态) ---
var (
	ErrRedemptionNotAvailable = errors.New("redemption code not available")
	ErrRedemptionUsed         = errors.New("redemption code already used")
	ErrRedemptionDisabled     = errors.New("redemption code disabled")
	ErrRedemptionExpired      = errors.New("redemption code expired")
)

// 兑换码状态(独立 scheme:与通用 StatusEnabled/Disabled 数值不同,勿混用)。
const (
	RedemptionStatusAvailable = 1 // 可用
	RedemptionStatusRedeemed  = 2 // 已兑换
	RedemptionStatusDisabled  = 3 // 已禁用
)

// McpToolPrice 市场服务条目级定价表(§4.4)。命中即生效,优先级最高(定价第 1 级)。
// Kind=tool 时条目名为工具名;resource 为上游资源 URI;prompt 为上游提示名。
type McpToolPrice struct {
	ID                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	MarketplaceItemID int64  `json:"marketplace_item_id" gorm:"not null;uniqueIndex:idx_item_kind_name"`
	Kind              string `json:"kind" gorm:"size:16;not null;default:tool;uniqueIndex:idx_item_kind_name"` // tool / resource / prompt
	// 条目名(工具名/资源上游 URI/提示名)。资源 URI 可能超 255,加宽到 512
	// (utf8mb4 下复合唯一索引 2120B < InnoDB 3072B 上限;TEXT 建不了唯一索引)。
	ToolName     string  `json:"tool_name" gorm:"size:512;not null;uniqueIndex:idx_item_kind_name"`
	BillingType  string  `json:"billing_type" gorm:"size:16;default:per_call"` // free / per_call / inherit(条目显式继承服务价,价格恒 0)
	PricePerCall float64 `json:"price_per_call" gorm:"type:decimal(10,4);default:0"`
	// 无 default 标签(GORM 对带 default 的 bool 零值 INSERT 时省略该列);
	// 写入点(SetItemEntryPrices)显式赋 true。
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (McpToolPrice) TableName() string { return "mcp_tool_prices" }

// ListToolPricesByItem 返回某市场项的全部条目级定价(三种 kind)。
func ListToolPricesByItem(itemID int64) ([]McpToolPrice, error) {
	var prices []McpToolPrice
	err := DB.Where("marketplace_item_id = ? AND enabled = ?", itemID, true).Find(&prices).Error
	return prices, err
}

// GetEntryPrice 取某条目的条目级定价(未启用或不存在返回 nil)。
func GetEntryPrice(itemID int64, kind, name string) (*McpToolPrice, error) {
	var p McpToolPrice
	err := DB.Where("marketplace_item_id = ? AND kind = ? AND tool_name = ? AND enabled = ?", itemID, kind, name, true).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (p *McpToolPrice) Insert() error  { return DB.Create(p).Error }
func (p *McpToolPrice) Update() error  { return DB.Save(p).Error }
func (p *McpToolPrice) Delete() error  { return DB.Delete(p).Error }

// Redemption 兑换码表(§4.8,V1)。列名用 code 而非 key(key 为 SQL 保留字)。
type Redemption struct {
	ID         int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	Code       string     `json:"code" gorm:"size:32;not null;uniqueIndex"`
	Name       string     `json:"name" gorm:"size:128;default:''"`
	Amount     float64    `json:"amount" gorm:"type:decimal(10,4);default:0;not null"` // 面值(货币单位,如 CNY 元);兑换时按 QuotaPerUnit 换算为额度
	Status     int        `json:"status" gorm:"default:1;index"` // 1=可用 2=已兑换 3=已禁用
	UserID     *int64     `json:"user_id" gorm:"index"`          // 兑换者用户 ID
	ExpiredAt  int64      `json:"expired_at" gorm:"default:0"`   // 过期时间戳,0=永不过期
	CreatedAt  time.Time  `json:"created_at"`
	RedeemedAt *time.Time `json:"redeemed_at"`
}

func (Redemption) TableName() string { return "redemptions" }

func GetRedemptionByCode(code string) (*Redemption, error) {
	var r Redemption
	err := DB.Where("code = ?", code).First(&r).Error
	return &r, err
}

func GetRedemptionByID(id int64) (*Redemption, error) {
	var r Redemption
	err := DB.First(&r, id).Error
	return &r, err
}

// ListRedemptions 分页/搜索兑换码(管理员)。
func ListRedemptions(offset, limit int, keyword string, status int) ([]Redemption, int64, error) {
	query := DB.Model(&Redemption{})
	if keyword != "" {
		query = query.Where("code LIKE ? OR name LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []Redemption
	err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func (r *Redemption) Insert() error { return DB.Create(r).Error }
func (r *Redemption) Update() error { return DB.Save(r).Error }
func (r *Redemption) Delete() error { return DB.Delete(r).Error }

// Redeem 原子核销兑换码并入账用户额度,返回入账的 quota。
//
// 并发安全靠"原子占领":仅当 status=可用 且未过期时把 status 置 2,以受影响行数
// 作为"本请求赢得竞争"的判据(SQLite/MySQL/PostgreSQL 三库通用,无需 FOR UPDATE,
// 因为状态翻转本身即串行化)。占领与入账在同一事务内,保证不丢账。参考 new-api
// Redemption.Redeem() 的幂等校验语义。
func (r *Redemption) Redeem(userID int64) (int64, error) {
	now := time.Now().Unix()
	redeemedAt := time.Now()
	// 面值(货币)按当前 QuotaPerUnit 换算为入账额度。
	quota := CurrencyToQuota(r.Amount)
	err := DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&Redemption{}).
			Where("id = ? AND status = ? AND (expired_at = 0 OR expired_at >= ?)",
				r.ID, RedemptionStatusAvailable, now).
			Updates(map[string]interface{}{
				"status":      RedemptionStatusRedeemed,
				"user_id":     userID,
				"redeemed_at": redeemedAt,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 占领失败:区分原因(已用/已禁用/已过期)
			latest, gErr := GetRedemptionByID(r.ID)
			if gErr != nil {
				return ErrRedemptionNotAvailable
			}
			switch latest.Status {
			case RedemptionStatusRedeemed:
				return ErrRedemptionUsed
			case RedemptionStatusDisabled:
				return ErrRedemptionDisabled
			default:
				return ErrRedemptionExpired
			}
		}
		// 入账(幂等增加)+ 累计充值审计
		if e := tx.Model(&User{}).Where("id = ?", userID).
			Update("quota", gorm.Expr("quota + ?", quota)).Error; e != nil {
			return e
		}
		return tx.Model(&User{}).Where("id = ?", userID).
			Update("total_topup", gorm.Expr("total_topup + ?", quota)).Error
	})
	if err != nil {
		return 0, err
	}
	return quota, nil
}
