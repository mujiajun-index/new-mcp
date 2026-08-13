// Package storage abstracts a binary blob backend for vision image uploads.
//
// The vision tool used to inline every image as base64, which forced the
// calling LLM to emit (and pay for) the entire base64 as output tokens. This
// package backs the replacement flow: an image is uploaded once (multipart,
// bytes never touch the LLM context), stored behind a content-addressed key,
// and handed to the vision tool as a short-lived URL that the upstream model
// fetches itself.
//
// Two backends ship behind one interface so the upload/analyze code never
// branches on where bytes live: localStorage (disk, default, zero deps) and
// s3Storage (S3-compatible via minio-go). The local-vs-S3 difference is fully
// hidden inside PublicURL — local returns an HMAC-signed URL pointing back at
// new-mcp's own file endpoint; S3 returns a bucket presigned GET URL.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/mujkjk/newmcp/model"
)

// Storage is the blob backend. Implementations must be safe for concurrent use.
type Storage interface {
	// Put stores bytes read from r under key. mimeType is set as Content-Type
	// on object-storage backends; local ignores it (MIME is sniffed at serve).
	Put(ctx context.Context, key string, r io.Reader, mimeType string) error

	// Get opens the object for reading. The caller must close the ReadCloser.
	// Used by the local self-serve file endpoint; S3 serves via presign and
	// never goes through Get.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object. It is idempotent: a missing key is not an error.
	Delete(ctx context.Context, key string) error

	// PublicURL returns a URL valid for ttl that lets a third party (the
	// upstream vision provider) fetch the object without authenticating to
	// new-mcp. Local builds an HMAC-signed URL; S3 returns a presigned GET.
	PublicURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// PutURL returns a short-lived URL the caller PUTs raw bytes to. The URL is
	// the only credential — no API key travels in the upload command. Local
	// returns a purpose-bound HMAC URL to the PUT file endpoint; S3 returns a
	// bucket presigned PUT (bytes bypass new-mcp entirely). Size/content-type
	// are NOT enforceable on a presigned PUT, so callers enforce them server-side.
	PutURL(ctx context.Context, key string, ttl time.Duration) (string, error)

	// Stat returns object metadata without reading the body. Used by analyze_image
	// to size-check an own-storage object (against VisionUploadMaxBytes) before
	// reverse-fetching its bytes to forward to the upstream. Returns
	// ErrObjectNotFound if the object is missing.
	Stat(ctx context.Context, key string) (ObjectInfo, error)

	// OwnsURL reports whether rawurl points into this backend (a URL this server
	// issued), so analyze_image reverse-fetches own-storage bytes (and forwards
	// them as base64) while leaving arbitrary http(s) URLs as pure passthrough
	// (no SSRF: own bytes are read locally; external URLs are never fetched).
	OwnsURL(rawurl string) bool

	// KeyFromURL recovers the storage key from a URL this backend issued (one for
	// which OwnsURL is true). analyze_image uses it to read own-storage bytes back
	// out and forward them as base64 to the upstream provider — so the provider
	// never has to reach ServerAddress (works behind localhost) and Gemini's flaky
	// native file_uri fetch is bypassed. ok=false means the key is not recoverable
	// from rawurl; the caller then falls back to passing the URL through.
	KeyFromURL(rawurl string) (key string, ok bool)

	// Backend returns a short identifier ("local" / "s3") for metadata/logging.
	Backend() string
}

// ObjectInfo is the lightweight metadata returned by Stat. MediaType is
// best-effort — the local backend leaves it empty because it doesn't store
// per-file metadata (the uploaded_images row is authoritative there).
type ObjectInfo struct {
	Size      int64
	MediaType string
}

// Config is assembled once at startup from model options (see LoadConfig).
type Config struct {
	Backend    string // "local" | "s3"
	LocalRoot  string // e.g. "./data/uploads"
	PathPrefix string // e.g. "vision" — physical isolation on disk/bucket
	// S3-compatible (minio-go covers AWS S3, MinIO, R2, B2, OSS, COS)
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// New builds the configured backend. "s3" requires a complete credential set
// (see newS3) and fails loudly at startup if any is missing; any other value
// selects the dependency-free local backend.
func New(ctx context.Context, cfg Config) (Storage, error) {
	switch cfg.Backend {
	case "s3":
		return newS3(ctx, cfg)
	default:
		return newLocal(cfg)
	}
}

// ContentKey turns a hex sha256 of the image bytes into the storage key used
// across Put/Get/PublicURL: a two-level shard "<sha[:2]>/<sha>". The shard keeps
// any single directory/bucket prefix from growing unbounded, and because the
// key is derived from content, identical bytes dedupe for free.
func ContentKey(sha256hex string) string {
	if len(sha256hex) < 2 {
		return sha256hex
	}
	return sha256hex[:2] + "/" + sha256hex
}

// LoadConfig reads backend settings from the options table. Centralizing it here
// keeps router/main.go's InitGateway free of storage-specific key names.
func LoadConfig() Config {
	return Config{
		Backend:    model.GetOptionString("StorageBackend"),
		LocalRoot:  model.GetOptionString("StorageLocalPath"),
		PathPrefix: model.GetOptionString("StoragePathPrefix"),
		Endpoint:   model.GetOptionString("StorageEndpoint"),
		Region:     model.GetOptionString("StorageRegion"),
		Bucket:     model.GetOptionString("StorageBucket"),
		AccessKey:  model.GetOptionString("StorageAccessKey"),
		SecretKey:  model.GetOptionString("StorageSecretKey"),
		UseSSL:     model.GetOptionBool("StorageUseSSL"),
	}
}

// Sentinel returned by backends when an object is missing; lets the cleanup
// loop and file endpoint distinguish "gone" from a real I/O error.
var ErrObjectNotFound = fmt.Errorf("storage: object not found")
