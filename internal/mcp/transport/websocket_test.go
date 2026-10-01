package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// legacyWSPeer models the unchanged XiaoZhi bridge's raw frames, including the
// two errors emitted by older Go/TypeScript and Python MCP implementations.
type legacyWSPeer struct {
	conn         *websocket.Conn
	version      string
	discoverCode int
	writeMu      sync.Mutex
	mu           sync.Mutex
	tools        []string
	failPage     bool
	handshake    []string
	cancelled    chan struct{}
	done         chan struct{}
}

type wsTestRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func newLegacyWSPeer(conn *websocket.Conn, code int, version string, configure ...func(*legacyWSPeer)) *legacyWSPeer {
	p := &legacyWSPeer{conn: conn, version: version, discoverCode: code,
		tools: []string{"echo", "wait"}, cancelled: make(chan struct{}, 1), done: make(chan struct{})}
	for _, apply := range configure {
		apply(p)
	}
	go p.run()
	return p
}

func (p *legacyWSPeer) write(v any) error {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	return p.conn.WriteJSON(v)
}

func (p *legacyWSPeer) result(id json.RawMessage, result any) {
	_ = p.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (p *legacyWSPeer) rpcError(id json.RawMessage, code int) {
	_ = p.write(map[string]any{"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": "legacy request rejected"}})
}

func (p *legacyWSPeer) run() {
	defer close(p.done)
	defer p.conn.Close()
	for {
		typ, data, err := p.conn.ReadMessage()
		if err != nil {
			return
		}
		if typ != websocket.TextMessage || strings.HasSuffix(string(data), "\n") {
			return // wire frames must stay compatible with mcp_pipe.py
		}
		var req wsTestRequest
		if json.Unmarshal(data, &req) != nil {
			return
		}
		switch req.Method {
		case "server/discover", "initialize", "notifications/initialized":
			p.mu.Lock()
			p.handshake = append(p.handshake, req.Method)
			p.mu.Unlock()
		}
		switch req.Method {
		case "server/discover":
			p.rpcError(req.ID, p.discoverCode)
		case "initialize":
			p.result(req.ID, map[string]any{"protocolVersion": p.version,
				"capabilities": map[string]any{"tools": map[string]any{"listChanged": true}},
				"serverInfo":   map[string]any{"name": "legacy-local", "version": "1"}})
		case "tools/list":
			var params struct {
				Cursor string `json:"cursor"`
			}
			_ = json.Unmarshal(req.Params, &params)
			index, _ := strconv.Atoi(params.Cursor)
			p.mu.Lock()
			names := append([]string(nil), p.tools...)
			fail := p.failPage && index > 0
			p.mu.Unlock()
			if fail {
				p.rpcError(req.ID, -32603)
				continue
			}
			tools := []any{}
			result := map[string]any{"tools": tools}
			if index < len(names) {
				result["tools"] = []any{map[string]any{"name": names[index],
					"inputSchema": map[string]any{"type": "object"}}}
				if index+1 < len(names) {
					result["nextCursor"] = strconv.Itoa(index + 1)
				}
			}
			p.result(req.ID, result)
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			if params.Name == "wait" {
				continue // force client context cancellation
			}
			go p.result(req.ID, map[string]any{"content": []any{map[string]any{
				"type": "text", "text": fmt.Sprint(params.Arguments["value"]),
			}}})
		case "ping":
			p.result(req.ID, map[string]any{})
		case "notifications/cancelled":
			select {
			case p.cancelled <- struct{}{}:
			default:
			}
		case "notifications/initialized":
		default:
			if len(req.ID) > 0 {
				p.rpcError(req.ID, -32601)
			}
		}
	}
}

func startLegacyWSAdapter(t *testing.T, code int, version string) (*SDKAdapter, *legacyWSPeer) {
	t.Helper()
	peers := make(chan *legacyWSPeer, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer upstream" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		peers <- newLegacyWSPeer(conn, code, version)
	}))
	t.Cleanup(srv.Close)
	a := NewWebSocketAdapter(1, "ws"+strings.TrimPrefix(srv.URL, "http"), map[string]string{"Authorization": "Bearer upstream"})
	t.Cleanup(func() { _ = a.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	peer := <-peers
	t.Cleanup(func() { _ = peer.conn.Close() })
	return a, peer
}

func TestWebSocketLegacyHandshake(t *testing.T) {
	for _, code := range []int{-32601, -32602} {
		for _, version := range []string{"2024-11-05", "2025-03-26"} {
			t.Run(fmt.Sprintf("%d_%s", code, version), func(t *testing.T) {
				a, peer := startLegacyWSAdapter(t, code, version)
				if a.GetProtocolVersion() != version || a.GetType() != TypeWebSocket {
					t.Fatalf("negotiation: version=%s type=%s", a.GetProtocolVersion(), a.GetType())
				}
				if tools := a.GetTools(); len(tools) != 2 || tools[0].Name != "echo" || tools[1].Name != "wait" {
					t.Fatalf("paginated tools: %v", tools)
				}
				peer.mu.Lock()
				handshake := strings.Join(peer.handshake, ",")
				peer.mu.Unlock()
				if handshake != "server/discover,initialize,notifications/initialized" {
					t.Fatalf("same-connection fallback: %s", handshake)
				}
				if err := a.Ping(context.Background()); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestWebSocketConcurrentCallsAndCancellation(t *testing.T) {
	a, peer := startLegacyWSAdapter(t, -32602, "2024-11-05")
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			raw, err := a.Call(ctx, "tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"value": i}})
			if err != nil || !strings.Contains(string(raw), fmt.Sprintf(`"text":"%d"`, i)) {
				t.Errorf("call %d: %s (%v)", i, raw, err)
			}
		})
	}
	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := a.Call(ctx, "tools/call", map[string]any{"name": "wait"}); err == nil {
		t.Fatal("waiting tool should be cancelled")
	}
	select {
	case <-peer.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("missing notifications/cancelled")
	}
	if !a.IsConnected() || a.Ping(context.Background()) != nil {
		t.Fatal("one cancelled call must leave the shared connection usable")
	}
}

func TestWebSocketToolRefreshAndDisconnect(t *testing.T) {
	a, peer := startLegacyWSAdapter(t, -32602, "2025-03-26")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	peer.mu.Lock()
	peer.tools = []string{"manual"}
	peer.mu.Unlock()
	if err := a.RefreshTools(ctx); err != nil {
		t.Fatal(err)
	}
	if tools := a.GetTools(); len(tools) != 1 || tools[0].Name != "manual" {
		t.Fatalf("manual refresh did not discover changed tools: %v", tools)
	}
	peer.mu.Lock()
	peer.tools, peer.failPage = []string{"first", "second"}, true
	peer.mu.Unlock()
	if err := a.RefreshTools(ctx); err == nil {
		t.Fatal("partial paginated catalog must fail")
	}
	if tools := a.GetTools(); len(tools) != 1 || tools[0].Name != "manual" {
		t.Fatalf("failed refresh replaced the old snapshot: %v", tools)
	}
	updated := make(chan struct{}, 1)
	a.SetToolsChangedHandler(func() {
		// Calling back into the adapter must not deadlock.
		if tools := a.GetTools(); len(tools) == 1 && tools[0].Name == "replacement" {
			updated <- struct{}{}
		}
	})
	peer.mu.Lock()
	peer.tools, peer.failPage = []string{"replacement"}, false
	peer.mu.Unlock()
	if err := peer.write(map[string]any{"jsonrpc": "2.0", "method": "notifications/tools/list_changed"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-updated:
	case <-time.After(5 * time.Second):
		t.Fatal("tool-change notification did not publish a refreshed catalog")
	}
	_ = peer.conn.Close()
	select {
	case <-a.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect did not end adapter lifecycle")
	}
	if a.IsConnected() {
		t.Fatal("adapter remains connected after peer disconnect")
	}
}

func TestWebSocketFailedInitializationClosesConnection(t *testing.T) {
	for _, invalidVersion := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid_version_%v", invalidVersion), func(t *testing.T) {
			peers := make(chan *legacyWSPeer, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				version := "2024-11-05"
				if invalidVersion {
					version = "1999-01-01"
				}
				peers <- newLegacyWSPeer(conn, -32602, version, func(p *legacyWSPeer) { p.failPage = !invalidVersion })
			}))
			defer srv.Close()
			a := NewWebSocketAdapter(1, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			defer a.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.Connect(ctx); err == nil {
				t.Fatal("invalid version or incomplete catalog must fail initialization")
			}
			peer := <-peers
			defer peer.conn.Close()
			select {
			case <-peer.done:
			case <-time.After(5 * time.Second):
				t.Fatal("failed initialization leaked its WebSocket connection")
			}
			select {
			case <-a.Done():
			default:
				t.Fatal("failed initialization did not close Done")
			}
		})
	}
}

func TestPassiveWebSocketAdapter(t *testing.T) {
	adapters := make(chan *SDKAdapter, 1)
	errors := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			errors <- err
			return
		}
		a := NewPassiveWSAdapter(42, conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Connect(ctx); err != nil {
			errors <- err
			return
		}
		adapters <- a
	}))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = newLegacyWSPeer(conn, -32602, "2024-11-05")
	var a *SDKAdapter
	select {
	case a = <-adapters:
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("passive initialization timed out")
	}
	defer a.Close()
	if a.GetType() != TypePassiveWS || len(a.GetTools()) != 2 {
		t.Fatal("passive tools were not discovered")
	}
	if err := a.Ping(context.Background()); err != nil {
		t.Fatalf("returning from HTTP handler killed accepted connection: %v", err)
	}
	if _, err := a.Call(context.Background(), "tools/call", map[string]any{"name": "echo", "arguments": map[string]any{"value": "local"}}); err != nil {
		t.Fatal(err)
	}
}
