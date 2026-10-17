package taskreference

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReferenceAssetsOwnershipRetentionAndLegacy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "references.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	model.DB = db
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close(); model.DB = oldDB })
	if err := db.AutoMigrate(&model.UploadedImage{}, &model.Option{}); err != nil {
		t.Fatal(err)
	}
	model.InitOptionMap()
	model.UpdateOption("ServerAddress", "https://files.example")
	store, err := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 3, 2)))
	img, _, _, _ := imagegen.Inspect(data.Bytes(), 1024)
	item, err := Capture(context.Background(), store, 1, img, "reference")
	if err != nil {
		t.Fatal(err)
	}
	encoded := Encode([]Image{item, {UploadID: item.UploadID, Role: "mask", MIME: img.MIME, Size: item.Size, Width: 3, Height: 2}})
	if strings.Contains(encoded, "base64") || strings.Contains(encoded, "?s=") {
		t.Fatal("inline bytes or signed URL persisted")
	}
	views := Views(db, 1, encoded)
	if len(views) != 2 || views[0].URL == "" || views[0].Status != "ready" || views[0].Width != 3 || views[1].Role != "mask" {
		t.Fatalf("views: %+v", views)
	}
	public, _ := json.Marshal(views)
	if strings.Contains(string(public), "upload_id") || strings.Contains(string(public), "storage_key") {
		t.Fatal("private asset identifiers exposed")
	}
	if _, total, err := model.ListAllUploads(0, 0, 20); err != nil || total != 0 {
		t.Fatal("reference cache leaked into upload management", total, err)
	}
	model.UpdateOption("MaxUploadsPerUser", "1")
	if reused, err := Capture(context.Background(), store, 1, img, "reference"); err != nil || reused.UploadID != item.UploadID {
		t.Fatal("reuse consumed quota", err)
	}
	if foreign := Views(db, 2, encoded); foreign[0].URL != "" || foreign[0].Status != "expired" {
		t.Fatal("cross-user reference leak")
	}
	legacy := LegacyUpload(db, 1, views[0].URL)
	if legacy == "" || LegacyUpload(db, 2, views[0].URL) != "" || LegacyUpload(db, 1, views[0].URL+"bad") != "" || LegacyUpload(db, 1, "https://other.example/u/foo?s=bad") != "" {
		t.Fatal("unsafe legacy recovery")
	}
	db.Model(&model.UploadedImage{}).Where("id = ?", item.UploadID).Update("created_at", time.Now().Add(-48*time.Hour))
	if expired := Views(db, 1, encoded); expired[0].URL != "" || expired[0].Status != "expired" || expired[0].ExpiresAt == nil {
		t.Fatal("expired reference remains available")
	}
	model.UpdateOption("UploadRetentionHours", "0")
	model.UpdateOption("ServerAddress", "https://new-files.example")
	permanent := Views(db, 1, encoded)
	if permanent[0].Status != "ready" || permanent[0].ExpiresAt != nil || !strings.HasPrefix(permanent[0].URL, "https://new-files.example/") {
		t.Fatal("retention or refreshed URL incorrect")
	}
	db.Delete(&model.UploadedImage{}, item.UploadID)
	if missing := Views(db, 1, encoded); missing[0].URL != "" || missing[0].Status != "expired" {
		t.Fatal("deleted reference remains available")
	}
	if len(Views(db, 1, "broken")) != 0 {
		t.Fatal("invalid legacy metadata")
	}
	if _, err := Capture(context.Background(), store, 1, imagegen.Image{Bytes: []byte("not an image")}, "reference"); err == nil {
		t.Fatal("invalid image archived")
	}
	// An explicit upload of the same bytes remains visible when reused by tasks.
	model.UpdateOption("MaxUploadsPerUser", "0")
	upload, err := imageasset.Save(context.Background(), store, 1, bytes.NewReader(img.Bytes), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if reused, err := Capture(context.Background(), store, 1, img, "reference"); err != nil || reused.UploadID != upload.UploadID {
		t.Fatal("upload was duplicated", err)
	}
	if _, total, _ := model.ListAllUploads(0, 0, 20); total != 1 {
		t.Fatal("task hid explicit upload")
	}
}
