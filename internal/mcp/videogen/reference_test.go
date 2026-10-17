package videogen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
)

func TestVideoHistoryReferenceSurvivesFailureRestartAndSameTaskRetry(t *testing.T) {
	m := managerFixture(t)
	model.UpdateOption("ServerAddress", "https://files.example")
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && posts.Add(1) == 1 {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"rejected"}}`)
			return
		}
		io.WriteString(w, `{"id":"video-job","status":"queued"}`)
	}))
	defer server.Close()
	var first, second bytes.Buffer
	png.Encode(&first, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	png.Encode(&second, image.NewNRGBA(image.Rect(0, 0, 3, 4)))
	cfg := configFor(server.URL)
	task, err := m.Submit(context.Background(), 1, cfg, "image", "test", map[string]any{"prompt": "animate", "image": base64.StdEncoding.EncodeToString(first.Bytes()), "request_id": "reference-task"})
	if err != nil {
		t.Fatal(err)
	}
	task = awaitTask(t, m, task.ID, func(t *model.VideoGenTask) bool { return t.Status == "failed" })
	refs := TaskView(task)["reference_images"].([]taskreference.View)
	if len(refs) != 1 || refs[0].Width != 2 || refs[0].URL == "" || task.Connection != "" {
		t.Fatalf("failed inputs: %+v", refs)
	}
	firstURL := refs[0].URL
	m.Close()
	restarted := NewManager(m.DB, m.Store)
	defer restarted.Close()
	loaded, err := restarted.Get(1, nil, task.ID, false)
	if err != nil || TaskView(loaded)["reference_images"].([]taskreference.View)[0].URL != firstURL {
		t.Fatal("restart lost references", err)
	}
	uploaded, err := imageasset.Save(context.Background(), m.Store, 1, bytes.NewReader(second.Bytes()), 1024)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := restarted.RetryGeneration(context.Background(), 1, task.ID, cfg, map[string]any{"prompt": "new motion", "image": uploaded.URL}, "retry-reference")
	if err != nil {
		t.Fatal(err)
	}
	retried = awaitTask(t, restarted, retried.ID, func(t *model.VideoGenTask) bool { return t.UpstreamID != "" })
	refs = TaskView(retried)["reference_images"].([]taskreference.View)
	var count int64
	m.DB.Model(&model.VideoGenTask{}).Count(&count)
	if retried.ID != task.ID || count != 1 || posts.Load() != 2 || refs[0].URL != uploaded.URL || refs[0].Width != 3 || refs[0].Height != 4 {
		t.Fatalf("retry reference not updated: %+v", refs)
	}
	// Legacy own-upload URLs can be recovered without a network fetch.
	input, _ := json.Marshal(map[string]any{"image": uploaded.URL})
	stored, _ := common.Encrypt(string(input))
	legacy := &model.VideoGenTask{UserID: 1, Operation: "image", RequestParams: stored}
	if refs := TaskView(legacy)["reference_images"].([]taskreference.View); len(refs) != 1 || refs[0].URL != uploaded.URL {
		t.Fatal("legacy upload not recovered")
	}
	legacy.UserID = 2
	if len(TaskView(legacy)["reference_images"].([]taskreference.View)) != 0 {
		t.Fatal("cross-user legacy reference")
	}
}
