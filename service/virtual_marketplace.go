package service

import (
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/internal/mcp/virtual/events"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func validateVirtualBindingTx(tx *gorm.DB, owner int64, b model.VirtualBinding) error {
	if b.VirtualType == "camera" {
		return ErrVirtualServiceNotListable
	}
	if err := model.LockVirtualConfig(tx, owner, b); err != nil {
		return fmt.Errorf("绑定配置不存在或不属于此管理员")
	}
	var row struct {
		AutoRegister        bool
		RegisteredServiceID *int64
		Status              int
	}
	if err := tx.Table(model.VirtualConfigTable(b.VirtualType)).Where("id = ? AND user_id = ? AND deleted_at IS NULL", b.RefID, owner).First(&row).Error; err != nil {
		return err
	}
	if !row.AutoRegister || row.RegisteredServiceID == nil {
		return fmt.Errorf("绑定配置未启用 MCP 服务")
	}
	if b.VirtualType != "systemone" && row.Status != common.StatusEnabled {
		return fmt.Errorf("绑定配置已停用")
	}
	return nil
}

// Synchronize descriptions and capabilities after a configuration edit. Public
// snapshots contain templates only; task lists remain local to each installation.
func syncVirtualMarketplaceBindings(kind string, id int64) {
	defer events.ConfigChanged(kind, id)
	if model.DB == nil || !model.DB.Migrator().HasTable(&model.MarketplaceItem{}) {
		return
	}
	var items []model.MarketplaceItem
	if model.DB.Where("transport_type = ?", "virtual").Find(&items).Error != nil {
		return
	}
	for _, item := range items {
		b := model.ItemVirtualBinding(&item)
		if b.VirtualType != kind || b.RefID != id {
			continue
		}
		source, err := virtual.ResolveBinding(b, item.AdminID)
		if err == nil {
			updates := map[string]any{"tools_snapshot": source.ToolsCache, "resources_snapshot": virtual.PublicResourceSnapshot(kind), "prompts_snapshot": "[]"}
			_ = model.DB.Transaction(func(tx *gorm.DB) error {
				result := tx.Model(&model.MarketplaceItem{}).Where("id = ? AND config_template = ?", item.ID, item.ConfigTemplate).Updates(updates)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected == 0 {
					return nil
				}
				return model.UpdateMarketplaceRefsByItems(tx, []int64{item.ID}, map[string]any{"tools_cache": source.ToolsCache, "resources_cache": virtual.PublicResourceSnapshot(kind), "prompts_cache": "[]"})
			})
		}
		if err != nil {
			events.MarketplaceChanged(item.ID)
		}
	}
}

// Old virtual-service cache rows may still contain one entry per task. Always
// serve their public template snapshot, including for marketplace references.
func virtualResourceSnapshotForService(svc *model.McpService) (string, bool) {
	if svc.TransportType == "virtual" {
		binding, err := virtual.ParseBinding(svc.Config)
		if err == nil {
			return virtual.PublicResourceSnapshot(binding.VirtualType), true
		}
	}
	if svc.Source == "marketplace" && svc.MarketplaceItemID != nil {
		item, err := model.GetMarketplaceItemByID(*svc.MarketplaceItemID)
		if err == nil && item.TransportType == "virtual" {
			binding := model.ItemVirtualBinding(item)
			return virtual.PublicResourceSnapshot(binding.VirtualType), true
		}
	}
	return "", false
}
