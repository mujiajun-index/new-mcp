package imageasset

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSaveWithLocalAndS3Backend(t *testing.T) {
	for _, backend := range []string{"local", "s3"} {
		t.Run(backend, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "store.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			if err != nil {
				t.Fatal(err)
			}
			oldDB := model.DB
			model.DB = db
			sqlDB, _ := db.DB()
			t.Cleanup(func() { sqlDB.Close(); model.DB = oldDB })
			if err := db.AutoMigrate(&model.Option{}, &model.UploadedImage{}); err != nil {
				t.Fatal(err)
			}
			model.InitOptionMap()
			model.UpdateOption("ServerAddress", "https://storage.example")
			var pngBytes bytes.Buffer
			png.Encode(&pngBytes, image.NewNRGBA(image.Rect(0, 0, 2, 3)))
			data := pngBytes.Bytes()
			cfg := storage.Config{Backend: backend, LocalRoot: t.TempDir(), PathPrefix: "images"}
			if backend == "s3" {
				// Exercise minio-go itself against a small S3 HTTP fixture, including its
				// multipart PUT, lazy GET/HEAD and DELETE paths, without cloud credentials.
				var mu sync.Mutex
				objects := map[string][]byte{}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					mu.Lock()
					defer mu.Unlock()
					if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
						t.Error("S3 authentication missing")
					}
					w.Header().Set("Content-Type", "application/xml")
					key := strings.TrimPrefix(r.URL.Path, "/bucket/")
					_, init := r.URL.Query()["uploads"]
					switch {
					case r.Method == "HEAD" && r.URL.Path == "/bucket/":
						w.WriteHeader(200)
					case r.Method == "POST" && init:
						fmt.Fprintf(w, `<InitiateMultipartUploadResult><Bucket>bucket</Bucket><Key>%s</Key><UploadId>upload-id</UploadId></InitiateMultipartUploadResult>`, xmlEscape(key))
					case r.Method == "PUT":
						body, _ := io.ReadAll(r.Body)
						if r.Header.Get("X-Amz-Decoded-Content-Length") != "" {
							var err error
							body, err = io.ReadAll(httputil.NewChunkedReader(bytes.NewReader(body)))
							if err != nil {
								t.Error(err)
								w.WriteHeader(400)
								return
							}
						}
						objects[key] = body
						w.Header().Set("ETag", `"d41d8cd98f00b204e9800998ecf8427e"`)
					case r.Method == "POST":
						io.Copy(io.Discard, r.Body)
						fmt.Fprintf(w, `<CompleteMultipartUploadResult><Bucket>bucket</Bucket><Key>%s</Key><ETag>"d41d8cd98f00b204e9800998ecf8427e"</ETag></CompleteMultipartUploadResult>`, xmlEscape(key))
					case r.Method == "HEAD" || r.Method == "GET":
						body, ok := objects[key]
						if !ok {
							w.WriteHeader(404)
							return
						}
						w.Header().Set("Content-Type", "image/png")
						w.Header().Set("Content-Length", fmt.Sprint(len(body)))
						w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
						w.Header().Set("ETag", `"d41d8cd98f00b204e9800998ecf8427e"`)
						if r.Method == "GET" {
							w.Write(body)
						}
					case r.Method == "DELETE":
						delete(objects, key)
						w.WriteHeader(204)
					default:
						t.Errorf("unexpected S3 request %s %s", r.Method, r.URL)
						w.WriteHeader(400)
					}
				}))
				defer server.Close()
				cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")
				cfg.Bucket = "bucket"
				cfg.AccessKey = "mock-access"
				cfg.SecretKey = "mock-secret"
				cfg.Region = "us-east-1"
			}
			ctx := context.Background()
			store, err := storage.New(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Save(ctx, store, 1, bytes.NewReader(data), 1024)
			if err != nil {
				t.Fatal(err)
			}
			if result.Backend != backend {
				t.Fatal(result)
			}
			rc, err := store.Get(ctx, result.Key)
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(rc)
			rc.Close()
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("round trip %d bytes: %v", len(got), err)
			}
			second, err := Save(ctx, store, 1, bytes.NewReader(data), 1024)
			if err != nil || !second.Deduped || result.URL != second.URL {
				t.Fatalf("dedup %v %v", second, err)
			}
			generated, err := SaveGenerated(ctx, store, 2, bytes.NewReader(data), 1024)
			if err != nil || generated.UploadID == result.UploadID || generated.Key != result.Key {
				t.Fatalf("generated image shared blob: %+v %v", generated, err)
			}
			if _, total, err := model.ListAllUploads(0, 0, 20); err != nil || total != 1 {
				t.Fatalf("generated images leaked into upload management: %d %v", total, err)
			}
			model.UpdateOption("UploadRetentionHours", "0")
			permanent, err := Save(ctx, store, 1, bytes.NewReader(data), 1024)
			if err != nil || permanent.ExpiresAt != nil || !permanent.Deduped {
				t.Fatalf("permanent image retention: result=%+v err=%v", permanent, err)
			}
			if err := store.Delete(ctx, result.Key); err != nil {
				t.Fatal(err)
			}
			var referenceBytes bytes.Buffer
			png.Encode(&referenceBytes, image.NewNRGBA(image.Rect(0, 0, 4, 5)))
			reference, err := SaveReference(ctx, store, 3, bytes.NewReader(referenceBytes.Bytes()), 1024)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := store.Get(ctx, reference.Key)
			if err != nil {
				t.Fatal(err)
			}
			gotReference, err := io.ReadAll(reader)
			reader.Close()
			if err != nil || !bytes.Equal(gotReference, referenceBytes.Bytes()) {
				t.Fatal("reference round trip failed", err)
			}
			if _, total, err := model.ListUploadsByUser(3, 0, 20); err != nil || total != 0 {
				t.Fatal("cached reference appeared in uploads", err)
			}
			if _, total, err := model.ListAllUploads(3, 0, 20); err != nil || total != 0 {
				t.Fatal("cached reference appeared in admin uploads", err)
			}
		})
	}
}
func xmlEscape(value string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
