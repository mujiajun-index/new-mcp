package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type notificationUpstream struct {
	server       *mcp.Server
	adapter      *SDKAdapter
	acknowledged chan mcp.NotificationSubscriptions
	subscribed   chan string
	unsubscribed chan string
}

func newNotificationUpstream(t *testing.T, legacy bool, rejectURI string) *notificationUpstream {
	return newNotificationUpstreamTransport(t, legacy, rejectURI, false)
}

func newNotificationUpstreamTransport(t *testing.T, legacy bool, rejectURI string, streamable bool) *notificationUpstream {
	t.Helper()
	u := &notificationUpstream{
		acknowledged: make(chan mcp.NotificationSubscriptions, 16),
		subscribed:   make(chan string, 16), unsubscribed: make(chan string, 16),
	}
	u.server = mcp.NewServer(&mcp.Implementation{Name: "notification-upstream", Version: "1"}, &mcp.ServerOptions{
		SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
			if req.Params.URI == rejectURI {
				return fmt.Errorf("subscription rejected")
			}
			u.subscribed <- req.Params.URI
			return nil
		},
		UnsubscribeHandler: func(_ context.Context, req *mcp.UnsubscribeRequest) error {
			u.unsubscribed <- req.Params.URI
			return nil
		},
	})
	u.server.AddTool(&mcp.Tool{Name: "initial", InputSchema: map[string]any{"type": "object"}}, nil)
	u.server.AddPrompt(&mcp.Prompt{Name: "initial"}, nil)
	u.server.AddResource(&mcp.Resource{URI: "memo://one", Name: "one"}, nil)
	u.server.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err == nil && method == "notifications/subscriptions/acknowledged" {
				if params, ok := req.GetParams().(*mcp.SubscriptionsAcknowledgedParams); ok {
					u.acknowledged <- params.Notifications
				}
			}
			return result, err
		}
	})
	if legacy {
		u.server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
			return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
				if method == "server/discover" {
					return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "legacy server"}
				}
				return next(ctx, method, req)
			}
		})
	}
	var closeServer func()
	if streamable {
		server := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return u.server
		}, &mcp.StreamableHTTPOptions{Stateless: true}))
		u.adapter = NewStreamableHTTPAdapter(1, server.URL, nil)
		closeServer = server.Close
	} else {
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		serverSession, err := u.server.Connect(context.Background(), serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		u.adapter = newSDKAdapter(TypeStdio)
		u.adapter.transport = clientTransport
		closeServer = func() { _ = serverSession.Close() }
	}
	t.Cleanup(func() {
		_ = u.adapter.Close()
		closeServer()
	})
	if err := u.adapter.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !legacy {
		if got := u.adapter.GetProtocolVersion(); got != "2026-07-28" {
			t.Fatalf("protocol version = %q", got)
		}
		select {
		case ack := <-u.acknowledged:
			if !ack.ToolsListChanged || !ack.PromptsListChanged || !ack.ResourcesListChanged {
				t.Fatalf("missing list subscriptions: %+v", ack)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("list subscriptions were not acknowledged")
		}
	}
	return u
}

func awaitResourceSubscription(t *testing.T, u *notificationUpstream, uri string) {
	t.Helper()
	select {
	case got := <-u.subscribed:
		if got != uri {
			t.Fatalf("subscribed URI = %q, want %q", got, uri)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("resource subscription was not received")
	}
	if u.adapter.GetProtocolVersion() == "2026-07-28" {
		select {
		case ack := <-u.acknowledged:
			if len(ack.ResourceSubscriptions) != 1 || ack.ResourceSubscriptions[0] != uri {
				t.Fatalf("resource subscription acknowledgement = %+v", ack)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("resource subscription was not acknowledged")
		}
	}
}

func awaitNotification(t *testing.T, events <-chan Notification, method string) Notification {
	t.Helper()
	select {
	case notification := <-events:
		if notification.Method != method {
			t.Fatalf("notification method = %q, want %q", notification.Method, method)
		}
		if !json.Valid(notification.Params) {
			t.Fatalf("invalid notification params: %q", notification.Params)
		}
		return notification
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", method)
		return Notification{}
	}
}

func TestSDKAdapterWatchModernNotifications(t *testing.T) {
	u := newNotificationUpstream(t, false, "")
	if caps := u.adapter.NotificationCapabilities(); !caps.ToolsListChanged || !caps.PromptsListChanged || !caps.ResourcesListChanged || !caps.ResourceSubscribe {
		t.Fatalf("missing upstream notification capabilities: %+v", caps)
	}
	events := make(chan Notification, 16)
	stop, err := u.adapter.WatchNotifications(context.Background(), []string{"memo://one"}, func(notification Notification) {
		events <- notification
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	awaitResourceSubscription(t, u, "memo://one")

	u.server.AddTool(&mcp.Tool{Name: "added", InputSchema: map[string]any{"type": "object"}}, nil)
	awaitNotification(t, events, "notifications/tools/list_changed")
	if tools := u.adapter.GetTools(); len(tools) != 2 {
		t.Fatalf("tool notification arrived before catalog refresh: %+v", tools)
	}
	u.server.AddPrompt(&mcp.Prompt{Name: "added"}, nil)
	awaitNotification(t, events, "notifications/prompts/list_changed")
	u.server.AddResource(&mcp.Resource{URI: "memo://two", Name: "two"}, nil)
	awaitNotification(t, events, "notifications/resources/list_changed")
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	notification := awaitNotification(t, events, "notifications/resources/updated")
	if notificationResourceURI(notification) != "memo://one" || !strings.Contains(string(notification.Params), mcp.MetaKeySubscriptionID) {
		t.Fatalf("resource notification lost its params: %s", notification.Params)
	}
}

func TestSDKAdapterWatchStreamableModernResourceUpdates(t *testing.T) {
	u := newNotificationUpstreamTransport(t, false, "", true)
	events := make(chan Notification, 8)
	stop, err := u.adapter.WatchNotifications(context.Background(), []string{"memo://one"}, func(n Notification) { events <- n })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	awaitResourceSubscription(t, u, "memo://one")
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	awaitNotification(t, events, "notifications/resources/updated")
	stop()
	select {
	case <-u.unsubscribed:
	case <-time.After(3 * time.Second):
		t.Fatal("closing the resource SSE response did not unsubscribe")
	}
}

func TestSDKAdapterWatchSharedResourceAndContextCancellation(t *testing.T) {
	u := newNotificationUpstream(t, false, "")
	firstEvents, secondEvents := make(chan Notification, 8), make(chan Notification, 8)
	stopFirst, err := u.adapter.WatchNotifications(context.Background(), []string{"memo://one", "memo://one"}, func(n Notification) {
		firstEvents <- n
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stopFirst()
	awaitResourceSubscription(t, u, "memo://one")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopSecond, err := u.adapter.WatchNotifications(ctx, []string{"memo://one"}, func(n Notification) {
		secondEvents <- n
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stopSecond()
	select {
	case uri := <-u.subscribed:
		t.Fatalf("shared watch subscribed twice: %q", uri)
	default:
	}
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	awaitNotification(t, firstEvents, "notifications/resources/updated")
	awaitNotification(t, secondEvents, "notifications/resources/updated")
	stopFirst()
	stopFirst()
	select {
	case uri := <-u.unsubscribed:
		t.Fatalf("stopping one listener unsubscribed shared resource: %q", uri)
	default:
	}
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	awaitNotification(t, secondEvents, "notifications/resources/updated")
	select {
	case n := <-firstEvents:
		t.Fatalf("stopped listener received %+v", n)
	default:
	}
	cancel()
	select {
	case uri := <-u.unsubscribed:
		if uri != "memo://one" {
			t.Fatalf("unsubscribed URI = %q", uri)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("context cancellation did not unsubscribe the final listener")
	}
	stopSecond()
	u.adapter.watchOperationMu.Lock()
	remaining := len(u.adapter.resourceWatchRefs)
	u.adapter.watchOperationMu.Unlock()
	if remaining != 0 {
		t.Fatal("resource references remained after all listeners stopped")
	}
}

func TestSDKAdapterWatchLegacySubscribeFailureRollsBack(t *testing.T) {
	u := newNotificationUpstream(t, true, "memo://reject")
	var calls atomic.Int32
	stop, err := u.adapter.WatchNotifications(context.Background(), []string{"memo://one", "memo://reject"}, func(Notification) {
		calls.Add(1)
	})
	if err == nil || stop != nil {
		t.Fatalf("watch = (%v, %v), want establishment failure", stop != nil, err)
	}
	awaitResourceSubscription(t, u, "memo://one")
	select {
	case uri := <-u.unsubscribed:
		if uri != "memo://one" {
			t.Fatalf("rollback unsubscribed %q", uri)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("rollback did not unsubscribe the acquired resource")
	}
	u.adapter.watchOperationMu.Lock()
	refs := len(u.adapter.resourceWatchRefs)
	u.adapter.watchOperationMu.Unlock()
	u.adapter.notificationMu.Lock()
	watchers := len(u.adapter.notificationWatchers)
	u.adapter.notificationMu.Unlock()
	if refs != 0 || watchers != 0 || calls.Load() != 0 {
		t.Fatalf("failed watch remained active: refs=%d, watchers=%d, calls=%d", refs, watchers, calls.Load())
	}
}

func TestSDKAdapterWatchModernMissingAcknowledgementRollsBack(t *testing.T) {
	u := newNotificationUpstream(t, false, "memo://reject")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stop, err := u.adapter.WatchNotifications(ctx, []string{"memo://one", "memo://reject"}, func(Notification) {
		t.Error("failed watch delivered a notification")
	})
	if err == nil || stop != nil {
		t.Fatalf("watch = (%v, %v), want missing acknowledgement failure", stop != nil, err)
	}
	awaitResourceSubscription(t, u, "memo://one")
	select {
	case uri := <-u.unsubscribed:
		if uri != "memo://one" {
			t.Fatalf("rollback unsubscribed %q", uri)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("failed modern watch did not roll back its acquired resource")
	}
	u.adapter.watchOperationMu.Lock()
	refs := len(u.adapter.resourceWatchRefs)
	u.adapter.watchOperationMu.Unlock()
	u.adapter.notificationMu.Lock()
	watchers, pending := len(u.adapter.notificationWatchers), len(u.adapter.resourceWatchAcks)
	u.adapter.notificationMu.Unlock()
	if refs != 0 || watchers != 0 || pending != 0 {
		t.Fatalf("failed watch left state: refs=%d, watchers=%d, pending=%d", refs, watchers, pending)
	}
}

func TestSDKAdapterNotificationListenerCanClose(t *testing.T) {
	u := newNotificationUpstream(t, false, "")
	closed := make(chan error, 1)
	stop, err := u.adapter.WatchNotifications(context.Background(), nil, func(Notification) {
		closed <- u.adapter.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	u.server.AddPrompt(&mcp.Prompt{Name: "close"}, nil)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listener closing the adapter deadlocked")
	}
	if _, err := u.adapter.WatchNotifications(context.Background(), nil, func(Notification) {}); err == nil {
		t.Fatal("closed adapter accepted a watcher")
	}
	if caps := u.adapter.NotificationCapabilities(); caps != (NotificationCapabilities{}) {
		t.Fatalf("closed adapter advertised notifications: %+v", caps)
	}
}

func TestSDKAdapterWatchLastResourceStopPreservesListNotifications(t *testing.T) {
	u := newNotificationUpstream(t, false, "")
	events := make(chan Notification, 8)
	stopLists, err := u.adapter.WatchNotifications(context.Background(), nil, func(n Notification) { events <- n })
	if err != nil {
		t.Fatal(err)
	}
	defer stopLists()
	stopResource, err := u.adapter.WatchNotifications(context.Background(), []string{"memo://one"}, func(Notification) {})
	if err != nil {
		t.Fatal(err)
	}
	awaitResourceSubscription(t, u, "memo://one")
	stopResource()
	select {
	case <-u.unsubscribed:
	case <-time.After(3 * time.Second):
		t.Fatal("resource listener did not unsubscribe")
	}
	u.server.AddTool(&mcp.Tool{Name: "after-unsubscribe", InputSchema: map[string]any{"type": "object"}}, nil)
	awaitNotification(t, events, "notifications/tools/list_changed")
	u.server.AddPrompt(&mcp.Prompt{Name: "after-unsubscribe"}, nil)
	awaitNotification(t, events, "notifications/prompts/list_changed")
	u.server.AddResource(&mcp.Resource{URI: "memo://after-unsubscribe", Name: "after-unsubscribe"}, nil)
	awaitNotification(t, events, "notifications/resources/list_changed")
}
