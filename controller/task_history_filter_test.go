package controller

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTaskHistoryStatusFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "history.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Option{}, &model.UploadedImage{}, &model.ImageGenTask{}, &model.ImageGenTaskImage{}, &model.VideoGenTask{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldManager := model.DB, virtual.VideoManager
	model.DB = db
	model.InitOptionMap()
	virtual.VideoManager = videogen.NewManager(db, nil)
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		virtual.VideoManager.Close()
		sqlDB.Close()
		model.DB, virtual.VideoManager = oldDB, oldManager
	})
	for i, row := range []struct {
		uid, config int64
		status      string
	}{
		{1, 1, "completed"}, {1, 1, "completed"}, {1, 1, "completed"},
		{1, 1, "failed"}, {1, 1, "running"}, {1, 2, "completed"}, {2, 1, "completed"},
	} {
		img := model.ImageGenTask{ID: fmt.Sprintf("image-%d", i), UserID: row.uid, ConfigID: row.config, Status: row.status}
		if err := db.Create(&img).Error; err != nil {
			t.Fatal(err)
		}
		status := row.status
		if status == "running" {
			status = "queued"
		}
		video := model.VideoGenTask{ID: fmt.Sprintf("video-%d", i), IdempotencyKey: fmt.Sprintf("key-%d", i), UserID: row.uid, ConfigID: row.config, Status: status}
		if err := db.Create(&video).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			list, running := ListImageGenTasks, "running"
			if kind == "video" {
				list, running = ListVideoGenTasks, "queued"
			}
			for _, tc := range []struct {
				query, status      string
				uid                int64
				total, count, code int
			}{
				{"", "", 1, 6, 6, 200},
				{"?status=all", "", 1, 6, 6, 200},
				{"?status=completed&config_id=1&page_size=2", "completed", 1, 3, 2, 200},
				{"?status=completed&config_id=1&page_size=2&page=2", "completed", 1, 3, 1, 200},
				{"?status=completed&config_id=2", "completed", 1, 1, 1, 200},
				{"?status=failed", "failed", 1, 1, 1, 200},
				{"?status=" + running, running, 1, 1, 1, 200},
				{"?status=completed", "completed", 2, 1, 1, 200},
				{"?status=completed", "completed", 3, 0, 0, 200},
				{"?status=not-a-state", "", 1, 0, 0, 400},
			} {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Set("user_id", tc.uid)
				c.Request = httptest.NewRequest("GET", "/tasks"+tc.query, nil)
				list(c)
				if w.Code != tc.code {
					t.Fatalf("%s: status=%d body=%s", tc.query, w.Code, w.Body)
				}
				if tc.code != 200 {
					continue
				}
				var reply struct {
					Data struct {
						Total int
						Items []struct {
							Status string `json:"status"`
						}
					}
				}
				if err := json.Unmarshal(w.Body.Bytes(), &reply); err != nil {
					t.Fatal(err)
				}
				if reply.Data.Total != tc.total || len(reply.Data.Items) != tc.count {
					t.Fatalf("%s: wrong filtered pagination: %s", tc.query, w.Body)
				}
				for _, item := range reply.Data.Items {
					if tc.status != "" && item.Status != tc.status {
						t.Fatalf("%s: wrong status %s", tc.query, item.Status)
					}
				}
			}
		})
	}
}
