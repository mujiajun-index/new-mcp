package model

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID           int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	Username     string         `json:"username" gorm:"uniqueIndex;size:64;not null"`
	Password     string         `json:"-" gorm:"not null"`
	DisplayName  string         `json:"display_name" gorm:"size:128"`
	Email        string         `json:"email" gorm:"size:255"`
	Role         string         `json:"role" gorm:"size:32;default:user"`
	Status       int            `json:"status" gorm:"default:1"`
	AvatarURL    string         `json:"avatar_url" gorm:"column:avatar_url;size:512"`
	Quota        int64          `json:"quota" gorm:"default:0"`
	UsedQuota    int64          `json:"used_quota" gorm:"default:0"`
	RequestCount int64          `json:"request_count" gorm:"default:0"`
	Group        string         `json:"group" gorm:"size:64;default:default"`
	Remark       string         `json:"remark" gorm:"size:255"`
	// 商业化:计费来源偏好(V1 固定 wallet_only,为 V2 订阅预留)
	BillingPreference string `json:"billing_preference" gorm:"size:16;default:wallet_only"`
	// 商业化:累计充值额度(quota),审计用
	TotalTopup int64 `json:"total_topup" gorm:"default:0"`
	RegisterIP   string         `json:"register_ip" gorm:"column:register_ip;size:64"`
	LastLoginAt  *time.Time     `json:"last_login_at" gorm:"column:last_login_at"`
	LastLoginIP  string         `json:"last_login_ip" gorm:"column:last_login_ip;size:64"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

func (User) TableName() string { return "users" }

func GetUserByUsername(username string) (*User, error) {
	var user User
	err := DB.Where("username = ?", username).First(&user).Error
	return &user, err
}

// IsEmailAlreadyTaken reports whether an account with the given email already exists.
func IsEmailAlreadyTaken(email string) bool {
	var count int64
	DB.Model(&User{}).Where("email = ?", email).Count(&count)
	return count > 0
}

// GroupsInUse returns the subset of `groups` that have at least one user bound
// to them. The group column is a reserved word in MySQL/PostgreSQL, so it is
// queried via a map condition (GORM quotes the column per dialect) rather than a
// raw SQL fragment. Used to prevent removing a user group option still in use.
func GroupsInUse(groups []string) ([]string, error) {
	var inUse []string
	for _, g := range groups {
		if g == "" {
			continue
		}
		var count int64
		if err := DB.Model(&User{}).Where(map[string]interface{}{"group": g}).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			inUse = append(inUse, g)
		}
	}
	return inUse, nil
}

// GetUserByUsernameOrEmail looks up a user by username OR email, so users can
// sign in with either identifier.
func GetUserByUsernameOrEmail(identifier string) (*User, error) {
	var user User
	err := DB.Where("username = ? OR email = ?", identifier, identifier).First(&user).Error
	return &user, err
}

func GetUserByID(id int64) (*User, error) {
	var user User
	err := DB.First(&user, id).Error
	return &user, err
}

func ListUsersWithPaged(offset, limit int, keyword string, excludeID int64) ([]User, int64, error) {
	var users []User
	var total int64
	query := DB.Model(&User{})
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if keyword != "" {
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(limit).Order("id ASC").Find(&users).Error
	return users, total, err
}

func IncreaseUserQuota(id int64, quota int64) error {
	return DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota + ?", quota)).Error
}

func IncreaseUserRequestCount(id int64) error {
	return DB.Model(&User{}).Where("id = ?", id).Update("request_count", gorm.Expr("request_count + ?", 1)).Error
}

func DecreaseUserQuota(id int64, quota int64) error {
	return DB.Model(&User{}).Where("id = ? AND quota >= ?", id, quota).Update("quota", gorm.Expr("quota - ?", quota)).Error
}

// DecreaseUserQuotaAtomic 原子扣减用户额度,仅当余额 >= quota 时成功。
// 返回受影响行数:0 表示余额不足(未扣)。quota <= 0 视为无需扣减,直接成功。
// "quota" 非保留字,可直接用列条件。
func DecreaseUserQuotaAtomic(id, quota int64) (int64, error) {
	if quota <= 0 {
		return 1, nil
	}
	res := DB.Model(&User{}).Where("id = ? AND quota >= ?", id, quota).
		Update("quota", gorm.Expr("quota - ?", quota))
	return res.RowsAffected, res.Error
}

// DecreaseUserQuotaUnguarded 无守卫扣减:用于信任旁路事后补扣(接受有界超支)。
func DecreaseUserQuotaUnguarded(id, quota int64) error {
	if quota <= 0 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ?", id).Update("quota", gorm.Expr("quota - ?", quota)).Error
}

// SetUserQuota 覆盖设置用户额度(管理员 mode=set)。
func SetUserQuota(id, quota int64) error {
	return DB.Model(&User{}).Where("id = ?", id).Update("quota", quota).Error
}

// AdjustUserUsedQuota 调整用户累计已用额度:成功消费传正、退款传负(净额反映真实消耗)。
func AdjustUserUsedQuota(id, delta int64) error {
	if delta == 0 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error
}

// IncreaseTotalTopup 累加用户累计充值额度(兑换/充值入账审计)。
func IncreaseTotalTopup(id, quota int64) error {
	if quota <= 0 {
		return nil
	}
	return DB.Model(&User{}).Where("id = ?", id).Update("total_topup", gorm.Expr("total_topup + ?", quota)).Error
}

// GetUserQuota 返回用户当前可用额度(select 仅取 quota 列,轻量)。
func GetUserQuota(id int64) (int64, error) {
	var u User
	err := DB.Select("quota").First(&u, id).Error
	return u.Quota, err
}

func (u *User) Insert() error {
	return DB.Create(u).Error
}

func (u *User) Update() error {
	return DB.Save(u).Error
}
