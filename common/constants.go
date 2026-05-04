package common

const (
	Version = "v1.0.0"

	// Roles
	RoleAdminUser = "admin"
	RoleCommonUser = "user"

	// Status
	StatusEnabled  = 1
	StatusDisabled = 2

	// API Key
	ApiKeyPrefix = "nm-"

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
