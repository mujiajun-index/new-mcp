package middleware

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

// rateLimitEntry is the in-memory counter for one (user, group) within its
// current window.
type rateLimitEntry struct {
	count    int
	expireAt time.Time
}

var (
	rlMu      sync.Mutex
	rlRecords = make(map[string]*rateLimitEntry)
)

// rateLimitRule is the resolved limit applied to a request.
type rateLimitRule struct {
	max    int
	window time.Duration
}

// resolveRateLimitRule picks the per-group override when present, otherwise the
// global default. A max <= 0 means "no limit" for that scope. The returned
// source string ("group(<name>)" or "global") is for diagnostics.
func resolveRateLimitRule(group string) (rateLimitRule, string) {
	if cfg := model.GetRateLimitGroupConfig(); cfg != nil {
		if r, ok := cfg[group]; ok && r.Max > 0 {
			return rateLimitRule{max: r.Max, window: windowDuration(r.WindowMinutes)}, fmt.Sprintf("group(%s)", group)
		}
	}
	return rateLimitRule{
		max:    model.GetOptionInt("RateLimitMaxRequests"),
		window: windowDuration(model.GetOptionInt("RateLimitWindowMinutes")),
	}, "global"
}

// windowDuration converts a minutes value (from settings) to a Duration,
// clamping non-positive values to 1 minute.
func windowDuration(minutes int) time.Duration {
	if minutes <= 0 {
		minutes = 1
	}
	return time.Duration(minutes) * time.Minute
}

// peekRPCMethod reads the JSON-RPC "method" from the request body and restores
// the body so downstream handlers can read it again. Returns "" if the body
// cannot be read or has no method.
func peekRPCMethod(c *gin.Context) string {
	if c.Request == nil || c.Request.Body == nil {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return ""
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var m struct {
		Method string `json:"method"`
	}
	_ = common.Unmarshal(body, &m)
	return m.Method
}

// RateLimit enforces the configurable per-user-group request rate on the MCP
// relay path. It must be mounted AFTER APIKeyAuth, which sets "api_key_user_id"
// and "user_group" in the context.
//
// Only actual tool invocations (JSON-RPC method "tools/call") consume the
// quota. The MCP handshake and discovery traffic that a client sends on connect
// — "initialize", "notifications/*", "tools/list" — passes through free, so the
// configured limit reflects real tool usage rather than protocol chatter.
// Storage is in-memory (single instance); the counter is keyed per user+group
// and uses a rolling window like the email verification limiter.
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !model.GetOptionBool("RateLimitEnabled") {
			c.Next()
			return
		}
		userID, ok := c.Get("api_key_user_id")
		if !ok {
			// No authenticated user identity to limit by — let it through.
			c.Next()
			return
		}

		// Only count real work. Everything else (handshake, notifications,
		// discovery, invalid methods) is free.
		if peekRPCMethod(c) != "tools/call" {
			c.Next()
			return
		}

		group, _ := c.Get("user_group")
		groupStr, _ := group.(string)

		rule, source := resolveRateLimitRule(groupStr)
		if rule.max <= 0 {
			c.Next()
			return
		}

		key := fmt.Sprintf("u:%v:g:%s", userID, groupStr)
		now := time.Now()

		count := 1
		blocked := false
		wait := 0

		rlMu.Lock()
		if e, exists := rlRecords[key]; exists && now.Before(e.expireAt) {
			e.count++
			count = e.count
			if e.count > rule.max {
				blocked = true
				wait = int(e.expireAt.Sub(now).Seconds())
				if wait < 1 {
					wait = 1
				}
			}
		} else {
			// New (or expired) window for this key.
			rlRecords[key] = &rateLimitEntry{count: 1, expireAt: now.Add(rule.window)}
			// Opportunistically prune expired entries to bound memory.
			for k, v := range rlRecords {
				if !now.Before(v.expireAt) {
					delete(rlRecords, k)
				}
			}
		}
		rlMu.Unlock()

		// Temporary diagnostic: shows the running count vs the resolved limit
		// for actual tool calls. Remove once confirmed.
		if blocked {
			log.Printf("[rate-limit] BLOCKED user=%v group=%q source=%s count=%d/%d wait=%ds",
				userID, groupStr, source, count, rule.max, wait)
			c.Header("Retry-After", fmt.Sprintf("%d", wait))
			c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", rule.max))
			common.Error(c, http.StatusTooManyRequests,
				fmt.Sprintf("请求过于频繁，请等待 %d 秒后再试", wait))
			c.Abort()
			return
		}
		log.Printf("[rate-limit] tools/call user=%v group=%q source=%s count=%d/%d",
			userID, groupStr, source, count, rule.max)

		c.Next()
	}
}
