package service

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/imageasset"
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

	// Permanent retention must keep existing files, even when they predate it.
	model.UpdateOption("UploadRetentionHours", "0")
	runCleanupSweep(ctx)
	if _, err := model.GetUploadedImageByID(oldRow.ID); err != nil {
		t.Fatal("permanent retention deleted an existing image", err)
	}
	if _, err := UploadStore.Stat(ctx, oldKey); err != nil {
		t.Fatal("permanent retention deleted image bytes", err)
	}
	model.UpdateOption("UploadRetentionHours", "1")
	runCleanupSweep(ctx)

	// Expired row + blob gone.
	if _, err := model.GetUploadedImageByID(oldRow.ID); err == nil {
		t.Fatalf("expired row should have been deleted")
	}
	if rc, err := UploadStore.Get(ctx, oldKey); err != storage.ErrObjectNotFound {
		if rc != nil {
			rc.Close()
		}
		t.Fatalf("expired blob should be gone, got err=%v", err)
	}

	// Recent row + blob retained.
	if _, err := model.GetUploadedImageByID(newRow.ID); err != nil {
		t.Fatalf("recent row should remain, got err=%v", err)
	}
	rc, err := UploadStore.Get(ctx, newKey)
	if err != nil {
		t.Fatalf("recent blob should remain, got err=%v", err)
	}
	rc.Close()
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func TestCleanupUsesIndependentGeneratedImageRetention(t *testing.T) {
	db := imageGenTestSetup(t)
	ctx := context.Background()
	data := imageGenPNG(t, 3, 4)
	upload, err := imageasset.Save(ctx, UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := imageasset.SaveGenerated(ctx, UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	db.Model(&model.UploadedImage{}).Where("id IN ?", []int64{upload.UploadID, generated.UploadID}).Update("created_at", old)
	model.UpdateOption("UploadRetentionHours", "0")
	model.UpdateOption("ImageGenRetentionHours", "1")
	runCleanupSweep(ctx)
	if _, err := model.GetUploadedImageByID(generated.UploadID); err == nil {
		t.Fatal("generated retention was skipped when uploads are permanent")
	}
	if _, err := model.GetUploadedImageByID(upload.UploadID); err != nil {
		t.Fatal("generated cleanup deleted permanent upload", err)
	}
	if _, err := UploadStore.Stat(ctx, upload.Key); err != nil {
		t.Fatal("shared blob removed", err)
	}

	model.UpdateOption("ImageGenRetentionHours", "0")
	permanent, err := imageasset.SaveGenerated(ctx, UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	db.Model(&model.UploadedImage{}).Where("id = ?", permanent.UploadID).Update("created_at", old)
	model.UpdateOption("UploadRetentionHours", "1")
	runCleanupSweep(ctx)
	if _, err := model.GetUploadedImageByID(upload.UploadID); err == nil {
		t.Fatal("expired upload not cleaned")
	}
	if _, err := model.GetUploadedImageByID(permanent.UploadID); err != nil {
		t.Fatal("upload cleanup deleted permanent generation", err)
	}
	if _, err := UploadStore.Stat(ctx, upload.Key); err != nil {
		t.Fatal("permanent generated bytes removed", err)
	}
	model.UpdateOption("ImageGenRetentionHours", "1")
	runCleanupSweep(ctx)
	if _, err := UploadStore.Stat(ctx, upload.Key); err == nil {
		t.Fatal("last expired source did not remove blob")
	}
}
