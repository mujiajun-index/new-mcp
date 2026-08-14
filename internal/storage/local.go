package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// localStorage keeps uploaded blobs on the local filesystem under
// {root}/{pathPrefix}/{key}. It is the default backend (zero external deps).
// Files are served via Get through the short-URL endpoint /u/<sid>.
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
	// Best-effort prune of the now-empty shard directory (e.g. "ab/" for a key
	// like "ab/abcd..."). os.Remove only succeeds when the dir is empty, so
	// blobs sharing the shard keep their dir; a concurrent Put that needs it
	// again re-creates it via MkdirAll. We prune only when filepath.Dir(full)
	// is strictly below {root}/{prefix} — never the prefix dir or root. The
	// guard also skips flat, shard-less keys (they do not occur in practice:
	// both ContentKey and the UUID path are "<2>/<rest>"), which would otherwise
	// collapse to the prefix dir.
	shardDir := filepath.Dir(full)
	if shardDir != filepath.Join(s.root, s.pathPrefix) {
		_ = os.Remove(shardDir)
	}
	return nil
}

// Stat returns on-disk size only; MediaType is left empty (the DB row sniffs and
// stores the real type at upload time). Missing file → ErrObjectNotFound.
func (s *localStorage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	full, err := s.fullpath(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	st, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectInfo{}, ErrObjectNotFound
		}
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: st.Size()}, nil
}
