package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
	"gorm.io/gorm"
)

// 1x1 white PNG (same fixture as service.TestVision) — a real, sniffable image.
const testPngB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8/5+hHgAHggJ/PchI7wAAAABJRU5ErkJggg=="

func setupUploadTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.UploadedImage{}, &model.Option{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.InitOptionMap()
	// Close the sql pool so Windows can delete the temp DB file at test end.
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
}

func setupUploadTestStore(t *testing.T) {
	t.Helper()
	store, err := storage.New(context.Background(), storage.Config{Backend: "local", LocalRoot: t.TempDir(), PathPrefix: "vision"})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	service.UploadStore = store
}

type uploadRespData struct {
	URL     string `json:"url"`
	Key     string `json:"key"`
	Deduped bool   `json:"deduped"`
}

func uploadPNG(t *testing.T, engine *gin.Engine, png []byte) uploadRespData {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("image", "t.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	fw.Write(png)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/vision/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data uploadRespData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", w.Body.String(), err)
	}
	return resp.Data
}

// TestUploadServeSecureEndToEnd covers the local-backend pipeline end to end:
// multipart upload returns a signed URL, the URL serves the original bytes, a
// second upload of identical bytes dedups, a tampered token is rejected (403),
// and a valid signature for a past expiry is gone (410).
func TestUploadServeSecureEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupUploadTestDB(t)
	setupUploadTestStore(t)
	defer func() { service.UploadStore = nil }()

	engine := gin.New()
	stubAuth := func(c *gin.Context) { c.Set("user_id", int64(42)); c.Next() }
	engine.POST("/api/v1/vision/upload", stubAuth, UploadVisionImage)
	engine.GET("/api/v1/vision/files/*key", GetVisionFile)

	png, _ := base64.StdEncoding.DecodeString(testPngB64)
	first := uploadPNG(t, engine, png)
	if first.URL == "" || first.Key == "" {
		t.Fatalf("upload returned empty url/key: %+v", first)
	}
	if first.Deduped {
		t.Fatalf("first upload should not be deduped")
	}

	// Second identical upload → deduped, same key.
	second := uploadPNG(t, engine, png)
	if !second.Deduped || second.Key != first.Key {
		t.Fatalf("expected deduped re-upload with same key; got %+v", second)
	}

	// Fetch via the signed URL (route through the test engine by path+query).
	u, err := url.Parse(first.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	signedPath := u.Path
	if u.RawQuery != "" {
		signedPath += "?" + u.RawQuery
	}
	getReq := httptest.NewRequest(http.MethodGet, signedPath, nil)
	getW := httptest.NewRecorder()
	engine.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("fetch status=%d want 200", getW.Code)
	}
	if !bytes.Equal(getW.Body.Bytes(), png) {
		t.Fatalf("served bytes do not match uploaded image")
	}
	if ct := getW.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type=%q want image/png", ct)
	}

	// Tampered token → 403.
	tampered := u.Path + "?expires=" + u.Query().Get("expires") + "&token=deadbeef"
	tw := httptest.NewRecorder()
	engine.ServeHTTP(tw, httptest.NewRequest(http.MethodGet, tampered, nil))
	if tw.Code != http.StatusForbidden {
		t.Fatalf("tampered token status=%d want 403", tw.Code)
	}

	// Valid signature, but already-expired → 410.
	past := time.Now().Add(-time.Hour).Unix()
	pastTok := storage.SignURL(first.Key, past)
	expired := u.Path + "?expires=" + strconv.FormatInt(past, 10) + "&token=" + pastTok
	ew := httptest.NewRecorder()
	engine.ServeHTTP(ew, httptest.NewRequest(http.MethodGet, expired, nil))
	if ew.Code != http.StatusGone {
		t.Fatalf("expired status=%d want 410", ew.Code)
	}
}
