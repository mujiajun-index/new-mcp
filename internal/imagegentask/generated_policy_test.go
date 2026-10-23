package imagegentask

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
)

func TestGeneratedQuotaRetryAndDeletion(t *testing.T) {
	m, cfg, data := fixture(t)
	var second bytes.Buffer
	png.Encode(&second, image.NewNRGBA(image.Rect(0, 0, 5, 4)))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			respondImage(w, data)
		} else {
			respondImage(w, second.Bytes())
		}
	}))
	defer server.Close()
	cfg.EndpointURL = server.URL
	first, err := m.Submit(context.Background(), 1, cfg, "generate_image", "test", map[string]any{"prompt": "first"})
	if err != nil {
		t.Fatal(err)
	}
	if row := done(t, m, first.ID); row.ArtifactStatus != "ready" {
		t.Fatal(row)
	}
	model.UpdateOption("MaxImagesPerUser", "1")
	if _, err := m.Submit(context.Background(), 1, cfg, "generate_image", "test", map[string]any{"prompt": "blocked"}); err == nil || calls.Load() != 1 {
		t.Fatal("full cap did not reject before generation", err)
	}

	// An accepted task keeps recoverable output if the cap changes before saving.
	model.UpdateOption("MaxImagesPerUser", "0")
	failing := &failingAssets{Storage: m.Store}
	failing.fail.Store(true)
	m.Store = failing
	next, err := m.Submit(context.Background(), 1, cfg, "generate_image", "test", map[string]any{"prompt": "next"})
	if err != nil {
		t.Fatal(err)
	}
	if row := done(t, m, next.ID); row.ArtifactStatus != "failed" || row.ResultKey == "" {
		t.Fatal("recoverable output missing", row)
	}
	model.UpdateOption("MaxImagesPerUser", "1")
	failing.fail.Store(false)
	if _, err := m.RetrySave(1, nil, next.ID); err != nil {
		t.Fatal(err)
	}
	if row := done(t, m, next.ID); row.ArtifactStatus != "failed" || row.ResultKey == "" {
		t.Fatal("cap bypassed during saving retry", row)
	}
	if calls.Load() != 2 {
		t.Fatal("saving retry generated again")
	}
	if err := m.Delete(1, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := imageasset.CheckGeneratedQuota(1); err != nil {
		t.Fatal("deletion did not release quota", err)
	}
	if _, err := m.RetrySave(1, nil, next.ID); err != nil {
		t.Fatal(err)
	}
	if row := done(t, m, next.ID); row.ArtifactStatus != "ready" || calls.Load() != 2 {
		t.Fatal("quota retry failed or regenerated", row, calls.Load())
	}
}

func TestDeletingSharedGeneratedImagesKeepsUploadsAndReferences(t *testing.T) {
	m, cfg, data := fixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { respondImage(w, data) }))
	defer server.Close()
	cfg.EndpointURL = server.URL
	var tasks []string
	for _, prompt := range []string{"one", "two"} {
		row, err := m.Submit(context.Background(), 1, cfg, "generate_image", "test", map[string]any{"prompt": prompt})
		if err != nil {
			t.Fatal(err)
		}
		if result := done(t, m, row.ID); result.ArtifactStatus != "ready" {
			t.Fatal(result)
		}
		tasks = append(tasks, row.ID)
	}
	input, _, _, err := imagegen.Inspect(data, 1024)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := taskreference.Capture(context.Background(), m.Store, 1, input, "reference")
	if err != nil {
		t.Fatal(err)
	}
	upload, err := imageasset.Save(context.Background(), m.Store, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	model.UpdateOption("MaxImagesPerUser", "1")
	if err := m.Delete(1, tasks[0]); err != nil {
		t.Fatal(err)
	}
	if err := imageasset.CheckGeneratedQuota(1); err == nil {
		t.Fatal("shared generated image quota released too soon")
	}
	row, err := m.Get(1, nil, tasks[1])
	if err != nil {
		t.Fatal(err)
	}
	views, err := Views([]model.ImageGenTask{*row})
	if err != nil || len(views[0].Images) != 1 || views[0].Images[0].Status != "ready" {
		t.Fatal("shared task output deleted", views, err)
	}
	if err := m.Delete(1, tasks[1]); err != nil {
		t.Fatal(err)
	}
	if err := imageasset.CheckGeneratedQuota(1); err != nil {
		t.Fatal("last output did not release quota", err)
	}
	if _, err := model.GetUploadedImageByID(upload.UploadID); err != nil {
		t.Fatal("upload deleted", err)
	}
	refs := taskreference.Views(m.DB, 1, taskreference.Encode([]taskreference.Image{ref}))
	if refs[0].Status != "ready" {
		t.Fatal("reference deleted", refs)
	}
	if _, err := m.Store.Stat(context.Background(), upload.Key); err != nil {
		t.Fatal("shared blob deleted", err)
	}
}
