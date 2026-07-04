package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

type McpSession struct {
	ServiceID   int64
	ServiceName string
	UserID      int64
	Adapter     transport.TransportAdapter
	Tools       []transport.Tool
	LastUsed    time.Time
	LastRefresh time.Time
	Health      string
	failCount   int
}

type SessionPool struct {
	mu          sync.RWMutex
	sessions    map[int64]*McpSession
	idleTimeout time.Duration
	maxRetries  int
}

func NewSessionPool() *SessionPool {
	return &SessionPool{
		sessions:    make(map[int64]*McpSession),
		idleTimeout: 10 * time.Minute,
		maxRetries:  5,
	}
}

func (p *SessionPool) GetOrConnect(ctx context.Context, svc *model.McpService) (*McpSession, error) {
	p.mu.RLock()
	if session, ok := p.sessions[svc.ID]; ok && session.Adapter.IsConnected() {
		session.LastUsed = time.Now()
		p.mu.RUnlock()
		return session, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Double check after acquiring write lock
	if session, ok := p.sessions[svc.ID]; ok && session.Adapter.IsConnected() {
		session.LastUsed = time.Now()
		return session, nil
	}

	adapter := CreateAdapter(svc)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported transport type: %s", svc.TransportType)
	}

	if err := adapter.Connect(ctx); err != nil {
		return nil, err
	}

	session := &McpSession{
		ServiceID:   svc.ID,
		ServiceName: svc.Name,
		UserID:      svc.UserID,
		Adapter:     adapter,
		Tools:       adapter.GetTools(),
		LastUsed:    time.Now(),
		LastRefresh: time.Now(),
		Health:      "healthy",
	}

	p.sessions[svc.ID] = session

	// Update tools cache in database
	if toolsData, err := json.Marshal(session.Tools); err == nil {
		now := time.Now()
		model.DB.Model(&model.McpService{}).Where("id = ?", svc.ID).Updates(map[string]interface{}{
			"tools_cache":    string(toolsData),
			"tools_updated_at": now,
			"health_status":  "healthy",
		})
	}

	return session, nil
}

func (p *SessionPool) Get(serviceID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[serviceID]
}

func (p *SessionPool) GetByName(serviceName string) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.ServiceName == serviceName {
			return s
		}
	}
	return nil
}

func (p *SessionPool) GetByNameForUser(serviceName string, userID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.ServiceName == serviceName && s.UserID == userID {
			return s
		}
	}
	return nil
}

func (p *SessionPool) Remove(serviceID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.sessions[serviceID]; ok {
		s.Adapter.Close()
		delete(p.sessions, serviceID)
	}
}

func (p *SessionPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, s := range p.sessions {
		s.Adapter.Close()
		delete(p.sessions, id)
	}
}

func (p *SessionPool) FindByToolName(toolName string) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		for _, t := range s.Tools {
			if t.Name == toolName {
				return s
			}
		}
	}
	return nil
}

func (p *SessionPool) FindByToolNameForUser(toolName string, userID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		if s.UserID != userID {
			continue
		}
		for _, t := range s.Tools {
			if t.Name == toolName {
				return s
			}
		}
	}
	return nil
}

func (p *SessionPool) GetAllSessions() []*McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*McpSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		result = append(result, s)
	}
	return result
}

func CreateAdapter(svc *model.McpService) transport.TransportAdapter {
	var config map[string]interface{}
	_ = json.Unmarshal([]byte(svc.Config), &config)

	switch transport.TransportType(svc.TransportType) {
	case transport.TypeStdio:
		cmd, _ := config["command"].(string)
		argsRaw, _ := config["args"].([]interface{})
		args := make([]string, len(argsRaw))
		for i, a := range argsRaw {
			args[i], _ = a.(string)
		}
		env, _ := config["env"].(map[string]interface{})
		envMap := make(map[string]string)
		for k, v := range env {
			envMap[k], _ = v.(string)
		}
		return transport.NewStdioAdapter(svc.ID, cmd, args, envMap)

	case transport.TypeStreamableHTTP:
		url, _ := config["url"].(string)
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewStreamableHTTPAdapter(svc.ID, url, h)

	case transport.TypeSSE:
		url, _ := config["url"].(string)
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewSSEAdapter(svc.ID, url, h)

	default:
		return nil
	}
}
