package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

func TestQueryCredentialRoundTrip(t *testing.T) {
	const raw = "https://example.test/mcp/?region=cn&tavilyApiKey=abc%2B123"
	base, key, err := takeQueryCredential(raw, "tavilyApiKey")
	if err != nil || key != "abc+123" || strings.Contains(base, "abc") || !strings.Contains(base, "region=cn") {
		t.Fatalf("take: base=%q key=%q err=%v", base, key, err)
	}
	full, err := putQueryCredential(base, "tavilyApiKey", key)
	if err != nil || !hasQueryCredential(full, "tavilyApiKey") {
		t.Fatalf("put: %q %v", full, err)
	}
	masked := maskQueryCredential(full, "tavilyApiKey")
	if strings.Contains(masked, "abc+123") || mergeMaskedQueryCredential(masked, full, "tavilyApiKey") != full {
		t.Fatalf("mask/merge failed: %q", masked)
	}
	if _, _, err := takeQueryCredential(raw+"&tavilyApiKey=other", "tavilyApiKey"); err == nil {
		t.Fatal("duplicate target parameter should be rejected")
	}
}

func TestServiceQueryKeyUpgradeAndDowngrade(t *testing.T) {
	setupConfigMaskTest(t)
	svc := &model.McpService{
		UserID: 1, Name: "query_service", TransportType: common.TransportStreamableHTTP,
		AuthType: "none", Config: `{"url":"https://example.test/mcp/?region=cn&tavilyApiKey=first","headers":{}}`,
		Status: common.StatusEnabled,
	}
	if err := model.DB.Create(svc).Error; err != nil {
		t.Fatal(err)
	}
	if err := upgradeToMultiKey(svc, common.KeyModePolling, "", "tavilyApiKey"); err != nil {
		t.Fatal(err)
	}
	if svc.AuthType != "query_param" || !svc.IsMultiKey() {
		t.Fatalf("auth state: %+v", svc)
	}
	if strings.Contains(svc.Config, "first") {
		t.Fatal("credential remained in base URL")
	}
	keys, err := model.ListKeysByService(svc.ID)
	if err != nil || len(keys) != 1 || keys[0].Value != "first" {
		t.Fatalf("keys: %+v %v", keys, err)
	}
	if err := downgradeToSingleKey(svc); err != nil {
		t.Fatal(err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(svc.Config), &config); err != nil {
		t.Fatal(err)
	}
	if !hasQueryCredential(config["url"].(string), "tavilyApiKey") || svc.IsMultiKey() {
		t.Fatal("single query credential not restored")
	}
	masked := (&McpServiceService{}).toDetail(svc).Config["url"].(string)
	if strings.Contains(masked, "first") {
		t.Fatal("service detail exposed query credential")
	}
}
