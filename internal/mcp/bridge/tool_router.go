package bridge

import (
	"fmt"
	"strings"
)

type ToolRouter struct {
	pool *SessionPool
}

func NewToolRouter(pool *SessionPool) *ToolRouter {
	return &ToolRouter{pool: pool}
}

// Route parses a namespaced tool name and returns the target session and original tool name.
// Direct mode: "serviceName__toolName" (double underscore)
// Smart mode: "serviceName.toolName" (dot)
func (r *ToolRouter) Route(namespacedTool string) (*McpSession, string, error) {
	serviceName, toolName := parseNamespacedName(namespacedTool)
	if serviceName == "" {
		return nil, "", fmt.Errorf("invalid tool name format: %s", namespacedTool)
	}

	session := r.pool.GetByName(serviceName)
	if session == nil {
		return nil, "", fmt.Errorf("service not found: %s", serviceName)
	}
	if !session.Adapter.IsConnected() {
		return nil, "", fmt.Errorf("service not connected: %s", serviceName)
	}
	return session, toolName, nil
}

func parseNamespacedName(name string) (serviceName, toolName string) {
	if idx := strings.Index(name, "__"); idx >= 0 {
		return name[:idx], name[idx+2:]
	}
	if idx := strings.Index(name, "."); idx >= 0 {
		return name[:idx], name[idx+1:]
	}
	return "", name
}
