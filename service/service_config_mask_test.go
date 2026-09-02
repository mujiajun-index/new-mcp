package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// setupConfigMaskTest 初始化凭证掩码测试环境。
func setupConfigMaskTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.McpService{}, &model.McpServiceKey{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

// TestServiceDetailMasksCredentials 详情响应的 headers/env 凭证值只出掩码,
// url 等非凭证结构保持明文。
func TestServiceDetailMasksCredentials(t *testing.T) {
	setupConfigMaskTest(t)
	s := &McpServiceService{}
	svc := &model.McpService{
		UserID: 1, Name: "m1", TransportType: common.TransportStreamableHTTP,
		Config: `{"url":"https://u.example/mcp","headers":{"X-API-Key":"sk-plain-123456","Authorization":"Bearer tok-abcdefgh12"}}`,
		AuthType: "api_key", Status: common.StatusEnabled,
	}
	if err := model.DB.Create(svc).Error; err != nil {
		t.Fatalf("create svc: %v", err)
	}

	d := s.toDetail(svc)
	cfg := d.Config
	if cfg["url"] != "https://u.example/mcp" {
		t.Fatalf("url should stay plaintext: %v", cfg["url"])
	}
	headers, _ := cfg["headers"].(map[string]interface{})
	if headers["X-API-Key"] != maskSecret("sk-plain-123456") {
		t.Fatalf("api key should be masked: %v", headers["X-API-Key"])
	}
	if headers["Authorization"] != "Bearer "+maskSecret("tok-abcdefgh12") {
		t.Fatalf("bearer token should be masked with scheme kept: %v", headers["Authorization"])
	}
}

// TestServiceUpdateMergesMaskedCredentials 保存回填:未改动的掩码值还原为库内明文,
// 改动过的按新值入库。
func TestServiceUpdateMergesMaskedCredentials(t *testing.T) {
	setupConfigMaskTest(t)
	s := &McpServiceService{}
	svc := &model.McpService{
		UserID: 2, Name: "m2", TransportType: common.TransportStreamableHTTP,
		Config: `{"url":"https://u.example/mcp","headers":{"X-API-Key":"sk-plain-123456","X-Other":"keep-me-12345"}}`,
		AuthType: "api_key", Status: common.StatusEnabled,
	}
	if err := model.DB.Create(svc).Error; err != nil {
		t.Fatalf("create svc: %v", err)
	}

	// 掩码原样传回 + 改动另一项:X-API-Key 应回填明文,X-Other 存新值
	incoming := map[string]interface{}{
		"url": "https://u.example/mcp",
		"headers": map[string]interface{}{
			"X-API-Key": maskSecret("sk-plain-123456"),
			"X-Other":   "brand-new-99999",
		},
	}
	if err := s.Update(2, svc.ID, &dto.UpdateServiceReq{Config: incoming}); err != nil {
		t.Fatalf("update: %v", err)
	}
	fresh, err := model.GetServiceByID(2, svc.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(fresh.Config), &cfg)
	headers, _ := cfg["headers"].(map[string]interface{})
	if headers["X-API-Key"] != "sk-plain-123456" {
		t.Fatalf("masked value should be restored: %v", headers["X-API-Key"])
	}
	if headers["X-Other"] != "brand-new-99999" {
		t.Fatalf("edited value should persist: %v", headers["X-Other"])
	}
}
