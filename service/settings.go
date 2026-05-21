package service

import (
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
	return result
}

func (s *SettingsService) UpdateSetting(key string, value string) error {
	if model.IsSensitiveKey(key) && value == "***" {
		return nil
	}
	return model.UpdateOption(key, value)
}
