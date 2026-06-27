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
	RoleSuperAdmin = "super_admin"
	RoleAdminUser  = "admin"
	RoleCommonUser = "user"

	// Status
	StatusEnabled  = 1
	StatusDisabled = 2

	// SuperAdminUserID 为系统初始化的超级管理员（首个账号，自增主键即 1）。
	// 其角色与启用状态受保护：不可被改角色、禁用或删除，避免管理员被锁在系统之外。
	SuperAdminUserID int64 = 1

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

// IsAdminRole 判断角色是否属于管理员级别（超级管理员或普通管理员），用于所有「仅管理员可访问」的门禁。
func IsAdminRole(role string) bool {
	return role == RoleSuperAdmin || role == RoleAdminUser
}
