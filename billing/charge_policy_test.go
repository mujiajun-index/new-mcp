package billing

import (
	"context"
	"errors"
	"fmt"
	"testing"

	jsonrpc "github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/mujkjk/newmcp/model"
)

// setChargeOption 测试内临时翻转计费选项,defer 恢复默认。
func setChargeOption(t *testing.T, key string, on bool) {
	t.Helper()
	model.OptionMapMutex.Lock()
	old := model.OptionMap[key]
	if on {
		model.OptionMap[key] = "true"
	} else {
		model.OptionMap[key] = "false"
	}
	model.OptionMapMutex.Unlock()
	t.Cleanup(func() {
		model.OptionMapMutex.Lock()
		model.OptionMap[key] = old
		model.OptionMapMutex.Unlock()
	})
}

// 结果内 isError(工具层失败,上游 key 错误/余额不足等常见形态):
// 默认退款(修复点:此前 err==nil 判定把这类失败当成功扣费),开关打开才计费。
func TestShouldChargeResultIsError(t *testing.T) {
	setupPricingTest(t)

	if !ShouldChargeCall(nil, false) {
		t.Fatal("成功调用应计费")
	}
	if ShouldChargeCall(nil, true) {
		t.Fatal("结果内 isError 默认(ChargeOnClientError=false)应退款")
	}
	setChargeOption(t, "ChargeOnClientError", true)
	if !ShouldChargeCall(nil, true) {
		t.Fatal("ChargeOnClientError=true 时结果内 isError 应计费")
	}
}

// 上游 JSON-RPC 错误:客户端码(解析/请求/方法/参数)归 ChargeOnClientError 管控,
// 内部错误(密钥失效/上游余额不足等平台侧)恒退款。
func TestShouldChargeRPCError(t *testing.T) {
	setupPricingTest(t)

	clientCodes := []int64{jsonrpc.CodeParseError, jsonrpc.CodeInvalidRequest, jsonrpc.CodeMethodNotFound, jsonrpc.CodeInvalidParams}
	for _, code := range clientCodes {
		err := fmt.Errorf("wrapped: %w", &jsonrpc.Error{Code: code, Message: "bad request"})
		if ShouldChargeCall(err, false) {
			t.Fatalf("客户端码 %d 默认应退款", code)
		}
	}
	internalErr := fmt.Errorf("upstream: %w", &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "Invalid API key"})
	if ShouldChargeCall(internalErr, false) {
		t.Fatal("上游内部错误默认应退款")
	}
	setChargeOption(t, "ChargeOnClientError", true)
	for _, code := range clientCodes {
		err := fmt.Errorf("wrapped: %w", &jsonrpc.Error{Code: code, Message: "bad request"})
		if !ShouldChargeCall(err, false) {
			t.Fatalf("ChargeOnClientError=true 时客户端码 %d 应计费", code)
		}
	}
	if ShouldChargeCall(internalErr, false) {
		t.Fatal("ChargeOnClientError=true 时上游内部错误仍应退款(平台侧失败恒退款)")
	}
	// 非结构化错误(连接失败/进程崩溃文本等):不可归类,恒退款。
	if ShouldChargeCall(errors.New("connection refused"), false) {
		t.Fatal("传输故障应退款")
	}
}

// 超时:默认退款,ChargeOnTimeout=true 才计费;超时不受 ChargeOnClientError 影响。
func TestShouldChargeTimeout(t *testing.T) {
	setupPricingTest(t)

	if ShouldChargeCall(context.DeadlineExceeded, false) {
		t.Fatal("超时默认应退款")
	}
	setChargeOption(t, "ChargeOnTimeout", true)
	if !ShouldChargeCall(context.DeadlineExceeded, false) {
		t.Fatal("ChargeOnTimeout=true 时超时应计费")
	}
	// 包装后的超时同样识别。
	setChargeOption(t, "ChargeOnTimeout", false)
	wrapped := fmt.Errorf("call tools/call: %w", context.DeadlineExceeded)
	if ShouldChargeCall(wrapped, false) {
		t.Fatal("包装超时默认应退款")
	}
}

func TestToolResultIsError(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"error flag", `{"content":[{"type":"text","text":"Invalid API key"}],"isError":true}`, true},
		{"no flag", `{"content":[{"type":"text","text":"ok"}]}`, false},
		{"flag false", `{"isError":false}`, false},
		{"empty", ``, false},
		{"invalid json", `{not json`, false},
	}
	for _, c := range cases {
		if got := ToolResultIsError([]byte(c.raw)); got != c.want {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
