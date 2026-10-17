package videogen

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/model"
)

func TestTaskViewDuration(t *testing.T) {
	created := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	stopped := created.Add(1250 * time.Millisecond)
	beforeCreation := created.Add(-time.Second)
	for _, test := range []struct {
		name     string
		task     model.VideoGenTask
		duration any
	}{
		{"saved", model.VideoGenTask{Status: "completed", ArtifactStatus: "expired", CreatedAt: created, StoppedAt: &stopped}, int64(1250)},
		{"generation failed", model.VideoGenTask{Status: "failed", CreatedAt: created, StoppedAt: &stopped}, int64(1250)},
		{"save failed", model.VideoGenTask{Status: "completed", ArtifactStatus: "failed", CreatedAt: created, StoppedAt: &stopped}, int64(1250)},
		{"paused", model.VideoGenTask{Status: "queued", TrackingPaused: true, CreatedAt: created, StoppedAt: &stopped}, int64(1250)},
		{"running", model.VideoGenTask{Status: "queued", CreatedAt: created}, nil},
		{"legacy missing stop", model.VideoGenTask{Status: "completed", CreatedAt: created}, nil},
		{"clock went backwards", model.VideoGenTask{CreatedAt: created, StoppedAt: &beforeCreation}, nil},
		{"missing creation", model.VideoGenTask{StoppedAt: &stopped}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Later cleanup or metadata writes must not change the elapsed time.
			test.task.UpdatedAt = created.Add(48 * time.Hour)
			view := TaskView(&test.task)
			if view["duration_ms"] != test.duration {
				t.Fatalf("duration = %v, want %v", view["duration_ms"], test.duration)
			}
			data, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			var public map[string]any
			if err := json.Unmarshal(data, &public); err != nil {
				t.Fatal(err)
			}
			if _, exists := public["duration_ms"]; !exists {
				t.Fatal("duration must be present, including null when unavailable")
			}
			if test.task.StoppedAt == nil && public["stopped_at"] != nil {
				t.Fatal("running task has a stop time")
			}
			if test.task.StoppedAt != nil && public["stopped_at"] != test.task.StoppedAt.Format(time.RFC3339Nano) {
				t.Fatal("stop time was not serialized correctly")
			}
		})
	}
}
