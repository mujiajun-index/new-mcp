// Package storage abstracts a binary blob backend for vision image uploads.
//
// The vision tool used to inline every image as base64, which forced the
// calling LLM to emit (and pay for) the entire base64 as output tokens. This
// package backs the replacement flow: an image is uploaded once (multipart,
// bytes never touch the LLM context), stored behind a content-addressed key,
// and handed to the vision tool as a short-lived URL that the upstream model
// fetches itself.
//
// Two backends ship behind one interface so the upload/serve/analyze code never
// branches on where bytes live: localStorage (disk, default, zero deps) and
// s3Storage (S3-compatible via minio-go). Bytes are always read back through Get
// (served at /u/<sid> or reverse-fetched for analyze_image); the local-vs-S3
// difference is fully hidden inside Put/Get/Delete.
package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/mujkjk/newmcp/model"
)

// Storage is the blob backend. Implementations must be safe for concurrent use.
type Storage interface {
	// Put stores bytes read from r under key. mimeType is set as Content-Type
	// on object-storage backends; local ignores it (MIME is sniffed at serve).
	Put(ctx context.Context, key string, r io.Reader, mimeType string) error

	// Get opens the object for reading. The caller must close the ReadCloser.
	// Used by the GET /u/:sid file endpoint and the own-URL reverse-fetch; both
	// local and S3 serve through Get.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes the object. It is idempotent: a missing key is not an error.
	Delete(ctx context.Context, key string) error

	// Stat returns object metadata without reading the body. Used by analyze_image
	// to size-check an own-storage object (against VisionUploadMaxBytes) before
	// reverse-fetching its bytes to forward to the upstream. Returns
	// ErrObjectNotFound if the object is missing.
	Stat(ctx context.Context, key string) (ObjectInfo, error)

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
