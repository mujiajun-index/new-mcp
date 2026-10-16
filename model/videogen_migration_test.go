package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type videoGenTaskBeforeRetry struct {
	ID             string `gorm:"primaryKey;size:36"`
	UserID         int64  `gorm:"not null;index"`
	IdempotencyKey string `gorm:"size:64;not null;uniqueIndex"`
	Prompt         string `gorm:"type:text"`
	Status         string `gorm:"size:32;index"`
}

func (videoGenTaskBeforeRetry) TableName() string { return "videogen_tasks" }

// Server DSNs must address disposable, empty test databases, not application
// data. SQLite is always exercised; the server matrix is opt-in infrastructure.
func TestVideoGenMigrations(t *testing.T) {
	for _, engine := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			var dialector gorm.Dialector
			switch engine {
			case "sqlite":
				dialector = sqlite.Open(filepath.Join(t.TempDir(), "migration.db"))
			case "mysql":
				dsn := os.Getenv("VIDEOGEN_TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("VIDEOGEN_TEST_MYSQL_DSN is not configured")
				}
				dialector = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("VIDEOGEN_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("VIDEOGEN_TEST_POSTGRES_DSN is not configured")
				}
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			sql, _ := db.DB()
			defer sql.Close()
			var version string
			query := "SELECT version()"
			if engine == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			if err := db.Raw(query).Scan(&version).Error; err != nil {
				t.Fatal(err)
			}
			t.Logf("%s %s", engine, version)
			for _, m := range []any{&VideoGenTask{}, &VideoGenConfig{}, &ImageGenConfig{}} {
				if db.Migrator().HasTable(m) {
					t.Fatal("migration test requires an empty database")
				}
			}
			if err := db.AutoMigrate(&ImageGenConfig{}); err != nil {
				t.Fatal(err)
			}
			legacy := ImageGenConfig{UserID: 1, Name: "existing image configuration", Provider: "openai", ModelName: "image-model", EndpointURL: "https://gateway.example", Operation: "both", DefaultParams: "{}"}
			if err := db.Create(&legacy).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&videoGenTaskBeforeRetry{}); err != nil {
				t.Fatal(err)
			}
			oldTask := videoGenTaskBeforeRetry{ID: "pre-retry-video", UserID: 1, IdempotencyKey: "pre-retry-video", Prompt: "existing failed prompt", Status: "failed"}
			if err := db.Create(&oldTask).Error; err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := db.AutoMigrate(&VideoGenConfig{}, &VideoGenTask{}); err != nil {
					t.Fatal(err)
				}
			}
			var persisted ImageGenConfig
			if err := db.First(&persisted, legacy.ID).Error; err != nil || persisted.Name != legacy.Name {
				t.Fatal("existing image data changed")
			}
			var migratedTask VideoGenTask
			if err := db.First(&migratedTask, "id = ?", oldTask.ID).Error; err != nil || migratedTask.Prompt != oldTask.Prompt || migratedTask.Status != "failed" || migratedTask.Protocol != "openai_video" || migratedTask.RequestParams != "" || migratedTask.RetryOf != "" || migratedTask.RetryRequests != "" || migratedTask.RetryCount != 0 {
				t.Fatalf("existing video retry migration: task=%+v err=%v", migratedTask, err)
			}
			for _, id := range []string{"first", "second"} {
				row := VideoGenTask{ID: id, UserID: 1, IdempotencyKey: id, Status: "failed", ArtifactStatus: "pending", LeaseUntil: time.Unix(0, 0).UTC(), NextPollAt: time.Now(), TrackingDeadline: time.Now()}
				if err := db.Create(&row).Error; err != nil {
					t.Fatal(err)
				}
			}
			duplicate := VideoGenTask{ID: "duplicate", UserID: 1, IdempotencyKey: "first", LeaseUntil: time.Unix(0, 0).UTC(), NextPollAt: time.Now(), TrackingDeadline: time.Now()}
			if err := db.Create(&duplicate).Error; err == nil {
				t.Fatal("idempotency uniqueness missing")
			}
			for _, m := range []any{&VideoGenTask{}, &VideoGenConfig{}, &ImageGenConfig{}} {
				if err := db.Migrator().DropTable(m); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
