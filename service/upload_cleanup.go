package service

import (
	"context"
	"log"
	"time"

	"github.com/mujkjk/newmcp/model"
)

// StartCleanupLoop reaps expired vision uploads on a ticker: for each row older
// than the retention window it deletes the stored blob (UploadStore.Delete) and
// the metadata row. It returns when ctx is cancelled. The sweep interval is
// read from options once at start; the retention cutoff is recomputed every
// sweep, so an admin changing UploadRetentionHours takes effect on the next
// tick rather than only on rows inserted after the change.
func StartCleanupLoop(ctx context.Context) {
	if UploadStore == nil {
		log.Printf("[upload-cleanup] storage not initialized; cleanup loop disabled")
		return
	}
	interval := cleanupInterval()
	log.Printf("[upload-cleanup] started; sweep interval=%s retention=%s", interval, uploadRetention())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Printf("[upload-cleanup] stopped")
			return
		case <-ticker.C:
			runCleanupSweep(ctx)
		}
	}
}

// runCleanupSweep deletes expired rows and their blobs. Bounded per sweep so a
// large backlog cannot monopolize a tick. Per-item errors are logged and
// skipped — one bad blob must not abort the whole sweep (the row is still
// removed, and a leftover blob is harmless / reaped idempotently next time).
func runCleanupSweep(ctx context.Context) {
	const sweepLimit = 500
	cutoff := time.Now().Add(-uploadRetention())
	expired, err := model.ListExpiredUploads(cutoff, sweepLimit)
	if err != nil {
		log.Printf("[upload-cleanup] list expired: %v", err)
		return
	}
	if len(expired) == 0 {
		return
	}
	deleted := 0
	for i := range expired {
		img := &expired[i]
		if err := UploadStore.Delete(ctx, img.StorageKey); err != nil {
			log.Printf("[upload-cleanup] delete blob %q: %v", img.StorageKey, err)
		}
		if err := model.DeleteUploadedImageByID(img.ID); err != nil {
			log.Printf("[upload-cleanup] delete row %d: %v", img.ID, err)
			continue
		}
		deleted++
	}
	log.Printf("[upload-cleanup] swept %d/%d expired uploads", deleted, len(expired))
}

func cleanupInterval() time.Duration {
	if m := model.GetOptionInt("UploadCleanupIntervalMinutes"); m > 0 {
		return time.Duration(m) * time.Minute
	}
	return time.Hour
}

func uploadRetention() time.Duration {
	if h := model.GetOptionInt("UploadRetentionHours"); h > 0 {
		return time.Duration(h) * time.Hour
	}
	return 24 * time.Hour
}
