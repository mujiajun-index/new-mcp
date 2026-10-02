package events

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testSource struct {
	mu       sync.Mutex
	emitters []func(Event)
	contexts []context.Context
	stops    atomic.Int32
}

func (s *testSource) source(ctx context.Context, _ string, _ json.RawMessage, emit func(Event)) (func(), error) {
	s.mu.Lock()
	s.emitters = append(s.emitters, emit)
	s.contexts = append(s.contexts, ctx)
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { s.stops.Add(1) }) }, nil
}
func (s *testSource) emit(index int, id string) {
	s.mu.Lock()
	fn := s.emitters[index]
	s.mu.Unlock()
	fn(Event{EventID: id, Data: map[string]any{"service": "test", "uri": "file:///a"}})
}
func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func params(cursor any) map[string]any {
	return map[string]any{"name": "mcp.tools.list_changed", "arguments": map[string]any{"service": "test"}, "cursor": cursor}
}
func call(t *testing.T, m *Manager, p Principal, method string, body any, source Source) map[string]any {
	t.Helper()
	v, err := m.Handle(context.Background(), p, method, mustJSON(body), source)
	if err != nil {
		t.Fatal(err)
	}
	return v.(map[string]any)
}
func rpcCode(t *testing.T, err error, want int) {
	t.Helper()
	var r *RPCError
	if !errors.As(err, &r) || r.Code != want {
		t.Fatalf("expected RPC %d, got %v", want, err)
	}
}
func recv(t *testing.T, ch <-chan Notification, method string) Notification {
	t.Helper()
	select {
	case n, ok := <-ch:
		if !ok || n.Method != method {
			t.Fatalf("expected %s, got %+v (open %v)", method, n, ok)
		}
		return n
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for %s", method)
	}
	return Notification{}
}

func TestPollCursorBoundsIsolationAndAuthorization(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	p := Principal{ID: "alice"}
	first := call(t, m, p, "events/poll", params(nil), s.source)
	c := first["cursor"].(string)
	if len(first["events"].([]Event)) != 0 {
		t.Fatal("null cursor must start from now")
	}
	s.emit(0, "a")
	s.emit(0, "b")
	s.emit(0, "c")
	args := params(c)
	args["maxEvents"] = 2
	one := call(t, m, p, "events/poll", args, s.source)
	if !one["hasMore"].(bool) || len(one["events"].([]Event)) != 2 {
		t.Fatalf("bad capped poll: %+v", one)
	}
	two := call(t, m, p, "events/poll", params(one["cursor"]), s.source)
	if two["hasMore"].(bool) || two["events"].([]Event)[0].EventID != "c" {
		t.Fatalf("intermediate cursor skipped event: %+v", two)
	}
	foreign := call(t, m, Principal{ID: "bob"}, "events/poll", params(c), s.source)
	if !foreign["truncated"].(bool) || len(foreign["events"].([]Event)) != 0 {
		t.Fatal("principal cursor leaked history")
	}
	m.mu.Lock()
	m.ringSize = 2
	m.mu.Unlock()
	bounded := call(t, m, p, "events/poll", params(c), s.source)
	if !bounded["truncated"].(bool) || bounded["events"].([]Event)[0].EventID != "b" {
		t.Fatalf("ring retention did not signal gap: %+v", bounded)
	}
	age := params(c)
	age["maxAgeMs"] = 0
	if r := call(t, m, p, "events/poll", age, s.source); !r["truncated"].(bool) || len(r["events"].([]Event)) != 0 {
		t.Fatalf("maxAge ignored: %+v", r)
	}
	var eventChecks atomic.Int32
	p.Check = func(ctx context.Context) error {
		if _, ok := EventFromContext(ctx); ok {
			eventChecks.Add(1)
			return errors.New("service revoked")
		}
		return nil
	}
	_, err := m.Handle(context.Background(), p, "events/poll", mustJSON(params(c)), s.source)
	rpcCode(t, err, -32012)
	if eventChecks.Load() != 1 {
		t.Fatal("cached event permissions were not checked")
	}
}

func TestPushLifecycleHeartbeatRevocationAndStop(t *testing.T) {
	m := NewManager()
	m.heartbeat = 20 * time.Millisecond
	defer m.Close()
	s := &testSource{}
	var revoke atomic.Bool
	p := Principal{ID: "a", Check: func(ctx context.Context) error {
		if _, ok := EventFromContext(ctx); ok && revoke.Load() {
			return errors.New("revoked")
		}
		return nil
	}}
	ch, stop, err := m.Stream(context.Background(), p, mustJSON(params(nil)), s.source)
	if err != nil {
		t.Fatal(err)
	}
	recv(t, ch, "notifications/events/active")
	recv(t, ch, "notifications/events/heartbeat")
	s.emit(0, "first")
	if n := recv(t, ch, "notifications/events/event"); n.Params.(Event).EventID != "first" {
		t.Fatal("event mismatch")
	}
	revoke.Store(true)
	s.emit(0, "forbidden")
	recv(t, ch, "notifications/events/terminated")
	stop()
	stop()
	if s.stops.Load() != 1 {
		t.Fatal("stream teardown did not release source exactly once")
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("stream remained open")
		}
	case <-time.After(time.Second):
		t.Fatal("stream leaked")
	}
}

func TestPollLeaseAndCloseReleaseResources(t *testing.T) {
	m := NewManager()
	s := &testSource{}
	call(t, m, Principal{ID: "a"}, "events/poll", params(nil), s.source)
	m.sweep(time.Now().Add(6 * time.Minute))
	if s.stops.Load() != 1 {
		t.Fatal("poll lease did not expire")
	}
	call(t, m, Principal{ID: "a"}, "events/poll", params(nil), s.source)
	m.Close()
	m.Close()
	if s.stops.Load() != 2 {
		t.Fatal("close leaked source")
	}
}

func TestEventValidation(t *testing.T) {
	m := NewManager()
	defer m.Close()
	for _, body := range []string{
		`{"name":"mcp.resources.updated","arguments":{}}`,
		`{"name":"mcp.tools.list_changed","arguments":{"service":null}}`,
		`{"name":"mcp.tools.list_changed","arguments":{"other":"test"}}`,
		`{"name":"mcp.tools.list_changed","maxEvents":0}`,
		`{"name":"mcp.tools.list_changed","maxAgeMs":-1}`,
		`{"name":"mcp.tools.list_changed","id":"not-a-key"}`,
	} {
		_, err := m.Handle(context.Background(), Principal{}, "events/poll", json.RawMessage(body), nil)
		rpcCode(t, err, -32602)
	}
	_, err := m.Handle(context.Background(), Principal{}, "events/poll", mustJSON(map[string]any{"name": "business.new"}), nil)
	rpcCode(t, err, -32011)
	c := call(t, m, Principal{}, "events/list", map[string]any{}, nil)
	if len(c["events"].([]map[string]any)) != 4 {
		t.Fatal("unexpected synthetic business events")
	}
}

type receivedWebhook struct {
	Header http.Header
	Body   []byte
}

func webhookHarness(t *testing.T, m *Manager, handler func(http.ResponseWriter, *http.Request)) string {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(handler))
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	m.webhooks.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	m.webhooks.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if !strings.HasPrefix(addr, "8.8.8.8:") {
			t.Errorf("unvalidated dial: %s", addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	// Test-only transport trust: production has no InsecureSkipVerify escape.
	m.webhooks.tlsConfig = &tls.Config{InsecureSkipVerify: true}
	m.retryDelay = 10 * time.Millisecond
	return "https://receiver.example/hooks/client"
}
func hookParams(u, secret string) map[string]any {
	p := params(nil)
	p["delivery"] = map[string]any{"mode": "webhook", "url": u, "secret": secret}
	p["ttlMs"] = 60000
	return p
}
func unsubscribeParams(u string) map[string]any {
	p := params(nil)
	delete(p, "cursor")
	p["delivery"] = map[string]any{"url": u}
	return p
}

var testSecret = "whsec_" + base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))

func verifySignature(t *testing.T, r receivedWebhook, key string) {
	t.Helper()
	k, _ := secret(key)
	h := hmac.New(sha256.New, k)
	h.Write([]byte(r.Header.Get("webhook-id") + "." + r.Header.Get("webhook-timestamp") + "."))
	h.Write(r.Body)
	want := "v1," + base64.StdEncoding.EncodeToString(h.Sum(nil))
	if !strings.Contains(r.Header.Get("webhook-signature"), want) {
		t.Fatal("invalid raw-body Standard Webhooks signature")
	}
	if r.Header.Get("X-MCP-Subscription-Id") == "" {
		t.Fatal("missing subscription header")
	}
}

func TestWebhookChallengeSignatureRetryRefreshIsolationAndUnsubscribe(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	received := make(chan receivedWebhook, 20)
	var verifications, attempts atomic.Int32
	u := webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- receivedWebhook{r.Header.Clone(), body}
		var control map[string]any
		json.Unmarshal(body, &control)
		if control["type"] == "verification" {
			verifications.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"challenge": control["challenge"]})
			return
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	})
	p := Principal{ID: "alice"}
	sub := call(t, m, p, "events/subscribe", hookParams(u, testSecret), s.source)
	verifySignature(t, <-received, testSecret)
	if s.stops.Load() != 0 {
		t.Fatal("active source was closed")
	}
	s.emit(0, "same-event")
	var deliveries []receivedWebhook
	for len(deliveries) < 2 {
		select {
		case r := <-received:
			deliveries = append(deliveries, r)
		case <-time.After(3 * time.Second):
			t.Fatal("retry not received")
		}
	}
	for _, r := range deliveries {
		verifySignature(t, r, testSecret)
		if r.Header.Get("webhook-id") != "same-event" {
			t.Fatal("retry changed event id")
		}
	}
	refresh := hookParams(u, testSecret)
	// Order and whitespace must not alter the subscription identity.
	refresh["arguments"] = json.RawMessage(`{ "service" : "test" }`)
	again := call(t, m, p, "events/subscribe", refresh, s.source)
	if again["id"] != sub["id"] || verifications.Load() != 1 {
		t.Fatal("idempotent refresh challenged again or changed id")
	}
	_, err := m.Handle(context.Background(), Principal{ID: "bob"}, "events/unsubscribe", mustJSON(unsubscribeParams(u)), nil)
	rpcCode(t, err, -32011)
	call(t, m, p, "events/unsubscribe", unsubscribeParams(u), nil)
	if s.stops.Load() != 1 {
		t.Fatal("unsubscribe did not release source")
	}
	_, err = m.Handle(context.Background(), p, "events/unsubscribe", mustJSON(map[string]any{"id": sub["id"]}), nil)
	rpcCode(t, err, -32602)
}

func TestWebhookPrivateAddressesRebindingAndRedirects(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.0.1", "192.0.2.1", "198.18.0.1", "::1", "fc00::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2001:db8::1"} {
		if publicIP(net.ParseIP(address)) {
			t.Fatalf("private/reserved address accepted: %s", address)
		}
	}
	m := NewManager()
	defer m.Close()
	var requests atomic.Int32
	u := webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Location", "https://127.0.0.1/private")
		w.WriteHeader(302)
	})
	_, err := m.Handle(context.Background(), Principal{ID: "a"}, "events/subscribe", mustJSON(hookParams(u, testSecret)), (&testSource{}).source)
	rpcCode(t, err, -32015)
	if requests.Load() != 1 {
		t.Fatal("redirect was followed")
	}
	var dialed atomic.Bool
	m.webhooks.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	m.webhooks.dial = func(context.Context, string, string) (net.Conn, error) {
		dialed.Store(true)
		return nil, errors.New("unexpected dial")
	}
	_, err = m.webhooks.post(context.Background(), u, "event", "sub", [][]byte{[]byte("secret")}, []byte(`{}`))
	if err == nil || dialed.Load() {
		t.Fatal("DNS rebinding/mixed DNS result allowed dialing")
	}
	for _, u := range []string{"http://public.example/hook", "https://127.0.0.1/hook", "https://user:pass@public.example/hook", "https://public.example/hook#fragment"} {
		if _, err := callbackURL(u); err == nil {
			t.Fatalf("unsafe callback accepted %s", u)
		}
	}
}

func TestWebhookChallengeFailureNoSourceAndSecretValidation(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	u := webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"challenge":"wrong"}`) })
	_, err := m.Handle(context.Background(), Principal{ID: "a"}, "events/subscribe", mustJSON(hookParams(u, testSecret)), s.source)
	rpcCode(t, err, -32015)
	var rpc *RPCError
	errors.As(err, &rpc)
	if rpc.Data.(map[string]any)["reason"] != "challenge_failed" {
		t.Fatal("raw callback response escaped")
	}
	if len(s.emitters) != 0 {
		t.Fatal("unverified callback provisioned source")
	}
	for _, value := range []string{"not-prefixed", "whsec_!", "whsec_" + base64.StdEncoding.EncodeToString([]byte("too-short"))} {
		if _, err := secret(value); err == nil {
			t.Fatal("invalid secret accepted")
		}
	}
	_, err = m.Handle(context.Background(), Principal{}, "events/subscribe", mustJSON(hookParams(u, testSecret)), s.source)
	rpcCode(t, err, -32012)
}

func TestWebhookExpiryAndEventRevocationCancelDelivery(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	received := make(chan string, 10)
	var eventChecks atomic.Int32
	u := webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var c map[string]any
		json.Unmarshal(body, &c)
		if c["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": c["challenge"]})
			return
		}
		if typ, ok := c["type"].(string); ok {
			received <- typ
		} else {
			received <- "event"
		}
		w.WriteHeader(204)
	})
	p := Principal{ID: "a", Check: func(ctx context.Context) error {
		if e, ok := EventFromContext(ctx); ok {
			eventChecks.Add(1)
			if e.EventID == "revoked" {
				return errors.New("resource disabled")
			}
		}
		return nil
	}}
	call(t, m, p, "events/subscribe", hookParams(u, testSecret), s.source)
	s.emit(0, "revoked")
	select {
	case typ := <-received:
		if typ != "terminated" {
			t.Fatal("revoked payload delivered")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no revocation signal")
	}
	deadline := time.Now().Add(time.Second)
	for s.stops.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.stops.Load() != 1 || eventChecks.Load() == 0 {
		t.Fatal("revocation did not teardown")
	}
	call(t, m, p, "events/subscribe", hookParams(u, testSecret), s.source)
	m.sweep(time.Now().Add(6 * time.Minute))
	deadline = time.Now().Add(time.Second)
	for s.stops.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if s.stops.Load() != 2 {
		t.Fatal("expiry leaked source")
	}
}

func TestWebhookRetryRechecksCachedEventAndCancellation(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	var checks, attempts atomic.Int32
	u := webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var c map[string]any
		json.Unmarshal(body, &c)
		if c["type"] == "verification" {
			json.NewEncoder(w).Encode(map[string]any{"challenge": c["challenge"]})
			return
		}
		if c["type"] == "terminated" {
			w.WriteHeader(204)
			return
		}
		attempts.Add(1)
		w.WriteHeader(503)
	})
	p := Principal{ID: "a", Check: func(ctx context.Context) error {
		if _, ok := EventFromContext(ctx); ok && checks.Add(1) > 1 {
			return errors.New("revoked after attempt")
		}
		return nil
	}}
	call(t, m, p, "events/subscribe", hookParams(u, testSecret), s.source)
	s.emit(0, "retry-revoked")
	deadline := time.Now().Add(time.Second)
	for s.stops.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if checks.Load() != 2 || attempts.Load() != 1 || s.stops.Load() != 1 {
		t.Fatalf("retry bypassed auth: checks %d attempts %d stops %d", checks.Load(), attempts.Load(), s.stops.Load())
	}
	// A separate verified subscription is cancelled while backing off. Once
	// unsubscribe acknowledges, no further callback or source is left running.
	p.Check = nil
	call(t, m, p, "events/subscribe", hookParams(u, testSecret), s.source)
	m.retryDelay = 200 * time.Millisecond
	s.emit(1, "cancel-retry")
	deadline = time.Now().Add(time.Second)
	for attempts.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	call(t, m, p, "events/unsubscribe", unsubscribeParams(u), nil)
	before := attempts.Load()
	time.Sleep(250 * time.Millisecond)
	if attempts.Load() != before || s.stops.Load() != 2 {
		t.Fatal("unsubscribe left retry running")
	}
}

func TestAsynchronousSourceFailureTerminatesStreamAndPoll(t *testing.T) {
	m := NewManager()
	defer m.Close()
	var srcCtx context.Context
	var stopCalls atomic.Int32
	workerDone := make(chan struct{})
	source := func(ctx context.Context, _ string, _ json.RawMessage, _ func(Event)) (func(), error) {
		srcCtx = ctx
		return func() { <-workerDone; stopCalls.Add(1) }, nil
	}
	p := Principal{ID: "a"}
	call(t, m, p, "events/poll", params(nil), source)
	ch, stop, err := m.Stream(context.Background(), p, mustJSON(params(nil)), source)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	recv(t, ch, "notifications/events/active")
	failure := &RPCError{Code: -32012, Message: "Forbidden", Data: map[string]any{"reason": "Source scope changed"}}
	if !FailSource(srcCtx, failure) {
		t.Fatal("live source report rejected")
	}
	// Reporting must return before teardown's supervisor-wait callback can end.
	close(workerDone)
	n := recv(t, ch, "notifications/events/terminated")
	rpcCode(t, n.Params.(map[string]any)["error"].(error), -32012)
	// A new call retries source setup immediately. A still failed source keeps
	// returning its original typed failure rather than being cached as live.
	_, err = m.Handle(context.Background(), p, "events/poll", mustJSON(params(nil)), func(context.Context, string, json.RawMessage, func(Event)) (func(), error) {
		return nil, failure
	})
	rpcCode(t, err, -32012)
	if FailSource(srcCtx, failure) || FailSource(context.Background(), failure) {
		t.Fatal("dead or unrelated source context accepted report")
	}
	stop()
	deadline := time.Now().Add(time.Second)
	for stopCalls.Load() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if stopCalls.Load() != 1 {
		t.Fatal("failure did not teardown exactly once")
	}
}

func TestFailedSourceCanImmediatelyRecoverUnderSameKey(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	p := Principal{ID: "recover"}
	first := call(t, m, p, "events/poll", params(nil), s.source)
	oldCursor := first["cursor"].(string)
	s.emit(0, "old event")
	s.mu.Lock()
	oldContext := s.contexts[0]
	s.mu.Unlock()
	if !FailSource(oldContext, errors.New("connection closed")) {
		t.Fatal("live failure was not accepted")
	}
	recovered := call(t, m, p, "events/poll", params(&oldCursor), s.source)
	if !recovered["truncated"].(bool) || recovered["cursor"] == oldCursor {
		t.Fatal("recovered feed did not invalidate the old cursor epoch")
	}
	s.emit(1, "new event")
	newCursor := recovered["cursor"].(string)
	replay := call(t, m, p, "events/poll", params(&newCursor), s.source)
	if got := replay["events"].([]Event); len(got) != 1 || got[0].EventID != "new event" {
		t.Fatalf("recovered feed replay = %#v", got)
	}
	if FailSource(oldContext, errors.New("late old failure")) {
		t.Fatal("old source failure affected its replacement")
	}
}

func TestSourceFailureBeforeSetupReturnsPreservesError(t *testing.T) {
	m := NewManager()
	defer m.Close()
	var stops atomic.Int32
	failure := &RPCError{Code: -32012, Message: "Forbidden"}
	source := func(ctx context.Context, _ string, _ json.RawMessage, _ func(Event)) (func(), error) {
		if !FailSource(ctx, failure) {
			t.Fatal("setup failure hook missing")
		}
		return func() { stops.Add(1) }, nil
	}
	_, err := m.Handle(context.Background(), Principal{ID: "a"}, "events/poll", mustJSON(params(nil)), source)
	rpcCode(t, err, -32012)
	if stops.Load() != 1 {
		t.Fatal("early failure leaked source")
	}
}

func TestOldFailedSetupCannotRemoveRecoveredFeed(t *testing.T) {
	m := NewManager()
	defer m.Close()
	started := make(chan context.Context, 1)
	finishOldSetup := make(chan struct{})
	oldResult := make(chan error, 1)
	var oldStops atomic.Int32
	p := Principal{ID: "recover-setup"}
	go func() {
		_, err := m.Handle(context.Background(), p, "events/poll", mustJSON(params(nil)), func(ctx context.Context, _ string, _ json.RawMessage, _ func(Event)) (func(), error) {
			started <- ctx
			<-finishOldSetup
			return func() { oldStops.Add(1) }, nil
		})
		oldResult <- err
	}()
	oldContext := <-started
	if !FailSource(oldContext, errors.New("setup disconnected")) {
		t.Fatal("setup failure rejected")
	}
	s := &testSource{}
	fresh := call(t, m, p, "events/poll", params(nil), s.source)
	close(finishOldSetup)
	rpcCode(t, <-oldResult, -32603)
	s.emit(0, "still recovered")
	replay := call(t, m, p, "events/poll", params(fresh["cursor"]), s.source)
	if got := replay["events"].([]Event); len(got) != 1 || got[0].EventID != "still recovered" || oldStops.Load() != 1 {
		t.Fatalf("old setup removed replacement: %#v, stops %d", got, oldStops.Load())
	}
}

func TestSourceFailureStopsCachedPushBatch(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	p := Principal{ID: "push-failure"}
	first := call(t, m, p, "events/poll", params(nil), s.source)
	s.emit(0, "first")
	s.emit(0, "second")
	secondCheck := make(chan struct{})
	finishCheck := make(chan struct{})
	p.Check = func(ctx context.Context) error {
		if e, ok := EventFromContext(ctx); ok && e.EventID == "second" {
			close(secondCheck)
			<-finishCheck
		}
		return nil
	}
	ch, stop, err := m.Stream(context.Background(), p, mustJSON(params(first["cursor"])), s.source)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	recv(t, ch, "notifications/events/active")
	recv(t, ch, "notifications/events/event")
	<-secondCheck
	s.mu.Lock()
	srcCtx := s.contexts[0]
	s.mu.Unlock()
	if !FailSource(srcCtx, errors.New("source disconnected during replay")) {
		t.Fatal("source failure rejected")
	}
	close(finishCheck)
	recv(t, ch, "notifications/events/terminated")
	stop()
	if _, open := <-ch; open {
		t.Fatal("cached second event sent after source failure")
	}
}

func TestSourceFailureStopsWebhookRetry(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	firstAttempt := make(chan struct{}, 1)
	terminated := make(chan struct{}, 1)
	var attempts atomic.Int32
	u := webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var envelope map[string]any
		_ = json.Unmarshal(body, &envelope)
		switch envelope["type"] {
		case "verification":
			_ = json.NewEncoder(w).Encode(map[string]any{"challenge": envelope["challenge"]})
		case "terminated":
			terminated <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		default:
			attempts.Add(1)
			firstAttempt <- struct{}{}
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	m.retryDelay = 200 * time.Millisecond
	p := Principal{ID: "hook-failure"}
	call(t, m, p, "events/subscribe", hookParams(u, testSecret), s.source)
	s.emit(0, "retry-failure")
	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatal("first attempt did not arrive")
	}
	s.mu.Lock()
	srcCtx := s.contexts[0]
	s.mu.Unlock()
	if !FailSource(srcCtx, errors.New("source disconnected during retry")) {
		t.Fatal("source failure rejected")
	}
	select {
	case <-terminated:
	case <-time.After(time.Second):
		t.Fatal("failed webhook did not terminate")
	}
	if attempts.Load() != 1 {
		t.Fatalf("retried detached source %d times", attempts.Load())
	}
}

func TestWebhookCanResubscribeBeforeTerminatedACK(t *testing.T) {
	m := NewManager()
	defer m.Close()
	s := &testSource{}
	p := Principal{ID: "hook-recover"}
	refreshed := make(chan error, 1)
	events := make(chan string, 2)
	var callbackURL string
	callbackURL = webhookHarness(t, m, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var envelope map[string]any
		_ = json.Unmarshal(body, &envelope)
		switch envelope["type"] {
		case "verification":
			_ = json.NewEncoder(w).Encode(map[string]any{"challenge": envelope["challenge"]})
		case "terminated":
			// Re-subscribe BEFORE acknowledging the old worker's control POST.
			_, err := m.Handle(context.Background(), p, "events/subscribe", mustJSON(hookParams(callbackURL, testSecret)), s.source)
			refreshed <- err
			w.WriteHeader(http.StatusNoContent)
		default:
			id, _ := envelope["eventId"].(string)
			events <- id
			w.WriteHeader(http.StatusNoContent)
		}
	})
	call(t, m, p, "events/subscribe", hookParams(callbackURL, testSecret), s.source)
	m.mu.Lock()
	var old *subscription
	for _, subscription := range m.subscriptions {
		old = subscription
	}
	m.mu.Unlock()
	s.mu.Lock()
	srcCtx := s.contexts[0]
	s.mu.Unlock()
	if !FailSource(srcCtx, errors.New("upstream briefly disconnected")) {
		t.Fatal("source failure rejected")
	}
	select {
	case err := <-refreshed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("endpoint could not immediately re-subscribe")
	}
	select {
	case <-old.done:
	case <-time.After(time.Second):
		t.Fatal("old worker did not finish")
	}
	s.mu.Lock()
	sources := len(s.emitters)
	s.mu.Unlock()
	if sources != 2 {
		t.Fatalf("terminal refresh reused the old source: %d sources", sources)
	}
	s.emit(1, "recovered delivery")
	select {
	case id := <-events:
		if id != "recovered delivery" {
			t.Fatalf("unexpected delivery %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("old worker cleanup deleted the replacement subscription")
	}
	call(t, m, p, "events/unsubscribe", unsubscribeParams(callbackURL), nil)
}
