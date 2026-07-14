package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type SettingsService struct{}

func (s *SettingsService) GetAllSettings() []dto.SettingItem {
	model.OptionMapMutex.RLock()
	defer model.OptionMapMutex.RUnlock()

	items := make([]dto.SettingItem, 0, len(model.OptionMap))
	for k, v := range model.OptionMap {
		if model.IsSensitiveKey(k) {
			items = append(items, dto.SettingItem{Key: k, Value: "***"})
		} else {
			items = append(items, dto.SettingItem{Key: k, Value: v})
		}
	}
	return items
}

func (s *SettingsService) GetPublicSettings() map[string]string {
	model.OptionMapMutex.RLock()
	defer model.OptionMapMutex.RUnlock()

	result := make(map[string]string)
	for k := range model.OptionMap {
		if model.IsPublicKey(k) {
			result[k] = model.OptionMap[k]
		}
	}
	// Expose whether SMTP is configured so the frontend knows whether email
	// binding requires verification. Read inline (not via GetOption*) since we
	// already hold the read lock.
	result["SMTPConfigured"] = strconv.FormatBool(model.OptionMap["SMTPServer"] != "" && model.OptionMap["SMTPAccount"] != "")
	return result
}

func (s *SettingsService) UpdateSetting(key string, value string) error {
	if model.IsSensitiveKey(key) && value == "***" {
		return nil
	}
	if key == "UserGroupOptions" {
		if err := s.validateUserGroupOptions(value); err != nil {
			return err
		}
	}
	return model.UpdateOption(key, value)
}

// denyRemovalOfBoundGroups returns an error if any group present in oldKeys but
// absent from newKeys still has users bound to it. Adding or reordering groups
// is always allowed; only removal of a group that users belong to is blocked.
func denyRemovalOfBoundGroups(oldKeys, newKeys []string) error {
	newSet := make(map[string]bool, len(newKeys))
	for _, g := range newKeys {
		if g = strings.TrimSpace(g); g != "" {
			newSet[g] = true
		}
	}
	var removed []string
	for _, g := range oldKeys {
		if g = strings.TrimSpace(g); g != "" && !newSet[g] {
			removed = append(removed, g)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	inUse, err := model.GroupsInUse(removed)
	if err != nil {
		return err
	}
	if len(inUse) > 0 {
		return fmt.Errorf("以下分组仍有用户绑定，无法删除: %s", strings.Join(inUse, ", "))
	}
	return nil
}

// validateUserGroupOptions ensures that any group removed from the option list
// is not still bound to users.
func (s *SettingsService) validateUserGroupOptions(newValue string) error {
	return denyRemovalOfBoundGroups(model.GetUserGroupOptions(), strings.Split(newValue, ","))
}
