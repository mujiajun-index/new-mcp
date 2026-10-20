package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func assertAPITime(t *testing.T, field, value string, want time.Time) {
	t.Helper()
	got, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || !got.Equal(want) {
		t.Fatalf("%s=%q want instant=%s: %v", field, value, want.Format(time.RFC3339Nano), err)
	}
	_, gotOffset := got.Zone()
	_, wantOffset := want.Zone()
	if gotOffset != wantOffset {
		t.Fatalf("%s=%q: offset=%d want=%d", field, value, gotOffset, wantOffset)
	}
}

func TestUserAndAPIKeyTimeZones(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "timezones.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&model.User{}, &model.ApiKey{}, &model.McpGroup{}); err != nil {
		t.Fatal(err)
	}

	for i, zone := range []*time.Location{time.UTC, time.FixedZone("CST", 8*60*60), time.FixedZone("UTC-8", -8*60*60)} {
		t.Run(zone.String(), func(t *testing.T) {
			created := time.Date(2026, 10, 19, 6, 14, 59, 0, time.UTC).In(zone)
			login := created.Add(time.Hour)
			expires := created.Add(48 * time.Hour)
			user := model.User{ID: int64(100 + i), Username: fmt.Sprintf("timezone-%d", i), Password: "test", Role: common.RoleCommonUser, AffCode: fmt.Sprintf("timezone-code-%d", i), CreatedAt: created, LastLoginAt: &login}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			profile, err := (&AuthService{}).GetProfile(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertAPITime(t, "profile.created_at", profile.CreatedAt, created)
			detail, err := (&AdminService{}).GetUserDetail(common.RoleSuperAdmin, user.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertAPITime(t, "user.created_at", detail.CreatedAt, created)
			assertAPITime(t, "user.last_login_at", detail.LastLoginAt, login)
			users, total, err := (&AdminService{}).ListUsers(common.RoleSuperAdmin, 1, 20, user.Username, "", 0, false)
			if err != nil || total != 1 || len(users) != 1 {
				t.Fatalf("list users: total=%d items=%d err=%v", total, len(users), err)
			}
			assertAPITime(t, "users.created_at", users[0].CreatedAt, created)
			assertAPITime(t, "users.last_login_at", users[0].LastLoginAt, login)

			key := model.ApiKey{UserID: user.ID, Name: "time-test", KeyHash: fmt.Sprintf("test-hash-%d", i), KeyPrefix: "test", CreatedAt: created, LastUsedAt: &login, ExpiresAt: &expires}
			if err := key.Insert(); err != nil {
				t.Fatal(err)
			}
			keys, total, err := (&ApiKeyService{}).List(user.ID, "", 1, 20)
			if err != nil || total != 1 || len(keys) != 1 {
				t.Fatalf("list keys: total=%d items=%d err=%v", total, len(keys), err)
			}
			assertAPITime(t, "key.created_at", keys[0].CreatedAt, created)
			assertAPITime(t, "key.last_used_at", keys[0].LastUsedAt, login)
			assertAPITime(t, "key.expires_at", keys[0].ExpiresAt, expires)

			group := model.McpGroup{UserID: user.ID, Name: user.Username, Status: common.StatusEnabled}
			if err := db.Create(&group).Error; err != nil {
				t.Fatal(err)
			}
			expiry := expires.Format(time.RFC3339)
			result, err := (&ApiKeyService{}).Create(user.ID, &dto.CreateApiKeyReq{Name: "new-key", Groups: []string{group.Name}, ExpiresAt: &expiry})
			if err != nil {
				t.Fatal(err)
			}
			assertAPITime(t, "new_key.expires_at", result.ExpiresAt, expires)
		})
	}
}

func TestDetailTimeZones(t *testing.T) {
	for _, zone := range []*time.Location{time.UTC, time.FixedZone("CST", 8*60*60), time.FixedZone("UTC-8", -8*60*60)} {
		t.Run(zone.String(), func(t *testing.T) {
			stamp := time.Date(2026, 10, 19, 6, 14, 59, 0, time.UTC).In(zone)
			camera := (&CameraService{}).toDetail(&model.Camera{CreatedAt: stamp, UpdatedAt: stamp, StreamKey: "test", StreamKeyExpiresAt: &stamp})
			vision := (&VisionService{}).toDetail(&model.VisionConfig{CreatedAt: stamp, UpdatedAt: stamp})
			connection := (&ConnectionService{}).toDetail(&model.CloudEndpoint{TokenExpiresAt: &stamp, LastConnectedAt: &stamp})
			service := (&McpServiceService{}).toDetail(&model.McpService{CreatedAt: stamp, ToolsUpdatedAt: &stamp, LastHealthCheck: &stamp, Config: "{}"})
			redemption := toRedemptionItem(&model.Redemption{CreatedAt: stamp, RedeemedAt: &stamp})
			group := (&MarketplaceGroupService{}).toItem(&model.MarketplaceGroup{CreatedAt: stamp})
			tag := (&MarketplaceTagService{}).toItem(&model.MarketplaceTag{CreatedAt: stamp})
			for field, value := range map[string]string{
				"camera.created_at": camera.CreatedAt, "camera.updated_at": camera.UpdatedAt, "camera.key_expires_at": camera.StreamKeyExpiresAt,
				"vision.created_at": vision.CreatedAt, "vision.updated_at": vision.UpdatedAt,
				"connection.token_expires_at": connection.TokenExpiresAt, "connection.last_connected_at": connection.LastConnectedAt,
				"service.created_at": service.CreatedAt, "service.tools_updated_at": service.ToolsUpdatedAt, "service.last_health_check": service.LastHealthCheck,
				"redemption.created_at": redemption.CreatedAt, "redemption.redeemed_at": redemption.RedeemedAt,
				"marketplace_group.created_at": group.CreatedAt, "marketplace_tag.created_at": tag.CreatedAt,
			} {
				assertAPITime(t, field, value, stamp)
			}
		})
	}
}
