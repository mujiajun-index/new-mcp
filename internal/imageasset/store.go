package imageasset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/vision"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
)

// Result describes an uploaded or generated image in the shared blob store.
type Result struct {
	URL       string    `json:"url"`        // short-lived signed URL the upstream provider fetches
	ExpiresAt time.Time `json:"expires_at"` // when the URL stops resolving
	MediaType string    `json:"mime"`       // sniffed image media type
	Size      int64     `json:"size"`       // decoded byte length
	Key       string    `json:"key"`        // content-addressed storage key
	Backend   string    `json:"backend"`    // "local" | "s3"
	Deduped   bool      `json:"deduped"`    // true if bytes were already stored (no new write)
}

// Save stores an image read from r and returns a short-lived signed URL plus
// metadata. maxBytes bounds the image size (the caller also bounds the HTTP
// body); an over-limit image is rejected before any storage write. Identical
// bytes dedup: the existing blob is reused, its retention refreshed, and no
// second copy is written.
func Save(ctx context.Context, store storage.Storage, userID int64, r io.Reader, maxBytes int64) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("upload storage not initialized")
	}
	if maxBytes <= 0 {
		maxBytes = 10 << 20 // 10 MiB failsafe
	}
	if err := CheckQuota(userID); err != nil {
		return nil, err
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

	ttl := Retention()
	now := time.Now()

	// Per-user dedup (V1.1): same user + same content key → their existing row.
	// Refresh its retention and re-sign; the blob is still on disk (cleanup
	// hasn't reaped it). Different users with identical bytes each get their own
	// row sharing one blob (handled below via CountByKey).
	if existing, err := model.GetUploadedImageByUserAndKey(userID, key); err == nil && existing != nil {
		if err := model.TouchRefresh(existing.ID, now); err != nil {
			return nil, fmt.Errorf("refresh image retention: %w", err)
		}
		return &Result{
			URL:       storage.ShortURL("GET", existing.ShortID),
			ExpiresAt: now.Add(ttl),
			MediaType: existing.MediaType,
			Size:      existing.Size,
			Key:       key,
			Backend:   existing.Backend,
			Deduped:   true,
		}, nil
	}

	// Blob-level dedup: even for a new (user, key) row, the blob may already
	// exist (another user uploaded identical bytes). Skip the write then —
	// storage keeps one copy, refcounted via CountByKey on delete.
	if n, err := model.CountByKey(key); err != nil || n == 0 {
		if err := store.Put(ctx, key, bytes.NewReader(data), mediaType); err != nil {
			return nil, fmt.Errorf("store image: %w", err)
		}
	}
	img := &model.UploadedImage{
		UserID:     userID,
		StorageKey: key,
		MediaType:  mediaType,
		Size:       int64(len(data)),
		Backend:    store.Backend(),
		Status:     model.UploadStatusUploaded,
	}
	if err := img.InsertWithGeneratedShortID(); err != nil {
		// Concurrent same-user+key insert: both missed the dedup lookup, the
		// winner's row now owns the (idempotent) blob. Recover as a dedup hit.
		// Do NOT delete the blob — it is shared/refcounted and the winner points at it.
		if existing, e2 := model.GetUploadedImageByUserAndKey(userID, key); e2 == nil && existing != nil {
			return &Result{
				URL:       storage.ShortURL("GET", existing.ShortID),
				ExpiresAt: now.Add(ttl),
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
		URL:       storage.ShortURL("GET", img.ShortID),
		ExpiresAt: now.Add(ttl),
		MediaType: mediaType,
		Size:      int64(len(data)),
		Key:       key,
		Backend:   store.Backend(),
		Deduped:   false,
	}, nil
}

func Retention() time.Duration {
	if hours := model.GetOptionInt("UploadRetentionHours"); hours > 0 {
		return time.Duration(hours) * time.Hour
	}
	return 24 * time.Hour
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
