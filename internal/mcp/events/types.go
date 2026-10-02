// Package events implements the experimental MCP Events design sketch. It is
// separate from the versioned MCP change-notification subscription protocol.
package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type Event struct {
	EventID   string  `json:"eventId"`
	Name      string  `json:"name"`
	Timestamp string  `json:"timestamp"`
	Data      any     `json:"data"`
	Cursor    *string `json:"cursor,omitempty"`
}

type Notification struct {
	Method string `json:"method"`
	Params any    `json:"params"`
}

// Principal is supplied by the gateway's authentication and scope resolver.
// Check must revalidate the current key, group and service permissions. It is
// invoked for every request and again immediately before each delivery.
type Principal struct {
	ID    string
	Check func(context.Context) error
}

type eventContextKey struct{}

// EventFromContext lets the gateway recheck the exact service/resource of a
// cached event before delivery. It is present on poll-event checks, push event
// checks and EVERY webhook attempt, including retries.
func EventFromContext(ctx context.Context) (Event, bool) {
	e, ok := ctx.Value(eventContextKey{}).(Event)
	return e, ok
}

func checkEvent(ctx context.Context, p Principal, e Event) error {
	return check(context.WithValue(ctx, eventContextKey{}, e), p)
}

type sourceFailureContextKey struct{}

// FailSource reports an asynchronous failure from the source supervisor. Only
// the context passed to a currently live Source carries this private hook.
// Stop runs asynchronously so a supervisor may safely report its own failure
// even when its teardown callback waits for that supervisor to finish.
func FailSource(ctx context.Context, err error) bool {
	fail, ok := ctx.Value(sourceFailureContextKey{}).(func(error) bool)
	return ok && fail(err)
}

// Source attaches an authorized listener. It must honor ctx and return an
// idempotent teardown callback. It may emit synchronously before returning.
// emit is nonblocking with respect to subscribers and ignores events after stop.
type Source func(ctx context.Context, name string, arguments json.RawMessage, emit func(Event)) (stop func(), err error)

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

func invalid(reason string) error {
	return &RPCError{Code: -32602, Message: "InvalidParams", Data: map[string]any{"reason": reason}}
}

func forbidden() *RPCError {
	return &RPCError{Code: -32012, Message: "Forbidden", Data: map[string]any{"reason": "Access revoked or unavailable"}}
}

func exhausted(limit string, max int) error {
	return &RPCError{Code: -32013, Message: "ResourceExhausted", Data: map[string]any{"limit": limit, "max": max}}
}

func sanitized(err error) *RPCError {
	var rpc *RPCError
	if errors.As(err, &rpc) {
		return rpc
	}
	return &RPCError{Code: -32603, Message: "UpstreamError"}
}

func check(ctx context.Context, p Principal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.Check != nil {
		if err := p.Check(ctx); err != nil {
			return forbidden()
		}
	}
	return nil
}

type request struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Cursor    *string         `json:"cursor"`
	MaxAgeMS  *int64          `json:"maxAgeMs"`
	MaxEvents *int            `json:"maxEvents"`
	TTLMS     json.RawMessage `json:"ttlMs"`
	Delivery  *struct {
		Mode   string `json:"mode,omitempty"`
		URL    string `json:"url"`
		Secret string `json:"secret,omitempty"`
	} `json:"delivery"`
}

func decode(raw json.RawMessage, value any) error {
	if len(raw) > 16<<10 {
		return invalid("event parameters exceed the supported size")
	}
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return invalid("params must be an object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return invalid("malformed or unsupported parameters")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return invalid("params must contain one object")
	}
	return nil
}

var eventDescriptions = []struct{ name, description string }{
	{"mcp.tools.list_changed", "An authorized upstream MCP service changed its tool catalog."},
	{"mcp.resources.list_changed", "An authorized upstream MCP service changed its resource catalog."},
	{"mcp.prompts.list_changed", "An authorized upstream MCP service changed its prompt catalog."},
	{"mcp.resources.updated", "A subscribed resource of an authorized upstream MCP service changed."},
}

func catalog() []map[string]any {
	items := make([]map[string]any, 0, len(eventDescriptions))
	for _, d := range eventDescriptions {
		input := map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"service": map[string]any{"type": "string", "minLength": 1}, "uri": map[string]any{"type": "string", "minLength": 1}},
		}
		if d.name == "mcp.resources.updated" {
			input["required"] = []string{"uri"}
		}
		items = append(items, map[string]any{
			"name": d.name, "description": d.description,
			"delivery": []string{"poll", "push", "webhook"}, "inputSchema": input,
			"payloadSchema": map[string]any{
				"type": "object", "required": []string{"service"},
				"properties": map[string]any{"service": map[string]any{"type": "string"}, "uri": map[string]any{"type": "string"}},
			},
		})
	}
	return items
}

func parseRequest(raw json.RawMessage) (request, error) {
	var p request
	if err := decode(raw, &p); err != nil {
		return p, err
	}
	known := false
	for _, d := range eventDescriptions {
		known = known || p.Name == d.name
	}
	if !known {
		return p, &RPCError{Code: -32011, Message: "NotFound", Data: map[string]any{"kind": "event"}}
	}
	if len(p.Arguments) == 0 {
		p.Arguments = json.RawMessage(`{}`)
	}
	var args map[string]string
	if err := decode(p.Arguments, &args); err != nil || args == nil {
		return p, invalid("arguments must be an object of strings")
	}
	for k, v := range args {
		if (k != "service" && k != "uri") || strings.TrimSpace(v) == "" || len(v) > 4096 {
			return p, invalid("unsupported or empty event argument")
		}
	}
	if p.Name == "mcp.resources.updated" && args["uri"] == "" {
		return p, invalid("mcp.resources.updated requires arguments.uri")
	}
	// encoding/json sorts map keys. Only string-valued arguments are allowed, so
	// this is canonical equality without JSON number-normalization ambiguity.
	p.Arguments, _ = json.Marshal(args)
	if p.MaxAgeMS != nil && *p.MaxAgeMS < 0 {
		return p, invalid("maxAgeMs must be a nonnegative integer")
	}
	if p.MaxEvents != nil && *p.MaxEvents <= 0 {
		return p, invalid("maxEvents must be a positive integer")
	}
	return p, nil
}
