package virtual

import (
	"encoding/json"
	"fmt"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

type Binding = model.VirtualBinding

func ParseBinding(raw string) (Binding, error) {
	var b Binding
	err := json.Unmarshal([]byte(raw), &b)
	if err != nil || b.RefID <= 0 {
		return b, fmt.Errorf("invalid virtual service binding")
	}
	switch b.VirtualType {
	case "vision", "systemone", "imagegen", "videogen", "audiogen", "camera":
	default:
		return b, fmt.Errorf("unsupported virtual service type")
	}
	return b, nil
}

// ResolveBinding reads ownership and enablement from the actual configuration,
// rather than trusting IDs or owners supplied in a service's JSON.
func ResolveBinding(b Binding, owner int64) (*model.McpService, error) {
	return ResolveBindingDB(model.DB, b, owner)
}
func ResolveBindingDB(db *gorm.DB, b Binding, owner int64) (*model.McpService, error) {
	table := model.VirtualConfigTable(b.VirtualType)
	if table == "" {
		return nil, fmt.Errorf("unsupported virtual service type")
	}
	var row struct {
		UserID              int64
		RegisteredServiceID *int64
		AutoRegister        bool
	}
	q := db.Table(table).Where("id = ? AND user_id = ? AND deleted_at IS NULL", b.RefID, owner)
	if err := q.First(&row).Error; err != nil {
		return nil, fmt.Errorf("bound configuration not found")
	}
	if !row.AutoRegister || row.RegisteredServiceID == nil {
		return nil, fmt.Errorf("bound configuration is disabled")
	}
	svc := &model.McpService{}
	err := db.Where("id = ? AND user_id = ?", *row.RegisteredServiceID, owner).First(svc).Error
	if err != nil || svc.Status != common.StatusEnabled {
		return nil, fmt.Errorf("bound service is disabled")
	}
	if b.VirtualType != "systemone" {
		var count int64
		if err := q.Where("status = ?", common.StatusEnabled).Count(&count).Error; err != nil || count != 1 {
			return nil, fmt.Errorf("bound configuration is disabled")
		}
	}
	return svc, nil
}
