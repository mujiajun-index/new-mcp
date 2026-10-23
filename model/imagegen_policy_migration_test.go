package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestImageSourcePolicyIndexMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "sources.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close(); DB = oldDB })
	if err := db.AutoMigrate(&UploadedImage{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX idx_uploaded_images_user_storage ON uploaded_images (user_id, storage_key)").Error; err != nil {
		t.Fatal(err)
	}
	upload := UploadedImage{UserID: 1, StorageKey: "ab/shared", Source: ImageSourceUpload, MediaType: "image/png", Backend: "local", Size: 10}
	if err := upload.InsertWithGeneratedShortID(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := migrateDB(); err != nil {
			t.Fatal(err)
		}
	}
	if db.Migrator().HasIndex(&UploadedImage{}, "idx_uploaded_images_user_storage") {
		t.Fatal("cross-source unique constraint remains")
	}
	generated := UploadedImage{UserID: 1, StorageKey: upload.StorageKey, Source: ImageSourceGenerated, MediaType: "image/png", Backend: "local", Size: 10}
	if err := generated.InsertWithGeneratedShortID(); err != nil {
		t.Fatal("independent generated asset rejected", err)
	}
	if err := (&UploadedImage{UserID: 1, StorageKey: upload.StorageKey, Source: ImageSourceGenerated, MediaType: "image/png", Backend: "local", Size: 10}).InsertWithGeneratedShortID(); err == nil {
		t.Fatal("same-source dedup constraint lost")
	}
	row, err := GetUploadedImageByID(upload.ID)
	if err != nil || row.ShortID != upload.ShortID || row.Source != ImageSourceUpload {
		t.Fatal("existing upload changed", row, err)
	}
}
