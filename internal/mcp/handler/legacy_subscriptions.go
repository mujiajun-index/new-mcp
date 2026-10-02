package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
)

type legacyRegistry struct {
	mu       sync.Mutex
	sessions map[string]*legacySession
	closed   bool
}

type legacySession struct {
	principal    string
	ctx          context.Context
	cancel       context.CancelFunc
	messages     chan JSONRPCNotification
	mu           sync.Mutex
	resources    map[string]func()
	streamCancel context.CancelFunc
}

func (h *GatewayHandler) CreateLegacySession(logCtx *LogContext) (string, error) {
	h.legacy.mu.Lock()
	defer h.legacy.mu.Unlock()
	if h.legacy.closed {
		return "", fmt.Errorf("gateway is shutting down")
	}
	if h.legacy.sessions == nil {
		h.legacy.sessions = map[string]*legacySession{}
	}
	count := 0
	for _, session := range h.legacy.sessions {
		if session.principal == principalKey(logCtx) {
			count++
		}
	}
	if count >= 32 || len(h.legacy.sessions) >= 1024 {
		return "", fmt.Errorf("too many legacy sessions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	id := uuid.NewString()
	session := &legacySession{principal: principalKey(logCtx), ctx: ctx, cancel: cancel, messages: make(chan JSONRPCNotification, 64), resources: map[string]func(){}}
	h.legacy.sessions[id] = session
	go func() {
		<-ctx.Done()
		h.legacy.mu.Lock()
		delete(h.legacy.sessions, id)
		h.legacy.mu.Unlock()
		session.close()
	}()
	return id, nil
}

func (s *legacySession) close() {
	s.cancel()
	s.mu.Lock()
	stops := s.resources
	s.resources = map[string]func(){}
	if s.streamCancel != nil {
		s.streamCancel()
	}
	s.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

func (h *GatewayHandler) legacySession(logCtx *LogContext) (*legacySession, error) {
	h.legacy.mu.Lock()
	session := h.legacy.sessions[logCtx.SessionID]
	h.legacy.mu.Unlock()
	if session == nil || session.principal != principalKey(logCtx) || session.ctx.Err() != nil {
		return nil, fmt.Errorf("invalid or expired MCP session")
	}
	return session, nil
}

func (h *GatewayHandler) RemoveLegacySession(logCtx *LogContext) error {
	session, err := h.legacySession(logCtx)
	if err != nil {
		return err
	}
	h.legacy.mu.Lock()
	delete(h.legacy.sessions, logCtx.SessionID)
	h.legacy.mu.Unlock()
	session.close()
	return nil
}

func (h *GatewayHandler) ValidateLegacySession(logCtx *LogContext) error {
	_, err := h.legacySession(logCtx)
	return err
}

func (h *GatewayHandler) closeLegacySessions() {
	h.legacy.mu.Lock()
	h.legacy.closed = true
	sessions := h.legacy.sessions
	h.legacy.sessions = map[string]*legacySession{}
	h.legacy.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
}

func (h *GatewayHandler) handleLegacyResourceSubscription(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if !h.nativeItemsAllowed(logCtx) {
		return rpcFailure(req.ID, -32601, "Resources are available through discovery tools in smart mode", nil)
	}
	session, err := h.legacySession(logCtx)
	if err != nil {
		return rpcFailure(req.ID, -32602, err.Error(), nil)
	}
	var p struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(req.Params, &p) != nil || p.URI == "" {
		return rpcFailure(req.ID, -32602, "uri is required", nil)
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if req.Method == "resources/unsubscribe" {
		if stop := session.resources[p.URI]; stop != nil {
			delete(session.resources, p.URI)
			stop()
		}
		return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	}
	if session.resources[p.URI] != nil {
		return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	}
	if len(session.resources) >= 128 {
		return rpcFailure(req.ID, -32013, "Too many resource subscriptions", nil)
	}
	watchCtx := withNotificationFailure(session.ctx, func(error) { session.cancel() })
	accepted, stop, err := h.watchSources(watchCtx, logCtx, mcp.NotificationSubscriptions{ResourceSubscriptions: []string{p.URI}}, func(notification transport.Notification) {
		session.emit(notification)
	})
	if err != nil {
		return rpcFailure(req.ID, -32602, err.Error(), nil)
	}
	if len(accepted.ResourceSubscriptions) == 0 {
		stop()
		return rpcFailure(req.ID, -32014, "Upstream does not support this resource subscription", nil)
	}
	session.resources[p.URI] = stop
	return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
}

func (s *legacySession) emit(notification transport.Notification) {
	params := map[string]any{}
	_ = json.Unmarshal(notification.Params, &params)
	delete(params, "_meta")
	select {
	case s.messages <- JSONRPCNotification{JSONRPC: "2.0", Method: notification.Method, Params: params}:
	case <-s.ctx.Done():
	default:
		s.cancel()
	}
}

func (h *GatewayHandler) OpenLegacyStream(parent context.Context, logCtx *LogContext) (*NotificationStream, error) {
	session, err := h.legacySession(logCtx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	session.mu.Lock()
	if session.streamCancel != nil {
		session.streamCancel()
	}
	session.streamCancel = cancel
	session.mu.Unlock()
	wanted := mcp.NotificationSubscriptions{}
	if h.nativeItemsAllowed(logCtx) {
		wanted = mcp.NotificationSubscriptions{ToolsListChanged: true, PromptsListChanged: true, ResourcesListChanged: true}
	}
	watchCtx := withNotificationFailure(ctx, func(error) { cancel() })
	_, unwatch, err := h.watchSources(watchCtx, logCtx, wanted, session.emit)
	if err != nil {
		cancel()
		return nil, err
	}
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); unwatch() }) }
	monitorDone, err := h.startGatewayTask()
	if err != nil {
		stop()
		return nil, err
	}
	go func() {
		defer monitorDone()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		defer stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-session.ctx.Done():
				cancel()
				return
			case <-ticker.C:
				if h.CheckPrincipal(ctx, logCtx) != nil {
					session.cancel()
					return
				}
			}
		}
	}()
	return &NotificationStream{Messages: session.messages, Stop: stop, Done: ctx.Done()}, nil
}
