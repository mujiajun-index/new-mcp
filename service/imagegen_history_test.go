package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/imagegentask"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func TestImageGenHistoryKeepsReferenceImagesAndMaskOnFailure(t *testing.T) {
	db := imageGenTestSetup(t)
	first := imageGenPNG(t, 2, 2)
	second := imageGenPNG(t, 3, 2)
	output := imageGenPNG(t, 4, 3)
	uploaded, err := imageasset.Save(context.Background(), UploadStore, 1, bytes.NewReader(first), 1024)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("prompt") == "fail editing" {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"rejected"}}`)
			return
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, base64.StdEncoding.EncodeToString(output))
	}))
	defer server.Close()
	s := &ImageGenService{}
	req := imageGenReq(server.URL)
	req.EditRequestFormat = igPtr(imagegen.EditFormatMultipart)
	detail, err := s.Create(1, req)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := model.GetImageGenConfig(1, detail.ID)
	for _, prompt := range []string{"edit images", "fail editing"} {
		raw, _ := json.Marshal(map[string]any{"prompt": prompt, "images": []string{uploaded.URL, base64.StdEncoding.EncodeToString(second)}, "mask": uploaded.URL})
		_, runErr := virtual.RunImageGen(context.Background(), UploadStore, 1, cfg, model.EditImageToolName, raw, "test")
		if (prompt == "fail editing") != (runErr != nil) {
			t.Fatal("unexpected generation result", runErr)
		}
		var task model.ImageGenTask
		if err := db.Where("prompt = ?", prompt).First(&task).Error; err != nil {
			t.Fatal(err)
		}
		views, err := imagegentask.Views([]model.ImageGenTask{task})
		if err != nil {
			t.Fatal(err)
		}
		refs := views[0].ReferenceImages
		if task.ReferenceCount != 2 || len(refs) != 3 || refs[0].URL != uploaded.URL || refs[0].Width != 2 || refs[1].Width != 3 || refs[2].Role != "mask" || refs[1].Status != "ready" {
			t.Fatalf("lost inputs: %+v", refs)
		}
		if strings.Contains(task.ReferenceImages, "base64") || strings.Contains(task.ReferenceImages, "?s=") {
			t.Fatal("input bytes or signed URLs stored in history")
		}
		if _, err := imagegentask.Get(2, task.ID); err == nil {
			t.Fatal("cross-user access")
		}
		if err := imagegentask.Delete(1, task.ID, UploadStore); err != nil {
			t.Fatal(err)
		}
	}
	if _, total, err := model.ListAllUploads(0, 0, 20); err != nil || total != 1 {
		t.Fatal("task references changed upload management", total, err)
	}
}

func TestImageGenTaskHistoryLifecycleAndStorageSeparation(t *testing.T) {
	db := imageGenTestSetup(t)
	data := imageGenPNG(t, 4, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p["prompt"] == "fail" {
			w.WriteHeader(503)
			fmt.Fprint(w, "secret-image-key no available server")
			return
		}
		fmt.Fprintf(w, `{"data":[{"b64_json":"%s","revised_prompt":"revised"}]}`, base64.StdEncoding.EncodeToString(data))
	}))
	defer server.Close()
	s := &ImageGenService{}
	cfg, err := s.Create(1, imageGenReq(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.Test(context.Background(), 1, &dto.ImageGenProbeReq{ConfigID: cfg.ID, Arguments: json.RawMessage(`{"prompt":"draw"}`)})
	if err != nil || preview.IsError {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	var task model.ImageGenTask
	if err := db.First(&task).Error; err != nil || task.Status != "completed" || task.Source != "test" || task.Operation != "generate" || task.FinishedAt == nil {
		t.Fatalf("history: %+v %v", task, err)
	}
	view, err := imagegentask.Views([]model.ImageGenTask{task})
	if err != nil || len(view) != 1 || len(view[0].Images) != 1 {
		t.Fatalf("view: %+v %v", view, err)
	}
	img := view[0].Images[0]
	if img.Status != "ready" || img.URL == "" || img.Width != 4 || img.Height != 3 || img.MIME != "image/png" || img.Size != int64(len(data)) || img.RevisedPrompt != "revised" || img.ExpiresAt == nil {
		t.Fatalf("image metadata: %+v", img)
	}
	encoded, _ := json.Marshal(view)
	for _, private := range []string{"api_key", "secret-image-key", "upload_id", "storage_key", "short_id"} {
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatalf("history exposes %s: %s", private, encoded)
		}
	}
	if _, err := imagegentask.Get(2, task.ID); err == nil {
		t.Fatal("other user read history")
	}
	if err := imagegentask.Delete(2, task.ID, UploadStore); err == nil {
		t.Fatal("other user deleted history")
	}
	if items, total, err := model.ListAllUploads(0, 0, 20); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("generated output leaked into admin uploads: %+v %d %v", items, total, err)
	}
	if _, total, err := model.ListUploadsByUser(1, 0, 20); err != nil || total != 0 {
		t.Fatal("generated output leaked into user uploads", total, err)
	}
	if n, _ := model.CountUploadsByUser(1); n != 0 {
		t.Fatal("generated assets counted toward upload quota", n)
	}

	// Uploads of the same bytes share the blob but have independent metadata.
	uploaded, err := imageasset.Save(context.Background(), UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil || uploaded.UploadID == img.UploadID {
		t.Fatal(err)
	}
	generated, err := imageasset.SaveGenerated(context.Background(), UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil || !generated.Deduped || generated.UploadID != img.UploadID {
		t.Fatalf("cross-source dedup failed: %+v %v", generated, err)
	}
	if _, total, err := model.ListAllUploads(0, 0, 20); err != nil || total != 1 {
		t.Fatal("explicit upload hidden after dedup", total, err)
	}

	// Retention changes and cleanup are reflected when history is read, never by
	// retaining expired URLs in the history table.
	old := time.Now().Add(-48 * time.Hour)
	db.Model(&model.UploadedImage{}).Where("id = ?", img.UploadID).Update("created_at", old)
	view, err = imagegentask.Views([]model.ImageGenTask{task})
	if err != nil || view[0].Images[0].Status != "expired" || view[0].Images[0].URL != "" {
		t.Fatalf("expired history: %+v %v", view, err)
	}
	model.UpdateOption("ImageGenRetentionHours", "0")
	view, err = imagegentask.Views([]model.ImageGenTask{task})
	if err != nil || view[0].Images[0].Status != "ready" || view[0].Images[0].ExpiresAt != nil {
		t.Fatalf("permanent history: %+v %v", view, err)
	}
	if err := model.DeleteUploadedImageByID(img.UploadID); err != nil {
		t.Fatal(err)
	}
	view, err = imagegentask.Views([]model.ImageGenTask{task})
	if err != nil || view[0].Images[0].Status != "expired" || view[0].Images[0].URL != "" {
		t.Fatalf("cleaned history: %+v %v", view, err)
	}

	failed, err := s.Test(context.Background(), 1, &dto.ImageGenProbeReq{ConfigID: cfg.ID, Arguments: json.RawMessage(`{"prompt":"fail"}`)})
	if err != nil || !failed.IsError {
		t.Fatal("upstream failure", failed, err)
	}
	var failedTask model.ImageGenTask
	if err := db.Where("status = ?", "submission_unknown").First(&failedTask).Error; err != nil || !strings.Contains(failedTask.Error, "503") || strings.Contains(failedTask.Error, "secret-image-key") {
		t.Fatalf("failure history: %+v %v", failedTask, err)
	}
	if err := s.Delete(1, cfg.ID); err != nil {
		t.Fatal(err)
	}
	if persisted, err := imagegentask.Get(1, task.ID); err != nil || persisted.ConfigName != "Image model" {
		t.Fatal("configuration deletion lost history", err)
	}
	if err := imagegentask.Delete(1, task.ID, UploadStore); err != nil {
		t.Fatal(err)
	}
	var artifacts int64
	db.Model(&model.ImageGenTaskImage{}).Where("task_id = ?", task.ID).Count(&artifacts)
	if artifacts != 0 {
		t.Fatal("deleted history left artifact rows", artifacts)
	}
}

func TestImageGenHistoryRecoveryAndDeleteRollback(t *testing.T) {
	db := imageGenTestSetup(t)
	task, err := imagegentask.Start(context.Background(), 1, &model.ImageGenConfig{UserID: 1, Name: "draft", Provider: "gemini", ModelName: "image"}, "edit_image", "mcp", json.RawMessage(`{"prompt":"edit","images":["data:image/png;base64,private"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if task.ReferenceCount != 1 || task.Operation != "edit" || task.Source != "mcp" {
		t.Fatal(task)
	}
	if err := imagegentask.Delete(1, task.ID, UploadStore); !errors.Is(err, imagegentask.ErrActiveTask) {
		t.Fatal("running task deletion", err)
	}
	if err := imagegentask.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	task, err = imagegentask.Get(1, task.ID)
	if err != nil || task.Status != "interrupted" || task.FinishedAt == nil {
		t.Fatal("restart recovery", task, err)
	}
	if err := imagegentask.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	artifact := model.ImageGenTaskImage{TaskID: task.ID, UploadID: 123, MIME: "image/png"}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("history_delete_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "imagegen_tasks" {
			tx.AddError(fmt.Errorf("injected delete failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := imagegentask.Delete(1, task.ID, UploadStore); err == nil {
		t.Fatal("expected rollback")
	}
	var count int64
	db.Model(&model.ImageGenTaskImage{}).Where("task_id = ?", task.ID).Count(&count)
	if count != 1 {
		t.Fatal("artifact deletion did not roll back")
	}
	if _, err := imagegentask.Get(1, task.ID); err != nil {
		t.Fatal("task deletion did not roll back")
	}
	// History-only deletion must not break a shared stored image.
	data := imageGenPNG(t, 2, 2)
	saved, err := imageasset.SaveGenerated(context.Background(), UploadStore, 1, bytes.NewReader(data), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := imageasset.Save(context.Background(), UploadStore, 1, bytes.NewReader(data), 1024); err != nil {
		t.Fatal(err)
	}
	db.Model(&artifact).Update("upload_id", saved.UploadID)
	db.Callback().Delete().Remove("history_delete_failure")
	if err := imagegentask.Delete(1, task.ID, UploadStore); err != nil {
		t.Fatal(err)
	}
	r, err := UploadStore.Get(context.Background(), saved.Key)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if !bytes.Equal(got, data) {
		t.Fatal("history deletion removed shared blob")
	}
}
