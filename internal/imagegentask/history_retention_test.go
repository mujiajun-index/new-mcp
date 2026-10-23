package imagegentask

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/model"
)

func TestImageHistoryRetention(t *testing.T) {
	m, _, _ := fixture(t)
	if model.GetOptionInt("ImageGenHistoryDays") != 30 {
		t.Fatal("image history should default to 30 days")
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)
	leasedUntil := time.Now().Add(time.Hour)
	rows := []model.ImageGenTask{
		{ID: "failed", Status: "failed", FinishedAt: &old, ResultKey: "image-results/old"},
		{ID: "unknown", Status: "submission_unknown", FinishedAt: &old},
		{ID: "interrupted", Status: "interrupted", FinishedAt: &old},
		{ID: "recent", Status: "failed", FinishedAt: &recent},
		{ID: "queued", Status: "queued", FinishedAt: &old},
		{ID: "running", Status: "running", FinishedAt: &old},
		{ID: "saving", Status: "saving", FinishedAt: &old},
		{ID: "leased", Status: "failed", FinishedAt: &old, LeaseUntil: &leasedUntil},
	}
	for i := range rows {
		rows[i].UserID, rows[i].CreatedAt = 1, old
		if err := m.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Store.Put(context.Background(), rows[0].ResultKey, bytes.NewReader([]byte("recoverable output")), "application/json"); err != nil {
		t.Fatal(err)
	}
	if err := model.UpdateOption("ImageGenHistoryDays", "0"); err != nil {
		t.Fatal(err)
	}
	m.cleanupHistory()
	var count int64
	if err := m.DB.Model(&model.ImageGenTask{}).Count(&count).Error; err != nil || count != int64(len(rows)) {
		t.Fatal("permanent history was cleaned", count, err)
	}
	if _, err := m.Store.Stat(context.Background(), rows[0].ResultKey); err != nil {
		t.Fatal("permanent history lost recoverable output", err)
	}
	model.UpdateOption("ImageGenHistoryDays", "30")
	m.cleanupHistory()
	for i, row := range rows {
		if err := m.DB.Model(&model.ImageGenTask{}).Where("id = ?", row.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(1)
		if i < 3 {
			want = 0
		}
		if count != want {
			t.Fatalf("history %s count = %d, want %d", row.ID, count, want)
		}
	}
	if _, err := m.Store.Stat(context.Background(), rows[0].ResultKey); err == nil {
		t.Fatal("expired history left recoverable output behind")
	}
}

func TestImageHistoryKeepsValidOutputs(t *testing.T) {
	m, _, data := fixture(t)
	old := time.Now().Add(-31 * 24 * time.Hour)
	expired := time.Now().Add(-48 * time.Hour)
	var expiredID int64
	var sharedKey string
	for _, uid := range []int64{1, 2} {
		asset, err := imageasset.SaveGenerated(context.Background(), m.Store, uid, bytes.NewReader(data), 1024)
		if err != nil {
			t.Fatal(err)
		}
		row := model.ImageGenTask{ID: fmt.Sprintf("output-%d", uid), UserID: uid, Status: "completed", ArtifactStatus: "ready", CreatedAt: old, FinishedAt: &old}
		if err := m.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		if err := m.DB.Create(&model.ImageGenTaskImage{TaskID: row.ID, UploadID: asset.UploadID}).Error; err != nil {
			t.Fatal(err)
		}
		if uid == 1 {
			expiredID, sharedKey = asset.UploadID, asset.Key
			if err := m.DB.Model(&model.UploadedImage{}).Where("id = ?", expiredID).Update("created_at", expired).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	model.UpdateOption("ImageGenRetentionHours", "0")
	m.cleanupHistory()
	var count int64
	if err := m.DB.Model(&model.ImageGenTask{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatal("permanently saved images lost their history", count, err)
	}
	model.UpdateOption("ImageGenRetentionHours", "24")
	m.cleanupHistory()
	if _, err := m.Get(1, nil, "output-1"); err == nil {
		t.Fatal("expired image history was retained")
	}
	if _, err := model.GetUploadedImageByID(expiredID); err == nil {
		t.Fatal("expired generated asset metadata was retained")
	}
	if _, err := m.Get(2, nil, "output-2"); err != nil {
		t.Fatal("still-valid output lost its history", err)
	}
	if _, err := m.Store.Stat(context.Background(), sharedKey); err != nil {
		t.Fatal("cleanup deleted a shared file used by another task", err)
	}
}
