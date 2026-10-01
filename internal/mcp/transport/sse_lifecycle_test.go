package transport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func startLifecycleSSEAdapter(t *testing.T) *SDKAdapter {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "sse-lifecycle", Version: "1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo"}, func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
	})
	srv := httptest.NewServer(mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil))
	t.Cleanup(srv.Close)
	a := NewSSEAdapter(1, srv.URL, nil)
	t.Cleanup(func() { _ = a.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSSEConnectionOutlivesCallerContext(t *testing.T) {
	a := startLifecycleSSEAdapter(t) // helper has already cancelled Connect ctx
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Ping(ctx); err != nil {
		t.Fatalf("caller cancellation closed pooled SSE stream: %v", err)
	}
	if _, err := a.Call(ctx, "tools/call", map[string]any{"name": "echo"}); err != nil {
		t.Fatal(err)
	}
}

func TestSSEConnectionOutlivesThirtySeconds(t *testing.T) {
	if testing.Short() {
		t.Skip("checks a real stream beyond the former 30-second timeout")
	}
	t.Parallel()
	a := startLifecycleSSEAdapter(t)
	select {
	case <-a.Done():
		t.Fatal("SSE stream closed before 31 seconds")
	case <-time.After(31 * time.Second):
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Ping(ctx); err != nil {
		t.Fatalf("SSE stream expired at the old HTTP client deadline: %v", err)
	}
}

func TestSSEInitialDiscoveryCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done() // no endpoint event; client must cancel initial GET
	}))
	defer srv.Close()
	a := NewSSEAdapter(1, srv.URL, nil)
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := a.Connect(ctx); err == nil {
		t.Fatal("SSE initial endpoint discovery ignored caller deadline")
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("failed initialization must end adapter lifecycle")
	}
}
