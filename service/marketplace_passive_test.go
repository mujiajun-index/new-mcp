package service

import (
	"errors"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

func TestPassiveServiceExcludedFromClonePages(t *testing.T) {
	setupItemKeysTest(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []model.McpService{
		{UserID: 7, Name: "http", Source: "user", TransportType: common.TransportStreamableHTTP, CreatedAt: created},
		{UserID: 7, Name: "websocket", Source: "user", TransportType: common.TransportWebSocket, CreatedAt: created.Add(time.Second)},
		{UserID: 7, Name: "passive", Source: "user", TransportType: common.TransportPassiveWS, CreatedAt: created.Add(2 * time.Second)},
		{UserID: 8, Name: "other_owner", Source: "user", TransportType: common.TransportStreamableHTTP, CreatedAt: created.Add(3 * time.Second)},
		{UserID: 7, Name: "market_ref", Source: "marketplace", TransportType: "marketplace", CreatedAt: created.Add(4 * time.Second)},
	}
	if err := model.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	for offset, want := range []string{"websocket", "http", ""} {
		page, total, err := (&McpServiceService{}).ListClonableServices(7, offset+1, 1)
		if err != nil || total != 2 {
			t.Fatalf("page %d: total=%d err=%v", offset+1, total, err)
		}
		if want == "" {
			if len(page) != 0 {
				t.Fatalf("page beyond filtered total is not empty: %v", page)
			}
		} else if len(page) != 1 || page[0].Name != want {
			t.Fatalf("page %d: got=%v want=%s", offset+1, page, want)
		}
	}
}

func TestPassiveServiceCannotBeClonedToMarketplace(t *testing.T) {
	setupItemKeysTest(t)
	s := &MarketplaceService{}
	for _, source := range []string{"user", "admin"} {
		svc := &model.McpService{UserID: 7, Name: "passive_" + source, Source: source,
			TransportType: common.TransportPassiveWS, Config: "{}", PassiveToken: "source-credential",
			PassiveConnected: true, ToolsCache: `[{"name":"local_tool"}]`, Status: common.StatusEnabled}
		if err := model.DB.Create(svc).Error; err != nil {
			t.Fatal(err)
		}
		req := &dto.CloneMarketplaceReq{FromServiceID: svc.ID, Name: "market_" + source, BillingType: "free"}
		if result, err := s.CloneFromService(7, req); result != nil || !errors.Is(err, ErrPassiveServiceNotListable) {
			t.Fatalf("passive %s clone: result=%+v err=%v", source, result, err)
		}
		if _, err := s.CloneFromService(8, req); !errors.Is(err, ErrServiceNotOwned) {
			t.Fatalf("ownership validation was bypassed: %v", err)
		}
		fresh, err := model.GetServiceByID(7, svc.ID)
		if err != nil || fresh.PassiveToken != svc.PassiveToken || !fresh.PassiveConnected {
			t.Fatalf("rejected clone changed source tunnel: %+v %v", fresh, err)
		}
	}
	var count int64
	if err := model.DB.Model(&model.MarketplaceItem{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected clone created market rows: count=%d err=%v", count, err)
	}
}

func TestMarketplaceUpdateCannotConvertToPassiveTunnel(t *testing.T) {
	setupItemKeysTest(t)
	item := newItemWithTemplate(t, "existing_http", common.TransportStreamableHTTP, `{"url":"https://upstream.test/mcp"}`)
	passive, display := common.TransportPassiveWS, "not saved"
	err := (&MarketplaceService{}).UpdateItem(item.ID, &dto.UpdateMarketplaceItemReq{TransportType: &passive, DisplayName: &display})
	if !errors.Is(err, ErrPassiveServiceNotListable) {
		t.Fatalf("passive transport update accepted: %v", err)
	}
	fresh := reloadItem(t, item.ID)
	if fresh.TransportType != item.TransportType || fresh.DisplayName != item.DisplayName || fresh.ConfigTemplate != item.ConfigTemplate {
		t.Fatalf("rejected update changed market item: %+v", fresh)
	}
}
