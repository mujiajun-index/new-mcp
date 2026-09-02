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

// setupItemKeysTest 初始化条目级秘钥测试环境:TempDir sqlite + 相关表。
func setupItemKeysTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.McpService{}, &model.McpServiceKey{},
		&model.MarketplaceItem{}, &model.MarketplaceItemKey{}, &model.MarketplaceItemGroup{},
		&model.McpToolPrice{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

// newItemWithTemplate 建一个启用的免费 HTTP instant 条目,模板按入参原文落库(不加密,
// 与存量明文行同等待遇:plainItemConfig 的 Decrypt 失败回退原值路径)。
func newItemWithTemplate(t *testing.T, name, transport, template string) *model.MarketplaceItem {
	t.Helper()
	item := &model.MarketplaceItem{
		AdminID: 1, Name: name, Category: "instant", TransportType: transport,
		ConfigTemplate: template, BillingType: "free", Status: common.StatusEnabled,
	}
	if err := model.DB.Create(item).Error; err != nil {
		t.Fatalf("create item %s: %v", name, err)
	}
	return item
}

// templateHeaders 读回条目模板的 headers(服务端会加密落库,解密失败回退原值,
// 与 plainItemConfig 同路径)。
func templateHeaders(t *testing.T, item *model.MarketplaceItem) map[string]string {
	t.Helper()
	plain := item.ConfigTemplate
	if p, err := common.Decrypt(item.ConfigTemplate); err == nil && p != "" {
		plain = p
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(plain), &cfg)
	out := map[string]string{}
	if m, ok := cfg["headers"].(map[string]interface{}); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func reloadItem(t *testing.T, id int64) *model.MarketplaceItem {
	t.Helper()
	item, err := model.GetMarketplaceItemByID(id)
	if err != nil {
		t.Fatalf("reload item %d: %v", id, err)
	}
	return item
}

// TestItemKeysUpgradeIncorporatesTemplateHeader 单→多:模板认证头被收编为首把秘钥、
// 从模板剥离,bearer 位按头名与现值推导落库;降级时首选秘钥写回模板并清池。
func TestItemKeysUpgradeIncorporatesTemplateHeader(t *testing.T) {
	setupItemKeysTest(t)
	s := &MarketplaceService{}
	item := newItemWithTemplate(t, "svc-a", common.TransportStreamableHTTP,
		`{"url":"https://up.example/mcp","headers":{"X-API-Key":"sk-test-12345678"}}`)

	// 单秘钥态:认证类型从模板反推
	resp, err := s.ListKeys(item.ID)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if resp.KeyMode != "" || resp.AuthType != "api_key" || resp.Total != 0 {
		t.Fatalf("single-mode resp = %+v", resp)
	}

	// 单→多(X-API-Key → 非 bearer)
	resp, err = s.UpdateKeyConfig(item.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "random"})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if resp.KeyMode != "random" || resp.HeaderName != "X-API-Key" || resp.Total != 1 || resp.Enabled != 1 {
		t.Fatalf("upgraded resp = %+v", resp)
	}
	fresh := reloadItem(t, item.ID)
	if _, exists := templateHeaders(t, fresh)["X-API-Key"]; exists {
		t.Fatal("auth header should be stripped from template after upgrade")
	}
	if cfg := fresh.ParseAuthKeyConfig(); cfg.Bearer {
		t.Fatalf("bearer flag should be false for X-API-Key, got %+v", cfg)
	}
	keys, _ := model.ListKeysByItem(item.ID)
	if len(keys) != 1 || keys[0].Value != "sk-test-12345678" || keys[0].SortOrder != 1 {
		t.Fatalf("incorporated key = %+v", keys)
	}

	// 追加两把 → 共 3 把
	res, err := s.UpdateKeys(item.ID, &dto.UpdateServiceKeysReq{Mode: "append", Values: []string{"k2-1234567890", "k3-1234567890"}})
	if err != nil || res.Added != 2 {
		t.Fatalf("append: res=%+v err=%v", res, err)
	}

	// 多→单:首选启用秘钥写回模板、清池、清配置
	if _, err = s.UpdateKeyConfig(item.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "single"}); err != nil {
		t.Fatalf("downgrade: %v", err)
	}
	fresh = reloadItem(t, item.ID)
	if v := templateHeaders(t, fresh)["X-API-Key"]; v != "sk-test-12345678" {
		t.Fatalf("downgraded template header = %q", v)
	}
	if fresh.ParseAuthKeyConfig().KeyMode != "" {
		t.Fatal("auth_config key_mode should be cleared after downgrade")
	}
	if total, _ := model.CountItemKeys(item.ID); total != 0 {
		t.Fatalf("pool should be empty after downgrade, got %d", total)
	}
}

// TestItemKeysBearerFlow Authorization 模板:收编剥 "Bearer " 前缀 + bearer 位落库,
// 降级补前缀写回。
func TestItemKeysBearerFlow(t *testing.T) {
	setupItemKeysTest(t)
	s := &MarketplaceService{}
	item := newItemWithTemplate(t, "svc-b", common.TransportSSE,
		`{"url":"https://up.example/mcp","headers":{"Authorization":"Bearer tok-abcdefgh1234"}}`)

	resp, err := s.UpdateKeyConfig(item.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "polling"})
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if resp.HeaderName != "Authorization" || resp.AuthType != "bearer" {
		t.Fatalf("bearer resp = %+v", resp)
	}
	fresh := reloadItem(t, item.ID)
	if cfg := fresh.ParseAuthKeyConfig(); !cfg.Bearer {
		t.Fatalf("bearer flag should be recorded: %+v", cfg)
	}
	keys, _ := model.ListKeysByItem(item.ID)
	if len(keys) != 1 || keys[0].Value != "tok-abcdefgh1234" {
		t.Fatalf("bearer key stored with prefix stripped: %+v", keys)
	}

	if _, err = s.UpdateKeyConfig(item.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "single"}); err != nil {
		t.Fatalf("downgrade: %v", err)
	}
	fresh = reloadItem(t, item.ID)
	if v := templateHeaders(t, fresh)["Authorization"]; v != "Bearer tok-abcdefgh1234" {
		t.Fatalf("downgraded Authorization = %q", v)
	}
}

// TestItemKeysUpgradeRejected 无认证头/策略切换换头/source 条目/stdio 条目的守卫。
func TestItemKeysUpgradeRejected(t *testing.T) {
	setupItemKeysTest(t)
	s := &MarketplaceService{}

	noAuth := newItemWithTemplate(t, "svc-c", common.TransportStreamableHTTP, `{"url":"https://up.example/mcp"}`)
	if _, err := s.UpdateKeyConfig(noAuth.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "random"}); err == nil {
		t.Fatal("upgrade without auth header should fail")
	}

	authed := newItemWithTemplate(t, "svc-d", common.TransportStreamableHTTP,
		`{"url":"https://u.example/mcp","headers":{"X-Custom-Auth":"secret-123456789"}}`)
	if _, err := s.UpdateKeyConfig(authed.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "random"}); err != nil {
		t.Fatalf("custom upgrade: %v", err)
	}
	// 已多秘钥 = 策略切换:换注入头被拒,沿用既有头切策略成功
	if _, err := s.UpdateKeyConfig(authed.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "polling", HeaderName: "X-Other"}); err == nil {
		t.Fatal("switching header in multi mode should fail")
	}
	if _, err := s.UpdateKeyConfig(authed.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "polling"}); err != nil {
		t.Fatalf("strategy switch should keep header: %v", err)
	}

	sourceItem := newItemWithTemplate(t, "svc-e", common.TransportStreamableHTTP, `{"url":"https://up.example/mcp"}`)
	sourceItem.Category = "source"
	if err := model.DB.Save(sourceItem).Error; err != nil {
		t.Fatalf("save source item: %v", err)
	}
	if _, err := s.ListKeys(sourceItem.ID); err == nil {
		t.Fatal("source-category item should not be manageable")
	}

	stdioItem := newItemWithTemplate(t, "svc-f", "stdio", `{"command":"npx","args":["x"],"env":{}}`)
	if _, err := s.ListKeys(stdioItem.ID); err == nil {
		t.Fatal("stdio item should not be manageable")
	}
}

// TestItemKeysBatchAndStatus 启/禁/删与批量操作(追加去重保状态、删除已禁用)。
func TestItemKeysBatchAndStatus(t *testing.T) {
	setupItemKeysTest(t)
	s := &MarketplaceService{}
	item := newItemWithTemplate(t, "svc-g", common.TransportStreamableHTTP,
		`{"url":"https://up.example/mcp","headers":{"X-API-Key":"seed-1234567890"}}`)

	if _, err := s.UpdateKeyConfig(item.ID, &dto.UpdateServiceKeyConfigReq{KeyMode: "random"}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if _, err := s.UpdateKeys(item.ID, &dto.UpdateServiceKeysReq{Mode: "append", Values: []string{"k2-1234567890"}}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// 追加重复值:跳过
	res, err := s.UpdateKeys(item.ID, &dto.UpdateServiceKeysReq{Mode: "append", Values: []string{"k2-1234567890"}})
	if err != nil || res.Added != 0 || res.Skipped != 1 {
		t.Fatalf("dedup append: res=%+v err=%v", res, err)
	}

	keys, _ := model.ListKeysByItem(item.ID)
	if err := s.SetKeyStatus(item.ID, keys[1].ID, &dto.SetServiceKeyStatusReq{Status: "disabled"}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := s.BatchKeys(item.ID, "delete_disabled"); err != nil {
		t.Fatalf("delete_disabled: %v", err)
	}
	keys, _ = model.ListKeysByItem(item.ID)
	if len(keys) != 1 || keys[0].SortOrder != 1 {
		t.Fatalf("pool after delete_disabled = %+v", keys)
	}
	// 替换整池:重排、状态清零
	if _, err := s.UpdateKeys(item.ID, &dto.UpdateServiceKeysReq{Mode: "replace", Values: []string{"n1-123456789", "n2-123456789"}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if total, enabled := model.CountItemKeys(item.ID); total != 2 || enabled != 2 {
		t.Fatalf("replace pool = (%d,%d)", total, enabled)
	}
}

// TestCloneFromServiceCarriesKeyPool 多秘钥源服务克隆:模板剥认证头、模式/头名/bearer
// 写入条目 auth_config、池整拷(状态重置启用);单秘钥源行为不变。
func TestCloneFromServiceCarriesKeyPool(t *testing.T) {
	setupItemKeysTest(t)
	s := &MarketplaceService{}

	// 多秘钥 bearer 源服务(手工构造已切换完成的形态)
	svc := &model.McpService{
		UserID: 7, Name: "multi-src", TransportType: common.TransportStreamableHTTP,
		Config: `{"url":"https://up.example/mcp","headers":{"X-Other":"v"}}`,
		AuthType: "bearer",
		AuthConfig: `{"key_mode":"polling","header_name":"Authorization"}`,
		Status: common.StatusEnabled,
	}
	if err := model.DB.Create(svc).Error; err != nil {
		t.Fatalf("create src service: %v", err)
	}
	if err := model.ReplaceKeys(svc.ID, []string{"tok-one-12345678", "tok-two-12345678"}); err != nil {
		t.Fatalf("seed service pool: %v", err)
	}

	detail, err := s.CloneFromService(7, &dto.CloneMarketplaceReq{
		FromServiceID: svc.ID, Name: "item-multi", DisplayName: "IM", BillingType: "free",
	})
	if err != nil {
		t.Fatalf("clone multi-key service: %v", err)
	}
	item := reloadItem(t, detail.ID)
	cfg := item.ParseAuthKeyConfig()
	if cfg.KeyMode != "polling" || cfg.HeaderName != "Authorization" || !cfg.Bearer {
		t.Fatalf("cloned auth_config = %+v", cfg)
	}
	if h := templateHeaders(t, item); len(h) != 1 || h["X-Other"] != "v" {
		t.Fatalf("cloned template headers = %v (auth header must be stripped, others kept)", h)
	}
	if total, enabled := model.CountItemKeys(item.ID); total != 2 || enabled != 2 {
		t.Fatalf("cloned pool = (%d,%d)", total, enabled)
	}
	keys, _ := model.ListKeysByItem(item.ID)
	if keys[0].Value != "tok-one-12345678" || keys[1].Value != "tok-two-12345678" {
		t.Fatalf("cloned key order = %+v", keys)
	}

	// 空池源:整拷拒绝
	if err := model.DeleteKeysByService(svc.ID); err != nil {
		t.Fatalf("clear pool: %v", err)
	}
	if _, err = s.CloneFromService(7, &dto.CloneMarketplaceReq{
		FromServiceID: svc.ID, Name: "item-empty", BillingType: "free",
	}); err == nil {
		t.Fatal("clone with empty pool should fail")
	}

	// 单秘钥源:行为不变(模板原样,条目无池、无多秘钥配置)
	single := &model.McpService{
		UserID: 7, Name: "single-src", TransportType: common.TransportStreamableHTTP,
		Config: `{"url":"https://up.example/mcp","headers":{"X-API-Key":"sk-1-1234567890"}}`,
		AuthType: "api_key", Status: common.StatusEnabled,
	}
	if err := model.DB.Create(single).Error; err != nil {
		t.Fatalf("create single src: %v", err)
	}
	detail, err = s.CloneFromService(7, &dto.CloneMarketplaceReq{
		FromServiceID: single.ID, Name: "item-single", BillingType: "free",
	})
	if err != nil {
		t.Fatalf("clone single-key service: %v", err)
	}
	singleItem := reloadItem(t, detail.ID)
	if singleItem.ParseAuthKeyConfig().KeyMode != "" {
		t.Fatal("single-key clone should not carry key_mode")
	}
	if v := templateHeaders(t, singleItem)["X-API-Key"]; v != "sk-1-1234567890" {
		t.Fatalf("single-key clone template = %v", templateHeaders(t, singleItem))
	}
	if total, _ := model.CountItemKeys(singleItem.ID); total != 0 {
		t.Fatalf("single-key clone should have no pool, got %d", total)
	}
}
