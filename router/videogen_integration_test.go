package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/videoasset"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVideoGenAPIAndMCPWorkflow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "video.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.ApiKey{}, &model.McpCallLog{}, &model.VideoGenConfig{}, &model.VideoGenTask{}, &model.UploadedImage{}, &model.McpService{}, &model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}, &model.SmartSearchConfig{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldRegistry, oldSvcRegistry, oldPool, oldSvcPool, oldHandler, oldStore, oldVStore, oldManager := model.DB, VirtualRegistry, service.VirtualRegistry, SessionPool, service.SessionPool, gatewayHandler, service.UploadStore, virtual.UploadStore, virtual.VideoManager
	model.DB = db
	model.InitOptionMap()
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	service.VirtualRegistry = VirtualRegistry
	pool := bridge.NewSessionPool()
	SessionPool, service.SessionPool = pool, pool
	store, err := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	service.UploadStore, virtual.UploadStore = store, store
	manager := videogen.NewManager(db, store)
	manager.PollInterval = time.Millisecond
	virtual.VideoManager = manager
	gateway := handler.NewGatewayHandler(pool, bridge.NewToolRouter(pool), VirtualRegistry)
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	SetApiRouter(engine)
	SetMCPRouter(engine, gateway)
	f := &protocol2026Fixture{db: db, server: httptest.NewServer(engine)}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		virtual.VideoManager.Close()
		manager.Close()
		gateway.Close()
		pool.CloseAll()
		f.server.Close()
		sqlDB.Close()
		model.DB, VirtualRegistry, service.VirtualRegistry, SessionPool, service.SessionPool, gatewayHandler, service.UploadStore, virtual.UploadStore, virtual.VideoManager = oldDB, oldRegistry, oldSvcRegistry, oldPool, oldSvcPool, oldHandler, oldStore, oldVStore, oldManager
	})
	model.UpdateOption("ServerAddress", f.server.URL)
	owner := model.User{ID: 1, Username: "video-owner", Status: 1, Group: "default", AffCode: "owner"}
	other := model.User{ID: 2, Username: "video-other", Status: 1, Group: "default", AffCode: "other"}
	db.Create(&owner)
	db.Create(&other)
	token, _ := middleware.GenerateToken(&owner)
	otherToken, _ := middleware.GenerateToken(&other)
	f.key = f.addKey(t, "videos", `{"groups":["videos"]}`)
	blocked := f.addKey(t, "blocked-video", `{"groups":["different-group"]}`)
	var posts, images atomic.Int32
	videoBytes := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '2'}, make([]byte, 64)...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-video-secret" {
			t.Error("missing upstream auth")
		}
		if r.URL.Path == "/v1/models" {
			io.WriteString(w, `{"data":[{"id":"sora-2"},{"id":"arbitrary-chat-model"}]}`)
			return
		}
		if r.Method == "POST" {
			posts.Add(1)
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				file, _, err := r.FormFile("input_reference")
				if err != nil {
					t.Error(err)
				} else {
					images.Add(1)
					file.Close()
				}
				r.MultipartForm.RemoveAll()
			}
			fmt.Fprintf(w, `{"id":"public-video-%d","status":"queued"}`, posts.Load())
			return
		}
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.Header().Set("Content-Type", "video/mp4")
			w.Write(videoBytes)
			return
		}
		io.WriteString(w, `{"status":"completed","progress":100}`)
	}))
	defer upstream.Close()
	apiRequest := func(method, path, key string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, f.server.URL+"/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ = io.ReadAll(resp.Body)
		if strings.Contains(string(raw), "upstream-video-secret") {
			t.Fatal("API key leaked")
		}
		var reply map[string]any
		json.Unmarshal(raw, &reply)
		return resp.StatusCode, reply
	}
	for _, test := range []struct {
		name string
		body map[string]any
	}{
		{"connection only", map[string]any{"endpoint_url": upstream.URL, "api_key": "upstream-video-secret"}},
		{"blank name and model", map[string]any{"name": "", "model_name": "", "endpoint_url": upstream.URL + "/v1", "api_key": "upstream-video-secret"}},
		{"whitespace name and model", map[string]any{"name": "  ", "model_name": "  ", "endpoint_url": upstream.URL + "/v1/", "api_key": "upstream-video-secret"}},
		{"incomplete generation settings", map[string]any{"endpoint_url": upstream.URL, "api_key": "upstream-video-secret", "default_params": []any{}, "task_timeout_seconds": 0}},
	} {
		t.Run("draft model discovery/"+test.name, func(t *testing.T) {
			code, reply := apiRequest("POST", "/videogen/models", token, test.body)
			if code != 200 {
				t.Fatalf("draft model discovery %d: %v", code, reply)
			}
			if len(reply["data"].([]any)) != 2 || posts.Load() != 0 {
				t.Fatal("model discovery must return the catalog without submitting generation", reply)
			}
		})
	}
	var configCount int64
	if err := db.Model(&model.VideoGenConfig{}).Count(&configCount).Error; err != nil || configCount != 0 {
		t.Fatalf("draft model discovery persisted configuration: count=%d err=%v", configCount, err)
	}
	code, reply := apiRequest("POST", "/videogen", token, map[string]any{"name": "Video model", "model_name": "sora-2", "endpoint_url": upstream.URL + "/v1", "api_key": "upstream-video-secret"})
	if code != 201 {
		t.Fatalf("create %d %v", code, reply)
	}
	config := reply["data"].(map[string]any)
	id := int64(config["id"].(float64))
	path := fmt.Sprintf("/videogen/%d", id)
	if code, _ := apiRequest("GET", path, otherToken, nil); code != 404 {
		t.Fatal("cross-user configuration access")
	}
	code, reply = apiRequest("POST", "/videogen/models", token, map[string]any{"config_id": id})
	if code != 200 || len(reply["data"].([]any)) != 2 || posts.Load() != 0 {
		t.Fatal("model discovery", reply)
	}
	code, reply = apiRequest("POST", "/videogen/models", token, map[string]any{"config_id": id, "name": "", "model_name": "", "api_key": ""})
	if code != 200 || len(reply["data"].([]any)) != 2 {
		t.Fatal("saved model discovery must retain the saved key and allow blank generation fields", reply)
	}
	if code, _ := apiRequest("POST", "/videogen/models", otherToken, map[string]any{"config_id": id}); code != 400 {
		t.Fatal("cross-user model discovery allowed")
	}
	for _, test := range []struct {
		path string
		body map[string]any
	}{
		{"/videogen/models", map[string]any{"endpoint_url": upstream.URL}},
		{"/videogen/models", map[string]any{"endpoint_url": "ftp://invalid.example", "api_key": "upstream-video-secret"}},
		{"/videogen/models", map[string]any{"endpoint_url": upstream.URL, "api_key": "upstream-video-secret", "protocol": "invalid"}},
		{"/videogen", map[string]any{"name": "", "model_name": "", "endpoint_url": upstream.URL, "api_key": "upstream-video-secret"}},
		{"/videogen/test", map[string]any{"model_name": "", "endpoint_url": upstream.URL, "api_key": "upstream-video-secret"}},
	} {
		if code, reply := apiRequest("POST", test.path, token, test.body); code != 400 {
			t.Fatalf("invalid request accepted at %s: %d %v", test.path, code, reply)
		}
	}
	saved, err := model.GetVideoGenConfig(1, id)
	if err != nil || saved.Name != "Video model" || saved.ModelName != "sora-2" {
		t.Fatalf("model discovery changed the saved configuration: %+v err=%v", saved, err)
	}
	for range 2 {
		if code, _ := apiRequest("POST", path+"/enable", token, nil); code != 200 {
			t.Fatal("enable")
		}
	}
	cfg, _ := model.GetVideoGenConfig(1, id)
	serviceID := *cfg.RegisteredServiceID
	group := model.McpGroup{UserID: 1, Name: "videos", EndpointSlug: "videos", Status: 1, ExposeMode: "direct"}
	db.Create(&group)
	if err := model.AddServicesToGroup(group.ID, []int64{serviceID}); err != nil {
		t.Fatal(err)
	}
	VirtualRegistry.Unregister(serviceID)
	loadVirtualServices()
	if !VirtualRegistry.IsVirtual(serviceID) {
		t.Fatal("handler not restored")
	}
	(&service.VideoGenService{}).SyncAllRegisteredTools()
	call := func(key, path, name string, args map[string]any) map[string]any {
		t.Helper()
		req := f.httpRequest(t, key, path, protocol2026Request(1, "tools/call", map[string]any{"name": name, "arguments": args}))
		req.Header.Set("Mcp-Name", name)
		_, _, reply := protocol2026RPC(t, req)
		return reply
	}
	toolID := fmt.Sprintf("videogen_%d__generate_video", id)
	reply = call(f.key, "/mcp", toolID, map[string]any{"prompt": "scene", "wait_seconds": 0, "request_id": "stable"})
	if reply["error"] != nil {
		t.Fatal("MCP submit", reply)
	}
	job := reply["result"].(map[string]any)["structuredContent"].(map[string]any)
	taskID := job["task_id"].(string)
	// Update and disable the config while the task runs: the private connection
	// snapshot and the task API must remain usable.
	if code, _ := apiRequest("PUT", path, token, map[string]any{"endpoint_url": "https://invalid.example", "api_key": ""}); code != 200 {
		t.Fatal("update")
	}
	if code, _ := apiRequest("POST", path+"/disable", token, nil); code != 200 || VirtualRegistry.IsVirtual(serviceID) {
		t.Fatal("disable")
	}
	if code, _ := apiRequest("DELETE", "/videogen/tasks/"+taskID, token, nil); code != 409 {
		t.Fatal("active task deletion allowed")
	}
	manager.Close()
	restarted := videogen.NewManager(db, store)
	restarted.PollInterval = time.Millisecond
	virtual.VideoManager = restarted
	deadline := time.Now().Add(4 * time.Second)
	var row *model.VideoGenTask
	for time.Now().Before(deadline) {
		restarted.ProcessDue()
		row, _ = restarted.Get(1, nil, taskID, false)
		if row.ArtifactStatus == "ready" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if row.ArtifactStatus != "ready" {
		t.Fatalf("recovery failed %+v", row)
	}
	if code, _ := apiRequest("GET", "/videogen/tasks/"+taskID, otherToken, nil); code != 404 {
		t.Fatal("cross-user task access")
	}
	url := videoasset.URL(*row.ShortID)
	for _, test := range []struct {
		method, rangeHeader string
		status              int
		bytes               int
		download            bool
	}{{"GET", "", 200, len(videoBytes), false}, {"HEAD", "", 200, 0, false}, {"GET", "bytes=0-7", 206, 8, false}, {"GET", "bytes=999999-", 416, -1, false}, {"GET", "", 200, len(videoBytes), true}, {"HEAD", "", 200, 0, true}, {"GET", "bytes=0-7", 206, 8, true}} {
		requestURL := url
		if test.download {
			requestURL += "&download=1"
		}
		req, _ := http.NewRequest(test.method, requestURL, nil)
		if test.rangeHeader != "" {
			req.Header.Set("Range", test.rangeHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != test.status || (test.bytes >= 0 && len(body) != test.bytes) {
			t.Fatalf("video %s %s: %d (%d bytes)", test.method, test.rangeHeader, resp.StatusCode, len(body))
		}
		if test.download {
			disposition, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
			if err != nil || disposition != "attachment" || params["filename"] != "generated-video-"+taskID+".mp4" {
				t.Fatal("video download must use an attachment filename", resp.Header.Get("Content-Disposition"), err)
			}
		} else if resp.Header.Get("Content-Disposition") != "" {
			t.Fatal("playback must not force an attachment")
		}
	}
	resp, err := http.Get(strings.Split(url, "?s=")[0] + "?s=invalid")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("bad video signature accepted")
	}
	if code, _ := apiRequest("DELETE", path, token, nil); code != 200 {
		t.Fatal("delete config")
	}
	code, reply = apiRequest("POST", "/videogen/test", token, map[string]any{"model_name": "custom-video", "endpoint_url": upstream.URL, "api_key": "upstream-video-secret", "tool": "generate_video_from_image", "arguments": map[string]any{"prompt": "motion", "image": func() string {
		var b bytes.Buffer
		png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
		return base64.StdEncoding.EncodeToString(b.Bytes())
	}(), "wait_seconds": 0}})
	if code != 200 || reply["data"].(map[string]any)["is_error"] == true {
		t.Fatal("draft image preview", reply)
	}
	draftTask := reply["data"].(map[string]any)["result"].(map[string]any)["structuredContent"].(map[string]any)["task_id"].(string)
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		restarted.ProcessDue()
		row, _ = restarted.Get(1, nil, draftTask, false)
		if row.ArtifactStatus == "ready" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if row.ArtifactStatus != "ready" || images.Load() != 1 {
		t.Fatal("draft image generation failed")
	}
	// Scope is enforced before any upstream submission even after re-registering.
	code, reply = apiRequest("POST", "/videogen", token, map[string]any{"name": "Scoped", "model_name": "sora-2", "endpoint_url": upstream.URL, "api_key": "upstream-video-secret"})
	scopedID := int64(reply["data"].(map[string]any)["id"].(float64))
	apiRequest("POST", fmt.Sprintf("/videogen/%d/enable", scopedID), token, nil)
	scopedConfig, _ := model.GetVideoGenConfig(1, scopedID)
	if err := model.AddServicesToGroup(group.ID, []int64{*scopedConfig.RegisteredServiceID}); err != nil {
		t.Fatal(err)
	}
	before := posts.Load()
	reply = call(blocked, "/mcp", fmt.Sprintf("videogen_%d__generate_video", scopedID), map[string]any{"prompt": "blocked", "wait_seconds": 0})
	if reply["error"] == nil || posts.Load() != before {
		t.Fatalf("MCP scope bypass: before=%d after=%d reply=%v", before, posts.Load(), reply)
	}
	filter := model.McpGroupTool{GroupID: group.ID, ServiceID: *scopedConfig.RegisteredServiceID, ToolName: "generate_video", Enabled: false}
	if err := db.Create(&filter).Error; err != nil {
		t.Fatal(err)
	}
	scopedTool := fmt.Sprintf("videogen_%d__generate_video", scopedID)
	for _, mode := range []string{"direct", "smart", "batch"} {
		args := map[string]any{"prompt": "disabled", "wait_seconds": 0}
		switch mode {
		case "direct":
			reply = call(f.key, "/mcp/group/videos", scopedTool, args)
		case "smart":
			reply = call(f.key, "/smart/mcp", "mcp.execute", map[string]any{"tool_id": scopedTool, "arguments": args})
		case "batch":
			reply = call(f.key, "/smart/mcp", "mcp.execute_batch", map[string]any{"calls": []any{map[string]any{"tool_id": scopedTool, "arguments": args}}})
		}
		if reply["error"] == nil && reply["result"].(map[string]any)["isError"] != true {
			t.Fatalf("disabled video tool bypass via %s: %v", mode, reply)
		}
	}
	if posts.Load() != before {
		t.Fatal("disabled video tool reached upstream")
	}
	legacyFailed := model.VideoGenTask{ID: "retry-api-parent", UserID: 1, ConfigID: scopedID, ConfigName: "Scoped", Model: "sora-2", Prompt: "retry scene", Operation: "generate", Status: "failed", ArtifactStatus: "pending", Error: "mode is required", IdempotencyKey: "retry-api-parent"}
	if err := db.Create(&legacyFailed).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := apiRequest("GET", "/videogen/tasks/"+legacyFailed.ID+"/retry-input", otherToken, nil); code != 400 {
		t.Fatal("foreign retry inputs")
	}
	code, reply = apiRequest("GET", "/videogen/tasks/"+legacyFailed.ID+"/retry-input", token, nil)
	if code != 200 || reply["data"].(map[string]any)["has_input_snapshot"] != false {
		t.Fatal("legacy retry input", reply)
	}
	if code, _ := apiRequest("POST", "/videogen/tasks/"+legacyFailed.ID+"/retry", otherToken, map[string]any{"config_id": scopedID, "request_id": "foreign-parent"}); code != 400 {
		t.Fatal("foreign task retry allowed")
	}
	draftFailed := legacyFailed
	draftFailed.ID, draftFailed.IdempotencyKey, draftFailed.ConfigID = "retry-draft-parent", "retry-draft-parent", 0
	if err := db.Create(&draftFailed).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := apiRequest("POST", "/videogen/tasks/"+draftFailed.ID+"/retry", token, map[string]any{"request_id": "missing-draft-config"}); code != 400 {
		t.Fatal("draft retried without saved configuration")
	}
	if code, reply := apiRequest("POST", "/videogen/tasks/"+draftFailed.ID+"/retry", token, map[string]any{"config_id": scopedID, "request_id": "selected-draft-config"}); code != 200 || reply["data"].(map[string]any)["task_id"] != draftFailed.ID || reply["data"].(map[string]any)["config_id"] != float64(scopedID) {
		t.Fatal("draft retry with saved configuration", reply)
	}
	foreignConfig := *scopedConfig
	foreignConfig.ID = 0
	foreignConfig.UserID = 2
	foreignConfig.RegisteredServiceID = nil
	if err := db.Create(&foreignConfig).Error; err != nil {
		t.Fatal(err)
	}
	for _, request := range []map[string]any{{"config_id": foreignConfig.ID, "request_id": "foreign"}, {"request_id": "bad-body", "arguments": []any{}}, {"request_id": "bad-input", "arguments": map[string]any{"extra_body": map[string]any{"model": "override"}}}, {"request_id": "local-control", "arguments": map[string]any{"request_id": "override"}}, {"request_id": ""}} {
		if code, _ := apiRequest("POST", "/videogen/tasks/"+legacyFailed.ID+"/retry", token, request); code != 400 {
			t.Fatal("invalid retry accepted", request)
		}
	}
	request := map[string]any{"request_id": "ui-retry", "arguments": map[string]any{"extra_body": map[string]any{"mode": "text"}}}
	var beforeRetryCount int64
	db.Model(&model.VideoGenTask{}).Count(&beforeRetryCount)
	code, reply = apiRequest("POST", "/videogen/tasks/"+legacyFailed.ID+"/retry", token, request)
	if code != 200 {
		t.Fatal("retry submit", code, reply)
	}
	newTask := reply["data"].(map[string]any)["task_id"].(string)
	if newTask != legacyFailed.ID || reply["data"].(map[string]any)["retry_count"] != float64(1) {
		t.Fatal("retry changed task identity", reply)
	}
	code, repeated := apiRequest("POST", "/videogen/tasks/"+legacyFailed.ID+"/retry", token, request)
	if code != 200 || repeated["data"].(map[string]any)["task_id"] != newTask {
		t.Fatal("retry duplicate", repeated)
	}
	var afterRetryCount int64
	db.Model(&model.VideoGenTask{}).Count(&afterRetryCount)
	if afterRetryCount != beforeRetryCount {
		t.Fatal("retry API grew task history")
	}
	if code, _ := apiRequest("POST", "/videogen/tasks/"+taskID+"/retry", token, map[string]any{"config_id": scopedID, "request_id": "already-completed"}); code != 400 {
		t.Fatal("completed task was regenerated")
	}
	expired := time.Now().Add(-time.Second)
	db.Model(&model.VideoGenTask{}).Where("id = ?", taskID).Update("expires_at", expired)
	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 410 {
		t.Fatal("expired video was served")
	}
	restarted.Cleanup()
	if code, _ := apiRequest("DELETE", "/videogen/tasks/"+taskID, token, nil); code != 200 {
		t.Fatal("completed task deletion")
	}
}
