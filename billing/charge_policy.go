package billing

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	jsonrpc "github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/mujkjk/newmcp/model"
)

// CallFailure 一次上游调用的失败形态(§6.6 失败与边界处理)。
// 零值 = 成功(恒计费);失败形态经 Charge() 应用全局选项得出是否计费。
type CallFailure struct {
	Failed bool
	// ClientError 客户端侧错误:上游 JSON-RPC 解析/请求/方法/参数错误码,或
	// tools/call 结果内 isError=true(工具层失败)。MCP 协议不区分 4xx/5xx,
	// 工具层失败统一归此类,由 ChargeOnClientError 决定是否计费。
	ClientError bool
	// Timeout 超时(context 截止/连接超时),由 ChargeOnTimeout 决定是否计费。
	Timeout bool
}

// ClassifyCallFailure 按上游调用错误与结果内 isError 标志分类失败形态。
// err == nil 且 resultIsError=false → 零值(成功)。
// 上游内部错误/密钥失效/上游余额不足/传输故障等平台侧原因:Failed 且无两类
// 标记,恒退款。
func ClassifyCallFailure(err error, resultIsError bool) CallFailure {
	if err == nil {
		return CallFailure{Failed: resultIsError, ClientError: resultIsError}
	}
	f := CallFailure{Failed: true}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		f.Timeout = true
	}
	var re *jsonrpc.Error
	if errors.As(err, &re) {
		switch re.Code {
		case jsonrpc.CodeParseError, jsonrpc.CodeInvalidRequest, jsonrpc.CodeMethodNotFound, jsonrpc.CodeInvalidParams:
			f.ClientError = true
		}
	}
	return f
}

// Charge 该次调用是否计费:成功恒计费;失败默认退款,ChargeOnClientError=true 时
// 客户端侧错误计费,ChargeOnTimeout=true 时超时计费,其余失败恒退款。
func (f CallFailure) Charge() bool {
	if !f.Failed {
		return true
	}
	if f.ClientError && model.GetOptionBool("ChargeOnClientError") {
		return true
	}
	if f.Timeout && model.GetOptionBool("ChargeOnTimeout") {
		return true
	}
	return false
}

// ShouldChargeCall 组合便捷入口:分类并应用计费选项。
// 网关/手动测试的计费插入点 B 统一走这里,替代旧的 err==nil 判定。
func ShouldChargeCall(err error, resultIsError bool) bool {
	return ClassifyCallFailure(err, resultIsError).Charge()
}

// ToolResultIsError 解析 tools/call 结果 JSON 的 isError 标志(工具层失败)。
// 空/非法 JSON 一律按无标志处理(false)。
func ToolResultIsError(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var r struct {
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return false
	}
	return r.IsError
}
