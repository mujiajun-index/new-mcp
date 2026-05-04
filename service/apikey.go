package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/dto"
	"github.com/mujkjk/newmcp/model"
)

type ApiKeyService struct{}

func (s *ApiKeyService) List(userID int64) ([]dto.ApiKeyListItem, error) {
	keys, err := model.ListApiKeysByUser(userID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.ApiKeyListItem, len(keys))
	for i, k := range keys {
		var expiresAt, lastUsedAt string
		if k.ExpiresAt != nil {
			expiresAt = k.ExpiresAt.Format("2006-01-02T15:04:05Z")
		}
		if k.LastUsedAt != nil {
			lastUsedAt = k.LastUsedAt.Format("2006-01-02T15:04:05Z")
		}
		items[i] = dto.ApiKeyListItem{
			ID:        k.ID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			Status:    k.Status,
			ExpiresAt: expiresAt,
			LastUsedAt: lastUsedAt,
			CreatedAt: k.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, nil
}

func (s *ApiKeyService) Create(userID int64, req *dto.CreateApiKeyReq) (*dto.ApiKeyCreateResult, error) {
	key := common.ApiKeyPrefix + generateRandomHex(32)
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])
	keyPrefix := key[:8]

	permissionsJSON := "{}"
	if req.Permissions != nil {
		b, _ := common.Marshal(req.Permissions)
		permissionsJSON = string(b)
	}

	apiKey := &model.ApiKey{
		UserID:      userID,
		Name:        req.Name,
		KeyHash:     keyHash,
		KeyPrefix:   keyPrefix,
		Permissions: permissionsJSON,
		Status:      common.StatusEnabled,
	}
	if req.ExpiresAt != nil {
		t, err := parseTime(*req.ExpiresAt)
		if err == nil {
			apiKey.ExpiresAt = &t
		}
	}

	if err := apiKey.Insert(); err != nil {
		return nil, err
	}

	var expiresAt string
	if apiKey.ExpiresAt != nil {
		expiresAt = apiKey.ExpiresAt.Format("2006-01-02T15:04:05Z")
	}

	return &dto.ApiKeyCreateResult{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       key,
		KeyPrefix: keyPrefix,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *ApiKeyService) Delete(userID, keyID int64) error {
	keys, err := model.ListApiKeysByUser(userID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k.ID == keyID {
			return k.Delete()
		}
	}
	return nil
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05Z", s)
}
