package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// decodeBlocks 把 buildExecuteBatchContent 的 content 数组(头部块为 map、透传块为
// json.RawMessage 的混合)统一解开成 (type, text) 便于断言;非 text 块保留 type 供
// 顺序/类型断言。
func decodeBlocks(t *testing.T, content []interface{}) []map[string]string {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	var blocks []map[string]string
	if err := json.Unmarshal(raw, &blocks); err != nil {
		t.Fatalf("unmarshal blocks: %v", err)
	}
	return blocks
}

func blockTexts(blocks []map[string]string) []string {
	var texts []string
	for _, b := range blocks {
		if b["type"] == "text" {
			texts = append(texts, b["text"])
		}
	}
	return texts
}

func batchCalls(toolIDs ...string) []executeBatchCall {
	calls := make([]executeBatchCall, len(toolIDs))
	for i, id := range toolIDs {
		calls[i] = executeBatchCall{ToolID: id}
	}
	return calls
}

func TestBuildExecuteBatchContentAllOK(t *testing.T) {
	calls := batchCalls("weather.get_forecast", "exa.web_search_exa")
	outcomes := []*callOutcome{
		{Result: json.RawMessage(`{"content":[{"type":"text","text":"Paris: 22C"}]}`)},
		{Result: json.RawMessage(`{"content":[{"type":"text","text":"3 results"},{"type":"image","data":"...","mimeType":"image/png"}]}`)},
	}
	content, failed := buildExecuteBatchContent(calls, outcomes)
	if failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
	blocks := decodeBlocks(t, content)
	if blocks[0]["text"] != "Batch of 2 calls: 2 ok, 0 failed." {
		t.Fatalf("summary = %q", blocks[0]["text"])
	}
	// 非文本透传块保持原类型不被改写
	hasImage := false
	for _, b := range blocks {
		if b["type"] == "image" {
			hasImage = true
		}
	}
	if !hasImage {
		t.Fatalf("image passthrough block lost: %v", blocks)
	}
	texts := blockTexts(blocks)
	for _, want := range []string{"[0] weather.get_forecast — ok", "[1] exa.web_search_exa — ok", "Paris: 22C", "3 results"} {
		found := false
		for _, s := range texts {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing text %q in %v", want, texts)
		}
	}
}

func TestBuildExecuteBatchContentPartialAndAllFailed(t *testing.T) {
	calls := batchCalls("weather.get_forecast", "calc.add")
	outcomes := []*callOutcome{
		{Result: json.RawMessage(`{"content":[{"type":"text","text":"ok data"}]}`)},
		{Err: "Execution failed: timeout"},
	}
	content, failed := buildExecuteBatchContent(calls, outcomes)
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	texts := blockTexts(decodeBlocks(t, content))
	summary := texts[0]
	if !strings.Contains(summary, "1 ok, 1 failed") || !strings.Contains(summary, "retry them individually with mcp.execute") {
		t.Fatalf("summary = %q", summary)
	}
	if !contains(texts, "[1] calc.add — failed: Execution failed: timeout") {
		t.Fatalf("missing failed header in %v", texts)
	}

	// 全部失败
	outcomes[0] = &callOutcome{Err: "service 'weather' is not accessible with this API key"}
	_, failed = buildExecuteBatchContent(calls, outcomes)
	if failed != 2 {
		t.Fatalf("failed = %d, want 2", failed)
	}
}

func TestBuildExecuteBatchContentUpstreamIsError(t *testing.T) {
	calls := batchCalls("db.query")
	outcomes := []*callOutcome{
		{Result: json.RawMessage(`{"content":[{"type":"text","text":"table locked"}],"isError":true}`)},
	}
	content, failed := buildExecuteBatchContent(calls, outcomes)
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	texts := blockTexts(decodeBlocks(t, content))
	if !contains(texts, "[0] db.query — failed: tool reported an error") {
		t.Fatalf("missing upstream-error header in %v", texts)
	}
	if !contains(texts, "table locked") {
		t.Fatalf("upstream error content not passed through: %v", texts)
	}
}

func TestBuildExecuteBatchContentFallback(t *testing.T) {
	calls := batchCalls("a.tool", "b.tool")
	outcomes := []*callOutcome{
		{Result: json.RawMessage(`{"structuredContent":{"rows":[1,2]}}`)}, // 无 content 块
		{Result: json.RawMessage(`not-json`)},                            // 不可解析
	}
	content, failed := buildExecuteBatchContent(calls, outcomes)
	if failed != 0 {
		t.Fatalf("failed = %d, want 0(无 content 不算失败)", failed)
	}
	texts := blockTexts(decodeBlocks(t, content))
	if !containsHasPrefix(texts, `{"structuredContent"`) {
		t.Fatalf("missing raw fallback for content-less result: %v", texts)
	}
	if !containsHasPrefix(texts, "not-json") {
		t.Fatalf("missing raw fallback for unparseable result: %v", texts)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func containsHasPrefix(list []string, prefix string) bool {
	for _, v := range list {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}

// 校验路径不发 DB 请求,可以直接构造 handler 测协议级错误与 Logs 回落。
func TestHandleExecuteBatchValidation(t *testing.T) {
	h := NewGatewayHandler(nil, nil, nil)
	logCtx := &LogContext{UserID: 1, ApiKeyID: 1}

	cases := []struct {
		name string
		args string
		want string
	}{
		{"missing calls", `{}`, "calls is required"},
		{"empty calls", `{"calls":[]}`, "calls is required"},
		{"too many", `{"calls":[` + strings.Repeat(`{"tool_id":"a.b"},`, 10) + `{"tool_id":"c.d"}]}`, "too many calls"},
		{"missing tool_id", `{"calls":[{"arguments":{}}]}`, "calls[0].tool_id is required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := h.handleExecuteBatch(context.Background(), nil, logCtx, json.RawMessage(c.args), 0, "")
			if res.Resp == nil || res.Resp.Error == nil {
				t.Fatalf("expected protocol error, got %+v", res.Resp)
			}
			if res.Resp.Error.Code != -32602 {
				t.Fatalf("code = %d, want -32602", res.Resp.Error.Code)
			}
			if !strings.Contains(res.Resp.Error.Message, c.want) {
				t.Fatalf("message = %q, want contains %q", res.Resp.Error.Message, c.want)
			}
			if res.Logs != nil {
				t.Fatalf("validation failure should not produce item logs, got %d", len(res.Logs))
			}
		})
	}
}
