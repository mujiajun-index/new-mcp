package model

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ErrInsufficientAffQuota 邀请奖励待提取余额不足(转账时)。
var ErrInsufficientAffQuota = errors.New("邀请额度不足")

// affCodeAlphabet 邀请码字符集(字母+数字),与 reference/new-api 的 AlphanumericCharset 对齐。
const affCodeAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

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
	// 邀请(完全对齐 reference/new-api):每用户一个邀请码;注册时绑定邀请人;
	// 邀请者奖励进入 AffQuota(待提取),需手动转入钱包;受邀者奖励直接进 Quota。
	AffCode         string `json:"aff_code" gorm:"size:32;uniqueIndex;column:aff_code"`
	InviterID       int64  `json:"inviter_id" gorm:"index;column:inviter_id;default:0"`
	AffCount        int    `json:"aff_count" gorm:"default:0;column:aff_count"`             // 已邀请人数
	AffQuota        int64  `json:"aff_quota" gorm:"default:0;column:aff_quota"`             // 邀请奖励待提取余额
	AffHistoryQuota int64  `json:"aff_history_quota" gorm:"default:0;column:aff_history"`   // 邀请奖励累计(列名 aff_history 与 new-api 一致)
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

// GetUsersByIDs 一次查询取回多个用户(按 ID)。返回 id→User 映射,供列表批量解析用户名等,避免 N+1。
func GetUsersByIDs(ids []int64) (map[int64]User, error) {
	result := make(map[int64]User, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var users []User
	if err := DB.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		result[u.ID] = u
	}
	return result, nil
}

// ListUsersWithPaged 分页列出用户。deletedOnly=true 时改用 Unscoped 只列软删除行
// (管理员"已删除"筛选视图,供查找与恢复);此时 status 过滤无意义,忽略。
func ListUsersWithPaged(offset, limit int, keyword string, excludeID int64, role string, status int, deletedOnly bool) ([]User, int64, error) {
	var users []User
	var total int64
	query := DB.Model(&User{})
	if deletedOnly {
		query = DB.Unscoped().Model(&User{}).Where("deleted_at IS NOT NULL")
	}
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if keyword != "" {
		query = query.Where("username LIKE ? OR email LIKE ? OR display_name LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	if role != "" {
		query = query.Where("role = ?", role)
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Offset(offset).Limit(limit).Order("id ASC").Find(&users).Error
	return users, total, err
}

// SoftDeleteUserByID 软删除用户(管理员删除用户):仅置 deleted_at,用户名下的全部数据
// (API Key/分组/服务/额度/调用日志)原样保留,可随时 RestoreUserByID 恢复。
// 访问立即切断:登录与 API Key 认证均走默认作用域查询,软删除后一律"查无此用户"。
// 用户名在删除期间继续占用唯一索引——防止他人抢注同名账号顶替身份,恢复时也不会冲突。
func SoftDeleteUserByID(id int64) error {
	return DB.Delete(&User{}, id).Error
}

// RestoreUserByID 恢复软删除的用户(清空 deleted_at)。CAS 只命中已删除行,
// 返回 false 表示该用户不存在或本就未被删除。
func RestoreUserByID(id int64) (bool, error) {
	res := DB.Unscoped().Model(&User{}).
		Where("id = ? AND deleted_at IS NOT NULL", id).
		Updates(map[string]interface{}{"deleted_at": nil})
	return res.RowsAffected > 0, res.Error
}

// GetUserByIDUnscoped 按 ID 取用户,含软删除行(管理员恢复流程需读取被删用户做权限校验)。
func GetUserByIDUnscoped(id int64) (*User, error) {
	var user User
	err := DB.Unscoped().First(&user, id).Error
	return &user, err
}

// IsUsernameTaken 报告用户名是否已被占用,含软删除用户(删除期间用户名继续占用唯一
// 索引以防抢注)。注册预检必须算上这类行,否则漏过预检、在落库时撞唯一索引报原始错误。
func IsUsernameTaken(username string) bool {
	var count int64
	DB.Unscoped().Model(&User{}).Where("username = ?", username).Count(&count)
	return count > 0
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
	// 注册即分配邀请码(对齐 new-api 在 Insert 里 user.AffCode = GetRandomString(4))。
	if u.AffCode == "" {
		code, err := GenerateAffCode()
		if err != nil {
			return err
		}
		u.AffCode = code
	}
	return DB.Create(u).Error
}

func (u *User) Update() error {
	return DB.Save(u).Error
}

// GetUserIDByAffCode 反查邀请码对应的用户 id(对齐 new-api GetUserIdByAffCode)。
// 空串或未找到均返回 (0, nil)——调用方据此把 inviterID 视为 0(无邀请人),坏码不阻断注册。
func GetUserIDByAffCode(code string) (int64, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0, nil
	}
	var u User
	err := DB.Select("id").Where("aff_code = ?", code).First(&u).Error
	if err != nil {
		return 0, nil
	}
	return u.ID, nil
}

// GenerateAffCode 生成 4 位字母数字邀请码(对齐 new-api GetRandomString(4))。
// 带 uniqueIndex,故生成后查重,碰撞则重试(安全网,不改变"4 位码"的可观测行为)。
func GenerateAffCode() (string, error) {
	const length = 4
	const maxRetry = 5
	for attempt := 0; attempt < maxRetry; attempt++ {
		code, err := randString(length)
		if err != nil {
			return "", err
		}
		var count int64
		if err := DB.Model(&User{}).Where("aff_code = ?", code).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", errors.New("生成邀请码失败:多次碰撞")
}

// randString 用 crypto/rand 从 affCodeAlphabet 取 n 位随机串。
func randString(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(affCodeAlphabet)))
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = affCodeAlphabet[idx.Int64()]
	}
	return string(out), nil
}

// RewardInviter 原子发放邀请者奖励(对齐 new-api inviteUser):
// aff_count+1、aff_quota+reward(待提取)、aff_history+reward(累计)。
// 返回 gorm.ErrRecordNotFound 表示邀请人不存在(理论不会发生:注册时已校验存在)。
func RewardInviter(inviterID, reward int64) error {
	res := DB.Model(&User{}).Where("id = ?", inviterID).Updates(map[string]interface{}{
		"aff_count":   gorm.Expr("aff_count + ?", 1),
		"aff_quota":   gorm.Expr("aff_quota + ?", reward),
		"aff_history": gorm.Expr("aff_history + ?", reward),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// TransferAffQuota 原子两段式转账:把 aff_quota(待提取)转为可用 quota(对齐 new-api
// TransferAffQuotaToQuota)。用 CAS(WHERE aff_quota >= ?)+ RowsAffected 判定,
// 三库(SQLite/MySQL/PostgreSQL)通用——与本仓 Redemption.Redeem 同一占领范式,不用 FOR UPDATE。
// 余额不足返回 ErrInsufficientAffQuota;最小转账额由 service 层校验。
func TransferAffQuota(userID, quota int64) error {
	res := DB.Model(&User{}).Where("id = ? AND aff_quota >= ?", userID, quota).Updates(map[string]interface{}{
		"aff_quota": gorm.Expr("aff_quota - ?", quota),
		"quota":     gorm.Expr("quota + ?", quota),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInsufficientAffQuota
	}
	return nil
}
