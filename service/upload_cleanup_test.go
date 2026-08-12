package service

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// TestCleanupSweepReapsExpired verifies the retention sweep reaps an expired
// upload (row + blob) while keeping a recent one — and that lowering retention
// via the option takes effect on the next sweep (the dynamic created_at model).
func TestCleanupSweepReapsExpired(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.UploadedImage{}, &model.Option{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	store, err := storage.New(context.Background(), storage.Config{Backend: "local", LocalRoot: t.TempDir(), PathPrefix: "vision"})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	UploadStore = store
	defer func() { UploadStore = nil }()

	// Retention = 1h: an 48h-old row is expired, a fresh one is not.
	if err := model.UpdateOption("UploadRetentionHours", "1"); err != nil {
		t.Fatalf("set retention: %v", err)
	}

	ctx := context.Background()
	oldKey := storage.ContentKey("abababab")
	newKey := storage.ContentKey("cdcdcdcd")
	payload := []byte("x")
	if err := UploadStore.Put(ctx, oldKey, bytesReader(payload), "image/png"); err != nil {
		t.Fatalf("put old: %v", err)
	}
	if err := UploadStore.Put(ctx, newKey, bytesReader(payload), "image/png"); err != nil {
		t.Fatalf("put new: %v", err)
	}

	oldRow := &model.UploadedImage{
		UserID: 1, StorageKey: oldKey, MediaType: "image/png", Size: 1, Backend: "local",
		CreatedAt: time.Now().Add(-48 * time.Hour), // explicitly old
	}
	newRow := &model.UploadedImage{
		UserID: 1, StorageKey: newKey, MediaType: "image/png", Size: 1, Backend: "local",
	}
	if err := oldRow.Insert(); err != nil {
		t.Fatalf("insert old row: %v", err)
	}
	if err := newRow.Insert(); err != nil {
		t.Fatalf("insert new row: %v", err)
	}

	runCleanupSweep(ctx)

	// Expired row + blob gone.
	if _, err := model.GetUploadedImageByKey(oldKey); err == nil {
		t.Fatalf("expired row should have been deleted")
	}
	if rc, err := UploadStore.Get(ctx, oldKey); err != storage.ErrObjectNotFound {
		if rc != nil {
			rc.Close()
		}
		t.Fatalf("expired blob should be gone, got err=%v", err)
	}

	// Recent row + blob retained.
	if _, err := model.GetUploadedImageByKey(newKey); err != nil {
		t.Fatalf("recent row should remain, got err=%v", err)
	}
	rc, err := UploadStore.Get(ctx, newKey)
	if err != nil {
		t.Fatalf("recent blob should remain, got err=%v", err)
	}
	rc.Close()
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
