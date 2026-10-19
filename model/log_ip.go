package model

import (
	"encoding/json"

	"gorm.io/gorm"
)

const (
	LogIPTypeClient    = "client"
	LogIPTypeCloudPeer = "cloud_peer"
	LogIPTypeUnknown   = "unknown"
)

func inferLogIPType(clientIP, extra string) string {
	var metadata struct {
		Source     string `json:"source"`
		ManualTest bool   `json:"manual_test"`
	}
	if err := json.Unmarshal([]byte(extra), &metadata); err == nil {
		if metadata.Source == "cloud" {
			return LogIPTypeCloudPeer
		}
		if metadata.ManualTest {
			return LogIPTypeClient
		}
	}
	if clientIP != "" {
		return LogIPTypeClient
	}
	return LogIPTypeUnknown
}

// BeforeCreate also covers callers using DB.Create directly.
func (l *McpCallLog) BeforeCreate(_ *gorm.DB) error {
	if l.IPType == "" {
		l.IPType = inferLogIPType(l.ClientIP, l.Extra)
	}
	return nil
}

// Backfill only missing types in bounded batches without loading call payloads.
func backfillLogIPTypes() error {
	for {
		var rows []struct {
			ID       int64
			ClientIP string
			Extra    string
		}
		if err := DB.Model(&McpCallLog{}).Select("id", "client_ip", "extra").
			Where("ip_type = '' OR ip_type IS NULL").Order("id ASC").Limit(500).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		groups := map[string][]int64{}
		for _, row := range rows {
			kind := inferLogIPType(row.ClientIP, row.Extra)
			groups[kind] = append(groups[kind], row.ID)
		}
		for kind, ids := range groups {
			if err := DB.Model(&McpCallLog{}).Where("id IN ? AND (ip_type = '' OR ip_type IS NULL)", ids).
				UpdateColumn("ip_type", kind).Error; err != nil {
				return err
			}
		}
	}
}
