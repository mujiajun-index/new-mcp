package router

import (
	"github.com/gin-gonic/gin"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/cloud"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/service"
)

var (
	GatewayHandler *handler.GatewayHandler
	SessionPool    *bridge.SessionPool
	CloudManager   *cloud.Manager
)

func InitGateway() {
	SessionPool = bridge.NewSessionPool()
	toolRouter := bridge.NewToolRouter(SessionPool)
	GatewayHandler = handler.NewGatewayHandler(SessionPool, toolRouter)

	CloudManager = cloud.NewManager(SessionPool, toolRouter)
	service.CloudManager = CloudManager
}

func SetRouter(engine *gin.Engine) {
	InitGateway()
	SetApiRouter(engine)
	SetMCPRouter(engine, GatewayHandler)
}

func StartCloudConnections() {
	if CloudManager != nil {
		CloudManager.StartAll()
	}
}

func StopCloudConnections() {
	if CloudManager != nil {
		CloudManager.StopAll()
	}
}
