package imagegentask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/internal/taskreference"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

var ErrActiveTask = errors.New("active image tasks cannot be deleted")

func Start(ctx context.Context, uid int64, cfg *model.ImageGenConfig, tool, source string, raw json.RawMessage) (*model.ImageGenTask, error) {
	if source != "test" {
		source = "mcp"
	}
	operation := "generate"
	if tool == model.EditImageToolName {
		operation = "edit"
	}
	var args struct {
		Prompt string            `json:"prompt"`
		Images []json.RawMessage `json:"images"`
	}
	_ = json.Unmarshal(raw, &args)
	task := &model.ImageGenTask{ID: uuid.NewString(), UserID: uid, ConfigID: cfg.ID, ConfigName: redact(cfg.Name, cfg.ApiKey), Source: source, Operation: operation, Provider: cfg.Provider, Model: cfg.ModelName, Prompt: redact(args.Prompt, cfg.ApiKey), ReferenceCount: len(args.Images), Status: "running"}
	if err := model.DB.WithContext(ctx).Create(task).Error; err != nil {
		return nil, fmt.Errorf("record image task: %w", err)
	}
	return task, nil
}

func redact(value, key string) string {
	if key != "" {
		return strings.ReplaceAll(value, key, "[REDACTED]")
	}
	return value
}

// Finish uses its own short deadline so cancellation of generation still leaves
// a terminal history record. No request bodies, reference bytes or keys persist.
func Finish(task *model.ImageGenTask, images []model.ImageGenTaskImage, raw json.RawMessage, runErr error, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var response struct {
		IsError bool `json:"isError"`
		Result  struct {
			Text     string         `json:"text"`
			Warnings []string       `json:"warnings"`
			Usage    map[string]any `json:"usage"`
		} `json:"structuredContent"`
	}
	_ = json.Unmarshal(raw, &response)
	status, message := "completed", ""
	if runErr != nil {
		status, message = "failed", runErr.Error()
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			status = "canceled"
		}
	} else if response.IsError || len(images) == 0 {
		status = "failed"
		if len(images) > 0 {
			status = "partial"
		}
		message = strings.Join(response.Result.Warnings, "\n")
	}
	now := time.Now()
	for i := range response.Result.Warnings {
		response.Result.Warnings[i] = redact(response.Result.Warnings[i], key)
	}
	warnings, _ := json.Marshal(response.Result.Warnings)
	usage, _ := json.Marshal(response.Result.Usage)
	if key != "" {
		encodedKey, _ := json.Marshal(key)
		usage = []byte(strings.ReplaceAll(string(usage), string(encodedKey[1:len(encodedKey)-1]), "[REDACTED]"))
	}
	return model.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range images {
			images[i].TaskID = task.ID
			images[i].RevisedPrompt = redact(images[i].RevisedPrompt, key)
		}
		if len(images) > 0 {
			if err := tx.Create(&images).Error; err != nil {
				return err
			}
		}
		return tx.Model(task).Updates(map[string]any{"status": status, "error": redact(message, key), "warnings": string(warnings), "text": redact(response.Result.Text, key), "usage": string(usage), "duration_ms": now.Sub(task.CreatedAt).Milliseconds(), "finished_at": now}).Error
	})
}

// Called once on server startup. Synchronous requests cannot resume after a
// process exit; recovering history must never resubmit a paid generation.
func RecoverInterrupted() error {
	return recoverInterrupted(model.DB)
}

func recoverInterrupted(db *gorm.DB) error {
	now := time.Now()
	return db.Model(&model.ImageGenTask{}).Where("status = ? AND idempotency_key IS NULL", "running").Updates(map[string]any{"status": "interrupted", "error": "Server restarted before the image request finished", "finished_at": now}).Error
}

func Get(uid int64, id string) (*model.ImageGenTask, error) {
	var task model.ImageGenTask
	err := model.DB.Where("id = ? AND user_id = ?", id, uid).First(&task).Error
	return &task, err
}

type ImageView struct {
	model.ImageGenTaskImage
	URL       string     `json:"url,omitempty"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type TaskView struct {
	model.ImageGenTask
	Warnings         []string             `json:"warnings"`
	Usage            map[string]any       `json:"usage,omitempty"`
	Images           []ImageView          `json:"images"`
	ReferenceImages  []taskreference.View `json:"reference_images"`
	PollAfterSeconds int                  `json:"poll_after_seconds"`
	CanRetrySave     bool                 `json:"can_retry_save"`
}

// Views signs fresh links and checks the current retention clock. Cleanup or a
// retention change therefore cannot leave a stale download link in history.
func Views(tasks []model.ImageGenTask) ([]TaskView, error) {
	views := make([]TaskView, 0, len(tasks))
	if len(tasks) == 0 {
		return views, nil
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	var images []model.ImageGenTaskImage
	if err := model.DB.Where("task_id IN ?", ids).Order("id ASC").Find(&images).Error; err != nil {
		return nil, err
	}
	uploadIDs := make([]int64, 0, len(images))
	for _, img := range images {
		uploadIDs = append(uploadIDs, img.UploadID)
	}
	uploads := map[int64]model.UploadedImage{}
	if len(uploadIDs) > 0 {
		var rows []model.UploadedImage
		if err := model.DB.Where("id IN ?", uploadIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			uploads[row.ID] = row
		}
	}
	byTask := map[string][]model.ImageGenTaskImage{}
	for _, img := range images {
		byTask[img.TaskID] = append(byTask[img.TaskID], img)
	}
	for _, task := range tasks {
		v := TaskView{ImageGenTask: task, Warnings: []string{}, Images: []ImageView{}}
		v.ReferenceImages = taskreference.Views(model.DB, task.UserID, task.ReferenceImages)
		if Active(&task) {
			v.PollAfterSeconds = 5
		}
		v.CanRetrySave = task.ResultKey != "" && (task.ArtifactStatus == "failed" || task.ArtifactStatus == "partial")
		if task.FinishedAt != nil && task.DurationMs == 0 {
			v.DurationMs = max(0, task.FinishedAt.Sub(task.CreatedAt).Milliseconds())
		}
		_ = json.Unmarshal([]byte(task.Warnings), &v.Warnings)
		if v.Warnings == nil {
			v.Warnings = []string{}
		}
		_ = json.Unmarshal([]byte(task.Usage), &v.Usage)
		for _, img := range byTask[task.ID] {
			item := ImageView{ImageGenTaskImage: img, Status: "expired"}
			if row, ok := uploads[img.UploadID]; ok && row.UserID == task.UserID {
				item.ExpiresAt = imageasset.Expiration(row.CreatedAt, row.Source)
				if row.Status == model.UploadStatusUploaded && !imageasset.Expired(row.CreatedAt, row.Source) {
					item.Status, item.URL = "ready", storage.ShortURL("GET", row.ShortID)
				}
			}
			v.Images = append(v.Images, item)
		}
		views = append(views, v)
	}
	return views, nil
}

// Deleting a task releases generated assets that no other output still uses.
// Upload and reference rows retain their own lifetimes and shared file bytes.
func Delete(uid int64, id string, store storage.Storage) error {
	var keys []string
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var task model.ImageGenTask
		if err := tx.Where("id = ? AND user_id = ?", id, uid).First(&task).Error; err != nil {
			return err
		}
		if Active(&task) || (task.LeaseUntil != nil && task.LeaseUntil.After(time.Now())) {
			return ErrActiveTask
		}
		if task.ResultKey != "" {
			return fmt.Errorf("use the image task manager to delete pending saved output")
		}
		var err error
		keys, err = deleteTaskImages(tx, uid, id)
		if err != nil {
			return err
		}
		return tx.Delete(&task).Error
	})
	if err == nil {
		deleteUnusedBlobs(context.Background(), model.DB, store, keys)
	}
	return err
}

func deleteTaskImages(tx *gorm.DB, uid int64, id string) ([]string, error) {
	var ids []int64
	if err := tx.Model(&model.ImageGenTaskImage{}).Where("task_id = ?", id).Pluck("upload_id", &ids).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("task_id = ?", id).Delete(&model.ImageGenTaskImage{}).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	var unused []model.UploadedImage
	used := tx.Model(&model.ImageGenTaskImage{}).Select("upload_id")
	if err := tx.Where("id IN ? AND user_id = ? AND source = ? AND id NOT IN (?)", ids, uid, model.ImageSourceGenerated, used).Find(&unused).Error; err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(unused))
	for _, img := range unused {
		if err := tx.Delete(&img).Error; err != nil {
			return nil, err
		}
		keys = append(keys, img.StorageKey)
	}
	return keys, nil
}

func deleteUnusedBlobs(ctx context.Context, db *gorm.DB, store storage.Storage, keys []string) {
	if store == nil {
		return
	}
	for _, key := range keys {
		var count int64
		if err := db.Model(&model.UploadedImage{}).Where("storage_key = ?", key).Count(&count).Error; err != nil {
			log.Printf("[image-task] check shared file references: %v", err)
		} else if count == 0 {
			if err := store.Delete(ctx, key); err != nil {
				log.Printf("[image-task] delete unused saved image: %v", err)
			}
		}
	}
}
