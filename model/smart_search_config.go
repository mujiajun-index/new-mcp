package model

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

// SmartSearchConfig is the single platform-owned decision connection.
type SmartSearchConfig struct {
	ID          int64  `gorm:"primaryKey"`
	Enabled     bool   `gorm:"not null"`
	Provider    string `gorm:"size:32;not null"`
	EndpointURL string `gorm:"size:512;not null"`
	ModelName   string `gorm:"size:128;not null"`
	APIKey      string `gorm:"type:text"`
	AllGroups   bool   `gorm:"not null"`
	GroupsJSON  string `gorm:"type:text;not null"`
	BatchSize   int    `gorm:"not null"`
	Concurrency int    `gorm:"not null"`
	UpdatedAt   time.Time
}

func (SmartSearchConfig) TableName() string { return "smart_search_configs" }

func GetSmartSearchConfig() (*SmartSearchConfig, error) {
	var config SmartSearchConfig
	err := DB.First(&config, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &SmartSearchConfig{ID: 1, Provider: "typesafe", ModelName: "jev-latest", AllGroups: true, GroupsJSON: "[]", BatchSize: 200, Concurrency: 4}, nil
	}
	return &config, err
}

func SmartSearchAllowed(c *SmartSearchConfig, userID int64) (bool, error) {
	if !c.Enabled {
		return false, nil
	}
	if c.AllGroups {
		return true, nil
	}
	var groups []string
	if err := json.Unmarshal([]byte(c.GroupsJSON), &groups); err != nil {
		return false, err
	}
	if len(groups) == 0 {
		return false, nil
	}
	var user User
	if err := DB.Select("id", "group").First(&user, userID).Error; err != nil {
		return false, err
	}
	for _, g := range groups {
		if g == user.Group {
			return true, nil
		}
	}
	return false, nil
}
