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
func (r *ToolRouter) Route(namespacedTool string, userID int64) (*McpSession, string, error) {
	serviceName, toolName := ParseNamespacedName(namespacedTool)
	if serviceName == "" {
		session := r.pool.FindByToolNameForUser(namespacedTool, userID)
		if session != nil {
			if !session.Adapter.IsConnected() {
				return nil, "", fmt.Errorf("service not connected for tool: %s", namespacedTool)
			}
			return session, namespacedTool, nil
		}
		return nil, "", fmt.Errorf("tool not found: %s", namespacedTool)
	}

	session := r.pool.GetByNameForUser(serviceName, userID)
	if session == nil {
		return nil, "", fmt.Errorf("service not found: %s", serviceName)
	}
	if !session.Adapter.IsConnected() {
		return nil, "", fmt.Errorf("service not connected: %s", serviceName)
	}
	return session, toolName, nil
}

func ParseNamespacedName(name string) (serviceName, toolName string) {
	if idx := strings.Index(name, "__"); idx >= 0 {
		return name[:idx], name[idx+2:]
	}
	if idx := strings.Index(name, "."); idx >= 0 {
		return name[:idx], name[idx+1:]
	}
	return "", name
}
