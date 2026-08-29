package billing

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// setupPricingTest 初始化定价解析测试环境:临时目录 sqlite + 选项(BillingEnabled=true,
// QuotaPerUnit 默认 500000)。每个测试函数独立数据库与选项;包级 pricingCache 跨测试
// 残留,须显式清空。
func setupPricingTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pricing.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.MarketplaceItem{}, &model.McpToolPrice{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	InvalidatePricingCache()
	if err := model.UpdateOption("BillingEnabled", "true"); err != nil {
		t.Fatalf("enable billing: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

func createPricingItem(t *testing.T, id int64, billingType string, price float64) {
	t.Helper()
	item := &model.MarketplaceItem{
		AdminID: 1, Name: fmt.Sprintf("item-%d", id), TransportType: "streamable-http",
		BillingType: billingType, PricePerCall: price,
		Status: 1,
	}
	item.ID = id
	if err := model.DB.Create(item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}
}

func insertEntryPrice(t *testing.T, itemID int64, kind, name, billingType string, price float64) {
	t.Helper()
	row := model.McpToolPrice{
		MarketplaceItemID: itemID, Kind: kind, ToolName: name,
		BillingType: billingType, PricePerCall: price, Enabled: true,
	}
	if err := model.DB.Create(&row).Error; err != nil {
		t.Fatalf("insert entry price: %v", err)
	}
}

// TestResolveToolPricing 工具条目:三级链回归(条目级>服务级>全局默认[仅自用]>未配置)。
func TestResolveToolPricing(t *testing.T) {
	setupPricingTest(t)
	createPricingItem(t, 1, "per_call", 0.01)
	insertEntryPrice(t, 1, EntryKindTool, "search", "per_call", 0.05)

	// 条目级命中:优先于服务级
	pi, err := ResolveMarketplaceEntryPrice(1, EntryKindTool, "search", "default")
	if err != nil || pi.Scope != "tool" || pi.UnitPriceQuota != 25000 || pi.UnitPriceDecimal != 0.05 {
		t.Fatalf("entry hit: %+v err=%v, want scope=tool quota=25000", pi, err)
	}
	// 未命中→服务级
	pi, err = ResolveMarketplaceEntryPrice(1, EntryKindTool, "other", "default")
	if err != nil || pi.Scope != "service" || pi.UnitPriceQuota != 5000 {
		t.Fatalf("service fallback: %+v err=%v, want scope=service quota=5000", pi, err)
	}
	// 分组倍率(vip=2)
	if err := model.UpdateOption("GroupRatio", `{"default":1,"vip":2}`); err != nil {
		t.Fatalf("set group ratio: %v", err)
	}
	pi, _ = ResolveMarketplaceEntryPrice(1, EntryKindTool, "other", "vip")
	if pi.UnitPriceQuota != 10000 {
		t.Fatalf("group ratio: quota=%d, want 10000", pi.UnitPriceQuota)
	}

	// 未定价服务(非自用模式):未配置报错;自用模式回退全局默认
	createPricingItem(t, 2, "per_call", 0)
	if _, err := ResolveMarketplaceEntryPrice(2, EntryKindTool, "any", "default"); err != ErrPriceNotConfigured {
		t.Fatalf("unpriced: err=%v, want ErrPriceNotConfigured", err)
	}
	if err := model.UpdateOption("SelfUseModeEnabled", "true"); err != nil {
		t.Fatalf("enable self-use: %v", err)
	}
	if err := model.UpdateOption("BillingDefaultType", "per_call"); err != nil {
		t.Fatalf("set default type: %v", err)
	}
	if err := model.UpdateOption("BillingDefaultPricePerCall", "0.02"); err != nil {
		t.Fatalf("set default price: %v", err)
	}
	pi, err = ResolveMarketplaceEntryPrice(2, EntryKindTool, "any", "default")
	if err != nil || pi.Scope != "global" || pi.UnitPriceQuota != 10000 {
		t.Fatalf("global default: %+v err=%v, want scope=global quota=10000", pi, err)
	}
}

// TestResolveResourcePromptPricing 资源/提示条目:命中→条目价(scope=entry);
// 未命中→免费(不回退服务价、不报未配置错),即使服务级未定价或为免费。
func TestResolveResourcePromptPricing(t *testing.T) {
	setupPricingTest(t)
	createPricingItem(t, 1, "per_call", 0.01)
	insertEntryPrice(t, 1, EntryKindResource, "memo://overview", "per_call", 0.02)
	insertEntryPrice(t, 1, EntryKindPrompt, "review", "free", 0)

	// 资源命中:scope=entry,quota=0.02×500000
	pi, err := ResolveMarketplaceEntryPrice(1, EntryKindResource, "memo://overview", "default")
	if err != nil || pi.Scope != PriceScopeEntry || pi.UnitPriceQuota != 10000 || pi.BillingType != BillingTypePerCall {
		t.Fatalf("resource hit: %+v err=%v, want scope=entry quota=10000", pi, err)
	}
	// 提示命中免费条目
	pi, err = ResolveMarketplaceEntryPrice(1, EntryKindPrompt, "review", "default")
	if err != nil || pi.Scope != PriceScopeEntry || pi.BillingType != BillingTypeFree || pi.UnitPriceQuota != 0 {
		t.Fatalf("prompt free hit: %+v err=%v, want scope=entry free", pi, err)
	}
	// 资源未命中→免费(不回退服务价 0.01)
	pi, err = ResolveMarketplaceEntryPrice(1, EntryKindResource, "memo://other", "default")
	if err != nil || pi.Scope != "free" || pi.UnitPriceQuota != 0 {
		t.Fatalf("resource miss: %+v err=%v, want free", pi, err)
	}
	// 提示未命中→免费
	pi, err = ResolveMarketplaceEntryPrice(1, EntryKindPrompt, "other", "default")
	if err != nil || pi.Scope != "free" {
		t.Fatalf("prompt miss: %+v err=%v, want free", pi, err)
	}

	// 服务级未定价(非自用):工具报错,资源/提示仍免费不报错
	createPricingItem(t, 2, "per_call", 0)
	if _, err := ResolveMarketplaceEntryPrice(2, EntryKindResource, "x", "default"); err != nil {
		t.Fatalf("resource on unpriced service: err=%v, want nil", err)
	}
	if _, err := ResolveMarketplaceEntryPrice(2, EntryKindPrompt, "x", "default"); err != nil {
		t.Fatalf("prompt on unpriced service: err=%v, want nil", err)
	}
	if _, err := ResolveMarketplaceEntryPrice(2, EntryKindTool, "x", "default"); err != ErrPriceNotConfigured {
		t.Fatalf("tool on unpriced service: err=%v, want ErrPriceNotConfigured", err)
	}
}

// TestEntryKindCoexist 同一市场项下同名工具与提示("x")分别命中各自条目价——
// 守护 (item_id, kind, tool_name) 复合唯一索引语义。
func TestEntryKindCoexist(t *testing.T) {
	setupPricingTest(t)
	createPricingItem(t, 1, "per_call", 0.01)
	insertEntryPrice(t, 1, EntryKindTool, "x", "per_call", 0.03)
	insertEntryPrice(t, 1, EntryKindPrompt, "x", "per_call", 0.04)

	pi, err := ResolveMarketplaceEntryPrice(1, EntryKindTool, "x", "default")
	if err != nil || pi.UnitPriceQuota != 15000 {
		t.Fatalf("tool x: %+v err=%v, want quota=15000", pi, err)
	}
	pi, err = ResolveMarketplaceEntryPrice(1, EntryKindPrompt, "x", "default")
	if err != nil || pi.UnitPriceQuota != 20000 {
		t.Fatalf("prompt x: %+v err=%v, want quota=20000", pi, err)
	}
}

// TestEntryPricingCacheInvalidation 条目价变更后 InvalidatePricingCacheItem 立即生效
// (不等 60s TTL)。
func TestEntryPricingCacheInvalidation(t *testing.T) {
	setupPricingTest(t)
	createPricingItem(t, 1, "per_call", 0.01)

	pi, _ := ResolveMarketplaceEntryPrice(1, EntryKindResource, "memo://a", "default")
	if pi.Scope != "free" {
		t.Fatalf("before insert: %+v, want free", pi)
	}
	insertEntryPrice(t, 1, EntryKindResource, "memo://a", "per_call", 0.02)
	InvalidatePricingCacheItem(1)
	pi, err := ResolveMarketplaceEntryPrice(1, EntryKindResource, "memo://a", "default")
	if err != nil || pi.Scope != PriceScopeEntry || pi.UnitPriceQuota != 10000 {
		t.Fatalf("after invalidate: %+v err=%v, want scope=entry quota=10000", pi, err)
	}
}
