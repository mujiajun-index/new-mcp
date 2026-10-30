package events

import (
	"sync"
	"sync/atomic"

	"github.com/mujkjk/newmcp/model"
)

// Subscription coalesces changes for one connected virtual service. Producers
// only signal after committing their database writes; consumers read the latest
// state once per wake instead of polling while the service is idle.
type Subscription struct {
	scope  model.VirtualExecution
	mu     sync.Mutex
	tasks  bool
	config bool
	closed bool
	wake   chan struct{}
}

var bus = struct {
	sync.RWMutex
	listeners map[*Subscription]struct{}
}{listeners: make(map[*Subscription]struct{})}
var generation atomic.Uint64
var invalidation = struct {
	sync.RWMutex
	service     func(int64)
	marketplace func(int64)
}{}

func Generation() uint64 { return generation.Load() }

func Subscribe(scope model.VirtualExecution) *Subscription {
	s := &Subscription{scope: scope, wake: make(chan struct{}, 1)}
	bus.Lock()
	bus.listeners[s] = struct{}{}
	bus.Unlock()
	return s
}

func (s *Subscription) Ready() <-chan struct{} { return s.wake }

func (s *Subscription) CatchUp() { s.signal(true, true) }

func (s *Subscription) Drain() (tasks, config bool) {
	s.mu.Lock()
	tasks, config = s.tasks, s.config
	s.tasks, s.config = false, false
	s.mu.Unlock()
	return
}

func (s *Subscription) Close() {
	bus.Lock()
	delete(bus.listeners, s)
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	bus.Unlock()
}

func (s *Subscription) signal(task, config bool) {
	s.mu.Lock()
	if !s.closed {
		s.tasks = s.tasks || task
		s.config = s.config || config
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	s.mu.Unlock()
}

// TaskChanged is called after a generated task is created, changed, or removed.
// Legacy tasks without a service ID remain visible only to their owner's
// virtual service, never to marketplace installations.
func TaskChanged(kind string, userID, serviceID, configID int64) {
	dispatchTask(kind, userID, serviceID, configID)
	if bridge := activeRedis.Load(); bridge != nil {
		bridge.enqueue(redisChange{Type: "task", Kind: kind, UserID: userID, ServiceID: serviceID, ConfigID: configID})
	}
}

func dispatchTask(kind string, userID, serviceID, configID int64) {
	generation.Add(1)
	bus.RLock()
	for s := range bus.listeners {
		v := s.scope
		if v.Kind == kind && v.UserID == userID && (v.ServiceID == serviceID && serviceID != 0 || serviceID == 0 && v.MarketplaceItemID == nil && v.ConfigID == configID) {
			s.signal(true, false)
		}
	}
	bus.RUnlock()
}

// ConfigChanged refreshes tools for every installation bound to this config.
func ConfigChanged(kind string, configID int64) {
	dispatchConfig(kind, configID)
	if bridge := activeRedis.Load(); bridge != nil {
		bridge.enqueue(redisChange{Type: "config", Kind: kind, ConfigID: configID})
	}
}

func dispatchConfig(kind string, configID int64) {
	generation.Add(1)
	bus.RLock()
	for s := range bus.listeners {
		if s.scope.Kind == kind && s.scope.ConfigID == configID {
			s.signal(false, true)
		}
	}
	bus.RUnlock()
}

func catchUpAll() {
	generation.Add(1)
	bus.RLock()
	for s := range bus.listeners {
		s.CatchUp()
	}
	bus.RUnlock()
}

// RegisterInvalidationHandlers connects service session cleanup without making
// the event package depend on the session pool.
func RegisterInvalidationHandlers(service, marketplace func(int64)) {
	invalidation.Lock()
	invalidation.service = service
	invalidation.marketplace = marketplace
	invalidation.Unlock()
}

func ServiceChanged(serviceID int64) {
	dispatchService(serviceID)
	if bridge := activeRedis.Load(); bridge != nil {
		bridge.enqueue(redisChange{Type: "service", ServiceID: serviceID})
	}
}

func MarketplaceChanged(itemID int64) {
	dispatchMarketplace(itemID)
	if bridge := activeRedis.Load(); bridge != nil {
		bridge.enqueue(redisChange{Type: "marketplace", MarketplaceItemID: itemID})
	}
}

func dispatchService(serviceID int64) {
	generation.Add(1)
	bus.RLock()
	for s := range bus.listeners {
		if s.scope.ServiceID == serviceID {
			s.CatchUp()
		}
	}
	bus.RUnlock()
	invalidation.RLock()
	handler := invalidation.service
	invalidation.RUnlock()
	if handler != nil {
		handler(serviceID)
	}
}

func dispatchMarketplace(itemID int64) {
	generation.Add(1)
	bus.RLock()
	for s := range bus.listeners {
		if s.scope.MarketplaceItemID != nil && *s.scope.MarketplaceItemID == itemID {
			s.CatchUp()
		}
	}
	bus.RUnlock()
	invalidation.RLock()
	handler := invalidation.marketplace
	invalidation.RUnlock()
	if handler != nil {
		handler(itemID)
	}
}
