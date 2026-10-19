package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLogsTimeZones(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.McpCallLog{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	previousLocation := time.Local
	model.DB = db
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		model.DB = previousDB
		time.Local = previousLocation
		_ = sqlDB.Close()
	})

	// 北京时间次日 00:34，覆盖跨日显示与日期范围筛选。
	instant := time.Date(2026, 10, 17, 16, 34, 10, 0, time.UTC)
	for i, location := range []*time.Location{time.UTC, time.FixedZone("CST", 8*60*60)} {
		time.Local = location
		createdAt := instant.In(location)
		log := model.McpCallLog{UserID: int64(i + 1), ToolName: "time-test", CreatedAt: createdAt}
		if err := log.Insert(); err != nil {
			t.Fatal(err)
		}
		items, total, err := (&AdminService{}).GetLogsForUser(log.UserID, false, nil, 1, 20)
		if err != nil || total != 1 || len(items) != 1 {
			t.Fatalf("%s: total=%d items=%d err=%v", location, total, len(items), err)
		}
		parsed, err := time.Parse(time.RFC3339, items[0].CreatedAt)
		if err != nil || !parsed.Equal(instant) {
			t.Fatalf("%s: API time %q changed the log's instant: %v", location, items[0].CreatedAt, err)
		}
		if want := createdAt.Format(time.RFC3339); items[0].CreatedAt != want {
			t.Fatalf("%s: API time=%q want=%q", location, items[0].CreatedAt, want)
		}

		outside := model.McpCallLog{UserID: log.UserID, ToolName: "outside-range", CreatedAt: createdAt.Add(-time.Hour)}
		if err := outside.Insert(); err != nil {
			t.Fatal(err)
		}
		for _, inputLocation := range []*time.Location{time.UTC, time.FixedZone("CST", 8*60*60)} {
			filter := &dto.LogFilter{
				StartDate: instant.Add(-time.Minute).In(inputLocation).Format(time.RFC3339),
				EndDate:   instant.Add(time.Minute).In(inputLocation).Format(time.RFC3339),
			}
			items, total, err := (&AdminService{}).GetLogsForUser(log.UserID, false, filter, 1, 20)
			if err != nil || total != 1 || len(items) != 1 || items[0].ID != log.ID {
				t.Fatalf("server=%s input=%s: date range returned total=%d items=%v err=%v", location, inputLocation, total, items, err)
			}
		}

		// 不指定 CreatedAt，验证真实新增日志的自动时间戳。
		beforeInsert := time.Now().UTC().Truncate(time.Second)
		latest := model.McpCallLog{UserID: int64(i + 101), ToolName: "latest-log"}
		if err := latest.Insert(); err != nil {
			t.Fatal(err)
		}
		items, total, err = (&AdminService{}).GetLogsForUser(latest.UserID, false, nil, 1, 20)
		if err != nil || total != 1 || len(items) != 1 {
			t.Fatalf("%s: latest log total=%d items=%d err=%v", location, total, len(items), err)
		}
		parsed, err = time.Parse(time.RFC3339, items[0].CreatedAt)
		if err != nil || parsed.Before(beforeInsert) || parsed.After(time.Now().UTC()) {
			t.Fatalf("%s: latest log time=%q is not the current instant: %v", location, items[0].CreatedAt, err)
		}
		if want := latest.CreatedAt.Format(time.RFC3339); items[0].CreatedAt != want {
			t.Fatalf("%s: latest log time=%q want=%q", location, items[0].CreatedAt, want)
		}
	}
}
