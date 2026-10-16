package videogen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/videoasset"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var ErrBusy = errors.New("video workers are busy; retry the same request_id shortly")
var ErrActiveTask = errors.New("a running video task cannot be deleted")
var unleasedAt = time.Unix(0, 0).UTC()

type Manager struct {
	DB           *gorm.DB
	Store        storage.Storage
	HTTPClient   *http.Client
	PollInterval time.Duration
	ctx          context.Context
	cancel       context.CancelFunc
	sem          chan struct{}
	start        sync.Once
	wg           sync.WaitGroup
	mu           sync.Mutex
	closed       bool
}

func NewManager(db *gorm.DB, store storage.Storage) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{DB: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), Store: store, PollInterval: 5 * time.Second, ctx: ctx, cancel: cancel, sem: make(chan struct{}, 4)}
}

func (m *Manager) Start(parent context.Context) {
	m.start.Do(func() {
		m.launch(func() {
			if err := m.reconcileRejectedSubmissions(); err != nil && m.ctx.Err() == nil {
				log.Printf("[videogen] reconcile queue rejections: %v", err)
			}
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			cleanup := time.NewTicker(time.Minute)
			defer cleanup.Stop()
			m.ProcessDue()
			m.Cleanup()
			for {
				select {
				case <-parent.Done():
					m.cancel()
					return
				case <-m.ctx.Done():
					return
				case <-ticker.C:
					m.ProcessDue()
				case <-cleanup.C:
					m.Cleanup()
				}
			}
		})
	})
}

func (m *Manager) launch(fn func()) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.wg.Add(1)
	go func() { defer m.wg.Done(); fn() }()
	return true
}

func (m *Manager) Close() { m.mu.Lock(); m.closed = true; m.cancel(); m.mu.Unlock(); m.wg.Wait() }

func hash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) Submit(ctx context.Context, uid int64, cfg *model.VideoGenConfig, operation, source string, args map[string]any) (*model.VideoGenTask, error) {
	return m.submitTask(ctx, uid, cfg, operation, source, args, nil)
}

type retrySubmission struct {
	task *model.VideoGenTask
	hash string
}

func retryRequests(t *model.VideoGenTask) (map[string]string, error) {
	requests := map[string]string{}
	if t.RetryRequests != "" && (json.Unmarshal([]byte(t.RetryRequests), &requests) != nil || requests == nil) {
		return nil, fmt.Errorf("invalid stored retry state")
	}
	return requests, nil
}

func (m *Manager) submitTask(ctx context.Context, uid int64, cfg *model.VideoGenConfig, operation, source string, args map[string]any, retry *retrySubmission) (*model.VideoGenTask, error) {
	if m.Store == nil {
		return nil, fmt.Errorf("video storage not initialized")
	}
	if uid == 0 || cfg.UserID != uid {
		return nil, fmt.Errorf("video configuration is not accessible to this user")
	}
	if err := ValidateParams(args, false); err != nil {
		return nil, err
	}
	if operation != "generate" && operation != "image" {
		return nil, fmt.Errorf("unknown video operation")
	}
	if cfg.Operation != "both" && cfg.Operation != operation {
		return nil, fmt.Errorf("video operation is disabled")
	}
	prompt, _ := args["prompt"].(string)
	if strings.TrimSpace(prompt) == "" || len(prompt) > 65536 {
		return nil, fmt.Errorf("prompt must contain 1 to 65536 bytes")
	}
	requestID, _ := args["request_id"].(string)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	idempotency := hash([]any{uid, cfg.ID, requestID})
	input := map[string]any{}
	for k, v := range args {
		if k != "wait_seconds" && k != "request_id" {
			input[k] = v
		}
	}
	digestInput := []any{operation, input}
	if cfg.ID == 0 {
		digestInput = append(digestInput, cfg.EndpointURL, cfg.ModelName, cfg.DefaultParams, cfg.APIKey, cfg.ImageRequestFormat)
	}
	digest := hash(digestInput)
	var existing model.VideoGenTask
	var requests map[string]string
	if retry != nil {
		var err error
		requests, err = retryRequests(retry.task)
		if err != nil {
			return nil, err
		}
		if previous, ok := requests[requestID]; ok {
			if previous != retry.hash {
				return nil, fmt.Errorf("retry request_id was already used with different input")
			}
			return retry.task, nil
		}
		if retry.task.Status != "failed" {
			return nil, fmt.Errorf("only failed generation tasks can be retried")
		}
		if retry.task.StorageKey != "" {
			return nil, fmt.Errorf("cannot retry generation while saved media exists")
		}
	} else {
		if err := m.DB.Where("idempotency_key = ?", idempotency).First(&existing).Error; err == nil {
			if existing.RequestHash != digest {
				return nil, fmt.Errorf("request_id was already used with different input")
			}
			return &existing, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	var defaults map[string]any
	if json.Unmarshal([]byte(cfg.DefaultParams), &defaults) != nil {
		return nil, fmt.Errorf("invalid default parameters")
	}
	p, _ := imagegen.MergeParams(defaults, args)
	if err := ValidateParams(p, false); err != nil {
		return nil, err
	}
	var img *imagegen.Image
	if operation == "image" {
		ref, _ := p["image"].(string)
		if ref == "" {
			return nil, fmt.Errorf("image is required")
		}
		refCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.RequestTimeoutSeconds)*time.Second)
		i, err := imagegen.LoadReference(refCtx, m.Store, uid, ref, imagegen.MaxImageBytes())
		cancel()
		if err != nil {
			return nil, err
		}
		img = &i
		if cfg.ImageRequestFormat == "json" {
			if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
				img.URL = ref
			} else {
				if model.GetOptionString("ServerAddress") == "" {
					return nil, fmt.Errorf("Set ServerAddress to an upstream-accessible URL before using local/base64 images in JSON mode")
				}
				asset, err := imageasset.Save(ctx, m.Store, uid, bytes.NewReader(img.Bytes), imagegen.MaxImageBytes())
				if err != nil {
					return nil, err
				}
				img.URL = asset.URL
			}
		}
	} else if _, ok := p["image"]; ok {
		return nil, fmt.Errorf("use generate_video_from_image for reference images")
	}
	key, err := common.Decrypt(cfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("cannot decrypt video API key")
	}
	conn := Connection{EndpointURL: cfg.EndpointURL, Model: cfg.ModelName, APIKey: key}
	client := &Client{Connection: conn, HTTPClient: m.HTTPClient}
	req, err := client.BuildSubmit(ctx, p, img, cfg.ImageRequestFormat)
	if err != nil {
		return nil, err
	}
	connectionJSON, _ := json.Marshal(conn)
	snapshot, err := common.Encrypt(string(connectionJSON))
	if err != nil {
		return nil, err
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	requestParams, err := common.Encrypt(string(inputJSON))
	if err != nil {
		return nil, err
	}
	select {
	case m.sem <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	now := time.Now()
	owner := uuid.NewString()
	t := &model.VideoGenTask{ID: uuid.NewString(), UserID: uid, ConfigID: cfg.ID, ConfigName: cfg.Name, Source: source, Operation: operation, Model: cfg.ModelName, Prompt: prompt, IdempotencyKey: idempotency, RequestHash: digest, RequestParams: requestParams, Connection: snapshot, Status: "submitting", ArtifactStatus: "pending", RequestTimeoutSeconds: cfg.RequestTimeoutSeconds, TaskTimeoutSeconds: cfg.TaskTimeoutSeconds, TrackingDeadline: now.Add(time.Duration(cfg.TaskTimeoutSeconds) * time.Second), NextPollAt: now, LeaseOwner: owner, LeaseUntil: now.Add(time.Duration(cfg.RequestTimeoutSeconds+15) * time.Second)}
	if retry != nil {
		// Replace the failed attempt in place while preserving the task identity
		// and the original submission's idempotency mapping.
		previous := retry.task
		t.ID, t.IdempotencyKey, t.RequestHash = previous.ID, previous.IdempotencyKey, previous.RequestHash
		t.Source, t.RetryOf, t.CreatedAt = previous.Source, previous.RetryOf, previous.CreatedAt
		t.RetryCount, t.Revision, t.UpdatedAt = previous.RetryCount+1, previous.Revision+1, now
		requests[requestID] = retry.hash
		encoded, _ := json.Marshal(requests)
		t.RetryRequests = string(encoded)
		values := map[string]any{
			"config_id": t.ConfigID, "config_name": t.ConfigName, "model": t.Model, "prompt": t.Prompt,
			"request_params": t.RequestParams, "retry_requests": t.RetryRequests, "retry_count": t.RetryCount,
			"connection": t.Connection, "upstream_id": "", "status": t.Status, "progress": 0,
			"artifact_status": "pending", "tracking_paused": false, "error": "", "last_error": "", "failures": 0,
			"request_timeout_seconds": t.RequestTimeoutSeconds, "task_timeout_seconds": t.TaskTimeoutSeconds,
			"tracking_deadline": t.TrackingDeadline, "next_poll_at": t.NextPollAt, "stopped_at": nil,
			"lease_owner": t.LeaseOwner, "lease_until": t.LeaseUntil, "revision": t.Revision, "updated_at": now,
			"short_id": nil, "mime": "", "size": 0, "backend": "", "expires_at": nil,
		}
		res := m.DB.Model(&model.VideoGenTask{}).Where("id = ? AND user_id = ? AND status = ? AND revision = ? AND lease_owner = ? AND lease_until <= ?", previous.ID, uid, "failed", previous.Revision, "", now).Updates(values)
		if res.Error != nil || res.RowsAffected != 1 {
			<-m.sem
			if res.Error != nil {
				return nil, res.Error
			}
			current, err := m.Get(uid, nil, previous.ID, false)
			if err != nil {
				return nil, err
			}
			accepted, err := retryRequests(current)
			if err != nil {
				return nil, err
			}
			if value, ok := accepted[requestID]; ok {
				if value != retry.hash {
					return nil, fmt.Errorf("retry request_id was already used with different input")
				}
				return current, nil
			}
			return nil, ErrBusy
		}
	} else if err := m.DB.Create(t).Error; err != nil {
		<-m.sem
		if e := m.DB.Where("idempotency_key = ?", idempotency).First(&existing).Error; e == nil {
			if existing.RequestHash != digest {
				return nil, fmt.Errorf("request_id was already used with different input")
			}
			return &existing, nil
		}
		return nil, err
	}
	if !m.launch(func() { defer func() { <-m.sem }(); m.submit(t, client, req) }) {
		<-m.sem
		m.finish(t, map[string]any{"status": "failed", "error": "video manager is shutting down", "connection": "", "stopped_at": time.Now()})
	}
	return t, nil
}

func (m *Manager) submit(t *model.VideoGenTask, client *Client, req *http.Request) {
	ctx, cancel := context.WithTimeout(m.ctx, time.Duration(t.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	v, err := client.Submit(req.Clone(ctx))
	if err != nil {
		status := "submission_unknown"
		var he *HTTPError
		if errors.As(err, &he) && he.submissionRejected() {
			status = "failed"
		}
		m.finish(t, map[string]any{"status": status, "error": err.Error(), "connection": "", "stopped_at": time.Now()})
		return
	}
	values := m.videoValues(t, client, v)
	values["upstream_id"] = v.ID
	m.finish(t, values)
}

func (m *Manager) videoValues(t *model.VideoGenTask, client *Client, v *Video) map[string]any {
	status := NormalizeStatus(v.Status)
	values := map[string]any{"status": status, "progress": v.ProgressValue(), "last_error": "", "failures": 0, "next_poll_at": time.Now().Add(m.PollInterval)}
	if status == "completed" {
		values["progress"] = 100
		values["next_poll_at"] = time.Now()
		if len(v.Error) > 0 && string(v.Error) != "null" {
			values["last_error"] = client.redact(responseError(v.Error))
		}
	}
	if status == "failed" {
		values["error"] = client.redact(responseError(v.Error))
		if string(v.Error) == "null" || len(v.Error) == 0 {
			values["error"] = "video generation failed"
		}
		values["connection"] = ""
		values["stopped_at"] = time.Now()
	}
	if status == "unknown" {
		values["last_error"] = "unrecognized upstream status: " + client.redact(v.Status)
	}
	return values
}

// Every worker write is conditional on its lease token. An expired worker can
// never overwrite state committed by the next lease holder.
func (m *Manager) finish(t *model.VideoGenTask, values map[string]any) {
	values["lease_owner"] = ""
	values["revision"] = gorm.Expr("revision + 1")
	values["lease_until"] = unleasedAt
	if err := m.DB.Model(&model.VideoGenTask{}).Where("id = ? AND lease_owner = ?", t.ID, t.LeaseOwner).Updates(values).Error; err != nil {
		log.Printf("[videogen] task=%s update failed: %v", t.ID, err)
	}
}

func (m *Manager) claim(t *model.VideoGenTask, duration time.Duration) bool {
	t.LeaseOwner = uuid.NewString()
	res := m.DB.Model(&model.VideoGenTask{}).Where("id = ? AND revision = ? AND updated_at = ? AND lease_until <= ?", t.ID, t.Revision, t.UpdatedAt, time.Now()).Updates(map[string]any{"lease_owner": t.LeaseOwner, "lease_until": time.Now().Add(duration), "revision": gorm.Expr("revision + 1")})
	return res.Error == nil && res.RowsAffected == 1
}

// Older versions classified explicit 503 queue rejections as unknown. Repair
// only rows with a complete structured rejection and no upstream task ID.
// Conditional updates protect tasks claimed or changed by another instance.
func (m *Manager) reconcileRejectedSubmissions() error {
	const prefix = "video API HTTP 503: "
	cursor := ""
	for {
		var rows []model.VideoGenTask
		err := m.DB.WithContext(m.ctx).Where("status = ? AND upstream_id = ? AND lease_owner = ? AND error LIKE ? AND id > ?", "submission_unknown", "", "", prefix+"%", cursor).Order("id").Limit(100).Find(&rows).Error
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for i := range rows {
			t := &rows[i]
			cursor = t.ID
			if !strings.HasPrefix(t.Error, prefix) || responseErrorCode([]byte(strings.TrimPrefix(t.Error, prefix))) != videoQueueFullCode {
				continue
			}
			values := map[string]any{"status": "failed", "connection": "", "lease_until": unleasedAt, "revision": gorm.Expr("revision + 1")}
			if t.StoppedAt == nil {
				values["stopped_at"] = time.Now()
			}
			if err := m.DB.WithContext(m.ctx).Model(&model.VideoGenTask{}).Where("id = ? AND status = ? AND upstream_id = ? AND lease_owner = ? AND revision = ? AND error = ?", t.ID, "submission_unknown", "", "", t.Revision, t.Error).Updates(values).Error; err != nil {
				return err
			}
		}
	}
}

func (m *Manager) ProcessDue() {
	if m.ctx.Err() != nil {
		return
	}
	now := time.Now()
	m.DB.Model(&model.VideoGenTask{}).Where("status = ? AND lease_until <= ?", "submitting", now).Updates(map[string]any{"status": "submission_unknown", "error": "Submission outcome is unknown; do not automatically resubmit this generation.", "connection": "", "stopped_at": now, "lease_owner": "", "lease_until": unleasedAt, "revision": gorm.Expr("revision + 1")})
	var rows []model.VideoGenTask
	err := m.DB.Where("upstream_id <> ? AND tracking_paused = ? AND next_poll_at <= ? AND lease_until <= ?", "", false, now, now).Where("status IN ? OR (status = ? AND artifact_status IN ?)", []string{"queued", "in_progress", "unknown"}, "completed", []string{"pending", "saving"}).Order("next_poll_at").Limit(100).Find(&rows).Error
	if err != nil {
		log.Printf("[videogen] list due tasks: %v", err)
		return
	}
	for i := range rows {
		select {
		case m.sem <- struct{}{}:
		default:
			return
		}
		t := rows[i]
		if !m.claim(&t, time.Duration(t.RequestTimeoutSeconds+15)*time.Second) {
			<-m.sem
			continue
		}
		if !m.launch(func() { defer func() { <-m.sem }(); m.process(&t) }) {
			<-m.sem
			m.finish(&t, map[string]any{})
		}
	}
}

func (m *Manager) process(t *model.VideoGenTask) {
	if t.Status != "completed" && time.Now().After(t.TrackingDeadline) {
		m.finish(t, map[string]any{"tracking_paused": true, "last_error": "Automatic tracking timed out; query this task to resume tracking.", "stopped_at": time.Now()})
		return
	}
	plain, err := common.Decrypt(t.Connection)
	var conn Connection
	if err != nil || json.Unmarshal([]byte(plain), &conn) != nil || conn.APIKey == "" {
		m.finish(t, map[string]any{"tracking_paused": true, "last_error": "Task credentials are unavailable; check CRYPTO_SECRET.", "stopped_at": time.Now()})
		return
	}
	client := &Client{Connection: conn, HTTPClient: m.HTTPClient}
	if t.Status == "completed" {
		m.save(t, client)
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, time.Duration(t.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	v, err := client.Query(ctx, t.UpstreamID)
	if err != nil {
		delay := min(time.Minute, m.PollInterval*time.Duration(1<<min(t.Failures+1, 6)))
		var he *HTTPError
		if errors.As(err, &he) && he.RetryAfter > delay {
			delay = min(time.Hour, he.RetryAfter)
		}
		m.finish(t, map[string]any{"last_error": client.redact(err.Error()), "failures": t.Failures + 1, "next_poll_at": time.Now().Add(delay)})
		return
	}
	m.finish(t, m.videoValues(t, client, v))
}

func (m *Manager) save(t *model.VideoGenTask, client *Client) {
	err := m.DB.Transaction(func(tx *gorm.DB) error {
		// Serialize admission against the user row on server databases; SQLite's
		// write transaction serializes the final reservation update.
		maxVideos := model.GetOptionInt64("MaxVideosPerUser")
		if maxVideos > 0 {
			q := tx.Select("id").Where("id = ?", t.UserID)
			if tx.Dialector.Name() != "sqlite" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			var u model.User
			if err := q.First(&u).Error; err != nil {
				return err
			}
			var n int64
			if err := tx.Model(&model.VideoGenTask{}).Where("user_id = ? AND id <> ?", t.UserID, t.ID).Where("artifact_status = ? OR (artifact_status = ? AND expires_at > ?)", "saving", "ready", time.Now()).Count(&n).Error; err != nil {
				return err
			}
			if n >= maxVideos {
				return fmt.Errorf("video storage quota exceeded; delete old videos and retry saving")
			}
		}
		res := tx.Model(&model.VideoGenTask{}).Where("id = ? AND lease_owner = ?", t.ID, t.LeaseOwner).Updates(map[string]any{"artifact_status": "saving", "lease_until": time.Now().Add(6 * time.Minute), "revision": gorm.Expr("revision + 1")})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return fmt.Errorf("video task lease lost")
		}
		return nil
	})
	if err != nil {
		m.finish(t, map[string]any{"artifact_status": "failed", "error": err.Error(), "stopped_at": time.Now()})
		return
	}
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()
	resp, err := client.Content(ctx, t.UpstreamID, videoasset.MaxBytes())
	if err != nil {
		m.finish(t, map[string]any{"artifact_status": "failed", "error": client.redact(err.Error()), "stopped_at": time.Now()})
		return
	}
	defer resp.Body.Close()
	asset, err := videoasset.Save(ctx, m.Store, t.UserID, t.ID, resp.Body)
	if err != nil {
		m.finish(t, map[string]any{"artifact_status": "failed", "error": client.redact(err.Error()), "stopped_at": time.Now(), "storage_key": fmt.Sprintf("video/%d/%s", t.UserID, t.ID)})
		return
	}
	sid, err := model.NewShortID()
	if err != nil {
		m.finish(t, map[string]any{"artifact_status": "failed", "error": "Could not create video link", "storage_key": asset.Key, "stopped_at": time.Now()})
		return
	}
	m.finish(t, map[string]any{"artifact_status": "ready", "storage_key": asset.Key, "backend": m.Store.Backend(), "short_id": sid, "mime": asset.MIME, "size": asset.Size, "expires_at": asset.ExpiresAt, "error": "", "connection": "", "stopped_at": time.Now()})
}

func (m *Manager) Get(uid int64, configID *int64, id string, resume bool) (*model.VideoGenTask, error) {
	var t model.VideoGenTask
	q := m.DB.Where("id = ? AND user_id = ?", id, uid)
	if configID != nil {
		q = q.Where("config_id = ?", *configID)
	}
	if err := q.First(&t).Error; err != nil {
		return nil, fmt.Errorf("video task not found")
	}
	if resume && t.TrackingPaused && t.UpstreamID != "" && t.Connection != "" {
		err := m.DB.Model(&model.VideoGenTask{}).Where("id = ? AND tracking_paused = ? AND lease_until <= ?", t.ID, true, time.Now()).Updates(map[string]any{"tracking_paused": false, "tracking_deadline": time.Now().Add(time.Duration(t.TaskTimeoutSeconds) * time.Second), "next_poll_at": time.Now(), "stopped_at": nil, "revision": gorm.Expr("revision + 1")}).Error
		if err != nil {
			return nil, err
		}
		_ = q.First(&t).Error
	}
	return &t, nil
}

func (m *Manager) RetryDownload(uid int64, configID *int64, id string) (*model.VideoGenTask, error) {
	t, err := m.Get(uid, configID, id, false)
	if err != nil {
		return nil, err
	}
	if t.Status != "completed" || t.ArtifactStatus != "failed" {
		return nil, fmt.Errorf("only failed saves of completed videos can be retried")
	}
	res := m.DB.Model(&model.VideoGenTask{}).Where("id = ? AND artifact_status = ? AND lease_until <= ?", id, "failed", time.Now()).Updates(map[string]any{"artifact_status": "pending", "tracking_paused": false, "error": "", "next_poll_at": time.Now(), "stopped_at": nil, "revision": gorm.Expr("revision + 1")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, ErrBusy
	}
	return m.Get(uid, configID, id, false)
}

// RetryInput never returns a connection or credential snapshot. Older rows
// retain only their prompt; callers can supply the missing parameters/image.
func RetryInput(t *model.VideoGenTask) (map[string]any, bool, error) {
	if t.Status != "failed" {
		return nil, false, fmt.Errorf("only failed generation tasks can be retried")
	}
	return storedTaskInput(t)
}

func storedTaskInput(t *model.VideoGenTask) (map[string]any, bool, error) {
	if t.RequestParams == "" {
		return map[string]any{"prompt": t.Prompt}, false, nil
	}
	plain, err := common.Decrypt(t.RequestParams)
	if err != nil {
		return nil, false, fmt.Errorf("cannot decrypt retry parameters")
	}
	var args map[string]any
	if json.Unmarshal([]byte(plain), &args) != nil || args == nil {
		return nil, false, fmt.Errorf("invalid stored retry parameters")
	}
	return args, true, nil
}

func (m *Manager) RetryGeneration(ctx context.Context, uid int64, id string, cfg *model.VideoGenConfig, args map[string]any, requestID string) (*model.VideoGenTask, error) {
	t, err := m.Get(uid, nil, id, false)
	if err != nil {
		return nil, err
	}
	if requestID == "" || len(requestID) > 128 {
		return nil, fmt.Errorf("retry request_id must contain 1 to 128 bytes")
	}
	// Hash the caller's input before applying fallbacks, so omitted arguments
	// remain idempotent even after a later retry edits the stored prompt.
	retry := &retrySubmission{task: t, hash: hash([]any{cfg.ID, t.Operation, args})}
	if args == nil {
		args, _, err = storedTaskInput(t)
		if err != nil {
			return nil, err
		}
	} else {
		copied := make(map[string]any, len(args))
		for key, value := range args {
			copied[key] = value
		}
		args = copied
	}
	if _, ok := args["prompt"]; !ok {
		args["prompt"] = t.Prompt
	}
	// Namespace each explicit retry by its task and caller-generated token.
	// Accepted tokens persist on the same row across retries and restarts.
	args["request_id"] = hash([]any{"video-retry", id, requestID})
	delete(args, "wait_seconds")
	return m.submitTask(ctx, uid, cfg, t.Operation, t.Source, args, retry)
}

func done(t *model.VideoGenTask) bool {
	return t.Status == "failed" || t.Status == "submission_unknown" || t.TrackingPaused || (t.Status == "completed" && (t.ArtifactStatus == "ready" || t.ArtifactStatus == "failed" || t.ArtifactStatus == "expired"))
}

func (m *Manager) Wait(ctx context.Context, uid int64, id string, seconds int) (*model.VideoGenTask, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	t, err := m.Get(uid, nil, id, false)
	if err != nil || seconds == 0 || done(t) {
		return t, err
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return t, nil
		case <-ticker.C:
			t, err = m.Get(uid, nil, id, false)
			if err != nil || done(t) {
				return t, err
			}
		}
	}
}

func TaskView(t *model.VideoGenTask) map[string]any {
	data, _ := json.Marshal(t)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	out["protocol"] = "openai_video"
	out["poll_after_seconds"] = 5
	out["videos"] = []any{}
	if t.ArtifactStatus == "ready" && t.ExpiresAt != nil && t.ExpiresAt.After(time.Now()) && t.ShortID != nil {
		out["videos"] = []any{map[string]any{"url": videoasset.URL(*t.ShortID), "mime": t.MIME, "size": t.Size, "expires_at": t.ExpiresAt}}
	} else if t.ArtifactStatus == "ready" {
		out["artifact_status"] = "expired"
	}
	return out
}

func MCPResult(t *model.VideoGenTask) (json.RawMessage, error) {
	view := TaskView(t)
	b, _ := json.Marshal(view)
	content := []map[string]any{{"type": "text", "text": string(b)}}
	if entries, ok := view["videos"].([]any); ok && len(entries) > 0 {
		v := entries[0].(map[string]any)
		content = append(content, map[string]any{"type": "resource_link", "uri": v["url"], "name": "generated-video-" + t.ID, "mimeType": t.MIME, "size": t.Size})
	}
	return json.Marshal(map[string]any{"content": content, "structuredContent": view, "isError": t.Status == "failed" || t.Status == "submission_unknown" || t.ArtifactStatus == "failed"})
}

func (m *Manager) Delete(uid int64, id string) error {
	t, err := m.Get(uid, nil, id, false)
	if err != nil {
		return err
	}
	if !done(t) {
		return ErrActiveTask
	}
	if !m.claim(t, time.Minute) {
		return ErrBusy
	}
	if t.StorageKey != "" {
		if err := m.Store.Delete(m.ctx, t.StorageKey); err != nil {
			m.finish(t, map[string]any{})
			return err
		}
	}
	res := m.DB.Where("id = ? AND lease_owner = ?", id, t.LeaseOwner).Delete(&model.VideoGenTask{})
	return res.Error
}

func (m *Manager) Cleanup() {
	videoasset.CleanupTemps()
	now := time.Now()
	var rows []model.VideoGenTask
	if m.DB.Where("storage_key <> ? AND artifact_status IN ? AND expires_at <= ? AND lease_until <= ?", "", []string{"ready", "expired"}, now, now).Limit(100).Find(&rows).Error == nil {
		for i := range rows {
			t := &rows[i]
			if !m.claim(t, time.Minute) {
				continue
			}
			if err := m.Store.Delete(m.ctx, t.StorageKey); err != nil {
				m.finish(t, map[string]any{})
				continue
			}
			m.finish(t, map[string]any{"artifact_status": "expired", "storage_key": ""})
		}
	}
	historyDays := model.GetOptionInt("VideoGenHistoryDays")
	if historyDays <= 0 {
		historyDays = 30
	}
	rows = nil
	if m.DB.Where("stopped_at <= ? AND lease_until <= ?", now.Add(-time.Duration(historyDays)*24*time.Hour), now).Where("artifact_status <> ? OR expires_at <= ?", "ready", now).Limit(100).Find(&rows).Error == nil {
		for i := range rows {
			_ = m.Delete(rows[i].UserID, rows[i].ID)
		}
	}
}
