package model

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestMigrateShortIDBackfillAndIndex validates the V1.4 migration path: legacy
// rows (short_id empty from the column default) are backfilled with distinct
// non-empty short_ids, and the unique index is then created — the order that
// avoids the startup crash a tag-based uniqueIndex would cause. Also asserts both
// steps are idempotent (safe to re-run on every startup).
func TestMigrateShortIDBackfillAndIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	DB = db
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})

	// Initial AutoMigrate: adds the short_id column with NO unique index (the index
	// is added later by ensureShortIDIndex, after backfill).
	if err := DB.AutoMigrate(&UploadedImage{}); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	// Seed legacy rows in the pre-V1.4 state (short_id left at the '' default).
	for i := 0; i < 5; i++ {
		if err := DB.Create(&UploadedImage{
			UserID:     int64(i + 1),
			StorageKey: fmt.Sprintf("ab/%d", i),
			Backend:    "local",
			Status:     UploadStatusUploaded,
		}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := backfillShortIDs(); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if err := ensureShortIDIndex(); err != nil {
		t.Fatalf("ensure index: %v", err)
	}

	// Every row now has a distinct non-empty short_id.
	var rows []UploadedImage
	if err := DB.Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.ShortID == "" {
			t.Fatalf("row %d still has empty short_id", r.ID)
		}
		if seen[r.ShortID] {
			t.Fatalf("duplicate short_id %q after backfill", r.ShortID)
		}
		seen[r.ShortID] = true
	}
	if !DB.Migrator().HasIndex(&UploadedImage{}, "idx_uploaded_images_short_id") {
		t.Fatalf("unique index was not created")
	}

	// Idempotent: re-running both steps is a no-op (no error, index still present).
	if err := backfillShortIDs(); err != nil {
		t.Fatalf("backfill re-run: %v", err)
	}
	if err := ensureShortIDIndex(); err != nil {
		t.Fatalf("index re-run: %v", err)
	}
}
