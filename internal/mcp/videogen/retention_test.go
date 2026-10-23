package videogen

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/model"
)

func TestPermanentVideoRetentionAndQuota(t *testing.T) {
	m := managerFixture(t)
	model.UpdateOption("VideoGenRetentionHours", "0")
	expired := time.Now().Add(-time.Hour)
	stopped := time.Now().Add(-31 * 24 * time.Hour)
	sid := "old-finite"
	old := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, IdempotencyKey: uuid.NewString(), Status: "completed", ArtifactStatus: "ready", StorageKey: "video/1/old-finite", ShortID: &sid, MIME: "video/mp4", ExpiresAt: &expired, StoppedAt: &stopped, LeaseUntil: unleasedAt}
	if err := m.Store.Put(context.Background(), old.StorageKey, strings.NewReader("old video"), "video/mp4"); err != nil {
		t.Fatal(err)
	}
	if err := m.DB.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	m.Cleanup()
	if _, err := m.Get(1, nil, old.ID, false); err != nil {
		t.Fatal("zero retention removed an existing video/history", err)
	}
	if _, err := m.Store.Stat(context.Background(), old.StorageKey); err != nil {
		t.Fatal("zero retention removed existing video bytes", err)
	}
	view := TaskView(&old)
	if view["artifact_status"] != "ready" || len(view["videos"].([]any)) != 1 {
		t.Fatal("zero retention still expired existing video")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			io.WriteString(w, `{"id":"permanent-video","status":"completed"}`)
			return
		}
		w.Write(mp4Bytes())
	}))
	defer server.Close()
	job, err := m.Submit(context.Background(), 1, configFor(server.URL), "generate", "test", map[string]any{"prompt": "permanent scene"})
	if err != nil {
		t.Fatal(err)
	}
	row := awaitTask(t, m, job.ID, func(row *model.VideoGenTask) bool { return row.ArtifactStatus == "ready" })
	if row.ExpiresAt != nil {
		t.Fatal("new permanent video received an expiry")
	}
	raw, _ := MCPResult(row)
	var result struct {
		Content    []map[string]any `json:"content"`
		Structured struct {
			Videos []struct {
				ExpiresAt *time.Time `json:"expires_at"`
			} `json:"videos"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Structured.Videos) != 1 || result.Structured.Videos[0].ExpiresAt != nil || !strings.Contains(string(raw), "resource_link") {
		t.Fatalf("permanent MCP resource unavailable: %s, %v", raw, err)
	}
	m.DB.Model(row).Update("stopped_at", stopped)
	model.UpdateOption("VideoGenRetentionHours", "24")
	m.Cleanup()
	if _, err := m.Get(1, nil, old.ID, false); err == nil {
		t.Fatal("finite retention did not resume existing expiry")
	}
	if _, err := m.Get(1, nil, row.ID, false); err != nil {
		t.Fatal("history cleanup removed a permanently saved video", err)
	}
	if _, err := m.Store.Stat(context.Background(), row.StorageKey); err != nil {
		t.Fatal("permanent bytes were cleaned", err)
	}
	m.DB.Create(&model.User{ID: 1, Username: "video-owner", AffCode: "video-owner"})
	model.UpdateOption("MaxVideosPerUser", "1")
	next, err := m.Submit(context.Background(), 1, configFor(server.URL), "generate", "test", map[string]any{"prompt": "another scene"})
	if err != nil {
		t.Fatal(err)
	}
	blocked := awaitTask(t, m, next.ID, func(row *model.VideoGenTask) bool { return row.ArtifactStatus == "failed" })
	if !strings.Contains(blocked.Error, "quota exceeded") {
		t.Fatal("permanent video did not count toward quota", blocked.Error)
	}
}

func TestPermanentVideoHistoryStillExpiresFiles(t *testing.T) {
	m := managerFixture(t)
	model.UpdateOption("VideoGenHistoryDays", "0")
	old := time.Now().Add(-31 * 24 * time.Hour)
	expired := time.Now().Add(-time.Hour)
	recent := time.Now().Add(-time.Hour)
	leasedUntil := time.Now().Add(time.Hour)
	rows := []model.VideoGenTask{
		{ID: uuid.NewString(), Status: "completed", ArtifactStatus: "ready", StorageKey: "video/1/expired", ExpiresAt: &expired, StoppedAt: &old},
		{ID: uuid.NewString(), Status: "failed", StoppedAt: &old},
		{ID: uuid.NewString(), Status: "failed", StoppedAt: &recent},
		{ID: uuid.NewString(), Status: "running", StoppedAt: &old},
		{ID: uuid.NewString(), Status: "failed", StoppedAt: &old, LeaseUntil: leasedUntil},
	}
	for i := range rows {
		row := &rows[i]
		row.UserID, row.IdempotencyKey, row.CreatedAt = 1, uuid.NewString(), old
		if row.LeaseUntil.IsZero() {
			row.LeaseUntil = unleasedAt
		}
		if err := m.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Store.Put(context.Background(), rows[0].StorageKey, strings.NewReader("expired video"), "video/mp4"); err != nil {
		t.Fatal(err)
	}
	m.Cleanup()
	for _, row := range rows {
		if _, err := m.Get(1, nil, row.ID, false); err != nil {
			t.Fatal("permanent video history was cleaned", err)
		}
	}
	if _, err := m.Store.Stat(context.Background(), rows[0].StorageKey); err == nil {
		t.Fatal("permanent history prevented expired file cleanup")
	}
	model.UpdateOption("VideoGenHistoryDays", "30")
	m.Cleanup()
	for i, row := range rows {
		var count int64
		if err := m.DB.Model(&model.VideoGenTask{}).Where("id = ?", row.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(1)
		if i < 2 {
			want = 0
		}
		if count != want {
			t.Fatalf("video history %s count = %d, want %d", row.ID, count, want)
		}
	}
}
