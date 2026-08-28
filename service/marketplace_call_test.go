package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

// startEchoMCPServer 起一个带 echo 工具 + 1 资源 + 1 提示的 MCP 服务(Streamable HTTP,
// 本地随机端口),作为市场服务的真实平台上游,返回其 URL 与结束函数。
func startEchoMCPServer(t *testing.T) (url string, shutdown func()) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "v0.0.1"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "echo back",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"msg": map[string]any{"type": "string"}},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo-ok"}}}, nil
	})
	readHandler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "content-of-" + req.Params.URI}},
		}, nil
	}
	server.AddResource(&mcp.Resource{URI: "memo://overview", Name: "overview"}, readHandler)
	server.AddPrompt(&mcp.Prompt{Name: "review", Description: "review code"},
		func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Description: "review prompt",
				Messages: []*mcp.PromptMessage{
					{Role: mcp.Role("user"), Content: &mcp.TextContent{Text: "review this code"}},
				},
			}, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpSrv := httptest.NewServer(handler)
	return httpSrv.URL, httpSrv.Close
}

// setupMarketplaceBillingTest 初始化计费测试环境:内存级 sqlite + 选项(BillingEnabled=true,
// 其余取默认:ChargeAdmin=true、PreConsumedQuota=500、QuotaPerUnit=500000)。
func setupMarketplaceBillingTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "t.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.McpService{},
		&model.MarketplaceItem{}, &model.McpCallLog{}, &model.ApiKey{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	if err := model.UpdateOption("BillingEnabled", "true"); err != nil {
		t.Fatalf("enable billing: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

// createMarketplaceFixture 造一个市场引用服务(source=marketplace)与其关联用户/市场项。
// pricePerCall>0 时为 per_call 计费;toolsJSON 同时作为市场项快照与引用行缓存。
func createMarketplaceFixture(t *testing.T, username, role string, quota int64, upstreamURL string, billingType string, pricePerCall float64) (*model.User, *model.McpService, *model.MarketplaceItem) {
	t.Helper()

	user := &model.User{
		Username: username, Password: "x", Role: role, Group: "default",
		Status: 1, Quota: quota, AffCode: "aff-" + username,
	}
	if err := model.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	toolsJSON := `[{"name":"echo","description":"echo back","inputSchema":{"type":"object","properties":{"msg":{"type":"string"}}}}]`
	item := &model.MarketplaceItem{
		AdminID: 1, Name: "item-" + username, TransportType: "streamable-http",
		ConfigTemplate: fmt.Sprintf(`{"url":%q}`, upstreamURL),
		ToolsSnapshot:  toolsJSON,
		BillingType:    billingType, PricePerCall: pricePerCall,
		Status: common.StatusEnabled,
	}
	if err := model.DB.Create(item).Error; err != nil {
		t.Fatalf("create item: %v", err)
	}

	svc := &model.McpService{
		UserID: user.ID, Name: "svc-" + username, TransportType: "marketplace", // 哨兵值,物化时还原
		Config: "{}", ToolsCache: toolsJSON, Source: "marketplace",
		MarketplaceItemID: &item.ID, Status: 1,
	}
	if err := model.DB.Create(svc).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}
	return user, svc, item
}

// lastManualTestLog 取最近一条手动测试日志(ApiKeyName=tool-test)。
func lastManualTestLog(t *testing.T) *model.McpCallLog {
	t.Helper()
	var l model.McpCallLog
	if err := model.DB.Where("api_key_name = ?", ManualTestTokenName).
		Order("id DESC").First(&l).Error; err != nil {
		t.Fatalf("load manual test log: %v", err)
	}
	return &l
}

func userQuota(t *testing.T, userID int64) (quota, used int64) {
	t.Helper()
	u, err := model.GetUserByID(userID)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	return u.Quota, u.UsedQuota
}

// TestCallToolMarketplaceBilling 覆盖市场服务工具测试的计费全链路(与网关同口径):
// 成功扣费 / 上游失败退款 / 余额不足拒绝 / 管理员不豁免 / 免费跳过 / 总开关关闭跳过。
func TestCallToolMarketplaceBilling(t *testing.T) {
	setupMarketplaceBillingTest(t)
	url, shutdown := startEchoMCPServer(t)
	defer shutdown()

	s := &McpServiceService{}
	req := &dto.CallToolReq{Name: "echo", Arguments: map[string]interface{}{"msg": "hi"}}
	// 单价 0.01(展示货币)× QuotaPerUnit 500000 = 5000 quota;预扣下限 500 不生效
	const priceQuota = 5000

	t.Run("charged_on_success", func(t *testing.T) {
		user, svc, item := createMarketplaceFixture(t, "u-paid", "user", 20000, url, "per_call", 0.01)
		res, err := s.CallTool(user.ID, svc.ID, req)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		log := lastManualTestLog(t)
		if log.BillingStatus != "charged" || log.QuotaConsumed != priceQuota {
			t.Fatalf("billing status=%s quota=%d, want charged/%d", log.BillingStatus, log.QuotaConsumed, priceQuota)
		}
		if log.MarketplaceItemID == nil || *log.MarketplaceItemID != item.ID {
			t.Fatalf("log item id = %v, want %d", log.MarketplaceItemID, item.ID)
		}
		if log.UnitPrice != 0.01 || log.BillingType != "per_call" || log.PriceScope != "service" {
			t.Fatalf("price snapshot wrong: %+v", log)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000-priceQuota || used != priceQuota {
			t.Fatalf("quota=%d used=%d, want %d/%d", quota, used, 20000-priceQuota, priceQuota)
		}
	})

	t.Run("refunded_on_upstream_failure", func(t *testing.T) {
		// 上游指向必拒端口:预扣后连接失败 → 全额退款,余额不变
		user, svc, _ := createMarketplaceFixture(t, "u-dead", "user", 20000, "http://127.0.0.1:1/mcp", "per_call", 0.01)
		res, err := s.CallTool(user.ID, svc.ID, req)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if !res.IsError {
			t.Fatalf("expected connection error result")
		}
		log := lastManualTestLog(t)
		if log.BillingStatus != "refunded" || log.QuotaConsumed != 0 {
			t.Fatalf("billing status=%s quota=%d, want refunded/0", log.BillingStatus, log.QuotaConsumed)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000 || used != 0 {
			t.Fatalf("quota=%d used=%d, want refunded to 20000/0", quota, used)
		}
	})

	t.Run("blocked_on_insufficient_quota", func(t *testing.T) {
		user, svc, _ := createMarketplaceFixture(t, "u-poor", "user", 100, url, "per_call", 0.01)
		res, err := s.CallTool(user.ID, svc.ID, req)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if !res.IsError || res.Error == "" {
			t.Fatalf("expected blocked error, got %+v", res)
		}
		log := lastManualTestLog(t)
		if log.BillingStatus != "blocked" {
			t.Fatalf("billing status=%s, want blocked", log.BillingStatus)
		}
		if quota, _ := userQuota(t, user.ID); quota != 100 {
			t.Fatalf("quota=%d, want unchanged 100", quota)
		}
	})

	t.Run("admin_not_exempt_by_default", func(t *testing.T) {
		// ChargeAdmin 默认 true:管理员同样计费,余额不足同样拒绝
		user, svc, _ := createMarketplaceFixture(t, "u-admin", "admin", 100, url, "per_call", 0.01)
		res, err := s.CallTool(user.ID, svc.ID, req)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		if !res.IsError {
			t.Fatalf("expected admin to be blocked, got %+v", res)
		}
		if log := lastManualTestLog(t); log.BillingStatus != "blocked" {
			t.Fatalf("billing status=%s, want blocked", log.BillingStatus)
		}
	})

	t.Run("skipped_when_free", func(t *testing.T) {
		user, svc, _ := createMarketplaceFixture(t, "u-free", "user", 20000, url, "free", 0)
		res, err := s.CallTool(user.ID, svc.ID, req)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
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
	})

	t.Run("skipped_when_billing_disabled", func(t *testing.T) {
		if err := model.UpdateOption("BillingEnabled", "false"); err != nil {
			t.Fatalf("disable billing: %v", err)
		}
		defer func() { _ = model.UpdateOption("BillingEnabled", "true") }()

		user, svc, _ := createMarketplaceFixture(t, "u-off", "user", 20000, url, "per_call", 0.01)
		res, err := s.CallTool(user.ID, svc.ID, req)
		if err != nil {
			t.Fatalf("CallTool: %v", err)
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
	})

	// 资源/提示测试对市场服务放开且不计费(与网关 resources/read、prompts/get 免费口径一致)
	t.Run("resource_test_free_for_marketplace", func(t *testing.T) {
		user, svc, _ := createMarketplaceFixture(t, "u-res", "user", 20000, url, "per_call", 0.01)
		res, err := s.ReadResource(user.ID, svc.ID, &dto.ReadResourceReq{URI: "memo://overview"})
		if err != nil {
			t.Fatalf("ReadResource: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		if log := lastManualTestLog(t); log.BillingStatus != "skipped" || log.Method != "resources/read" {
			t.Fatalf("log method=%s billing=%s, want resources/read/skipped", log.Method, log.BillingStatus)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000 || used != 0 {
			t.Fatalf("quota=%d used=%d, want untouched 20000/0", quota, used)
		}
	})

	t.Run("prompt_test_free_for_marketplace", func(t *testing.T) {
		user, svc, _ := createMarketplaceFixture(t, "u-prm", "user", 20000, url, "per_call", 0.01)
		res, err := s.GetPrompt(user.ID, svc.ID, &dto.GetPromptReq{Name: "review"})
		if err != nil {
			t.Fatalf("GetPrompt: %v", err)
		}
		if res.IsError {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		if log := lastManualTestLog(t); log.BillingStatus != "skipped" || log.Method != "prompts/get" {
			t.Fatalf("log method=%s billing=%s, want prompts/get/skipped", log.Method, log.BillingStatus)
		}
		if quota, used := userQuota(t, user.ID); quota != 20000 || used != 0 {
			t.Fatalf("quota=%d used=%d, want untouched 20000/0", quota, used)
		}
	})
}
