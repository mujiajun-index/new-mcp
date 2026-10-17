// Package taskreference records validated task inputs as owner-scoped image
// assets. History stores asset references, never inline bytes or signed URLs.
package taskreference

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"time"

	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/mcp/imagegen"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

type Image struct {
	UploadID int64  `json:"upload_id"`
	Role     string `json:"role"`
	MIME     string `json:"mime"`
	Size     int64  `json:"size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type View struct {
	URL       string     `json:"url,omitempty"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	MIME      string     `json:"mime"`
	Size      int64      `json:"size"`
	Width     int        `json:"width"`
	Height    int        `json:"height"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func Capture(ctx context.Context, store storage.Storage, uid int64, img imagegen.Image, role string) (Image, error) {
	inspected, width, height, err := imagegen.Inspect(img.Bytes, imagegen.MaxImageBytes())
	if err != nil {
		return Image{}, err
	}
	item := Image{Role: role, MIME: inspected.MIME, Size: int64(len(img.Bytes)), Width: width, Height: height}
	sum := sha256.Sum256(img.Bytes)
	key := storage.ContentKey(hex.EncodeToString(sum[:]))
	if row, err := model.GetUploadedImageByUserAndKey(uid, key); err == nil && row.Status == model.UploadStatusUploaded && !imageasset.Expired(row.CreatedAt) {
		// Reusing an upload neither consumes another quota slot nor extends its lifetime.
		item.UploadID = row.ID
		return item, nil
	}
	saved, err := imageasset.SaveReference(ctx, store, uid, bytes.NewReader(img.Bytes), imagegen.MaxImageBytes())
	if err != nil {
		return Image{}, err
	}
	item.UploadID = saved.UploadID
	return item, nil
}

func Encode(images []Image) string {
	data, _ := json.Marshal(images)
	return string(data)
}

// LegacyUpload recovers an existing owned upload without downloading untrusted
// URLs or persisting the old signed link. Other legacy inputs cannot be restored.
func LegacyUpload(db *gorm.DB, uid int64, raw string) string {
	if db == nil || !storage.OwnsShortURL(raw) {
		return ""
	}
	u, err := url.Parse(raw)
	sid, ok := storage.ShortIDFromURL(raw)
	if err != nil || !ok || (u.Scheme != "https" && u.Scheme != "http") || !storage.VerifyShort("GET", sid, u.Query().Get("s")) {
		return ""
	}
	var row model.UploadedImage
	if db.Where("user_id = ? AND short_id = ?", uid, sid).First(&row).Error != nil {
		return ""
	}
	return Encode([]Image{{UploadID: row.ID, Role: "reference", MIME: row.MediaType, Size: row.Size}})
}

func Views(db *gorm.DB, uid int64, raw string) []View {
	views := []View{}
	var images []Image
	if json.Unmarshal([]byte(raw), &images) != nil || len(images) == 0 {
		return views
	}
	ids := make([]int64, 0, len(images))
	for _, img := range images {
		ids = append(ids, img.UploadID)
	}
	rows := []model.UploadedImage{}
	if db != nil {
		// A stored reference is not authority to read another user's asset.
		_ = db.Where("user_id = ? AND id IN ?", uid, ids).Find(&rows).Error
	}
	assets := map[int64]model.UploadedImage{}
	for _, row := range rows {
		assets[row.ID] = row
	}
	for _, img := range images {
		v := View{Role: img.Role, Status: "expired", MIME: img.MIME, Size: img.Size, Width: img.Width, Height: img.Height}
		if row, ok := assets[img.UploadID]; ok {
			v.ExpiresAt = imageasset.Expiration(row.CreatedAt)
			if row.Status == model.UploadStatusUploaded && !imageasset.Expired(row.CreatedAt) {
				v.URL, v.Status = storage.ShortURL("GET", row.ShortID), "ready"
			}
		}
		views = append(views, v)
	}
	return views
}
