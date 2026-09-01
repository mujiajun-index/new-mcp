package transport

import (
	"context"
	"encoding/json"
)

// DynamicAuth 由 bridge 层的 KeySelector 实现:按策略(随机/轮询)为上游请求
// 供认证头值,并在传输层观察到上游 401/403 时熔断对应秘钥。
type DynamicAuth interface {
	// Pick 返回秘钥池内序号(1 起)与完整头值(Bearer 前缀已拼好);
	// 池空或全部禁用时返回错误。
	Pick() (keyIndex int, headerValue string, err error)
	// OnAuthFailure 上游对 keyIndex 这把秘钥返回了 401/403,自动禁用之。
	OnAuthFailure(keyIndex int)
}

// CallMeta 是一次逻辑调用(tools/call 等)的附加元数据。
type CallMeta struct {
	// KeyIndex 多秘钥调用所用秘钥的池内序号;0 = 未使用多秘钥。
	KeyIndex int
}

// MetaCaller 是 TransportAdapter 的可选扩展:网关与服务测试路径类型断言后使用,
// 把本次调用实际使用的秘钥序号带回,写入 mcp_call_logs.key_index。
type MetaCaller interface {
	CallWithMeta(ctx context.Context, method string, params interface{}) (json.RawMessage, CallMeta, error)
}

// ResourceMetaCaller / PromptMetaCaller 是同款可选扩展:服务详情页资源/提示测试
// 路径类型断言后使用,作用与 MetaCaller 一致(带回秘钥序号落日志)。
type ResourceMetaCaller interface {
	ReadResourceWithMeta(ctx context.Context, uri string) (json.RawMessage, CallMeta, error)
}

type PromptMetaCaller interface {
	GetPromptWithMeta(ctx context.Context, name string, arguments map[string]string) (json.RawMessage, CallMeta, error)
}

// authChoice 是一次逻辑调用选定的秘钥,经 ctx 传给 RoundTripper:
// 同一次调用的 POST/GET 全程用同一把 key,并发调用互不串。
type authChoice struct {
	index int
	value string
}

type authChoiceCtxKey struct{}

// WithAuthChoice 把选定秘钥放入 ctx(SDKAdapter.Call 在调用 sess 前设置)。
func WithAuthChoice(ctx context.Context, index int, value string) context.Context {
	return context.WithValue(ctx, authChoiceCtxKey{}, authChoice{index: index, value: value})
}

func authChoiceFrom(ctx context.Context) (authChoice, bool) {
	c, ok := ctx.Value(authChoiceCtxKey{}).(authChoice)
	return c, ok
}
