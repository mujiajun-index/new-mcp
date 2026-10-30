package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/internal/mcp/virtual/events"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func adapterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "virtual.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	old := model.DB
	model.DB = db
	if err = db.AutoMigrate(&model.McpService{}, &model.MarketplaceItem{}, &model.VisionConfig{}, &model.SystemOneConfig{}, &model.ImageGenConfig{}, &model.VideoGenConfig{}, &model.AudioGenConfig{}, &model.VideoGenTask{}, &model.ImageGenTask{}, &model.AudioGenTask{}, &model.Option{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { model.DB = old; sql, _ := db.DB(); _ = sql.Close() })
	return db
}
func boundFixture(t *testing.T, db *gorm.DB, kind string) (*model.MarketplaceItem, *model.McpService) {
	t.Helper()
	source := model.McpService{UserID: 1, Name: kind, Source: kind, TransportType: "virtual", Config: fmt.Sprintf(`{"virtual_type":%q,"ref_id":1}`, kind), ToolsCache: `[{"name":"sample","inputSchema":{"type":"object"}}]`, Status: 1}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	switch kind {
	case "vision":
		db.Create(&model.VisionConfig{ID: 1, UserID: 1, Name: kind, AutoRegister: true, RegisteredServiceID: &source.ID})
	case "systemone":
		db.Create(&model.SystemOneConfig{ID: 1, UserID: 1, Name: kind, AutoRegister: true, RegisteredServiceID: &source.ID})
	case "imagegen":
		db.Create(&model.ImageGenConfig{ID: 1, UserID: 1, Name: kind, AutoRegister: true, RegisteredServiceID: &source.ID, Status: 1})
	case "videogen":
		db.Create(&model.VideoGenConfig{ID: 1, UserID: 1, Name: kind, AutoRegister: true, RegisteredServiceID: &source.ID, Status: 1})
	case "audiogen":
		db.Create(&model.AudioGenConfig{ID: 1, UserID: 1, Name: kind, AutoRegister: true, RegisteredServiceID: &source.ID, Status: 1})
	}
	item := model.MarketplaceItem{AdminID: 1, Name: "market-" + kind, Category: "instant", TransportType: "virtual", ConfigTemplate: source.Config, Status: 1}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	ref := model.McpService{UserID: 2, Name: "installed-" + kind, Source: "marketplace", TransportType: "marketplace", MarketplaceItemID: &item.ID, Config: "{}", Status: 1}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatal(err)
	}
	return &item, &ref
}
func TestAdapterAllMarketBindings(t *testing.T) {
	db := adapterTestDB(t)
	for _, kind := range []string{"vision", "systemone", "imagegen", "videogen", "audiogen"} {
		t.Run(kind, func(t *testing.T) {
			_, ref := boundFixture(t, db, kind)
			a := NewAdapter(ref)
			if err := a.Connect(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			if len(a.GetTools()) != 1 {
				t.Fatal("tools missing")
			}
			if _, err := a.ListResources(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := a.ListPrompts(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := a.Call(context.Background(), "tools/call", map[string]any{"name": "forged"}); err == nil {
				t.Fatal("unknown tool accepted")
			}
			// A warm session must read enablement again on each request.
			if err := db.Table(model.VirtualConfigTable(kind)).Where("id = ?", 1).Update("auto_register", false).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := a.Call(context.Background(), "ping", nil); err == nil {
				t.Fatal("disabled binding accepted")
			}
		})
	}
}
func TestAdapterTaskIsolationAndNotifications(t *testing.T) {
	db := adapterTestDB(t)
	_, ref := boundFixture(t, db, "videogen")
	rows := []model.VideoGenTask{
		{ID: "mine", UserID: 2, ServiceID: ref.ID, ConfigID: 1, IdempotencyKey: "a", Status: "queued"},
		{ID: "other-user", UserID: 3, ServiceID: ref.ID, ConfigID: 1, IdempotencyKey: "b", Status: "queued"},
		{ID: "other-service", UserID: 2, ServiceID: ref.ID + 1, ConfigID: 1, IdempotencyKey: "c", Status: "queued"},
		{ID: "platform-history", UserID: 1, ConfigID: 1, IdempotencyKey: "d", Status: "queued"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	a := NewAdapter(ref)
	if err := a.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	list, err := a.ListResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(list) != `{"resources":[]}` {
		t.Fatalf("task history should not appear in resources/list: %s", list)
	}
	templates, err := a.ListResourceTemplates(context.Background())
	if err != nil || !strings.Contains(string(templates), "tasks://videogen/{task_id}") {
		t.Fatalf("missing task resource template: %s (%v)", templates, err)
	}
	caps := a.NotificationCapabilities()
	if caps.ResourcesListChanged || !caps.ResourceSubscribe {
		t.Fatalf("unexpected resource notification capabilities: %+v", caps)
	}
	if _, err = a.ReadResource(context.Background(), "tasks://videogen/other-user"); err == nil {
		t.Fatal("cross-user read")
	}
	if _, err = a.WatchNotifications(context.Background(), []string{"tasks://videogen/other-service"}, func(transport.Notification) {}); err == nil {
		t.Fatal("cross-service watch")
	}
	notifications := make(chan transport.Notification, 10)
	stop, err := a.WatchNotifications(context.Background(), []string{"tasks://videogen/mine"}, func(n transport.Notification) { notifications <- n })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err = db.Model(&model.VideoGenTask{}).Where("id = ?", "mine").Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	events.TaskChanged("videogen", 2, ref.ID, 1)
	select {
	case n := <-notifications:
		if n.Method != "notifications/resources/updated" {
			t.Fatalf("unexpected notification: %+v", n)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("missing task notification")
	}
	if err = db.Create(&model.VideoGenTask{ID: "new-task", UserID: 2, ServiceID: ref.ID, ConfigID: 1, IdempotencyKey: "e", Status: "queued"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = a.ReadResource(context.Background(), "tasks://videogen/new-task"); err != nil {
		t.Fatalf("template task read failed: %v", err)
	}
	newTaskNotices := make(chan transport.Notification, 1)
	stopNewTask, err := a.WatchNotifications(context.Background(), []string{"tasks://videogen/new-task"}, func(n transport.Notification) { newTaskNotices <- n })
	if err != nil {
		t.Fatal(err)
	}
	defer stopNewTask()
	if err = db.Model(&model.VideoGenTask{}).Where("id = ?", "new-task").Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	events.TaskChanged("videogen", 2, ref.ID, 1)
	select {
	case n := <-newTaskNotices:
		if n.Method != "notifications/resources/updated" || !strings.Contains(string(n.Params), "tasks://videogen/new-task") {
			t.Fatalf("unexpected task notification: %+v", n)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("subscribed task update was not delivered")
	}
	stopNewTask()
	if err = db.Model(&model.McpService{}).Where("id = ?", 1).Update("tools_cache", `[{"name":"changed","inputSchema":{"type":"object"}}]`).Error; err != nil {
		t.Fatal(err)
	}
	events.ConfigChanged("videogen", 1)
	select {
	case n := <-notifications:
		if n.Method != "notifications/tools/list_changed" {
			t.Fatalf("unexpected notification: %+v", n)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("missing tools notification")
	}
	// Rebinding keeps the service's previous tasks accessible.
	if err = db.Model(&model.VideoGenTask{}).Where("id = ?", "mine").Update("config_id", 999).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := a.ReadResource(context.Background(), "tasks://videogen/mine")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil || result["contents"] == nil {
		t.Fatalf("invalid MCP resource result: %s", raw)
	}
}

func TestAdapterIdleDoesNotQuery(t *testing.T) {
	db := adapterTestDB(t)
	_, ref := boundFixture(t, db, "imagegen")
	a := NewAdapter(ref)
	if err := a.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var queries atomic.Int64
	if err := db.Callback().Query().After("gorm:query").Register("count_virtual_idle_queries", func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if got := queries.Load(); got != 0 {
		t.Fatalf("idle virtual adapter ran %d queries", got)
	}
}
