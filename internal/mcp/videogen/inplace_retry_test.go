package videogen

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/model"
)

func TestConcurrentInPlaceRetries(t *testing.T) {
	for _, sameAction := range []bool{true, false} {
		name := "different actions"
		if sameAction {
			name = "same action"
		}
		t.Run(name, func(t *testing.T) {
			m := managerFixture(t)
			second := NewManager(m.DB, m.Store)
			defer second.Close()
			var posts atomic.Int32
			entered := make(chan struct{}, 2)
			release := make(chan struct{})
			var releaseOnce sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				entered <- struct{}{}
				select {
				case <-release:
					io.WriteString(w, `{"id":"retried-upstream","status":"queued"}`)
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			defer releaseOnce.Do(func() { close(release) })
			stopped := time.Now().Add(-time.Hour)
			row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, ConfigID: 1, Source: "test", Operation: "generate", Prompt: "old prompt", IdempotencyKey: uuid.NewString(), Status: "failed", Error: "old failure", LastError: "old query error", Progress: 99, TrackingPaused: true, UpstreamID: "old-failed-upstream", ArtifactStatus: "pending", LeaseUntil: unleasedAt, StoppedAt: &stopped}
			if err := m.DB.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			cfg := configFor(server.URL)
			results := make(chan error, 2)
			start := make(chan struct{})
			for i, manager := range []*Manager{m, second} {
				token := "same-action"
				if !sameAction && i == 1 {
					token = "different-action"
				}
				go func(manager *Manager, token string) {
					<-start
					retried, err := manager.RetryGeneration(context.Background(), 1, row.ID, cfg, map[string]any{"prompt": "new prompt"}, token)
					if err == nil && retried.ID != row.ID {
						t.Error("concurrent retry returned a different task ID")
					}
					results <- err
				}(manager, token)
			}
			close(start)
			successes := 0
			for range 2 {
				if err := <-results; err == nil {
					successes++
				} else if sameAction {
					t.Fatal(err)
				}
			}
			if sameAction && successes != 2 || !sameAction && successes != 1 {
				t.Fatal("unexpected concurrent acceptance count", successes)
			}
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("retry was not submitted")
			}
			updated, err := m.Get(1, nil, row.ID, false)
			if err != nil || updated.Status != "submitting" || updated.RetryCount != 1 || updated.Prompt != "new prompt" || updated.Error != "" || updated.LastError != "" || updated.UpstreamID != "" || updated.Progress != 0 || updated.TrackingPaused || updated.StoppedAt != nil {
				t.Fatal("failed attempt was not reset in place")
			}
			if !updated.CreatedAt.Equal(row.CreatedAt) || posts.Load() != 1 {
				t.Fatal("retry changed row ordering or submitted more than once")
			}
			stale := row
			stale.LeaseOwner = "old-worker"
			m.finish(&stale, map[string]any{"status": "failed", "error": "stale result"})
			current, _ := m.Get(1, nil, row.ID, false)
			if current.Status != "submitting" || current.Error != "" {
				t.Fatal("an old worker overwrote the retried attempt")
			}
			releaseOnce.Do(func() { close(release) })
			awaitTask(t, m, row.ID, func(row *model.VideoGenTask) bool { return row.UpstreamID == "retried-upstream" })
			var count int64
			m.DB.Model(&model.VideoGenTask{}).Count(&count)
			if count != 1 {
				t.Fatal("concurrent retries created task records")
			}
		})
	}
}

func TestInvalidInPlaceRetryLeavesFailure(t *testing.T) {
	m := managerFixture(t)
	row := model.VideoGenTask{ID: uuid.NewString(), UserID: 1, ConfigID: 1, Source: "test", Operation: "generate", Prompt: "original prompt", IdempotencyKey: uuid.NewString(), Status: "failed", Error: "original failure", ArtifactStatus: "pending", LeaseUntil: unleasedAt}
	if err := m.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	_, err := m.RetryGeneration(context.Background(), 1, row.ID, configFor("https://upstream.example"), map[string]any{"extra_body": map[string]any{"model": "override"}}, "invalid-retry")
	if err == nil {
		t.Fatal("invalid retry accepted")
	}
	stored, err := m.Get(1, nil, row.ID, false)
	if err != nil || stored.Status != "failed" || stored.Error != row.Error || stored.Prompt != row.Prompt || stored.RetryCount != 0 || stored.Revision != row.Revision || stored.RetryRequests != "" {
		t.Fatal("validation failure changed the original task")
	}
}
