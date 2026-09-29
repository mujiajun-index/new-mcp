package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type queryTestSelector struct {
	mu     sync.Mutex
	values []string
	next   int
	failed []int
}

type constantQuerySelector struct{}

func (constantQuerySelector) Pick() (int, string, error) { return 1, "good", nil }
func (constantQuerySelector) OnAuthFailure(int)          {}

func TestDynamicQueryAuthSDKHandshake(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "query-upstream", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tavilyApiKey") != "good" || r.URL.Query().Get("region") != "cn" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	adapter := NewStreamableHTTPAdapter(1, srv.URL+"/mcp?region=cn", nil, WithDynamicQueryAuth("tavilyApiKey", constantQuerySelector{}))
	if err := adapter.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	_, meta, err := adapter.CallWithMeta(context.Background(), "tools/call", map[string]interface{}{"name": "echo", "arguments": map[string]interface{}{}})
	if err != nil || meta.KeyIndex != 1 {
		t.Fatalf("call: index=%d err=%v", meta.KeyIndex, err)
	}
}

func TestDynamicQueryAuthSSE(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "query-sse", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	handler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil)
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("tavilyApiKey") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		seen[r.Method]++
		mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	adapter := NewSSEAdapter(1, srv.URL+"/sse", nil, WithDynamicQueryAuth("tavilyApiKey", constantQuerySelector{}))
	if err := adapter.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	_, meta, err := adapter.CallWithMeta(context.Background(), "tools/call", map[string]interface{}{"name": "echo", "arguments": map[string]interface{}{}})
	if err != nil || meta.KeyIndex != 1 {
		t.Fatalf("call: index=%d err=%v", meta.KeyIndex, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if seen[http.MethodGet] == 0 || seen[http.MethodPost] == 0 {
		t.Fatalf("GET and POST must carry query key: %v", seen)
	}
}

func (s *queryTestSelector) Pick() (int, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.next
	s.next++
	return i + 1, s.values[i], nil
}

func (s *queryTestSelector) OnAuthFailure(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, index)
}

func TestDynamicQueryAuthRoundTrip(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("region") != "cn" {
			t.Errorf("other query parameter lost: %s", r.URL.RawQuery)
		}
		if r.Header.Get("tavilyApiKey") != "" {
			t.Error("query key was sent as header")
		}
		seen = append(seen, r.URL.Query().Get("tavilyApiKey"))
		if seen[len(seen)-1] == "bad" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	sel := &queryTestSelector{values: []string{"bad", "good"}}
	client := httpClientWithHeaders(nil, &dynamicSlot{target: "tavilyApiKey", query: true, dyn: sel})
	url := srv.URL + "/mcp?region=cn&tavilyApiKey=stale"
	for _, want := range []int{http.StatusUnauthorized, http.StatusOK} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("status=%d want=%d", resp.StatusCode, want)
		}
		if req.URL.Query().Get("tavilyApiKey") != "stale" {
			t.Fatal("original request mutated")
		}
	}
	if len(seen) != 2 || seen[0] != "bad" || seen[1] != "good" || len(sel.failed) != 1 || sel.failed[0] != 1 {
		t.Fatalf("seen=%v failed=%v", seen, sel.failed)
	}
	ctx := WithAuthChoice(context.Background(), 7, "special+value")
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if seen[2] != "special+value" {
		t.Fatalf("context choice not applied: %v", seen)
	}
}

func TestSSEInitialGetAndPostUseSameQueryKey(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+":"+r.URL.Query().Get("key"))
	}))
	defer srv.Close()
	selector := &queryTestSelector{values: []string{"first", "second"}}
	client := httpClientWithHeaders(nil, &dynamicSlot{target: "key", query: true, dyn: selector})
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPost} {
		req, _ := http.NewRequest(method, srv.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	want := []string{"GET:first", "POST:first", "POST:second"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("seen=%v want=%v", seen, want)
		}
	}
}

func TestQueryTransportErrorRedactsKey(t *testing.T) {
	err := (&redactedQueryError{cause: errors.New("dial https://example.test/mcp?key=abc%2B123"), value: "abc+123"}).Error()
	if strings.Contains(err, "abc%2B123") || strings.Contains(err, "abc+123") {
		t.Fatalf("credential in error: %s", err)
	}
}
