package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSetApiRouterRegistersWithoutPanic registers the API routes against a bare
// engine. Handlers and middleware are referenced but never invoked, so no DB or
// storage is needed. Its purpose is to catch gin radix-tree conflicts that the
// compiler cannot — e.g. a newly added static segment clashing with an existing
// :param child (such as /vision/upload vs /vision/:id/enable).
func TestSetApiRouterRegistersWithoutPanic(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	SetApiRouter(engine)
}
