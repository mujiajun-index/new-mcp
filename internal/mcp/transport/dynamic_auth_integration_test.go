package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// e2eSelector 模拟 bridge.KeySelector:轮询供值 + 401/403 熔断(内存态)。
type e2eSelector struct {
	mu      sync.Mutex
	keys    []e2eKey
	cursor  int
	failed  []int
	picked  []int
}

type e2eKey struct {
	index   int
	value   string
	enabled bool
}

func newE2ESelector(values ...string) *e2eSelector {
	s := &e2eSelector{}
	for i, v := range values {
		s.keys = append(s.keys, e2eKey{index: i + 1, value: v, enabled: true})
	}
	return s
}

func (s *e2eSelector) Pick() (int, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cursor >= len(s.keys) {
		s.cursor = 0
	}
	for i := 0; i < len(s.keys); i++ {
		idx := (s.cursor + i) % len(s.keys)
		if s.keys[idx].enabled {
			s.cursor = (idx + 1) % len(s.keys)
			s.picked = append(s.picked, s.keys[idx].index)
			return s.keys[idx].index, "Bearer " + s.keys[idx].value, nil
		}
	}
	return 0, "", errFakeNoKeys
}

func (s *e2eSelector) OnAuthFailure(keyIndex int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.keys {
		if s.keys[i].index == keyIndex {
			s.keys[i].enabled = false
		}
	}
	s.failed = append(s.failed, keyIndex)
}

func (s *e2eSelector) failedList() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.failed...)
}

// startAuthedMCPServer 起真实 MCP 上游(仅接受 Bearer good),返回 URL。
// 附带一个工具、一个资源和一个提示,覆盖三类逻辑调用的秘钥归因。
func startAuthedMCPServer(t *testing.T) string {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "v0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo"},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		})
	server.AddResource(&mcp.Resource{URI: "test://data", Name: "data"},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "test://data", Text: "hi"}}}, nil
		})
	server.AddPrompt(&mcp.Prompt{Name: "greet", Description: "greet"},
		func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Description: "greet",
				Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "hello"}}},
			}, nil
		})
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestMultiKeyEndToEnd 走真实 Streamable HTTP 握手:坏 key(#1)触发上游 401 被熔断,
// 换 key(#2)建连成功;tools/call 的 CallWithMeta 带回所用秘钥序号。
func TestMultiKeyEndToEnd(t *testing.T) {
	url := startAuthedMCPServer(t)
	sel := newE2ESelector("bad", "good")

	a1 := NewStreamableHTTPAdapter(1, url, nil, WithDynamicAuth("Authorization", sel))
	err1 := a1.Connect(context.Background())
	// 坏 key 的 initialize 必然 401;SDK 是否内部重试换到 #2 不可控,两条路径都合法:
	// 路径 A:Connect 失败 → 关闭后用第二个 adapter;路径 B:SDK 重试期间 RoundTripper
	// 轮换到 #2 并成功。无论哪条,#1 都必须已被 OnAuthFailure 熔断。
	failed := sel.failedList()
	if len(failed) == 0 || failed[0] != 1 {
		t.Fatalf("key #1 should be auto-disabled on 401, failed = %v", failed)
	}

	var adapter *SDKAdapter
	if err1 == nil {
		adapter = a1
	} else {
		_ = a1.Close()
		adapter = NewStreamableHTTPAdapter(1, url, nil, WithDynamicAuth("Authorization", sel))
		if err := adapter.Connect(context.Background()); err != nil {
			t.Fatalf("reconnect with next key: %v", err)
		}
	}
	defer adapter.Close()

	if !adapter.IsConnected() {
		t.Fatal("adapter should be connected")
	}

	raw, meta, err := adapter.CallWithMeta(context.Background(), "tools/call", map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]interface{}{"msg": "hi"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	var res struct {
		Content []map[string]interface{} `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || len(res.Content) == 0 {
		t.Fatalf("unexpected result: %s (%v)", raw, err)
	}
	// #1 已熔断,调用必然用 #2
	if meta.KeyIndex != 2 {
		t.Fatalf("CallWithMeta KeyIndex = %d, want 2", meta.KeyIndex)
	}

	// 资源/提示测试路径(服务详情页)同样经 WithMeta 变体带回实际使用的秘钥序号。
	rRaw, rMeta, rErr := adapter.ReadResourceWithMeta(context.Background(), "test://data")
	if rErr != nil {
		t.Fatalf("read resource: %v", rErr)
	}
	if len(rRaw) == 0 {
		t.Fatal("empty resource result")
	}
	if rMeta.KeyIndex != 2 {
		t.Fatalf("ReadResourceWithMeta KeyIndex = %d, want 2", rMeta.KeyIndex)
	}
	pRaw, pMeta, pErr := adapter.GetPromptWithMeta(context.Background(), "greet", nil)
	if pErr != nil {
		t.Fatalf("get prompt: %v", pErr)
	}
	if len(pRaw) == 0 {
		t.Fatal("empty prompt result")
	}
	if pMeta.KeyIndex != 2 {
		t.Fatalf("GetPromptWithMeta KeyIndex = %d, want 2", pMeta.KeyIndex)
	}
}
