package router

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/internal/mcp/bridge"
	"github.com/mujkjk/newmcp/internal/mcp/handler"
	"github.com/mujkjk/newmcp/internal/mcp/virtual"
	"github.com/mujkjk/newmcp/model"
	"github.com/mujkjk/newmcp/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const protocol2026Version = "2026-07-28"

// These tests deliberately use the production Gin routes and a real SDK MCP
// server. The upstream subscription counters observe actual HTTP lifetimes,
// rather than mocking the gateway's watcher or transport implementation.
type protocol2026Fixture struct {
	server    *httptest.Server
	db        *gorm.DB
	upstreams []*httptest.Server
	key       string
}

type protocol2026Upstream struct {
	server       *mcp.Server
	service      *model.McpService
	group        *model.McpGroup
	active       atomic.Int64
	subscribed   atomic.Int64
	unsubscribed atomic.Int64
}

func newProtocol2026Fixture(t *testing.T) *protocol2026Fixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "protocol.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Option{}, &model.User{}, &model.McpService{}, &model.McpServiceKey{},
		&model.McpGroup{}, &model.McpGroupService{}, &model.McpGroupTool{}, &model.McpGroupItem{},
		&model.ApiKey{}, &model.McpCallLog{}, &model.SmartSearchConfig{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldPool, oldServicePool, oldHandler := model.DB, SessionPool, service.SessionPool, gatewayHandler
	model.DB = db
	pool := bridge.NewSessionPool()
	SessionPool, service.SessionPool = pool, pool
	model.InitOptionMap()
	if err := db.Create(&model.User{ID: 1, Username: "protocol-owner", Status: 1, Group: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	gateway := handler.NewGatewayHandler(pool, bridge.NewToolRouter(pool), virtual.NewVirtualToolRegistry())
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	SetMCPRouter(engine, gateway)
	serveFrontend(engine)
	f := &protocol2026Fixture{db: db, server: httptest.NewServer(engine)}
	if err := model.UpdateOption("ServerAddress", f.server.URL); err != nil {
		t.Fatal(err)
	}
	f.key = f.addKey(t, "all", `{"groups":["*"]}`)
	t.Cleanup(func() {
		gateway.Close()
		pool.CloseAll()
		f.server.Close()
		for _, upstream := range f.upstreams {
			upstream.Close()
		}
		_ = sqlDB.Close()
		model.DB, SessionPool, service.SessionPool, gatewayHandler = oldDB, oldPool, oldServicePool, oldHandler
	})
	return f
}

func (f *protocol2026Fixture) addKey(t *testing.T, name, permissions string) string {
	t.Helper()
	key := "sk-protocol2026-" + name
	hash := sha256.Sum256([]byte(key))
	if err := f.db.Create(&model.ApiKey{UserID: 1, Name: name, KeyHash: hex.EncodeToString(hash[:]), Permissions: permissions, Status: 1, UnlimitedQuota: true}).Error; err != nil {
		t.Fatal(err)
	}
	return key
}

func (f *protocol2026Fixture) addUpstream(t *testing.T, name string) *protocol2026Upstream {
	t.Helper()
	u := &protocol2026Upstream{}
	u.server = mcp.NewServer(&mcp.Implementation{Name: name, Version: "1"}, &mcp.ServerOptions{
		SubscribeHandler:   func(context.Context, *mcp.SubscribeRequest) error { u.subscribed.Add(1); return nil },
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error { u.unsubscribed.Add(1); return nil },
	})
	u.server.AddTool(&mcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: name + "-response"}}}, nil
		})
	for _, uri := range []string{"memo://one", "memo://two", "memo://中文"} {
		u.server.AddResource(&mcp.Resource{URI: uri, Name: uri}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: name + "-resource"}}}, nil
		})
	}
	u.server.AddPrompt(&mcp.Prompt{Name: "review"}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{}}, nil
	})
	sdkHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return u.server }, &mcp.StreamableHTTPOptions{Stateless: true})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count only resource listen streams; the SDK adapter also holds one
		// list-change stream for its catalog throughout the pooled connection.
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(body))
			var req struct {
				Method string `json:"method"`
				Params struct {
					Notifications struct {
						URIs []string `json:"resourceSubscriptions"`
					} `json:"notifications"`
				} `json:"params"`
			}
			_ = json.Unmarshal(body, &req)
			if req.Method == "subscriptions/listen" && len(req.Params.Notifications.URIs) > 0 {
				u.active.Add(1)
				defer u.active.Add(-1)
			}
		}
		sdkHandler.ServeHTTP(w, r)
	}))
	f.upstreams = append(f.upstreams, upstream)
	config, _ := json.Marshal(map[string]any{"url": upstream.URL})
	u.service = &model.McpService{UserID: 1, Name: name, TransportType: common.TransportStreamableHTTP, Config: string(config), Source: "user", Status: 1,
		ToolsCache: `[{"name":"echo","inputSchema":{"type":"object"}}]`}
	if err := f.db.Create(u.service).Error; err != nil {
		t.Fatal(err)
	}
	u.group = &model.McpGroup{UserID: 1, Name: name, EndpointSlug: name, AutoDiscover: true, Status: 1, ExposeMode: "direct"}
	if err := u.group.Insert(); err != nil {
		t.Fatal(err)
	}
	if err := model.AddServicesToGroup(u.group.ID, []int64{u.service.ID}); err != nil {
		t.Fatal(err)
	}
	return u
}

func protocol2026Request(id any, method string, fields map[string]any) map[string]any {
	params := map[string]any{"_meta": map[string]any{mcp.MetaKeyProtocolVersion: protocol2026Version, mcp.MetaKeyClientCapabilities: map[string]any{}, mcp.MetaKeyClientInfo: map[string]any{"name": "route-regression", "version": "1"}}}
	for k, v := range fields {
		params[k] = v
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
}

func (f *protocol2026Fixture) httpRequest(t *testing.T, key, path string, body map[string]any) *http.Request {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, f.server.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-API-Key", key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if params, ok := body["params"].(map[string]any); ok {
		if meta, ok := params["_meta"].(map[string]any); ok {
			if version, ok := meta[mcp.MetaKeyProtocolVersion].(string); ok {
				req.Header.Set("MCP-Protocol-Version", version)
			}
			req.Header.Set("Mcp-Method", body["method"].(string))
		}
	}
	return req
}

func protocol2026RPC(t *testing.T, req *http.Request) (int, http.Header, map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
	defer cancel()
	response, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var reply map[string]any
	if len(raw) > 0 && json.Unmarshal(raw, &reply) != nil {
		t.Fatalf("HTTP %d did not return JSON: %s", response.StatusCode, raw)
	}
	return response.StatusCode, response.Header, reply
}

func protocol2026Error(t *testing.T, reply map[string]any, code int) {
	t.Helper()
	err, ok := reply["error"].(map[string]any)
	if !ok || err["code"] != float64(code) {
		t.Fatalf("want RPC error %d; got %#v", code, reply)
	}
}

func protocol2026Wait(t *testing.T, description string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(description)
}

type protocol2026SSE struct {
	body   io.ReadCloser
	frames <-chan map[string]any
}

func protocol2026OpenSSE(t *testing.T, req *http.Request) *protocol2026SSE {
	t.Helper()
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("listen failed: HTTP %d %s", response.StatusCode, raw)
	}
	frames := make(chan map[string]any, 32)
	go func() {
		defer close(frames)
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
				var frame map[string]any
				if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame) == nil {
					frames <- frame
				}
			}
		}
	}()
	t.Cleanup(func() { _ = response.Body.Close() })
	return &protocol2026SSE{body: response.Body, frames: frames}
}

func (s *protocol2026SSE) next(t *testing.T) map[string]any {
	t.Helper()
	select {
	case frame, ok := <-s.frames:
		if !ok {
			t.Fatal("SSE closed before the expected notification")
		}
		return frame
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE notification")
	}
	return nil
}

func protocol2026AssertNotification(t *testing.T, frame map[string]any, method string, id any) map[string]any {
	t.Helper()
	if frame["method"] != method {
		t.Fatalf("want %s, got %#v", method, frame)
	}
	params, ok := frame["params"].(map[string]any)
	if !ok {
		t.Fatalf("missing notification params: %#v", frame)
	}
	if id != nil {
		meta, ok := params["_meta"].(map[string]any)
		if !ok || meta[mcp.MetaKeySubscriptionID] != id {
			t.Fatalf("wrong original subscription ID: %#v", frame)
		}
	}
	return params
}

func TestProtocol2026HTTPDiscoveryAndValidation(t *testing.T) {
	f := newProtocol2026Fixture(t)
	status, headers, reply := protocol2026RPC(t, f.httpRequest(t, f.key, "/mcp", protocol2026Request("discover", "server/discover", nil)))
	if status != http.StatusOK || headers.Get("Mcp-Session-Id") != "" {
		t.Fatalf("modern discovery created a legacy session: %d %#v", status, headers)
	}
	result, ok := reply["result"].(map[string]any)
	if !ok || result["resultType"] != "complete" || result["ttlMs"] != float64(0) || result["cacheScope"] != "private" {
		t.Fatalf("missing complete discovery result: %#v", reply)
	}
	versions, _ := result["supportedVersions"].([]any)
	if len(versions) == 0 || versions[0] != protocol2026Version {
		t.Fatalf("wrong supportedVersions: %#v", result)
	}
	meta, _ := result["_meta"].(map[string]any)
	info, _ := meta[mcp.MetaKeyServerInfo].(map[string]any)
	if info["name"] != "newmcp" || info["version"] == "" {
		t.Fatalf("serverInfo belongs in result metadata: %#v", result)
	}
	caps, _ := result["capabilities"].(map[string]any)
	if _, ok := caps["resources"]; !ok {
		t.Fatalf("direct discovery did not advertise resources: %#v", caps)
	}
	_, _, smartReply := protocol2026RPC(t, f.httpRequest(t, f.key, "/smart/mcp", protocol2026Request("smart", "server/discover", nil)))
	smartCaps := smartReply["result"].(map[string]any)["capabilities"].(map[string]any)
	if _, ok := smartCaps["resources"]; ok {
		t.Fatalf("smart discovery advertised native resources: %#v", smartCaps)
	}

	for _, tc := range []struct {
		name         string
		modify       func(map[string]any, *http.Request)
		status, code int
	}{
		{"missing version header", func(_ map[string]any, r *http.Request) { r.Header.Del("MCP-Protocol-Version") }, 400, -32020},
		{"mismatched version header", func(_ map[string]any, r *http.Request) { r.Header.Set("MCP-Protocol-Version", "2025-11-25") }, 400, -32020},
		{"missing method header", func(_ map[string]any, r *http.Request) { r.Header.Del("Mcp-Method") }, 400, -32020},
		{"mismatched method header", func(_ map[string]any, r *http.Request) { r.Header.Set("Mcp-Method", "tools/list") }, 400, -32020},
		{"missing metadata version", func(b map[string]any, _ *http.Request) {
			delete(b["params"].(map[string]any)["_meta"].(map[string]any), mcp.MetaKeyProtocolVersion)
		}, 400, -32602},
		{"missing client capabilities", func(b map[string]any, _ *http.Request) {
			delete(b["params"].(map[string]any)["_meta"].(map[string]any), mcp.MetaKeyClientCapabilities)
		}, 400, -32602},
		{"non-object client capabilities", func(b map[string]any, _ *http.Request) {
			b["params"].(map[string]any)["_meta"].(map[string]any)[mcp.MetaKeyClientCapabilities] = []any{}
		}, 400, -32602},
		{"unknown modern method", func(b map[string]any, r *http.Request) {
			b["method"] = "missing/method"
			r.Header.Set("Mcp-Method", "missing/method")
		}, 404, -32601},
		{"removed initialize", func(b map[string]any, r *http.Request) {
			b["method"] = "initialize"
			r.Header.Set("Mcp-Method", "initialize")
		}, 404, -32601},
		{"unsupported future version", func(b map[string]any, r *http.Request) {
			b["params"].(map[string]any)["_meta"].(map[string]any)[mcp.MetaKeyProtocolVersion] = "2099-01-01"
			r.Header.Set("MCP-Protocol-Version", "2099-01-01")
		}, 400, -32022},
		{"unsupported past version", func(b map[string]any, r *http.Request) {
			b["params"].(map[string]any)["_meta"].(map[string]any)[mcp.MetaKeyProtocolVersion] = "1900-01-01"
			r.Header.Set("MCP-Protocol-Version", "1900-01-01")
		}, 400, -32022},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := protocol2026Request(tc.name, "server/discover", nil)
			req := f.httpRequest(t, f.key, "/mcp", body)
			tc.modify(body, req)
			raw, _ := json.Marshal(body)
			req.Body = io.NopCloser(bytes.NewReader(raw))
			req.ContentLength = int64(len(raw))
			status, _, reply := protocol2026RPC(t, req)
			if status != tc.status {
				t.Fatalf("want HTTP %d, got %d: %#v", tc.status, status, reply)
			}
			protocol2026Error(t, reply, tc.code)
			if tc.code == -32022 {
				data, _ := reply["error"].(map[string]any)["data"].(map[string]any)
				if data["requested"] == nil || data["supported"] == nil {
					t.Fatalf("missing version negotiation error data: %#v", reply)
				}
			}
		})
	}
}

func TestProtocol2026LegacyHTTPSessionAndGET(t *testing.T) {
	f := newProtocol2026Fixture(t)
	body := map[string]any{"jsonrpc": "2.0", "id": "legacy", "method": "initialize", "params": map[string]any{"protocolVersion": protocol2026Version, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "legacy-test", "version": "1"}}}
	status, headers, reply := protocol2026RPC(t, f.httpRequest(t, f.key, "/mcp", body))
	result, _ := reply["result"].(map[string]any)
	sessionID := headers.Get("Mcp-Session-Id")
	if status != 200 || result["protocolVersion"] != "2025-11-25" || sessionID == "" {
		t.Fatalf("legacy initialize must negotiate a handshake-era revision and session: HTTP %d %#v %#v", status, headers, reply)
	}
	initialized := f.httpRequest(t, f.key, "/mcp", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	initialized.Header.Set("Mcp-Session-Id", sessionID)
	status, _, _ = protocol2026RPC(t, initialized)
	if status != http.StatusAccepted {
		t.Fatalf("initialized notification: HTTP %d", status)
	}
	for _, path := range []string{"/mcp", "/smart/mcp", "/mcp/group/missing"} {
		req, _ := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
		req.Header.Set("X-API-Key", f.key)
		status, headers, _ := protocol2026RPC(t, req)
		if status != 405 || strings.Contains(headers.Get("Content-Type"), "html") || headers.Get("Allow") != "POST" {
			t.Fatalf("GET %s must return JSON HTTP405: %d %#v", path, status, headers)
		}
	}
	get, _ := http.NewRequest(http.MethodGet, f.server.URL+"/mcp", nil)
	get.Header.Set("X-API-Key", f.key)
	get.Header.Set("Mcp-Session-Id", sessionID)
	get.Header.Set("Accept", "text/event-stream")
	sse := protocol2026OpenSSE(t, get)
	_ = sse.body.Close()
	otherKey := f.addKey(t, "other", `{"groups":["*"]}`)
	crossKeyPost := f.httpRequest(t, otherKey, "/mcp", map[string]any{"jsonrpc": "2.0", "id": "cross-key", "method": "tools/list"})
	crossKeyPost.Header.Set("Mcp-Session-Id", sessionID)
	status, _, _ = protocol2026RPC(t, crossKeyPost)
	if status != 404 {
		t.Fatalf("legacy POST accepted another API key's session: HTTP %d", status)
	}
	get.Header.Set("X-API-Key", otherKey)
	status, _, _ = protocol2026RPC(t, get)
	if status != 404 {
		t.Fatalf("legacy session crossed API-key boundary: HTTP %d", status)
	}
	get.Header.Set("X-API-Key", f.key)
	get.Header.Set("MCP-Protocol-Version", protocol2026Version)
	status, _, _ = protocol2026RPC(t, get)
	if status != 405 {
		t.Fatalf("modern GET accepted legacy session: HTTP %d", status)
	}
}

func TestProtocol2026HTTPSubscriptionsAndDisconnect(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	wantedURI := "newmcp://alpha/memo://one"
	listen := protocol2026Request("listen-http", "subscriptions/listen", map[string]any{"notifications": map[string]any{"toolsListChanged": true, "resourceSubscriptions": []string{wantedURI}}})
	sse := protocol2026OpenSSE(t, f.httpRequest(t, f.key, "/mcp", listen))
	ack := protocol2026AssertNotification(t, sse.next(t), "notifications/subscriptions/acknowledged", "listen-http")
	accepted, _ := ack["notifications"].(map[string]any)
	if accepted["toolsListChanged"] != true || fmt.Sprint(accepted["resourceSubscriptions"]) != "["+wantedURI+"]" {
		t.Fatalf("ack must report actual upstream subscriptions: %#v", ack)
	}
	protocol2026Wait(t, "upstream resource stream was never established", func() bool { return u.active.Load() == 1 })
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	params := protocol2026AssertNotification(t, sse.next(t), "notifications/resources/updated", "listen-http")
	if params["uri"] != wantedURI {
		t.Fatalf("upstream resource URI not rewritten: %#v", params)
	}
	u.server.AddTool(&mcp.Tool{Name: "new-tool", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	protocol2026AssertNotification(t, sse.next(t), "notifications/tools/list_changed", "listen-http")
	_ = sse.body.Close()
	protocol2026Wait(t, "disconnect did not release the upstream resource watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 1 })
	// Reusing the original ID proves disconnect also removed the registry entry.
	sse = protocol2026OpenSSE(t, f.httpRequest(t, f.key, "/mcp", listen))
	protocol2026AssertNotification(t, sse.next(t), "notifications/subscriptions/acknowledged", "listen-http")
	_ = sse.body.Close()
	protocol2026Wait(t, "reopened watcher was not released", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 2 })
}

func TestProtocol2026ResourceScopeAndRuntimeFiltering(t *testing.T) {
	f := newProtocol2026Fixture(t)
	alpha, beta := f.addUpstream(t, "alpha"), f.addUpstream(t, "beta")
	alphaKey := f.addKey(t, "alpha-only", `{"groups":["alpha"]}`)
	for _, tc := range []struct{ name, key, path, uri string }{
		{"API key groups", alphaKey, "/mcp", "newmcp://beta/memo://one"},
		{"group endpoint", f.key, "/mcp/group/alpha", "newmcp://beta/memo://one"},
		{"key cannot select another group", alphaKey, "/mcp/group/beta", "newmcp://beta/memo://one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := protocol2026Request(tc.name, "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourceSubscriptions": []string{tc.uri}}})
			status, _, reply := protocol2026RPC(t, f.httpRequest(t, tc.key, tc.path, body))
			if status != 400 {
				t.Fatalf("inaccessible subscription must fail before SSE: %d %#v", status, reply)
			}
			protocol2026Error(t, reply, -32602)
		})
	}
	if beta.active.Load() != 0 || beta.subscribed.Load() != 0 {
		t.Fatal("unauthorized beta upstream received a resource subscription")
	}
	listen := protocol2026Request("runtime-filter", "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourceSubscriptions": []string{"newmcp://alpha/memo://one", "newmcp://alpha/memo://two"}}})
	sse := protocol2026OpenSSE(t, f.httpRequest(t, alphaKey, "/mcp/group/alpha", listen))
	protocol2026AssertNotification(t, sse.next(t), "notifications/subscriptions/acknowledged", "runtime-filter")
	if err := f.db.Create(&model.McpGroupItem{GroupID: alpha.group.ID, ServiceID: alpha.service.ID, ItemKind: "resource", ItemKey: "memo://one", Enabled: false}).Error; err != nil {
		t.Fatal(err)
	}
	for _, uri := range []string{"memo://one", "memo://two"} {
		if err := alpha.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: uri}); err != nil {
			t.Fatal(err)
		}
	}
	params := protocol2026AssertNotification(t, sse.next(t), "notifications/resources/updated", "runtime-filter")
	if params["uri"] != "newmcp://alpha/memo://two" {
		t.Fatalf("disabled resource leaked through an existing stream: %#v", params)
	}
	_ = sse.body.Close()
	protocol2026Wait(t, "filtered stream leaked upstream watchers", func() bool { return alpha.active.Load() == 0 })
	disabled := protocol2026Request("disabled", "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourceSubscriptions": []string{"newmcp://alpha/memo://one"}}})
	status, _, reply := protocol2026RPC(t, f.httpRequest(t, alphaKey, "/mcp/group/alpha", disabled))
	if status != 400 {
		t.Fatalf("disabled resource accepted: %d %#v", status, reply)
	}
	protocol2026Error(t, reply, -32602)
}

type protocol2026AuthTransport struct{ key string }

func (a protocol2026AuthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	copy.Header = r.Header.Clone()
	copy.Header.Set("X-API-Key", a.key)
	return http.DefaultTransport.RoundTrip(copy)
}

func TestProtocol2026OfficialSDKClientEndToEnd(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	updates := make(chan string, 4)
	client := mcp.NewClient(&mcp.Implementation{Name: "official-sdk-client", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}, ResourceUpdatedHandler: func(_ context.Context, r *mcp.ResourceUpdatedNotificationRequest) { updates <- r.Params.URI }})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: f.server.URL + "/mcp", HTTPClient: &http.Client{Transport: protocol2026AuthTransport{f.key}}}, &mcp.ClientSessionOptions{ProtocolVersion: protocol2026Version})
	if err != nil {
		t.Fatalf("official SDK could not discover production gateway: %v", err)
	}
	defer session.Close()
	if session.InitializeResult().ProtocolVersion != protocol2026Version {
		t.Fatalf("SDK unexpectedly fell back to legacy: %+v", session.InitializeResult())
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != "alpha__echo" {
		t.Fatalf("SDK catalog did not reach scoped upstream: %+v, %v", tools, err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "alpha__echo", Arguments: map[string]any{}})
	if err != nil || len(result.Content) != 1 {
		t.Fatalf("SDK tool call: %+v, %v", result, err)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || text.Text != "alpha-response" {
		t.Fatalf("tool call did not execute real upstream: %+v", result)
	}
	uri := "newmcp://alpha/memo://one"
	if err := session.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); err != nil {
		t.Fatal(err)
	}
	protocol2026Wait(t, "SDK subscribe did not open a resource stream", func() bool { return u.active.Load() == 1 })
	if err := u.server.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-updates:
		if got != uri {
			t.Fatalf("SDK notification URI = %q", got)
		}
	case <-ctx.Done():
		t.Fatal("SDK did not receive the real upstream resource notification")
	}
	if err := session.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri}); err != nil {
		t.Fatal(err)
	}
	protocol2026Wait(t, "SDK unsubscribe did not release upstream watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 1 })
	// Header mirroring must decode the spec's ASCII base64 sentinel first.
	read := f.httpRequest(t, f.key, "/mcp", protocol2026Request("unicode", "resources/read", map[string]any{"uri": "newmcp://alpha/memo://中文"}))
	read.Header.Set("Mcp-Name", "=?base64?"+base64.StdEncoding.EncodeToString([]byte("newmcp://alpha/memo://中文"))+"?=")
	status, _, reply := protocol2026RPC(t, read)
	if status != 200 || reply["error"] != nil {
		t.Fatalf("encoded Unicode Mcp-Name rejected: %d %#v", status, reply)
	}
}

func protocol2026WS(t *testing.T, f *protocol2026Fixture) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial(strings.Replace(f.server.URL, "http://", "ws://", 1)+"/mcp/ws", http.Header{"X-API-Key": []string{f.key}})
	if err != nil {
		if response != nil {
			t.Fatalf("WebSocket upgrade HTTP%d: %v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func protocol2026WSRead(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var frame map[string]any
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestProtocol2026WebSocketSubscriptionsAndLegacy(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	conn := protocol2026WS(t, f)
	if err := conn.WriteJSON(protocol2026Request("discover-ws", "server/discover", nil)); err != nil {
		t.Fatal(err)
	}
	discover := protocol2026WSRead(t, conn)
	if discover["id"] != "discover-ws" || discover["result"].(map[string]any)["resultType"] != "complete" {
		t.Fatalf("WebSocket discovery: %#v", discover)
	}
	listen := protocol2026Request("listen-ws", "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourceSubscriptions": []string{"newmcp://alpha/memo://one"}}})
	if err := conn.WriteJSON(listen); err != nil {
		t.Fatal(err)
	}
	protocol2026AssertNotification(t, protocol2026WSRead(t, conn), "notifications/subscriptions/acknowledged", "listen-ws")
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	params := protocol2026AssertNotification(t, protocol2026WSRead(t, conn), "notifications/resources/updated", "listen-ws")
	if params["uri"] != "newmcp://alpha/memo://one" {
		t.Fatalf("WebSocket did not rewrite resource URI: %#v", params)
	}
	if err := conn.WriteJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": "listen-ws"}}); err != nil {
		t.Fatal(err)
	}
	complete := protocol2026WSRead(t, conn)
	if complete["id"] != "listen-ws" || complete["result"].(map[string]any)["resultType"] != "complete" {
		t.Fatalf("cancel did not complete stream: %#v", complete)
	}
	protocol2026Wait(t, "WebSocket cancel did not release upstream watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 1 })
	if err := conn.WriteJSON(listen); err != nil {
		t.Fatal(err)
	}
	protocol2026AssertNotification(t, protocol2026WSRead(t, conn), "notifications/subscriptions/acknowledged", "listen-ws")
	_ = conn.Close()
	protocol2026Wait(t, "WebSocket disconnect did not release reopened watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 2 })

	legacy := protocol2026WS(t, f)
	if err := legacy.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": "legacy-ws", "method": "initialize", "params": map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "legacy-ws", "version": "1"}}}); err != nil {
		t.Fatal(err)
	}
	init := protocol2026WSRead(t, legacy)
	if init["result"].(map[string]any)["protocolVersion"] != "2025-11-25" {
		t.Fatalf("legacy WS init: %#v", init)
	}
	if err := legacy.WriteJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.WriteJSON(map[string]any{"jsonrpc": "2.0", "id": "legacy-subscribe", "method": "resources/subscribe", "params": map[string]any{"uri": "newmcp://alpha/memo://one"}}); err != nil {
		t.Fatal(err)
	}
	subscribed := protocol2026WSRead(t, legacy)
	if subscribed["id"] != "legacy-subscribe" || subscribed["error"] != nil {
		t.Fatalf("legacy initialized notification or subscribe was mishandled: %#v", subscribed)
	}
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	params = protocol2026AssertNotification(t, protocol2026WSRead(t, legacy), "notifications/resources/updated", nil)
	if params["uri"] != "newmcp://alpha/memo://one" || params["_meta"] != nil {
		t.Fatalf("legacy resource notification contains modern subscription metadata: %#v", params)
	}
	_ = legacy.Close()
	protocol2026Wait(t, "legacy WebSocket disconnect leaked resource watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 3 })
}

func protocol2026EventRequest(id any, method string, fields map[string]any) map[string]any {
	request := protocol2026Request(id, method, fields)
	request["params"].(map[string]any)["_meta"].(map[string]any)[mcp.MetaKeyClientCapabilities] = map[string]any{
		"extensions": map[string]any{"io.newmcp/events": map[string]any{}},
	}
	return request
}

func TestProtocol2026EventsOptInAndRealPoll(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	status, _, reply := protocol2026RPC(t, f.httpRequest(t, f.key, "/mcp", protocol2026Request("missing-opt-in", "events/list", nil)))
	if status != 400 {
		t.Fatalf("events without client opt-in: HTTP %d %#v", status, reply)
	}
	protocol2026Error(t, reply, -32021)
	data, _ := reply["error"].(map[string]any)["data"].(map[string]any)
	if data["requiredCapabilities"] == nil {
		t.Fatalf("missing capability negotiation error data: %#v", reply)
	}
	status, _, reply = protocol2026RPC(t, f.httpRequest(t, f.key, "/mcp", protocol2026EventRequest("events-list", "events/list", nil)))
	if status != 200 || reply["error"] != nil {
		t.Fatalf("opted-in events/list failed: HTTP %d %#v", status, reply)
	}
	result := reply["result"].(map[string]any)
	catalog, _ := result["events"].([]any)
	if result["resultType"] != "complete" || len(catalog) != 4 {
		t.Fatalf("experimental event catalog must contain the four implemented events: %#v", result)
	}
	fields := map[string]any{"name": "mcp.resources.updated", "arguments": map[string]any{"uri": "newmcp://alpha/memo://one"}}
	status, _, reply = protocol2026RPC(t, f.httpRequest(t, f.key, "/mcp", protocol2026EventRequest("initial-poll", "events/poll", fields)))
	if status != 200 || reply["error"] != nil {
		t.Fatalf("initial event poll failed: HTTP %d %#v", status, reply)
	}
	result = reply["result"].(map[string]any)
	cursor, _ := result["cursor"].(string)
	if cursor == "" || len(result["events"].([]any)) != 0 {
		t.Fatalf("initial poll must establish an empty bounded replay cursor: %#v", result)
	}
	protocol2026Wait(t, "event poll did not attach a real resource watcher", func() bool { return u.active.Load() == 1 })
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	fields["cursor"] = cursor
	var event map[string]any
	protocol2026Wait(t, "event poll did not receive a real upstream notification", func() bool {
		status, _, reply := protocol2026RPC(t, f.httpRequest(t, f.key, "/mcp", protocol2026EventRequest("next-poll", "events/poll", fields)))
		if status != 200 || reply["error"] != nil {
			t.Fatalf("event replay failed: HTTP %d %#v", status, reply)
		}
		batch := reply["result"].(map[string]any)["events"].([]any)
		if len(batch) == 0 {
			return false
		}
		if len(batch) != 1 {
			t.Fatalf("unexpected poll replay: %#v", batch)
		}
		event = batch[0].(map[string]any)
		return true
	})
	data = event["data"].(map[string]any)
	if event["name"] != "mcp.resources.updated" || event["eventId"] == "" || event["timestamp"] == "" || data["service"] != "alpha" || data["uri"] != "newmcp://alpha/memo://one" || data["_meta"] != nil {
		t.Fatalf("incorrect experimental event envelope: %#v", event)
	}
}

func TestProtocol2026WebSocketSameIDConnectionIsolation(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	one, two := protocol2026WS(t, f), protocol2026WS(t, f)
	listen := protocol2026Request("shared-id", "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourceSubscriptions": []string{"newmcp://alpha/memo://one"}}})
	for _, conn := range []*websocket.Conn{one, two} {
		if err := conn.WriteJSON(listen); err != nil {
			t.Fatal(err)
		}
		protocol2026AssertNotification(t, protocol2026WSRead(t, conn), "notifications/subscriptions/acknowledged", "shared-id")
	}
	// Cancel is scoped to the sending WebSocket, even when principal and request
	// ID match another live connection. The pooled upstream watcher is shared.
	if err := one.WriteJSON(map[string]any{"jsonrpc": "2.0", "method": "notifications/cancelled", "params": map[string]any{"requestId": "shared-id"}}); err != nil {
		t.Fatal(err)
	}
	complete := protocol2026WSRead(t, one)
	if complete["id"] != "shared-id" || complete["result"].(map[string]any)["resultType"] != "complete" {
		t.Fatalf("first stream failed to complete: %#v", complete)
	}
	if err := u.server.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: "memo://one"}); err != nil {
		t.Fatal(err)
	}
	protocol2026AssertNotification(t, protocol2026WSRead(t, two), "notifications/resources/updated", "shared-id")
	if u.active.Load() != 1 {
		t.Fatalf("cancel on first connection stopped second connection's watcher: active=%d", u.active.Load())
	}
	_ = one.Close()
	_ = two.Close()
	protocol2026Wait(t, "shared upstream watcher leaked after both sockets closed", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 1 })
}

func TestProtocol2026EventTerminationPrecedesCompletion(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	request := protocol2026EventRequest("terminated", "events/stream", map[string]any{
		"name": "mcp.resources.updated", "arguments": map[string]any{"uri": "newmcp://alpha/memo://one"},
	})
	sse := protocol2026OpenSSE(t, f.httpRequest(t, f.key, "/mcp", request))
	protocol2026AssertNotification(t, sse.next(t), "notifications/events/active", "terminated")
	// Revoking the source ends this stream naturally. Its final experimental
	// notification must precede the final JSON-RPC complete response on the wire.
	if err := f.db.Model(&model.McpService{}).Where("id = ?", u.service.ID).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	terminated := protocol2026AssertNotification(t, sse.next(t), "notifications/events/terminated", "terminated")
	if terminated["error"] == nil {
		t.Fatalf("missing stream termination reason: %#v", terminated)
	}
	complete := sse.next(t)
	result, _ := complete["result"].(map[string]any)
	if complete["id"] != "terminated" || result["resultType"] != "complete" {
		t.Fatalf("termination was not followed by final completion: %#v", complete)
	}
	protocol2026Wait(t, "terminated event stream retained its upstream watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 1 })
}

func TestProtocol2026CacheableResultsAndContentType(t *testing.T) {
	f := newProtocol2026Fixture(t)
	f.addUpstream(t, "alpha")
	for _, method := range []string{"server/discover", "tools/list", "resources/list", "resources/templates/list", "resources/read", "prompts/list"} {
		params := map[string]any{}
		if method == "resources/read" {
			params["uri"] = "newmcp://alpha/memo://one"
		}
		request := f.httpRequest(t, f.key, "/mcp", protocol2026Request(method, method, params))
		if method == "resources/read" {
			request.Header.Set("Mcp-Name", params["uri"].(string))
		}
		status, _, reply := protocol2026RPC(t, request)
		result, _ := reply["result"].(map[string]any)
		if status != 200 || result["resultType"] != "complete" || result["ttlMs"] != float64(0) || result["cacheScope"] != "private" {
			t.Fatalf("%s missing modern cache fields: HTTP %d %#v", method, status, reply)
		}
	}
	for _, contentType := range []string{"", "application/json-invalid", "text/plain"} {
		request := f.httpRequest(t, f.key, "/mcp", protocol2026Request("bad-content", "server/discover", nil))
		request.Header.Set("Content-Type", contentType)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnsupportedMediaType {
			t.Fatalf("content type %q accepted: %d", contentType, response.StatusCode)
		}
	}
}

func TestProtocol2026EventServiceRestrictsUpstreamConnections(t *testing.T) {
	f := newProtocol2026Fixture(t)
	alpha := f.addUpstream(t, "alpha")
	f.addUpstream(t, "beta")
	request := protocol2026EventRequest("selected", "events/stream", map[string]any{
		"name": "mcp.tools.list_changed", "arguments": map[string]any{"service": "alpha"},
	})
	sse := protocol2026OpenSSE(t, f.httpRequest(t, f.key, "/mcp", request))
	protocol2026AssertNotification(t, sse.next(t), "notifications/events/active", "selected")
	var beta model.McpService
	if err := f.db.Where("name = ?", "beta").First(&beta).Error; err != nil {
		t.Fatal(err)
	}
	// Session publication persists serverInfo. Unselected sources must not be
	// connected even though this principal can access both services.
	if beta.ServerInfo != "" && beta.ServerInfo != "{}" {
		t.Fatalf("connected an unselected event source: %s", beta.ServerInfo)
	}
	alpha.server.AddTool(&mcp.Tool{Name: "new", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	})
	event := protocol2026AssertNotification(t, sse.next(t), "notifications/events/event", "selected")
	data, _ := event["data"].(map[string]any)
	if data["service"] != "alpha" {
		t.Fatalf("wrong selected source: %#v", event)
	}
}

func TestProtocol2026SilentModeChangeEndsSubscription(t *testing.T) {
	f := newProtocol2026Fixture(t)
	u := f.addUpstream(t, "alpha")
	listen := protocol2026Request("mode-change", "subscriptions/listen", map[string]any{"notifications": map[string]any{"resourceSubscriptions": []string{"newmcp://alpha/memo://one"}}})
	sse := protocol2026OpenSSE(t, f.httpRequest(t, f.key, "/mcp/group/alpha", listen))
	ack := protocol2026AssertNotification(t, sse.next(t), "notifications/subscriptions/acknowledged", "mode-change")
	filters, _ := ack["notifications"].(map[string]any)
	if uris, _ := filters["resourceSubscriptions"].([]any); len(uris) != 1 {
		t.Fatalf("resource was not acknowledged: %#v", ack)
	}
	if err := f.db.Model(&model.McpGroup{}).Where("id = ?", u.group.ID).Update("expose_mode", "smart").Error; err != nil {
		t.Fatal(err)
	}
	// There is no upstream event to trigger a check. The source supervisor must
	// terminate the obsolete contract and release its lease on its own.
	select {
	case complete, ok := <-sse.frames:
		result, _ := complete["result"].(map[string]any)
		if !ok || complete["id"] != "mode-change" || result["resultType"] != "complete" {
			t.Fatalf("mode change did not complete the old stream: %#v", complete)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("silent mode change left the resource subscription open")
	}
	protocol2026Wait(t, "mode change retained its upstream watcher", func() bool { return u.active.Load() == 0 && u.unsubscribed.Load() >= 1 })
	newStream := protocol2026OpenSSE(t, f.httpRequest(t, f.key, "/mcp/group/alpha", listen))
	newAck := protocol2026AssertNotification(t, newStream.next(t), "notifications/subscriptions/acknowledged", "mode-change")
	newFilters, _ := newAck["notifications"].(map[string]any)
	if uris, _ := newFilters["resourceSubscriptions"].([]any); len(uris) != 0 {
		t.Fatalf("smart mode acknowledged an obsolete resource filter: %#v", newAck)
	}
}
