package imagegentask

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

const leaseDuration = 30 * time.Second
const maxResultBytes int64 = 200 << 20

type Manager struct {
	DB         *gorm.DB
	Store      storage.Storage
	HTTPClient *http.Client
	ctx        context.Context
	cancel     context.CancelFunc
	sem        chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	closed     bool
	start      sync.Once
}

func NewManager(db *gorm.DB, store storage.Storage) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{DB: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), Store: store, ctx: ctx, cancel: cancel, sem: make(chan struct{}, 4)}
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

func (m *Manager) Start(parent context.Context) {
	m.start.Do(func() {
		m.launch(func() {
			_ = recoverInterrupted(m.DB)
			if err := m.reconcileRejectedSubmissions(); err != nil {
				log.Printf("[imagegen] could not reconcile queue rejections")
			}
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			cleanup := time.NewTicker(time.Minute)
			defer cleanup.Stop()
			m.ProcessDue()
			for {
				select {
				case <-parent.Done():
					m.cancel()
					return
				case <-m.ctx.Done():
					return
				case <-tick.C:
					m.ProcessDue()
				case <-cleanup.C:
					m.cleanupResults()
					m.cleanupHistory()
				}
			}
		})
	})
}

func (m *Manager) Close() { m.mu.Lock(); m.closed = true; m.cancel(); m.mu.Unlock(); m.wg.Wait() }

func Active(t *model.ImageGenTask) bool {
	return t.Status == "queued" || t.Status == "running" || t.Status == "saving"
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func WaitSeconds(args map[string]any, fallback int) (int, error) {
	if value, ok := args["wait_seconds"]; ok {
		n, valid := value.(float64)
		if !valid || math.IsNaN(n) || n < 0 || n > 30 || n != math.Trunc(n) {
			return 0, fmt.Errorf("wait_seconds must be an integer from 0 to 30")
		}
		return int(n), nil
	}
	return fallback, nil
}

func (m *Manager) Submit(ctx context.Context, uid int64, cfg *model.ImageGenConfig, tool, source string, args map[string]any) (*model.ImageGenTask, error) {
	if uid == 0 || cfg.UserID != uid {
		return nil, fmt.Errorf("image configuration is not accessible to this user")
	}
	if m.Store == nil {
		return nil, fmt.Errorf("image storage not initialized")
	}
	if _, err := WaitSeconds(args, 10); err != nil {
		return nil, err
	}
	op := "generate"
	if tool == model.EditImageToolName {
		op = "edit"
	} else if tool != model.GenerateImageToolName {
		return nil, fmt.Errorf("unknown image tool")
	}
	requestID := uuid.NewString()
	if value, ok := args["request_id"]; ok {
		var valid bool
		requestID, valid = value.(string)
		if !valid || strings.TrimSpace(requestID) == "" || len(requestID) > 128 {
			return nil, fmt.Errorf("request_id must contain 1 to 128 bytes")
		}
	}
	params := map[string]any{}
	for k, v := range args {
		if k != "request_id" && k != "wait_seconds" {
			params[k] = v
		}
	}
	input := []any{op, params}
	if cfg.ID == 0 {
		input = append(input, cfg.Provider, cfg.EndpointURL, cfg.ModelName, cfg.DefaultParams, cfg.ApiKey, cfg.EditRequestFormat, cfg.TimeoutSeconds)
	}
	hash, key := digest(input), digest([]any{uid, cfg.ID, requestID})
	lookup := func() (*model.ImageGenTask, error) {
		var t model.ImageGenTask
		err := m.DB.Where("idempotency_key = ? AND user_id = ?", key, uid).First(&t).Error
		if err == nil && t.RequestHash != hash {
			return nil, fmt.Errorf("request_id was already used with different input")
		}
		return &t, err
	}
	if t, err := lookup(); err == nil {
		return t, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	prepareCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s, err := prepare(prepareCtx, m.Store, uid, cfg, op, params)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(s)
	if len(encoded) > 1<<20 {
		return nil, fmt.Errorf("image parameters exceed 1 MiB")
	}
	sealed, err := common.Encrypt(string(encoded))
	if err != nil {
		return nil, err
	}
	prompt, _ := s.Params["prompt"].(string)
	count := 0
	for _, ref := range s.References {
		if ref.Role == "reference" {
			count++
		}
	}
	if source != "test" {
		source = "mcp"
	}
	t := &model.ImageGenTask{ID: uuid.NewString(), UserID: uid, ConfigID: cfg.ID, ConfigName: redact(cfg.Name, cfg.ApiKey), Source: source, Operation: op, Provider: cfg.Provider, Model: cfg.ModelName, Prompt: redact(prompt, cfg.ApiKey), ReferenceCount: count, ReferenceImages: taskreference.Encode(s.References), Status: "queued", ArtifactStatus: "pending", IdempotencyKey: &key, RequestHash: hash, Snapshot: sealed}
	if err := m.DB.WithContext(ctx).Create(t).Error; err != nil {
		if previous, e := lookup(); e == nil {
			return previous, nil
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, e
		}
		return nil, err
	}
	// From this point cancellation affects the short wait, never the accepted work.
	m.ProcessDue()
	return t, nil
}

func (m *Manager) Get(uid int64, configID *int64, id string) (*model.ImageGenTask, error) {
	var t model.ImageGenTask
	q := m.DB.Where("id = ? AND user_id = ?", id, uid)
	if configID != nil {
		q = q.Where("config_id = ?", *configID)
	}
	err := q.First(&t).Error
	return &t, err
}

func (m *Manager) Wait(ctx context.Context, uid int64, id string, seconds int) (*model.ImageGenTask, error) {
	t, err := m.Get(uid, nil, id)
	if err != nil || seconds == 0 || !Active(t) {
		return t, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return t, nil
		case <-tick.C:
			t, err = m.Get(uid, nil, id)
			if err != nil || !Active(t) {
				return t, err
			}
		}
	}
}

func freeLease(q *gorm.DB, now time.Time) *gorm.DB {
	return q.Where("lease_until IS NULL OR lease_until <= ?", now)
}

func (m *Manager) ProcessDue() {
	if m.ctx.Err() != nil {
		return
	}
	now := time.Now()
	// A lost synchronous HTTP request has no upstream job ID to query. Never POST it again.
	freeLease(m.DB.Model(&model.ImageGenTask{}).Where("status = ? AND idempotency_key IS NOT NULL", "running"), now).Updates(map[string]any{"status": "submission_unknown", "error": "Image submission result is unknown after worker interruption; generation was not resubmitted", "snapshot": "", "lease_owner": "", "lease_until": nil, "finished_at": now})
	var rows []model.ImageGenTask
	if freeLease(m.DB.Where("status IN ? AND idempotency_key IS NOT NULL", []string{"queued", "saving"}), now).Order("created_at ASC, id ASC").Limit(8).Find(&rows).Error != nil {
		return
	}
	for _, row := range rows {
		select {
		case m.sem <- struct{}{}:
		default:
			return
		}
		owner := uuid.NewString()
		until := now.Add(leaseDuration)
		res := freeLease(m.DB.Model(&model.ImageGenTask{}).Where("id = ? AND status = ?", row.ID, row.Status), now).Updates(map[string]any{"lease_owner": owner, "lease_until": until})
		if res.Error != nil || res.RowsAffected != 1 {
			<-m.sem
			continue
		}
		row.LeaseOwner, row.LeaseUntil = owner, &until
		if !m.launch(func() { defer func() { <-m.sem }(); m.process(row) }) {
			<-m.sem
			m.owned(&row).Updates(map[string]any{"lease_owner": "", "lease_until": nil})
		}
	}
}

func (m *Manager) owned(t *model.ImageGenTask) *gorm.DB {
	return m.DB.Model(&model.ImageGenTask{}).Where("id = ? AND lease_owner = ?", t.ID, t.LeaseOwner)
}
func (m *Manager) update(t *model.ImageGenTask, values map[string]any) bool {
	r := m.owned(t).Updates(values)
	return r.Error == nil && r.RowsAffected == 1
}
func (m *Manager) terminal(t *model.ImageGenTask, status, message string) {
	now := time.Now()
	m.update(t, map[string]any{"status": status, "artifact_status": "failed", "error": message, "snapshot": "", "lease_owner": "", "lease_until": nil, "finished_at": now, "duration_ms": now.Sub(t.CreatedAt).Milliseconds()})
}

func (m *Manager) process(t model.ImageGenTask) {
	ctx, cancel := context.WithCancel(m.ctx)
	defer cancel()
	stopped := make(chan struct{})
	heartbeatDone := make(chan struct{})
	defer func() { close(stopped); <-heartbeatDone }()
	go func() {
		defer close(heartbeatDone)
		tick := time.NewTicker(10 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-ctx.Done():
				return
			case <-tick.C:
				if !m.update(&t, map[string]any{"lease_until": time.Now().Add(leaseDuration)}) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[imagegen] worker panic task=%s", t.ID)
			m.update(&t, map[string]any{"lease_owner": "", "lease_until": time.Now()})
		}
	}()
	plain, err := common.Decrypt(t.Snapshot)
	var s snapshot
	if err != nil || json.Unmarshal([]byte(plain), &s) != nil {
		m.terminal(&t, "failed", "Cannot read stored image task connection")
		return
	}
	if t.Status == "saving" {
		m.restoreAndSave(ctx, &t, &s)
		return
	}
	work := ctx
	var stop context.CancelFunc
	if s.Timeout > 0 {
		work, stop = context.WithTimeout(ctx, time.Duration(s.Timeout)*time.Second)
		defer stop()
	}
	var refs []imagegen.Image
	var mask *imagegen.Image
	for _, ref := range s.References {
		img, err := m.loadReference(work, t.UserID, ref.UploadID)
		if err != nil {
			if m.ctx.Err() != nil {
				m.releaseQueued(&t)
				return
			}
			m.terminal(&t, "failed", "Reference image is unavailable: "+redact(err.Error(), s.APIKey))
			return
		}
		if ref.Role == "mask" {
			mask = &img
		} else {
			refs = append(refs, img)
		}
	}
	if err := work.Err(); err != nil {
		if m.ctx.Err() != nil {
			m.releaseQueued(&t)
			return
		}
		m.terminal(&t, "failed", "Image preparation stopped before submission: "+err.Error())
		return
	}
	now := time.Now()
	if !m.update(&t, map[string]any{"status": "running", "started_at": now}) {
		return
	}
	t.Status = "running"
	client := &imagegen.Client{Provider: s.Provider, EndpointURL: s.Endpoint, APIKey: s.APIKey, Model: s.Model, EditRequestFormat: s.Format, HTTPClient: m.HTTPClient}
	result, err := client.Run(work, t.Operation, s.Params, refs, mask)
	if err != nil {
		status := "submission_unknown"
		// Explicit rejections, including a full image queue, did not accept work.
		var upstream *imagegen.APIError
		if errors.As(err, &upstream) && (upstream.StatusCode < 500 || upstream.QueueFull()) {
			status = "failed"
		}
		m.terminal(&t, status, redact(err.Error(), s.APIKey))
		return
	}
	if len(result.Images) == 0 {
		m.terminal(&t, "failed", "The upstream returned no images")
		return
	}
	result.Text = redact(result.Text, s.APIKey)
	for i := range result.Images {
		result.Images[i].RevisedPrompt = redact(result.Images[i].RevisedPrompt, s.APIKey)
	}
	if s.APIKey != "" && len(result.Usage) > 0 {
		encodedKey, _ := json.Marshal(s.APIKey)
		result.Usage = json.RawMessage(strings.ReplaceAll(string(result.Usage), string(encodedKey[1:len(encodedKey)-1]), "[REDACTED]"))
	}
	// Persist generated output before downloading/saving. Recovery only replays this phase.
	encoded, err := json.Marshal(result)
	if err != nil || int64(len(encoded)) > maxResultBytes {
		m.terminal(&t, "failed", "Generated image result is too large to persist")
		return
	}
	saveCtx, saveCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer saveCancel()
	key := "imagegen/tasks/" + t.ID + "/" + t.LeaseOwner + ".json"
	spoolErr := m.Store.Put(saveCtx, key, bytes.NewReader(encoded), "application/json")
	if spoolErr == nil {
		t.ResultKey = key
	}
	s.APIKey = ""
	clean, _ := json.Marshal(s)
	sealed, _ := common.Encrypt(string(clean))
	if !m.update(&t, map[string]any{"status": "saving", "artifact_status": "saving", "snapshot": sealed, "result_key": t.ResultKey, "result_count": len(result.Images)}) {
		if spoolErr == nil {
			_ = m.Store.Delete(saveCtx, key)
		}
		return
	}
	t.Status, t.Snapshot, t.ResultCount = "saving", sealed, len(result.Images)
	m.saveResult(saveCtx, &t, &s, result)
}

func (m *Manager) loadReference(ctx context.Context, uid, id int64) (imagegen.Image, error) {
	var row model.UploadedImage
	if err := m.DB.Where("id = ? AND user_id = ? AND status = ?", id, uid, model.UploadStatusUploaded).First(&row).Error; err != nil {
		return imagegen.Image{}, err
	}
	if imageasset.Expired(row.CreatedAt, row.Source) {
		return imagegen.Image{}, fmt.Errorf("reference image has expired")
	}
	r, err := m.Store.Get(ctx, row.StorageKey)
	if err != nil {
		return imagegen.Image{}, err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, imagegen.MaxImageBytes()+1))
	if err != nil {
		return imagegen.Image{}, err
	}
	img, _, _, err := imagegen.Inspect(data, imagegen.MaxImageBytes())
	return img, err
}

func (m *Manager) restoreAndSave(ctx context.Context, t *model.ImageGenTask, s *snapshot) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if t.ResultKey == "" {
		m.saveFailure(t, "Generated output could not be persisted; no automatic generation retry is performed")
		return
	}
	r, err := m.Store.Get(ctx, t.ResultKey)
	if err != nil {
		m.saveFailure(t, "Cannot read generated output from file storage; check storage availability and retry saving")
		return
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, maxResultBytes+1))
	var result imagegen.Result
	if err != nil || int64(len(data)) > maxResultBytes || json.Unmarshal(data, &result) != nil || len(result.Images) == 0 {
		m.saveFailure(t, "Invalid stored generated output")
		return
	}
	m.saveResult(ctx, t, s, &result)
}

func (m *Manager) saveFailure(t *model.ImageGenTask, message string) {
	now := time.Now()
	m.update(t, map[string]any{"status": "failed", "artifact_status": "failed", "error": message, "lease_owner": "", "lease_until": nil, "finished_at": now, "duration_ms": now.Sub(t.CreatedAt).Milliseconds()})
}

func (m *Manager) saveResult(ctx context.Context, t *model.ImageGenTask, s *snapshot, result *imagegen.Result) {
	warnings := []string{}
	saved := 0
	for index, img := range result.Images {
		var existing model.ImageGenTaskImage
		if m.DB.Where("task_id = ? AND output_index = ?", t.ID, index).First(&existing).Error == nil {
			var row model.UploadedImage
			if m.DB.Where("id = ? AND user_id = ? AND status = ?", existing.UploadID, t.UserID, model.UploadStatusUploaded).First(&row).Error == nil && !imageasset.Expired(row.CreatedAt, row.Source) {
				saved++
				continue
			}
		}
		data := img.Bytes
		var err error
		if len(data) == 0 {
			data, err = imagegen.Download(ctx, img.URL, s.Endpoint, imagegen.MaxImageBytes())
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("image %d: download failed; retry saving when the image source is available", index+1))
			continue
		}
		inspected, w, h, err := imagegen.Inspect(data, imagegen.MaxImageBytes())
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("image %d: %v", index+1, err))
			continue
		}
		asset, err := imageasset.SaveGenerated(ctx, m.Store, t.UserID, bytes.NewReader(data), imagegen.MaxImageBytes())
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("image %d: save failed; check file storage and generated image quota", index+1))
			continue
		}
		item := model.ImageGenTaskImage{TaskID: t.ID, OutputIndex: &index, UploadID: asset.UploadID, MIME: inspected.MIME, Size: asset.Size, Width: w, Height: h, RevisedPrompt: img.RevisedPrompt}
		err = m.DB.Transaction(func(tx *gorm.DB) error {
			var owned model.ImageGenTask
			if err := tx.Where("id = ? AND lease_owner = ? AND status = ?", t.ID, t.LeaseOwner, "saving").First(&owned).Error; err != nil {
				return err
			}
			return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "task_id"}, {Name: "output_index"}}, DoUpdates: clause.AssignmentColumns([]string{"upload_id", "mime", "size", "width", "height", "revised_prompt"})}).Create(&item).Error
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("image %d: metadata save failed", index+1))
			continue
		}
		saved++
	}
	status, artifact := "completed", "ready"
	if len(warnings) > 0 {
		status, artifact = "failed", "failed"
		if saved > 0 {
			status, artifact = "partial", "partial"
		}
	}
	encoded, _ := json.Marshal(warnings)
	now := time.Now()
	values := map[string]any{"status": status, "artifact_status": artifact, "warnings": string(encoded), "error": strings.Join(warnings, "\n"), "text": result.Text, "usage": string(result.Usage), "lease_owner": "", "lease_until": nil, "finished_at": now, "duration_ms": now.Sub(t.CreatedAt).Milliseconds()}
	if artifact == "ready" || t.ResultKey == "" {
		values["snapshot"] = ""
	}
	if !m.update(t, values) {
		return
	}
	if artifact == "ready" && t.ResultKey != "" {
		m.removeResult(t)
	}
}

func (m *Manager) releaseQueued(t *model.ImageGenTask) {
	m.owned(t).Where("status = ?", "queued").Updates(map[string]any{"lease_owner": "", "lease_until": nil})
}

func (m *Manager) RetrySave(uid int64, configID *int64, id string) (*model.ImageGenTask, error) {
	t, err := m.Get(uid, configID, id)
	if err != nil {
		return nil, err
	}
	if t.ResultKey == "" || (t.ArtifactStatus != "failed" && t.ArtifactStatus != "partial") {
		return nil, fmt.Errorf("only failed image saving can be retried")
	}
	res := freeLease(m.DB.Model(&model.ImageGenTask{}).Where("id = ? AND user_id = ? AND status = ? AND artifact_status = ?", id, uid, t.Status, t.ArtifactStatus), time.Now()).Updates(map[string]any{"status": "saving", "artifact_status": "saving", "finished_at": nil, "error": "", "warnings": "[]"})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, fmt.Errorf("image task is busy")
	}
	m.ProcessDue()
	return m.Get(uid, configID, id)
}

func (m *Manager) Delete(uid int64, id string) error {
	t, err := m.Get(uid, nil, id)
	if err != nil {
		return err
	}
	if Active(t) {
		return ErrActiveTask
	}
	owner := uuid.NewString()
	until := time.Now().Add(leaseDuration)
	res := freeLease(m.DB.Model(&model.ImageGenTask{}).Where("id = ? AND user_id = ? AND status = ?", id, uid, t.Status), time.Now()).Updates(map[string]any{"lease_owner": owner, "lease_until": until})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrActiveTask
	}
	defer m.DB.Model(&model.ImageGenTask{}).Where("id = ? AND lease_owner = ?", id, owner).Updates(map[string]any{"lease_owner": "", "lease_until": nil})
	if t.ResultKey != "" {
		if err := m.Store.Delete(m.ctx, t.ResultKey); err != nil {
			return err
		}
	}
	var keys []string
	err = m.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		keys, err = deleteTaskImages(tx, uid, id)
		if err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ? AND lease_owner = ?", id, uid, owner).Delete(&model.ImageGenTask{}).Error
	})
	if err == nil {
		deleteUnusedBlobs(m.ctx, m.DB, m.Store, keys)
	}
	return err
}

func (m *Manager) removeResult(t *model.ImageGenTask) {
	if m.Store.Delete(m.ctx, t.ResultKey) == nil {
		m.DB.Model(&model.ImageGenTask{}).Where("id = ? AND result_key = ? AND artifact_status = ?", t.ID, t.ResultKey, "ready").Update("result_key", "")
	}
}
func (m *Manager) cleanupResults() {
	var rows []model.ImageGenTask
	if freeLease(m.DB.Where("artifact_status = ? AND result_key <> ?", "ready", ""), time.Now()).Limit(50).Find(&rows).Error == nil {
		for i := range rows {
			m.removeResult(&rows[i])
		}
	}
}

func (m *Manager) cleanupHistory() {
	days := model.GetOptionInt("ImageGenHistoryDays")
	if days <= 0 {
		return
	}
	now := time.Now()
	// Keep the history that provides access to any still-valid output, just as
	// video history cleanup does. Apply each asset source's current retention.
	liveGenerated := m.DB.Where("uploaded_images.source = ?", model.ImageSourceGenerated)
	if retention := imageasset.Retention(model.ImageSourceGenerated); retention > 0 {
		liveGenerated = liveGenerated.Where("uploaded_images.created_at > ?", now.Add(-retention))
	}
	liveOther := m.DB.Where("uploaded_images.source <> ?", model.ImageSourceGenerated)
	if retention := imageasset.Retention(model.ImageSourceUpload); retention > 0 {
		liveOther = liveOther.Where("uploaded_images.created_at > ?", now.Add(-retention))
	}
	liveImages := m.DB.Model(&model.ImageGenTaskImage{}).Select("1").
		Joins("JOIN uploaded_images ON uploaded_images.id = imagegen_task_images.upload_id").
		Where("imagegen_task_images.task_id = imagegen_tasks.id AND uploaded_images.user_id = imagegen_tasks.user_id AND uploaded_images.status = ?", model.UploadStatusUploaded).
		Where(liveGenerated.Or(liveOther))
	var rows []model.ImageGenTask
	err := freeLease(m.DB.WithContext(m.ctx).Where("finished_at <= ? AND status NOT IN ?", now.Add(-time.Duration(days)*24*time.Hour), []string{"queued", "running", "saving"}), now).
		Where("NOT EXISTS (?)", liveImages).Order("finished_at ASC").Limit(100).Find(&rows).Error
	if err != nil {
		log.Printf("[image-task] list expired history: %v", err)
		return
	}
	for _, task := range rows {
		if err := m.Delete(task.UserID, task.ID); err != nil && !errors.Is(err, ErrActiveTask) {
			log.Printf("[image-task] delete expired history: %v", err)
		}
	}
}
