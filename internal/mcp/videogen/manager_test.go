package videogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func managerFixture(t *testing.T) *Manager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "video.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.VideoGenTask{}, &model.VideoGenConfig{}, &model.User{}, &model.Option{}, &model.UploadedImage{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	oldDB := model.DB
	model.DB = db
	model.InitOptionMap()
	store, err := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(db, store)
	m.PollInterval = 10 * time.Millisecond
	t.Cleanup(func() { m.Close(); sqlDB.Close(); model.DB = oldDB })
	return m
}

func configFor(server string) *model.VideoGenConfig {
	key, _ := common.Encrypt("fixture-secret")
	return &model.VideoGenConfig{ID: 1, UserID: 1, Name: "fixture", EndpointURL: server, APIKey: key, ModelName: "any-video", Operation: "both", ImageRequestFormat: "multipart", DefaultParams: "{}", RequestTimeoutSeconds: 2, TaskTimeoutSeconds: 3600}
}

func mp4Bytes() []byte {
	return append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm', 'm', 'p', '4', '2'}, make([]byte, 64)...)
}

func awaitTask(t *testing.T, m *Manager, id string, predicate func(*model.VideoGenTask) bool) *model.VideoGenTask {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		m.ProcessDue()
		row, err := m.Get(1, nil, id, false)
		if err == nil && predicate(row) {
			return row
		}
		time.Sleep(10 * time.Millisecond)
	}
	row, _ := m.Get(1, nil, id, false)
	t.Fatalf("task did not reach expected state: %+v", row)
	return nil
}

func TestTaskContinuesAfterDisconnectAndDeduplicates(t *testing.T) {
	m := managerFixture(t)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("missing gateway key")
		}
		switch {
		case r.Method == "POST":
			posts.Add(1)
			io.WriteString(w, `{"id":"public-video","status":"queued"}`)
		case strings.HasSuffix(r.URL.Path, "/content"):
			w.Header().Set("Content-Type", "video/mp4")
			w.Write(mp4Bytes())
		default:
			io.WriteString(w, `{"id":"public-video","status":"completed","progress":100}`)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := configFor(server.URL)
	args := map[string]any{"prompt": "motion", "request_id": "same-request"}
	first, err := m.Submit(ctx, 1, cfg, "generate", "test", args)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	duplicate, err := m.Submit(context.Background(), 1, cfg, "generate", "test", args)
	if err != nil || duplicate.ID != first.ID {
		t.Fatalf("duplicate: %v", err)
	}
	changed := map[string]any{"prompt": "different", "request_id": "same-request"}
	if _, err := m.Submit(context.Background(), 1, cfg, "generate", "test", changed); err == nil {
		t.Fatal("accepted conflicting request_id")
	}
	cfg.EndpointURL = "https://invalid.example" // The persisted connection must still use the original gateway.
	row := awaitTask(t, m, first.ID, func(t *model.VideoGenTask) bool { return t.ArtifactStatus == "ready" })
	if posts.Load() != 1 || row.Connection != "" || row.Progress != 100 {
		t.Fatalf("posts=%d snapshot retained=%v", posts.Load(), row.Connection != "")
	}
	raw, _ := MCPResult(row)
	if strings.Contains(string(raw), "fixture-secret") || !strings.Contains(string(raw), "resource_link") {
		t.Fatal("missing resource or leaked key")
	}
	if _, err := m.Get(2, nil, row.ID, false); err == nil {
		t.Fatal("cross-user access")
	}
	otherConfig := int64(99)
	if _, err := m.Get(1, &otherConfig, row.ID, false); err == nil {
		t.Fatal("cross-config access")
	}
}

func TestSavingRetryNeverResubmitsGeneration(t *testing.T) {
	m := managerFixture(t)
	var posts, downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts.Add(1)
			io.WriteString(w, `{"id":"video","status":"completed"}`)
			return
		}
		if downloads.Add(1) == 1 {
			io.WriteString(w, "<html>upstream error</html>")
			return
		}
		w.Write(mp4Bytes())
	}))
	defer server.Close()
	job, err := m.Submit(context.Background(), 1, configFor(server.URL), "generate", "test", map[string]any{"prompt": "motion"})
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, m, job.ID, func(t *model.VideoGenTask) bool { return t.ArtifactStatus == "failed" })
	if failed.Status != "completed" || failed.Connection == "" {
		t.Fatal("save failure lost generation or retry credentials")
	}
	if _, err := m.RetryDownload(1, nil, job.ID); err != nil {
		t.Fatal(err)
	}
	awaitTask(t, m, job.ID, func(t *model.VideoGenTask) bool { return t.ArtifactStatus == "ready" })
	if posts.Load() != 1 || downloads.Load() != 2 {
		t.Fatalf("posts=%d downloads=%d", posts.Load(), downloads.Load())
	}
}

func TestRetryGenerationUsesCurrentConfigAndDeduplicates(t *testing.T) {
	m := managerFixture(t)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			io.WriteString(w, `{"status":"queued"}`)
			return
		}
		posts.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["mode"] != "text" {
			w.WriteHeader(400)
			io.WriteString(w, `{"message":"mode is required"}`)
			return
		}
		if body["size"] != "720P" || body["metadata"].(map[string]any)["seed"] != float64(42) {
			t.Error("retry lost saved arguments or current defaults", body)
		}
		io.WriteString(w, `{"id":"retry-upstream","status":"queued"}`)
	}))
	defer server.Close()
	cfg := configFor(server.URL)
	originalArgs := map[string]any{"prompt": "scene", "metadata": map[string]any{"seed": float64(42)}, "request_id": "original-submit"}
	parent, err := m.Submit(context.Background(), 1, cfg, "generate", "test", originalArgs)
	if err != nil {
		t.Fatal(err)
	}
	failed := awaitTask(t, m, parent.ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" })
	if failed.Connection != "" || failed.RequestParams == "" {
		t.Fatal("failed task lost retry input or retained credentials")
	}
	view := TaskView(failed)
	if _, ok := view["RequestParams"]; ok {
		t.Fatal("private input exposed")
	}
	if _, ok := view["request_params"]; ok {
		t.Fatal("private input exposed")
	}
	cfg.DefaultParams = `{"size":"720P","extra_body":{"mode":"text"}}`
	child, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-action")
	if err != nil {
		t.Fatal(err)
	}
	child = awaitTask(t, m, child.ID, func(row *model.VideoGenTask) bool { return row.UpstreamID == "retry-upstream" })
	again, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-action")
	if err != nil || again.ID != child.ID || posts.Load() != 2 {
		t.Fatalf("duplicate retry: %v posts=%d", err, posts.Load())
	}
	if child.ID != parent.ID || child.RetryOf != "" || child.Source != parent.Source || child.RetryCount != 1 || !child.CreatedAt.Equal(parent.CreatedAt) {
		t.Fatal("retry did not preserve task identity")
	}
	if _, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, map[string]any{"prompt": "changed"}, "retry-action"); err == nil {
		t.Fatal("changed retry reused request ID")
	}
	if _, err := m.RetryGeneration(context.Background(), 2, parent.ID, cfg, nil, "cross-user"); err == nil {
		t.Fatal("cross-user retry")
	}
	if _, err := m.RetryGeneration(context.Background(), 1, parent.ID, &model.VideoGenConfig{UserID: 2}, nil, "cross-config"); err == nil {
		t.Fatal("foreign configuration retry")
	}
	updated, _ := m.Get(1, nil, parent.ID, false)
	if updated.Status != "queued" || updated.Error != "" || updated.LastError != "" || updated.StoppedAt != nil || updated.Progress != 0 || updated.IdempotencyKey != failed.IdempotencyKey || updated.RequestHash != failed.RequestHash {
		t.Fatal("retry did not reset the failed attempt or preserve original idempotency")
	}
	var count int64
	m.DB.Model(&model.VideoGenTask{}).Count(&count)
	if count != 1 {
		t.Fatal("retry created another history row")
	}
	for _, key := range []string{"RetryRequests", "retry_requests", "request_params"} {
		if _, ok := TaskView(updated)[key]; ok {
			t.Fatal("private retry state exposed")
		}
	}
	retransmitted, err := m.Submit(context.Background(), 1, cfg, "generate", "test", originalArgs)
	if err != nil || retransmitted.ID != parent.ID || posts.Load() != 2 {
		t.Fatal("original submission lost its idempotency mapping")
	}
	restarted := NewManager(m.DB, m.Store)
	defer restarted.Close()
	if repeated, err := restarted.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-action"); err != nil || repeated.ID != parent.ID || posts.Load() != 2 {
		t.Fatal("retry deduplication did not survive restart")
	}
}

func TestRetryGenerationGuardsAndLegacyImage(t *testing.T) {
	m := managerFixture(t)
	for _, status := range []string{"queued", "in_progress", "completed", "submission_unknown"} {
		row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, Status: status, Operation: "generate", Prompt: "scene", IdempotencyKey: uuid.NewString()}
		if err := m.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if _, _, err := RetryInput(&row); err == nil {
			t.Fatal("retry input accepted", status)
		}
		if _, err := m.RetryGeneration(context.Background(), 1, row.ID, configFor("https://invalid.example"), nil, "attempt"); err == nil {
			t.Fatal("non-failed retry accepted", status)
		}
	}
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			io.WriteString(w, `{"status":"queued"}`)
			return
		}
		posts.Add(1)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("input_reference")
		if err != nil {
			t.Error(err)
			return
		}
		file.Close()
		io.WriteString(w, `{"id":"legacy-image-retry","status":"queued"}`)
	}))
	defer server.Close()
	row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, Status: "failed", Operation: "image", Prompt: "original scene", IdempotencyKey: uuid.NewString()}
	if err := m.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	args, complete, err := RetryInput(&row)
	if err != nil || complete || args["prompt"] != row.Prompt {
		t.Fatal("legacy fallback", args, complete, err)
	}
	if _, err := m.RetryGeneration(context.Background(), 1, row.ID, configFor(server.URL), nil, "missing-image"); err == nil || posts.Load() != 0 {
		t.Fatal("retried image without reference")
	}
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	ref := base64.StdEncoding.EncodeToString(pngData.Bytes())
	child, err := m.RetryGeneration(context.Background(), 1, row.ID, configFor(server.URL), map[string]any{"image": ref}, "with-image")
	if err != nil {
		t.Fatal(err)
	}
	child = awaitTask(t, m, child.ID, func(row *model.VideoGenTask) bool { return row.UpstreamID != "" })
	if child.ID != row.ID || child.RetryCount != 1 {
		t.Fatal("image retry created a new task")
	}
	stored, complete, err := RetryInput(&model.VideoGenTask{Status: "failed", RequestParams: child.RequestParams})
	if err != nil || !complete || stored["image"] != ref || stored["prompt"] != row.Prompt || posts.Load() != 1 {
		t.Fatal("image retry input not preserved", complete, err)
	}
}

func TestCompletedUpstreamNoticeIsPreserved(t *testing.T) {
	m := &Manager{PollInterval: time.Second}
	values := m.videoValues(&model.VideoGenTask{}, &Client{}, &Video{Status: "completed", Error: json.RawMessage(`{"message":"Output blocked by moderation"}`)})
	if values["status"] != "completed" || values["last_error"] != "Output blocked by moderation" {
		t.Fatal("completed upstream notice was lost")
	}
}

func TestRecoveryTimeoutPauseAndTransientQuery(t *testing.T) {
	m := managerFixture(t)
	var queries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.Write(mp4Bytes())
			return
		}
		if queries.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"try again fixture-secret"}}`)
			return
		}
		io.WriteString(w, `{"id":"existing","status":"completed"}`)
	}))
	defer server.Close()
	connJSON, _ := json.Marshal(Connection{EndpointURL: server.URL, APIKey: "fixture-secret", Model: "model"})
	snapshot, _ := common.Encrypt(string(connJSON))
	row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, ConfigID: 1, IdempotencyKey: uuid.NewString(), Connection: snapshot, UpstreamID: "existing", Status: "queued", ArtifactStatus: "pending", RequestTimeoutSeconds: 2, TaskTimeoutSeconds: 3600, TrackingDeadline: time.Now().Add(-time.Second), NextPollAt: unleasedAt, LeaseUntil: unleasedAt}
	if err := m.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	awaitTask(t, m, row.ID, func(t *model.VideoGenTask) bool { return t.TrackingPaused })
	if queries.Load() != 0 {
		t.Fatal("queried beyond tracking deadline")
	}
	if _, err := m.Get(1, nil, row.ID, true); err != nil {
		t.Fatal(err)
	}
	transient := awaitTask(t, m, row.ID, func(t *model.VideoGenTask) bool { return t.Failures == 1 })
	if transient.Status != "queued" || transient.NextPollAt.Before(time.Now().Add(time.Second)) || strings.Contains(transient.LastError, "fixture-secret") {
		t.Fatal("transient failure was not safely deferred")
	}
	if err := m.DB.Model(&row).Update("next_poll_at", unleasedAt).Error; err != nil {
		t.Fatal(err)
	}
	// A new manager over the same database resumes from the persisted upstream ID.
	restarted := NewManager(m.DB, m.Store)
	restarted.PollInterval = m.PollInterval
	defer restarted.Close()
	awaitTask(t, restarted, row.ID, func(t *model.VideoGenTask) bool { return t.ArtifactStatus == "ready" })
}

func TestUnknownSubmissionAndStaleLease(t *testing.T) {
	m := managerFixture(t)
	row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, IdempotencyKey: uuid.NewString(), Connection: "private", Status: "submitting", ArtifactStatus: "pending", NextPollAt: unleasedAt, LeaseUntil: unleasedAt, TrackingDeadline: time.Now().Add(time.Hour)}
	if err := m.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	m.ProcessDue()
	recovered, err := m.Get(1, nil, row.ID, false)
	if err != nil || recovered.Status != "submission_unknown" || recovered.Connection != "" {
		t.Fatalf("%+v %v", recovered, err)
	}
	stale := *recovered
	if err := m.DB.Model(recovered).Update("error", "new state").Error; err != nil {
		t.Fatal(err)
	}
	if m.claim(&stale, time.Minute) {
		t.Fatal("stale worker acquired a changed task")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"status":"queued"}`) }))
	defer server.Close()
	job, err := m.Submit(context.Background(), 1, configFor(server.URL), "generate", "test", map[string]any{"prompt": "motion"})
	if err != nil {
		t.Fatal(err)
	}
	awaitTask(t, m, job.ID, func(t *model.VideoGenTask) bool { return t.Status == "submission_unknown" })
}

func TestLeasePreventsConcurrentPollingAndActiveDelete(t *testing.T) {
	m := managerFixture(t)
	var queries atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		entered <- struct{}{}
		<-release
		io.WriteString(w, `{"id":"video","status":"queued"}`)
	}))
	defer server.Close()
	data, _ := json.Marshal(Connection{EndpointURL: server.URL, APIKey: "fixture-secret"})
	snapshot, _ := common.Encrypt(string(data))
	row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, IdempotencyKey: uuid.NewString(), Connection: snapshot, UpstreamID: "video", Status: "queued", ArtifactStatus: "pending", NextPollAt: unleasedAt, LeaseUntil: unleasedAt, TrackingDeadline: time.Now().Add(time.Hour), RequestTimeoutSeconds: 2, TaskTimeoutSeconds: 3600}
	if err := m.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	other := NewManager(m.DB, m.Store)
	defer other.Close()
	m.ProcessDue()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not query")
	}
	other.ProcessDue()
	if err := m.Delete(1, row.ID); !errors.Is(err, ErrActiveTask) {
		t.Fatalf("active delete: %v", err)
	}
	if queries.Load() != 1 {
		t.Fatal("duplicate query")
	}
	close(release)
}

func TestExpiredAssetsAndOldPartialFilesAreCleaned(t *testing.T) {
	m := managerFixture(t)
	sid := "fixture"
	expired := time.Now().Add(-time.Hour)
	stopped := time.Now().Add(-31 * 24 * time.Hour)
	for i, status := range []string{"ready", "failed"} {
		row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, IdempotencyKey: uuid.NewString(), Status: "completed", ArtifactStatus: status, StorageKey: fmt.Sprintf("video/1/fixture-%d", i), NextPollAt: unleasedAt, LeaseUntil: unleasedAt, TrackingDeadline: time.Now(), StoppedAt: &stopped}
		if status == "ready" {
			row.ShortID = &sid
			row.ExpiresAt = &expired
		}
		if err := m.Store.Put(context.Background(), row.StorageKey, strings.NewReader("partial"), "video/mp4"); err != nil {
			t.Fatal(err)
		}
		if err := m.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	m.Cleanup()
	var n int64
	m.DB.Model(&model.VideoGenTask{}).Count(&n)
	if n != 0 {
		t.Fatalf("retained %d old tasks", n)
	}
	for i := range 2 {
		if _, err := m.Store.Stat(context.Background(), fmt.Sprintf("video/1/fixture-%d", i)); !errors.Is(err, storage.ErrObjectNotFound) {
			t.Fatalf("partial file retained: %v", err)
		}
	}
}
