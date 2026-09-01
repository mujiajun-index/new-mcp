package model

import (
	"errors"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
)

// McpServiceKey 是 MCP 服务多秘钥池(仅 HTTP 类传输)中的一把秘钥。
// SortOrder 从 1 起,= 轮询次序 = mcp_call_logs.key_index(0 保留给「未使用多秘钥」)。
// 状态挂在行上而非索引:删 key 后序号重排不会把状态串到别的 key。
type McpServiceKey struct {
	ID             int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	ServiceID      int64      `json:"service_id" gorm:"not null;uniqueIndex:idx_svc_key_pos"`
	SortOrder      int        `json:"sort_order" gorm:"not null;uniqueIndex:idx_svc_key_pos"`
	Value          string     `json:"value" gorm:"type:text"`
	Status         int        `json:"status" gorm:"not null"`
	DisabledReason string     `json:"disabled_reason" gorm:"size:255"`
	DisabledAt     *time.Time `json:"disabled_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (McpServiceKey) TableName() string { return "mcp_service_keys" }

// MaxServiceKeys 单服务秘钥池上限,防滥用。
const MaxServiceKeys = 100

// MaxServiceKeyBytes 单把秘钥长度上限。
const MaxServiceKeyBytes = 8 * 1024

// ListKeysByService 返回池内全部秘钥,按 SortOrder ASC(轮询次序)。
func ListKeysByService(serviceID int64) ([]McpServiceKey, error) {
	var keys []McpServiceKey
	err := DB.Where("service_id = ?", serviceID).Order("sort_order ASC").Find(&keys).Error
	return keys, err
}

// AppendKeys 追加秘钥:对池内已有值去重,SortOrder 接在当前最大值之后;
// 既有行与状态一律不动(追加语义)。返回实际新增的行数。
func AppendKeys(serviceID int64, values []string) (int64, error) {
	clean, err := CleanKeyValues(values)
	if err != nil {
		return 0, err
	}
	if len(clean) == 0 {
		return 0, nil
	}
	var existing []McpServiceKey
	if err := DB.Where("service_id = ?", serviceID).Find(&existing).Error; err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(existing))
	for _, k := range existing {
		seen[k.Value] = true
	}
	now := time.Now()
	rows := make([]McpServiceKey, 0, len(clean))
	next := len(existing) + 1
	for _, v := range clean {
		if seen[v] {
			continue
		}
		seen[v] = true
		rows = append(rows, McpServiceKey{
			ServiceID: serviceID,
			SortOrder: next,
			Value:     v,
			Status:    common.StatusEnabled,
			CreatedAt: now,
			UpdatedAt: now,
		})
		next++
	}
	if len(rows) == 0 {
		return 0, nil
	}
	if len(existing)+len(rows) > MaxServiceKeys {
		return 0, errors.New("秘钥数量超出上限(100)")
	}
	if err := DB.Create(&rows).Error; err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}

// ReplaceKeys 替换全部秘钥:事务内删全量、按入参顺序从 1 重排插入,状态清零。
func ReplaceKeys(serviceID int64, values []string) error {
	clean, err := CleanKeyValues(values)
	if err != nil {
		return err
	}
	if len(clean) == 0 || len(clean) > MaxServiceKeys {
		return errors.New("秘钥列表为空或超出上限(100)")
	}
	now := time.Now()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("service_id = ?", serviceID).Delete(&McpServiceKey{}).Error; err != nil {
			return err
		}
		rows := make([]McpServiceKey, len(clean))
		for i, v := range clean {
			rows[i] = McpServiceKey{
				ServiceID: serviceID,
				SortOrder: i + 1,
				Value:     v,
				Status:    common.StatusEnabled,
				CreatedAt: now,
				UpdatedAt: now,
			}
		}
		return tx.Create(&rows).Error
	})
}

// SetKeyStatus 更新单把秘钥状态;启用时清空禁用原因/时间。
// 校验行属于指定服务,返回是否命中。
func SetKeyStatus(serviceID, keyID int64, status int, reason string) (bool, error) {
	updates := map[string]interface{}{"status": status, "updated_at": time.Now()}
	if status == common.StatusEnabled {
		updates["disabled_reason"] = ""
		updates["disabled_at"] = nil
	} else {
		updates["disabled_reason"] = reason
		updates["disabled_at"] = time.Now()
	}
	res := DB.Model(&McpServiceKey{}).
		Where("id = ? AND service_id = ?", keyID, serviceID).
		Updates(updates)
	return res.Error == nil && res.RowsAffected > 0, res.Error
}

// DeleteServiceKey 删除单把秘钥(校验归属),返回是否命中。
func DeleteServiceKey(serviceID, keyID int64) (bool, error) {
	res := DB.Where("id = ? AND service_id = ?", keyID, serviceID).Delete(&McpServiceKey{})
	return res.Error == nil && res.RowsAffected > 0, res.Error
}

// BatchEnableAllKeys 全部启用(清禁用原因/时间)。
func BatchEnableAllKeys(serviceID int64) (int64, error) {
	res := DB.Model(&McpServiceKey{}).Where("service_id = ?", serviceID).Updates(map[string]interface{}{
		"status":          common.StatusEnabled,
		"disabled_reason": "",
		"disabled_at":     nil,
		"updated_at":      time.Now(),
	})
	return res.RowsAffected, res.Error
}

// DeleteDisabledKeys 删除所有已禁用(手动+自动)的秘钥。
func DeleteDisabledKeys(serviceID int64) (int64, error) {
	res := DB.Where("service_id = ? AND status <> ?", serviceID, common.StatusEnabled).Delete(&McpServiceKey{})
	return res.RowsAffected, res.Error
}

// DeleteKeysByService 删服务时一并清池。
func DeleteKeysByService(serviceID int64) error {
	return DB.Where("service_id = ?", serviceID).Delete(&McpServiceKey{}).Error
}

// CountServiceKeys 点查池内秘钥总数与启用数(详情徽章用)。
func CountServiceKeys(serviceID int64) (total, enabled int64) {
	DB.Model(&McpServiceKey{}).Where("service_id = ?", serviceID).Count(&total)
	DB.Model(&McpServiceKey{}).Where("service_id = ? AND status = ?", serviceID, common.StatusEnabled).Count(&enabled)
	return total, enabled
}

// CleanKeyValues 规整入参秘钥列表:去首尾空白、丢空行、批内去重、超长报错。
func CleanKeyValues(values []string) ([]string, error) {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		if len(v) > MaxServiceKeyBytes {
			return nil, errors.New("单把秘钥长度超出上限(8KB)")
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}
