package imageasset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Result describes an uploaded or generated image in the shared blob store.
type Result struct {
	UploadID  int64      `json:"-"`          // internal history reference; never returned by upload APIs
	URL       string     `json:"url"`        // short-lived signed URL the upstream provider fetches
	ExpiresAt *time.Time `json:"expires_at"` // nil means permanent retention
	MediaType string     `json:"mime"`       // sniffed image media type
	Size      int64      `json:"size"`       // decoded byte length
	Key       string     `json:"key"`        // content-addressed storage key
	Backend   string     `json:"backend"`    // "local" | "s3"
	Deduped   bool       `json:"deduped"`    // true if bytes were already stored (no new write)
}

// Save stores an image read from r and returns a short-lived signed URL plus
// metadata. maxBytes bounds the image size (the caller also bounds the HTTP
// body); an over-limit image is rejected before any storage write. Identical
// bytes dedup: the existing blob is reused, its retention refreshed, and no
// second copy is written.
func Save(ctx context.Context, store storage.Storage, userID int64, r io.Reader, maxBytes int64) (*Result, error) {
	return save(ctx, store, userID, r, maxBytes, model.ImageSourceUpload)
}

func SaveGenerated(ctx context.Context, store storage.Storage, userID int64, r io.Reader, maxBytes int64) (*Result, error) {
	return save(ctx, store, userID, r, maxBytes, model.ImageSourceGenerated)
}

// SaveReference stores validated task inputs without adding them to upload management.
func SaveReference(ctx context.Context, store storage.Storage, userID int64, r io.Reader, maxBytes int64) (*Result, error) {
	return save(ctx, store, userID, r, maxBytes, model.ImageSourceReference)
}

func refresh(ctx context.Context, existing *model.UploadedImage, now time.Time) error {
	return admitGenerated(ctx, existing.UserID, existing.Source == model.ImageSourceGenerated && Expired(existing.CreatedAt, existing.Source), func(db *gorm.DB) error {
		return db.Model(existing).Update("created_at", now).Error
	})
}

func save(ctx context.Context, store storage.Storage, userID int64, r io.Reader, maxBytes int64, source string) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("upload storage not initialized")
	}
	if maxBytes <= 0 {
		maxBytes = 10 << 20 // 10 MiB failsafe
	}

	// Read the whole image (bounded) so we can sniff + hash it. LimitReader(cap+1)
	// lets us detect an over-limit image by reading one byte past the cap.
	var buf bytes.Buffer
	n, err := io.Copy(&buf, io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	if n > maxBytes {
		return nil, fmt.Errorf("image exceeds the %d-byte limit", maxBytes)
	}
	data := buf.Bytes()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	mediaType := vision.SniffMediaType(data)
	if mediaType == "" {
		return nil, fmt.Errorf("unsupported or unrecognized image format")
	}

	sum := sha256.Sum256(data)
	key := storage.ContentKey(hex.EncodeToString(sum[:]))

	now := time.Now()

	// Per-user dedup (V1.1): same user + same content key → their existing row.
	// Refresh its retention and re-sign; the blob is still on disk (cleanup
	// hasn't reaped it). Different users with identical bytes each get their own
	// row sharing one blob (handled below via CountByKey).
	if existing, err := model.GetUploadedImageByUserAndKey(userID, key, source); err == nil && existing != nil {
		if err := refresh(ctx, existing, now); err != nil {
			return nil, fmt.Errorf("refresh image retention: %w", err)
		}
		return &Result{
			UploadID:  existing.ID,
			URL:       storage.ShortURL("GET", existing.ShortID),
			ExpiresAt: Expiration(now, source),
			MediaType: existing.MediaType,
			Size:      existing.Size,
			Key:       key,
			Backend:   existing.Backend,
			Deduped:   true,
		}, nil
	}
	if source == model.ImageSourceGenerated {
		if err := CheckGeneratedQuota(userID); err != nil {
			return nil, err
		}
	} else if err := CheckQuota(userID); err != nil {
		return nil, err
	}

	img := &model.UploadedImage{
		UserID:     userID,
		StorageKey: key,
		MediaType:  mediaType,
		Size:       int64(len(data)),
		Backend:    store.Backend(),
		Status:     model.UploadStatusUploaded,
		Source:     source,
	}
	if err := insertImage(ctx, store, img, data); err != nil {
		// Concurrent same-user+key insert: both missed the dedup lookup, the
		// winner's row now owns the (idempotent) blob. Recover as a dedup hit.
		// Do NOT delete the blob — it is shared/refcounted and the winner points at it.
		if existing, e2 := model.GetUploadedImageByUserAndKey(userID, key, source); e2 == nil && existing != nil {
			if err := refresh(ctx, existing, now); err != nil {
				return nil, fmt.Errorf("refresh image retention: %w", err)
			}
			return &Result{
				UploadID:  existing.ID,
				URL:       storage.ShortURL("GET", existing.ShortID),
				ExpiresAt: Expiration(now, source),
				MediaType: existing.MediaType,
				Size:      existing.Size,
				Key:       key,
				Backend:   existing.Backend,
				Deduped:   true,
			}, nil
		}
		return nil, fmt.Errorf("record upload metadata: %w", err)
	}

	return &Result{
		UploadID:  img.ID,
		URL:       storage.ShortURL("GET", img.ShortID),
		ExpiresAt: Expiration(now, source),
		MediaType: mediaType,
		Size:      int64(len(data)),
		Key:       key,
		Backend:   store.Backend(),
		Deduped:   false,
	}, nil
}

func Retention(source string) time.Duration {
	key := "UploadRetentionHours"
	if source == model.ImageSourceGenerated {
		key = "ImageGenRetentionHours"
	}
	if hours, err := strconv.Atoi(model.GetOptionString(key)); err == nil && hours >= 0 {
		return time.Duration(hours) * time.Hour
	}
	return 24 * time.Hour
}

func Expiration(createdAt time.Time, source string) *time.Time {
	if retention := Retention(source); retention > 0 {
		expires := createdAt.Add(retention)
		return &expires
	}
	return nil
}

func Expired(createdAt time.Time, source string) bool {
	expires := Expiration(createdAt, source)
	return expires != nil && !expires.After(time.Now())
}

// CheckQuota returns an error if the user has reached MaxUploadsPerUser
// (0 = unlimited). Shared by the multipart and presigned-PUT upload paths so
// both guardrails count against the same per-user active-upload limit.
func CheckQuota(userID int64) error {
	max := model.GetOptionInt64("MaxUploadsPerUser")
	if max <= 0 {
		return nil
	}
	n, err := model.CountUploadsByUser(userID)
	if err != nil {
		return fmt.Errorf("check upload quota: %w", err)
	}
	if n >= max {
		return fmt.Errorf("upload quota exceeded: %d/%d active uploads; delete old images or wait for cleanup", n, max)
	}
	return nil
}

// Recheck admission in a transaction so concurrent generation tasks cannot
// exceed the saved-image cap. User-row locking serializes server databases;
// SQLite serializes writes and rejects conflicting transactions.
func admitGenerated(ctx context.Context, uid int64, generated bool, save func(*gorm.DB) error) error {
	db := model.DB.WithContext(ctx)
	if !generated || model.GetOptionInt64("MaxImagesPerUser") <= 0 {
		return save(db)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() != "sqlite" {
			var user model.User
			if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, uid).Error; err != nil {
				return err
			}
		}
		if err := checkGeneratedQuota(tx, uid); err != nil {
			return err
		}
		return save(tx)
	})
}

func insertImage(ctx context.Context, store storage.Storage, img *model.UploadedImage, data []byte) error {
	return admitGenerated(ctx, img.UserID, img.Source == model.ImageSourceGenerated, func(db *gorm.DB) error {
		// All sources share the physical blob; only their metadata and policies differ.
		var count int64
		if err := db.Model(&model.UploadedImage{}).Where("storage_key = ?", img.StorageKey).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := store.Put(ctx, img.StorageKey, bytes.NewReader(data), img.MediaType); err != nil {
				return fmt.Errorf("store image: %w", err)
			}
		}
		sid, err := model.NewShortID()
		if err != nil {
			return err
		}
		img.ShortID = sid
		return db.Create(img).Error
	})
}

func CheckGeneratedQuota(userID int64) error { return checkGeneratedQuota(model.DB, userID) }

func checkGeneratedQuota(db *gorm.DB, userID int64) error {
	limit := model.GetOptionInt64("MaxImagesPerUser")
	if limit <= 0 {
		return nil
	}
	q := db.Model(&model.UploadedImage{}).Where("user_id = ? AND status = ? AND source = ?", userID, model.UploadStatusUploaded, model.ImageSourceGenerated)
	if retention := Retention(model.ImageSourceGenerated); retention > 0 {
		q = q.Where("created_at > ?", time.Now().Add(-retention))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return fmt.Errorf("check generated image quota: %w", err)
	}
	if count >= limit {
		return fmt.Errorf("generated image storage quota exceeded: %d/%d; delete old image tasks and retry saving", count, limit)
	}
	return nil
}
