package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/imagegentask"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func imageGenTestSetup(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "imagegen.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	oldDB, oldRegistry, oldStore, oldVStore := model.DB, VirtualRegistry, UploadStore, virtual.UploadStore
	model.DB = db
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	if err := db.AutoMigrate(&model.ImageGenConfig{}, &model.ImageGenTask{}, &model.ImageGenTaskImage{}, &model.McpService{}, &model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}, &model.UploadedImage{}, &model.Option{}); err != nil {
		t.Fatal(err)
	}
	model.InitOptionMap()
	if err := model.UpdateOption("ServerAddress", "https://storage.example"); err != nil {
		t.Fatal(err)
	}
	UploadStore, err = storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	virtual.UploadStore = UploadStore
	oldManager := virtual.ImageManager
	virtual.ImageManager = imagegentask.NewManager(db, UploadStore)
	manager := virtual.ImageManager
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		manager.Close()
		virtual.ImageManager = oldManager
		_ = sqlDB.Close()
		model.DB, VirtualRegistry, UploadStore, virtual.UploadStore = oldDB, oldRegistry, oldStore, oldVStore
	})
	return db
}
func igPtr[T any](value T) *T { return &value }
func imageGenReq(endpoint string) *dto.ImageGenConfigReq {
	return &dto.ImageGenConfigReq{Name: igPtr("Image model"), ModelName: igPtr("custom-alias"), EndpointURL: igPtr(endpoint), ApiKey: igPtr("secret-image-key")}
}
func imageGenPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestImageGenConfigLifecycleAndIsolation(t *testing.T) {
	db := imageGenTestSetup(t)
	s := &ImageGenService{}
	detail, err := s.Create(1, imageGenReq("https://gateway.example/v1"))
	if err != nil {
		t.Fatal(err)
	}
	if detail.AutoRegister || detail.TimeoutSeconds != 180 || detail.Operation != "both" || detail.EditRequestFormat != imagegen.EditFormatGenerationsJSON || !detail.HasAPIKey {
		t.Fatalf("defaults: %+v", detail)
	}
	raw, _ := json.Marshal(detail)
	if strings.Contains(string(raw), "secret-image-key") || strings.Contains(string(raw), `"api_key"`) {
		t.Fatalf("leaked API key: %s", raw)
	}
	if _, err := s.Get(2, detail.ID); err == nil {
		t.Fatal("cross-user detail")
	}
	if err := s.Enable(2, detail.ID); err == nil {
		t.Fatal("cross-user enable")
	}
	if list, _ := s.List(2); len(list) != 0 {
		t.Fatal("cross-user list")
	}
	for range 2 {
		if err := s.Enable(1, detail.ID); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	db.Model(&model.McpService{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate service: %d", count)
	}
	cfg, _ := model.GetImageGenConfig(1, detail.ID)
	sid := *cfg.RegisteredServiceID
	svc, _ := model.GetServiceByID(1, sid)
	if svc.Source != "imagegen" || svc.TransportType != "virtual" || strings.Contains(svc.Config, "secret") || !VirtualRegistry.IsVirtual(sid) {
		t.Fatalf("registration: %+v", svc)
	}
	if _, _, ok := VirtualRegistry.LookupByName(2, svc.Name); ok {
		t.Fatal("cross-user registry")
	}
	if _, err := VirtualRegistry.Handle(virtual.WithCallerUserID(context.Background(), 2), sid, nil, "generate_image", json.RawMessage(`{"prompt":"x"}`)); err == nil {
		t.Fatal("cross-user tool")
	}
	if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{Operation: igPtr("generate"), GenerateImageDesc: igPtr("Custom generate description"), TimeoutSeconds: igPtr(0), ApiKey: igPtr("")}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = model.GetImageGenConfig(1, detail.ID)
	svc, _ = model.GetServiceByID(1, sid)
	if cfg.TimeoutSeconds != 0 || cfg.ApiKey != "secret-image-key" || cfg.EditRequestFormat != imagegen.EditFormatGenerationsJSON || strings.Contains(svc.ToolsCache, "edit_image") || !strings.Contains(svc.ToolsCache, "Custom generate description") {
		t.Fatal("update/key/zero timeout/cache not synchronized")
	}
	// Startup repair rebuilds old tool caches from the current configuration.
	db.Model(svc).Update("tools_cache", "[]")
	if s.SyncAllRegisteredTools() != 1 {
		t.Fatal("startup sync")
	}
	svc, _ = model.GetServiceByID(1, sid)
	if svc.ToolsCache == "[]" {
		t.Fatal("stale cache")
	}
	group := model.McpGroup{UserID: 1, Name: "images", EndpointSlug: "images"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.AddServicesToGroup(group.ID, []int64{sid}); err != nil {
		t.Fatal(err)
	}
	db.Create(&model.McpGroupTool{GroupID: group.ID, ServiceID: sid, ToolName: "generate_image", Enabled: false})
	db.Create(&model.McpGroupItem{GroupID: group.ID, ServiceID: sid, ItemKind: "resource", ItemKey: "dummy"})
	for range 2 {
		if err := s.Disable(1, detail.ID); err != nil {
			t.Fatal(err)
		}
	}
	if VirtualRegistry.IsVirtual(sid) {
		t.Fatal("registry survived disable")
	}
	for _, table := range []any{&model.McpService{}, &model.McpGroupTool{}, &model.McpGroupService{}, &model.McpGroupItem{}} {
		db.Model(table).Count(&count)
		if count != 0 {
			t.Fatalf("orphaned %T rows: %d", table, count)
		}
	}
	if err := s.Enable(1, detail.ID); err != nil {
		t.Fatal("re-enable", err)
	}
	cfg, _ = model.GetImageGenConfig(1, detail.ID)
	sid = *cfg.RegisteredServiceID
	if err := s.Delete(2, detail.ID); err == nil {
		t.Fatal("cross-user delete")
	}
	if err := s.Delete(1, detail.ID); err != nil {
		t.Fatal(err)
	}
	if VirtualRegistry.IsVirtual(sid) {
		t.Fatal("registry survived delete")
	}
	if _, err := s.Get(1, detail.ID); err == nil {
		t.Fatal("config survived delete")
	}
	geminiReq := imageGenReq("https://gateway.example/v1beta")
	geminiReq.Provider = igPtr("gemini")
	gemini, err := s.Create(1, geminiReq)
	if err != nil || gemini.EditRequestFormat != imagegen.EditFormatMultipart {
		t.Fatal("OpenAI editing default affected Gemini creation", err)
	}
}

func TestImageGenJSONEditingWithStoredReference(t *testing.T) {
	db := imageGenTestSetup(t)
	s := &ImageGenService{}
	pngBytes := imageGenPNG(t, 3, 2)
	stored, err := imageasset.Save(context.Background(), UploadStore, 1, bytes.NewReader(pngBytes), imagegen.MaxImageBytes())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	wantFormat := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/images/generations" || r.Header.Get("Authorization") != "Bearer secret-image-key" {
			t.Error("wrong route/auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		inputs := body
		if wantFormat == imagegen.EditFormatGenerationsJSONExtraBody {
			inputs, _ = body["extra_body"].(map[string]any)
		}
		refs, _ := inputs["image"].([]any)
		if len(refs) != 1 || refs[0] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(pngBytes) {
			t.Error("signed storage link was not read and converted to image content")
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString(pngBytes))
	}))
	defer upstream.Close()
	detail, err := s.Create(1, imageGenReq(upstream.URL))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Enable(1, detail.ID); err != nil {
		t.Fatal(err)
	}
	registered, err := model.GetImageGenConfig(1, detail.ID)
	if err != nil {
		t.Fatal(err)
	}
	sid := *registered.RegisteredServiceID
	// Omitting the editing format on creation must send references through
	// the generations API in both registered MCP and unsaved web previews.
	wantFormat = imagegen.EditFormatGenerationsJSON
	args, _ := json.Marshal(map[string]any{"prompt": "edit reference", "images": []string{stored.URL}, "n": 1, "return_mode": "url"})
	if raw, err := VirtualRegistry.Handle(virtual.WithCallerUserID(context.Background(), 1), sid, nil, "edit_image", args); err != nil || strings.Contains(string(raw), `"isError":true`) {
		t.Fatalf("default MCP editing format: %s, %v", raw, err)
	}
	if preview, err := s.Test(context.Background(), 1, &dto.ImageGenProbeReq{ImageGenConfigReq: *imageGenReq(upstream.URL), Tool: "edit_image", Arguments: args}); err != nil || preview.IsError {
		t.Fatal("unsaved web preview did not use the new editing default", err)
	}
	// Existing rows before this field was introduced retain the standard API.
	if err := db.Migrator().DropColumn(&model.ImageGenConfig{}, "edit_request_format"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.AutoMigrate(&model.ImageGenConfig{}); err != nil {
			t.Fatal(err)
		}
	}
	detail, err = s.Get(1, detail.ID)
	if err != nil || detail.EditRequestFormat != imagegen.EditFormatMultipart || *detail.RegisteredServiceID != sid || !detail.HasAPIKey {
		t.Fatal("legacy configuration lost its default, registration or key", err)
	}
	if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{Description: igPtr("Updated legacy description")}); err != nil {
		t.Fatal(err)
	}
	if legacy, err := s.Get(1, detail.ID); err != nil || legacy.EditRequestFormat != imagegen.EditFormatMultipart {
		t.Fatal("updating a legacy configuration changed its editing format", err)
	}
	for _, format := range []string{imagegen.EditFormatGenerationsJSON, imagegen.EditFormatGenerationsJSONExtraBody} {
		wantFormat = format
		if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{EditRequestFormat: &format}); err != nil {
			t.Fatal(err)
		}
		cfg, _ := model.GetImageGenConfig(1, detail.ID)
		if cfg.EditRequestFormat != format {
			t.Fatal("editing format was not persisted")
		}
		s.SyncAllRegisteredTools()
		svc, _ := model.GetServiceByID(1, sid)
		var tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]any `json:"properties"`
			} `json:"inputSchema"`
		}
		_ = json.Unmarshal([]byte(svc.ToolsCache), &tools)
		for _, tool := range tools {
			if tool.Name == "edit_image" && tool.InputSchema.Properties["mask"] != nil {
				t.Fatal("JSON editing advertised unsupported masks")
			}
		}
		raw, err := VirtualRegistry.Handle(virtual.WithCallerUserID(context.Background(), 1), sid, nil, "edit_image", args)
		if err != nil || !strings.Contains(string(raw), `"resource_link"`) || strings.Contains(string(raw), `"isError":true`) {
			t.Fatalf("MCP editing: %s, %v", raw, err)
		}
		preview, err := s.Test(context.Background(), 1, &dto.ImageGenProbeReq{ConfigID: detail.ID, Tool: "edit_image", Arguments: args})
		if err != nil || preview.IsError {
			t.Fatal("web preview did not use the configured editing format", err)
		}
		before := calls
		masked, _ := json.Marshal(map[string]any{"prompt": "edit", "images": []string{stored.URL}, "mask": stored.URL})
		if _, err := VirtualRegistry.Handle(virtual.WithCallerUserID(context.Background(), 1), sid, nil, "edit_image", masked); err == nil || calls != before {
			t.Fatal("unsupported mask reached the upstream")
		}
	}
	if calls != 6 {
		t.Fatalf("unexpected upstream retries: %d", calls)
	}
	if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{Provider: igPtr("gemini")}); err == nil {
		t.Fatal("accepted incompatible protocol and editing format")
	}
	cfg, _ := model.GetImageGenConfig(1, detail.ID)
	if cfg.Provider != "openai" || cfg.EditRequestFormat != wantFormat {
		t.Fatal("invalid update was not rolled back")
	}
}

func TestImageGenOpenRouterConfiguration(t *testing.T) {
	imageGenTestSetup(t)
	s := &ImageGenService{}
	req := imageGenReq("")
	req.Provider = igPtr(imagegen.ProviderOpenRouter)
	req.ModelName = igPtr("google/gemini-3.1-flash-image")
	req.DefaultParams = igPtr(json.RawMessage(`{"resolution":"2K","aspect_ratio":"16:9","extra_body":{"seed":42}}`))
	detail, err := s.Create(1, req)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Provider != imagegen.ProviderOpenRouter || detail.EndpointURL != imagegen.DefaultEndpoint(imagegen.ProviderOpenRouter) {
		t.Fatal("OpenRouter configuration did not retain its protocol or official default URL")
	}
	if err := s.Enable(1, detail.ID); err != nil {
		t.Fatal(err)
	}
	cfg, _ := model.GetImageGenConfig(1, detail.ID)
	svc, _ := model.GetServiceByID(1, *cfg.RegisteredServiceID)
	var tools []struct {
		Name        string `json:"name"`
		InputSchema struct {
			Properties map[string]any `json:"properties"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal([]byte(svc.ToolsCache), &tools); err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools {
		if tool.Name == "upload_image" || tool.Name == "get_image_status" {
			continue
		}
		props := tool.InputSchema.Properties
		if props["resolution"] == nil || props["aspect_ratio"] == nil || props["n"] == nil || props["image_size"] != nil || props["mask"] != nil {
			t.Fatalf("wrong OpenRouter tool schema: %s %v", tool.Name, props)
		}
	}
	if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{EndpointURL: igPtr("https://proxy.example/router/api/v1")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{Name: igPtr("Renamed image config"), ApiKey: igPtr("")}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = model.GetImageGenConfig(1, detail.ID)
	if cfg.EndpointURL != "https://proxy.example/router/api/v1" || cfg.ApiKey != "secret-image-key" {
		t.Fatal("an omitted endpoint or blank API key overwrote a saved connection")
	}
	if err := s.Update(1, detail.ID, &dto.ImageGenConfigReq{EndpointURL: igPtr("")}); err != nil {
		t.Fatal(err)
	}
	cfg, _ = model.GetImageGenConfig(1, detail.ID)
	if cfg.EndpointURL != imagegen.DefaultEndpoint(imagegen.ProviderOpenRouter) {
		t.Fatal("clearing the endpoint did not restore the protocol default")
	}
	if _, err := virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, "edit_image", json.RawMessage(`{"prompt":"edit","images":["https://private.example/input.png"],"mask":"bad"}`)); err == nil || !strings.Contains(err.Error(), "OpenRouter Image API does not support mask") {
		t.Fatalf("mask was not rejected before resolving input images: %v", err)
	}
}

func TestImageGenDefaultDescriptionsUpgradeAndCacheSync(t *testing.T) {
	db := imageGenTestSetup(t)
	s := &ImageGenService{}
	for _, tc := range []struct {
		name, generate, edit, wantGenerate, wantEdit string
	}{
		{"legacy defaults", legacyGenerateImageDesc, legacyEditImageDesc, model.DefaultGenerateImageDesc, model.DefaultEditImageDesc},
		{"previous edit default", model.DefaultGenerateImageDesc, legacyEditImageDescV2, model.DefaultGenerateImageDesc, model.DefaultEditImageDesc},
		{"custom edit", legacyGenerateImageDesc, "用户自定义编辑指引", model.DefaultGenerateImageDesc, "用户自定义编辑指引"},
		{"empty defaults", "", "  ", model.DefaultGenerateImageDesc, model.DefaultEditImageDesc},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created, err := s.Create(1, imageGenReq("https://gateway.example"))
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Enable(1, created.ID); err != nil {
				t.Fatal(err)
			}
			// Simulate configurations persisted by a previous release.
			if err := db.Model(&model.ImageGenConfig{}).Where("id = ?", created.ID).Updates(map[string]any{
				"generate_image_desc": tc.generate, "edit_image_desc": tc.edit,
			}).Error; err != nil {
				t.Fatal(err)
			}
			s.SyncAllRegisteredTools()
			detail, err := s.Get(1, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if detail.GenerateImageDesc != tc.wantGenerate || detail.EditImageDesc != tc.wantEdit {
				t.Fatalf("default upgrade changed a custom description or retained an old default: %+v", detail)
			}
			if detail.GenerateImageDescDefault != model.DefaultGenerateImageDesc || detail.EditImageDescDefault != model.DefaultEditImageDesc {
				t.Fatal("restore-default metadata is stale")
			}
			for _, desc := range []string{detail.GenerateImageDescDefault, detail.EditImageDescDefault, detail.UploadImageDesc} {
				for _, ch := range desc {
					if ch > 127 {
						t.Fatal("built-in description is not English ASCII")
					}
				}
			}
			svc, err := model.GetServiceByID(1, *detail.RegisteredServiceID)
			if err != nil {
				t.Fatal(err)
			}
			var tools []struct{ Name, Description string }
			if err := json.Unmarshal([]byte(svc.ToolsCache), &tools); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{
				"generate_image":   detail.GenerateImageDesc,
				"edit_image":       detail.EditImageDesc,
				"upload_image":     detail.UploadImageDesc,
				"get_image_status": detail.GetImageStatusDesc,
			}
			if len(tools) != len(want) {
				t.Fatalf("missing registered tools: %+v", tools)
			}
			for _, tool := range tools {
				if tool.Description != want[tool.Name] {
					t.Fatalf("detail and MCP description differ for %s", tool.Name)
				}
			}
		})
	}
}

func TestImageGenTransactionsRollbackBeforeRegistryChanges(t *testing.T) {
	db := imageGenTestSetup(t)
	s := &ImageGenService{}
	c, err := s.Create(1, imageGenReq("https://gateway.example"))
	if err != nil {
		t.Fatal(err)
	}
	// Fail the second write: the service INSERT must roll back along with the
	// config link, and the in-memory registry must remain unchanged.
	if err := db.Exec(`CREATE TRIGGER fail_image_link BEFORE UPDATE ON imagegen_configs WHEN NEW.auto_register = 1 BEGIN SELECT RAISE(ABORT, 'injected link failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Enable(1, c.ID); err == nil {
		t.Fatal("expected enable failure")
	}
	var count int64
	db.Model(&model.McpService{}).Count(&count)
	cfg, _ := model.GetImageGenConfig(1, c.ID)
	if count != 0 || cfg.AutoRegister {
		t.Fatal("enable did not roll back")
	}
	db.Exec("DROP TRIGGER fail_image_link")
	if err := s.Enable(1, c.ID); err != nil {
		t.Fatal(err)
	}
	cfg, _ = model.GetImageGenConfig(1, c.ID)
	sid := *cfg.RegisteredServiceID
	db.Exec(`CREATE TRIGGER fail_service_delete BEFORE DELETE ON mcp_services BEGIN SELECT RAISE(ABORT, 'injected delete failure'); END`)
	if err := s.Delete(1, c.ID); err == nil {
		t.Fatal("expected delete failure")
	}
	cfg, _ = model.GetImageGenConfig(1, c.ID)
	if !cfg.AutoRegister || !VirtualRegistry.IsVirtual(sid) {
		t.Fatal("failed transaction changed registry/config")
	}
	db.Exec("DROP TRIGGER fail_service_delete")
	db.Exec(`CREATE TRIGGER fail_service_update BEFORE UPDATE ON mcp_services BEGIN SELECT RAISE(ABORT, 'injected sync failure'); END`)
	if err := s.Update(1, c.ID, &dto.ImageGenConfigReq{Name: igPtr("unsaved")}); err == nil {
		t.Fatal("expected update failure")
	}
	cfg, _ = model.GetImageGenConfig(1, c.ID)
	if cfg.Name != "Image model" {
		t.Fatal("config update did not roll back")
	}
}

func TestImageGenStoredResultsReferencesMaskAndInline(t *testing.T) {
	imageGenTestSetup(t)
	data := imageGenPNG(t, 3, 2)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s","revised_prompt":"revised"}]}`, base64.StdEncoding.EncodeToString(data))
	}))
	defer server.Close()
	s := &ImageGenService{}
	req := imageGenReq(server.URL)
	req.EditRequestFormat = igPtr(imagegen.EditFormatMultipart)
	detail, err := s.Create(1, req)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &detail.ImageGenConfig
	decode := func(raw json.RawMessage) map[string]any {
		var res map[string]any
		if err := json.Unmarshal(raw, &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	raw, err := virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, "generate_image", json.RawMessage(`{"prompt":"draw","return_mode":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}
	res := decode(raw)
	blocks := res["content"].([]any)
	if len(blocks) != 3 || blocks[1].(map[string]any)["type"] != "resource_link" || blocks[2].(map[string]any)["type"] != "image" {
		t.Fatal(res)
	}
	meta := res["structuredContent"].(map[string]any)["images"].([]any)[0].(map[string]any)
	url := meta["url"].(string)
	if meta["width"] != float64(3) || meta["height"] != float64(2) || meta["mime"] != "image/png" || meta["revised_prompt"] != "revised" {
		t.Fatal(meta)
	}
	if _, err := imagegen.LoadReference(context.Background(), UploadStore, 2, url, 1024); err == nil {
		t.Fatal("cross-user storage read")
	}
	if _, err := imagegen.LoadReference(context.Background(), UploadStore, 1, url+"tampered", 1024); err == nil {
		t.Fatal("accepted bad signature")
	}
	args, _ := json.Marshal(map[string]any{"prompt": "edit", "images": []string{url}, "mask": "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)})
	if _, err := virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, "edit_image", args); err != nil {
		t.Fatal(err)
	}
	before := calls
	args, _ = json.Marshal(map[string]any{"prompt": "edit", "images": []string{url}, "mask": base64.StdEncoding.EncodeToString(imageGenPNG(t, 1, 1))})
	if _, err := virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, "edit_image", args); err == nil || !strings.Contains(err.Error(), "dimensions") || calls != before {
		t.Fatalf("mask validation failed: %v", err)
	}
	cfg.Provider = "gemini"
	if _, err := virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, "edit_image", args); err == nil || !strings.Contains(err.Error(), "does not support mask") || calls != before {
		t.Fatal("Gemini mask not rejected before API call")
	}
	cfg.Provider = "openai"
	if err := model.UpdateOption("ImageGenInlineMaxBytes", "200"); err != nil {
		t.Fatal(err)
	}
	raw, err = virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, "generate_image", json.RawMessage(`{"prompt":"draw","return_mode":"inline"}`))
	if err != nil {
		t.Fatal(err)
	}
	res = decode(raw)
	if len(res["content"].([]any)) != 2 || res["isError"] == true || !strings.Contains(string(raw), "Inline response exceeds") {
		t.Fatal(res)
	}
	// Retention, upload status and ownership are validated before reading bytes.
	sid, _ := storage.ShortIDFromURL(url)
	row, _ := model.GetUploadedImageByShortID(sid)
	model.DB.Model(row).Update("status", model.UploadStatusPending)
	if _, err := imagegen.LoadReference(context.Background(), UploadStore, 1, url, 1024); err == nil {
		t.Fatal("pending reference accepted")
	}
	model.DB.Model(row).Updates(map[string]any{"status": model.UploadStatusUploaded, "created_at": time.Now().Add(-48 * time.Hour)})
	if _, err := imagegen.LoadReference(context.Background(), UploadStore, 1, url, 1024); err == nil {
		t.Fatal("expired reference accepted")
	}
	runCleanupSweep(context.Background())
	if _, err := model.GetUploadedImageByShortID(sid); err == nil {
		t.Fatal("generated image not reaped")
	}
}

type imageFailStore struct {
	storage.Storage
	puts int
}

func (s *imageFailStore) Put(ctx context.Context, key string, r io.Reader, mime string) error {
	s.puts++
	if s.puts == 2 {
		return errors.New("injected storage failure")
	}
	return s.Storage.Put(ctx, key, r, mime)
}

func TestImageGenPartialDeliveryQuotaAndCancellation(t *testing.T) {
	db := imageGenTestSetup(t)
	a, b := imageGenPNG(t, 1, 1), imageGenPNG(t, 2, 2)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p["prompt"] == "slow" {
			time.Sleep(150 * time.Millisecond)
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s"},{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString(a), base64.StdEncoding.EncodeToString(b))
	}))
	defer server.Close()
	s := &ImageGenService{}
	c, err := s.Create(1, imageGenReq(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := virtual.RunImageGen(context.Background(), &imageFailStore{Storage: UploadStore}, 1, &c.ImageGenConfig, "generate_image", json.RawMessage(`{"prompt":"draw"}`))
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		IsError    bool `json:"isError"`
		Structured struct {
			Images   []any    `json:"images"`
			Warnings []string `json:"warnings"`
		} `json:"structuredContent"`
	}
	_ = json.Unmarshal(raw, &res)
	if !res.IsError || len(res.Structured.Images) != 1 || len(res.Structured.Warnings) != 1 || calls != 1 {
		t.Fatalf("partial results: %s", raw)
	}
	var partial model.ImageGenTask
	if err := db.Where("status = ?", "partial").First(&partial).Error; err != nil {
		t.Fatal("partial delivery history missing", err)
	}
	view, err := imagegentask.Views([]model.ImageGenTask{partial})
	if err != nil || len(view[0].Images) != 1 || len(view[0].Warnings) != 1 || view[0].Images[0].URL == "" {
		t.Fatalf("partial history lost successful result: %+v %v", view, err)
	}
	model.UpdateOption("MaxImagesPerUser", "1")
	before := calls
	if _, err := virtual.RunImageGen(context.Background(), UploadStore, 1, &c.ImageGenConfig, "generate_image", json.RawMessage(`{"prompt":"draw"}`)); err == nil || calls != before {
		t.Fatal("quota must reject before paid generation")
	}
	model.UpdateOption("MaxImagesPerUser", "0")
	c.TimeoutSeconds = 0
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := s.Test(ctx, 1, &dto.ImageGenProbeReq{ConfigID: c.ID, Arguments: json.RawMessage(`{"prompt":"slow"}`)})
	if err != nil || result.IsError {
		t.Fatalf("accepted preview must survive caller cancellation: %+v %v", result, err)
	}
	var accepted model.ImageGenTask
	if err := db.Where("source = ?", "test").First(&accepted).Error; err != nil {
		t.Fatal(err)
	}
	finished, err := virtual.ImageManager.Wait(context.Background(), 1, accepted.ID, 2)
	if err != nil || finished.Status != "completed" {
		t.Fatalf("client cancellation stopped accepted task: %+v %v", finished, err)
	}
	// A zero config timeout is persisted and does not introduce the old 60s limit.
	if err := s.Update(1, c.ID, &dto.ImageGenConfigReq{TimeoutSeconds: igPtr(0)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Enable(1, c.ID); err != nil {
		t.Fatal(err)
	}
	cfg, _ := model.GetImageGenConfig(1, c.ID)
	svc, _ := model.GetServiceByID(1, *cfg.RegisteredServiceID)
	result, ok, _ := callToolTestedContext(ctx, svc, 1, &dto.CallToolReq{Name: "generate_image", Arguments: map[string]any{"prompt": "slow"}})
	if ok || !result.IsError {
		t.Fatal("service test ignored caller cancellation")
	}
}

func TestSharedImageStorageDedupAndQuota(t *testing.T) {
	imageGenTestSetup(t)
	data := imageGenPNG(t, 1, 1)
	ctx := context.Background()
	first, err := (&UploadService{}).Upload(ctx, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	second, err := imageasset.Save(ctx, UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil || !second.Deduped || first.URL != second.URL {
		t.Fatalf("dedup %v %v", second, err)
	}
	other, err := imageasset.Save(ctx, UploadStore, 2, bytes.NewReader(data), 1024)
	if err != nil || other.URL == first.URL || other.Key != first.Key {
		t.Fatalf("owner rows %v %v", other, err)
	}
	if n, _ := model.CountByKey(first.Key); n != 2 {
		t.Fatalf("blob refcount %d", n)
	}
	if second.ExpiresAt == nil || time.Until(*second.ExpiresAt) < 23*time.Hour {
		t.Fatal("expiry must match retention rather than SignedURLTTL")
	}
	if _, err := imageasset.Save(ctx, UploadStore, 1, bytes.NewReader(data), 10); err == nil {
		t.Fatal("oversized shared save accepted")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := imageasset.Save(canceled, UploadStore, 3, bytes.NewReader(data), 1024); err == nil {
		t.Fatal("canceled save accepted")
	}
}

func TestImageGenOutputURLStoresActualImageWithoutCredentials(t *testing.T) {
	imageGenTestSetup(t)
	data := imageGenPNG(t, 4, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("x-goog-api-key") != "" {
				t.Error("model credentials leaked to image download")
			}
			w.Header().Set("Content-Type", "image/jpeg") // actual bytes are authoritative
			_, _ = w.Write(data)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret-image-key" {
			t.Error("missing model authentication")
		}
		fmt.Fprint(w, `{"data":[{"url":"/output.png"}]}`)
	}))
	defer server.Close()
	c, err := (&ImageGenService{}).Create(1, imageGenReq(server.URL+"/v1"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := virtual.RunImageGen(context.Background(), UploadStore, 1, &c.ImageGenConfig, "generate_image", json.RawMessage(`{"prompt":"draw"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"isError":true`) || !strings.Contains(string(raw), `"mime":"image/png"`) || !strings.Contains(string(raw), `"width":4`) {
		t.Fatalf("URL image not delivered: %s", raw)
	}
}

func TestImageGenServiceTestDeadlineRules(t *testing.T) {
	imageGenTestSetup(t)
	s := &ImageGenService{}
	c, err := s.Create(1, imageGenReq("https://gateway.example"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Enable(1, c.ID); err != nil {
		t.Fatal(err)
	}
	for _, seconds := range []int{0, 2} {
		if err := s.Update(1, c.ID, &dto.ImageGenConfigReq{TimeoutSeconds: &seconds}); err != nil {
			t.Fatal(err)
		}
		cfg, _ := model.GetImageGenConfig(1, c.ID)
		svc, _ := model.GetServiceByID(1, *cfg.RegisteredServiceID)
		ctx, cancel := toolTestContext(context.Background(), svc)
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		cancel()
		if !ok || remaining > 40*time.Second || remaining < 35*time.Second {
			t.Fatalf("async tool test deadline: %v, %v", ok, remaining)
		}

	}
}
