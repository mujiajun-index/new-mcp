package model

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

type ApiKey struct {
	ID             int64          `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID         int64          `json:"user_id" gorm:"not null;index"`
	Name           string         `json:"name" gorm:"size:128;not null"`
	Key            string         `json:"-" gorm:"size:255;not null;default:''"`
	KeyHash        string         `json:"-" gorm:"column:key_hash;size:255;not null;uniqueIndex"`
	KeyPrefix      string         `json:"key_prefix" gorm:"column:key_prefix;size:16;not null;index"`
	Permissions    string         `json:"permissions" gorm:"type:varchar(4096);default:'{}'"`
	Status         int            `json:"status" gorm:"default:1"`
	Quota          int64          `json:"quota" gorm:"default:0"`
	UsedQuota      int64          `json:"used_quota" gorm:"default:0"`
	UnlimitedQuota bool           `json:"unlimited_quota" gorm:"default:true"`
	AllowIPs       string         `json:"allow_ips" gorm:"size:512;default:''"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	LastUsedAt     *time.Time     `json:"last_used_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

func (k *ApiKey) GetFullKey() string {
	return k.Key
}

func (ApiKey) TableName() string { return "api_keys" }

// GetApiKeyByHash 通过 key 的 hash 反查启用的 API Key（仅校验 key 自身状态）。
// 注意：这里不校验所属用户的状态——用户禁用判断放在 middleware.APIKeyAuth 中，
// 以便对被禁用用户返回明确的 403 提示。
func GetApiKeyByHash(keyHash string) (*ApiKey, error) {
	var key ApiKey
	err := DB.Where("key_hash = ? AND status = ?", keyHash, common.StatusEnabled).First(&key).Error
	return &key, err
}

func GetApiKeyByName(userID int64, name string) (*ApiKey, error) {
	var key ApiKey
	err := DB.Where("user_id = ? AND name = ?", userID, name).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func GetApiKeyByID(id int64) (*ApiKey, error) {
	var key ApiKey
	err := DB.First(&key, id).Error
	return &key, err
}

func ListApiKeysByUser(userID int64, keyword string) ([]ApiKey, error) {
	q := DB.Where("user_id = ?", userID)
	if keyword != "" {
		q = q.Where("name LIKE ? OR key_prefix LIKE ? OR `key` LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	var keys []ApiKey
	err := q.Order("created_at DESC").Find(&keys).Error
	return keys, err
}

func BatchDeleteApiKeys(userID int64, ids []int64) error {
	return DB.Where("user_id = ? AND id IN ?", userID, ids).Delete(&ApiKey{}).Error
}

func BatchUpdateApiKeyStatus(userID int64, ids []int64, status int) error {
	return DB.Model(&ApiKey{}).Where("user_id = ? AND id IN ?", userID, ids).Update("status", status).Error
}

// DecreaseApiKeyQuotaAtomic 原子占用**非无限** Key 的消费预算:仅当 quota-used_quota >= need 时
// used_quota += need。返回受影响行数:0 表示预算不足或该 Key 为无限额度(无限 Key 由调用方另行处理)。
// 仅对市场来源服务消费生效。
func DecreaseApiKeyQuotaAtomic(apiKeyID, need int64) (int64, error) {
	if need <= 0 {
		return 1, nil
	}
	res := DB.Model(&ApiKey{}).
		Where("id = ? AND unlimited_quota = ? AND quota - used_quota >= ?", apiKeyID, false, need).
		Update("used_quota", gorm.Expr("used_quota + ?", need))
	return res.RowsAffected, res.Error
}

// AdjustApiKeyUsedQuota 调整 Key 已用额度:成功消费传正、退款传负(净额反映真实消耗,Key 预算随之恢复)。
// 供**无限额度** Key 记账与退款回退使用;预算 Key 的占用由 DecreaseApiKeyQuotaAtomic 原子完成。
func AdjustApiKeyUsedQuota(apiKeyID, delta int64) error {
	if delta == 0 {
		return nil
	}
	return DB.Model(&ApiKey{}).Where("id = ?", apiKeyID).Update("used_quota", gorm.Expr("used_quota + ?", delta)).Error
}

func (k *ApiKey) Insert() error {
	return DB.Create(k).Error
}

func (k *ApiKey) Update() error {
	return DB.Save(k).Error
}

func (k *ApiKey) Delete() error {
	return DB.Delete(k).Error
}

// GetApiKeysReferencingGroup 返回该用户下 permissions 显式引用了 groupName 的 API Key。
// 注意：通配符 "*" 表示"所有分组"，不针对具体分组，因此不计入。
// 不区分启用/禁用状态——只要引用存在就返回，避免删除分组后产生悬空引用。
func GetApiKeysReferencingGroup(userID int64, groupName string) ([]ApiKey, error) {
	var keys []ApiKey
	if err := DB.Where("user_id = ?", userID).Find(&keys).Error; err != nil {
		return nil, err
	}
	matched := make([]ApiKey, 0)
	for i := range keys {
		var perms struct {
			Groups []string `json:"groups"`
		}
		if err := json.Unmarshal([]byte(keys[i].Permissions), &perms); err != nil {
			continue
		}
		if slices.Contains(perms.Groups, groupName) {
			matched = append(matched, keys[i])
		}
	}
	return matched, nil
}
