package service

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

// UploadStore is the blob backend used for vision image uploads. It is set once
// during router.InitGateway (local disk by default, S3-compatible when
// configured) and read by UploadService and the public file endpoint.
var UploadStore storage.Storage

// UploadService handles vision image uploads: it reads bytes, content-addresses
// them, dedups against already-stored blobs, persists metadata, and returns a
// signed URL the caller can hand to a vision tool's image_url parameter.
type UploadService struct{}

// UploadResult is handed back to the uploader. The URL is what an MCP client or
// the web UI passes through to a vision tool; the upstream provider fetches the
// bytes itself, so they never enter the calling LLM's context.
type UploadResult struct {
	URL       string    `json:"url"`        // short-lived signed URL the upstream provider fetches
	ExpiresAt time.Time `json:"expires_at"` // when the URL stops resolving
	MediaType string    `json:"mime"`       // sniffed image media type
	Size      int64     `json:"size"`       // decoded byte length
	Key       string    `json:"key"`        // content-addressed storage key
	Backend   string    `json:"backend"`    // "local" | "s3"
	Deduped   bool      `json:"deduped"`    // true if bytes were already stored (no new write)
}

// Upload stores an image read from r and returns a short-lived signed URL plus
// metadata. maxBytes bounds the image size (the caller also bounds the HTTP
// body); an over-limit image is rejected before any storage write. Identical
// bytes dedup: the existing blob is reused, its retention refreshed, and no
// second copy is written.
func (s *UploadService) Upload(ctx context.Context, userID int64, r io.Reader, maxBytes int64) (*UploadResult, error) {
	if UploadStore == nil {
		return nil, fmt.Errorf("upload storage not initialized")
	}
	if maxBytes <= 0 {
		maxBytes = 10 << 20 // 10 MiB failsafe
	}
	if err := CheckUploadQuota(userID); err != nil {
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

	mediaType := vision.SniffMediaType(data)
	if mediaType == "" {
		return nil, fmt.Errorf("unsupported or unrecognized image format")
	}

	sum := sha256.Sum256(data)
	key := storage.ContentKey(hex.EncodeToString(sum[:]))

	ttl := signedURLTTL()
	now := time.Now()

	// Per-user dedup (V1.1): same user + same content key → their existing row.
	// Refresh its retention and re-sign; the blob is still on disk (cleanup
	// hasn't reaped it). Different users with identical bytes each get their own
	// row sharing one blob (handled below via CountByKey).
	if existing, err := model.GetUploadedImageByUserAndKey(userID, key); err == nil && existing != nil {
		_ = model.TouchRefresh(existing.ID, now) // extend retention from this re-upload
		url, err := UploadStore.PublicURL(ctx, key, ttl)
		if err != nil {
			return nil, err
		}
		return &UploadResult{
			URL:       url,
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
		if err := UploadStore.Put(ctx, key, bytes.NewReader(data), mediaType); err != nil {
			return nil, fmt.Errorf("store image: %w", err)
		}
	}
	img := &model.UploadedImage{
		UserID:     userID,
		StorageKey: key,
		MediaType:  mediaType,
		Size:       int64(len(data)),
		Backend:    UploadStore.Backend(),
		Status:     model.UploadStatusUploaded,
	}
	if err := img.Insert(); err != nil {
		// Concurrent same-user+key insert: both missed the dedup lookup, the
		// winner's row now owns the (idempotent) blob. Recover as a dedup hit.
		// Do NOT delete the blob — it is shared/refcounted and the winner points at it.
		if existing, e2 := model.GetUploadedImageByUserAndKey(userID, key); e2 == nil && existing != nil {
			url, e3 := UploadStore.PublicURL(ctx, key, ttl)
			if e3 != nil {
				return nil, e3
			}
			return &UploadResult{
				URL:       url,
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

	url, err := UploadStore.PublicURL(ctx, key, ttl)
	if err != nil {
		return nil, err
	}
	return &UploadResult{
		URL:       url,
		ExpiresAt: now.Add(ttl),
		MediaType: mediaType,
		Size:      int64(len(data)),
		Key:       key,
		Backend:   UploadStore.Backend(),
		Deduped:   false,
	}, nil
}

func signedURLTTL() time.Duration {
	if secs := model.GetOptionInt("SignedURLTTLSeconds"); secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return time.Hour
}

// presignedPutTTL is the lifetime of a presigned PUT URL (the shell direct-upload
// path). Shorter than signedURLTTL: the GET URL handed to analyze must outlive
// the upload window. Used by the cleanup loop's pending fast-reap too.
func presignedPutTTL() time.Duration {
	if secs := model.GetOptionInt("PresignedPutTTLSeconds"); secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 10 * time.Minute
}

// CheckUploadQuota returns an error if the user has reached MaxUploadsPerUser
// (0 = unlimited). Shared by the multipart and presigned-PUT upload paths so
// both guardrails count against the same per-user active-upload limit.
func CheckUploadQuota(userID int64) error {
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
