package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

		var groups []string
		var perms struct {
			Groups []string `json:"groups"`
		}
		_ = json.Unmarshal([]byte(k.Permissions), &perms)
		if len(perms.Groups) > 0 {
			groups = perms.Groups
		}

		items[i] = dto.ApiKeyListItem{
			ID:             k.ID,
			Name:           k.Name,
			KeyPrefix:      k.KeyPrefix,
			Status:         k.Status,
			Groups:         groups,
			Quota:          k.Quota,
			UsedQuota:      k.UsedQuota,
			UnlimitedQuota: k.UnlimitedQuota,
			AllowIPs:       k.AllowIPs,
			ExpiresAt:      expiresAt,
			LastUsedAt:     lastUsedAt,
			CreatedAt:      k.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return items, nil
}

func (s *ApiKeyService) Create(userID int64, req *dto.CreateApiKeyReq) (*dto.ApiKeyCreateResult, error) {
	key := common.ApiKeyPrefix + generateRandomHex(32)
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])
	keyPrefix := key[:8]

	var groups []string
	if len(req.Groups) > 0 {
		validated, err := s.validateGroups(userID, req.Groups)
		if err != nil {
			return nil, err
		}
		groups = validated
	}

	permissionsJSON := "{}"
	if len(groups) > 0 {
		perms := map[string]interface{}{"groups": groups}
		b, _ := common.Marshal(perms)
		permissionsJSON = string(b)
	}

	unlimitedQuota := true
	if req.UnlimitedQuota != nil {
		unlimitedQuota = *req.UnlimitedQuota
	}

	var quota int64
	if req.Quota != nil {
		quota = *req.Quota
	}

	apiKey := &model.ApiKey{
		UserID:         userID,
		Name:           req.Name,
		KeyHash:        keyHash,
		KeyPrefix:      keyPrefix,
		Permissions:    permissionsJSON,
		Status:         common.StatusEnabled,
		Quota:          quota,
		UnlimitedQuota: unlimitedQuota,
		AllowIPs:       req.AllowIPs,
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
		ID:             apiKey.ID,
		Name:           apiKey.Name,
		Key:            key,
		KeyPrefix:      keyPrefix,
		Groups:         groups,
		Quota:          quota,
		UnlimitedQuota: unlimitedQuota,
		ExpiresAt:      expiresAt,
	}, nil
}

func (s *ApiKeyService) Update(userID, keyID int64, req *dto.UpdateApiKeyReq) error {
	apiKey, err := model.GetApiKeyByID(keyID)
	if err != nil {
		return fmt.Errorf("API Key 不存在")
	}
	if apiKey.UserID != userID {
		return fmt.Errorf("无权操作")
	}

	if req.Name != nil {
		apiKey.Name = *req.Name
	}
	if req.Status != nil {
		apiKey.Status = *req.Status
	}
	if req.Quota != nil {
		apiKey.Quota = *req.Quota
	}
	if req.UnlimitedQuota != nil {
		apiKey.UnlimitedQuota = *req.UnlimitedQuota
	}
	if req.AllowIPs != nil {
		apiKey.AllowIPs = *req.AllowIPs
	}
	if req.ExpiresAt != nil {
		t, err := parseTime(*req.ExpiresAt)
		if err == nil {
			apiKey.ExpiresAt = &t
		}
	}
	if req.Groups != nil {
		validated, err := s.validateGroups(userID, req.Groups)
		if err != nil {
			return err
		}
		permissionsJSON := "{}"
		if len(validated) > 0 {
			perms := map[string]interface{}{"groups": validated}
			b, _ := common.Marshal(perms)
			permissionsJSON = string(b)
		}
		apiKey.Permissions = permissionsJSON
	}

	return apiKey.Update()
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

func (s *ApiKeyService) validateGroups(userID int64, groupNames []string) ([]string, error) {
	if len(groupNames) == 1 && groupNames[0] == "*" {
		return []string{"*"}, nil
	}
	var count int64
	model.DB.Model(&model.McpGroup{}).
		Where("user_id = ? AND name IN ? AND status = ?", userID, groupNames, common.StatusEnabled).
		Count(&count)
	if int(count) != len(groupNames) {
		return nil, fmt.Errorf("one or more groups not found")
	}
	return groupNames, nil
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05Z", s)
}
