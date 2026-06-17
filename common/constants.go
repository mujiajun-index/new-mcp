package common

import "time"

var SystemInitialized = true

// Version 为当前系统版本，可在构建时通过 ldflags 注入：
//
//	go build -ldflags "-X github.com/mujkjk/newmcp/common.Version=vX.Y.Z"
var Version = "v0.0.0"

// StartTime 记录进程启动时间（包级初始化 ≈ 服务启动），单位：秒。
var StartTime = time.Now().Unix()

const (
	// Roles
	RoleAdminUser  = "admin"
	RoleCommonUser = "user"

	// Status
	StatusEnabled  = 1
	StatusDisabled = 2

	// API Key
	ApiKeyPrefix = "sk-"

	// Transport types
	TransportStdio          = "stdio"
	TransportSSE            = "sse"
	TransportStreamableHTTP = "streamable-http"
	TransportWebSocket      = "websocket"
	TransportPassiveWS      = "passive-ws"

	// Expose modes
	ExposeModeDirect = "direct"
	ExposeModeSmart  = "smart"

	// Health status
	HealthHealthy   = "healthy"
	HealthUnhealthy = "unhealthy"
	HealthUnknown   = "unknown"
	HealthDead      = "dead"

	// Connection status
	ConnConnected    = "connected"
	ConnDisconnected = "disconnected"
	ConnConnecting   = "connecting"
	ConnError        = "error"
)
