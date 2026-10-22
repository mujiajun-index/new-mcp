package audiogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func managerDB(t *testing.T) (*gorm.DB, storage.Storage) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audio.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	sql.SetMaxOpenConns(1)
	if e = db.AutoMigrate(&model.AudioGenConfig{}, &model.AudioGenConnection{}, &model.AudioGenModel{}, &model.AudioGenTask{}, &model.AudioAsset{}, &model.UploadedImage{}, &model.Option{}); e != nil {
		t.Fatal(e)
	}
	oldDB := model.DB
	model.DB = db
	model.OptionMapMutex.Lock()
	oldMap := model.OptionMap
	model.OptionMap = map[string]string{"ServerAddress": "http://localhost:3000", "AudioGenConcurrency": "4"}
	model.OptionMapMutex.Unlock()
	t.Cleanup(func() {
		model.DB = oldDB
		model.OptionMapMutex.Lock()
		model.OptionMap = oldMap
		model.OptionMapMutex.Unlock()
		_ = sql.Close()
	})
	store, e := storage.New(context.Background(), storage.Config{LocalRoot: t.TempDir()})
	if e != nil {
		t.Fatal(e)
	}
	return db, store
}
func managerConfig(t *testing.T, base string, kind string) *model.AudioGenConfig {
	t.Helper()
	key, e := common.Encrypt("private-test-key")
	if e != nil {
		t.Fatal(e)
	}
	s := testSlot(kind, "openai_speech")
	if kind == "audio" {
		s = testSlot(kind, "openai_chat_audio")
	}
	return &model.AudioGenConfig{ID: 42, UserID: 1, Name: "Audio test", Connections: []model.AudioGenConnection{{Key: "main", BaseURL: base, APIKey: key}}, Models: []model.AudioGenModel{s}}
}
func awaitAudio(t *testing.T, m *Manager, id string, status, artifact string) *model.AudioGenTask {
	t.Helper()
	until := time.Now().Add(8 * time.Second)
	for time.Now().Before(until) {
		row, e := m.Get(1, nil, id)
		if e == nil && row.Status == status && (artifact == "" || row.ArtifactStatus == artifact) {
			return row
		}
		time.Sleep(25 * time.Millisecond)
	}
	row, _ := m.Get(1, nil, id)
	t.Fatalf("task did not reach %s/%s: %+v", status, artifact, row)
	return nil
}

func TestTaskSnapshotsDedupeInlineAndOwnership(t *testing.T) {
	db, store := managerDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p["model"] != "test-model" || r.Header.Get("Authorization") != "Bearer private-test-key" {
			t.Error("did not use accepted connection snapshot")
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(testWAV())
	}))
	defer server.Close()
	m := NewManager(db, store)
	cfg := managerConfig(t, server.URL, "speech")
	args := jsonArgs(`{"text":"hello","request_id":"once","wait_seconds":0}`)
	task, e := m.Submit(context.Background(), 1, cfg, "speech", args, "test")
	if e != nil {
		t.Fatal(e)
	}
	same, e := m.Submit(context.Background(), 1, cfg, "speech", args, "test")
	if e != nil || same.ID != task.ID {
		t.Fatal(same, e)
	}
	if _, e = m.Submit(context.Background(), 1, cfg, "speech", jsonArgs(`{"text":"different","request_id":"once"}`), "test"); e == nil {
		t.Fatal("accepted changed idempotent input")
	}
	cfg.Connections[0].BaseURL = "http://invalid.invalid"
	cfg.Models[0].Model = "changed"
	m.Start(context.Background())
	defer m.Close()
	completed := awaitAudio(t, m, task.ID, "completed", "ready")
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	if _, e = m.Get(2, nil, task.ID); e == nil {
		t.Fatal("cross-user access")
	}
	other := int64(99)
	if _, e = m.Get(1, &other, task.ID); e == nil {
		t.Fatal("cross-config access")
	}
	raw, e := m.Result(context.Background(), completed, "inline")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(raw, []byte(`"type":"audio"`)) || bytes.Contains(raw, []byte("private-test-key")) || bytes.Contains(raw, []byte("audio-stage/")) {
		t.Fatal(string(raw))
	}
	if completed.Connection != "" {
		t.Fatal("task retained connection after staging")
	}
	model.OptionMapMutex.Lock()
	model.OptionMap["AudioGenInlineMaxBytes"] = "1"
	model.OptionMapMutex.Unlock()
	raw, e = m.Result(context.Background(), completed, "inline")
	if e != nil || bytes.Contains(raw, []byte(`"type":"audio"`)) || !bytes.Contains(raw, []byte(`"type":"resource_link"`)) {
		t.Fatal(string(raw), e)
	}
}

type failAudioStore struct {
	storage.Storage
	fail atomic.Bool
}

func (s *failAudioStore) Put(ctx context.Context, key string, r io.Reader, mime string) error {
	if s.fail.Load() && strings.HasPrefix(key, "audio/") {
		return fmt.Errorf("simulated storage failure")
	}
	return s.Storage.Put(ctx, key, r, mime)
}
func TestSaveRetrySurvivesRestartWithoutRegeneration(t *testing.T) {
	db, base := managerDB(t)
	store := &failAudioStore{Storage: base}
	store.fail.Store(true)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(testWAV())
	}))
	defer server.Close()
	m := NewManager(db, store)
	m.Start(context.Background())
	task, e := m.Submit(context.Background(), 1, managerConfig(t, server.URL, "speech"), "speech", jsonArgs(`{"text":"hello"}`), "test")
	if e != nil {
		t.Fatal(e)
	}
	failed := awaitAudio(t, m, task.ID, "completed", "failed")
	if failed.OutputKey == "" {
		t.Fatal("no durable output")
	}
	m.Close()
	store.fail.Store(false)
	m2 := NewManager(db, store)
	m2.Start(context.Background())
	defer m2.Close()
	if _, e = m2.RetrySave(1, nil, task.ID); e != nil {
		t.Fatal(e)
	}
	awaitAudio(t, m2, task.ID, "completed", "ready")
	if calls.Load() != 1 {
		t.Fatalf("saving generated another paid request: %d", calls.Load())
	}
}

func TestRestartNeverResubmitsUnknownAndReplaysStagedOutput(t *testing.T) {
	db, store := managerDB(t)
	m := NewManager(db, store)
	cfg := managerConfig(t, "http://unused.invalid", "speech")
	first, e := m.Submit(context.Background(), 1, cfg, "speech", jsonArgs(`{"text":"first"}`), "test")
	if e != nil {
		t.Fatal(e)
	}
	second, e := m.Submit(context.Background(), 1, cfg, "speech", jsonArgs(`{"text":"second"}`), "test")
	if e != nil {
		t.Fatal(e)
	}
	past := time.Now().Add(-time.Hour)
	db.Model(&model.AudioGenTask{}).Where("id IN ?", []string{first.ID, second.ID}).Updates(map[string]any{"status": "running", "lease_until": past})
	if e = m.stage(context.Background(), "audio-stage/"+second.ID+"/0", &Output{Audios: []Audio{{Data: testWAV()}}, Lyrics: "recovered lyrics"}); e != nil {
		t.Fatal(e)
	}
	m.Start(context.Background())
	defer m.Close()
	unknown := awaitAudio(t, m, first.ID, "submission_unknown", "")
	if _, e = m.Retry(context.Background(), 1, unknown.ID, cfg, nil); e == nil {
		t.Fatal("unknown task was regenerated")
	}
	ready := awaitAudio(t, m, second.ID, "completed", "ready")
	if ready.Lyrics != "recovered lyrics" {
		t.Fatal(ready.Lyrics)
	}
}

func TestAudioInputCapturedAndValidatedBeforeSubmission(t *testing.T) {
	db, store := managerDB(t)
	var calls atomic.Int32
	encoded := base64.StdEncoding.EncodeToString(testWAV())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		b, _ := json.Marshal(p)
		if !bytes.Contains(b, []byte(encoded)) || bytes.Contains(b, []byte("http://localhost:3000/a/")) {
			t.Error("audio URL was passed instead of Base64")
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"audio":{"data":%q,"transcript":"reply"}}}]}`, encoded)
	}))
	defer server.Close()
	asset, e := audioasset.Save(context.Background(), db, store, 1, "", "upload", testWAV())
	if e != nil {
		t.Fatal(e)
	}
	cfg := managerConfig(t, server.URL, "audio")
	m := NewManager(db, store)
	args := map[string]any{"audios": []any{audioasset.URL("GET", asset.ShortID)}, "request_id": "recording"}
	task, e := m.Submit(context.Background(), 1, cfg, "audio", args, "test")
	if e != nil {
		t.Fatal(e)
	}
	_ = store.Delete(context.Background(), asset.StorageKey)
	m.Start(context.Background())
	defer m.Close()
	done := awaitAudio(t, m, task.ID, "completed", "ready")
	if done.Transcript != "reply" || done.InputTranscript != "" {
		t.Fatal("input transcript was invented")
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	before := calls.Load()
	if _, e = m.Submit(context.Background(), 1, cfg, "audio", map[string]any{"audios": []any{"http://127.0.0.1/private.wav"}}, "test"); e == nil {
		t.Fatal("accepted private reference URL")
	}
	if calls.Load() != before {
		t.Fatal("invalid input reached upstream")
	}
}

func TestExplicitFailureRetryKeepsTaskAndKind(t *testing.T) {
	db, store := managerDB(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(testWAV())
	}))
	defer server.Close()
	m := NewManager(db, store)
	m.Start(context.Background())
	defer m.Close()
	cfg := managerConfig(t, server.URL, "speech")
	task, e := m.Submit(context.Background(), 1, cfg, "speech", jsonArgs(`{"text":"hello"}`), "test")
	if e != nil {
		t.Fatal(e)
	}
	failed := awaitAudio(t, m, task.ID, "failed", "")
	cfg.ID = 99
	retried, e := m.Retry(context.Background(), 1, task.ID, cfg, nil)
	if e != nil {
		t.Fatal(e)
	}
	done := awaitAudio(t, m, retried.ID, "completed", "ready")
	if done.ID != failed.ID || !done.CreatedAt.Equal(failed.CreatedAt) || done.RetryCount != 1 || done.ConfigID != 42 || done.ExecutionConfigID != 99 {
		t.Fatalf("retry lost task identity: %+v", done)
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
}
