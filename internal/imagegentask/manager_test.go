package imagegentask

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func fixture(t *testing.T) (*Manager, *model.ImageGenConfig, []byte) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "async.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.Option{}, &model.ImageGenConfig{}, &model.ImageGenTask{}, &model.ImageGenTaskImage{}, &model.UploadedImage{}); err != nil {
		t.Fatal(err)
	}
	oldDB, oldSecret := model.DB, common.CryptoSecret
	model.DB, common.CryptoSecret = db, "async-test-encryption-key"
	model.InitOptionMap()
	model.UpdateOption("ServerAddress", "https://storage.example")
	store, err := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	m := NewManager(db, store)
	t.Cleanup(func() { m.Close(); sqlDB.Close(); model.DB, common.CryptoSecret = oldDB, oldSecret })
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	cfg := &model.ImageGenConfig{ID: 1, UserID: 1, Name: "images", Provider: "openai", ModelName: "test-image", ApiKey: "private-upstream-key", Operation: "both", DefaultParams: "{}", TimeoutSeconds: 180}
	return m, cfg, data.Bytes()
}

func done(t *testing.T, m *Manager, id string) *model.ImageGenTask {
	t.Helper()
	row, err := m.Wait(context.Background(), 1, id, 4)
	if err != nil || Active(row) {
		t.Fatalf("task did not finish: %+v %v", row, err)
	}
	return row
}

func respondImage(w http.ResponseWriter, data []byte) {
	fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString(data))
}

func TestAcceptedTaskSurvivesCallerCancellationAndIsIdempotent(t *testing.T) {
	m, cfg, data := fixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(started)
		if r.Header.Get("Authorization") != "Bearer private-upstream-key" {
			t.Error("missing snapshot credential")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		for _, key := range []string{"request_id", "wait_seconds", "return_mode"} {
			if _, ok := body[key]; ok {
				t.Error("local control sent upstream", key)
			}
		}
		select {
		case <-release:
			respondImage(w, data)
		case <-r.Context().Done():
			t.Error("caller canceled accepted upstream request")
		}
	}))
	defer server.Close()
	defer close(release)
	cfg.EndpointURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	args := map[string]any{"prompt": "draw", "request_id": "stable", "wait_seconds": float64(0)}
	task, err := m.Submit(ctx, 1, cfg, "generate_image", "mcp", args)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	waited, err := m.Wait(ctx, 1, task.ID, 30)
	if err != nil || !Active(waited) {
		t.Fatal("canceled short wait must return task", err)
	}
	if err = m.Delete(1, task.ID); !errors.Is(err, ErrActiveTask) {
		t.Fatal("active task deletable", err)
	}
	for i := 0; i < 3; i++ {
		again, e := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", args)
		if e != nil || again.ID != task.ID {
			t.Fatal("retry created a new task", e)
		}
	}
	changed := map[string]any{"prompt": "different", "request_id": "stable"}
	if _, err = m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", changed); err == nil {
		t.Fatal("different input reused request ID")
	}
	if _, err = m.Get(2, nil, task.ID); err == nil {
		t.Fatal("cross-user access")
	}
	otherConfig := int64(2)
	if _, err = m.Get(1, &otherConfig, task.ID); err == nil {
		t.Fatal("cross-config access")
	}
	// Release without double-closing the channel at function exit.
	release <- struct{}{}
	final := done(t, m, task.ID)
	if final.Status != "completed" || final.ArtifactStatus != "ready" || final.Snapshot != "" || calls.Load() != 1 {
		t.Fatalf("wrong final task: %+v calls=%d", final, calls.Load())
	}
	raw, err := m.MCPResult(context.Background(), final, "inline")
	if err != nil || !bytes.Contains(raw, []byte(`"type":"resource_link"`)) || !bytes.Contains(raw, []byte(`"type":"image"`)) {
		t.Fatalf("missing image results %s %v", raw, err)
	}
	for _, secret := range []string{"private-upstream-key", "snapshot", "result_key", "upload_id", "lease_owner"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("private field leaked", secret)
		}
	}
}

// Hold all worker slots so a queued task can be recovered by another manager.
func hold(m *Manager) {
	for i := 0; i < cap(m.sem); i++ {
		m.sem <- struct{}{}
	}
}
func unhold(m *Manager) {
	for i := 0; i < cap(m.sem); i++ {
		<-m.sem
	}
}

func TestRestartUsesDraftSnapshotAndOnlyOneWorkerSubmits(t *testing.T) {
	m, cfg, data := fixture(t)
	hold(m)
	defer unhold(m)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "test-image" || r.Header.Get("Authorization") != "Bearer private-upstream-key" {
			t.Error("snapshot changed with config")
		}
		time.Sleep(80 * time.Millisecond)
		respondImage(w, data)
	}))
	defer server.Close()
	cfg.ID = 0
	cfg.EndpointURL = server.URL
	task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "test", map[string]any{"prompt": "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(task.Snapshot, "private-upstream-key") {
		t.Fatal("connection snapshot is plaintext")
	}
	cfg.EndpointURL = "http://invalid.invalid"
	cfg.ApiKey = "changed"
	cfg.ModelName = "changed"
	m.Close()
	a, b := NewManager(m.DB, m.Store), NewManager(m.DB, m.Store)
	defer a.Close()
	defer b.Close()
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				a.ProcessDue()
			} else {
				b.ProcessDue()
			}
		}(i)
	}
	wg.Wait()
	final := done(t, a, task.ID)
	if calls.Load() != 1 || final.Status != "completed" || final.ConfigID != 0 {
		t.Fatalf("restart/lease failed %+v calls=%d", final, calls.Load())
	}
}

func TestExpiredRunningLeaseBecomesUnknownWithoutResubmission(t *testing.T) {
	m, cfg, _ := fixture(t)
	hold(m)
	defer unhold(m)
	cfg.EndpointURL = "http://127.0.0.1:1"
	task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "lost"})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Hour)
	m.DB.Model(task).Updates(map[string]any{"status": "running", "lease_owner": "other", "lease_until": future})
	m.ProcessDue()
	live, _ := m.Get(1, nil, task.ID)
	if live.Status != "running" {
		t.Fatal("live lease was stolen")
	}
	past := time.Now().Add(-time.Second)
	m.DB.Model(task).Update("lease_until", past)
	m.ProcessDue()
	lost, _ := m.Get(1, nil, task.ID)
	if lost.Status != "submission_unknown" || lost.Snapshot != "" || lost.FinishedAt == nil {
		t.Fatalf("interrupted submission incorrectly recovered %+v", lost)
	}
	m.ProcessDue()
	again, _ := m.Get(1, nil, task.ID)
	if again.Status != "submission_unknown" {
		t.Fatal("unknown task resubmitted")
	}
}

type failingAssets struct {
	storage.Storage
	fail    atomic.Bool
	failKey string
}

func (s *failingAssets) Put(ctx context.Context, key string, r io.Reader, mime string) error {
	if s.fail.Load() && !strings.HasPrefix(key, "imagegen/tasks/") && (s.failKey == "" || s.failKey == key) {
		return errors.New("disk temporarily unavailable")
	}
	return s.Storage.Put(ctx, key, r, mime)
}

func TestSavingRetryAndRestartNeverRegenerates(t *testing.T) {
	m, cfg, data := fixture(t)
	failing := &failingAssets{Storage: m.Store}
	failing.fail.Store(true)
	m.Store = failing
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); respondImage(w, data) }))
	defer server.Close()
	cfg.EndpointURL = server.URL
	task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "save retry"})
	if err != nil {
		t.Fatal(err)
	}
	failed := done(t, m, task.ID)
	if failed.ArtifactStatus != "failed" || failed.ResultKey == "" {
		t.Fatalf("missing recoverable output %+v", failed)
	}
	plain, _ := common.Decrypt(failed.Snapshot)
	if strings.Contains(plain, "private-upstream-key") {
		t.Fatal("credential retained after generation")
	}
	views, _ := Views([]model.ImageGenTask{*failed})
	if !views[0].CanRetrySave {
		t.Fatal("retry unavailable")
	}
	if _, err = m.RetrySave(2, nil, task.ID); err == nil {
		t.Fatal("cross-user saving retry")
	}
	hold(m)
	if _, err = m.RetrySave(1, nil, task.ID); err != nil {
		t.Fatal(err)
	}
	m.Close()
	unhold(m)
	failing.fail.Store(false)
	restarted := NewManager(m.DB, failing)
	defer restarted.Close()
	restarted.ProcessDue()
	final := done(t, restarted, task.ID)
	views, _ = Views([]model.ImageGenTask{*final})
	if final.Status != "completed" || len(views[0].Images) != 1 || calls.Load() != 1 {
		t.Fatalf("saving retry regenerated or duplicated: %+v calls=%d", views, calls.Load())
	}
	if _, err = restarted.RetrySave(1, nil, task.ID); err == nil {
		t.Fatal("ready task retry accepted")
	}
}

func TestUpstreamTimeoutAndRejectionClassification(t *testing.T) {
	for _, code := range []int{400, 429, 503, 0} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			m, cfg, _ := fixture(t)
			cfg.TimeoutSeconds = 1
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if code == 0 {
					select {
					case <-r.Context().Done():
					case <-time.After(1500 * time.Millisecond):
					}
					return
				}
				w.WriteHeader(code)
				fmt.Fprint(w, `{"error":{"message":"private-upstream-key rejected"}}`)
			}))
			defer server.Close()
			cfg.EndpointURL = server.URL
			task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "draw"})
			if err != nil {
				t.Fatal(err)
			}
			final := done(t, m, task.ID)
			want := "failed"
			if code == 0 || code >= 500 {
				want = "submission_unknown"
			}
			if final.Status != want || final.Snapshot != "" || strings.Contains(final.Error, "private-upstream-key") {
				t.Fatalf("classification/secrets: %+v", final)
			}
			m.ProcessDue()
			if calls.Load() != 1 {
				t.Fatal("automatic paid resubmission")
			}
		})
	}
}

func TestControlsAndReferenceValidationBeforeAccepting(t *testing.T) {
	m, cfg, data := fixture(t)
	hold(m)
	defer unhold(m)
	cfg.EndpointURL = "https://gateway.example"
	for _, args := range []map[string]any{
		{"prompt": "x", "wait_seconds": float64(31)}, {"prompt": "x", "wait_seconds": 1.5},
		{"prompt": "x", "wait_seconds": "1"}, {"prompt": "x", "request_id": ""},
		{"prompt": "x", "extra_body": map[string]any{"wait_seconds": float64(2)}},
		{"prompt": "x", "images": []any{"http://127.0.0.1/a.png"}},
	} {
		if _, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", args); err == nil {
			t.Fatalf("invalid input accepted %+v", args)
		}
	}
	if _, err := m.Submit(context.Background(), 1, cfg, "edit_image", "mcp", map[string]any{"prompt": "x", "images": []any{"http://127.0.0.1/a.png"}}); err == nil {
		t.Fatal("private reference URL accepted")
	}
	task, err := m.Submit(context.Background(), 1, cfg, "edit_image", "test", map[string]any{"prompt": "edit", "images": []any{"data:image/png;base64," + base64.StdEncoding.EncodeToString(data)}})
	if err != nil {
		t.Fatal(err)
	}
	if task.ReferenceCount != 1 || strings.Contains(task.ReferenceImages, "base64") {
		t.Fatal("reference bytes leaked into history")
	}
	plain, _ := common.Decrypt(task.Snapshot)
	if strings.Contains(plain, "base64") {
		t.Fatal("snapshot embeds reference bytes")
	}
	views, _ := Views([]model.ImageGenTask{*task})
	if len(views[0].ReferenceImages) != 1 || views[0].ReferenceImages[0].URL == "" {
		t.Fatal("missing input image history")
	}
	var count int64
	m.DB.Model(&model.ImageGenTask{}).Count(&count)
	if count != 1 {
		t.Fatalf("invalid inputs created tasks: %d", count)
	}
}

func TestSavingCheckpointRecovery(t *testing.T) {
	m, cfg, data := fixture(t)
	hold(m)
	defer unhold(m)
	cfg.EndpointURL = "https://gateway.example"
	task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "checkpoint"})
	if err != nil {
		t.Fatal(err)
	}
	key := "imagegen/tasks/" + task.ID + "/checkpoint.json"
	encoded, _ := json.Marshal(&imagegen.Result{Images: []imagegen.Image{{Bytes: data}}})
	if err = m.Store.Put(context.Background(), key, bytes.NewReader(encoded), "application/json"); err != nil {
		t.Fatal(err)
	}
	m.DB.Model(task).Updates(map[string]any{"status": "saving", "artifact_status": "saving", "result_key": key})
	restarted := NewManager(m.DB, m.Store)
	defer restarted.Close()
	restarted.ProcessDue()
	final := done(t, restarted, task.ID)
	if final.Status != "completed" {
		t.Fatalf("checkpoint did not recover %+v", final)
	}
	if err = restarted.Delete(1, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = restarted.Get(1, nil, task.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("delete failed", err)
	}
}

func TestPartialSaveRetrySkipsExistingOutput(t *testing.T) {
	m, cfg, first := fixture(t)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	second := buf.Bytes()
	sum := sha256.Sum256(second)
	failing := &failingAssets{Storage: m.Store, failKey: storage.ContentKey(fmt.Sprintf("%x", sum))}
	failing.fail.Store(true)
	m.Store = failing
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s"},{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString(first), base64.StdEncoding.EncodeToString(second))
	}))
	defer server.Close()
	cfg.EndpointURL = server.URL
	task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "two", "n": float64(2)})
	if err != nil {
		t.Fatal(err)
	}
	partial := done(t, m, task.ID)
	if partial.Status != "partial" || partial.ArtifactStatus != "partial" {
		t.Fatalf("wrong partial result %+v", partial)
	}
	var before model.ImageGenTaskImage
	m.DB.Where("task_id = ?", task.ID).First(&before)
	failing.fail.Store(false)
	if _, err = m.RetrySave(1, nil, task.ID); err != nil {
		t.Fatal(err)
	}
	final := done(t, m, task.ID)
	var outputs []model.ImageGenTaskImage
	m.DB.Where("task_id = ?", task.ID).Order("output_index ASC").Find(&outputs)
	if final.Status != "completed" || len(outputs) != 2 || outputs[0].ID != before.ID || calls.Load() != 1 {
		t.Fatalf("partial retry duplicated outputs %+v calls=%d", outputs, calls.Load())
	}
}

func TestConcurrentIdempotentSubmissionCreatesOnePaidTask(t *testing.T) {
	m, cfg, data := fixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(80 * time.Millisecond)
		respondImage(w, data)
	}))
	defer server.Close()
	cfg.EndpointURL = server.URL
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "concurrent", "request_id": "same"})
			if err != nil {
				errs <- err
			} else {
				ids <- task.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	id := ""
	for next := range ids {
		if id != "" && id != next {
			t.Fatal("duplicate task IDs")
		}
		id = next
	}
	if id == "" {
		t.Fatal("no accepted tasks")
	}
	done(t, m, id)
	var count int64
	m.DB.Model(&model.ImageGenTask{}).Count(&count)
	if count != 1 || calls.Load() != 1 {
		t.Fatalf("concurrent retries generated multiple tasks: records=%d calls=%d", count, calls.Load())
	}
}

func TestImageQueueFullFailsWithoutResubmission(t *testing.T) {
	m, cfg, _ := fixture(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		fmt.Fprint(w, `{"error":{"message":"image queue is full, please retry later (request id: 20261017082618341048268EJforpaI)"}}`)
	}))
	defer server.Close()
	cfg.EndpointURL = server.URL
	task, err := m.Submit(context.Background(), 1, cfg, "generate_image", "mcp", map[string]any{"prompt": "draw", "request_id": "queue-full"})
	if err != nil {
		t.Fatal(err)
	}
	final := done(t, m, task.ID)
	if final.Status != "failed" || final.ArtifactStatus != "failed" || final.Snapshot != "" {
		t.Fatalf("queue rejection misclassified: %+v", final)
	}
	m.ProcessDue()
	if calls.Load() != 1 {
		t.Fatal("queue rejection was automatically resubmitted")
	}
}

func TestReconcileImageQueueFullHistory(t *testing.T) {
	m, _, _ := fixture(t)
	message := "image API HTTP 503: image queue is full, please retry later (request id: 20261017082618341048268EJforpaI)"
	finished := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	rows := []model.ImageGenTask{
		{ID: "repair", Status: "submission_unknown", Error: message},
		{ID: "generic", Status: "submission_unknown", Error: "image API HTTP 503: service unavailable"},
		{ID: "timeout", Status: "submission_unknown", Error: "image API HTTP 504: image queue is full, please retry later"},
		{ID: "active", Status: "running", Error: message},
		{ID: "leased", Status: "submission_unknown", Error: message, LeaseUntil: &future},
		{ID: "saved", Status: "submission_unknown", Error: message, ResultKey: "private-output"},
	}
	for i := range rows {
		rows[i].UserID = 1
		rows[i].FinishedAt = &finished
		rows[i].DurationMs = 500
		rows[i].Snapshot = "private"
		if err := m.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := m.reconcileRejectedSubmissions(); err != nil {
			t.Fatal(err)
		}
	}
	for _, before := range rows {
		after, err := m.Get(1, nil, before.ID)
		if err != nil {
			t.Fatal(err)
		}
		if before.ID == "repair" {
			if after.Status != "failed" || after.ArtifactStatus != "failed" || after.Snapshot != "" {
				t.Fatalf("history not repaired: %+v", after)
			}
		} else if after.Status != before.Status || after.Snapshot != before.Snapshot {
			t.Fatalf("unrelated task changed: %+v", after)
		}
		if after.Error != before.Error || !after.CreatedAt.Equal(before.CreatedAt) || !after.FinishedAt.Equal(finished) || after.DurationMs != 500 {
			t.Fatal("repair changed task identity or timing")
		}
	}
}
