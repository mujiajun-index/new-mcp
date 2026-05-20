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

type ServiceEntry struct {
	UserID  int64
	Name    string
	Config  map[string]interface{}
	handler VirtualToolHandler
}

type VirtualToolRegistry struct {
	mu        sync.RWMutex
	entries   map[int64]*ServiceEntry // key: McpService.ID
	nameIndex map[string]int64        // key: "userID:serviceName"
}

func NewVirtualToolRegistry() *VirtualToolRegistry {
	return &VirtualToolRegistry{
		entries:   make(map[int64]*ServiceEntry),
		nameIndex: make(map[string]int64),
	}
}

func nameKey(userID int64, name string) string {
	return fmt.Sprintf("%d:%s", userID, name)
}

func (r *VirtualToolRegistry) Register(serviceID, userID int64, serviceName string, config map[string]interface{}, handler VirtualToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[serviceID] = &ServiceEntry{
		UserID:  userID,
		Name:    serviceName,
		Config:  config,
		handler: handler,
	}
	r.nameIndex[nameKey(userID, serviceName)] = serviceID
}

func (r *VirtualToolRegistry) Unregister(serviceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.entries[serviceID]; ok {
		delete(r.nameIndex, nameKey(entry.UserID, entry.Name))
	}
	delete(r.entries, serviceID)
}

func (r *VirtualToolRegistry) Handle(ctx context.Context, serviceID int64, config map[string]interface{}, toolName string, args json.RawMessage) (json.RawMessage, error) {
	r.mu.RLock()
	entry, ok := r.entries[serviceID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("virtual tool handler not found for service %d", serviceID)
	}

	cfg := config
	if cfg == nil {
		cfg = entry.Config
	}
	return entry.handler(ctx, serviceID, cfg, toolName, args)
}

// LookupByName finds a virtual service by userID + serviceName without DB access.
func (r *VirtualToolRegistry) LookupByName(userID int64, serviceName string) (int64, *ServiceEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	svcID, ok := r.nameIndex[nameKey(userID, serviceName)]
	if !ok {
		return 0, nil, false
	}
	entry := r.entries[svcID]
	return svcID, entry, true
}

func (r *VirtualToolRegistry) IsVirtual(serviceID int64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.entries[serviceID]
	return ok
}

// ParseConfig is a helper to unmarshal config JSON.
func ParseConfig(raw string) map[string]interface{} {
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(raw), &cfg)
	return cfg
}
