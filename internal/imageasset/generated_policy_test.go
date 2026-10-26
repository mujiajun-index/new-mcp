package imageasset

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func policyFixture(t *testing.T) (*gorm.DB, storage.Storage) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "policy.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	oldDB := model.DB
	model.DB = db
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close(); model.DB = oldDB })
	if err := db.AutoMigrate(&model.Option{}, &model.UploadedImage{}); err != nil {
		t.Fatal(err)
	}
	model.InitOptionMap()
	store, err := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return db, store
}

func policyPNG(t *testing.T, width int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewNRGBA(image.Rect(0, 0, width, 2))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestGeneratedImageRetentionIsIndependent(t *testing.T) {
	db, store := policyFixture(t)
	ctx := context.Background()
	data := policyPNG(t, 2)
	model.UpdateOption("UploadRetentionHours", "1")
	model.UpdateOption("ImageGenRetentionHours", "72")
	upload, err := Save(ctx, store, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := SaveGenerated(ctx, store, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if upload.UploadID == generated.UploadID || upload.Key != generated.Key || generated.ExpiresAt.Sub(*upload.ExpiresAt) < 70*time.Hour {
		t.Fatal("sources do not have independent metadata and retention", upload, generated)
	}
	old := time.Now().Add(-3 * time.Hour)
	db.Model(&model.UploadedImage{}).Where("id IN ?", []int64{upload.UploadID, generated.UploadID}).Update("created_at", old)
	if !Expired(&model.UploadedImage{CreatedAt: old, Source: model.ImageSourceUpload}) || Expired(&model.UploadedImage{CreatedAt: old, Source: model.ImageSourceGenerated}) {
		t.Fatal("upload retention affected generated image")
	}
	if _, err := Save(ctx, store, 1, bytes.NewReader(data), 1024); err != nil {
		t.Fatal(err)
	}
	row, _ := model.GetUploadedImageByID(generated.UploadID)
	if !row.CreatedAt.Equal(old) {
		t.Fatal("upload refreshed generated image lifetime")
	}
	model.UpdateOption("UploadRetentionHours", "0")
	model.UpdateOption("ImageGenRetentionHours", "1")
	if Expiration(&model.UploadedImage{CreatedAt: old, Source: model.ImageSourceUpload}) != nil || !Expired(&model.UploadedImage{CreatedAt: old, Source: model.ImageSourceGenerated}) {
		t.Fatal("permanent upload changed generated retention")
	}
	model.UpdateOption("ImageGenRetentionHours", "0")
	permanent, err := SaveGenerated(ctx, store, 1, bytes.NewReader(data), 1024)
	if err != nil || permanent.ExpiresAt != nil || !permanent.Deduped {
		t.Fatal("permanent generation retention", permanent, err)
	}
}

func TestGeneratedImageQuotaIsolationAndExpiredImages(t *testing.T) {
	db, store := policyFixture(t)
	ctx := context.Background()
	model.UpdateOption("MaxUploadsPerUser", "1")
	model.UpdateOption("MaxImagesPerUser", "2")
	if _, err := Save(ctx, store, 1, bytes.NewReader(policyPNG(t, 1)), 1024); err != nil {
		t.Fatal(err)
	}
	var generated []*Result
	for _, width := range []int{2, 3} {
		result, err := SaveGenerated(ctx, store, 1, bytes.NewReader(policyPNG(t, width)), 1024)
		if err != nil {
			t.Fatal("upload cap affected generation", err)
		}
		generated = append(generated, result)
	}
	if count, _ := model.CountUploadsByUser(1); count != 1 {
		t.Fatal("generation consumed upload quota", count)
	}
	if _, err := SaveGenerated(ctx, store, 1, bytes.NewReader(policyPNG(t, 4)), 1024); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Fatal("generated cap not enforced", err)
	}
	duplicate, err := SaveGenerated(ctx, store, 1, bytes.NewReader(policyPNG(t, 2)), 1024)
	if err != nil || !duplicate.Deduped || duplicate.UploadID != generated[0].UploadID {
		t.Fatal("saving existing output consumed another slot", duplicate, err)
	}
	if _, err := SaveGenerated(ctx, store, 2, bytes.NewReader(policyPNG(t, 4)), 1024); err != nil {
		t.Fatal("other user consumed quota", err)
	}
	db.Model(&model.UploadedImage{}).Where("id = ?", generated[0].UploadID).Update("created_at", time.Now().Add(-48*time.Hour))
	if _, err := SaveGenerated(ctx, store, 1, bytes.NewReader(policyPNG(t, 4)), 1024); err != nil {
		t.Fatal("expired image consumed quota", err)
	}
	if _, err := SaveGenerated(ctx, store, 1, bytes.NewReader(policyPNG(t, 2)), 1024); err == nil {
		t.Fatal("reviving expired output bypassed quota")
	}
	model.UpdateOption("ImageGenRetentionHours", "0")
	if err := CheckGeneratedQuota(1); err == nil {
		t.Fatal("permanent images not counted")
	}
	model.UpdateOption("MaxImagesPerUser", "0")
	if _, err := SaveGenerated(ctx, store, 1, bytes.NewReader(policyPNG(t, 5)), 1024); err != nil {
		t.Fatal("unlimited cap rejected save", err)
	}
}

func TestConcurrentGeneratedSavesRespectQuota(t *testing.T) {
	db, store := policyFixture(t)
	model.UpdateOption("MaxImagesPerUser", "1")
	var wg sync.WaitGroup
	for i := 1; i <= 8; i++ {
		data := policyPNG(t, i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = SaveGenerated(context.Background(), store, 1, bytes.NewReader(data), 1024)
		}()
	}
	wg.Wait()
	var count int64
	db.Model(&model.UploadedImage{}).Where("user_id = ? AND source = ?", 1, model.ImageSourceGenerated).Count(&count)
	if count != 1 {
		t.Fatal("concurrent saves exceeded quota", count)
	}
}
