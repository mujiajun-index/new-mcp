package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/audioasset"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
	"mime"
	"net/http"
	"strconv"
	"time"
)

var audioGenService = &service.AudioGenService{}

func bindAudio(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 33<<20)
	if e := c.ShouldBindJSON(v); e != nil {
		common.Error(c, 400, e.Error())
		return false
	}
	return true
}
func audioID(c *gin.Context) (int64, bool) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id <= 0 {
		common.Error(c, 400, "Invalid audio configuration ID")
		return 0, false
	}
	return id, true
}
func ListAudioGenConfigs(c *gin.Context) {
	v, e := audioGenService.List(c.GetInt64("user_id"))
	if e != nil {
		common.Error(c, 500, "Could not list audio configurations")
		return
	}
	common.Success(c, v)
}
func GetAudioGenConfig(c *gin.Context) {
	id, ok := audioID(c)
	if !ok {
		return
	}
	v, e := audioGenService.Get(c.GetInt64("user_id"), id)
	if e != nil {
		common.Error(c, 404, "Audio configuration not found")
		return
	}
	common.Success(c, v)
}
func CreateAudioGenConfig(c *gin.Context) {
	var r dto.AudioGenConfigReq
	if !bindAudio(c, &r) {
		return
	}
	v, e := audioGenService.Create(c.GetInt64("user_id"), &r)
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Created(c, v)
}
func UpdateAudioGenConfig(c *gin.Context) {
	id, ok := audioID(c)
	if !ok {
		return
	}
	var r dto.AudioGenConfigReq
	if !bindAudio(c, &r) {
		return
	}
	if e := audioGenService.Update(c.GetInt64("user_id"), id, &r); e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, nil)
}
func mutateAudio(c *gin.Context, fn func(int64, int64) error) {
	id, ok := audioID(c)
	if !ok {
		return
	}
	if e := fn(c.GetInt64("user_id"), id); e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, nil)
}
func DeleteAudioGenConfig(c *gin.Context)  { mutateAudio(c, audioGenService.Delete) }
func EnableAudioGenConfig(c *gin.Context)  { mutateAudio(c, audioGenService.Enable) }
func DisableAudioGenConfig(c *gin.Context) { mutateAudio(c, audioGenService.Disable) }
func ListAudioGenModels(c *gin.Context) {
	var r dto.AudioGenProbeReq
	if !bindAudio(c, &r) {
		return
	}
	v, e := audioGenService.Models(c.Request.Context(), c.GetInt64("user_id"), &r)
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, v)
}
func TestAudioGenConfig(c *gin.Context) {
	var r dto.AudioGenProbeReq
	if !bindAudio(c, &r) {
		return
	}
	v, e := audioGenService.Test(c.Request.Context(), c.GetInt64("user_id"), &r)
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, v)
}
func requireAudio(c *gin.Context) bool {
	if virtual.AudioManager == nil {
		common.Error(c, 503, "Audio manager not initialized")
		return false
	}
	return true
}
func ListAudioGenTasks(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	page = max(1, page)
	size = min(100, max(1, size))
	q := model.DB.Model(&model.AudioGenTask{}).Where("user_id = ?", c.GetInt64("user_id"))
	for _, key := range []string{"status", "kind", "config_id"} {
		if v := c.Query(key); v != "" && v != "all" {
			q = q.Where(key+" = ?", v)
		}
	}
	var total int64
	if e := q.Count(&total).Error; e != nil {
		common.Error(c, 500, "Could not list audio tasks")
		return
	}
	var rows []model.AudioGenTask
	if e := q.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; e != nil {
		common.Error(c, 500, "Could not list audio tasks")
		return
	}
	items := []any{}
	for i := range rows {
		items = append(items, virtual.AudioManager.View(&rows[i]))
	}
	common.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}
func GetAudioGenTask(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	t, e := virtual.AudioManager.Get(c.GetInt64("user_id"), nil, c.Param("task_id"))
	if e != nil {
		common.Error(c, 404, "Audio task not found")
		return
	}
	common.Success(c, virtual.AudioManager.View(t))
}
func RetryAudioGenDownload(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	t, e := virtual.AudioManager.RetrySave(c.GetInt64("user_id"), nil, c.Param("task_id"))
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, virtual.AudioManager.View(t))
}
func GetAudioGenRetryInput(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	t, e := virtual.AudioManager.Get(c.GetInt64("user_id"), nil, c.Param("task_id"))
	if e != nil {
		common.Error(c, 404, "Audio task not found")
		return
	}
	args, e := virtual.AudioManager.RetryInput(t)
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, gin.H{"config_id": t.ExecutionConfigID, "kind": t.Kind, "arguments": args})
}
func RetryAudioGenTask(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	uid := c.GetInt64("user_id")
	var r dto.AudioGenRetryReq
	if !bindAudio(c, &r) {
		return
	}
	t, e := virtual.AudioManager.Get(uid, nil, c.Param("task_id"))
	if e != nil {
		common.Error(c, 404, "Audio task not found")
		return
	}
	if r.ConfigID == 0 {
		r.ConfigID = t.ExecutionConfigID
	}
	cfg, e := model.GetAudioGenConfig(uid, r.ConfigID)
	if e != nil {
		common.Error(c, 400, "Select an available audio configuration")
		return
	}
	var args map[string]any
	if len(r.Arguments) > 0 {
		if json.Unmarshal(r.Arguments, &args) != nil || args == nil {
			common.Error(c, 400, "arguments must be a JSON object")
			return
		}
	}
	t, e = virtual.AudioManager.Retry(c.Request.Context(), uid, t.ID, cfg, args)
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, virtual.AudioManager.View(t))
}
func DeleteAudioGenTask(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	if e := virtual.AudioManager.Delete(c.GetInt64("user_id"), c.Param("task_id")); e != nil {
		common.Error(c, 409, e.Error())
		return
	}
	common.Success(c, nil)
}
func BatchDeleteAudioGenTasks(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	var r struct {
		IDs []string `json:"ids"`
	}
	if !bindAudio(c, &r) {
		return
	}
	if len(r.IDs) > 100 {
		common.Error(c, 400, "Select at most 100 tasks")
		return
	}
	failed := map[string]string{}
	for _, id := range r.IDs {
		if e := virtual.AudioManager.Delete(c.GetInt64("user_id"), id); e != nil {
			failed[id] = e.Error()
		}
	}
	common.Success(c, gin.H{"failed": failed})
}
func UploadAudio(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, audioasset.InputLimit()+1<<20)
	file, _, e := c.Request.FormFile("audio")
	if e != nil {
		common.Error(c, 400, "Select a WAV or MP3 audio file")
		return
	}
	defer file.Close()
	b, e := audioasset.Read(file, audioasset.InputLimit())
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	if _, _, e = audioasset.Inspect(b, true); e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	pending, e := audioasset.NewPending(virtual.AudioManager.DB, c.GetInt64("user_id"))
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	if e = finishAudioUpload(c, pending, b); e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Created(c, audioasset.View(pending))
}
func finishAudioUpload(c *gin.Context, a *model.AudioAsset, b []byte) error {
	mt, _, e := audioasset.Inspect(b, true)
	if e != nil {
		return e
	}
	db := virtual.AudioManager.DB
	res := db.Model(a).Where("status = ?", "pending").Update("status", "uploading")
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return fmt.Errorf("upload slot already used")
	}
	if e = virtual.AudioManager.Store.Put(c.Request.Context(), a.StorageKey, bytes.NewReader(b), mt); e != nil {
		_ = db.Model(a).Update("status", "pending").Error
		return e
	}
	model.MediaRetentionMutex.RLock()
	defer model.MediaRetentionMutex.RUnlock()
	a.Status = "ready"
	a.MIME = mt
	a.Size = int64(len(b))
	if hours := audioasset.Option("AudioGenRetentionHours", 24); hours > 0 {
		expiry := a.CreatedAt.Add(time.Duration(hours) * time.Hour)
		a.ExpiresAt = &expiry
	}
	if e = db.Save(a).Error; e != nil {
		_ = virtual.AudioManager.Store.Delete(c.Request.Context(), a.StorageKey)
		_ = db.Model(a).Update("status", "pending").Error
	}
	return e
}
func PutAudioFile(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	sid := c.Param("sid")
	if !audioasset.Verify("PUT", sid, c.Query("s")) {
		common.Error(c, 403, "Invalid audio upload signature")
		return
	}
	var a model.AudioAsset
	if model.DB.Where("short_id = ? AND source = ?", sid, "upload").First(&a).Error != nil {
		common.Error(c, 404, "Audio upload slot not found")
		return
	}
	if time.Since(a.CreatedAt) > 15*time.Minute {
		common.Error(c, 410, "Audio upload slot expired")
		return
	}
	if a.Status != "pending" {
		common.Error(c, 409, "Audio upload slot already used")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, audioasset.InputLimit())
	b, e := audioasset.Read(c.Request.Body, audioasset.InputLimit())
	if e == nil {
		e = finishAudioUpload(c, &a, b)
	}
	if e != nil {
		common.Error(c, 400, e.Error())
		return
	}
	common.Success(c, audioasset.View(&a))
}
func GetAudioFile(c *gin.Context) {
	if !requireAudio(c) {
		return
	}
	sid := c.Param("sid")
	if !audioasset.Verify(c.Request.Method, sid, c.Query("s")) {
		common.Error(c, 403, "Invalid audio signature")
		return
	}
	var a model.AudioAsset
	if model.DB.Where("short_id = ?", sid).First(&a).Error != nil {
		common.Error(c, 404, "Audio not found")
		return
	}
	if audioasset.Expired(&a) {
		common.Error(c, 410, "Audio expired")
		return
	}
	if a.Status != "ready" {
		common.Error(c, 404, "Audio not found")
		return
	}
	store, ok := virtual.AudioManager.Store.(storage.SeekableStorage)
	if !ok {
		common.Error(c, 503, "Audio storage cannot seek")
		return
	}
	r, e := store.OpenSeek(c.Request.Context(), a.StorageKey)
	if e != nil {
		common.Error(c, 404, "Audio unavailable")
		return
	}
	defer r.Close()
	c.Header("Content-Type", a.MIME)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Query("download") == "1" {
		extensions := map[string]string{"audio/wav": ".wav", "audio/mpeg": ".mp3", "audio/ogg": ".ogg", "audio/flac": ".flac", "audio/mp4": ".m4a", "audio/aac": ".aac"}
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "audio-" + sid + extensions[a.MIME]}))
	}
	http.ServeContent(c.Writer, c.Request, "audio", a.CreatedAt, r)
}
