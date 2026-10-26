// Package videoasset saves bounded video streams using the shared blob backend.
package videoasset

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
)

type Result struct {
	Key       string
	MIME      string
	Size      int64
	ExpiresAt *time.Time
}

func MaxBytes() int64 {
	if n := model.GetOptionInt64("VideoGenMaxBytes"); n > 0 {
		return n
	}
	return 256 << 20
}

func Save(ctx context.Context, store storage.Storage, uid int64, taskID string, r io.Reader) (*Result, error) {
	if store == nil {
		return nil, fmt.Errorf("video storage not initialized")
	}
	f, err := os.CreateTemp("", "newmcp-video-*")
	if err != nil {
		return nil, err
	}
	defer func() { f.Close(); os.Remove(f.Name()) }()
	limit := MaxBytes()
	n, err := io.Copy(f, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read video: %w", err)
	}
	if n > limit {
		return nil, fmt.Errorf("video exceeds %d-byte limit", limit)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	head := make([]byte, 512)
	count, _ := io.ReadFull(f, head)
	mime := http.DetectContentType(head[:count])
	if mime != "video/mp4" && mime != "video/webm" {
		return nil, fmt.Errorf("video must be a recognized MP4 or WebM file")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("video/%d/%s", uid, taskID)
	if err := store.Put(ctx, key, f, mime); err != nil {
		return nil, fmt.Errorf("save video: %w", err)
	}
	var expiresAt *time.Time
	if retention := Retention(); retention > 0 {
		expires := time.Now().Add(retention)
		expiresAt = &expires
	}
	return &Result{Key: key, MIME: mime, Size: n, ExpiresAt: expiresAt}, nil
}

func Retention() time.Duration {
	if hours, err := strconv.Atoi(model.GetOptionString("VideoGenRetentionHours")); err == nil && hours >= 0 {
		return time.Duration(hours) * time.Hour
	}
	return 24 * time.Hour
}

// Zero disables expiry for files that still exist. Settings changes also renew
// stored deadlines from the change time, including previously permanent files.
func EffectiveExpiration(expiresAt *time.Time) *time.Time {
	if Retention() == 0 {
		return nil
	}
	return expiresAt
}

// The video-read domain permits GET and HEAD only. Expiry is authoritative in
// the task row, independently of the image URL's signing domain.
func Sign(sid string) string {
	secret := common.SessionSecret
	if secret == "" {
		secret = "default-secret-change-me"
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte("video-read|" + sid))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:12])
}

func Verify(sid, token string) bool {
	return token != "" && hmac.Equal([]byte(token), []byte(Sign(sid)))
}

func URL(sid string) string {
	return strings.TrimRight(model.GetOptionString("ServerAddress"), "/") + "/v/" + sid + "?s=" + Sign(sid)
}

// Interrupted processes cannot run their tempfile defer. Only our regular
// spool files older than the five-minute processing budget plus a large grace
// period are removed; no directory traversal or recursive deletion is used.
func CleanupTemps() {
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	count := 0
	for _, entry := range entries {
		if count >= 100 {
			return
		}
		if !strings.HasPrefix(entry.Name(), "newmcp-video-") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < time.Hour {
			continue
		}
		if os.Remove(filepath.Join(os.TempDir(), entry.Name())) == nil {
			count++
		}
	}
}
