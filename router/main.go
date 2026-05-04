package router

import (
	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
)

var (
	GatewayHandler *handler.GatewayHandler
	SessionPool    *bridge.SessionPool
)

func InitGateway() {
	SessionPool = bridge.NewSessionPool()
	toolRouter := bridge.NewToolRouter(SessionPool)
	GatewayHandler = handler.NewGatewayHandler(SessionPool, toolRouter)
}

func SetRouter(engine *gin.Engine) {
	InitGateway()
	SetApiRouter(engine)
	SetMCPRouter(engine, GatewayHandler)
}
