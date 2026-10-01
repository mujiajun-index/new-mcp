package service

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/model"
)

func setupPassiveTest(t *testing.T) *McpServiceService {
	t.Helper()
	setupConfigMaskTest(t)
	if err := model.DB.AutoMigrate(&model.User{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{}); err != nil {
		t.Fatal(err)
	}
	oldPool, oldSecret := SessionPool, common.CryptoSecret
	common.CryptoSecret = "passive-test-credential-encryption"
	SessionPool = bridge.NewSessionPool()
	t.Cleanup(func() {
		SessionPool.CloseAll()
		SessionPool, common.CryptoSecret = oldPool, oldSecret
	})
	if err := model.DB.Create(&model.User{ID: 1, Username: "passive-owner", Status: common.StatusEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateOption("ServerAddress", "https://mcp.example.test"); err != nil {
		t.Fatal(err)
	}
	return &McpServiceService{}
}

func createPassiveTest(t *testing.T, s *McpServiceService) (*dto.ServiceDetail, string) {
	t.Helper()
	detail, err := s.Create(1, &dto.CreateServiceReq{Name: "local_tools", TransportType: common.TransportPassiveWS,
		Config: map[string]interface{}{"url": "ws://old-placeholder"}, AuthType: "bearer"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(detail.PassiveURL)
	if err != nil || u.Scheme != "wss" || u.Path != "/mcp/passive/" {
		t.Fatalf("invalid endpoint: %q, %v", detail.PassiveURL, err)
	}
	return detail, u.Query().Get("token")
}

func TestPassiveRegistrationCredentialsAndOwnership(t *testing.T) {
	s := setupPassiveTest(t)
	detail, token := createPassiveTest(t, s)
	if detail.PassiveConnected || detail.AuthType != "none" || len(detail.Config) != 0 {
		t.Fatalf("passive registration used outbound configuration: %+v", detail)
	}
	if got := s.testPassive(&model.McpService{ID: detail.ID, Status: 1}); got.Connected || got.Error != "等待服务接入" {
		t.Fatalf("offline test: %+v", got)
	}
	svc, err := ValidatePassiveToken(token)
	if err != nil || svc.ID != detail.ID {
		t.Fatalf("valid token rejected: %v", err)
	}
	if strings.Contains(svc.PassiveToken, strings.Split(token, ".")[1]) {
		t.Fatal("stored credential was not encrypted")
	}
	for _, invalid := range []string{"", "login-token", "1.wrong", "0." + strings.Split(token, ".")[1], token + ".extra"} {
		if _, err := ValidatePassiveToken(invalid); !errors.Is(err, ErrPassiveCredentials) {
			t.Fatalf("invalid token accepted: %v", err)
		}
	}
	if _, err := s.GetByID(2, detail.ID); err == nil {
		t.Fatal("another user accessed endpoint")
	}
	if _, err := s.ResetPassiveToken(2, detail.ID); err == nil {
		t.Fatal("another user reset endpoint")
	}
	replacement, err := s.ResetPassiveToken(1, detail.ID)
	if err != nil || replacement.PassiveURL == detail.PassiveURL {
		t.Fatalf("reset did not generate a new endpoint: %v", err)
	}
	if _, err := ValidatePassiveToken(token); !errors.Is(err, ErrPassiveCredentials) {
		t.Fatal("old token remained valid after reset")
	}
	u, _ := url.Parse(replacement.PassiveURL)
	if _, err := ValidatePassiveToken(u.Query().Get("token")); err != nil {
		t.Fatal(err)
	}
}

func TestPassiveDisabledAndDeletedServicesRejectAccess(t *testing.T) {
	s := setupPassiveTest(t)
	detail, token := createPassiveTest(t, s)
	status := 0
	if err := s.Update(1, detail.ID, &dto.UpdateServiceReq{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePassiveToken(token); !errors.Is(err, ErrPassiveDisabled) {
		t.Fatalf("disabled service allowed access: %v", err)
	}
	status = 1
	if err := s.Update(1, detail.ID, &dto.UpdateServiceReq{Status: &status}); err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Model(&model.User{}).Where("id = 1").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePassiveToken(token); !errors.Is(err, ErrPassiveDisabled) {
		t.Fatalf("disabled owner allowed access: %v", err)
	}
	if err := model.DB.Model(&model.User{}).Where("id = 1").Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(1, detail.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePassiveToken(token); !errors.Is(err, ErrPassiveCredentials) {
		t.Fatalf("deleted service allowed access: %v", err)
	}
}

func TestPassiveStartupBackfillPreservesCatalogAndCredential(t *testing.T) {
	s := setupPassiveTest(t)
	legacy := &model.McpService{UserID: 1, Name: "legacy", TransportType: common.TransportPassiveWS,
		PassiveConnected: true, ToolsCache: `[{"name":"kept"}]`, Config: `{"url":"wss://obsolete"}`,
		AuthType: "bearer", Status: 1, HealthStatus: common.HealthHealthy}
	if err := legacy.Insert(); err != nil {
		t.Fatal(err)
	}
	if err := InitializePassiveServices(); err != nil {
		t.Fatal(err)
	}
	row, _ := model.GetServiceByID(1, legacy.ID)
	if row.PassiveToken == "" || row.PassiveConnected || row.ToolsCache != legacy.ToolsCache || row.Config != "{}" || row.AuthType != "none" {
		t.Fatalf("incorrect placeholder upgrade: %+v", row)
	}
	first, err := s.GetByID(1, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := InitializePassiveServices(); err != nil {
		t.Fatal(err)
	}
	second, _ := s.GetByID(1, legacy.ID)
	if first.PassiveURL != second.PassiveURL {
		t.Fatal("restart rotated an existing credential")
	}
}

func TestPassiveEndpointAddressAndConfig(t *testing.T) {
	s := setupPassiveTest(t)
	if err := model.UpdateOption("ServerAddress", "http://localhost:3000/prefix/"); err != nil {
		t.Fatal(err)
	}
	detail, err := s.Create(1, &dto.CreateServiceReq{Name: "dev", TransportType: common.TransportPassiveWS})
	if err != nil || !strings.HasPrefix(detail.PassiveURL, "ws://localhost:3000/prefix/mcp/passive/?token=") {
		t.Fatalf("development endpoint: %+v, %v", detail, err)
	}
	name, auth := "edited", "bearer"
	if err := s.Update(1, detail.ID, &dto.UpdateServiceReq{DisplayName: &name, Config: map[string]interface{}{"url": "ignored"}, AuthType: &auth}); err != nil {
		t.Fatal(err)
	}
	updated, _ := s.GetByID(1, detail.ID)
	if updated.DisplayName != name || updated.PassiveURL != detail.PassiveURL || len(updated.Config) != 0 || updated.AuthType != "none" {
		t.Fatalf("display edit changed connection: %+v", updated)
	}
	if err := model.UpdateOption("ServerAddress", "invalid-address"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(1, &dto.CreateServiceReq{Name: "invalid", TransportType: common.TransportPassiveWS}); err == nil {
		t.Fatal("invalid server address accepted")
	}
	if result, err := s.TestConnection(&dto.TestConnectionReq{TransportType: common.TransportPassiveWS}); err != nil || result.Connected || result.Error == "" {
		t.Fatalf("passive preconnection test: %+v %v", result, err)
	}
}

func TestAdminPassiveRegistrationInMarketplaceOnlyMode(t *testing.T) {
	s := setupPassiveTest(t)
	if err := model.UpdateOption("UserOwnedServicesEnabled", "false"); err != nil {
		t.Fatal(err)
	}
	detail, err := s.CreateAdminService(1, &dto.CreateServiceReq{
		Name: "platform_local", TransportType: common.TransportPassiveWS,
		Config: map[string]interface{}{"url": "ws://obsolete"}, AuthType: "bearer",
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := url.Parse(detail.PassiveURL)
	if err != nil || endpoint.Scheme != "wss" || detail.PassiveConnected || detail.AuthType != "none" || len(detail.Config) != 0 {
		t.Fatalf("invalid admin passive endpoint: %+v, %v", detail, err)
	}
	svc, err := ValidatePassiveToken(endpoint.Query().Get("token"))
	if err != nil || svc.Source != "admin" || svc.PassiveToken == "" {
		t.Fatalf("admin passive endpoint rejected in marketplace-only mode: %+v, %v", svc, err)
	}
	if _, err := s.Create(1, &dto.CreateServiceReq{Name: "user_local", TransportType: common.TransportPassiveWS}); err == nil {
		t.Fatal("marketplace-only mode allowed a user-owned registration")
	}
	if _, err := s.CreateAdminService(1, &dto.CreateServiceReq{
		Name: "platform_keys", TransportType: common.TransportPassiveWS, KeyMode: "round_robin",
	}); err == nil {
		t.Fatal("admin passive registration allowed upstream key configuration")
	}
}

func TestUserPassiveEndpointRespectsMarketplaceOnlyMode(t *testing.T) {
	s := setupPassiveTest(t)
	_, token := createPassiveTest(t, s)
	if err := model.UpdateOption("UserOwnedServicesEnabled", "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePassiveToken(token); !errors.Is(err, ErrPassiveDisabled) {
		t.Fatalf("user-owned passive endpoint bypassed marketplace-only mode: %v", err)
	}
}
