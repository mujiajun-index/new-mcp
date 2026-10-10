package service

import (
	"context"
	"github.com/mujkjk/newmcp/internal/imageasset"
	"github.com/mujkjk/newmcp/internal/storage"
	"github.com/mujkjk/newmcp/model"
	"io"
	"time"
)

var UploadStore storage.Storage

type UploadService struct{}
type UploadResult = imageasset.Result

func (s *UploadService) Upload(ctx context.Context, userID int64, r io.Reader, maxBytes int64) (*UploadResult, error) {
	return imageasset.Save(ctx, UploadStore, userID, r, maxBytes)
}

func signedURLTTL() time.Duration {
	if secs := model.GetOptionInt("SignedURLTTLSeconds"); secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return time.Hour
}

func presignedPutTTL() time.Duration {
	if secs := model.GetOptionInt("PresignedPutTTLSeconds"); secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 10 * time.Minute
}

func CheckUploadQuota(userID int64) error { return imageasset.CheckQuota(userID) }
