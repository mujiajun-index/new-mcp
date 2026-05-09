package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		keyStr := c.GetHeader("X-API-Key")
		if keyStr == "" {
			auth := c.GetHeader("Authorization")
			if after, ok := strings.CutPrefix(auth, "Bearer "); ok {
				keyStr = strings.TrimSpace(after)
			}
		}
		if keyStr == "" {
			common.Error(c, http.StatusUnauthorized, "缺少 API Key")
			c.Abort()
			return
		}
		if !strings.HasPrefix(keyStr, common.ApiKeyPrefix) {
			common.Error(c, http.StatusUnauthorized, "无效的 API Key 格式")
			c.Abort()
			return
		}
		hash := sha256.Sum256([]byte(keyStr))
		keyHash := hex.EncodeToString(hash[:])

		apiKey, err := model.GetApiKeyByHash(keyHash)
		if err != nil {
			common.Error(c, http.StatusUnauthorized, "无效的 API Key")
			c.Abort()
			return
		}
		if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
			common.Error(c, http.StatusUnauthorized, "API Key 已过期")
			c.Abort()
			return
		}

		now := time.Now()
		apiKey.LastUsedAt = &now
		_ = model.DB.Save(apiKey)

		c.Set("api_key_id", apiKey.ID)
		c.Set("api_key_user_id", apiKey.UserID)
		c.Set("api_key_permissions", apiKey.Permissions)
		c.Next()
	}
}
