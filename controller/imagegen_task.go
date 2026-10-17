package controller

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/imagegentask"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

func ListImageGenTasks(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	page, size = max(1, page), min(100, max(1, size))
	q := model.DB.Model(&model.ImageGenTask{}).Where("user_id = ?", c.GetInt64("user_id"))
	if status := c.Query("status"); status != "" && status != "all" {
		switch status {
		case "running", "completed", "partial", "failed", "canceled", "interrupted":
			q = q.Where("status = ?", status)
		default:
			common.Error(c, 400, "Invalid task status")
			return
		}
	}
	if value := c.Query("config_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id < 0 {
			common.Error(c, 400, "Invalid config_id")
			return
		}
		q = q.Where("config_id = ?", id)
	}
	var total int64
	var tasks []model.ImageGenTask
	if err := q.Count(&total).Error; err != nil {
		common.Error(c, 500, "Could not list image tasks")
		return
	}
	if err := q.Order("created_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Find(&tasks).Error; err != nil {
		common.Error(c, 500, "Could not list image tasks")
		return
	}
	items, err := imagegentask.Views(tasks)
	if err != nil {
		common.Error(c, 500, "Could not load image task results")
		return
	}
	common.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

func GetImageGenTask(c *gin.Context) {
	task, err := imagegentask.Get(c.GetInt64("user_id"), c.Param("task_id"))
	if err != nil {
		common.Error(c, 404, "Image task not found")
		return
	}
	items, err := imagegentask.Views([]model.ImageGenTask{*task})
	if err != nil {
		common.Error(c, 500, "Could not load image task results")
		return
	}
	common.Success(c, items[0])
}

func DeleteImageGenTask(c *gin.Context) {
	err := imagegentask.Delete(c.GetInt64("user_id"), c.Param("task_id"))
	if err != nil {
		status := 500
		message := "Could not delete image task history"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, message = 404, "Image task not found"
		} else if errors.Is(err, imagegentask.ErrActiveTask) {
			status, message = 409, err.Error()
		}
		common.Error(c, status, message)
		return
	}
	common.Success(c, nil)
}
