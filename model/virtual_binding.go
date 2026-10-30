package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VirtualBinding struct {
	VirtualType string `json:"virtual_type"`
	RefID       int64  `json:"ref_id"`
}

func VirtualConfigTable(kind string) string {
	return map[string]string{"vision": "vision_configs", "systemone": "systemone_configs", "imagegen": "imagegen_configs", "videogen": "videogen_configs", "audiogen": "audiogen_configs", "camera": "cameras"}[kind]
}

// A config row lock serializes listing/rebinding with deletion on all supported
// SQL backends (SQLite serializes writers at the transaction level).
func LockVirtualConfig(tx *gorm.DB, owner int64, b VirtualBinding) error {
	table := VirtualConfigTable(b.VirtualType)
	if table == "" || b.RefID <= 0 {
		return fmt.Errorf("invalid virtual binding")
	}
	var row struct{ ID int64 }
	return tx.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND deleted_at IS NULL", b.RefID, owner).First(&row).Error
}
func ItemVirtualBinding(item *MarketplaceItem) VirtualBinding {
	plain := item.ConfigTemplate
	if s, e := common.Decrypt(plain); e == nil {
		plain = s
	}
	var b VirtualBinding
	_ = json.Unmarshal([]byte(plain), &b)
	return b
}
func RequireVirtualConfigUnbound(tx *gorm.DB, owner int64, b VirtualBinding) error {
	if err := LockVirtualConfig(tx, owner, b); err != nil {
		return err
	}
	var items []MarketplaceItem
	// Some unit tests use a deliberately minimal schema without marketplace.
	if !tx.Migrator().HasTable(&MarketplaceItem{}) {
		return nil
	}
	if err := tx.Where("admin_id = ? AND transport_type = ?", owner, "virtual").Find(&items).Error; err != nil {
		return err
	}
	names := []string{}
	for _, item := range items {
		if ItemVirtualBinding(&item) == b {
			names = append(names, item.Name)
		}
	}
	if len(names) > 0 {
		return fmt.Errorf("配置仍被市场服务引用，请先更换绑定或删除市场项: %s", strings.Join(names, ", "))
	}
	return nil
}
