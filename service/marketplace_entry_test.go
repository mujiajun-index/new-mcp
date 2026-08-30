package service

import (
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/billing"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

// enrichItemSnapshots 给市场项补资源(含模板)/提示快照,供条目级定价校验与计费键使用
// (createMarketplaceFixture 只带工具快照)。
func enrichItemSnapshots(t *testing.T, itemID int64) {
	t.Helper()
	updates := map[string]interface{}{
		"resources_snapshot": `{"resources":[{"uri":"memo://overview","name":"overview"}],"templates":[{"uriTemplate":"memo://notes/{id}","name":"notes"}]}`,
		"prompts_snapshot":   `[{"name":"review","description":"review code"}]`,
	}
	if err := model.DB.Model(&model.MarketplaceItem{}).Where("id = ?", itemID).Updates(updates).Error; err != nil {
		t.Fatalf("update snapshots: %v", err)
	}
}

// entryPricesOf 取市场项当前条目价(经管理详情 toDetail 同源逻辑校验暴露)。
func entryPricesOf(t *testing.T, itemID int64) []dto.MarketplaceEntryPrice {
	t.Helper()
	s := &MarketplaceService{}
	detail, err := s.GetItemByID(itemID)
	if err != nil {
		t.Fatalf("GetItemByID: %v", err)
	}
	return detail.EntryPrices
}

// TestMarketplaceEntryPricingBilling 条目级定价计费全链路(与工具/网关同口径):
// 资源/提示有条目价→扣费(scope=entry),无条目价→免费(见 marketplace_call_test 回归);
// 上游失败退款、余额不足拒绝;工具条目价覆盖服务价;SetItemEntryPrices 校验与全量替换。
func TestMarketplaceEntryPricingBilling(t *testing.T) {
	setupMarketplaceBillingTest(t)
	url, shutdown := startEchoMCPServer(t)
	defer shutdown()

	s := &MarketplaceService{}
	// 0.02 × QuotaPerUnit 500000 = 10000 quota
	const entryQuota = 10000

	setPrices := func(t *testing.T, itemID int64, prices ...dto.MarketplaceEntryPrice) {
		t.Helper()
		if err := s.SetItemEntryPrices(itemID, prices); err != nil {
			t.Fatalf("SetItemEntryPrices: %v", err)
		}
	}

	t.Run("resource_charged_with_entry_price", func(t *testing.T) {
		user, svc, item := createMarketplaceFixture(t, "e-res", "user", 20000, url, "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindResource, Name: "memo://overview", BillingType: "per_call", PricePerCall: 0.02})

		svcSvc := &McpServiceService{}
		res, err := svcSvc.ReadResource(user.ID, svc.ID, &dto.ReadResourceReq{URI: "memo://overview"})
		if err != nil {
			t.Fatalf("ReadResource: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		log := lastManualTestLog(t)
		if log.BillingStatus != "charged" || log.QuotaConsumed != entryQuota || log.Method != "resources/read" {
			t.Fatalf("log method=%s status=%s quota=%d, want resources/read/charged/%d", log.Method, log.BillingStatus, log.QuotaConsumed, entryQuota)
		}
		if log.PriceScope != billing.PriceScopeEntry || log.UnitPrice != 0.02 {
			t.Fatalf("price snapshot wrong: scope=%s unit=%v", log.PriceScope, log.UnitPrice)
		}
		if log.MarketplaceItemID == nil || *log.MarketplaceItemID != item.ID {
			t.Fatalf("log item id = %v, want %d", log.MarketplaceItemID, item.ID)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000-entryQuota || used != entryQuota {
			t.Fatalf("quota=%d used=%d, want %d/%d", quota, used, 20000-entryQuota, entryQuota)
		}
	})

	t.Run("prompt_charged_with_entry_price", func(t *testing.T) {
		user, svc, item := createMarketplaceFixture(t, "e-prm", "user", 20000, url, "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindPrompt, Name: "review", BillingType: "per_call", PricePerCall: 0.01})

		svcSvc := &McpServiceService{}
		res, err := svcSvc.GetPrompt(user.ID, svc.ID, &dto.GetPromptReq{Name: "review"})
		if err != nil {
			t.Fatalf("GetPrompt: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		log := lastManualTestLog(t)
		if log.BillingStatus != "charged" || log.Method != "prompts/get" || log.PriceScope != billing.PriceScopeEntry {
			t.Fatalf("log method=%s status=%s scope=%s, want prompts/get/charged/entry", log.Method, log.BillingStatus, log.PriceScope)
		}
	})

	t.Run("resource_free_without_entry_price_even_if_service_priced", func(t *testing.T) {
		// 服务级 per_call 0.01,但资源无条目价 → 免费(不回退服务价)
		user, svc, item := createMarketplaceFixture(t, "e-free", "user", 20000, url, "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)

		svcSvc := &McpServiceService{}
		res, err := svcSvc.ReadResource(user.ID, svc.ID, &dto.ReadResourceReq{URI: "memo://overview"})
		if err != nil {
			t.Fatalf("ReadResource: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		if log := lastManualTestLog(t); log.BillingStatus != "skipped" {
			t.Fatalf("billing status=%s, want skipped", log.BillingStatus)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000 || used != 0 {
			t.Fatalf("quota=%d used=%d, want untouched 20000/0", quota, used)
		}
		_ = item
	})

	t.Run("resource_refunded_on_upstream_failure", func(t *testing.T) {
		user, svc, item := createMarketplaceFixture(t, "e-dead", "user", 20000, "http://127.0.0.1:1/mcp", "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindResource, Name: "memo://overview", BillingType: "per_call", PricePerCall: 0.02})

		svcSvc := &McpServiceService{}
		res, err := svcSvc.ReadResource(user.ID, svc.ID, &dto.ReadResourceReq{URI: "memo://overview"})
		if err != nil {
			t.Fatalf("ReadResource: %v", err)
		}
		if !res.IsError {
			t.Fatalf("expected connection error result")
		}
		if log := lastManualTestLog(t); log.BillingStatus != "refunded" {
			t.Fatalf("billing status=%s, want refunded", log.BillingStatus)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000 || used != 0 {
			t.Fatalf("quota=%d used=%d, want refunded to 20000/0", quota, used)
		}
	})

	t.Run("resource_blocked_on_insufficient_quota", func(t *testing.T) {
		user, svc, item := createMarketplaceFixture(t, "e-poor", "user", 100, url, "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindResource, Name: "memo://overview", BillingType: "per_call", PricePerCall: 0.02})

		svcSvc := &McpServiceService{}
		res, err := svcSvc.ReadResource(user.ID, svc.ID, &dto.ReadResourceReq{URI: "memo://overview"})
		if err != nil {
			t.Fatalf("ReadResource: %v", err)
		}
		if !res.IsError || res.Error == "" {
			t.Fatalf("expected blocked error, got %+v", res)
		}
		if log := lastManualTestLog(t); log.BillingStatus != "blocked" {
			t.Fatalf("billing status=%s, want blocked", log.BillingStatus)
		}
		if quota, _ := userQuota(t, user.ID); quota != 100 {
			t.Fatalf("quota=%d, want unchanged 100", quota)
		}
	})

	t.Run("resource_inherits_service_price", func(t *testing.T) {
		// 资源显式继承(billing_type=inherit)→ 按服务统一价计费(scope=service)
		user, svc, item := createMarketplaceFixture(t, "e-inh", "user", 20000, url, "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindResource, Name: "memo://overview", BillingType: "inherit", PricePerCall: 0})

		svcSvc := &McpServiceService{}
		res, err := svcSvc.ReadResource(user.ID, svc.ID, &dto.ReadResourceReq{URI: "memo://overview"})
		if err != nil {
			t.Fatalf("ReadResource: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		// 服务级 0.01 × 500000 = 5000(而非条目价/免费)
		log := lastManualTestLog(t)
		if log.BillingStatus != "charged" || log.QuotaConsumed != 5000 || log.PriceScope != "service" || log.UnitPrice != 0.01 {
			t.Fatalf("log status=%s quota=%d scope=%s unit=%v, want charged/5000/service/0.01", log.BillingStatus, log.QuotaConsumed, log.PriceScope, log.UnitPrice)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000-5000 || used != 5000 {
			t.Fatalf("quota=%d used=%d, want %d/%d", quota, used, 20000-5000, 5000)
		}
	})

	t.Run("tool_entry_price_overrides_service", func(t *testing.T) {
		user, svc, item := createMarketplaceFixture(t, "e-tool", "user", 50000, url, "per_call", 0.01)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindTool, Name: "echo", BillingType: "per_call", PricePerCall: 0.05})

		svcSvc := &McpServiceService{}
		res, err := svcSvc.CallTool(user.ID, svc.ID, &dto.CallToolReq{Name: "echo", Arguments: map[string]interface{}{"msg": "hi"}})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		// 0.05 × 500000 = 25000(覆盖服务级 0.01×500000=5000)
		log := lastManualTestLog(t)
		if log.BillingStatus != "charged" || log.QuotaConsumed != 25000 || log.PriceScope != "tool" {
			t.Fatalf("log status=%s quota=%d scope=%s, want charged/25000/tool", log.BillingStatus, log.QuotaConsumed, log.PriceScope)
		}
	})

	t.Run("set_entry_prices_validation_and_replace", func(t *testing.T) {
		_, _, item := createMarketplaceFixture(t, "e-valid", "user", 20000, url, "per_call", 0.01)
		enrichItemSnapshots(t, item.ID)

		cases := []struct {
			name    string
			prices  []dto.MarketplaceEntryPrice
			wantSub string
		}{
			{"template_rejected", []dto.MarketplaceEntryPrice{{Kind: "resource", Name: "memo://notes/{id}", BillingType: "per_call", PricePerCall: 0.01}}, "不存在于服务快照"},
			{"unknown_name_rejected", []dto.MarketplaceEntryPrice{{Kind: "tool", Name: "nope", BillingType: "per_call", PricePerCall: 0.01}}, "不存在于服务快照"},
			{"per_call_zero_rejected", []dto.MarketplaceEntryPrice{{Kind: "tool", Name: "echo", BillingType: "per_call", PricePerCall: 0}}, "必须大于 0"},
			{"duplicate_rejected", []dto.MarketplaceEntryPrice{
				{Kind: "tool", Name: "echo", BillingType: "free", PricePerCall: 0},
				{Kind: "tool", Name: "echo", BillingType: "per_call", PricePerCall: 0.01},
			}, "重复"},
		}
		for _, tc := range cases {
			err := s.SetItemEntryPrices(item.ID, tc.prices)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("%s: err=%v, want contains %q", tc.name, err, tc.wantSub)
			}
		}

		// 合法全量替换:工具免费 + 资源定价 → 详情暴露两条
		setPrices(t, item.ID,
			dto.MarketplaceEntryPrice{Kind: billing.EntryKindTool, Name: "echo", BillingType: "free", PricePerCall: 0},
			dto.MarketplaceEntryPrice{Kind: billing.EntryKindResource, Name: "memo://overview", BillingType: "per_call", PricePerCall: 0.02},
		)
		got := entryPricesOf(t, item.ID)
		if len(got) != 2 {
			t.Fatalf("entry prices = %+v, want 2 rows", got)
		}
		byKey := map[string]dto.MarketplaceEntryPrice{}
		for _, p := range got {
			byKey[p.Kind+":"+p.Name] = p
		}
		if r := byKey["tool:echo"]; r.BillingType != "free" {
			t.Fatalf("tool row wrong: %+v", r)
		}
		if r := byKey["resource:memo://overview"]; r.BillingType != "per_call" || r.PricePerCall != 0.02 {
			t.Fatalf("resource row wrong: %+v", r)
		}

		// 空数组=清空全部条目价
		setPrices(t, item.ID)
		if got := entryPricesOf(t, item.ID); len(got) != 0 {
			t.Fatalf("after clear, entry prices = %+v, want empty", got)
		}

		// inherit 行合法:价格强制归零存储(解析时回退服务级链)
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindPrompt, Name: "review", BillingType: "inherit", PricePerCall: 0.05})
		got = entryPricesOf(t, item.ID)
		if len(got) != 1 || got[0].BillingType != "inherit" || got[0].PricePerCall != 0 {
			t.Fatalf("inherit row = %+v, want billing_type=inherit price=0", got)
		}

		// 公开详情(启用态)同样暴露条目价
		setPrices(t, item.ID, dto.MarketplaceEntryPrice{Kind: billing.EntryKindPrompt, Name: "review", BillingType: "per_call", PricePerCall: 0.01})
		pub, err := s.GetPublished(item.ID)
		if err != nil {
			t.Fatalf("GetPublished: %v", err)
		}
		if len(pub.EntryPrices) != 1 || pub.EntryPrices[0].Kind != "prompt" {
			t.Fatalf("published entry prices = %+v, want 1 prompt", pub.EntryPrices)
		}
	})
}
