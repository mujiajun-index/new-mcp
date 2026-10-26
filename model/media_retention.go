package model

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MediaRetentionMutex keeps policy changes atomic with publishing new assets
// and deleting expired files. It does not guard unrelated task history limits.
var MediaRetentionMutex sync.RWMutex

// UpdateMediaRetentionOption renews existing files from the change time without
// changing creation timestamps. The option and asset updates commit together.
func UpdateMediaRetentionOption(key, value string) (bool, error) {
	switch key {
	case "UploadRetentionHours", "ImageGenRetentionHours", "VideoGenRetentionHours", "AudioGenRetentionHours":
	default:
		return false, nil
	}
	hours, err := strconv.ParseInt(value, 10, 64)
	if err != nil || hours < 0 || hours > 8760 {
		return true, fmt.Errorf("retention must be 0–8760 hours (0 = permanent)")
	}
	MediaRetentionMutex.Lock()
	defer MediaRetentionMutex.Unlock()
	oldHours, oldErr := strconv.ParseInt(GetOptionString(key), 10, 64)
	now := time.Now()
	var expires *time.Time
	if hours > 0 {
		deadline := now.Add(time.Duration(hours) * time.Hour)
		expires = &deadline
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		option := Option{Key: key, Value: value}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns([]string{"value"})}).Create(&option).Error; err != nil {
			return err
		}
		// Re-saving an unchanged value must not extend existing files again.
		if oldErr == nil && oldHours == hours {
			return nil
		}
		switch key {
		case "UploadRetentionHours", "ImageGenRetentionHours":
			q := tx.Model(&UploadedImage{}).Where("status = ?", UploadStatusUploaded)
			if key == "ImageGenRetentionHours" {
				q = q.Where("source = ?", ImageSourceGenerated)
			} else {
				q = q.Where("source <> ?", ImageSourceGenerated)
			}
			return q.UpdateColumn("retention_started_at", now).Error
		case "VideoGenRetentionHours":
			return tx.Model(&VideoGenTask{}).Where("artifact_status = ? AND storage_key <> ?", "ready", "").UpdateColumn("expires_at", expires).Error
		default:
			return tx.Model(&AudioAsset{}).Where("status = ? AND storage_key <> ?", "ready", "").UpdateColumn("expires_at", expires).Error
		}
	})
	if err == nil {
		OptionMapMutex.Lock()
		OptionMap[key] = value
		OptionMapMutex.Unlock()
	}
	return true, err
}
