package controller

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/internal/mcp/videogen"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/videoasset"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
)

var videoGenService = &service.VideoGenService{}

func videoGenID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		common.Error(c, 400, "Invalid video configuration ID")
		return 0, false
	}
	return id, true
}
func bindVideoJSON(c *gin.Context, v any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 17<<20)
	if err := c.ShouldBindJSON(v); err != nil {
		common.Error(c, 400, err.Error())
		return false
	}
	return true
}
func ListVideoGenConfigs(c *gin.Context) {
	items, err := videoGenService.List(c.GetInt64("user_id"))
	if err != nil {
		common.Error(c, 500, "Failed to list video configurations")
		return
	}
	common.Success(c, items)
}
func CreateVideoGenConfig(c *gin.Context) {
	var r dto.VideoGenConfigReq
	if !bindVideoJSON(c, &r) {
		return
	}
	item, err := videoGenService.Create(c.GetInt64("user_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Created(c, item)
}
func GetVideoGenConfig(c *gin.Context) {
	id, ok := videoGenID(c)
	if !ok {
		return
	}
	item, err := videoGenService.Get(c.GetInt64("user_id"), id)
	if err != nil {
		common.Error(c, 404, "Video configuration not found")
		return
	}
	common.Success(c, item)
}
func UpdateVideoGenConfig(c *gin.Context) {
	id, ok := videoGenID(c)
	if !ok {
		return
	}
	var r dto.VideoGenConfigReq
	if !bindVideoJSON(c, &r) {
		return
	}
	if err := videoGenService.Update(c.GetInt64("user_id"), id, &r); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, nil)
}
func DeleteVideoGenConfig(c *gin.Context)  { mutateVideoConfig(c, videoGenService.Delete) }
func EnableVideoGenConfig(c *gin.Context)  { mutateVideoConfig(c, videoGenService.Enable) }
func DisableVideoGenConfig(c *gin.Context) { mutateVideoConfig(c, videoGenService.Disable) }
func mutateVideoConfig(c *gin.Context, fn func(int64, int64) error) {
	id, ok := videoGenID(c)
	if !ok {
		return
	}
	if err := fn(c.GetInt64("user_id"), id); err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, nil)
}
func ListVideoGenModels(c *gin.Context) {
	var r dto.VideoGenProbeReq
	if !bindVideoJSON(c, &r) {
		return
	}
	items, err := videoGenService.Models(c.Request.Context(), c.GetInt64("user_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, items)
}
func TestVideoGenConfig(c *gin.Context) {
	var r dto.VideoGenProbeReq
	if !bindVideoJSON(c, &r) {
		return
	}
	item, err := videoGenService.Test(c.Request.Context(), c.GetInt64("user_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, item)
}

func requireVideoManager(c *gin.Context) bool {
	if virtual.VideoManager == nil {
		common.Error(c, 503, "Video manager not initialized")
		return false
	}
	return true
}
func ListVideoGenTasks(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	page = max(1, page)
	pageSize = min(100, max(1, pageSize))
	q := model.DB.Model(&model.VideoGenTask{}).Where("user_id = ?", c.GetInt64("user_id"))
	if status := c.Query("status"); status != "" && status != "all" {
		switch status {
		case "submitting", "queued", "in_progress", "completed", "failed", "unknown", "submission_unknown":
			q = q.Where("status = ?", status)
		default:
			common.Error(c, 400, "Invalid task status")
			return
		}
	}
	if id := c.Query("config_id"); id != "" {
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err != nil || parsed < 0 {
			common.Error(c, 400, "Invalid config_id")
			return
		}
		q = q.Where("config_id = ?", parsed)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		common.Error(c, 500, "Could not list video tasks")
		return
	}
	var tasks []model.VideoGenTask
	if err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tasks).Error; err != nil {
		common.Error(c, 500, "Could not list video tasks")
		return
	}
	items := make([]map[string]any, 0, len(tasks))
	for i := range tasks {
		items = append(items, videogen.TaskView(&tasks[i]))
	}
	common.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize})
}
func GetVideoGenTask(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	t, err := virtual.VideoManager.Get(c.GetInt64("user_id"), nil, c.Param("task_id"), true)
	if err != nil {
		common.Error(c, 404, "Video task not found")
		return
	}
	common.Success(c, videogen.TaskView(t))
}
func RetryVideoGenDownload(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	t, err := virtual.VideoManager.RetryDownload(c.GetInt64("user_id"), nil, c.Param("task_id"))
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, videogen.TaskView(t))
}
func GetVideoGenRetryInput(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	input, err := videoGenService.RetryInput(c.GetInt64("user_id"), c.Param("task_id"))
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, input)
}
func RetryVideoGenTask(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	var r dto.VideoGenRetryReq
	if !bindVideoJSON(c, &r) {
		return
	}
	t, err := videoGenService.Retry(c.Request.Context(), c.GetInt64("user_id"), c.Param("task_id"), &r)
	if err != nil {
		common.Error(c, 400, err.Error())
		return
	}
	common.Success(c, videogen.TaskView(t))
}
func DeleteVideoGenTask(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	err := virtual.VideoManager.Delete(c.GetInt64("user_id"), c.Param("task_id"))
	if err != nil {
		status := 400
		if errors.Is(err, videogen.ErrActiveTask) || errors.Is(err, videogen.ErrBusy) {
			status = 409
		}
		common.Error(c, status, err.Error())
		return
	}
	common.Success(c, nil)
}

func GetVideoFile(c *gin.Context) {
	if !requireVideoManager(c) {
		return
	}
	sid := c.Param("sid")
	if !videoasset.Verify(sid, c.Query("s")) {
		common.Error(c, 403, "Invalid video signature")
		return
	}
	var t model.VideoGenTask
	if model.DB.Where("short_id = ?", sid).First(&t).Error != nil {
		common.Error(c, 404, "Video not found")
		return
	}
	expiresAt := videoasset.EffectiveExpiration(t.ExpiresAt)
	if (expiresAt != nil && !expiresAt.After(time.Now())) || t.ArtifactStatus == "expired" {
		common.Error(c, 410, "Video expired")
		return
	}
	if t.ArtifactStatus != "ready" {
		common.Error(c, 404, "Video not found")
		return
	}
	store, ok := virtual.VideoManager.Store.(storage.SeekableStorage)
	if !ok {
		common.Error(c, 503, "Video storage does not support seeking")
		return
	}
	r, err := store.OpenSeek(c.Request.Context(), t.StorageKey)
	if err != nil {
		common.Error(c, 404, "Video file unavailable")
		return
	}
	defer r.Close()
	c.Header("Content-Type", t.MIME)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Query("download") == "1" {
		ext := ".mp4"
		if t.MIME == "video/webm" {
			ext = ".webm"
		}
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "generated-video-" + t.ID + ext}))
	}
	http.ServeContent(c.Writer, c.Request, "generated-video", t.CreatedAt, r)
}
