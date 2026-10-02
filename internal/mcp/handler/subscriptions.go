package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

const (
	notificationSetupTimeout  = 20 * time.Second
	notificationCheckInterval = 10 * time.Second
	maxNotificationSources    = 64
)

// Include cleanup in the source limit: a stalled upstream must not accumulate
// an unbounded number of background subscribe or unsubscribe operations.
var notificationSourceSlots = make(chan struct{}, 2048)
var notificationAttachSlots = make(chan struct{}, 32)

// startGatewayTask shares the shutdown gate with request registration.
// Close prevents new work and waits for requests, authorization checks and
// source setup already in progress before closing the application database.
func (h *GatewayHandler) startGatewayTask() (func(), error) {
	h.streams.mu.Lock()
	defer h.streams.mu.Unlock()
	if h.streams.closed {
		return nil, fmt.Errorf("gateway is shutting down")
	}
	h.workWG.Add(1)
	return h.workWG.Done, nil
}

type notificationFailureKey struct{}

func withNotificationFailure(ctx context.Context, report func(error)) context.Context {
	return context.WithValue(ctx, notificationFailureKey{}, report)
}

func reportNotificationFailure(ctx context.Context, err error) {
	if report, ok := ctx.Value(notificationFailureKey{}).(func(error)); ok && report != nil {
		report(err)
	}
}

type JSONRPCNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type NotificationStream struct {
	Messages     <-chan JSONRPCNotification
	Acknowledged mcp.NotificationSubscriptions
	Stop         func()
	Done         <-chan struct{}
	Finished     <-chan struct{}
}

type streamRegistry struct {
	mu     sync.Mutex
	active map[string]context.CancelFunc
	closed bool
}

func principalKey(logCtx *LogContext) string {
	return fmt.Sprintf("%d:%d:%s:%s", logCtx.UserID, logCtx.ApiKeyID, logCtx.GroupSlug, logCtx.ExposeMode)
}

func streamKey(logCtx *LogContext, id any) string {
	raw, _ := json.Marshal(id)
	return principalKey(logCtx) + ":" + logCtx.SessionID + ":" + logCtx.ConnectionID + ":" + string(raw)
}

func (h *GatewayHandler) registerStream(logCtx *LogContext, id any, cancel context.CancelFunc) (func(), error) {
	h.streams.mu.Lock()
	defer h.streams.mu.Unlock()
	if h.streams.closed {
		return nil, fmt.Errorf("gateway is shutting down")
	}
	if h.streams.active == nil {
		h.streams.active = make(map[string]context.CancelFunc)
	}
	key := streamKey(logCtx, id)
	if logCtx.SessionID == "" && logCtx.ConnectionID == "" {
		key += ":" + uuid.NewString()
	}
	if _, exists := h.streams.active[key]; exists {
		return nil, fmt.Errorf("request ID already has an active stream")
	}
	if len(h.streams.active) >= 1024 {
		return nil, fmt.Errorf("too many active gateway streams")
	}
	principalPrefix := principalKey(logCtx) + ":"
	principalStreams := 0
	for activeKey := range h.streams.active {
		if strings.HasPrefix(activeKey, principalPrefix) {
			principalStreams++
		}
	}
	if principalStreams >= 32 {
		return nil, fmt.Errorf("too many active streams for this API key and endpoint")
	}
	h.streams.active[key] = cancel
	var once sync.Once
	return func() {
		once.Do(func() { h.streams.mu.Lock(); delete(h.streams.active, key); h.streams.mu.Unlock(); cancel() })
	}, nil
}

func (h *GatewayHandler) cancelStream(req *JSONRPCRequest, logCtx *LogContext) {
	if logCtx.SessionID == "" && logCtx.ConnectionID == "" {
		return // A stateless HTTP request is cancelled by aborting its transport.
	}
	var params struct {
		RequestID any `json:"requestId"`
	}
	decoder := json.NewDecoder(bytes.NewReader(req.Params))
	decoder.UseNumber()
	if decoder.Decode(&params) != nil {
		return
	}
	h.streams.mu.Lock()
	cancel := h.streams.active[streamKey(logCtx, params.RequestID)]
	h.streams.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// CheckPrincipal rechecks authorization during a stream or webhook lifetime.
func (h *GatewayHandler) CheckPrincipal(ctx context.Context, logCtx *LogContext) error {
	taskDone, err := h.startGatewayTask()
	if err != nil {
		return err
	}
	defer taskDone()
	if err := ctx.Err(); err != nil {
		return err
	}
	if model.DB == nil {
		return fmt.Errorf("authorization database is unavailable")
	}
	var key model.ApiKey
	err = model.DB.WithContext(ctx).First(&key, logCtx.ApiKeyID).Error
	if err != nil || key.UserID != logCtx.UserID || key.Status != common.StatusEnabled || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return fmt.Errorf("API key is no longer authorized")
	}
	var user model.User
	err = model.DB.WithContext(ctx).First(&user, logCtx.UserID).Error
	if err != nil || user.Status != common.StatusEnabled {
		return fmt.Errorf("user is no longer authorized")
	}
	_, err = h.notificationScope(logCtx)
	return err
}

func subscriptionParams(req *JSONRPCRequest) (*mcp.NotificationSubscriptions, error) {
	var params struct {
		Notifications *mcp.NotificationSubscriptions `json:"notifications"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Notifications == nil {
		return nil, fmt.Errorf("notifications must be an object")
	}
	if len(params.Notifications.ResourceSubscriptions) > 128 {
		return nil, fmt.Errorf("at most 128 resource subscriptions are allowed")
	}
	return params.Notifications, nil
}

// notificationScope snapshots the effective sources, including marketplace
// configuration. Cache refreshes are intentionally absent from this contract.
func (h *GatewayHandler) notificationScope(logCtx *LogContext) ([]scopeEntry, error) {
	scope, err := h.servicesInScope(logCtx)
	if err != nil {
		return nil, err
	}
	out := make([]scopeEntry, 0, len(scope))
	for _, entry := range scope {
		if !h.userOwnedServicesAllowed(entry.svc.Source) {
			continue
		}
		if h.materializeMarketplaceConfig(entry.svc) != nil {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func (h *GatewayHandler) notificationContract(logCtx *LogContext, scope []scopeEntry) [32]byte {
	type source struct {
		ID, GroupID                                                         int64
		Name, Transport, Config, AuthType, AuthConfig, Source, PassiveToken string
		MarketplaceItemID                                                   *int64
		SharedProcess                                                       bool
	}
	sources := make([]source, 0, len(scope))
	for _, entry := range scope {
		svc := entry.svc
		sources = append(sources, source{svc.ID, entry.groupID, svc.Name, svc.TransportType, svc.Config, svc.AuthType, svc.AuthConfig, svc.Source, svc.PassiveToken, svc.MarketplaceItemID, svc.SharedProcess})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].ID < sources[j].ID })
	raw, _ := json.Marshal(struct {
		Native  bool
		Sources []source
	}{h.nativeItemsAllowed(logCtx), sources})
	return sha256.Sum256(raw)
}

type watchedNotificationSource struct {
	session  *bridge.McpSession
	accepted mcp.NotificationSubscriptions
	stop     func()
}

// attachNotificationSource bounds setup independently from the lifetime of the
// watcher. Late setup results release their leases after the caller times out.
func (h *GatewayHandler) attachNotificationSource(setupCtx, ctx context.Context, svc *model.McpService, wanted mcp.NotificationSubscriptions, uris []string, listener func(transport.Notification)) (*watchedNotificationSource, error) {
	taskDone, err := h.startGatewayTask()
	if err != nil {
		return nil, err
	}
	launched := false
	defer func() {
		if !launched {
			taskDone()
		}
	}()
	select {
	case notificationSourceSlots <- struct{}{}:
	default:
		return nil, fmt.Errorf("too many notification sources")
	}
	select {
	case notificationAttachSlots <- struct{}{}:
	case <-setupCtx.Done():
		<-notificationSourceSlots
		return nil, setupCtx.Err()
	}
	type attached struct {
		source *watchedNotificationSource
		err    error
	}
	result := make(chan attached)
	launched = true
	go func() {
		defer taskDone()
		defer func() { <-notificationAttachSlots }()
		watchCtx, cancel := context.WithCancel(ctx)
		// Only setup has a deadline. A successful watcher lives with ctx.
		deadlineStop := context.AfterFunc(setupCtx, cancel)
		var unwatch, release func()
		var cleanupOnce sync.Once
		cleanup := func() {
			cleanupOnce.Do(func() {
				cancel()
				if unwatch != nil {
					unwatch()
				}
				if release != nil {
					release()
				}
				<-notificationSourceSlots
			})
		}
		session, lease, err := h.pool.Acquire(setupCtx, svc)
		release = lease
		var source *watchedNotificationSource
		if err == nil {
			watcher, ok := session.Adapter.(transport.NotificationWatcher)
			if !ok {
				err = fmt.Errorf("upstream cannot forward notifications")
			} else {
				caps := transport.NotificationCapabilities{}
				if provider, ok := session.Adapter.(transport.NotificationCapabilitiesProvider); ok {
					caps = provider.NotificationCapabilities()
				}
				effective := mcp.NotificationSubscriptions{
					ToolsListChanged:     wanted.ToolsListChanged && caps.ToolsListChanged,
					PromptsListChanged:   wanted.PromptsListChanged && caps.PromptsListChanged,
					ResourcesListChanged: wanted.ResourcesListChanged && caps.ResourcesListChanged,
				}
				if caps.ResourceSubscribe {
					effective.ResourceSubscriptions = uris
				}
				if !effective.ToolsListChanged && !effective.PromptsListChanged && !effective.ResourcesListChanged && len(effective.ResourceSubscriptions) == 0 {
					err = fmt.Errorf("upstream does not support the requested notifications")
				} else {
					var acknowledged atomic.Pointer[mcp.NotificationSubscriptions]
					guarded := func(n transport.Notification) {
						accepted := acknowledged.Load()
						if accepted == nil || watchCtx.Err() != nil || !notificationWanted(n.Method, *accepted) {
							return
						}
						if n.Method == "notifications/resources/updated" {
							var params struct {
								URI string `json:"uri"`
							}
							if json.Unmarshal(n.Params, &params) != nil {
								return
							}
							found := false
							for _, uri := range accepted.ResourceSubscriptions {
								if uri == params.URI {
									found = true
									break
								}
							}
							if !found {
								return
							}
						}
						listener(n)
					}
					unwatch, err = watcher.WatchNotifications(watchCtx, effective.ResourceSubscriptions, guarded)
					if err != nil && len(effective.ResourceSubscriptions) > 0 && watchCtx.Err() == nil {
						effective.ResourceSubscriptions = nil
						if effective.ToolsListChanged || effective.PromptsListChanged || effective.ResourcesListChanged {
							unwatch, err = watcher.WatchNotifications(watchCtx, nil, guarded)
						}
					}
					if err == nil {
						acknowledged.Store(&effective)
						source = &watchedNotificationSource{session: session, accepted: effective, stop: cleanup}
					}
				}
			}
		}
		if !deadlineStop() && setupCtx.Err() != nil {
			err = setupCtx.Err()
		}
		if err != nil {
			cleanup()
			source = nil
		}
		select {
		case result <- attached{source, err}:
		case <-setupCtx.Done():
			cleanup()
		case <-ctx.Done():
			cleanup()
		}
	}()
	select {
	case attached := <-result:
		return attached.source, attached.err
	case <-setupCtx.Done():
		return nil, setupCtx.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// watchSources shares pooled upstream connections and rewrites resource URIs.
// Its acknowledged contract remains fixed until the stream ends. Source or
// scope changes terminate the stream so the client can obtain a fresh contract.
func (h *GatewayHandler) watchSources(parent context.Context, logCtx *LogContext, wanted mcp.NotificationSubscriptions, emit func(transport.Notification)) (mcp.NotificationSubscriptions, func(), error) {
	return h.watchSelectedSources(parent, logCtx, wanted, "", emit)
}

func selectNotificationScope(scope []scopeEntry, service string) []scopeEntry {
	if service == "" {
		return scope
	}
	selected := make([]scopeEntry, 0, 1)
	for _, entry := range scope {
		if entry.svc.Name == service {
			selected = append(selected, entry)
		}
	}
	return selected
}

func (h *GatewayHandler) watchSelectedSources(parent context.Context, logCtx *LogContext, wanted mcp.NotificationSubscriptions, service string, emit func(transport.Notification)) (mcp.NotificationSubscriptions, func(), error) {
	taskDone, err := h.startGatewayTask()
	if err != nil {
		return mcp.NotificationSubscriptions{}, nil, err
	}
	supervising := false
	defer func() {
		if !supervising {
			taskDone()
		}
	}()
	scope, err := h.notificationScope(logCtx)
	if err != nil {
		return mcp.NotificationSubscriptions{}, nil, err
	}
	scope = selectNotificationScope(scope, service)
	if !wanted.ToolsListChanged && !wanted.PromptsListChanged && !wanted.ResourcesListChanged && len(wanted.ResourceSubscriptions) == 0 {
		return mcp.NotificationSubscriptions{}, func() {}, nil
	}
	byService := map[string][]string{}
	for _, uri := range wanted.ResourceSubscriptions {
		name, upstreamURI, ok := parseGatewayResourceURI(uri)
		if !ok {
			return mcp.NotificationSubscriptions{}, nil, fmt.Errorf("resource subscriptions require newmcp://<service>/<upstream-uri>")
		}
		allowed := false
		for _, entry := range scope {
			if entry.svc.Name == name && !h.itemDisabledInOwningGroup(logCtx, entry.svc.ID, itemKindResource, upstreamURI) {
				allowed = true
				break
			}
		}
		if !allowed {
			return mcp.NotificationSubscriptions{}, nil, fmt.Errorf("resource is not accessible with this API key")
		}
		byService[name] = append(byService[name], upstreamURI)
	}
	candidates := make([]scopeEntry, 0, len(scope))
	for _, entry := range scope {
		if wanted.ToolsListChanged || wanted.PromptsListChanged || wanted.ResourcesListChanged || len(byService[entry.svc.Name]) > 0 {
			candidates = append(candidates, entry)
		}
	}
	if len(candidates) > maxNotificationSources {
		return mcp.NotificationSubscriptions{}, nil, fmt.Errorf("at most %d notification sources are allowed per stream", maxNotificationSources)
	}
	ctx, cancel := context.WithCancel(parent)
	setupCtx, setupCancel := context.WithTimeout(ctx, notificationSetupTimeout)
	defer setupCancel()
	var accepted mcp.NotificationSubscriptions
	var sources []*watchedNotificationSource
	var sourcesMu sync.Mutex
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			// Unsubscribe may wait on a stalled peer. Cleanup remains bounded by the
			// global source limit and must not hold up the downstream response.
			sourcesMu.Lock()
			for _, source := range sources {
				go source.stop()
			}
			sourcesMu.Unlock()
		})
	}
	var failureOnce sync.Once
	fail := func(err error) { failureOnce.Do(func() { reportNotificationFailure(parent, err); stop() }) }
	for _, entry := range candidates {
		svcID, svcName := entry.svc.ID, entry.svc.Name
		listener := func(notification transport.Notification) {
			if ctx.Err() != nil {
				return
			}
			listenerDone, err := h.startGatewayTask()
			if err != nil {
				return
			}
			defer listenerDone()
			if err := h.CheckPrincipal(ctx, logCtx); err != nil {
				fail(err)
				return
			}
			current, err := h.notificationScope(logCtx)
			if err != nil {
				fail(err)
				return
			}
			visible := false
			for _, entry := range current {
				if entry.svc.ID == svcID && entry.svc.Name == svcName {
					visible = true
					break
				}
			}
			if !visible {
				fail(fmt.Errorf("notification source is no longer accessible"))
				return
			}
			params := map[string]any{}
			if len(notification.Params) > 0 {
				_ = json.Unmarshal(notification.Params, &params)
			}
			if params == nil {
				params = map[string]any{}
			}
			delete(params, "_meta")
			if notification.Method == "notifications/resources/updated" {
				uri, _ := params["uri"].(string)
				if uri == "" || h.itemDisabledInOwningGroup(logCtx, svcID, itemKindResource, uri) {
					return
				}
				params["uri"] = gatewayResourceURI(svcName, uri)
			}
			params["_meta"] = map[string]any{"io.newmcp/service": svcName}
			raw, _ := json.Marshal(params)
			if ctx.Err() == nil {
				emit(transport.Notification{Method: notification.Method, Params: raw})
			}
		}
		source, err := h.attachNotificationSource(setupCtx, ctx, entry.svc, wanted, byService[svcName], listener)
		if err != nil {
			if setupCtx.Err() != nil || ctx.Err() != nil {
				stop()
				return accepted, nil, err
			}
			continue
		}
		sourcesMu.Lock()
		if ctx.Err() != nil {
			sourcesMu.Unlock()
			go source.stop()
			stop()
			return accepted, nil, ctx.Err()
		}
		sources = append(sources, source)
		sourcesMu.Unlock()
		accepted.ToolsListChanged = accepted.ToolsListChanged || source.accepted.ToolsListChanged
		accepted.PromptsListChanged = accepted.PromptsListChanged || source.accepted.PromptsListChanged
		accepted.ResourcesListChanged = accepted.ResourcesListChanged || source.accepted.ResourcesListChanged
		for _, uri := range source.accepted.ResourceSubscriptions {
			accepted.ResourceSubscriptions = append(accepted.ResourceSubscriptions, gatewayResourceURI(svcName, uri))
		}
	}
	contract := h.notificationContract(logCtx, scope)
	supervising = true
	go func() {
		defer taskDone()
		ticker := time.NewTicker(notificationCheckInterval)
		defer ticker.Stop()
		defer stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := h.CheckPrincipal(ctx, logCtx); err != nil {
					fail(err)
					return
				}
				current, err := h.notificationScope(logCtx)
				if err != nil {
					fail(err)
					return
				}
				current = selectNotificationScope(current, service)
				if h.notificationContract(logCtx, current) != contract {
					fail(fmt.Errorf("notification source configuration or scope changed; subscribe again"))
					return
				}
				for _, source := range sources {
					if !source.session.Adapter.IsConnected() {
						fail(fmt.Errorf("notification source disconnected; subscribe again"))
						return
					}
					if observer, ok := source.session.Adapter.(interface{ Done() <-chan struct{} }); ok {
						select {
						case <-observer.Done():
							fail(fmt.Errorf("notification source disconnected; subscribe again"))
							return
						default:
						}
					}
				}
			}
		}
	}()
	return accepted, stop, nil
}

func notificationWanted(method string, wanted mcp.NotificationSubscriptions) bool {
	switch method {
	case "notifications/tools/list_changed":
		return wanted.ToolsListChanged
	case "notifications/prompts/list_changed":
		return wanted.PromptsListChanged
	case "notifications/resources/list_changed":
		return wanted.ResourcesListChanged
	case "notifications/resources/updated":
		return len(wanted.ResourceSubscriptions) > 0
	default:
		return false
	}
}

func (h *GatewayHandler) OpenSubscriptions(parent context.Context, req *JSONRPCRequest, logCtx *LogContext) (*NotificationStream, *JSONRPCResponse) {
	if failure := ValidateRequest(req); failure != nil {
		return nil, failure
	}
	wanted, err := subscriptionParams(req)
	if err != nil {
		return nil, rpcFailure(req.ID, -32602, err.Error(), nil)
	}
	if !h.nativeItemsAllowed(logCtx) {
		wanted = &mcp.NotificationSubscriptions{}
	}
	ctx, cancel := context.WithCancel(parent)
	remove, err := h.registerStream(logCtx, req.ID, cancel)
	if err != nil {
		cancel()
		return nil, rpcFailure(req.ID, -32013, err.Error(), nil)
	}
	messages := make(chan JSONRPCNotification, 64)
	emit := func(notification transport.Notification) {
		params := map[string]any{}
		_ = json.Unmarshal(notification.Params, &params)
		if params == nil {
			params = map[string]any{}
		}
		meta, _ := params["_meta"].(map[string]any)
		if meta == nil {
			meta = map[string]any{}
		}
		meta[mcp.MetaKeySubscriptionID] = req.ID
		params["_meta"] = meta
		select {
		case messages <- JSONRPCNotification{JSONRPC: "2.0", Method: notification.Method, Params: params}:
		case <-ctx.Done():
		default:
			cancel() // Bound memory: close a slow consumer instead of silently dropping updates.
		}
	}
	watchCtx := withNotificationFailure(ctx, func(error) { cancel() })
	accepted, unwatch, err := h.watchSources(watchCtx, logCtx, *wanted, emit)
	if err != nil {
		remove()
		return nil, rpcFailure(req.ID, -32602, err.Error(), nil)
	}
	var once sync.Once
	stop := func() { once.Do(func() { remove(); unwatch() }) }
	monitorDone, err := h.startGatewayTask()
	if err != nil {
		stop()
		return nil, rpcFailure(req.ID, -32603, err.Error(), nil)
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
			case <-ticker.C:
				if h.CheckPrincipal(ctx, logCtx) != nil {
					cancel()
					return
				}
			}
		}
	}()
	return &NotificationStream{Messages: messages, Acknowledged: accepted, Stop: stop, Done: ctx.Done()}, nil
}

func (h *GatewayHandler) Close() {
	h.streams.mu.Lock()
	h.streams.closed = true
	for _, cancel := range h.streams.active {
		cancel()
	}
	h.streams.mu.Unlock()
	h.closeLegacySessions()
	h.closeEvents()
	h.workWG.Wait()
	h.logMu.Lock()
	h.logClosed = true
	h.logMu.Unlock()
	h.logWG.Wait()
}

func addSubscriptionID(params any, id any) map[string]any {
	raw, _ := json.Marshal(params)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{}
	}
	meta, _ := out["_meta"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	meta[mcp.MetaKeySubscriptionID] = id
	out["_meta"] = meta
	return out
}

func (h *GatewayHandler) StreamCompletion(req *JSONRPCRequest) *JSONRPCResponse {
	resp := &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"_meta": map[string]any{mcp.MetaKeySubscriptionID: req.ID}}}
	h.CompleteResponse(req, resp)
	return resp
}

func eventName(method string) string {
	return "mcp." + strings.TrimPrefix(strings.ReplaceAll(method, "/", "."), "notifications.")
}
