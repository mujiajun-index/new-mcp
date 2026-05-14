package cloud

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/model"
)

type Manager struct {
	clients    map[int64]*XiaoZhiClient // key: cloud_endpoint ID
	pool       *bridge.SessionPool
	toolRouter *bridge.ToolRouter
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewManager(pool *bridge.SessionPool, toolRouter *bridge.ToolRouter) *Manager {
	return &Manager{
		clients:    make(map[int64]*XiaoZhiClient),
		pool:       pool,
		toolRouter: toolRouter,
	}
}

// StartAll connects all auto-connect cloud endpoints (async)
func (m *Manager) StartAll() {
	m.ctx, m.cancel = context.WithCancel(context.Background())

	var endpoints []model.CloudEndpoint
	model.DB.Where("auto_connect = ? AND status = ?", true, common.StatusEnabled).
		Find(&endpoints)

	for _, ep := range endpoints {
		ep := ep
		go m.startEndpointAsync(&ep)
	}

	log.Printf("[cloud] started %d auto-connect endpoints", len(endpoints))
}

// StopAll disconnects all cloud endpoints
func (m *Manager) StopAll() {
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.clients {
		c.Disconnect()
		delete(m.clients, id)
	}
}

// StartEndpoint connects a specific endpoint
func (m *Manager) StartEndpoint(ep *model.CloudEndpoint) error {
	return m.startEndpoint(ep)
}

// startEndpointAsync is used by StartAll for background auto-connect (async)
func (m *Manager) startEndpointAsync(ep *model.CloudEndpoint) {
	client := NewXiaoZhiClient(ep, m.pool, m.toolRouter)

	m.mu.Lock()
	m.clients[ep.ID] = client
	m.mu.Unlock()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[cloud] endpoint %d (%s) panic: %v", ep.ID, ep.Name, r)
			client.updateStatus(common.ConnDisconnected, fmt.Sprintf("panic: %v", r))
			m.mu.Lock()
			delete(m.clients, ep.ID)
			m.mu.Unlock()
		}
	}()

	m.connectWithRetry(client)
}

// StopEndpoint disconnects a specific endpoint
func (m *Manager) StopEndpoint(endpointID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[endpointID]; ok {
		c.Disconnect()
		delete(m.clients, endpointID)
	}
}

// RestartEndpoint disconnects and reconnects an endpoint
func (m *Manager) RestartEndpoint(ep *model.CloudEndpoint) error {
	m.StopEndpoint(ep.ID)
	return m.startEndpoint(ep)
}

// GetStatus returns the connection status of an endpoint
func (m *Manager) GetStatus(endpointID int64) (connected bool, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[endpointID]
	if !ok {
		return false, false
	}
	return c.IsConnected(), true
}

func (m *Manager) startEndpoint(ep *model.CloudEndpoint) error {
	switch ep.CloudType {
	case "xiaozhi":
		return m.startXiaoZhi(ep)
	case "custom":
		return m.startCustom(ep)
	default:
		log.Printf("[cloud] unsupported cloud_type: %s for endpoint %d", ep.CloudType, ep.ID)
		return nil
	}
}

func (m *Manager) startXiaoZhi(ep *model.CloudEndpoint) error {
	return m.startClient(ep)
}

func (m *Manager) startCustom(ep *model.CloudEndpoint) error {
	return m.startClient(ep)
}

func (m *Manager) startClient(ep *model.CloudEndpoint) error {
	client := NewXiaoZhiClient(ep, m.pool, m.toolRouter)

	m.mu.Lock()
	m.clients[ep.ID] = client
	m.mu.Unlock()

	firstConnect := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[cloud] endpoint %d (%s) panic: %v", ep.ID, ep.Name, r)
				client.updateStatus(common.ConnDisconnected, fmt.Sprintf("panic: %v", r))
				m.mu.Lock()
				delete(m.clients, ep.ID)
				m.mu.Unlock()
			}
		}()

		// 首次连接尝试
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := client.Connect(ctx)
		cancel()

		if err != nil {
			log.Printf("[cloud] endpoint %d (%s) connect failed: %v", ep.ID, ep.Name, err)
			client.updateStatus(common.ConnDisconnected, err.Error())
			m.mu.Lock()
			delete(m.clients, ep.ID)
			m.mu.Unlock()
			firstConnect <- err
			return
		}

		log.Printf("[cloud] endpoint %d connected", ep.ID)
		firstConnect <- nil

		// goroutine 继续维护连接生命周期
		<-client.Done()
		log.Printf("[cloud] endpoint %d disconnected", ep.ID)
		client.updateStatus(common.ConnDisconnected, "")
		m.mu.Lock()
		delete(m.clients, ep.ID)
		m.mu.Unlock()
	}()

	// 同步等待首次连接结果
	select {
	case err := <-firstConnect:
		return err
	case <-time.After(15 * time.Second):
		return fmt.Errorf("连接超时")
	}
}

// connectWithRetry is used by StartAll for auto-connect endpoints (async)
func (m *Manager) connectWithRetry(client *XiaoZhiClient) {
	backoff := 2 * time.Second
	maxBackoff := 30 * time.Second
	maxRetries := 1

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		err := client.Connect(ctx)
		cancel()

		if err == nil {
			log.Printf("[cloud] endpoint %d connected", client.endpointID)
			<-client.Done()
			log.Printf("[cloud] endpoint %d disconnected, will retry", client.endpointID)
			backoff = 2 * time.Second
		} else {
			log.Printf("[cloud] endpoint %d connect attempt %d failed: %v", client.endpointID, attempt+1, err)
			client.updateStatus(common.ConnDisconnected, err.Error())
		}

		select {
		case <-m.ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	client.updateStatus(common.ConnDisconnected, "max retries exceeded")
}
