package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-API-Key", "MCP-Protocol-Version", "Mcp-Method", "Mcp-Name", "Mcp-Session-Id", "Last-Event-ID"},
		ExposeHeaders:    []string{"Content-Length", "Mcp-Session-Id", "MCP-Protocol-Version"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}
	standard := cors.New(config)
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			requestHeaders := strings.Split(c.GetHeader("Access-Control-Request-Headers"), ",")
			if len(requestHeaders) <= 64 {
				dynamic := config
				dynamic.AllowHeaders = append([]string(nil), config.AllowHeaders...)
				for _, header := range requestHeaders {
					if strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "mcp-param-") {
						dynamic.AllowHeaders = append(dynamic.AllowHeaders, strings.TrimSpace(header))
					}
				}
				cors.New(dynamic)(c)
				return
			}
		}
		standard(c)
	}
}
