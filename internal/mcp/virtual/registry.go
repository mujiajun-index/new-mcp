package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// VirtualToolHandler processes a virtual tool call.
// toolName is the specific tool within the virtual service (e.g. "vision.analyze_image").
type VirtualToolHandler func(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error)

type VirtualToolRegistry struct {
	mu       sync.RWMutex
	handlers map[int64]VirtualToolHandler // key: McpService.ID
}

func NewVirtualToolRegistry() *VirtualToolRegistry {
	return &VirtualToolRegistry{
		handlers: make(map[int64]VirtualToolHandler),
	}
}

func (r *VirtualToolRegistry) Register(serviceID int64, handler VirtualToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[serviceID] = handler
}

func (r *VirtualToolRegistry) Unregister(serviceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.handlers, serviceID)
}

func (r *VirtualToolRegistry) Handle(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error) {
	r.mu.RLock()
	handler, ok := r.handlers[serviceID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("virtual tool handler not found for service %d", serviceID)
	}
	return handler(ctx, serviceID, config, toolName, args)
}

func (r *VirtualToolRegistry) IsVirtual(serviceID int64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[serviceID]
	return ok
}
