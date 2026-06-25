package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/model"
)

// GetRequestIP returns the real client IP for the request.
//
// When the site is behind Cloudflare (CloudflareProxyEnabled on, set by the
// admin in System Settings -> 系统维护), Cloudflare records the visitor IP in
// the single-value CF-Connecting-IP header; we prefer it so the recorded IP is
// the real user rather than Cloudflare's edge IP. Otherwise fall back to gin's
// c.ClientIP() (X-Forwarded-For / X-Real-IP / RemoteAddr).
func GetRequestIP(c *gin.Context) string {
	if model.GetOptionBool("CloudflareProxyEnabled") {
		if raw := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); raw != "" {
			// CF-Connecting-IP is a single address; reject anything that isn't a
			// clean single IP (defends against comma lists / garbage).
			if ip := net.ParseIP(raw); ip != nil {
				return ip.String()
			}
		}
	}
	return c.ClientIP()
}
