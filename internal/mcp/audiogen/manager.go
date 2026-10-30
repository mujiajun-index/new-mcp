package audiogen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual/events"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type Manager struct {
	DB         *gorm.DB
	Store      storage.Storage
	HTTPClient *http.Client
	mu         sync.Mutex
	wg         sync.WaitGroup
	cancel     context.CancelFunc
	wake       chan struct{}
}

func NewManager(db *gorm.DB, store storage.Storage) *Manager {
	quiet := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	_ = quiet.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Option{Key: "AudioGenConcurrency", Value: fmt.Sprint(max(1, audioasset.Option("AudioGenConcurrency", 4)))}).Error
	return &Manager{DB: quiet, Store: store, wake: make(chan struct{}, 1)}
}

// A marker written after the payload prevents a crash during Put from being
// mistaken for a complete output. The digest also detects truncated objects.
func (m *Manager) stage(ctx context.Context, key string, out *Output) error {
	b, e := json.Marshal(out)
	if e != nil {
		return e
	}
	if e = m.Store.Put(ctx, key, bytes.NewReader(b), "application/json"); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	return m.Store.Put(ctx, key+"-complete", strings.NewReader(hex.EncodeToString(sum[:])), "text/plain")
}
func (m *Manager) readStage(ctx context.Context, key string) (Output, error) {
	var out Output
	r, e := m.Store.Get(ctx, key+"-complete")
	if e != nil {
		return out, e
	}
	marker, e := audioasset.Read(r, 64)
	r.Close()
	if e != nil {
		return out, e
	}
	r, e = m.Store.Get(ctx, key)
	if e != nil {
		return out, e
	}
	b, e := audioasset.Read(r, audioasset.OutputLimit()*3+1<<20)
	r.Close()
	if e != nil {
		return out, e
	}
	sum := sha256.Sum256(b)
	if string(marker) != hex.EncodeToString(sum[:]) {
		return out, fmt.Errorf("staged output is incomplete")
	}
	e = json.Unmarshal(b, &out)
	return out, e
}
func (m *Manager) Start(parent context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			m.sweep(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-m.wake:
			}
		}
	}()
}
func (m *Manager) Close() {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
}
func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func Resolve(c *model.AudioGenConfig, kind string) (Connection, error) {
	for _, s := range c.Models {
		if s.Kind != kind {
			continue
		}
		if !s.Enabled {
			return Connection{}, fmt.Errorf("%s slot is disabled", kind)
		}
		for _, v := range c.Connections {
			if v.Key == s.ConnectionKey {
				key, e := common.Decrypt(v.APIKey)
				if e != nil {
					return Connection{}, fmt.Errorf("could not decrypt audio connection")
				}
				return Connection{BaseURL: v.BaseURL, APIKey: key, Slot: s}, nil
			}
		}
	}
	return Connection{}, fmt.Errorf("%s slot is not configured", kind)
}
func (m *Manager) prepare(ctx context.Context, uid int64, conn Connection, args map[string]any) (Input, string, error) {
	p := map[string]any{}
	_ = json.Unmarshal([]byte(conn.Slot.DefaultParams), &p)
	for k, v := range args {
		p[k] = v
	}
	if _, e := Normalize(&conn.Slot, p, false); e != nil {
		return Input{}, "", e
	}
	in := Input{Params: p}
	total := int64(0)
	if refs, ok := p["images"].([]any); ok {
		for _, v := range refs {
			img, e := imagegen.LoadReference(ctx, m.Store, uid, v.(string), imagegen.MaxImageBytes())
			if e != nil {
				return in, "", e
			}
			in.Images = append(in.Images, Media{Data: img.Bytes, MIME: img.MIME})
		}
	}
	if refs, ok := p["audios"].([]any); ok {
		for _, v := range refs {
			b, format, e := audioasset.Load(ctx, m.DB, m.Store, uid, v.(string), audioasset.InputLimit()-total)
			if e != nil {
				return in, "", e
			}
			total += int64(len(b))
			if total > audioasset.InputLimit() {
				return in, "", fmt.Errorf("input audio exceeds total limit")
			}
			mt, _, _ := audioasset.Inspect(b, true)
			in.Audios = append(in.Audios, Media{Data: b, MIME: mt, Format: format})
		}
	}
	// Fingerprints use resolved content, not expiring URLs or response controls.
	hashInput := Input{Params: map[string]any{}, Images: in.Images, Audios: in.Audios}
	for k, v := range p {
		if !Contains([]string{"images", "audios", "request_id", "return_mode", "wait_seconds"}, k) {
			hashInput.Params[k] = v
		}
	}
	b, _ := json.Marshal(hashInput)
	sum := sha256.Sum256(b)
	return in, hex.EncodeToString(sum[:]), nil
}
func (m *Manager) capture(ctx context.Context, t *model.AudioGenTask, in Input) error {
	refs := []taskreference.Image{}
	for _, img := range in.Images {
		ref, e := taskreference.Capture(ctx, m.Store, t.UserID, imagegen.Image{Bytes: img.Data, MIME: img.MIME}, "reference")
		if e != nil {
			return e
		}
		refs = append(refs, ref)
	}
	t.ReferenceImages = taskreference.Encode(refs)
	ids := []int64{}
	for _, a := range in.Audios {
		saved, e := audioasset.Save(ctx, m.DB, m.Store, t.UserID, t.ID, "reference", a.Data)
		if e != nil {
			return e
		}
		ids = append(ids, saved.ID)
	}
	b, _ := json.Marshal(ids)
	t.ReferenceAudios = string(b)
	p := map[string]any{}
	for k, v := range in.Params {
		if !Contains([]string{"images", "audios", "request_id", "wait_seconds", "return_mode"}, k) {
			p[k] = v
		}
	}
	b, _ = json.Marshal(p)
	t.RequestParams = string(b)
	// The execution snapshot contains validated bytes, never duplicate URL/Base64 references.
	delete(in.Params, "images")
	delete(in.Params, "audios")
	b, _ = json.Marshal(in)
	t.InputKey = "audio-input/" + t.ID + "/" + uuid.NewString()
	return m.Store.Put(ctx, t.InputKey, bytes.NewReader(b), "application/json")
}
func (m *Manager) Submit(ctx context.Context, uid int64, c *model.AudioGenConfig, kind string, args map[string]any, source string) (*model.AudioGenTask, error) {
	if uid == 0 || !model.CanExecuteConfig(ctx, uid, c.UserID, c.ID, "audiogen") {
		return nil, fmt.Errorf("audio configuration is not accessible")
	}
	conn, e := Resolve(c, kind)
	if e != nil {
		return nil, e
	}
	in, hash, e := m.prepare(ctx, uid, conn, args)
	if e != nil {
		return nil, e
	}
	requestID := Text(args, "request_id")
	if requestID == "" {
		requestID = uuid.NewString()
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s|%s", uid, c.ID, kind, requestID)))
	idem := model.ScopedTaskKey(ctx, hex.EncodeToString(sum[:]))
	var existing model.AudioGenTask
	if e = m.DB.Where("idempotency_key = ?", idem).First(&existing).Error; e == nil {
		if existing.RequestHash != hash {
			return nil, fmt.Errorf("request_id already used with different input")
		}
		return &existing, nil
	} else if e != gorm.ErrRecordNotFound {
		return nil, e
	}
	b, _ := json.Marshal(conn)
	encrypted, e := common.Encrypt(string(b))
	if e != nil {
		return nil, e
	}
	taskScope := model.VirtualExecutionFrom(ctx)
	t := &model.AudioGenTask{ID: uuid.NewString(), UserID: uid, ServiceID: taskScope.ServiceID, MarketplaceItemID: taskScope.MarketplaceItemID, ConfigID: c.ID, ExecutionConfigID: c.ID, ConfigName: c.Name, Kind: kind, Source: source, Protocol: conn.Slot.Protocol, Model: conn.Slot.Model, Prompt: prompt(kind, in.Params), Status: "queued", ArtifactStatus: "none", IdempotencyKey: idem, RequestHash: hash, Connection: encrypted}
	if e = m.capture(ctx, t, in); e != nil {
		return nil, e
	}
	if e = m.DB.Create(t).Error; e != nil {
		_ = m.Store.Delete(context.Background(), t.InputKey)
		if m.DB.Where("idempotency_key = ?", idem).First(&existing).Error == nil && existing.RequestHash == hash {
			return &existing, nil
		}
		return nil, e
	}
	notifyAudioTask(t)
	m.notify()
	return t, nil
}

func notifyAudioTask(t *model.AudioGenTask) {
	events.TaskChanged("audiogen", t.UserID, t.ServiceID, t.ConfigID)
}
func (m *Manager) Get(uid int64, configID *int64, id string) (*model.AudioGenTask, error) {
	q := m.DB.Where("id = ? AND user_id = ?", id, uid)
	if configID != nil {
		q = q.Where("config_id = ?", *configID)
	}
	var t model.AudioGenTask
	e := q.First(&t).Error
	return &t, e
}
func (m *Manager) Wait(ctx context.Context, uid int64, id string, seconds int) (*model.AudioGenTask, error) {
	deadline := time.NewTimer(time.Duration(seconds) * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		t, e := m.Get(uid, nil, id)
		if e != nil {
			return nil, e
		}
		if seconds == 0 || !(t.Status == "queued" || t.Status == "running" || t.ArtifactStatus == "pending" || t.ArtifactStatus == "saving") {
			return t, nil
		}
		select {
		case <-ctx.Done():
			return t, nil
		case <-deadline.C:
			return t, nil
		case <-tick.C:
		}
	}
}
func (m *Manager) sweep(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	var stale []model.AudioGenTask
	_ = m.DB.Where("status = ? AND lease_until < ?", "running", time.Now()).Limit(16).Find(&stale).Error
	for _, t := range stale {
		key := "audio-stage/" + t.ID + fmt.Sprintf("/%d", t.RetryCount)
		status, artifact, errText := "submission_unknown", "none", "Process interrupted after submission; generation was not resubmitted"
		if _, e := m.readStage(ctx, key); e == nil {
			status, artifact, errText = "completed", "pending", ""
		}
		res := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND status = ? AND lease_until < ?", t.ID, "running", time.Now()).Updates(map[string]any{"status": status, "artifact_status": artifact, "output_key": key, "error": errText, "connection": "", "lease_owner": ""})
		if res.Error == nil && res.RowsAffected == 1 {
			notifyAudioTask(&t)
		}
	}
	var saveCount int64
	_ = m.DB.Model(&model.AudioGenTask{}).Where("artifact_status = ? AND lease_until > ?", "saving", time.Now()).Count(&saveCount).Error
	var saving []model.AudioGenTask
	_ = m.DB.Where("status = ? AND (artifact_status = ? OR (artifact_status = ? AND lease_until < ?))", "completed", "pending", "saving", time.Now()).Limit(4).Find(&saving).Error
	for _, t := range saving {
		if saveCount >= max(1, audioasset.Option("AudioGenConcurrency", 4)) {
			break
		}
		owner := uuid.NewString()
		res := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND (artifact_status = ? OR (artifact_status = ? AND lease_until < ?))", t.ID, "pending", "saving", time.Now()).Updates(map[string]any{"artifact_status": "saving", "lease_owner": owner, "lease_until": time.Now().Add(10 * time.Minute)})
		if res.Error == nil && res.RowsAffected == 1 {
			t.LeaseOwner = owner
			notifyAudioTask(&t)
			saveCount++
			m.wg.Add(1)
			go func(t model.AudioGenTask) { defer m.wg.Done(); m.save(ctx, &t) }(t)
		}
	}
	for i := 0; i < int(max(1, audioasset.Option("AudioGenConcurrency", 4))); i++ {
		var task model.AudioGenTask
		claimed := false
		err := m.DB.Transaction(func(tx *gorm.DB) error {
			// Serializes scheduling across processes sharing this database.
			if tx.Dialector.Name() != "sqlite" {
				var option model.Option
				if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(&model.Option{Key: "AudioGenConcurrency"}).First(&option).Error; e != nil && e != gorm.ErrRecordNotFound {
					return e
				}
			}
			var count int64
			if e := tx.Model(&model.AudioGenTask{}).Where("status = ? AND lease_until > ?", "running", time.Now()).Count(&count).Error; e != nil {
				return e
			}
			if count >= max(1, audioasset.Option("AudioGenConcurrency", 4)) {
				return nil
			}
			if e := tx.Where("status = ?", "queued").Order("created_at").First(&task).Error; e != nil {
				return e
			}
			lease := time.Now().Add(25 * time.Hour)
			var conn Connection
			raw, e := common.Decrypt(task.Connection)
			if e == nil {
				_ = json.Unmarshal([]byte(raw), &conn)
				lease = time.Now().Add(time.Duration(max(1, conn.Slot.TimeoutSeconds)+60) * time.Second)
			}
			owner := uuid.NewString()
			res := tx.Model(&model.AudioGenTask{}).Where("id = ? AND status = ?", task.ID, "queued").Updates(map[string]any{"status": "running", "lease_owner": owner, "lease_until": lease})
			claimed = res.Error == nil && res.RowsAffected == 1
			if claimed {
				task.LeaseOwner = owner
			}
			return res.Error
		})
		if err != nil || !claimed {
			break
		}
		notifyAudioTask(&task)
		m.wg.Add(1)
		go func(t model.AudioGenTask) { defer m.wg.Done(); m.generate(ctx, &t) }(task)
	}
	m.cleanup(ctx)
}
func (m *Manager) generate(parent context.Context, t *model.AudioGenTask) {
	var conn Connection
	raw, e := common.Decrypt(t.Connection)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &conn)
	}
	if e != nil {
		m.fail(t, "failed", "invalid private connection snapshot")
		return
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(max(1, conn.Slot.TimeoutSeconds))*time.Second)
	defer cancel()
	var in Input
	r, e := m.Store.Get(ctx, t.InputKey)
	if e == nil {
		b, err := audioasset.Read(r, 256<<20)
		r.Close()
		e = err
		if e == nil {
			e = json.Unmarshal(b, &in)
		}
	}
	if e != nil {
		m.fail(t, "failed", "input snapshot unavailable; request was not sent")
		return
	}
	out, e := (&Client{Connection: conn, HTTPClient: m.HTTPClient}).Generate(ctx, in)
	if e != nil {
		status := "failed"
		if IsUncertain(e) {
			status = "submission_unknown"
		}
		m.fail(t, status, e.Error())
		return
	}
	out.Connection = t.Connection
	// Persist the complete result before publishing completion or touching final files.
	key := "audio-stage/" + t.ID + fmt.Sprintf("/%d", t.RetryCount)
	if e = m.stage(ctx, key, out); e != nil {
		m.fail(t, "submission_unknown", "generation returned but durable staging failed; automatic resubmission is disabled")
		return
	}
	res := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND status = ? AND lease_owner = ?", t.ID, "running", t.LeaseOwner).Updates(map[string]any{"status": "completed", "artifact_status": "pending", "output_key": key, "connection": "", "result_text": out.Text, "transcript": out.Transcript, "input_transcript": out.InputTranscript, "lyrics": out.Lyrics, "usage": string(out.Usage)})
	// result_text is the column name; no caller-visible state is inferred from HTTP success.
	if res.Error != nil {
		res = m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND status = ? AND lease_owner = ?", t.ID, "running", t.LeaseOwner).Updates(map[string]any{"status": "completed", "artifact_status": "pending", "output_key": key, "connection": ""})
	}
	if res.Error == nil && res.RowsAffected == 1 {
		notifyAudioTask(t)
	}
	m.notify()
}
func (m *Manager) fail(t *model.AudioGenTask, status, msg string) {
	res := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND status = ? AND lease_owner = ?", t.ID, "running", t.LeaseOwner).Updates(map[string]any{"status": status, "error": msg, "connection": "", "lease_owner": "", "lease_until": time.Time{}})
	if res.Error == nil && res.RowsAffected == 1 {
		notifyAudioTask(t)
	}
}
func (m *Manager) save(parent context.Context, t *model.AudioGenTask) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	out, e := m.readStage(ctx, t.OutputKey)
	if e == nil && len(out.Audios) == 0 {
		e = fmt.Errorf("staged result contains no audio")
	}
	var total int64
	var download Connection
	if out.Connection != "" {
		if plain, err := common.Decrypt(out.Connection); err == nil {
			_ = json.Unmarshal([]byte(plain), &download)
		}
	}
	for i := range out.Audios {
		if e != nil {
			break
		}
		a := &out.Audios[i]
		if a.URL != "" {
			headers := http.Header{}
			if strings.HasPrefix(download.Slot.Protocol, "gemini_") {
				headers.Set("x-goog-api-key", download.APIKey)
			} else if download.APIKey != "" {
				headers.Set("Authorization", "Bearer "+download.APIKey)
			}
			var response *http.Response
			response, e = imagegen.OpenDownloadWithHeaders(ctx, a.URL, download.BaseURL, audioasset.OutputLimit()-total, headers)
			if e == nil {
				a.Data, e = audioasset.Read(response.Body, audioasset.OutputLimit()-total)
				response.Body.Close()
			}
		}
		if e != nil {
			break
		}
		if a.MIME == "audio/pcm" {
			a.Data, e = audioasset.WAV(a.Data, a.Rate, a.Channels, a.Bits)
		}
		total += int64(len(a.Data))
		if total > audioasset.OutputLimit() {
			e = fmt.Errorf("audio exceeds output limit")
			break
		}
		if e == nil {
			_, _, e = audioasset.Inspect(a.Data, false)
		}
	}
	if e == nil {
		var existing []model.AudioAsset
		_ = m.DB.Where("task_id = ? AND source = ?", t.ID, "generated").Find(&existing).Error
		for _, a := range existing {
			_ = m.Store.Delete(ctx, a.StorageKey)
			_ = m.DB.Delete(&a).Error
		}
		for _, a := range out.Audios {
			if _, e = audioasset.Save(ctx, m.DB, m.Store, t.UserID, t.ID, "generated", a.Data); e != nil {
				break
			}
		}
	}
	updates := map[string]any{"lease_owner": "", "lease_until": time.Time{}}
	if e != nil {
		updates["artifact_status"] = "failed"
		updates["last_error"] = "Could not save generated audio: " + e.Error()
	} else {
		updates["artifact_status"] = "ready"
		updates["last_error"] = ""
		updates["result_text"] = out.Text
		updates["transcript"] = out.Transcript
		updates["input_transcript"] = out.InputTranscript
		updates["lyrics"] = out.Lyrics
		updates["usage"] = string(out.Usage)
	}
	res := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND lease_owner = ? AND artifact_status = ?", t.ID, t.LeaseOwner, "saving").Updates(updates)
	if res.Error == nil && res.RowsAffected == 1 {
		notifyAudioTask(t)
	}
}
func (m *Manager) RetrySave(uid int64, configID *int64, id string) (*model.AudioGenTask, error) {
	t, e := m.Get(uid, configID, id)
	if e != nil {
		return nil, e
	}
	if t.Status != "completed" || t.ArtifactStatus != "failed" || t.OutputKey == "" {
		return nil, fmt.Errorf("task has no recoverable failed save")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, e = m.Store.Stat(ctx, t.OutputKey+"-complete"); e != nil {
		return nil, fmt.Errorf("staged output is unavailable")
	}
	if _, e = m.Store.Stat(ctx, t.OutputKey); e != nil {
		return nil, fmt.Errorf("staged output is unavailable")
	}
	res := m.DB.Model(t).Where("artifact_status = ?", "failed").Updates(map[string]any{"artifact_status": "pending", "last_error": ""})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 1 {
		notifyAudioTask(t)
	}
	m.notify()
	return m.Get(uid, configID, id)
}
func (m *Manager) RetryInput(t *model.AudioGenTask) (map[string]any, error) {
	p := map[string]any{}
	if json.Unmarshal([]byte(t.RequestParams), &p) != nil {
		return nil, fmt.Errorf("input snapshot unavailable")
	}
	images := taskreference.Views(m.DB, t.UserID, t.ReferenceImages)
	if len(images) > 0 {
		refs := []any{}
		for _, v := range images {
			if v.Status != "ready" {
				return nil, fmt.Errorf("reference image expired; upload it again")
			}
			refs = append(refs, v.URL)
		}
		p["images"] = refs
	}
	var ids []int64
	_ = json.Unmarshal([]byte(t.ReferenceAudios), &ids)
	if len(ids) > 0 {
		refs := []any{}
		for _, id := range ids {
			var a model.AudioAsset
			if m.DB.Where("id = ? AND user_id = ?", id, t.UserID).First(&a).Error != nil || audioasset.Expired(&a) {
				return nil, fmt.Errorf("reference audio expired; upload it again")
			}
			refs = append(refs, audioasset.URL("GET", a.ShortID))
		}
		p["audios"] = refs
	}
	return p, nil
}
func (m *Manager) Retry(ctx context.Context, uid int64, id string, c *model.AudioGenConfig, args map[string]any) (*model.AudioGenTask, error) {
	t, e := m.Get(uid, nil, id)
	if e != nil {
		return nil, e
	}
	if t.Status != "failed" {
		return nil, fmt.Errorf("only explicitly failed tasks can be regenerated")
	}
	conn, e := Resolve(c, t.Kind)
	if e != nil {
		return nil, e
	}
	if args == nil {
		args, e = m.RetryInput(t)
		if e != nil {
			return nil, e
		}
	}
	if _, err := m.RetryInput(t); err != nil {
		images := taskreference.Views(m.DB, uid, t.ReferenceImages)
		for _, ref := range images {
			if ref.Status != "ready" && args["images"] == nil {
				return nil, fmt.Errorf("reference images expired; upload replacements before retrying")
			}
		}
		var ids []int64
		_ = json.Unmarshal([]byte(t.ReferenceAudios), &ids)
		for _, id := range ids {
			var a model.AudioAsset
			if m.DB.Where("id = ? AND user_id = ?", id, uid).First(&a).Error != nil || audioasset.Expired(&a) {
				if args["audios"] == nil {
					return nil, fmt.Errorf("reference audio expired; upload replacements before retrying")
				}
			}
		}
	}
	in, _, e := m.prepare(ctx, uid, conn, args)
	if e != nil {
		return nil, e
	}
	oldInput := t.InputKey
	if e = m.capture(ctx, t, in); e != nil {
		return nil, e
	}
	b, _ := json.Marshal(conn)
	encrypted, e := common.Encrypt(string(b))
	if e != nil {
		return nil, e
	}
	res := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND user_id = ? AND status = ?", id, uid, "failed").Updates(map[string]any{"execution_config_id": c.ID, "config_name": c.Name, "protocol": conn.Slot.Protocol, "model": conn.Slot.Model, "prompt": prompt(t.Kind, in.Params), "connection": encrypted, "input_key": t.InputKey, "request_params": t.RequestParams, "reference_images": t.ReferenceImages, "reference_audios": t.ReferenceAudios, "status": "queued", "artifact_status": "none", "error": "", "last_error": "", "output_key": "", "retry_count": gorm.Expr("retry_count + 1")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, fmt.Errorf("task changed during retry")
	}
	notifyAudioTask(t)
	_ = m.Store.Delete(context.Background(), oldInput)
	m.notify()
	return m.Get(uid, nil, id)
}
func (m *Manager) View(t *model.AudioGenTask) map[string]any {
	b, _ := json.Marshal(t)
	v := map[string]any{}
	_ = json.Unmarshal(b, &v)
	v["reference_images"] = taskreference.Views(m.DB, t.UserID, t.ReferenceImages)
	var refs []int64
	_ = json.Unmarshal([]byte(t.ReferenceAudios), &refs)
	input := []any{}
	for _, id := range refs {
		var a model.AudioAsset
		if m.DB.Where("id = ? AND user_id = ?", id, t.UserID).First(&a).Error == nil {
			input = append(input, audioasset.View(&a))
		} else {
			input = append(input, map[string]any{"id": id, "status": "expired"})
		}
	}
	v["reference_audios"] = input
	var assets []model.AudioAsset
	_ = m.DB.Where("task_id = ? AND user_id = ? AND source = ?", t.ID, t.UserID, "generated").Order("id").Find(&assets).Error
	audios := []any{}
	for _, a := range assets {
		audios = append(audios, audioasset.View(&a))
	}
	v["audios"] = audios
	v["can_retry_save"] = t.Status == "completed" && t.ArtifactStatus == "failed" && t.OutputKey != ""
	if t.Usage != "" {
		var usage any
		if json.Unmarshal([]byte(t.Usage), &usage) == nil {
			v["usage"] = usage
		}
	}
	return v
}
func (m *Manager) Result(ctx context.Context, t *model.AudioGenTask, mode string) (json.RawMessage, error) {
	view := m.View(t)
	summary, _ := json.Marshal(view)
	content := []any{map[string]any{"type": "text", "text": string(summary)}}
	var assets []model.AudioAsset
	_ = m.DB.Where("task_id = ? AND user_id = ? AND source = ? AND status = ?", t.ID, t.UserID, "generated", "ready").Order("id").Find(&assets).Error
	var total int64
	for _, a := range assets {
		if !audioasset.Expired(&a) {
			total += a.Size
		}
	}
	inline := mode == "inline" && total <= audioasset.Option("AudioGenInlineMaxBytes", 1<<20)
	if mode == "inline" && !inline {
		content = append(content, map[string]any{"type": "text", "text": "Inline audio exceeds the response limit; use the saved links."})
	}
	for _, a := range assets {
		if audioasset.Expired(&a) {
			continue
		}
		content = append(content, map[string]any{"type": "resource_link", "uri": audioasset.URL("GET", a.ShortID), "name": "generated-audio", "mimeType": a.MIME, "size": a.Size})
		if inline {
			r, e := m.Store.Get(ctx, a.StorageKey)
			if e == nil {
				b, err := audioasset.Read(r, a.Size)
				r.Close()
				if err == nil {
					content = append(content, map[string]any{"type": "audio", "data": base64.StdEncoding.EncodeToString(b), "mimeType": a.MIME})
				}
			}
		}
	}
	return json.Marshal(map[string]any{"content": content, "structuredContent": view, "isError": t.Status == "failed"})
}
func (m *Manager) Delete(uid int64, id string) (err error) {
	t, e := m.Get(uid, nil, id)
	if e != nil {
		return e
	}
	if t.Status == "queued" || t.Status == "running" || t.ArtifactStatus == "pending" || t.ArtifactStatus == "saving" {
		return fmt.Errorf("active audio tasks cannot be deleted")
	}
	claim := m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND user_id = ? AND status = ? AND artifact_status = ?", id, uid, t.Status, t.ArtifactStatus).Update("status", "deleting")
	if claim.Error != nil {
		return claim.Error
	}
	if claim.RowsAffected != 1 {
		return fmt.Errorf("task changed during deletion")
	}
	defer func() {
		if err != nil {
			_ = m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND status = ?", id, "deleting").Update("status", t.Status).Error
		}
	}()
	var assets []model.AudioAsset
	_ = m.DB.Where("task_id = ? AND user_id = ?", id, uid).Find(&assets).Error
	for _, a := range assets {
		if a.StorageKey != "" {
			e = m.Store.Delete(context.Background(), a.StorageKey)
		}
		if e != nil {
			return e
		}
		if e = m.DB.Delete(&a).Error; e != nil {
			return e
		}
	}
	for _, key := range []string{t.InputKey, t.OutputKey} {
		if key != "" {
			_ = m.Store.Delete(context.Background(), key)
			if key == t.OutputKey {
				_ = m.Store.Delete(context.Background(), key+"-complete")
			}
		}
	}
	res := m.DB.Where("id = ? AND user_id = ? AND status = ?", id, uid, "deleting").Delete(&model.AudioGenTask{})
	if res.Error == nil && res.RowsAffected == 1 {
		notifyAudioTask(t)
	}
	return res.Error
}
func (m *Manager) cleanup(ctx context.Context) {
	model.MediaRetentionMutex.RLock()
	defer model.MediaRetentionMutex.RUnlock()
	// Small bounded batches keep normal task scheduling responsive.
	var assets []model.AudioAsset
	q := m.DB.Where("(status IN ? AND created_at < ?)", []string{"pending", "uploading"}, time.Now().Add(-15*time.Minute))
	if audioasset.Option("AudioGenRetentionHours", 24) > 0 {
		q = q.Or("status = ? AND expires_at IS NOT NULL AND expires_at < ?", "ready", time.Now())
	}
	_ = q.Limit(16).Find(&assets).Error
	for _, a := range assets {
		if m.Store.Delete(ctx, a.StorageKey) == nil {
			if a.TaskID != "" {
				if m.DB.Model(&a).Updates(map[string]any{"status": "expired", "storage_key": ""}).Error == nil {
					if task, err := m.Get(a.UserID, nil, a.TaskID); err == nil {
						if m.DB.Model(task).Update("updated_at", time.Now()).Error == nil {
							notifyAudioTask(task)
						}
					}
				}
			} else {
				_ = m.DB.Delete(&a).Error
			}
		}
	}
	var snapshots []model.AudioGenTask
	_ = m.DB.Where("status NOT IN ? AND (input_key <> '' OR (artifact_status = ? AND output_key <> ''))", []string{"queued", "running"}, "ready").Limit(16).Find(&snapshots).Error
	for _, t := range snapshots {
		updates := map[string]any{}
		if t.InputKey != "" && m.Store.Delete(ctx, t.InputKey) == nil {
			updates["input_key"] = ""
		}
		if t.ArtifactStatus == "ready" && t.OutputKey != "" && m.Store.Delete(ctx, t.OutputKey) == nil {
			_ = m.Store.Delete(ctx, t.OutputKey+"-complete")
			updates["output_key"] = ""
		}
		if len(updates) > 0 {
			_ = m.DB.Model(&model.AudioGenTask{}).Where("id = ? AND status NOT IN ?", t.ID, []string{"queued", "running"}).Updates(updates).Error
		}
	}
	if days := audioasset.Option("AudioGenHistoryDays", 30); days > 0 {
		var tasks []model.AudioGenTask
		liveOutputs := m.DB.Model(&model.AudioAsset{}).Select("1").
			Where("audio_assets.task_id = audiogen_tasks.id AND audio_assets.user_id = audiogen_tasks.user_id AND source = ? AND status = ? AND storage_key <> ?", "generated", "ready", "")
		if audioasset.Option("AudioGenRetentionHours", 24) > 0 {
			liveOutputs = liveOutputs.Where("expires_at IS NULL OR expires_at > ?", time.Now())
		}
		_ = m.DB.Where("created_at < ? AND status NOT IN ? AND artifact_status NOT IN ?", time.Now().Add(-time.Duration(days)*24*time.Hour), []string{"queued", "running"}, []string{"pending", "saving"}).
			Where("NOT EXISTS (?)", liveOutputs).Limit(8).Find(&tasks).Error
		for _, t := range tasks {
			_ = m.Delete(t.UserID, t.ID)
		}
	}
}
