package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"gorm.io/gorm"
)

// VirtualExecution is constructed only after the service and marketplace binding
// have been authorized. Configuration ownership never becomes task ownership.
type VirtualExecution struct {
	UserID, OwnerID, ConfigID, ServiceID int64
	Kind                                 string
	MarketplaceItemID                    *int64
}
type virtualExecutionKey struct{}

func WithVirtualExecution(ctx context.Context, s VirtualExecution) context.Context {
	return context.WithValue(ctx, virtualExecutionKey{}, s)
}
func VirtualExecutionFrom(ctx context.Context) VirtualExecution {
	s, _ := ctx.Value(virtualExecutionKey{}).(VirtualExecution)
	return s
}
func CanExecuteConfig(ctx context.Context, uid, owner, configID int64, kind string) bool {
	if uid == 0 {
		return false
	}
	if uid == owner {
		return true
	}
	s := VirtualExecutionFrom(ctx)
	return s.MarketplaceItemID != nil && s.UserID == uid && s.OwnerID == owner && s.ConfigID == configID && s.Kind == kind
}
func ScopedTaskKey(ctx context.Context, key string) string {
	s := VirtualExecutionFrom(ctx)
	if s.ServiceID == 0 {
		return key
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", s.ServiceID, key)))
	return hex.EncodeToString(sum[:])
}

// ScopeVirtualTasks also exposes legacy self-owned history, never platform history.
func ScopeVirtualTasks(q *gorm.DB, s VirtualExecution) *gorm.DB {
	q = q.Where("user_id = ?", s.UserID)
	if s.MarketplaceItemID != nil {
		return q.Where("service_id = ?", s.ServiceID)
	}
	return q.Where("service_id = ? OR ((service_id IS NULL OR service_id = 0) AND config_id = ?)", s.ServiceID, s.ConfigID)
}
func CheckVirtualTask(ctx context.Context, table, id string) error {
	s := VirtualExecutionFrom(ctx)
	if s.ServiceID == 0 {
		return nil
	}
	var count int64
	if err := ScopeVirtualTasks(DB.Table(table), s).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("task not found or not accessible to this service")
	}
	return nil
}
