package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

// catalogAdapter models independently mutable upstream resources and prompts.
// Its synchronous invalidations let the tests exercise storms and late events
// without depending on the SDK's own worker scheduling.
type catalogAdapter struct {
	*lifecycleAdapter
	catalogMu     sync.Mutex
	resource      string
	prompt        string
	listener      func(transport.Notification)
	watchURIs     []string
	watchReady    chan struct{}
	stopCalls     atomic.Int32
	resourceCalls atomic.Int32
	promptCalls   atomic.Int32
	resourceError atomic.Bool
	resourceHook  func(context.Context) (json.RawMessage, error)
}

func newCatalogAdapter(name string) *catalogAdapter {
	return &catalogAdapter{
		lifecycleAdapter: newLifecycleAdapter(name), resource: name, prompt: name,
		watchReady: make(chan struct{}),
	}
}

func (a *catalogAdapter) WatchNotifications(ctx context.Context, uris []string, listener func(transport.Notification)) (func(), error) {
	a.catalogMu.Lock()
	a.listener = listener
	a.watchURIs = append([]string(nil), uris...)
	a.catalogMu.Unlock()
	close(a.watchReady)
	var once sync.Once
	return func() { once.Do(func() { a.stopCalls.Add(1) }) }, nil
}

func (a *catalogAdapter) notify(method string) {
	a.catalogMu.Lock()
	listener := a.listener
	a.catalogMu.Unlock()
	if listener != nil {
		listener(transport.Notification{Method: method, Params: json.RawMessage(`{"uri":"file:///ignored"}`)})
	}
}

func (a *catalogAdapter) changeCatalog(resource, prompt string) {
	a.catalogMu.Lock()
	a.resource, a.prompt = resource, prompt
	a.catalogMu.Unlock()
}

func (a *catalogAdapter) ListResources(ctx context.Context) (json.RawMessage, error) {
	a.resourceCalls.Add(1)
	a.catalogMu.Lock()
	hook, resource := a.resourceHook, a.resource
	a.catalogMu.Unlock()
	if hook != nil {
		return hook(ctx)
	}
	if a.resourceError.Load() {
		return nil, errors.New("temporary catalog failure")
	}
	return json.Marshal(map[string]any{"resources": []map[string]string{{"uri": "file:///" + resource, "name": resource}}})
}

func (a *catalogAdapter) ListResourceTemplates(ctx context.Context) (json.RawMessage, error) {
	a.catalogMu.Lock()
	resource := a.resource
	a.catalogMu.Unlock()
	return json.Marshal(map[string]any{"resourceTemplates": []map[string]string{{"uriTemplate": "file:///" + resource + "/{id}", "name": resource + "-template"}}})
}

func (a *catalogAdapter) ListPrompts(ctx context.Context) (json.RawMessage, error) {
	a.promptCalls.Add(1)
	a.catalogMu.Lock()
	prompt := a.prompt
	a.catalogMu.Unlock()
	return json.Marshal(map[string]any{"prompts": []map[string]string{{"name": prompt}}})
}

func attachCatalogForTest(t *testing.T, pool *SessionPool, svc *model.McpService, adapter *catalogAdapter) *McpSession {
	t.Helper()
	var session *McpSession
	if err := pool.WithServiceLock(svc.ID, func() error {
		var err error
		session, err = pool.AttachPassiveLocked(svc, adapter)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// A production Close never joins this worker while holding a service
		// lock. Tests join after cancellation before replacing the global DB.
		_ = closeSession(session)
		select {
		case <-session.catalogDone:
		case <-time.After(2 * time.Second):
			t.Error("catalog worker remained alive after close")
		}
	})
	select {
	case <-adapter.watchReady:
	case <-time.After(time.Second):
		t.Fatal("list notification watcher was not registered")
	}
	return session
}

func awaitCatalogRow(t *testing.T, svc *model.McpService, resource, prompt string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, err := model.GetServiceByID(svc.UserID, svc.ID)
		if err == nil && strings.Contains(row.ResourcesCache, fmt.Sprintf(`"name":"%s"`, resource)) &&
			strings.Contains(row.ResourcesCache, fmt.Sprintf(`"name":"%s-template"`, resource)) &&
			strings.Contains(row.PromptsCache, fmt.Sprintf(`"name":"%s"`, prompt)) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	row, _ := model.GetServiceByID(svc.UserID, svc.ID)
	t.Fatalf("catalog row did not converge to resource=%q prompt=%q: %+v", resource, prompt, row)
}

func TestCatalogNotificationsPersistResourcesAndPromptsIndependently(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newCatalogAdapter("initial")
	attachCatalogForTest(t, pool, svc, adapter)
	awaitCatalogRow(t, svc, "initial", "initial")
	if len(adapter.watchURIs) != 0 {
		t.Fatalf("catalog observation must not subscribe to resource URIs: %v", adapter.watchURIs)
	}
	initialResources, initialPrompts := adapter.resourceCalls.Load(), adapter.promptCalls.Load()
	adapter.changeCatalog("updated-resource", "updated-prompt")
	adapter.notify("notifications/resources/list_changed")
	awaitCatalogRow(t, svc, "updated-resource", "initial")
	if adapter.promptCalls.Load() != initialPrompts {
		t.Fatal("resource notification unnecessarily refreshed prompts")
	}
	resourceCalls := adapter.resourceCalls.Load()
	if resourceCalls <= initialResources {
		t.Fatal("resource list change did not fetch the new catalog")
	}
	adapter.notify("notifications/prompts/list_changed")
	awaitCatalogRow(t, svc, "updated-resource", "updated-prompt")
	if adapter.resourceCalls.Load() != resourceCalls {
		t.Fatal("prompt notification unnecessarily refreshed resources")
	}
}

func TestCatalogNotificationStormCoalescesAndReleasesLease(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newCatalogAdapter("initial")
	session := attachCatalogForTest(t, pool, svc, adapter)
	awaitCatalogRow(t, svc, "initial", "initial")
	started, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	adapter.catalogMu.Lock()
	adapter.resourceHook = func(ctx context.Context) (json.RawMessage, error) {
		once.Do(func() { close(started) })
		select {
		case <-unblock:
			return json.RawMessage(`{"resources":[{"name":"updated","uri":"file:///updated"}]}`), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	adapter.catalogMu.Unlock()
	adapter.changeCatalog("updated", "initial")
	baseline := adapter.resourceCalls.Load()
	adapter.notify("notifications/resources/list_changed")
	<-started
	for i := 0; i < 5000; i++ {
		adapter.notify("notifications/resources/list_changed")
	}
	session.useMu.Lock()
	leases := session.inUse
	session.useMu.Unlock()
	if leases != 1 {
		t.Fatalf("notification storm must share one refresh lease, got %d", leases)
	}
	close(unblock)
	awaitCatalogRow(t, svc, "updated", "initial")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		session.useMu.Lock()
		leases = session.inUse
		session.useMu.Unlock()
		if leases == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if leases != 0 || adapter.resourceCalls.Load()-baseline > 2 {
		t.Fatalf("unbounded refreshes or leaked lease: calls=%d leases=%d", adapter.resourceCalls.Load()-baseline, leases)
	}
}

func TestCatalogReplacementCancelsStaleWorkerAndRejectsLateEvents(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	oldAdapter := newCatalogAdapter("old")
	old := attachCatalogForTest(t, pool, svc, oldAdapter)
	awaitCatalogRow(t, svc, "old", "old")
	started, unblock := make(chan struct{}), make(chan struct{})
	oldAdapter.catalogMu.Lock()
	oldAdapter.resourceHook = func(context.Context) (json.RawMessage, error) {
		close(started)
		<-unblock // Model a late response from a transport ignoring cancellation.
		return json.RawMessage(`{"resources":[{"name":"stale","uri":"file:///stale"}]}`), nil
	}
	oldAdapter.catalogMu.Unlock()
	oldAdapter.notify("notifications/resources/list_changed")
	<-started
	replacement := newCatalogAdapter("new")
	attachCatalogForTest(t, pool, svc, replacement)
	awaitCatalogRow(t, svc, "new", "new")
	close(unblock)
	select {
	case <-old.catalogDone:
	case <-time.After(time.Second):
		t.Fatal("replacement did not stop the old catalog worker")
	}
	oldAdapter.notify("notifications/prompts/list_changed")
	awaitCatalogRow(t, svc, "new", "new")
	if oldAdapter.stopCalls.Load() != 1 {
		t.Fatalf("old watcher cleanup ran %d times", oldAdapter.stopCalls.Load())
	}
	old.useMu.Lock()
	leases := old.inUse
	old.useMu.Unlock()
	if leases != 0 {
		t.Fatalf("replaced worker leaked %d leases", leases)
	}
}

func TestCatalogCloseCancelsFetchWithoutWaitingUnderServiceLock(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newCatalogAdapter("initial")
	session := attachCatalogForTest(t, pool, svc, adapter)
	awaitCatalogRow(t, svc, "initial", "initial")
	started := make(chan struct{})
	adapter.catalogMu.Lock()
	adapter.resourceHook = func(ctx context.Context) (json.RawMessage, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	adapter.catalogMu.Unlock()
	adapter.notify("notifications/resources/list_changed")
	<-started
	closed := make(chan struct{})
	go func() {
		_ = pool.WithServiceLock(svc.ID, func() error { pool.Remove(svc.ID); return nil })
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("closing under the service lock deadlocked with catalog refresh")
	}
	select {
	case <-session.catalogDone:
	case <-time.After(time.Second):
		t.Fatal("catalog fetch did not observe session cancellation")
	}
	awaitCatalogRow(t, svc, "initial", "initial")
}

func TestCatalogFetchFailurePreservesCacheAndRetriesInSameWorker(t *testing.T) {
	pool, svc := passivePoolFixture(t)
	adapter := newCatalogAdapter("initial")
	attachCatalogForTest(t, pool, svc, adapter)
	awaitCatalogRow(t, svc, "initial", "initial")
	adapter.changeCatalog("recovered", "initial")
	adapter.resourceError.Store(true)
	baseline := adapter.resourceCalls.Load()
	adapter.notify("notifications/resources/list_changed")
	deadline := time.Now().Add(time.Second)
	for adapter.resourceCalls.Load() == baseline && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	awaitCatalogRow(t, svc, "initial", "initial")
	adapter.resourceError.Store(false)
	// No second notification: a failed invalidation remains pending with bounded
	// backoff, so recovery does not require another upstream list change.
	awaitCatalogRow(t, svc, "recovered", "initial")
}

type sharedCatalogAdapter struct{ *catalogAdapter }

func (a *sharedCatalogAdapter) GetType() transport.TransportType { return transport.TypeStdio }

func TestSharedCatalogNotificationsPublishEnabledMarketplaceReferences(t *testing.T) {
	pool, _ := passivePoolFixture(t)
	itemID := int64(902)
	var references []*model.McpService
	for i, source := range []string{"marketplace", "marketplace", "marketplace", "user", "marketplace"} {
		ref := &model.McpService{
			UserID: int64(100 + i), Name: fmt.Sprintf("installed-%d", i), Source: source,
			MarketplaceItemID: &itemID, Status: common.StatusEnabled, Config: "{}",
			AuthConfig: "installation-secret", TransportType: common.TransportStdio,
			ResourcesCache: `{"resources":[{"name":"untouched"}],"templates":[]}`,
			PromptsCache:   `[{"name":"untouched"}]`,
		}
		if i == 2 {
			ref.Status = common.StatusDisabled
		}
		if i == 4 {
			otherItem := itemID + 1
			ref.MarketplaceItemID = &otherItem
		}
		if err := ref.Insert(); err != nil {
			t.Fatal(err)
		}
		references = append(references, ref)
	}
	prewarm := &model.McpService{Name: "shared", Source: "marketplace", SharedProcess: true, MarketplaceItemID: &itemID}
	adapter := &sharedCatalogAdapter{newCatalogAdapter("initial")}
	session := newSession(prewarm, adapter)
	if err := pool.withSessionLock(session.key, func() error { return pool.publishSessionLocked(session) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = closeSession(session)
		select {
		case <-session.catalogDone:
		case <-time.After(time.Second):
			t.Error("shared catalog worker remained alive after close")
		}
	})
	select {
	case <-adapter.watchReady:
	case <-time.After(time.Second):
		t.Fatal("shared session did not attach its list watcher")
	}
	for _, ref := range references[:2] {
		awaitCatalogRow(t, ref, "initial", "initial")
	}
	adapter.changeCatalog("new-resource", "new-prompt")
	adapter.notify("notifications/resources/list_changed")
	adapter.notify("notifications/prompts/list_changed")
	for _, ref := range references[:2] {
		awaitCatalogRow(t, ref, "new-resource", "new-prompt")
		row, err := model.GetServiceByID(ref.UserID, ref.ID)
		if err != nil || row.Config != "{}" || row.AuthConfig != "installation-secret" {
			t.Fatalf("public catalog refresh modified installation credentials: %+v, %v", row, err)
		}
	}
	for _, ref := range references[2:] {
		row, err := model.GetServiceByID(ref.UserID, ref.ID)
		if err != nil || !strings.Contains(row.ResourcesCache, "untouched") || !strings.Contains(row.PromptsCache, "untouched") {
			t.Fatalf("shared catalog leaked into an unrelated or disabled reference: %+v, %v", row, err)
		}
	}
	pool.CloseAll()
	select {
	case <-session.catalogDone:
	case <-time.After(time.Second):
		t.Fatal("pool close did not stop the shared list watcher")
	}
	if err := pool.withSessionLock(session.key, func() error {
		return pool.writeCatalogUpdatesLocked(context.Background(), session, map[string]interface{}{"resources_cache": "stale"})
	}); err == nil {
		t.Fatal("closed shared session was allowed to overwrite installation catalogs")
	}
}
