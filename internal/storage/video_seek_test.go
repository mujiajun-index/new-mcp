package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestS3VideoSeekUsesRange(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	var ranged atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/" || r.URL.Path == "/videos" {
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("ETag", `"fixture-etag"`)
		if r.Header.Get("Range") != "" {
			ranged.Add(1)
		}
		http.ServeContent(w, r, "video.mp4", time.Unix(100, 0), bytes.NewReader(data))
	}))
	defer server.Close()
	store, err := New(context.Background(), Config{Backend: "s3", Endpoint: strings.TrimPrefix(server.URL, "http://"), Region: "us-east-1", Bucket: "videos", AccessKey: "fixture-access", SecretKey: "fixture-secret", PathPrefix: "media"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := store.(SeekableStorage).OpenSeek(context.Background(), "video/1/test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.Seek(5, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "fghijklm" || ranged.Load() == 0 {
		t.Fatalf("bytes=%q range requests=%d", buf, ranged.Load())
	}
}
