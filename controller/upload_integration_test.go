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
	"strings"
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
	engine.GET("/u/:sid", GetVisionFileByShortID)

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

	// Fetch via the short URL (route through the test engine by path+query).
	u, err := url.Parse(first.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if !strings.HasPrefix(u.Path, "/u/") {
		t.Fatalf("upload URL is not the short /u/ form: %s", first.URL)
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

	// Tampered signature → 403.
	tampered := u.Path + "?s=deadbeefdeadbeefdeadbeef"
	tw := httptest.NewRecorder()
	engine.ServeHTTP(tw, httptest.NewRequest(http.MethodGet, tampered, nil))
	if tw.Code != http.StatusForbidden {
		t.Fatalf("tampered signature status=%d want 403", tw.Code)
	}

	// Valid signature, but past retention → 410. Short URLs carry no embedded
	// expiry; the row's created_at + UploadRetentionHours is the authority, so age
	// the row past the 24h default and re-fetch.
	if err := model.DB.Model(&model.UploadedImage{}).Where("storage_key = ?", first.Key).
		Update("created_at", time.Now().Add(-25*time.Hour)).Error; err != nil {
		t.Fatalf("age row: %v", err)
	}
	ew := httptest.NewRecorder()
	engine.ServeHTTP(ew, httptest.NewRequest(http.MethodGet, signedPath, nil))
	if ew.Code != http.StatusGone {
		t.Fatalf("expired status=%d want 410", ew.Code)
	}
}

// TestPutShortSlot covers the upload_image curl path: a pending slot is filled by
// a PUT to /u/:sid (the upload_command target), flipping it to uploaded, after
// which the GET short URL serves the bytes. Also verifies method binding (a GET
// token cannot PUT) and one-shot semantics (re-PUT → 409).
func TestPutShortSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupUploadTestDB(t)
	setupUploadTestStore(t)
	defer func() { service.UploadStore = nil }()

	engine := gin.New()
	engine.PUT("/u/:sid", PutVisionFileByShortID)
	engine.GET("/u/:sid", GetVisionFileByShortID)

	// A pending slot, exactly as upload_image would create it (UUID storage key,
	// pending status, generated short_id).
	img := &model.UploadedImage{
		UserID:     42,
		StorageKey: "a1/a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Backend:    "local",
		Status:     model.UploadStatusPending,
	}
	if err := img.InsertWithGeneratedShortID(); err != nil {
		t.Fatalf("insert pending slot: %v", err)
	}

	png, _ := base64.StdEncoding.DecodeString(testPngB64)
	putU, _ := url.Parse(storage.ShortURL("PUT", img.ShortID))
	putPath := putU.Path + "?" + putU.RawQuery

	// PUT the bytes → 200, slot flips to uploaded.
	pw := httptest.NewRecorder()
	engine.ServeHTTP(pw, httptest.NewRequest(http.MethodPut, putPath, bytes.NewReader(png)))
	if pw.Code != http.StatusOK {
		t.Fatalf("PUT status=%d want 200 (body=%s)", pw.Code, pw.Body.String())
	}

	// GET token replayed as PUT → 403 (method binding).
	getU, _ := url.Parse(storage.ShortURL("GET", img.ShortID))
	gw := httptest.NewRecorder()
	engine.ServeHTTP(gw, httptest.NewRequest(http.MethodPut, getU.Path+"?"+getU.RawQuery, bytes.NewReader(png)))
	if gw.Code != http.StatusForbidden {
		t.Fatalf("GET-token-as-PUT status=%d want 403", gw.Code)
	}

	// Re-PUT to an already-used slot → 409.
	pw2 := httptest.NewRecorder()
	engine.ServeHTTP(pw2, httptest.NewRequest(http.MethodPut, putPath, bytes.NewReader(png)))
	if pw2.Code != http.StatusConflict {
		t.Fatalf("re-PUT status=%d want 409", pw2.Code)
	}

	// GET now serves the uploaded bytes.
	gw2 := httptest.NewRecorder()
	engine.ServeHTTP(gw2, httptest.NewRequest(http.MethodGet, getU.Path+"?"+getU.RawQuery, nil))
	if gw2.Code != http.StatusOK {
		t.Fatalf("GET status=%d want 200", gw2.Code)
	}
	if !bytes.Equal(gw2.Body.Bytes(), png) {
		t.Fatalf("served bytes do not match uploaded image")
	}
}
