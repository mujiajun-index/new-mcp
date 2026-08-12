package model

import (
	"time"
)

// UploadedImage is the metadata row backing a vision image uploaded to storage.
// It tracks one content-addressed blob (see internal/storage): the same bytes
// always map to the same StorageKey, so an upload of known bytes can be turned
// into a no-op Put + a freshly signed URL by looking the key up here.
//
// Retention is dynamic: there is no ExpiresAt column. A row is reaped when
// created_at is older than (now - UploadRetentionHours), evaluated by the
// cleanup loop each tick. That means an admin LOWERING retention takes effect
// on the very next sweep — existing uploads granted a longer window get pulled
// in immediately — which is the behavior the design verification relies on.
// TouchRefresh extends a row's life by resetting created_at, so a re-upload of
// identical bytes restarts the clock.
//
// Like most models it has CreatedAt/UpdatedAt, but unlike them it has NO
// gorm.DeletedAt: rows are ephemeral and the cleanup loop hard-deletes both the
// row and the blob. A soft-delete tombstone would defeat content-addressed
// dedup — a re-upload of the same bytes after expiry would create a duplicate
// row instead of reviving the existing key. Hard delete keeps dedup clean.
type UploadedImage struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID     int64     `json:"user_id" gorm:"not null;index"`
	StorageKey string    `json:"storage_key" gorm:"size:128;not null;uniqueIndex"` // content-addressed "ab/<sha256>"
	MediaType  string    `json:"media_type" gorm:"size:64;not null"`               // sniffed magic-byte mime, e.g. image/png
	Size       int64     `json:"size" gorm:"not null"`                              // decoded byte length
	Backend    string    `json:"backend" gorm:"size:16;not null"`                   // "local" | "s3" — which backend holds the blob
	CreatedAt  time.Time `json:"created_at" gorm:"index"`                          // retention clock; reaped when older than now-retention
	UpdatedAt  time.Time `json:"updated_at"`
}

func (UploadedImage) TableName() string { return "uploaded_images" }

func (u *UploadedImage) Insert() error { return DB.Create(u).Error }
func (u *UploadedImage) Update() error { return DB.Save(u).Error }

// GetUploadedImageByKey looks up a blob by its content-addressed storage key.
// Used by both the upload dedup path (found → skip Put, refresh retention) and
// the public file endpoint (found → serve; not found → reaped/unknown → 404).
// Returns gorm.ErrRecordNotFound when absent.
func GetUploadedImageByKey(key string) (*UploadedImage, error) {
	var img UploadedImage
	err := DB.Where("storage_key = ?", key).First(&img).Error
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// TouchRefresh extends a row's retention window by resetting created_at to now.
// Called when an upload of identical bytes hits the dedup path, so a popular
// image stays alive from its most recent request rather than its first.
func TouchRefresh(id int64, now time.Time) error {
	return DB.Model(&UploadedImage{}).Where("id = ?", id).Update("created_at", now).Error
}

// ListExpiredUploads returns rows whose created_at is older than the cutoff
// (the cleanup loop passes now - UploadRetentionHours), oldest first, capped at
// limit. A bounded limit keeps each sweep's Storage.Delete fan-out predictable.
func ListExpiredUploads(olderThan time.Time, limit int) ([]UploadedImage, error) {
	var imgs []UploadedImage
	q := DB.Where("created_at < ?", olderThan).Order("created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&imgs).Error; err != nil {
		return nil, err
	}
	return imgs, nil
}

// DeleteUploadedImageByID hard-deletes a row. Paired with Storage.Delete in the
// cleanup loop — there is intentionally no soft-delete path (see struct comment).
func DeleteUploadedImageByID(id int64) error {
	return DB.Delete(&UploadedImage{}, id).Error
}
