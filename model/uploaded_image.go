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
//
// V1.1: dedup is PER-USER. (user_id, storage_key) is the unique constraint, so
// different users each get their own row for identical bytes while sharing one
// blob on disk/bucket; the blob is refcounted via CountByKey (deleted only when
// the last row referencing it goes away). Status tracks the presigned-PUT slot
// lifecycle (pending → uploaded); multipart uploads insert straight to uploaded.
type UploadedImage struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID     int64     `json:"user_id" gorm:"not null;uniqueIndex:idx_uploaded_images_user_storage,priority:1"`
	StorageKey string    `json:"storage_key" gorm:"size:128;not null;uniqueIndex:idx_uploaded_images_user_storage,priority:2;index:idx_uploaded_images_storage_key_lookup"` // content-addressed "ab/<sha256>"
	MediaType  string    `json:"media_type" gorm:"size:64;not null"`               // sniffed magic-byte mime, e.g. image/png
	Size       int64     `json:"size" gorm:"not null"`                              // decoded byte length
	Backend    string    `json:"backend" gorm:"size:16;not null"`                   // "local" | "s3" — which backend holds the blob
	Status     string    `json:"status" gorm:"size:16;not null;default:'uploaded';index"` // pending | uploaded (V1.1 presigned-PUT slots)
	CreatedAt  time.Time `json:"created_at" gorm:"index"`                          // retention clock; reaped when older than now-retention
	UpdatedAt  time.Time `json:"updated_at"`
}

func (UploadedImage) TableName() string { return "uploaded_images" }

// V1.1 presigned-PUT slot lifecycle. Multipart uploads insert straight to
// Uploaded; only upload_image creates Pending slots (flipped by PutVisionFile).
const (
	UploadStatusPending  = "pending"
	UploadStatusUploaded = "uploaded"
)

func (u *UploadedImage) Insert() error { return DB.Create(u).Error }
func (u *UploadedImage) Update() error { return DB.Save(u).Error }

// GetUploadedImageByKey looks up a blob by its content-addressed storage key.
// V1.1 semantics: with per-user dedup multiple rows may share a key (one per
// user), so this returns ANY one of them — fine for the public file endpoint,
// which only needs to confirm the key exists and serve the (shared) blob. The
// dedup hit check uses GetUploadedImageByUserAndKey instead.
// Returns gorm.ErrRecordNotFound when absent.
func GetUploadedImageByKey(key string) (*UploadedImage, error) {
	var img UploadedImage
	err := DB.Where("storage_key = ?", key).First(&img).Error
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// GetUploadedImageByID loads a row by primary key. Used by the delete handlers
// to owner-check and to read the storage key for blob refcounting.
func GetUploadedImageByID(id int64) (*UploadedImage, error) {
	var img UploadedImage
	if err := DB.First(&img, id).Error; err != nil {
		return nil, err
	}
	return &img, nil
}

// GetUploadedImageByUserAndKey is the V1.1 per-user dedup hit: same user +
// same content key → their existing row. Replaces the global lookup for dedup.
func GetUploadedImageByUserAndKey(userID int64, key string) (*UploadedImage, error) {
	var img UploadedImage
	err := DB.Where("user_id = ? AND storage_key = ?", userID, key).First(&img).Error
	if err != nil {
		return nil, err
	}
	return &img, nil
}

// CountByKey is the blob reference count: how many rows point at this content
// key. Called after deleting a row to decide whether to delete the shared blob
// (0 → safe to delete; >0 → another user still references it). See §15.1.
func CountByKey(key string) (int64, error) {
	var n int64
	err := DB.Model(&UploadedImage{}).Where("storage_key = ?", key).Count(&n).Error
	return n, err
}

// MarkUploaded flips a pending presigned-PUT slot to uploaded and records the
// sniffed media type + size after the PUT bytes land. Called by PutVisionFile.
func MarkUploaded(key, mediaType string, size int64) error {
	return DB.Model(&UploadedImage{}).Where("storage_key = ? AND status = ?", key, UploadStatusPending).
		Updates(map[string]interface{}{"status": UploadStatusUploaded, "media_type": mediaType, "size": size}).Error
}

// ListUploadsByUser returns a user's own uploads (uploaded only; pending slots
// are hidden from the management view), newest first, paginated.
func ListUploadsByUser(userID int64, offset, limit int) ([]UploadedImage, int64, error) {
	var imgs []UploadedImage
	var total int64
	q := DB.Model(&UploadedImage{}).Where("user_id = ? AND status = ?", userID, UploadStatusUploaded)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&imgs).Error; err != nil {
		return nil, 0, err
	}
	return imgs, total, nil
}

// ListAllUploads is the admin view: all users' uploads (optionally filtered to
// one user), uploaded only, newest first, paginated.
func ListAllUploads(filterUserID int64, offset, limit int) ([]UploadedImage, int64, error) {
	var imgs []UploadedImage
	var total int64
	q := DB.Model(&UploadedImage{}).Where("status = ?", UploadStatusUploaded)
	if filterUserID > 0 {
		q = q.Where("user_id = ?", filterUserID)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&imgs).Error; err != nil {
		return nil, 0, err
	}
	return imgs, total, nil
}

// CountUploadsByUser is the per-user active-upload count for the
// MaxUploadsPerUser guardrail.
func CountUploadsByUser(userID int64) (int64, error) {
	var n int64
	err := DB.Model(&UploadedImage{}).Where("user_id = ? AND status = ?", userID, UploadStatusUploaded).Count(&n).Error
	return n, err
}

// ListPendingExpired returns pending slots older than the cutoff for the V1.1
// fast-reap sweep (upload_image created the slot but the model never ran curl).
func ListPendingExpired(olderThan time.Time, limit int) ([]UploadedImage, error) {
	var imgs []UploadedImage
	q := DB.Where("status = ? AND created_at < ?", UploadStatusPending, olderThan).Order("created_at ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&imgs).Error; err != nil {
		return nil, err
	}
	return imgs, nil
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
