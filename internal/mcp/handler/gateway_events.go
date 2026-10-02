package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/internal/mcp/events"
	"github.com/mujkjk/newmcp/internal/mcp/transport"
)

type gatewayEventState struct {
	mu      sync.Mutex
	manager *events.Manager
	closed  bool
}

func (h *GatewayHandler) eventManager() *events.Manager {
	h.eventState.mu.Lock()
	defer h.eventState.mu.Unlock()
	if h.eventState.manager == nil && !h.eventState.closed {
		h.eventState.manager = events.NewManager()
	}
	return h.eventState.manager
}

func (h *GatewayHandler) closeEvents() {
	h.eventState.mu.Lock()
	h.eventState.closed = true
	manager := h.eventState.manager
	h.eventState.mu.Unlock()
	if manager != nil {
		manager.Close()
	}
}

func (h *GatewayHandler) eventPrincipal(logCtx *LogContext, raw json.RawMessage) events.Principal {
	copy := *logCtx
	var p struct {
		Arguments struct {
			Service string `json:"service"`
			URI     string `json:"uri"`
		} `json:"arguments"`
	}
	_ = json.Unmarshal(raw, &p)
	return events.Principal{ID: principalKey(&copy), Check: func(ctx context.Context) error {
		if err := h.CheckPrincipal(ctx, &copy); err != nil {
			return err
		}
		service, uri := p.Arguments.Service, p.Arguments.URI
		if event, ok := events.EventFromContext(ctx); ok {
			body, _ := json.Marshal(event.Data)
			var payload struct {
				Service string `json:"service"`
				URI     string `json:"uri"`
			}
			_ = json.Unmarshal(body, &payload)
			service, uri = payload.Service, payload.URI
		}
		if uri != "" {
			resourceService, _, ok := parseGatewayResourceURI(uri)
			if !ok || (service != "" && service != resourceService) {
				return errors.New("resource is no longer accessible")
			}
			service = resourceService
		}
		if service == "" {
			return nil
		}
		scope, err := h.servicesInScope(&copy)
		if err != nil {
			return err
		}
		for _, entry := range scope {
			if entry.svc.Name == service && h.userOwnedServicesAllowed(entry.svc.Source) {
				if uri != "" {
					_, original, _ := parseGatewayResourceURI(uri)
					if h.itemDisabledInOwningGroup(&copy, entry.svc.ID, itemKindResource, original) {
						break
					}
				}
				return nil
			}
		}
		return errors.New("event source is no longer accessible")
	}}
}

func stripRequestMeta(raw json.RawMessage) json.RawMessage {
	var params map[string]json.RawMessage
	if json.Unmarshal(raw, &params) != nil {
		return raw
	}
	delete(params, "_meta")
	stripped, _ := json.Marshal(params)
	return stripped
}

func eventRPCFailure(id any, err error) *JSONRPCResponse {
	var failure *events.RPCError
	if errors.As(err, &failure) {
		return rpcFailure(id, failure.Code, failure.Message, failure.Data)
	}
	return rpcFailure(id, -32603, err.Error(), nil)
}

func (h *GatewayHandler) eventSource(logCtx *LogContext) events.Source {
	copy := *logCtx
	return func(ctx context.Context, name string, arguments json.RawMessage, emit func(events.Event)) (func(), error) {
		var args struct {
			Service string `json:"service"`
			URI     string `json:"uri"`
		}
		if len(arguments) != 0 && json.Unmarshal(arguments, &args) != nil {
			return nil, &events.RPCError{Code: -32602, Message: "Invalid event arguments"}
		}
		if err := h.CheckPrincipal(ctx, &copy); err != nil {
			return nil, &events.RPCError{Code: -32012, Message: "Event access is no longer authorized"}
		}
		if args.Service != "" {
			scope, err := h.servicesInScope(&copy)
			if err != nil {
				return nil, err
			}
			visible := false
			for _, entry := range scope {
				if entry.svc.Name == args.Service && h.userOwnedServicesAllowed(entry.svc.Source) {
					visible = true
					break
				}
			}
			if !visible {
				return nil, &events.RPCError{Code: -32012, Message: "Service is not accessible with this API key"}
			}
		}
		wanted := mcp.NotificationSubscriptions{}
		switch name {
		case "mcp.tools.list_changed":
			wanted.ToolsListChanged = true
		case "mcp.prompts.list_changed":
			wanted.PromptsListChanged = true
		case "mcp.resources.list_changed":
			wanted.ResourcesListChanged = true
		case "mcp.resources.updated":
			service, _, ok := parseGatewayResourceURI(args.URI)
			if !ok || (args.Service != "" && args.Service != service) {
				return nil, &events.RPCError{Code: -32602, Message: "uri must identify a resource in the selected service"}
			}
			wanted.ResourceSubscriptions = []string{args.URI}
		default:
			return nil, &events.RPCError{Code: -32011, Message: "Unknown event", Data: map[string]any{"kind": "event"}}
		}
		sourceCtx := withNotificationFailure(ctx, func(err error) { events.FailSource(ctx, err) })
		accepted, stop, err := h.watchSelectedSources(sourceCtx, &copy, wanted, args.Service, func(notification transport.Notification) {
			params := map[string]any{}
			_ = json.Unmarshal(notification.Params, &params)
			meta, _ := params["_meta"].(map[string]any)
			service, _ := meta["io.newmcp/service"].(string)
			if args.Service != "" && args.Service != service {
				return
			}
			delete(params, "_meta")
			params["service"] = service
			emit(events.Event{Name: eventName(notification.Method), Data: params})
		})
		if err != nil {
			return nil, &events.RPCError{Code: -32602, Message: err.Error()}
		}
		if !notificationWanted("notifications/"+strings.ReplaceAll(strings.TrimPrefix(name, "mcp."), ".", "/"), accepted) {
			stop()
			return nil, &events.RPCError{Code: -32014, Message: "Upstream does not support this event subscription"}
		}
		return stop, nil
	}
}

func requireEventExtension(req *JSONRPCRequest) *JSONRPCResponse {
	if !IsModernRequest(req) {
		return nil
	}
	var caps struct {
		Extensions map[string]json.RawMessage `json:"extensions"`
	}
	_ = json.Unmarshal(requestMeta(req)[mcp.MetaKeyClientCapabilities], &caps)
	var settings map[string]json.RawMessage
	if json.Unmarshal(caps.Extensions[eventExtension], &settings) != nil || settings == nil {
		return rpcFailure(req.ID, -32021, "Client must opt in to the experimental events extension", map[string]any{"requiredCapabilities": map[string]any{"extensions": map[string]any{eventExtension: map[string]any{}}}})
	}
	return nil
}

func (h *GatewayHandler) handleEvents(ctx context.Context, req *JSONRPCRequest, logCtx *LogContext) *JSONRPCResponse {
	if failure := requireEventExtension(req); failure != nil {
		return failure
	}
	manager := h.eventManager()
	if manager == nil {
		return rpcFailure(req.ID, -32603, "Gateway is shutting down", nil)
	}
	result, err := manager.Handle(ctx, h.eventPrincipal(logCtx, req.Params), req.Method, stripRequestMeta(req.Params), h.eventSource(logCtx))
	if err != nil {
		return eventRPCFailure(req.ID, err)
	}
	return &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (h *GatewayHandler) OpenEventStream(parent context.Context, req *JSONRPCRequest, logCtx *LogContext) (*NotificationStream, *JSONRPCResponse) {
	if failure := ValidateRequest(req); failure != nil {
		return nil, failure
	}
	if failure := requireEventExtension(req); failure != nil {
		return nil, failure
	}
	ctx, cancel := context.WithCancel(parent)
	remove, err := h.registerStream(logCtx, req.ID, cancel)
	if err != nil {
		cancel()
		return nil, rpcFailure(req.ID, -32013, err.Error(), nil)
	}
	manager := h.eventManager()
	if manager == nil {
		remove()
		return nil, rpcFailure(req.ID, -32603, "Gateway is shutting down", nil)
	}
	incoming, unwatch, err := manager.Stream(ctx, h.eventPrincipal(logCtx, req.Params), stripRequestMeta(req.Params), h.eventSource(logCtx))
	if err != nil {
		remove()
		return nil, eventRPCFailure(req.ID, err)
	}
	messages := make(chan JSONRPCNotification, 64)
	finished := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { remove(); unwatch() }) }
	go func() {
		defer close(finished)
		defer stop()
		for {
			select {
			case <-ctx.Done():
				return
			case notification, ok := <-incoming:
				if !ok {
					return
				}
				message := JSONRPCNotification{JSONRPC: "2.0", Method: notification.Method, Params: addSubscriptionID(notification.Params, req.ID)}
				select {
				case messages <- message:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return &NotificationStream{Messages: messages, Stop: stop, Done: ctx.Done(), Finished: finished}, nil
}
