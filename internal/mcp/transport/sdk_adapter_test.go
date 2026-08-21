package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startResourcePromptServer 起一个带 1 资源 + 1 资源模板 + 1 提示的 MCP 服务,
// 以 Streamable HTTP 暴露在本地随机端口,返回其 URL 与结束函数。
func startResourcePromptServer(t *testing.T) (url string, shutdown func()) {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "v0.0.1"}, nil)

	readHandler := func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "content-of-" + req.Params.URI}},
		}, nil
	}
	server.AddResource(&mcp.Resource{URI: "memo://overview", Name: "overview"}, readHandler)
	server.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "memo://items/{id}", Name: "item"}, readHandler)

	server.AddPrompt(&mcp.Prompt{Name: "review", Description: "review code"},
		func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			lang := req.Params.Arguments["lang"]
			return &mcp.GetPromptResult{
				Description: "review prompt",
				Messages: []*mcp.PromptMessage{
					{Role: mcp.Role("user"), Content: &mcp.TextContent{Text: "review this " + lang + " code"}},
				},
			}, nil
		})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpSrv := httptest.NewServer(handler)
	return httpSrv.URL, httpSrv.Close
}

// TestSDKAdapterResourcesPrompts 走真实 Streamable HTTP 传输,验证 adapter 的
// resources/prompts 五个方法与上游往返正确(含能力声明、翻页迭代、参数透传)。
func TestSDKAdapterResourcesPrompts(t *testing.T) {
	url, shutdown := startResourcePromptServer(t)
	defer shutdown()

	adapter := NewStreamableHTTPAdapter(1, url, nil)
	if err := adapter.Connect(context.Background()); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer adapter.Close()

	ctx := context.Background()

	// resources/list
	raw, err := adapter.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	var listRes struct {
		Resources []map[string]interface{} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &listRes); err != nil {
		t.Fatalf("unmarshal ListResources: %v", err)
	}
	if len(listRes.Resources) != 1 || listRes.Resources[0]["uri"] != "memo://overview" {
		t.Fatalf("unexpected resources: %s", raw)
	}

	// resources/templates/list
	raw, err = adapter.ListResourceTemplates(ctx)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if !strings.Contains(string(raw), "memo://items/{id}") {
		t.Fatalf("unexpected templates: %s", raw)
	}

	// resources/read(模板展开出的 URI 也应可读)
	raw, err = adapter.ReadResource(ctx, "memo://items/42")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if !strings.Contains(string(raw), "content-of-memo://items/42") {
		t.Fatalf("unexpected read result: %s", raw)
	}

	// prompts/list
	raw, err = adapter.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if !strings.Contains(string(raw), `"review"`) {
		t.Fatalf("unexpected prompts: %s", raw)
	}

	// prompts/get(带参数)
	raw, err = adapter.GetPrompt(ctx, "review", map[string]string{"lang": "go"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if !strings.Contains(string(raw), "review this go code") {
		t.Fatalf("unexpected prompt result: %s", raw)
	}
}
