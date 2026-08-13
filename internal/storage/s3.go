package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Storage stores blobs in any S3-compatible bucket (AWS S3, MinIO, Cloudflare
// R2, B2, Alibaba OSS, Tencent COS, …) via minio-go. Unlike localStorage, the
// public URL it hands out is a bucket presigned GET — the upstream vision
// provider fetches straight from the object store and never touches new-mcp.
type s3Storage struct {
	client     *minio.Client
	bucket     string
	pathPrefix string
}

// newS3 builds the S3 backend. Missing required config (endpoint/bucket/creds)
// is a hard error — per the design, selecting "s3" without full credentials must
// fail loudly at startup rather than silently fall back to local.
func newS3(ctx context.Context, cfg Config) (Storage, error) {
	var missing []string
	if cfg.Endpoint == "" {
		missing = append(missing, "StorageEndpoint")
	}
	if cfg.Bucket == "" {
		missing = append(missing, "StorageBucket")
	}
	if cfg.AccessKey == "" {
		missing = append(missing, "StorageAccessKey")
	}
	if cfg.SecretKey == "" {
		missing = append(missing, "StorageSecretKey")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("s3 backend selected but missing config: %s", strings.Join(missing, ", "))
	}
	prefix := cfg.PathPrefix
	if prefix == "" {
		prefix = "vision"
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}

	// Fail fast on a misconfigured bucket: better at startup than on the first
	// upload (which would surface as a generic put error mid-request).
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("reach s3 bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		return nil, fmt.Errorf("s3 bucket %q does not exist", cfg.Bucket)
	}
	return &s3Storage{client: client, bucket: cfg.Bucket, pathPrefix: prefix}, nil
}

func (s *s3Storage) Backend() string { return "s3" }

// objectName turns a content key into the bucket object name "<prefix>/<key>".
// S3 keys are opaque strings (no path traversal), but we still reject empty/.. /
// double-slash keys for parity with local and to keep keys clean.
func (s *s3Storage) objectName(key string) (string, error) {
	clean := strings.Trim(key, "/")
	if clean == "" || strings.Contains(clean, "..") || strings.Contains(clean, "//") {
		return "", fmt.Errorf("invalid storage key %q", key)
	}
	return s.pathPrefix + "/" + clean, nil
}

func (s *s3Storage) Put(ctx context.Context, key string, r io.Reader, mimeType string) error {
	obj, err := s.objectName(key)
	if err != nil {
		return err
	}
	ct := mimeType
	if ct == "" {
		ct = "application/octet-stream"
	}
	// size -1 → minio streams via multipart, accepting any length from a plain
	// io.Reader without needing to know it up front.
	if _, err := s.client.PutObject(ctx, s.bucket, obj, r, -1, minio.PutObjectOptions{ContentType: ct}); err != nil {
		return fmt.Errorf("s3 put %q: %w", obj, err)
	}
	return nil
}

func (s *s3Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.objectName(key)
	if err != nil {
		return nil, err
	}
	o, err := s.client.GetObject(ctx, s.bucket, obj, minio.GetObjectOptions{})
	if err != nil {
		return nil, normS3Err(err)
	}
	// GetObject is lazy — Stat() forces the request so a missing key surfaces as
	// ErrObjectNotFound here instead of on the caller's first Read.
	if _, err := o.Stat(); err != nil {
		o.Close()
		return nil, normS3Err(err)
	}
	return o, nil
}

func (s *s3Storage) Delete(ctx context.Context, key string) error {
	obj, err := s.objectName(key)
	if err != nil {
		return err
	}
	if err := s.client.RemoveObject(ctx, s.bucket, obj, minio.RemoveObjectOptions{}); err != nil {
		// RemoveObject is normally idempotent; treat an explicit NoSuchKey as nil
		// to honor the Storage contract.
		if errors.Is(normS3Err(err), ErrObjectNotFound) {
			return nil
		}
		return fmt.Errorf("s3 delete %q: %w", obj, err)
	}
	return nil
}

// PublicURL returns a short-lived presigned GET URL on the bucket. The upstream
// vision provider fetches this directly from S3 — new-mcp is not in the data
// path for S3 uploads (unlike local, where the URL points back at new-mcp).
func (s *s3Storage) PublicURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	obj, err := s.objectName(key)
	if err != nil {
		return "", err
	}
	u, err := s.client.PresignedGetObject(ctx, s.bucket, obj, ttl, url.Values{})
	if err != nil {
		return "", fmt.Errorf("s3 presign %q: %w", obj, err)
	}
	return u.String(), nil
}

// PutURL returns a bucket presigned PUT — the agent's curl uploads straight to
// the object store and the bytes never touch new-mcp (unlike local). presigned
// PUT cannot bind size/content-type, so those are enforced at Stat/analyze time.
func (s *s3Storage) PutURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	obj, err := s.objectName(key)
	if err != nil {
		return "", err
	}
	u, err := s.client.PresignedPutObject(ctx, s.bucket, obj, ttl)
	if err != nil {
		return "", fmt.Errorf("s3 presign PUT %q: %w", obj, err)
	}
	return u.String(), nil
}

func (s *s3Storage) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	obj, err := s.objectName(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	oi, err := s.client.StatObject(ctx, s.bucket, obj, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, normS3Err(err)
	}
	return ObjectInfo{Size: oi.Size, MediaType: oi.ContentType}, nil
}

// OwnsURL reports whether rawurl points at this bucket's endpoint host. The
// presigned GET/PUT URLs this backend issues carry the configured endpoint host
// (path- or virtual-host-style), so a host match means "we issued it".
func (s *s3Storage) OwnsURL(rawurl string) bool {
	u, err := url.Parse(rawurl)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Host == s.client.EndpointURL().Host
}

// KeyFromURL recovers the storage key from a presigned GET/PUT URL this backend
// issued. The object name is "<pathPrefix>/<key>" and travels in the URL path
// (path- or virtual-host-style endpoints both put it there), so locating
// "/<pathPrefix>/" and taking the remainder yields the key. analyze_image uses
// it to read the object back and forward it as base64 to the upstream — same
// reason as local (ServerAddress reachability / Gemini file_uri bypass). ok=false
// if the marker is absent or the remainder is not a valid key.
func (s *s3Storage) KeyFromURL(rawurl string) (string, bool) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", false
	}
	// pathPrefix is non-empty (newS3 defaults it to "vision"), so the marker is
	// well-formed and appears exactly once between bucket and key.
	marker := "/" + s.pathPrefix + "/"
	idx := strings.Index(u.Path, marker)
	if idx < 0 {
		return "", false
	}
	key := u.Path[idx+len(marker):]
	clean := strings.Trim(key, "/")
	if clean == "" || strings.Contains(clean, "..") || strings.Contains(clean, "//") {
		return "", false
	}
	return clean, true
}

// normS3Err maps S3 "no such object" responses onto the storage sentinel so the
// file endpoint returns a clean 404 and the cleanup loop treats a reaped object
// as already-gone. Other errors pass through unchanged.
func normS3Err(err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		switch resp.Code {
		case "NoSuchKey", "NoSuchObject":
			return ErrObjectNotFound
		}
	}
	return err
}
