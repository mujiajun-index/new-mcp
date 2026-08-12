package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mujkjk/newmcp/model"
)

// localStorage keeps uploaded blobs on the local filesystem under
// {root}/{pathPrefix}/{key}. It is the default backend (zero external deps).
// PublicURL does not produce a "remote" URL — it signs a URL that points back
// at new-mcp's own GET /api/v1/vision/files/*key endpoint, which reads via Get.
type localStorage struct {
	root       string
	pathPrefix string
}

func newLocal(cfg Config) (Storage, error) {
	root := cfg.LocalRoot
	if root == "" {
		root = "./data/uploads"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root %q: %w", root, err)
	}
	prefix := cfg.PathPrefix
	if prefix == "" {
		prefix = "vision"
	}
	return &localStorage{root: root, pathPrefix: prefix}, nil
}

func (s *localStorage) Backend() string { return "local" }

// fullpath validates key and returns its absolute on-disk path, guarding against
// path traversal. Keys are content-addressed hex shards ("ab/abcdef..."), so in
// practice they never contain ".." or absolutes — but we defend in depth and
// verify the resolved path stays under root/{prefix}.
func (s *localStorage) fullpath(key string) (string, error) {
	clean := strings.Trim(key, "/")
	if clean == "" || strings.Contains(clean, "..") || strings.ContainsAny(clean, `\`) || strings.Contains(clean, "//") {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	full := filepath.Join(s.root, s.pathPrefix, clean)

	absRoot, err := filepath.Abs(filepath.Join(s.root, s.pathPrefix))
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if absFull != absRoot && !strings.HasPrefix(absFull, absRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid storage key %q escapes root", key)
	}
	return full, nil
}

func (s *localStorage) Put(ctx context.Context, key string, r io.Reader, mimeType string) error {
	full, err := s.fullpath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write storage object: %w", err)
	}
	return nil
}

func (s *localStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	full, err := s.fullpath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return f, nil
}

func (s *localStorage) Delete(ctx context.Context, key string) error {
	full, err := s.fullpath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		if os.IsNotExist(err) {
			return nil // idempotent
		}
		return err
	}
	return nil
}

// PublicURL builds a short-lived, HMAC-signed URL pointing back at new-mcp's own
// file endpoint. The key travels in the path (URL-safe hex + "/"), expiry and
// token in the query. ServerAddress is read fresh each call so an admin changing
// it takes effect on the next upload without restarting.
func (s *localStorage) PublicURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	base := strings.TrimRight(model.GetOptionString("ServerAddress"), "/")
	expires := time.Now().Add(ttl).Unix()
	token := SignURL(key, expires)
	return fmt.Sprintf("%s/api/v1/vision/files/%s?expires=%d&token=%s", base, key, expires, token), nil
}
