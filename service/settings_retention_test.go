package service

import (
	"errors"
	"testing"

	"github.com/mujkjk/newmcp/model"
)

func TestPermanentFileRetentionSettings(t *testing.T) {
	db := imageGenTestSetup(t)
	if err := db.AutoMigrate(&model.McpCallLog{}); err != nil {
		t.Fatal(err)
	}
	s := &SettingsService{}
	for _, key := range []string{"UploadRetentionHours", "VideoGenRetentionHours"} {
		for _, value := range []string{"24", "0"} {
			if err := s.UpdateSetting(model.Operator{}, key, value); err != nil || model.GetOptionString(key) != value {
				t.Fatalf("setting %s=%s: %v", key, value, err)
			}
		}
		for _, invalid := range []string{"-1", "1.5", "", "permanent"} {
			if err := s.UpdateSetting(model.Operator{}, key, invalid); !errors.Is(err, ErrInvalidSetting) {
				t.Fatalf("accepted invalid retention %s=%q: %v", key, invalid, err)
			}
			if model.GetOptionString(key) != "0" {
				t.Fatal("invalid setting overwrote permanent retention")
			}
		}
	}
	for _, key := range []string{"VideoGenMaxBytes", "VideoGenHistoryDays"} {
		if err := s.UpdateSetting(model.Operator{}, key, "0"); !errors.Is(err, ErrInvalidSetting) {
			t.Fatal("unrelated positive-only video setting accepted zero", key, err)
		}
	}
}
