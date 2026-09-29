package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func TestSystemOneSchemaRequiresStateOrItems(t *testing.T) {
	var tools []struct {
		InputSchema jsonschema.Schema `json:"inputSchema"`
	}
	if err := json.Unmarshal([]byte(systemOneTools()), &tools); err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected one tool, got %d", len(tools))
	}
	schema, err := tools[0].InputSchema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		input string
		valid bool
	}{
		{"neither", `{}`, false},
		{"null state only", `{"state":null}`, false},
		{"null items only", `{"items":null}`, false},
		{"empty items", `{"items":{}}`, false},
		{"text state", `{"state":"payment failed"}`, true},
		{"structured state", `{"state":{"message":"payment failed"}}`, true},
		{"false state is non-null", `{"state":false}`, true},
		{"items only", `{"items":{"a":"payment failed"}}`, true},
		{"both", `{"state":"shared context","items":{"a":"payment failed"}}`, true},
		{"null state with items", `{"state":null,"items":{"a":"payment failed"}}`, true},
		{"state with empty items", `{"state":"context","items":{}}`, false},
		{"items array", `{"items":["payment failed"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input map[string]any
			if err := json.Unmarshal([]byte(tc.input), &input); err != nil {
				t.Fatal(err)
			}
			input["questions"] = map[string]any{"q": map[string]any{
				"type": "noul", "instructions": "Is there a payment problem?",
			}}
			if err := schema.Validate(input); (err == nil) != tc.valid {
				t.Fatalf("valid = %v, want %v: %v", err == nil, tc.valid, err)
			}
			if tc.valid {
				delete(input, "questions")
				if err := schema.Validate(input); err == nil {
					t.Fatal("accepted missing questions")
				}
			}
		})
	}
	for _, count := range []int{500, 501} {
		t.Run(fmt.Sprintf("%d items", count), func(t *testing.T) {
			items := make(map[string]any, count)
			for i := range count {
				items[fmt.Sprint(i)] = "record"
			}
			input := map[string]any{
				"items": items,
				"questions": map[string]any{"q": map[string]any{
					"type": "noul", "instructions": "Is there a payment problem?",
				}},
			}
			if err := schema.Validate(input); (err == nil) != (count == 500) {
				t.Fatalf("unexpected validation result for %d items: %v", count, err)
			}
		})
	}
}

func TestSystemOneConfigLifecycle(t *testing.T) {
	oldDB, oldRegistry := model.DB, VirtualRegistry
	db, err := gorm.Open(sqlite.Open("file:systemone_lifecycle?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	model.DB = db
	VirtualRegistry = virtual.NewVirtualToolRegistry()
	t.Cleanup(func() { model.DB, VirtualRegistry = oldDB, oldRegistry })
	if err := db.AutoMigrate(&model.SystemOneConfig{}, &model.McpService{}, &model.McpGroupTool{}, &model.McpGroupService{}); err != nil {
		t.Fatal(err)
	}
	svc := &SystemOneService{}
	cfg, err := svc.Create(11, &dto.SystemOneConfigReq{Name: "CLM", Provider: "typesafe", EndpointURL: "http://localhost:8700", ModelName: "clm-latest", APIKey: "local-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HasAPIKey != true || strings.Contains(cfg.ResolvedURL, "local-secret") {
		t.Fatalf("credential in response: %+v", cfg)
	}
	saved, err := model.GetSystemOneConfig(11, cfg.ID)
	if err != nil || saved.APIKey == "local-secret" {
		t.Fatalf("API key was not stored via encryption: %v", err)
	}
	if _, err := svc.Get(12, cfg.ID); err == nil {
		t.Fatal("another user can read config")
	}
	if err := svc.Enable(11, cfg.ID); err != nil {
		t.Fatal(err)
	}
	enabled, err := model.GetSystemOneConfig(11, cfg.ID)
	if err != nil || !enabled.AutoRegister || enabled.RegisteredServiceID == nil {
		t.Fatalf("not enabled: %+v %v", enabled, err)
	}
	var mcp model.McpService
	if err := db.First(&mcp, *enabled.RegisteredServiceID).Error; err != nil {
		t.Fatal(err)
	}
	var tools []map[string]interface{}
	if err := json.Unmarshal([]byte(mcp.ToolsCache), &tools); err != nil || len(tools) != 1 || tools[0]["name"] != "evaluate" {
		t.Fatalf("unexpected tools: %s %v", mcp.ToolsCache, err)
	}
	// Existing registered configurations pick up schema constraints on startup.
	if err := db.Model(&mcp).Update("tools_cache", "[]").Error; err != nil {
		t.Fatal(err)
	}
	if n := svc.SyncAllRegisteredTools(); n != 1 {
		t.Fatalf("refreshed %d services, want 1", n)
	}
	if err := db.First(&mcp, mcp.ID).Error; err != nil || mcp.ToolsCache != systemOneTools() {
		t.Fatalf("schema cache not refreshed: %v", err)
	}
	if _, _, ok := VirtualRegistry.LookupByName(11, mcp.Name); !ok {
		t.Fatal("virtual handler missing")
	}
	if _, _, ok := VirtualRegistry.LookupByName(12, mcp.Name); ok {
		t.Fatal("another user can access the virtual service")
	}
	if _, err := VirtualRegistry.Handle(context.Background(), mcp.ID, nil, "other_tool", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unexpected tool was accepted")
	}
	if err := svc.Update(11, cfg.ID, &dto.SystemOneConfigReq{Name: "CLM updated", Provider: "typesafe", EndpointURL: "http://localhost:8700", ModelName: "clm-latest"}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&mcp, mcp.ID).Error; err != nil || mcp.DisplayName != "CLM updated" {
		t.Fatalf("service display name not updated: %s %v", mcp.DisplayName, err)
	}
	if err := svc.Disable(11, cfg.ID); err != nil {
		t.Fatal(err)
	}
	if VirtualRegistry.IsVirtual(mcp.ID) {
		t.Fatal("virtual handler remains after disable")
	}
	if err := db.First(&mcp, mcp.ID).Error; err == nil {
		t.Fatal("service remains after disable")
	}
	if err := svc.Delete(11, cfg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(11, cfg.ID); err == nil {
		t.Fatal("config remains after delete")
	}
}
