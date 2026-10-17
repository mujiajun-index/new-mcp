package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestImageGenHistoryMigrationPreservesUploads(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "history.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	t.Cleanup(func() { sqlDB, _ := db.DB(); sqlDB.Close(); DB = oldDB })
	// Seed the upload table before the source column exists.
	if err := db.Exec(`CREATE TABLE uploaded_images (id integer PRIMARY KEY AUTOINCREMENT, user_id integer NOT NULL, storage_key text NOT NULL, short_id text NOT NULL DEFAULT '', media_type text NOT NULL, size integer NOT NULL, backend text NOT NULL, status text NOT NULL DEFAULT 'uploaded', created_at datetime, updated_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO uploaded_images (user_id, storage_key, short_id, media_type, size, backend) VALUES (1, 'ab/legacy', 'legacy', 'image/png', 123, 'local')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE imagegen_tasks (id text PRIMARY KEY, user_id integer NOT NULL, prompt text, status text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO imagegen_tasks (id, user_id, prompt, status) VALUES ('legacy-task', 1, 'old prompt', 'completed')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE imagegen_task_images (id integer PRIMARY KEY AUTOINCREMENT, task_id text NOT NULL, upload_id integer NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO imagegen_task_images (task_id, upload_id) VALUES ('legacy-task', 1), ('legacy-task', 1)`).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.AutoMigrate(&UploadedImage{}, &ImageGenTask{}, &ImageGenTaskImage{}); err != nil {
			t.Fatal(err)
		}
	}
	items, total, err := ListAllUploads(0, 0, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].Source != ImageSourceUpload || items[0].ShortID != "legacy" {
		t.Fatalf("legacy uploads changed during migration: %+v %d %v", items, total, err)
	}
	var legacy ImageGenTask
	if err := db.First(&legacy, "id = ?", "legacy-task").Error; err != nil || legacy.Prompt != "old prompt" || legacy.ReferenceImages != "" {
		t.Fatal("legacy reference migration changed history", err)
	}
	const references = `[{"upload_id":1,"role":"reference","mime":"image/png"}]`
	if err := db.Create(&ImageGenTask{ID: "persisted-task", UserID: 1, Status: "completed", ReferenceImages: references}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ImageGenTask{}, &ImageGenTaskImage{}); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&ImageGenTask{}).Count(&count)
	if count != 2 {
		t.Fatal("repeated migration lost history")
	}
	var persisted ImageGenTask
	if err := db.First(&persisted, "id = ?", "persisted-task").Error; err != nil || persisted.ReferenceImages != references {
		t.Fatal("repeated migration lost reference metadata", err)
	}
	var images []ImageGenTaskImage
	if err := db.Where("task_id = ?", "legacy-task").Find(&images).Error; err != nil || len(images) != 2 || images[0].OutputIndex != nil || legacy.IdempotencyKey != nil {
		t.Fatalf("nullable async indexes did not preserve legacy multi-image history: %+v %v", images, err)
	}
}
