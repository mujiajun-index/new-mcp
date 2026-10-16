package videogen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

type submissionTransportFunc func(*http.Request) (*http.Response, error)

func (f submissionTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSubmissionTransportTimeoutRemainsUnknown(t *testing.T) {
	m := managerFixture(t)
	var calls atomic.Int32
	m.HTTPClient = &http.Client{Transport: submissionTransportFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, context.DeadlineExceeded
	})}
	job, err := m.Submit(context.Background(), 1, configFor("https://upstream.example"), "generate", "test", map[string]any{"prompt": "motion"})
	if err != nil {
		t.Fatal(err)
	}
	row := awaitTask(t, m, job.ID, func(row *model.VideoGenTask) bool { return row.Status == "submission_unknown" })
	if row.UpstreamID != "" || calls.Load() != 1 {
		t.Fatal("timeout caused a second submission")
	}
	if _, err := m.RetryGeneration(context.Background(), 1, row.ID, configFor("https://upstream.example"), nil, "unsafe-retry"); err == nil {
		t.Fatal("a transport timeout became eligible for generation retry")
	}
}

func TestSubmissionHTTPErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   string
		want   string
	}{
		{"queue full", 503, `{"code":"video_queue_full","message":"video queue is full, please retry later","data":null}`, videoQueueFullCode, "failed"},
		{"nested queue full", 503, `{"error":{"code":"video_queue_full","message":"queue full"}}`, videoQueueFullCode, "failed"},
		{"wrapped queue full", 503, `{"error":{"code":"fail_to_submit_task","message":"{\"code\":\"video_queue_full\",\"message\":\"video queue is full, please retry later\",\"data\":null}"}}`, "fail_to_submit_task", "failed"},
		{"task error wrapper", 503, `{"code":"fail_to_fetch_task","message":"{\"code\":\"video_queue_full\",\"message\":\"video queue is full, please retry later\",\"data\":null}"}`, "fail_to_fetch_task", "failed"},
		{"numeric wrapper code", 503, `{"code":503,"message":"{\"code\":\"video_queue_full\",\"message\":\"video queue is full\",\"data\":null}"}`, "", "failed"},
		{"string error wrapper", 503, `{"error":"{\"code\":\"video_queue_full\",\"message\":\"video queue is full\",\"data\":null}"}`, "", "failed"},
		{"wrapped text only", 503, `{"error":{"code":"fail_to_submit_task","message":"video_queue_full: video queue is full"}}`, "fail_to_submit_task", "submission_unknown"},
		{"wrapped truncated rejection", 503, `{"error":{"code":"fail_to_submit_task","message":"{\"code\":\"video_queue_full\","}}`, "fail_to_submit_task", "submission_unknown"},
		{"invalid request", 400, `{"code":"invalid_request","message":"mode is required"}`, "invalid_request", "failed"},
		{"rate limit", 429, `{"error":{"code":"rate_limit_exceeded","message":"try later"}}`, "rate_limit_exceeded", "failed"},
		{"generic unavailable", 503, `{"code":"service_unavailable","message":"try later"}`, "service_unavailable", "submission_unknown"},
		{"queue text only", 503, `{"message":"video_queue_full: video queue is full"}`, "", "submission_unknown"},
		{"wrong status", 500, `{"code":"video_queue_full","message":"internal error"}`, videoQueueFullCode, "submission_unknown"},
		{"proxy timeout", 504, `gateway timeout`, "", "submission_unknown"},
		{"empty response", 503, "", "", "submission_unknown"},
		{"truncated rejection", 503, `{"code":"video_queue_full",`, "", "submission_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := managerFixture(t)
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				posts.Add(1)
				w.Header().Set("Retry-After", "2")
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := &Client{Connection: Connection{EndpointURL: server.URL}}
			req, err := client.BuildSubmit(context.Background(), map[string]any{"prompt": "motion"}, nil, "json")
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Submit(req)
			var he *HTTPError
			if !errors.As(err, &he) || he.Code != tc.code || he.RetryAfter != 2*time.Second {
				t.Fatalf("unexpected parsed error: %+v", err)
			}
			posts.Store(0)
			job, err := m.Submit(context.Background(), 1, configFor(server.URL), "generate", "test", map[string]any{"prompt": "motion", "request_id": "rejected"})
			if err != nil {
				t.Fatal(err)
			}
			row := awaitTask(t, m, job.ID, func(row *model.VideoGenTask) bool { return row.Status == tc.want })
			if row.UpstreamID != "" || row.Connection != "" || row.StoppedAt == nil {
				t.Fatal("submission failure did not safely stop and clear credentials")
			}
			m.ProcessDue()
			if posts.Load() != 1 {
				t.Fatal("submission error caused automatic resubmission")
			}
		})
	}
}

func TestQueueFullAllowsExplicitRetry(t *testing.T) {
	m := managerFixture(t)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			io.WriteString(w, `{"id":"accepted-retry","status":"queued"}`)
			return
		}
		if posts.Add(1) == 1 {
			w.WriteHeader(503)
			io.WriteString(w, `{"code":"video_queue_full","message":"video queue is full, please retry later","data":null}`)
			return
		}
		io.WriteString(w, `{"id":"accepted-retry","status":"queued"}`)
	}))
	defer server.Close()
	cfg := configFor(server.URL)
	args := map[string]any{"prompt": "motion", "request_id": "original", "extra_body": map[string]any{"mode": "text"}}
	parent, err := m.Submit(context.Background(), 1, cfg, "generate", "test", args)
	if err != nil {
		t.Fatal(err)
	}
	awaitTask(t, m, parent.ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" })
	again, err := m.Submit(context.Background(), 1, cfg, "generate", "test", args)
	if err != nil || again.ID != parent.ID || posts.Load() != 1 {
		t.Fatal("retransmitting the original request must not generate another task")
	}
	child, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "manual-retry")
	if err != nil {
		t.Fatal(err)
	}
	child = awaitTask(t, m, child.ID, func(row *model.VideoGenTask) bool { return row.UpstreamID == "accepted-retry" })
	if child.ID != parent.ID || child.RetryCount != 1 || child.Status != "queued" || posts.Load() != 2 {
		t.Fatal("explicit retry did not submit exactly once on the original row")
	}
	stored, err := m.Get(1, nil, parent.ID, false)
	if err != nil || stored.Status != "queued" || stored.Error != "" {
		t.Fatal("retry did not update the original record")
	}
}

func TestQueueFullDuringQueryDoesNotFailAcceptedTask(t *testing.T) {
	m := managerFixture(t)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			io.WriteString(w, `{"id":"already-accepted","status":"queued"}`)
			return
		}
		w.WriteHeader(503)
		io.WriteString(w, `{"code":"video_queue_full","message":"try later"}`)
	}))
	defer server.Close()
	job, err := m.Submit(context.Background(), 1, configFor(server.URL), "generate", "test", map[string]any{"prompt": "motion"})
	if err != nil {
		t.Fatal(err)
	}
	row := awaitTask(t, m, job.ID, func(row *model.VideoGenTask) bool { return row.Failures > 0 })
	if row.Status != "queued" || row.UpstreamID != "already-accepted" || row.Connection == "" || posts.Load() != 1 {
		t.Fatal("a temporary query failure changed or resubmitted the accepted task")
	}
}

func TestQueueFullRetryRemainsFailed(t *testing.T) {
	m := managerFixture(t)
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.WriteHeader(503)
		io.WriteString(w, `{"code":"fail_to_fetch_task","message":"{\"code\":\"video_queue_full\",\"message\":\"video queue is full, please retry later\",\"data\":null}"}`)
	}))
	defer server.Close()
	cfg := configFor(server.URL)
	parent, err := m.Submit(context.Background(), 1, cfg, "generate", "test", map[string]any{"prompt": "motion", "request_id": "queue-full-original"})
	if err != nil {
		t.Fatal(err)
	}
	parent = awaitTask(t, m, parent.ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" })
	child, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-once")
	if err != nil {
		t.Fatal(err)
	}
	child = awaitTask(t, m, child.ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" })
	if child.ID != parent.ID || child.RetryOf != "" || child.Source != parent.Source || child.RetryCount != 1 || posts.Load() != 2 {
		t.Fatal("manual retry must reuse the original task")
	}
	if TaskView(child)["status"] != "failed" || !strings.Contains(child.Error, videoQueueFullCode) {
		t.Fatal("the retry API would show an ambiguous status instead of queue rejection")
	}
	if _, _, err := RetryInput(child); err != nil {
		t.Fatal("queue-full retry is not eligible for another explicit retry")
	}
	repeated, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-once")
	if err != nil || repeated.ID != child.ID || posts.Load() != 2 {
		t.Fatal("retransmitting the same retry action created another task")
	}
	if _, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-again"); err != nil {
		t.Fatal(err)
	}
	latest := awaitTask(t, m, parent.ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" && row.RetryCount == 2 })
	if _, err := m.RetryGeneration(context.Background(), 1, parent.ID, cfg, nil, "retry-once"); err != nil || posts.Load() != 3 {
		t.Fatal("an earlier retry token resubmitted after a later attempt")
	}
	var count int64
	m.DB.Model(&model.VideoGenTask{}).Count(&count)
	if count != 1 || latest.ID != parent.ID {
		t.Fatal("repeated retries grew task history")
	}
}

func TestRecoverHistoricalQueueRejections(t *testing.T) {
	m := managerFixture(t)
	prefix := "video API HTTP 503: "
	queueError := prefix + `{"code":"video_queue_full","message":"video queue is full (request id: 2026101306172161127757pXOiR4rA)","data":null}`
	stopped := time.Now().Add(-time.Hour)
	var rows []model.VideoGenTask
	// More than one page checks that unmatched rows don't stop the scan.
	for i := 0; i < 103; i++ {
		row := model.VideoGenTask{ID: fmt.Sprintf("%036d", i), UserID: 1, ConfigID: 1, Operation: "generate", Prompt: "motion", IdempotencyKey: uuid.NewString(), Status: "submission_unknown", Error: queueError, ArtifactStatus: "pending", Connection: "private", LeaseUntil: unleasedAt, StoppedAt: &stopped, Revision: 2}
		rows = append(rows, row)
	}
	input, err := common.Encrypt(`{"prompt":"motion","extra_body":{"mode":"text"}}`)
	if err != nil {
		t.Fatal(err)
	}
	rows[102].RequestParams = input
	rows[102].Source = "retry"
	rows[102].RetryOf = rows[101].ID
	rows[0].Error = prefix + `{"message":"video_queue_full"}`
	rows[1].Error = prefix + `{"code":"video_queue_full",`
	rows[2].UpstreamID = "existing-task"
	rows[3].LeaseOwner = "another-worker"
	rows[4].Error = "video API HTTP 504: " + `{"code":"video_queue_full"}`
	rows[5].Status = "queued"
	rows[6].StoppedAt = nil
	if err := m.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	// Verify the startup path performs the reconciliation, with no upstream calls.
	m.Start(context.Background())
	awaitTask(t, m, rows[102].ID, func(row *model.VideoGenTask) bool { return row.Status == "failed" })
	m.Close()
	for i := range rows {
		stored, err := m.Get(1, nil, rows[i].ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if i < 6 {
			if stored.Status != rows[i].Status || stored.Revision != rows[i].Revision {
				t.Fatalf("changed an ambiguous, accepted or leased task: %d", i)
			}
			continue
		}
		if stored.Status != "failed" || stored.Connection != "" || stored.Revision != 3 || stored.StoppedAt == nil || stored.Error != queueError || stored.RetryOf != rows[i].RetryOf || stored.Source != rows[i].Source {
			t.Fatalf("historical rejection not restored: %d", i)
		}
		if i != 6 && !stored.StoppedAt.Equal(stopped) {
			t.Fatal("reconciliation changed the retention deadline")
		}
		if _, _, err := RetryInput(stored); err != nil {
			t.Fatal("restored task cannot use the existing retry API")
		}
		if i == 102 {
			args, complete, err := RetryInput(stored)
			if err != nil || !complete || args["extra_body"].(map[string]any)["mode"] != "text" {
				t.Fatal("reconciliation lost the original retry parameters")
			}
		}
	}
	// A second startup must not change the row again.
	restarted := NewManager(m.DB, m.Store)
	defer restarted.Close()
	if err := restarted.reconcileRejectedSubmissions(); err != nil {
		t.Fatal(err)
	}
	stored, _ := m.Get(1, nil, rows[102].ID, false)
	if stored.Revision != 3 || strings.Contains(stored.Error, "private") {
		t.Fatal("reconciliation is not idempotent")
	}
}
