package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/internal/mcp/audiogen"
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

func TestAudioGenAPIAndMCPWorkflow(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audio.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&model.Option{}, &model.User{}, &model.ApiKey{}, &model.McpCallLog{}, &model.AudioGenConfig{}, &model.AudioGenConnection{}, &model.AudioGenModel{}, &model.AudioGenTask{}, &model.AudioAsset{}, &model.UploadedImage{}, &model.McpService{}, &model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}, &model.SmartSearchConfig{}); e != nil {
		t.Fatal(e)
	}
	oldDB, oldRegistry, oldSvcRegistry, oldPool, oldSvcPool, oldHandler, oldStore, oldVStore, oldManager := model.DB, VirtualRegistry, service.VirtualRegistry, SessionPool, service.SessionPool, gatewayHandler, service.UploadStore, virtual.UploadStore, virtual.AudioManager
	model.DB = db
	model.InitOptionMap()
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	service.VirtualRegistry = VirtualRegistry
	pool := bridge.NewSessionPool()
	SessionPool, service.SessionPool = pool, pool
	store, e := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	service.UploadStore, virtual.UploadStore = store, store
	manager := audiogen.NewManager(db, store)
	virtual.AudioManager = manager
	manager.Start(context.Background())
	gateway := handler.NewGatewayHandler(pool, bridge.NewToolRouter(pool), VirtualRegistry)
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	SetApiRouter(engine)
	SetMCPRouter(engine, gateway)
	f := &protocol2026Fixture{db: db, server: httptest.NewServer(engine)}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() {
		manager.Close()
		gateway.Close()
		pool.CloseAll()
		f.server.Close()
		sql.Close()
		model.DB, VirtualRegistry, service.VirtualRegistry, SessionPool, service.SessionPool, gatewayHandler, service.UploadStore, virtual.UploadStore, virtual.AudioManager = oldDB, oldRegistry, oldSvcRegistry, oldPool, oldSvcPool, oldHandler, oldStore, oldVStore, oldManager
	})
	model.UpdateOption("ServerAddress", f.server.URL)
	owner := model.User{ID: 1, Username: "audio-owner", Status: 1, Group: "default", AffCode: "audio-owner"}
	other := model.User{ID: 2, Username: "audio-other", Status: 1, Group: "default", AffCode: "audio-other"}
	db.Create(&owner)
	db.Create(&other)
	token, _ := middleware.GenerateToken(&owner)
	otherToken, _ := middleware.GenerateToken(&other)
	f.key = f.addKey(t, "audios", `{"groups":["audios"]}`)
	blocked := f.addKey(t, "blocked-audio", `{"groups":["different-group"]}`)
	wav, _ := audioasset.WAV([]byte{0, 0, 1, 0, 2, 0, 3, 0}, 24000, 1, 16)
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream-audio-secret" {
			t.Error("missing upstream key")
		}
		if r.URL.Path == "/v1/models" {
			io.WriteString(w, `{"data":[{"id":"tts-test"}]}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/audio/speech" {
			t.Errorf("OpenAI speech did not use the resolved adapter: %s %s", r.Method, r.URL.Path)
		}
		posts.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(wav)
	}))
	defer upstream.Close()
	request := func(method, path, key string, body any) (int, map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		r, _ := http.NewRequest(method, f.server.URL+"/api/v1"+path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		resp, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		b, _ = io.ReadAll(resp.Body)
		if bytes.Contains(b, []byte("upstream-audio-secret")) {
			t.Fatal("key leaked")
		}
		var reply map[string]any
		_ = json.Unmarshal(b, &reply)
		return resp.StatusCode, reply
	}
	body := map[string]any{"name": "Audio model", "connections": []any{map[string]any{"key": "main", "protocol": "openai", "base_url": upstream.URL + "/v1", "api_key": "upstream-audio-secret"}}, "models": []any{map[string]any{"kind": "speech", "enabled": true, "connection_key": "main", "model": "tts-test"}, map[string]any{"kind": "audio", "enabled": true, "connection_key": "main", "model": "gpt-audio"}}}
	code, reply := request("POST", "/audiogen", token, body)
	if code != 201 {
		t.Fatal(code, reply)
	}
	id := int64(reply["data"].(map[string]any)["id"].(float64))
	path := fmt.Sprintf("/audiogen/%d", id)
	if code, _ = request("GET", path, otherToken, nil); code != 404 {
		t.Fatal("cross-user config access")
	}
	if code, reply = request("POST", path+"/enable", token, nil); code != 200 {
		t.Fatal(code, reply)
	}
	cfg, _ := model.GetAudioGenConfig(1, id)
	sid := *cfg.RegisteredServiceID
	group := model.McpGroup{UserID: 1, Name: "audios", EndpointSlug: "audios", Status: 1, ExposeMode: "direct"}
	db.Create(&group)
	if e = model.AddServicesToGroup(group.ID, []int64{sid}); e != nil {
		t.Fatal(e)
	}
	VirtualRegistry.Unregister(sid)
	loadVirtualServices()
	if !VirtualRegistry.IsVirtual(sid) {
		t.Fatal("audio handler not restored")
	}
	call := func(key, name string, args map[string]any) map[string]any {
		t.Helper()
		r := f.httpRequest(t, key, "/mcp", protocol2026Request(1, "tools/call", map[string]any{"name": name, "arguments": args}))
		r.Header.Set("Mcp-Name", name)
		_, _, reply := protocol2026RPC(t, r)
		return reply
	}
	tool := fmt.Sprintf("audiogen_%d__generate_speech", id)
	reply = call(f.key, tool, map[string]any{"text": "hello", "wait_seconds": 0, "request_id": "stable"})
	if reply["error"] != nil {
		t.Fatal(reply)
	}
	result := reply["result"].(map[string]any)
	taskID := result["structuredContent"].(map[string]any)["task_id"].(string)
	reply = call(f.key, tool, map[string]any{"text": "hello", "wait_seconds": 0, "request_id": "stable"})
	if reply["error"] != nil {
		t.Fatal(reply)
	}
	var task *model.AudioGenTask
	until := time.Now().Add(8 * time.Second)
	for time.Now().Before(until) {
		task, e = manager.Get(1, nil, taskID)
		if e == nil && task.ArtifactStatus == "ready" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if task == nil || task.ArtifactStatus != "ready" {
		t.Fatal(task)
	}
	if posts.Load() != 1 {
		t.Fatal("idempotent call regenerated", posts.Load())
	}
	if code, _ = request("GET", "/audiogen/tasks/"+taskID, otherToken, nil); code != 404 {
		t.Fatal("cross-user task access")
	}
	var asset model.AudioAsset
	if db.Where("task_id = ? AND source = ?", taskID, "generated").First(&asset).Error != nil {
		t.Fatal("missing asset")
	}
	fileURL := audioasset.URL("GET", asset.ShortID)
	r, _ := http.NewRequest("GET", fileURL, nil)
	r.Header.Set("Range", "bytes=0-3")
	resp, e := http.DefaultClient.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || string(b) != "RIFF" {
		t.Fatal("audio range", resp.StatusCode, string(b))
	}
	uploadTool := fmt.Sprintf("audiogen_%d__upload_audio", id)
	reply = call(f.key, uploadTool, map[string]any{"local_path": "C:\\audio\\input.wav"})
	if reply["error"] != nil {
		t.Fatal(reply)
	}
	var pending model.AudioAsset
	if db.Where("source = ? AND status = ?", "upload", "pending").First(&pending).Error != nil {
		t.Fatal("upload slot missing")
	}
	put, _ := http.NewRequest("PUT", audioasset.URL("PUT", pending.ShortID), bytes.NewReader(wav))
	resp, e = http.DefaultClient.Do(put)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatal("signed upload failed", resp.StatusCode)
	}
	put, _ = http.NewRequest("PUT", audioasset.URL("GET", pending.ShortID), bytes.NewReader(wav))
	resp, e = http.DefaultClient.Do(put)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("read token authorized upload")
	}
	put, _ = http.NewRequest("PUT", audioasset.URL("PUT", pending.ShortID), bytes.NewReader(wav))
	resp, e = http.DefaultClient.Do(put)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatal("overwrote completed upload")
	}
	if reply = call(blocked, tool, map[string]any{"text": "blocked"}); reply["error"] == nil {
		if r, ok := reply["result"].(map[string]any); !ok || r["isError"] != true {
			t.Fatal("group permission bypass", reply)
		}
	}
	if code, reply = request("DELETE", path, token, nil); code != 200 {
		t.Fatal(code, reply)
	}
	if code, reply = request("GET", "/audiogen/tasks/"+taskID, token, nil); code != 200 {
		t.Fatal("history removed with config", reply)
	}
	if _, e = manager.Get(1, nil, taskID); e != nil {
		t.Fatal(e)
	}
	if VirtualRegistry.IsVirtual(sid) {
		t.Fatal("deleted registry entry remains")
	}
	if code, reply = request("DELETE", "/audiogen/tasks/"+taskID, token, nil); code != 200 {
		t.Fatal(code, reply)
	}
	resp, e = http.Get(fileURL)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatal("deleted file still available")
	}
}
