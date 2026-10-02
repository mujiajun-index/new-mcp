package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/installer"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

type McpSession struct {
	key         sessionKey
	ServiceID   int64
	ServiceName string
	UserID      int64
	// 商业化:服务来源(user/admin/marketplace)。计费 hook 据此判定是否扣费(§6)。
	Source            string
	MarketplaceItemID *int64
	Adapter           transport.TransportAdapter
	Tools             []transport.Tool
	toolsMu           sync.RWMutex
	refreshMu         sync.Mutex
	LastUsed          time.Time
	LastRefresh       time.Time
	Health            string
	failCount         int
	useMu             sync.Mutex
	inUse             int
	catalogCtx        context.Context
	catalogCancel     context.CancelFunc
	catalogDone       chan struct{}
	catalogMu         sync.Mutex
	catalogPending    catalogRefresh
	catalogWake       chan struct{}
}

type catalogRefresh uint8

const (
	catalogTools catalogRefresh = 1 << iota
	catalogResources
	catalogPrompts
)

// ErrServiceBusy 共享市场 stdio 会话并发达到上限时返回。调用方应转成
// "负载较高,请稍后重试"的友好响应(errors.Is 判定)。
var ErrServiceBusy = errors.New("当前服务负载较高，请稍后重试")

var ErrPassiveNotConnected = errors.New("等待本地 MCP 服务接入")
var ErrSessionPoolClosed = errors.New("MCP session pool is closed")

type toolRefresher interface {
	RefreshTools(context.Context) error
}

type toolsChangeObserver interface {
	SetToolsChangedHandler(func())
}

type sessionDoneObserver interface {
	Done() <-chan struct{}
}

// keyedMutex 按 sessionKey 键控的建连互斥锁,引用计数归零后从池中摘除。
type keyedMutex struct {
	mu  sync.Mutex
	ref int
}

type SessionPool struct {
	mu         sync.RWMutex
	sessions   map[sessionKey]*McpSession
	maxRetries int
	// connMu/connectLocks: per-key 建连锁。慢启动(npx 冷下载可达数十秒)只
	// 排队同键请求,不再持整池写锁卡住其他服务的会话查找与建连。
	connMu       sync.Mutex
	connectLocks map[sessionKey]*keyedMutex
	closed       bool
}

// sessionKey 会话池键:默认按服务行键控(自有服务/独占市场引用一行一会话);
// 共享市场 stdio 条目按条目键控(itemID≠0,serviceID 恒 0),全部安装用户复用
// 同一条目会话与子进程。svc.SharedProcess 由 materialize 从条目 isolated_process
// 反算(仅市场 stdio 条目会置 true),市场行必先 materialize 再入池,键控可靠。
type sessionKey struct {
	serviceID int64
	itemID    int64
}

func sessionKeyFor(svc *model.McpService) sessionKey {
	if svc.SharedProcess && svc.MarketplaceItemID != nil {
		return sessionKey{itemID: *svc.MarketplaceItemID}
	}
	return sessionKey{serviceID: svc.ID}
}

func NewSessionPool() *SessionPool {
	return &SessionPool{
		sessions:     make(map[sessionKey]*McpSession),
		maxRetries:   5,
		connectLocks: make(map[sessionKey]*keyedMutex),
	}
}

func (p *SessionPool) GetOrConnect(ctx context.Context, svc *model.McpService) (*McpSession, error) {
	key := sessionKeyFor(svc)
	if session := p.connectedSession(key); session != nil {
		return session, nil
	}
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return nil, ErrSessionPoolClosed
	}
	if svc.TransportType == common.TransportPassiveWS {
		return nil, ErrPassiveNotConnected
	}

	// 同键建连互斥:拿到 key 锁后 double check,仍无会话才真正建连。
	connLock := p.lockConnect(key)
	defer p.unlockConnect(key, connLock)

	if session := p.connectedSession(key); session != nil {
		return session, nil
	}

	adapter := CreateAdapter(svc)
	if adapter == nil {
		return nil, fmt.Errorf("unsupported transport type: %s", svc.TransportType)
	}

	if err := adapter.Connect(ctx); err != nil {
		_ = adapter.Close()
		// 多秘钥服务建连失败(首把 key 可能已被 401 熔断)时换 key 立即重试一次:
		// OnAuthFailure 已在 RoundTripper 里禁掉坏 key,新 adapter 的 initialize
		// 会选下一把。非鉴权性失败重试一次也无害。
		reconnected := false
		if KeySelectors.Get(svc) != nil {
			if retry := CreateAdapter(svc); retry != nil {
				if err2 := retry.Connect(ctx); err2 == nil {
					adapter = retry
					reconnected = true
				} else {
					_ = retry.Close()
				}
			}
		}
		if !reconnected {
			return nil, err
		}
	}

	session := newSession(svc, adapter)
	if err := p.publishSessionLocked(session); err != nil {
		_ = adapter.Close()
		return nil, err
	}
	return session, nil
}

func newSession(svc *model.McpService, adapter transport.TransportAdapter) *McpSession {
	catalogCtx, catalogCancel := context.WithCancel(context.Background())
	return &McpSession{
		key:               sessionKeyFor(svc),
		ServiceID:         svc.ID,
		ServiceName:       svc.Name,
		UserID:            svc.UserID,
		Source:            svc.Source,
		MarketplaceItemID: svc.MarketplaceItemID,
		Adapter:           adapter,
		Tools:             cloneTools(adapter.GetTools()),
		LastUsed:          time.Now(),
		LastRefresh:       time.Now(),
		Health:            "healthy",
		catalogCtx:        catalogCtx,
		catalogCancel:     catalogCancel,
		catalogDone:       make(chan struct{}),
		catalogWake:       make(chan struct{}, 1),
	}
}

// WithServiceLock serializes service credentials/status mutations with passive
// attachment and cache publication. Remove does not take this lock itself.
func (p *SessionPool) WithServiceLock(serviceID int64, fn func() error) error {
	return p.withSessionLock(sessionKey{serviceID: serviceID}, fn)
}

func (p *SessionPool) withSessionLock(key sessionKey, fn func() error) error {
	km := p.lockConnect(key)
	defer p.unlockConnect(key, km)
	return fn()
}

// AttachPassiveLocked publishes an already initialized inbound MCP server. The
// caller must hold WithServiceLock and revalidate the current token/status in
// that lock immediately before calling this method.
func (p *SessionPool) AttachPassiveLocked(svc *model.McpService, adapter transport.TransportAdapter) (*McpSession, error) {
	if svc.TransportType != common.TransportPassiveWS || svc.Status != common.StatusEnabled {
		return nil, errors.New("被动 MCP 服务不存在或已禁用")
	}
	if adapter == nil || adapter.GetType() != transport.TypePassiveWS || !adapter.IsConnected() {
		return nil, ErrPassiveNotConnected
	}
	session := newSession(svc, adapter)
	if err := p.publishSessionLocked(session); err != nil {
		return nil, err
	}
	return session, nil
}

// publishSessionLocked requires the session's connection lock. Adapter.Close
// always runs outside the map lock; observers may themselves inspect the pool.
func (p *SessionPool) publishSessionLocked(session *McpSession) error {
	passive := session.Adapter.GetType() == transport.TypePassiveWS
	var updates map[string]interface{}
	if passive {
		var err error
		updates, err = sessionRowUpdates(session, session.ToolsSnapshot())
		if err != nil {
			return err
		}
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrSessionPoolClosed
	}
	if passive {
		if !session.Adapter.IsConnected() {
			p.mu.Unlock()
			return ErrPassiveNotConnected
		}
		// The old session stays current until the replacement's catalog is
		// durable. The map lock also excludes shutdown while committing it.
		if err := persistSessionUpdates(session, updates); err != nil {
			p.mu.Unlock()
			return err
		}
	}
	old := p.sessions[session.key]
	p.sessions[session.key] = session
	p.mu.Unlock()
	if old != nil && old != session {
		closeSession(old)
	}

	// Shared prewarm sessions have no own service row. Their resource/prompt
	// catalogs are published to installation rows by the catalog observer.
	if session.ServiceID != 0 {
		if !passive {
			if err := p.updateSessionRowLocked(session); err != nil {
				log.Printf("[mcp] service %d catalog persistence failed: %v", session.ServiceID, err)
			}
		}
	}
	p.observeSession(session)
	return nil
}

func cloneTools(tools []transport.Tool) []transport.Tool {
	out := make([]transport.Tool, len(tools))
	copy(out, tools)
	for i := range out {
		out[i].InputSchema = append(json.RawMessage(nil), out[i].InputSchema...)
	}
	return out
}

// ToolsSnapshot returns a detached copy safe for concurrent list-change events.
func (s *McpSession) ToolsSnapshot() []transport.Tool {
	s.toolsMu.RLock()
	defer s.toolsMu.RUnlock()
	return cloneTools(s.Tools)
}

func (s *McpSession) setTools(tools []transport.Tool) {
	s.toolsMu.Lock()
	s.Tools = cloneTools(tools)
	s.toolsMu.Unlock()
}

func (p *SessionPool) observeSession(session *McpSession) {
	if observer, ok := session.Adapter.(toolsChangeObserver); ok {
		observer.SetToolsChangedHandler(func() {
			// The SDK has already refreshed its snapshot. Coalesce invalidations
			// without waiting for a service lock or spawning a goroutine per event.
			session.queueCatalogRefresh(catalogTools)
		})
	}
	go p.observeCatalog(session)
	if observer, ok := session.Adapter.(sessionDoneObserver); ok {
		go func() {
			select {
			case <-observer.Done():
			case <-session.catalogCtx.Done():
				return
			}
			_ = p.withSessionLock(session.key, func() error {
				p.removeIfCurrent(session)
				return nil
			})
		}()
	}
}

func (s *McpSession) queueCatalogRefresh(items catalogRefresh) {
	if s.catalogCtx.Err() != nil {
		return
	}
	s.catalogMu.Lock()
	s.catalogPending |= items
	s.catalogMu.Unlock()
	select {
	case s.catalogWake <- struct{}{}:
	default:
	}
}

// observeCatalog owns one worker and one list-only watcher per pooled session.
// Resource URI subscriptions are deliberately left to the requesting streams.
func (p *SessionPool) observeCatalog(session *McpSession) {
	defer close(session.catalogDone)
	if watcher, ok := session.Adapter.(transport.NotificationWatcher); ok {
		// The watch retains the session lifetime rather than a setup deadline.
		// Only cancel this watch if registration exceeds its setup budget.
		watchCtx, cancelWatch := context.WithCancel(session.catalogCtx)
		defer cancelWatch()
		timer := time.AfterFunc(15*time.Second, cancelWatch)
		stop, err := watcher.WatchNotifications(watchCtx, nil, func(n transport.Notification) {
			switch n.Method {
			case "notifications/resources/list_changed":
				session.queueCatalogRefresh(catalogResources)
			case "notifications/prompts/list_changed":
				session.queueCatalogRefresh(catalogPrompts)
			}
		})
		timer.Stop()
		if stop != nil {
			defer stop()
		}
		if err != nil && session.catalogCtx.Err() == nil {
			log.Printf("[mcp] service %d catalog notification watch failed: %v", session.ServiceID, err)
		}
	}
	if session.ServiceID != 0 || session.key.itemID != 0 {
		session.queueCatalogRefresh(catalogResources | catalogPrompts)
	}
	retryDelay := time.Second
	for {
		select {
		case <-session.catalogCtx.Done():
			return
		case <-session.catalogWake:
		}
		if session.catalogCtx.Err() != nil {
			return
		}
		session.catalogMu.Lock()
		items := session.catalogPending
		session.catalogPending = 0
		session.catalogMu.Unlock()
		if retry := p.refreshCatalogWhenLeased(session, items); retry != 0 {
			session.queueCatalogRefresh(retry)
			timer := time.NewTimer(retryDelay)
			select {
			case <-session.catalogCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if retryDelay < 30*time.Second {
				retryDelay *= 2
				if retryDelay > 30*time.Second {
					retryDelay = 30 * time.Second
				}
			}
		} else {
			retryDelay = time.Second
		}
	}
}

func (p *SessionPool) refreshCatalogWhenLeased(session *McpSession, items catalogRefresh) catalogRefresh {
	// Background invalidations have the same shared-process concurrency budget
	// as requests and never hold a lease for the lifetime of the watcher.
	release, ok, err := p.AcquireSession(session)
	if errors.Is(err, ErrServiceBusy) {
		return items
	}
	if err != nil || !ok {
		return 0
	}
	defer release()
	session.refreshMu.Lock()
	defer session.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(session.catalogCtx, 15*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return 0
	}
	var retry catalogRefresh
	if items&catalogTools != 0 {
		if err := p.withSessionLock(session.key, func() error { return p.syncToolsLocked(session) }); err != nil && ctx.Err() == nil {
			log.Printf("[mcp] service %d tools cache refresh failed: %v", session.ServiceID, err)
			retry |= catalogTools
		}
	}
	updates := make(map[string]interface{}, 2)
	if items&catalogResources != 0 && (session.ServiceID != 0 || session.key.itemID != 0) {
		if data, err := fetchResourcesCacheResult(ctx, session.Adapter); err == nil {
			updates["resources_cache"] = data
		} else if ctx.Err() == nil {
			log.Printf("[mcp] service %d resources cache refresh failed: %v", session.ServiceID, err)
			retry |= catalogResources
		}
	}
	if items&catalogPrompts != 0 && (session.ServiceID != 0 || session.key.itemID != 0) {
		if data, err := fetchPromptsCacheResult(ctx, session.Adapter); err == nil {
			updates["prompts_cache"] = data
		} else if ctx.Err() == nil {
			log.Printf("[mcp] service %d prompts cache refresh failed: %v", session.ServiceID, err)
			retry |= catalogPrompts
		}
	}
	if len(updates) != 0 && ctx.Err() == nil {
		if err := p.withSessionLock(session.key, func() error { return p.writeCatalogUpdatesLocked(ctx, session, updates) }); err != nil && ctx.Err() == nil {
			log.Printf("[mcp] service %d catalog cache persistence failed: %v", session.ServiceID, err)
			retry |= items & (catalogResources | catalogPrompts)
		}
	}
	if session.catalogCtx.Err() != nil {
		return 0
	}
	if ctx.Err() != nil {
		// A bounded network attempt may expire while the session remains live.
		// Keep its invalidation pending for the same worker's next attempt.
		return items
	}
	return retry
}

func (p *SessionPool) removeIfCurrent(session *McpSession) {
	p.mu.Lock()
	if p.sessions[session.key] != session {
		p.mu.Unlock()
		return
	}
	delete(p.sessions, session.key)
	p.markPassiveOfflineLocked(session)
	p.mu.Unlock()
	closeSession(session)
}

// connectedSession 返回键控会话(须已连接),命中即刷新使用时间;无可用会话返回 nil。
func (p *SessionPool) connectedSession(key sessionKey) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if session, ok := p.sessions[key]; ok && session.Adapter.IsConnected() {
		session.markUsed()
		return session
	}
	return nil
}

// lockConnect/unlockConnect 维护 per-key 建连互斥:引用计数归零即从 map 摘除,
// 防止长期运行下 map 无界增长。已持有锁引用的等待者不受摘除影响(操作的是同一把锁)。
func (p *SessionPool) lockConnect(key sessionKey) *keyedMutex {
	p.connMu.Lock()
	km := p.connectLocks[key]
	if km == nil {
		km = &keyedMutex{}
		p.connectLocks[key] = km
	}
	km.ref++
	p.connMu.Unlock()
	km.mu.Lock()
	return km
}

func (p *SessionPool) unlockConnect(key sessionKey, km *keyedMutex) {
	p.connMu.Lock()
	km.ref--
	if km.ref == 0 {
		delete(p.connectLocks, key)
	}
	p.connMu.Unlock()
	km.mu.Unlock()
}

// AcquireSession holds a lease for a session previously returned by this pool.
// A (nil,false,nil) result means it was concurrently removed and the caller
// should retry through its normal connect path; (nil,true,err) with
// errors.Is(err, ErrServiceBusy) means the shared stdio concurrency limit
// was hit and the caller should fail with a friendly retry-later response.
func (p *SessionPool) AcquireSession(session *McpSession) (func(), bool, error) {
	if session == nil {
		return nil, false, nil
	}
	return p.acquireExisting(session.key, session)
}

// Acquire connects a service if necessary and holds a usage lease until the
// returned release function is called. The lease prevents idle cleanup from
// closing a platform stdio process while an upstream request is in flight.
func (p *SessionPool) Acquire(ctx context.Context, svc *model.McpService) (*McpSession, func(), error) {
	for i := 0; i < 2; i++ {
		session, err := p.GetOrConnect(ctx, svc)
		if err != nil {
			return nil, nil, err
		}
		release, ok, aerr := p.acquireExisting(sessionKeyFor(svc), session)
		if aerr != nil {
			return nil, nil, aerr // ErrServiceBusy 不可重试,直接上抛
		}
		if ok {
			return session, release, nil
		}
	}
	return nil, nil, fmt.Errorf("service session was released while acquiring")
}

func (p *SessionPool) acquireExisting(key sessionKey, want *McpSession) (func(), bool, error) {
	p.mu.RLock()
	session, ok := p.sessions[key]
	if !ok || session != want || !session.Adapter.IsConnected() {
		p.mu.RUnlock()
		return nil, false, nil
	}
	session.useMu.Lock()
	if err := session.busyLocked(); err != nil {
		session.useMu.Unlock()
		p.mu.RUnlock()
		return nil, true, err
	}
	session.inUse++
	session.LastUsed = time.Now()
	session.useMu.Unlock()
	p.mu.RUnlock()
	return func() {
		session.useMu.Lock()
		if session.inUse > 0 {
			session.inUse--
		}
		session.LastUsed = time.Now()
		session.useMu.Unlock()
	}, true, nil
}

// busyLocked 报告共享市场 stdio 会话的租用是否已达并发上限(须持 useMu 调用)。
// 仅条目键控的共享会话受限(独占/自有 stdio 每用户一进程,天然低并发);上限每次
// 实时读配置 SharedStdioMaxConcurrency,0 或负数 = 不限,管理端改动即时生效。
// 超限快速失败(不排队),由调用方转成友好重试提示。
func (s *McpSession) busyLocked() error {
	if s.key.itemID == 0 {
		return nil
	}
	limit := model.GetOptionInt("SharedStdioMaxConcurrency")
	if limit <= 0 || s.inUse < limit {
		return nil
	}
	return ErrServiceBusy
}

func (s *McpSession) markUsed() {
	s.useMu.Lock()
	s.LastUsed = time.Now()
	s.useMu.Unlock()
}

// StartIdleReaper checks every minute for idle platform-managed stdio sessions.
// timeout is read for every sweep so an administrator's setting takes effect
// without restarting the server.
func (p *SessionPool) StartIdleReaper(ctx context.Context, timeout func() time.Duration) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.ReleaseIdlePlatformStdio(timeout())
		}
	}
}

// ReleaseIdlePlatformStdio removes only marketplace-owned stdio sessions that
// have no active lease. Adapters are closed after releasing the pool lock;
// 每个被释放的进程记一条日志(服务/条目标识 + 空闲时长),便于运维对账。
func (p *SessionPool) ReleaseIdlePlatformStdio(timeout time.Duration) int {
	if timeout <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-timeout)
	var victims []*McpSession
	p.mu.Lock()
	for key, session := range p.sessions {
		if session.Source != "marketplace" || session.Adapter.GetType() != transport.TypeStdio {
			continue
		}
		session.useMu.Lock()
		idle := session.inUse == 0 && session.LastUsed.Before(cutoff)
		session.useMu.Unlock()
		if !idle {
			continue
		}
		delete(p.sessions, key)
		victims = append(victims, session)
	}
	p.mu.Unlock()
	for _, session := range victims {
		// 会话已出池且无人能再租到,LastUsed 可直接读
		idle := time.Since(session.LastUsed).Round(time.Minute)
		target := fmt.Sprintf("service %d", session.ServiceID)
		if session.key.itemID != 0 {
			target = fmt.Sprintf("shared item %d", session.key.itemID)
		}
		log.Printf("[idle-reaper] released marketplace stdio process %q (%s) after %s idle", session.ServiceName, target, idle)
		if err := closeSession(session); err != nil {
			log.Printf("[idle-reaper] close %q (%s) failed: %v", session.ServiceName, target, err)
		}
	}
	return len(victims)
}

// updateSessionRowLocked requires the session's connection lock and only writes
// while the exact session is still current. Replaced sockets cannot overwrite
// their successor's tools or connection state.
func (p *SessionPool) updateSessionRowLocked(session *McpSession) error {
	updates, err := sessionRowUpdates(session, session.ToolsSnapshot())
	if err != nil {
		return err
	}
	return p.writeSessionUpdatesLocked(session, updates)
}

func sessionRowUpdates(session *McpSession, tools []transport.Tool) (map[string]interface{}, error) {
	adapter := session.Adapter
	now := time.Now()
	toolsData, err := json.Marshal(tools)
	if err != nil {
		return nil, fmt.Errorf("encode MCP tools catalog: %w", err)
	}
	updates := map[string]interface{}{
		"tools_updated_at": now,
		"health_status":    "healthy",
		"tools_cache":      string(toolsData),
	}
	if adapter.GetType() == transport.TypePassiveWS {
		updates["passive_connected"] = true
	}
	// 上游握手拿到的真实协议版本/serverInfo 一并落库;拿不到时不动旧值
	if v := adapter.GetProtocolVersion(); v != "" {
		updates["protocol_version"] = v
	}
	if si := adapter.GetServerInfo(); si != nil {
		if b, err := json.Marshal(si); err == nil {
			updates["server_info"] = string(b)
		}
	}
	return updates, nil
}

func persistSessionUpdates(session *McpSession, updates map[string]interface{}) error {
	if session.ServiceID == 0 {
		return nil // shared marketplace stdio prewarm has no service row
	}
	if model.DB == nil {
		return errors.New("MCP service database is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result := model.DB.WithContext(ctx).Model(&model.McpService{}).Where("id = ?", session.ServiceID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("persist MCP service catalog: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// MySQL may report no changed rows for an identical snapshot. Distinguish
		// that from a deleted service before rejecting the publication.
		var count int64
		if err := model.DB.WithContext(ctx).Model(&model.McpService{}).
			Where("id = ?", session.ServiceID).Count(&count).Error; err != nil {
			return fmt.Errorf("check MCP service catalog row: %w", err)
		}
		if count == 0 {
			return errors.New("MCP service row no longer exists")
		}
	}
	return nil
}

func (p *SessionPool) writeSessionUpdatesLocked(session *McpSession, updates map[string]interface{}) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed || p.sessions[session.key] != session || !session.Adapter.IsConnected() {
		return errors.New("MCP service session was replaced or disconnected")
	}
	return persistSessionUpdates(session, updates)
}

// Shared marketplace processes serve many installation rows, including sessions
// prewarmed with ServiceID=0. Publish only public catalogs to their enabled
// references; the platform's connection configuration never belongs in them.
func (p *SessionPool) writeCatalogUpdatesLocked(ctx context.Context, session *McpSession, updates map[string]interface{}) error {
	if session.key.itemID == 0 {
		return p.writeSessionUpdatesLocked(session, updates)
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed || p.sessions[session.key] != session || !session.Adapter.IsConnected() || ctx.Err() != nil {
		return errors.New("MCP service session was replaced or disconnected")
	}
	if model.DB == nil {
		return errors.New("MCP service database is unavailable")
	}
	// This helper may be reused by manual cache refresh, so allowlist its fields.
	catalogs := make(map[string]interface{}, 2)
	for _, field := range []string{"resources_cache", "prompts_cache"} {
		if value, ok := updates[field]; ok {
			catalogs[field] = value
		}
	}
	return model.DB.WithContext(ctx).Model(&model.McpService{}).
		Where("marketplace_item_id = ? AND source = ? AND status = ?", session.key.itemID, "marketplace", common.StatusEnabled).
		Updates(catalogs).Error
}

func (p *SessionPool) syncToolsLocked(session *McpSession) error {
	p.mu.RLock()
	current := !p.closed && p.sessions[session.key] == session
	p.mu.RUnlock()
	if !current || !session.Adapter.IsConnected() {
		return nil // late notifications from a detached session are harmless
	}
	tools := session.Adapter.GetTools()
	updates, err := sessionRowUpdates(session, tools)
	if err != nil {
		return err
	}
	if err := p.writeSessionUpdatesLocked(session, updates); err != nil {
		return err
	}
	session.setTools(tools)
	session.useMu.Lock()
	session.LastRefresh = time.Now()
	session.useMu.Unlock()
	return nil
}

// RefreshTools rediscovers tools on an existing connection and publishes the
// snapshot only if that connection is still current. Call outside WithServiceLock.
func (p *SessionPool) RefreshTools(ctx context.Context, session *McpSession) error {
	if session == nil {
		return errors.New("MCP service session is unavailable")
	}
	session.refreshMu.Lock()
	defer session.refreshMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if refresher, ok := session.Adapter.(toolRefresher); ok {
		if err := refresher.RefreshTools(ctx); err != nil {
			return err
		}
	}
	var current bool
	err := p.withSessionLock(session.key, func() error {
		p.mu.RLock()
		current = !p.closed && p.sessions[session.key] == session && session.Adapter.IsConnected()
		p.mu.RUnlock()
		if !current {
			return errors.New("MCP service session was replaced or disconnected")
		}
		return p.syncToolsLocked(session)
	})
	if err != nil {
		return err
	}
	p.refreshItemCachesLocked(ctx, session)
	return nil
}

// RefreshItemCaches 拉取上游 resources/templates/prompts 并回写 mcp_services 缓存列。
// 服务详情/分组勾选 UI 读这些缓存;上游未声明能力时对应列表为空。
func (p *SessionPool) RefreshItemCaches(ctx context.Context, session *McpSession) {
	if session == nil {
		return
	}
	session.refreshMu.Lock()
	defer session.refreshMu.Unlock()
	p.refreshItemCachesLocked(ctx, session)
}

func (p *SessionPool) refreshItemCachesLocked(ctx context.Context, session *McpSession) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	updates := make(map[string]interface{}, 2)
	if data, err := fetchResourcesCacheResult(ctx, session.Adapter); err == nil {
		updates["resources_cache"] = data
	}
	if data, err := fetchPromptsCacheResult(ctx, session.Adapter); err == nil {
		updates["prompts_cache"] = data
	}
	if ctx.Err() != nil || len(updates) == 0 {
		return
	}
	_ = p.withSessionLock(session.key, func() error {
		return p.writeCatalogUpdatesLocked(ctx, session, updates)
	})
}

// fetchResourcesCache 合并静态资源与模板为 {"resources":[...],"templates":[...]}。
func fetchResourcesCache(ctx context.Context, adapter transport.TransportAdapter) string {
	// One-shot snapshots preserve the existing best-effort behavior. Persistent
	// invalidations use the strict helper below to keep a good cache on failure.
	combined := map[string]json.RawMessage{
		"resources": json.RawMessage(`[]`),
		"templates": json.RawMessage(`[]`),
	}
	if raw, err := adapter.ListResources(ctx); err == nil {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil && fields["resources"] != nil {
			combined["resources"] = fields["resources"]
		}
	}
	if raw, err := adapter.ListResourceTemplates(ctx); err == nil {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil && fields["resourceTemplates"] != nil {
			combined["templates"] = fields["resourceTemplates"]
		}
	}
	data, _ := json.Marshal(combined)
	return string(data)
}

func fetchResourcesCacheResult(ctx context.Context, adapter transport.TransportAdapter) (string, error) {
	combined := map[string]json.RawMessage{
		"resources": json.RawMessage(`[]`),
		"templates": json.RawMessage(`[]`),
	}
	raw, err := adapter.ListResources(ctx)
	if err != nil {
		return "", err
	}
	var resources map[string]json.RawMessage
	if err := json.Unmarshal(raw, &resources); err != nil {
		return "", err
	}
	if resources["resources"] != nil {
		combined["resources"] = resources["resources"]
	}
	raw, err = adapter.ListResourceTemplates(ctx)
	if err != nil {
		return "", err
	}
	var templates map[string]json.RawMessage
	if err := json.Unmarshal(raw, &templates); err != nil {
		return "", err
	}
	if templates["resourceTemplates"] != nil {
		combined["templates"] = templates["resourceTemplates"]
	}
	b, err := json.Marshal(combined)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// fetchPromptsCache 返回提示裸数组(与 tools_cache 的裸数组形态一致)。
func fetchPromptsCache(ctx context.Context, adapter transport.TransportAdapter) string {
	data, err := fetchPromptsCacheResult(ctx, adapter)
	if err != nil {
		return "[]"
	}
	return data
}

func fetchPromptsCacheResult(ctx context.Context, adapter transport.TransportAdapter) (string, error) {
	raw, err := adapter.ListPrompts(ctx)
	if err != nil {
		return "", err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	if m["prompts"] != nil {
		b, err := json.Marshal(m["prompts"])
		return string(b), err
	}
	return "[]", nil
}

// FetchAdapterCaches 一次性拉取上游 tools/resources/prompts 缓存 JSON(形态与 mcp_services 缓存列一致)。
// 供临时连接场景使用(如市场项快照手动刷新):不落库,由调用方决定写入位置。
func FetchAdapterCaches(ctx context.Context, adapter transport.TransportAdapter) (toolsJSON, resourcesJSON, promptsJSON string) {
	toolsJSON = "[]"
	if toolsData, err := json.Marshal(adapter.GetTools()); err == nil {
		toolsJSON = string(toolsData)
	}
	return toolsJSON, fetchResourcesCache(ctx, adapter), fetchPromptsCache(ctx, adapter)
}

func (p *SessionPool) Get(serviceID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[sessionKey{serviceID: serviceID}]
}

// GetByItem 共享市场条目的平台会话(条目键控);共享条目在池内至多一条。
// 不校验连接状态,调用方自行判定(与 Get 同口径)。
func (p *SessionPool) GetByItem(itemID int64) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessions[sessionKey{itemID: itemID}]
}

// GetSessionsByItem 某市场条目在池内的全部会话(按会话携带的 item 归属过滤,
// 与键控方式无关):独占条目=各安装用户的行会话,共享条目=至多一条条目会话。
// 供条目级进程视图/健康判定枚举。
func (p *SessionPool) GetSessionsByItem(itemID int64) []*McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var out []*McpSession
	for _, s := range p.sessions {
		if s.MarketplaceItemID != nil && *s.MarketplaceItemID == itemID {
			out = append(out, s)
		}
	}
	return out
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
	key := sessionKey{serviceID: serviceID}
	s := p.sessions[key]
	if s != nil {
		delete(p.sessions, key)
		p.markPassiveOfflineLocked(s)
	}
	p.mu.Unlock()
	if s != nil {
		closeSession(s)
	}
}

func (p *SessionPool) markPassiveOfflineLocked(session *McpSession) {
	if session.Adapter.GetType() == transport.TypePassiveWS && model.DB != nil {
		model.DB.Model(&model.McpService{}).Where("id = ?", session.ServiceID).Updates(map[string]interface{}{
			"passive_connected": false,
			"health_status":     common.HealthUnknown,
		})
	}
}

func closeSession(session *McpSession) error {
	if session.catalogCancel != nil {
		session.catalogCancel()
	}
	if observer, ok := session.Adapter.(toolsChangeObserver); ok {
		observer.SetToolsChangedHandler(nil)
	}
	return session.Adapter.Close()
}

// RemoveByMarketplaceItem 踢掉某市场项全部引用服务的池内会话(市场项平台上游配置变更后调用),
// 下次调用时按新 config_template 重新物化连接。只清内存池,不查 DB:无引用的项自然无会话可踢。
func (p *SessionPool) RemoveByMarketplaceItem(itemID int64) {
	var victims []*McpSession
	p.mu.Lock()
	for id, s := range p.sessions {
		if s.MarketplaceItemID != nil && *s.MarketplaceItemID == itemID {
			delete(p.sessions, id)
			p.markPassiveOfflineLocked(s)
			victims = append(victims, s)
		}
	}
	p.mu.Unlock()
	for _, s := range victims {
		closeSession(s)
	}
}

func (p *SessionPool) CloseAll() {
	var victims []*McpSession
	p.mu.Lock()
	p.closed = true
	for id, s := range p.sessions {
		delete(p.sessions, id)
		p.markPassiveOfflineLocked(s)
		victims = append(victims, s)
	}
	p.mu.Unlock()
	for _, s := range victims {
		closeSession(s)
	}
}

func (p *SessionPool) FindByToolName(toolName string) *McpSession {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, s := range p.sessions {
		for _, t := range s.ToolsSnapshot() {
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
		for _, t := range s.ToolsSnapshot() {
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

// dynamicAuthOptions 返回多秘钥动态注入的构造选项;单秘钥/stdio 服务为空
// (保持既有静态 headers 行为)。
func dynamicAuthOptions(svc *model.McpService) []transport.AdapterOption {
	sel := KeySelectors.Get(svc)
	if sel == nil {
		return nil
	}
	if sel.IsQueryParam() {
		return []transport.AdapterOption{transport.WithDynamicQueryAuth(sel.TargetName(), sel)}
	}
	return []transport.AdapterOption{transport.WithDynamicAuth(sel.TargetName(), sel)}
}

type fixedQueryAuth struct{ value string }

func (a fixedQueryAuth) Pick() (int, string, error) { return 0, a.value, nil }
func (a fixedQueryAuth) OnAuthFailure(int)          {}

// 单密钥 URL 参数认证也需逐请求注入：SSE 返回的 POST 端点不一定保留初始
// GET URL 的查询参数。只在内存中从静态 URL 提取，不更改库内单密钥配置。
func staticQueryOption(svc *model.McpService, raw string) (string, []transport.AdapterOption) {
	name := svc.ParseAuthKeyConfig().QueryParamName
	if svc.AuthType != "query_param" || name == "" {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw, nil
	}
	q := u.Query()
	values := q[name]
	if len(values) != 1 || values[0] == "" {
		return raw, nil
	}
	delete(q, name)
	u.RawQuery = q.Encode()
	return u.String(), []transport.AdapterOption{transport.WithDynamicQueryAuth(name, fixedQueryAuth{value: values[0]})}
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
		// 将所选包管理源镜像注入子进程环境变量（npx→NPM_CONFIG_REGISTRY；
		// uvx→UV_DEFAULT_INDEX+PIP_INDEX_URL）。用户在 env 里显式写过的同名变量优先。
		if registry, _ := config["registry"].(string); registry != "" {
			for k, v := range installer.RegistryEnv(installer.ClassifyCommand(cmd), registry) {
				if _, exists := envMap[k]; !exists {
					envMap[k] = v
				}
			}
		}
		return transport.NewStdioAdapter(svc.ID, cmd, args, envMap)

	case transport.TypeStreamableHTTP:
		url, _ := config["url"].(string)
		opts := dynamicAuthOptions(svc)
		if len(opts) == 0 {
			url, opts = staticQueryOption(svc, url)
		}
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewStreamableHTTPAdapter(svc.ID, url, h, opts...)

	case transport.TypeSSE:
		url, _ := config["url"].(string)
		opts := dynamicAuthOptions(svc)
		if len(opts) == 0 {
			url, opts = staticQueryOption(svc, url)
		}
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string)
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewSSEAdapter(svc.ID, url, h, opts...)

	case transport.TypeWebSocket:
		url, _ := config["url"].(string)
		headers, _ := config["headers"].(map[string]interface{})
		h := make(map[string]string, len(headers))
		for k, v := range headers {
			h[k], _ = v.(string)
		}
		return transport.NewWebSocketAdapter(svc.ID, url, h)

	default:
		return nil
	}
}
