package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// emailVerificationRateLimitWindow is the rolling window during which a
	// client IP is capped at emailVerificationRateLimitMax requests.
	emailVerificationRateLimitWindow = 60 * time.Second
	// emailVerificationRateLimitMax is the maximum number of verification
	// emails a single IP may request per window.
	emailVerificationRateLimitMax = 1
)

type evEntry struct {
	count    int
	expireAt time.Time
}

var (
	evMu      sync.Mutex
	evRecords = make(map[string]*evEntry)
)

// EmailVerificationRateLimit caps how often a single client IP can request a
// verification email. Storage is in-memory (this project does not use Redis),
func EmailVerificationRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := GetRequestIP(c)
		now := time.Now()

		evMu.Lock()
		if e, ok := evRecords[key]; ok && now.Before(e.expireAt) {
			e.count++
			if e.count > emailVerificationRateLimitMax {
				wait := int(e.expireAt.Sub(now).Seconds())
				if wait < 1 {
					wait = 1
				}
				evMu.Unlock()
				c.JSON(http.StatusTooManyRequests, gin.H{
					"success": false,
					"message": fmt.Sprintf("发送过于频繁，请等待 %d 秒后再试", wait),
				})
				c.Abort()
				return
			}
		} else {
			// New (or expired) window for this IP.
			evRecords[key] = &evEntry{count: 1, expireAt: now.Add(emailVerificationRateLimitWindow)}
			// Opportunistically prune expired entries to bound memory.
			for k, v := range evRecords {
				if !now.Before(v.expireAt) {
					delete(evRecords, k)
				}
			}
		}
		evMu.Unlock()

		c.Next()
	}
}
