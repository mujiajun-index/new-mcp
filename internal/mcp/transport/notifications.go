package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type notificationObserver struct {
	mu        sync.Mutex
	resources map[string]bool
	listener  func(Notification)
	queue     []Notification
	wake      chan struct{}
	stoppedCh chan struct{}
	stopped   bool
}

func (a *SDKAdapter) NotificationCapabilities() NotificationCapabilities {
	sess, err := a.session()
	if err != nil {
		return NotificationCapabilities{}
	}
	upstream := upstreamCapability(sess)
	if upstream == nil {
		return NotificationCapabilities{}
	}
	caps := NotificationCapabilities{}
	if upstream.Tools != nil {
		caps.ToolsListChanged = upstream.Tools.ListChanged
	}
	if upstream.Prompts != nil {
		caps.PromptsListChanged = upstream.Prompts.ListChanged
	}
	if upstream.Resources != nil {
		caps.ResourcesListChanged = upstream.Resources.ListChanged
		caps.ResourceSubscribe = upstream.Resources.Subscribe
	}
	return caps
}

func makeNotification(method string, params any) Notification {
	raw, err := json.Marshal(params)
	if err != nil || string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	return Notification{Method: method, Params: raw}
}

func notificationResourceURI(notification Notification) string {
	var params struct {
		URI string `json:"uri"`
	}
	_ = json.Unmarshal(notification.Params, &params)
	return params.URI
}

func (o *notificationObserver) enqueue(notification Notification) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped {
		return
	}
	uri := ""
	if notification.Method == "notifications/resources/updated" {
		uri = notificationResourceURI(notification)
		if !o.resources[uri] {
			return
		}
	}
	notification.Params = append(json.RawMessage(nil), notification.Params...)
	// These messages invalidate state rather than carry an event log. Keeping
	// one pending notice per method/URI also bounds a slow listener's queue.
	for i, pending := range o.queue {
		if pending.Method == notification.Method && (uri == "" || notificationResourceURI(pending) == uri) {
			o.queue[i] = notification
			return
		}
	}
	o.queue = append(o.queue, notification)
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

func (o *notificationObserver) run() {
	for range o.wake {
		for {
			o.mu.Lock()
			if o.stopped {
				o.mu.Unlock()
				return
			}
			if len(o.queue) == 0 {
				o.mu.Unlock()
				break
			}
			notification := o.queue[0]
			o.queue[0] = Notification{}
			o.queue = o.queue[1:]
			o.mu.Unlock()
			o.listener(notification)
		}
	}
}

func (o *notificationObserver) stop() {
	o.mu.Lock()
	if !o.stopped {
		o.stopped = true
		o.queue = nil
		close(o.stoppedCh)
	}
	o.mu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
}

func (a *SDKAdapter) publishNotification(notification Notification) {
	a.notificationMu.Lock()
	watchers := make([]*notificationObserver, 0, len(a.notificationWatchers))
	for _, watcher := range a.notificationWatchers {
		watchers = append(watchers, watcher)
	}
	a.notificationMu.Unlock()
	for _, watcher := range watchers {
		watcher.enqueue(notification)
	}
}

func (a *SDKAdapter) closeNotificationWatchers() {
	a.notificationMu.Lock()
	watchers := a.notificationWatchers
	a.notificationWatchers = make(map[uint64]*notificationObserver)
	a.notificationMu.Unlock()
	for _, watcher := range watchers {
		watcher.stop()
	}
}

// WatchNotifications shares upstream resource subscriptions between listeners.
// Callbacks run outside SDK handlers and adapter locks and should return promptly.
// The returned stop function is idempotent; cancelling ctx also stops the watch.
// A callback already in progress may finish after stop returns.
func (a *SDKAdapter) WatchNotifications(ctx context.Context, resourceURIs []string, listener func(Notification)) (func(), error) {
	if listener == nil {
		return nil, fmt.Errorf("notification listener is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resources := make(map[string]bool, len(resourceURIs))
	uris := make([]string, 0, len(resourceURIs))
	for _, uri := range resourceURIs {
		if uri == "" {
			return nil, fmt.Errorf("resource subscription URI is required")
		}
		if !resources[uri] {
			resources[uri] = true
			uris = append(uris, uri)
		}
	}

	a.watchOperationMu.Lock()
	defer a.watchOperationMu.Unlock()
	sess, err := a.session()
	if err != nil {
		return nil, err
	}
	if len(uris) > 0 {
		caps := upstreamCapability(sess)
		if caps == nil || caps.Resources == nil || !caps.Resources.Subscribe {
			return nil, fmt.Errorf("upstream does not support resource subscriptions")
		}
	}
	watcher := &notificationObserver{
		resources: resources, listener: listener, wake: make(chan struct{}, 1), stoppedCh: make(chan struct{}),
	}
	a.notificationMu.Lock()
	select {
	case <-a.done:
		a.notificationMu.Unlock()
		return nil, fmt.Errorf("adapter is closed")
	default:
	}
	a.nextWatcherID++
	id := a.nextWatcherID
	a.notificationWatchers[id] = watcher
	a.notificationMu.Unlock()

	acquired := make([]string, 0, len(uris))
	rollback := func(err error) (func(), error) {
		a.removeNotificationWatcher(id, watcher)
		a.releaseResourceWatchesLocked(sess, acquired)
		return nil, err
	}
	for _, uri := range uris {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		if a.resourceWatchRefs[uri] == 0 {
			if err := a.subscribeResourceWatch(ctx, sess, uri); err != nil {
				if result := sess.InitializeResult(); result != nil && result.ProtocolVersion >= "2026-07-28" {
					a.releaseResourceWatchesLocked(sess, []string{uri})
				}
				return rollback(fmt.Errorf("subscribe resource %s: %w", uri, err))
			}
		}
		a.resourceWatchRefs[uri]++
		acquired = append(acquired, uri)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	select {
	case <-a.done:
		return rollback(fmt.Errorf("adapter is closed"))
	default:
	}

	var once sync.Once
	stop := func() {
		once.Do(func() {
			a.removeNotificationWatcher(id, watcher)
			a.watchOperationMu.Lock()
			a.releaseResourceWatchesLocked(sess, acquired)
			a.watchOperationMu.Unlock()
		})
	}
	go watcher.run()
	go func() {
		select {
		case <-ctx.Done():
		case <-a.done:
		case <-watcher.stoppedCh:
		}
		stop()
	}()
	return stop, nil
}

func (a *SDKAdapter) acknowledgeResourceWatches(uris []string) {
	a.notificationMu.Lock()
	defer a.notificationMu.Unlock()
	for _, uri := range uris {
		if ack := a.resourceWatchAcks[uri]; ack != nil {
			select {
			case ack <- struct{}{}:
			default:
			}
		}
	}
}

func (a *SDKAdapter) subscribeResourceWatch(ctx context.Context, sess *mcp.ClientSession, uri string) error {
	if result := sess.InitializeResult(); result == nil || result.ProtocolVersion < "2026-07-28" {
		return sess.Subscribe(ctx, &mcp.SubscribeParams{URI: uri})
	}
	// The SDK's modern Subscribe returns before the server accepts the URI.
	// A watch is established only when the honored subset acknowledges it.
	ack := make(chan struct{}, 1)
	a.notificationMu.Lock()
	a.resourceWatchAcks[uri] = ack
	a.notificationMu.Unlock()
	defer func() {
		a.notificationMu.Lock()
		delete(a.resourceWatchAcks, uri)
		a.notificationMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sess.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); err != nil {
		return err
	}
	select {
	case <-ack:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("subscription was not acknowledged: %w", ctx.Err())
	case <-a.done:
		return fmt.Errorf("adapter is closed")
	}
}

func (a *SDKAdapter) removeNotificationWatcher(id uint64, watcher *notificationObserver) {
	watcher.stop()
	a.notificationMu.Lock()
	delete(a.notificationWatchers, id)
	a.notificationMu.Unlock()
}

// The operation lock serializes a last unsubscribe with a new first subscribe.
// SDK calls never hold a.mu or the lock used by notification handlers.
func (a *SDKAdapter) releaseResourceWatchesLocked(sess *mcp.ClientSession, uris []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, uri := range uris {
		refs := a.resourceWatchRefs[uri]
		if refs > 1 {
			a.resourceWatchRefs[uri] = refs - 1
			continue
		}
		delete(a.resourceWatchRefs, uri)
		select {
		case <-a.done:
			continue
		default:
		}
		_ = sess.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri})
	}
}

var _ NotificationWatcher = (*SDKAdapter)(nil)
var _ NotificationCapabilitiesProvider = (*SDKAdapter)(nil)
