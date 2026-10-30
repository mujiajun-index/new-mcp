package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
)

func TestVirtualMarketplaceImageExecutionAndLifecycle(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.MarketplaceItem{}, &model.MarketplaceItemGroup{}, &model.MarketplaceGroup{}, &model.MarketplaceTag{}, &model.MarketplaceItemKey{}, &model.McpToolPrice{}, &model.User{}, &model.McpCallLog{}); err != nil {
		t.Fatal(err)
	}
	image := base64.StdEncoding.EncodeToString(imageGenPNG(t, 2, 2))
	var mu sync.Mutex
	models := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-image-key" {
			t.Error("bound credential not used")
		}
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		models = append(models, p["model"].(string))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": image}}})
	}))
	defer upstream.Close()
	configs := &ImageGenService{}
	market := &MarketplaceService{}
	cfg, err := configs.Create(1, imageGenReq(upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err = configs.Enable(1, cfg.ID); err != nil {
		t.Fatal(err)
	}
	current, _ := model.GetImageGenConfig(1, cfg.ID)
	item, err := market.CloneFromService(1, &dto.CloneMarketplaceReq{FromServiceID: *current.RegisteredServiceID, Name: "platform-image", BillingType: "free"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = market.CloneFromService(2, &dto.CloneMarketplaceReq{FromServiceID: *current.RegisteredServiceID, Name: "stolen", BillingType: "free"}); err == nil {
		t.Fatal("listed another user's config")
	}
	installed, err := market.AddToMyServices(2, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := model.GetServiceByID(2, installed.ServiceID)
	adapter := virtual.NewAdapter(ref)
	if err = adapter.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	notifications := make(chan transport.Notification, 16)
	stop, err := adapter.WatchNotifications(context.Background(), nil, func(n transport.Notification) { notifications <- n })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	submit := func(request string) model.ImageGenTask {
		t.Helper()
		raw, e := adapter.Call(context.Background(), "tools/call", map[string]any{"name": "generate_image", "arguments": map[string]any{"prompt": "draw", "wait_seconds": 0, "request_id": request}})
		if e != nil || strings.Contains(string(raw), `"isError":true`) {
			t.Fatalf("market call: %s %v", raw, e)
		}
		var task model.ImageGenTask
		if e = db.Where("user_id = ? AND service_id = ?", 2, ref.ID).Order("created_at DESC").First(&task).Error; e != nil {
			t.Fatal(e)
		}
		if task.MarketplaceItemID == nil || *task.MarketplaceItemID != item.ID || task.ConfigID != cfg.ID {
			t.Fatalf("incorrect task attribution: %+v", task)
		}
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			_ = db.First(&task, "id = ?", task.ID).Error
			if task.Status == "completed" {
				return task
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("generation did not complete: %+v", task)
		return task
	}
	first := submit("one")
	resources, err := adapter.ListResources(context.Background())
	if err != nil || string(resources) != `{"resources":[]}` {
		t.Fatalf("market task history should not be listed: %s (%v)", resources, err)
	}
	templates, err := adapter.ListResourceTemplates(context.Background())
	if err != nil || !strings.Contains(string(templates), "tasks://imagegen/{task_id}") {
		t.Fatalf("missing generation task template: %s (%v)", templates, err)
	}
	if _, err = adapter.ReadResource(context.Background(), "tasks://imagegen/"+first.ID); err != nil {
		t.Fatalf("market task cannot be read by ID: %v", err)
	}
	staleResources := `{"resources":[{"uri":"tasks://imagegen/` + first.ID + `","name":"old task"}],"templates":[{"uriTemplate":"tasks://imagegen/{task_id}"}]}`
	for _, serviceID := range []int64{ref.ID, *current.RegisteredServiceID} {
		if err = db.Model(&model.McpService{}).Where("id = ?", serviceID).Update("resources_cache", staleResources).Error; err != nil {
			t.Fatal(err)
		}
		ownerID := int64(2)
		if serviceID == *current.RegisteredServiceID {
			ownerID = 1
		}
		cached, getErr := (&McpServiceService{}).GetResources(ownerID, serviceID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if rows, ok := cached["resources"].([]any); !ok || len(rows) != 0 {
			t.Fatalf("old virtual task cache leaked for service %d: %+v (%v)", serviceID, cached, getErr)
		}
	}
	if err = db.Model(&model.MarketplaceItem{}).Where("id = ?", item.ID).Update("resources_snapshot", staleResources).Error; err != nil {
		t.Fatal(err)
	}
	marketDetail, err := market.GetPublished(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rows, ok := marketDetail.ResourcesSnapshot["resources"].([]any); !ok || len(rows) != 0 {
		t.Fatalf("old market task snapshot leaked: %+v (%v)", marketDetail, err)
	}
	nextModel := "updated-model"
	nextDescription := "Updated platform image generation"
	if err = configs.Update(1, cfg.ID, &dto.ImageGenConfigReq{ModelName: &nextModel, GenerateImageDesc: &nextDescription}); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case n := <-notifications:
			if n.Method == "notifications/tools/list_changed" {
				goto toolsChanged
			}
		case <-deadline:
			t.Fatal("market image config update did not notify tool subscribers")
		}
	}
toolsChanged:
	_ = submit("two")
	mu.Lock()
	seen := append([]string(nil), models...)
	mu.Unlock()
	if len(seen) != 2 || seen[0] != "custom-alias" || seen[1] != nextModel {
		t.Fatalf("binding did not refresh: %v", seen)
	}
	// A second installed service for the same user/config gets a separate idempotency scope.
	another := *ref
	another.ID = 0
	another.Name = "second-installation"
	if err = db.Create(&another).Error; err != nil {
		t.Fatal(err)
	}
	otherAdapter := virtual.NewAdapter(&another)
	if err = otherAdapter.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer otherAdapter.Close()
	if _, err = otherAdapter.ReadResource(context.Background(), "tasks://imagegen/"+first.ID); err == nil {
		t.Fatal("cross-service task read")
	}
	if err = configs.Delete(1, cfg.ID); err == nil {
		t.Fatal("deleted bound config")
	}
	public, err := market.GetPublished(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	publicJSON, _ := json.Marshal(public)
	if public.VirtualBinding != nil || strings.Contains(string(publicJSON), "secret-image-key") || strings.Contains(string(publicJSON), first.ID) {
		t.Fatalf("public data leak: %s", publicJSON)
	}
	// Refreshing public snapshots must not copy the platform owner's task list.
	if _, err = market.RefreshItemSnapshots(item.ID); err != nil {
		t.Fatal(err)
	}
	if err = configs.Disable(1, cfg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.Call(context.Background(), "tools/call", map[string]any{"name": "generate_image", "arguments": map[string]any{"prompt": "blocked"}}); err == nil {
		t.Fatal("warm adapter called disabled configuration")
	}
	if err = configs.Enable(1, cfg.ID); err != nil {
		t.Fatal(err)
	}
	if err = adapter.Close(); err != nil {
		t.Fatal(err)
	}
	if err = adapter.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	stopReconnected, watchErr := adapter.WatchNotifications(context.Background(), nil, func(n transport.Notification) { notifications <- n })
	if watchErr != nil {
		t.Fatal(watchErr)
	}
	defer stopReconnected()
	if _, err = adapter.ReadResource(context.Background(), "tasks://imagegen/"+first.ID); err != nil {
		t.Fatal(err)
	}
	// Ownership validation also applies to the rebind API, including raw-config bypasses.
	otherCfg, err := configs.Create(3, imageGenReq(upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err = configs.Enable(3, otherCfg.ID); err != nil {
		t.Fatal(err)
	}
	if err = market.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{VirtualBinding: &model.VirtualBinding{VirtualType: "imagegen", RefID: otherCfg.ID}}); err == nil {
		t.Fatal("foreign rebind accepted")
	}
	if err = market.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{ConfigTemplate: map[string]any{"virtual_type": "imagegen", "ref_id": otherCfg.ID}}); err == nil {
		t.Fatal("raw configuration bypass accepted")
	}
	secondCfg, err := configs.Create(1, imageGenReq(upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err = configs.Enable(1, secondCfg.ID); err != nil {
		t.Fatal(err)
	}
	if err = market.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{VirtualBinding: &model.VirtualBinding{VirtualType: "imagegen", RefID: secondCfg.ID}}); err != nil {
		t.Fatal(err)
	}
	waitForToolsChange := func(reason string) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		seen := []string{}
		for {
			select {
			case n := <-notifications:
				seen = append(seen, n.Method)
				if n.Method == "notifications/tools/list_changed" {
					return
				}
			case <-deadline:
				t.Fatalf("%s did not notify tools/list_changed (connected=%t, seen=%v)", reason, adapter.IsConnected(), seen)
			}
		}
	}
	waitForToolsChange("marketplace rebind")
	newBoundDescription := "New bound configuration"
	if err = configs.Update(1, secondCfg.ID, &dto.ImageGenConfigReq{GenerateImageDesc: &newBoundDescription}); err != nil {
		t.Fatal(err)
	}
	waitForToolsChange("new bound configuration edit")
	if _, err = adapter.ReadResource(context.Background(), "tasks://imagegen/"+first.ID); err != nil {
		t.Fatal("rebind lost history", err)
	}
	if err = configs.Delete(1, cfg.ID); err != nil {
		t.Fatal("unbound config deletion failed", err)
	}
	// The shared service-test path must charge a market virtual call, while keeping ownership with the caller.
	if err = db.Create(&model.User{ID: 2, Username: "virtual-market-caller", AffCode: "virtual-market-caller", Status: 1, Group: "default", Quota: 20000, Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = model.UpdateOption("BillingEnabled", "true"); err != nil {
		t.Fatal(err)
	}
	billingType := "per_call"
	price := 0.01
	if err = market.UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{BillingType: &billingType, PricePerCall: &price}); err != nil {
		t.Fatal(err)
	}
	result, err := (&McpServiceService{}).CallTool(2, ref.ID, &dto.CallToolReq{Name: "generate_image", Arguments: map[string]any{"prompt": "paid", "wait_seconds": 0}})
	if err != nil || result.IsError {
		t.Fatalf("paid virtual tool: %+v %v", result, err)
	}
	log := lastManualTestLog(t)
	if log.BillingStatus != "charged" || log.QuotaConsumed != 5000 || log.ServiceID != ref.ID {
		t.Fatalf("virtual billing: %+v", log)
	}
	if quota, used := userQuota(t, 2); quota != 15000 || used != 5000 {
		t.Fatalf("virtual balance: %d/%d", quota, used)
	}
	// Installation still uses the ordinary transport factory after materialization.
	ref.TransportType = "virtual"
	if bridge.CreateAdapter(ref) == nil {
		t.Fatal("virtual transport missing")
	}
}
