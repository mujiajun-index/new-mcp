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

	// Dedup: identical bytes → identical key. If a row exists, the blob is still
	// on disk (cleanup hasn't reaped it), so skip the Put, refresh retention,
	// and re-sign.
	if existing, err := model.GetUploadedImageByKey(key); err == nil && existing != nil {
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

	// New blob: write it, then record the row.
	if err := UploadStore.Put(ctx, key, bytes.NewReader(data), mediaType); err != nil {
		return nil, fmt.Errorf("store image: %w", err)
	}
	img := &model.UploadedImage{
		UserID:     userID,
		StorageKey: key,
		MediaType:  mediaType,
		Size:       int64(len(data)),
		Backend:    UploadStore.Backend(),
	}
	if err := img.Insert(); err != nil {
		// Two concurrent uploads of identical bytes can both miss the dedup
		// lookup and both Put; the loser fails the unique-key constraint.
		// Recover by treating the winner's row as the dedup hit and deleting
		// the redundant blob we just wrote.
		_ = UploadStore.Delete(ctx, key)
		if existing, e2 := model.GetUploadedImageByKey(key); e2 == nil && existing != nil {
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
