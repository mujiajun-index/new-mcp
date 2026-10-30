package service

import (
	"context"
	"log"
	"time"

	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/virtual/events"
	"github.com/mujkjk/newmcp/model"
)

// StartCleanupLoop reaps expired vision uploads on a ticker: for each row older
// than the retention window it deletes the stored blob (UploadStore.Delete) and
// the metadata row. It returns when ctx is cancelled. A first sweep runs
// immediately at startup — a short-lived server may never reach the first tick.
// The sweep interval is read once at start. Each sweep uses the current policy
// and each file's retention start, which is renewed when the policy changes.
func StartCleanupLoop(ctx context.Context) {
	if UploadStore == nil {
		log.Printf("[upload-cleanup] storage not initialized; cleanup loop disabled")
		return
	}
	interval := cleanupInterval()
	log.Printf("[upload-cleanup] started; sweep interval=%s retention=%s", interval, uploadRetention())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Sweep immediately at startup, not only on the first tick: a frequently
	// restarted server (dev sessions, short-lived deploys) may never reach the
	// first tick, so expired rows+blobs would linger indefinitely while reads
	// already answer 410.
	runCleanupSweep(ctx)
	runPendingReapSweep(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[upload-cleanup] stopped")
			return
		case <-ticker.C:
			runCleanupSweep(ctx)
			runPendingReapSweep(ctx)
		}
	}
}

// runCleanupSweep deletes expired rows and their blobs. Bounded per sweep so a
// large backlog cannot monopolize a tick. Per-item errors are logged and
// skipped — one bad blob must not abort the whole sweep (the row is still
// removed, and a leftover blob is harmless / reaped idempotently next time).
//
// V1.1: blobs are shared across users (content-addressed), so the blob is only
// deleted once the last row referencing it is gone (CountByKey refcount).
func runCleanupSweep(ctx context.Context) {
	model.MediaRetentionMutex.RLock()
	defer model.MediaRetentionMutex.RUnlock()
	const sweepLimit = 500
	var expired []model.UploadedImage
	for _, source := range []string{model.ImageSourceUpload, model.ImageSourceGenerated} {
		retention := imageasset.Retention(source)
		if retention == 0 {
			continue
		}
		rows, err := model.ListExpiredUploads(time.Now().Add(-retention), source == model.ImageSourceGenerated, sweepLimit)
		if err != nil {
			log.Printf("[upload-cleanup] list expired %s: %v", source, err)
			continue
		}
		expired = append(expired, rows...)
	}
	if len(expired) == 0 {
		return
	}
	deleted := 0
	for i := range expired {
		img := &expired[i]
		// Delete the row first, then drop the blob only if no other row still
		// references this (shared) content key.
		if err := model.DeleteUploadedImageByID(img.ID); err != nil {
			log.Printf("[upload-cleanup] delete row %d: %v", img.ID, err)
			continue
		}
		if img.Source == model.ImageSourceGenerated {
			notifyExpiredImageTasks(img.ID)
		}
		if n, err := model.CountByKey(img.StorageKey); err != nil {
			log.Printf("[upload-cleanup] refcount %q: %v", img.StorageKey, err)
		} else if n == 0 {
			if err := UploadStore.Delete(ctx, img.StorageKey); err != nil {
				log.Printf("[upload-cleanup] delete blob %q: %v", img.StorageKey, err)
			}
		}
		deleted++
	}
	log.Printf("[upload-cleanup] swept %d/%d expired uploads", deleted, len(expired))
}

func notifyExpiredImageTasks(uploadID int64) {
	var ids []string
	if err := model.DB.Model(&model.ImageGenTaskImage{}).Where("upload_id = ?", uploadID).Distinct().Pluck("task_id", &ids).Error; err != nil || len(ids) == 0 {
		return
	}
	var tasks []model.ImageGenTask
	if model.DB.Where("id IN ?", ids).Find(&tasks).Error != nil {
		return
	}
	for i := range tasks {
		task := &tasks[i]
		res := model.DB.Model(task).UpdateColumn("updated_at", time.Now())
		if res.Error == nil && res.RowsAffected == 1 {
			events.TaskChanged("imagegen", task.UserID, task.ServiceID, task.ConfigID)
		}
	}
}

// runPendingReapSweep (V1.1) fast-reaps presigned-PUT slots that never received
// their curl: once the PUT URL has expired (plus a grace window) a pending slot
// can never be completed, so reclaim it immediately rather than waiting the full
// retention window. These rows have no blob on disk/bucket (the PUT never
// landed), so only the row is deleted.
func runPendingReapSweep(ctx context.Context) {
	const pendingLimit = 500
	cutoff := time.Now().Add(-(presignedPutTTL() + 5*time.Minute))
	pending, err := model.ListPendingExpired(cutoff, pendingLimit)
	if err != nil {
		log.Printf("[upload-cleanup] list pending: %v", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	deleted := 0
	for i := range pending {
		img := &pending[i]
		if err := model.DeleteUploadedImageByID(img.ID); err != nil {
			log.Printf("[upload-cleanup] delete pending row %d: %v", img.ID, err)
			continue
		}
		deleted++
	}
	log.Printf("[upload-cleanup] reaped %d/%d abandoned pending slots", deleted, len(pending))
}

func cleanupInterval() time.Duration {
	if m := model.GetOptionInt("UploadCleanupIntervalMinutes"); m > 0 {
		return time.Duration(m) * time.Minute
	}
	return time.Hour
}

func uploadRetention() time.Duration {
	return imageasset.Retention(model.ImageSourceUpload)
}
