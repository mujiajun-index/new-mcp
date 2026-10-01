package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
	"gorm.io/gorm"
)

type lifecycleAdapter struct {
	fakeAdapter
	connected atomic.Bool
	done      chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	tools     []transport.Tool
	handler   func()
	refresh   func(context.Context) error
	onClose   func()
}

func newLifecycleAdapter(name string) *lifecycleAdapter {
	a := &lifecycleAdapter{done: make(chan struct{})}
	a.connected.Store(true)
	a.setTools(name)
	return a
}

func (a *lifecycleAdapter) GetType() transport.TransportType { return transport.TypePassiveWS }
func (a *lifecycleAdapter) IsConnected() bool                { return a.connected.Load() }
func (a *lifecycleAdapter) Done() <-chan struct{}            { return a.done }
func (a *lifecycleAdapter) Close() error {
	a.closeOnce.Do(func() {
		a.connected.Store(false)
		close(a.done)
		if a.onClose != nil {
			a.onClose()
		}
	})
	return nil
}
func (a *lifecycleAdapter) setTools(name string) {
	a.mu.Lock()
	a.tools = []transport.Tool{{Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}}
	a.mu.Unlock()
}
func (a *lifecycleAdapter) GetTools() []transport.Tool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return cloneTools(a.tools)
}
func (a *lifecycleAdapter) SetToolsChangedHandler(handler func()) {
	a.mu.Lock()
	a.handler = handler
	a.mu.Unlock()
}
func (a *lifecycleAdapter) notifyTools(name string) {
	a.setTools(name)
	a.mu.Lock()
	handler := a.handler
	a.mu.Unlock()
	if handler != nil {
		handler()
	}
}
func (a *lifecycleAdapter) RefreshTools(ctx context.Context) error {
	if a.refresh != nil {
		return a.refresh(ctx)
	}
	return nil
}

func passivePoolFixture(t *testing.T) (*SessionPool, *model.McpService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pool.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.McpService{}); err != nil {
		t.Fatal(err)
	}
	previous := model.DB
	model.DB = db
	pool := NewSessionPool()
	t.Cleanup(func() {
		pool.CloseAll()
		model.DB = previous
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	svc := &model.McpService{
		UserID: 31, Name: "local", Source: "user", TransportType: common.TransportPassiveWS,
		Status: common.StatusEnabled, Config: "{}", PassiveToken: "test-token",
	}
	if err := svc.Insert(); err != nil {
		t.Fatal(err)
	}
	return pool, svc
}

func attachForTest(t *testing.T, pool *SessionPool, svc *model.McpService, adapter *lifecycleAdapter) *McpSession {
	t.Helper()
	var session *McpSession
	if err := pool.WithServiceLock(svc.ID, func() error {
		var err error
		session, err = pool.AttachPassiveLocked(svc, adapter)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return session
}

func requirePassiveRow(t *testing.T, svc *model.McpService, connected bool, toolName string) {
	t.Helper()
	row, err := model.GetServiceByID(svc.UserID, svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	var tools []transport.Tool
	if err := json.Unmarshal([]byte(row.ToolsCache), &tools); err != nil {
		t.Fatal(err)
	}
	if row.PassiveConnected != connected || len(tools) != 1 || tools[0].Name != toolName {
		t.Fatalf("connected=%v tools=%+v, want connected=%v tool=%q", row.PassiveConnected, tools, connected, toolName)
	}
}

func TestPassivePoolWaitsForInboundAndPublishesTools(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	if _, err := pool.GetOrConnect(context.Background(), svc); !errors.Is(err, ErrPassiveNotConnected) {
		t.Fatalf("offline passive service must wait for inbound, got %v", err)
	}
	adapter := newLifecycleAdapter("hello")
	session := attachForTest(t, pool, svc, adapter)
	connected, err := pool.GetOrConnect(context.Background(), svc)
	if err != nil || connected != session || session.UserID != svc.UserID || session.ServiceName != svc.Name {
		t.Fatalf("attached session mismatch: %+v, %v", connected, err)
	}
	requirePassiveRow(t, svc, true, "hello")
	if routed, name, err := NewToolRouter(pool).Route("local__hello", svc.UserID); err != nil || routed != session || name != "hello" {
		t.Fatalf("inbound tool did not route: session=%v name=%q err=%v", routed, name, err)
	}
	if _, _, err := NewToolRouter(pool).Route("local__hello", svc.UserID+1); err == nil {
		t.Fatal("inbound session leaked across users")
	}
	snapshot := session.ToolsSnapshot()
	snapshot[0].Name = "changed"
	snapshot[0].InputSchema[0] = '['
	if got := session.ToolsSnapshot(); got[0].Name != "hello" || got[0].InputSchema[0] != '{' {
		t.Fatal("tools snapshot is not detached")
	}
}

func TestPassiveReplacementRejectsStaleCleanupAndRefresh(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	oldAdapter := newLifecycleAdapter("old")
	old := attachForTest(t, pool, svc, oldAdapter)
	refreshStarted, finishRefresh := make(chan struct{}), make(chan struct{})
	oldAdapter.refresh = func(ctx context.Context) error {
		close(refreshStarted)
		select {
		case <-finishRefresh:
			oldAdapter.setTools("stale")
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	refreshed := make(chan error, 1)
	go func() { refreshed <- pool.RefreshTools(context.Background(), old) }()
	<-refreshStarted
	current := attachForTest(t, pool, svc, newLifecycleAdapter("new"))
	close(finishRefresh)
	if err := <-refreshed; err == nil {
		t.Fatal("refresh of a replaced session must fail")
	}
	_ = pool.WithServiceLock(svc.ID, func() error {
		pool.removeIfCurrent(old)
		return nil
	})
	if pool.Get(svc.ID) != current {
		t.Fatal("old disconnect removed replacement")
	}
	requirePassiveRow(t, svc, true, "new")
}

func TestPassiveCatalogFailurePreservesCurrentConnection(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	writeFailure := errors.New("catalog write failed")
	var failWrites atomic.Bool
	const callbackName = "bridge:fail_catalog_update"
	if err := model.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if failWrites.Load() {
			tx.AddError(writeFailure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { failWrites.Store(false) })
	oldAdapter := newLifecycleAdapter("old")
	old := attachForTest(t, pool, svc, oldAdapter)
	failWrites.Store(true)
	replacement := newLifecycleAdapter("new")
	defer replacement.Close()
	err := pool.WithServiceLock(svc.ID, func() error {
		_, err := pool.AttachPassiveLocked(svc, replacement)
		return err
	})
	if !errors.Is(err, writeFailure) {
		t.Fatalf("attachment must report failed catalog publication, got %v", err)
	}
	if pool.Get(svc.ID) != old || !oldAdapter.IsConnected() {
		t.Fatal("failed replacement evicted the working connection")
	}
	requirePassiveRow(t, svc, true, "old")
	oldAdapter.refresh = func(context.Context) error {
		oldAdapter.setTools("unpublished")
		return nil
	}
	if err := pool.RefreshTools(context.Background(), old); !errors.Is(err, writeFailure) {
		t.Fatalf("manual refresh must report failed catalog publication, got %v", err)
	}
	if got := old.ToolsSnapshot(); got[0].Name != "old" {
		t.Fatalf("failed refresh changed the published pool catalog: %+v", got)
	}
	requirePassiveRow(t, svc, true, "old")
}

func TestPassiveToolsRefreshAndNotification(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newLifecycleAdapter("initial")
	adapter.refresh = func(context.Context) error {
		adapter.setTools("refreshed")
		return nil
	}
	session := attachForTest(t, pool, svc, adapter)
	if err := pool.RefreshTools(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	requirePassiveRow(t, svc, true, "refreshed")
	adapter.notifyTools("notification")
	deadline := time.After(time.Second)
	for {
		if got := session.ToolsSnapshot(); len(got) == 1 && got[0].Name == "notification" {
			// Acquire the same lock to wait until its row publication finishes.
			_ = pool.WithServiceLock(svc.ID, func() error { return nil })
			break
		}
		select {
		case <-deadline:
			t.Fatal("list-change notification did not publish tools")
		case <-time.After(time.Millisecond):
		}
	}
	requirePassiveRow(t, svc, true, "notification")
}

func TestPassiveRemoveClosesOutsidePoolLockAndPreservesCache(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newLifecycleAdapter("cached")
	adapter.onClose = func() { _ = pool.Get(svc.ID) }
	attachForTest(t, pool, svc, adapter)
	removed := make(chan struct{})
	go func() {
		_ = pool.WithServiceLock(svc.ID, func() error {
			pool.Remove(svc.ID)
			return nil
		})
		close(removed)
	}()
	select {
	case <-removed:
	case <-time.After(time.Second):
		t.Fatal("adapter Close deadlocked on the pool map lock")
	}
	requirePassiveRow(t, svc, false, "cached")
	if _, err := pool.GetOrConnect(context.Background(), svc); !errors.Is(err, ErrPassiveNotConnected) {
		t.Fatalf("removed passive service must await reconnect: %v", err)
	}
}

func TestPassiveDisconnectWatcherPersistsOffline(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newLifecycleAdapter("cached")
	attachForTest(t, pool, svc, adapter)
	_ = adapter.Close()
	deadline := time.After(time.Second)
	for pool.Get(svc.ID) != nil {
		select {
		case <-deadline:
			t.Fatal("disconnected adapter remained in pool")
		case <-time.After(time.Millisecond):
		}
	}
	// map removal and the offline row update complete in the same map lock.
	requirePassiveRow(t, svc, false, "cached")
}

func TestPassiveShutdownRejectsLateAttachment(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newLifecycleAdapter("cached")
	attachForTest(t, pool, svc, adapter)
	pool.CloseAll()
	requirePassiveRow(t, svc, false, "cached")
	if _, err := pool.GetOrConnect(context.Background(), svc); !errors.Is(err, ErrSessionPoolClosed) {
		t.Fatalf("closed pool must reject connections: %v", err)
	}
	late := newLifecycleAdapter("late")
	defer late.Close()
	if err := pool.WithServiceLock(svc.ID, func() error {
		_, err := pool.AttachPassiveLocked(svc, late)
		return err
	}); !errors.Is(err, ErrSessionPoolClosed) {
		t.Fatalf("closed pool must reject attachment: %v", err)
	}
	pool.removeIfCurrent(&McpSession{key: sessionKey{serviceID: svc.ID}, Adapter: late})
	requirePassiveRow(t, svc, false, "cached")
}
