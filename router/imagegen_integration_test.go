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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/middleware"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestImageGenAPIAndMCPWorkflow(t *testing.T) {
	for _, provider := range []string{"openai", "openrouter"} {
		t.Run(provider, func(t *testing.T) { testImageGenAPIAndMCPWorkflow(t, provider) })
	}
}

func testImageGenAPIAndMCPWorkflow(t *testing.T, provider string) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "imagegen.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.ApiKey{}, &model.McpCallLog{}, &model.ImageGenConfig{}, &model.UploadedImage{}, &model.McpService{}, &model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}, &model.SmartSearchConfig{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldRegistry, oldSvcRegistry, oldPool, oldSvcPool, oldHandler, oldStore, oldVStore := model.DB, VirtualRegistry, service.VirtualRegistry, SessionPool, service.SessionPool, gatewayHandler, service.UploadStore, virtual.UploadStore
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
	gateway := handler.NewGatewayHandler(pool, bridge.NewToolRouter(pool), VirtualRegistry)
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	SetApiRouter(engine)
	SetMCPRouter(engine, gateway)
	f := &protocol2026Fixture{db: db, server: httptest.NewServer(engine)}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		gateway.Close()
		pool.CloseAll()
		f.server.Close()
		sqlDB.Close()
		model.DB, VirtualRegistry, service.VirtualRegistry, SessionPool, service.SessionPool, gatewayHandler, service.UploadStore, virtual.UploadStore = oldDB, oldRegistry, oldSvcRegistry, oldPool, oldSvcPool, oldHandler, oldStore, oldVStore
	})
	model.UpdateOption("ServerAddress", f.server.URL)
	owner := model.User{ID: 1, Username: "image-owner", Status: 1, Group: "default", AffCode: "owner"}
	other := model.User{ID: 2, Username: "image-other", Status: 1, Group: "default", AffCode: "other"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	token, _ := middleware.GenerateToken(&owner)
	otherToken, _ := middleware.GenerateToken(&other)
	f.key = f.addKey(t, "images", `{"groups":["images"]}`)
	blockedKey := f.addKey(t, "blocked", `{"groups":["different-group"]}`)
	var payload bytes.Buffer
	png.Encode(&payload, image.NewNRGBA(image.Rect(0, 0, 3, 2)))
	imageBytes := payload.Bytes()
	modelPath, generationPath := "/v1/models", "/v1/images/generations"
	if provider == "openrouter" {
		modelPath, generationPath = "/proxy/api/v1/images/models", "/proxy/api/v1/images"
	}
	var generationCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Error("bad upstream key")
		}
		if r.URL.Path == modelPath {
			fmt.Fprint(w, `{"data":[{"id":"qwen-image-edit"},{"id":"unfiltered-chat-model"}]}`)
			return
		}
		generationCalls.Add(1)
		if r.URL.Path == generationPath {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if provider == "openrouter" && body["prompt"] == "edit" {
				refs, ok := body["input_references"].([]any)
				if !ok || len(refs) != 1 || refs[0].(map[string]any)["image_url"].(map[string]any)["url"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes) {
					t.Error("stored reference was not validated and sent through the OpenRouter protocol")
				}
			}
			if body["prompt"] == "slow" {
				<-r.Context().Done()
				return
			}
		} else {
			t.Errorf("unexpected image API route: %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString(imageBytes))
	}))
	defer upstream.Close()
	apiRequest := func(method, path, bearer string, body any) (int, map[string]any) {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, f.server.URL+"/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ = io.ReadAll(resp.Body)
		if strings.Contains(string(raw), "upstream-secret") {
			t.Fatalf("API key leaked: %s", raw)
		}
		var result map[string]any
		json.Unmarshal(raw, &result)
		return resp.StatusCode, result
	}
	endpoint := upstream.URL + strings.TrimSuffix(modelPath, "/models")
	if provider == "openrouter" {
		endpoint = strings.TrimSuffix(endpoint, "/images")
	}
	connection := map[string]any{"provider": provider, "endpoint_url": endpoint, "api_key": "upstream-secret", "default_params": map[string]any{"not-a-generation-parameter": true}, "timeout_seconds": -1}
	code, reply := apiRequest("POST", "/imagegen/models", token, connection)
	if code != 200 || len(reply["data"].([]any)) != 2 || generationCalls.Load() != 0 {
		t.Fatal("model discovery required unrelated generation fields", reply)
	}
	code, reply = apiRequest("POST", "/imagegen", token, map[string]any{"name": "Image model", "provider": provider, "model_name": "qwen-image-edit", "endpoint_url": endpoint, "api_key": "upstream-secret"})
	if code != 201 {
		t.Fatalf("create %d %v", code, reply)
	}
	cfg := reply["data"].(map[string]any)
	id := int64(cfg["id"].(float64))
	path := fmt.Sprintf("/imagegen/%d", id)
	if cfg["auto_register"] != false {
		t.Fatal("new config auto-registered")
	}
	if code, response := apiRequest("GET", path, otherToken, nil); code != 404 {
		t.Fatalf("cross-user API detail: %d %v", code, response)
	}
	code, reply = apiRequest("POST", "/imagegen/models", token, map[string]any{"config_id": id})
	if code != 200 || len(reply["data"].([]any)) != 2 || generationCalls.Load() != 0 {
		t.Fatal("model list filtered or generated", reply)
	}
	code, reply = apiRequest("POST", "/imagegen/test", token, map[string]any{"config_id": id, "arguments": map[string]any{"prompt": "draw"}})
	if code != 200 || reply["data"].(map[string]any)["is_error"] == true {
		t.Fatal("preview", reply)
	}
	for range 2 {
		if code, _ := apiRequest("POST", path+"/enable", token, nil); code != 200 {
			t.Fatal("enable")
		}
	}
	stored, _ := model.GetImageGenConfig(1, id)
	sid := *stored.RegisteredServiceID
	group := model.McpGroup{UserID: 1, Name: "images", EndpointSlug: "images", Status: 1, ExposeMode: "direct"}
	db.Create(&group)
	if err := model.AddServicesToGroup(group.ID, []int64{sid}); err != nil {
		t.Fatal(err)
	}
	// Simulate process restart: clear the runtime entry and restore from persisted
	// virtual service references, then rebuild the tools cache.
	VirtualRegistry.Unregister(sid)
	loadVirtualServices()
	if !VirtualRegistry.IsVirtual(sid) {
		t.Fatal("startup did not restore image handler")
	}
	(&service.ImageGenService{}).SyncAllRegisteredTools()
	toolID := fmt.Sprintf("imagegen_%d__generate_image", id)
	call := func(key, path, name string, args map[string]any) map[string]any {
		t.Helper()
		req := f.httpRequest(t, key, path, protocol2026Request(1, "tools/call", map[string]any{"name": name, "arguments": args}))
		req.Header.Set("Mcp-Name", name)
		_, _, reply := protocol2026RPC(t, req)
		return reply
	}
	reply = call(f.key, "/mcp", toolID, map[string]any{"prompt": "draw"})
	if reply["error"] != nil {
		t.Fatal("direct call", reply)
	}
	result := reply["result"].(map[string]any)
	images := result["structuredContent"].(map[string]any)["images"].([]any)
	url := images[0].(map[string]any)["url"].(string)
	downloaded, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(downloaded.Body)
	downloaded.Body.Close()
	if downloaded.StatusCode != 200 || !bytes.Equal(got, imageBytes) {
		t.Fatal("download")
	}
	editID := fmt.Sprintf("imagegen_%d__edit_image", id)
	reply = call(f.key, "/mcp/group/images", editID, map[string]any{"prompt": "edit", "images": []string{url}})
	if reply["error"] != nil {
		t.Fatal("reference edit", reply)
	}
	reply = call(f.key, "/smart/mcp", "mcp.execute", map[string]any{"tool_id": toolID, "arguments": map[string]any{"prompt": "draw"}})
	if reply["error"] != nil {
		t.Fatal("smart execute", reply)
	}
	reply = call(f.key, "/smart/mcp", "mcp.execute_batch", map[string]any{"calls": []any{map[string]any{"tool_id": toolID, "arguments": map[string]any{"prompt": "draw"}}}})
	if reply["error"] != nil || reply["result"].(map[string]any)["isError"] == true {
		t.Fatal("batch execute", reply)
	}
	before := generationCalls.Load()
	reply = call(blockedKey, "/mcp", toolID, map[string]any{"prompt": "draw"})
	if reply["error"] == nil || generationCalls.Load() != before {
		t.Fatal("direct scope bypass")
	}
	filter := model.McpGroupTool{GroupID: group.ID, ServiceID: sid, ToolName: "generate_image", Enabled: false}
	db.Create(&filter)
	for _, mode := range []string{"direct", "smart", "batch"} {
		switch mode {
		case "direct":
			reply = call(f.key, "/mcp", toolID, map[string]any{"prompt": "draw"})
		case "smart":
			reply = call(f.key, "/smart/mcp", "mcp.execute", map[string]any{"tool_id": toolID, "arguments": map[string]any{"prompt": "draw"}})
		case "batch":
			reply = call(f.key, "/smart/mcp", "mcp.execute_batch", map[string]any{"tool_id": toolID, "arguments_list": []any{map[string]any{"prompt": "draw"}}})
		}
		if reply["error"] == nil && reply["result"].(map[string]any)["isError"] != true {
			t.Fatal("disabled tool bypass via "+mode, reply)
		}
	}
	if generationCalls.Load() != before {
		t.Fatal("disabled tool reached upstream")
	}
	db.Delete(&filter)
	reply = call(f.key, "/smart/mcp", "mcp.execute", map[string]any{"tool_id": toolID, "arguments": map[string]any{"prompt": "slow"}, "timeout_ms": 50})
	if reply["error"] == nil || !strings.Contains(fmt.Sprint(reply), "deadline") {
		t.Fatal("explicit execute timeout", reply)
	}
	reply = call(f.key, "/smart/mcp", "mcp.execute_batch", map[string]any{"tool_id": toolID, "arguments_list": []any{map[string]any{"prompt": "slow"}}, "timeout_ms": 50})
	if reply["result"].(map[string]any)["isError"] != true || !strings.Contains(fmt.Sprint(reply), "deadline") {
		t.Fatal("batch timeout", reply)
	}
	if code, _ := apiRequest("POST", path+"/disable", token, nil); code != 200 {
		t.Fatal("disable")
	}
	if VirtualRegistry.IsVirtual(sid) {
		t.Fatal("disabled handler retained")
	}
	if code, _ := apiRequest("DELETE", path, token, nil); code != 200 {
		t.Fatal("delete")
	}
}
