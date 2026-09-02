package bridge

import (
	"testing"

	"github.com/mujkjk/newmcp/common"
)

// newTestSelector 构造内存态选择器(不触 DB;OnAuthFailure 的落库路径不在单测范围)。
func newTestSelector(mode string, statuses ...int) *KeySelector {
	keys := make([]keyEntry, len(statuses))
	for i, st := range statuses {
		keys[i] = keyEntry{ID: int64(i + 100), SortOrder: i + 1, Value: "k" + string(rune('1'+i)), Status: st}
	}
	return &KeySelector{owner: 1, kind: selectorKindService, svcName: "svc", target: "X-API-Key", mode: mode, keys: keys}
}

// TestKeySelectorPollingSkipsDisabled 轮询:跳过禁用、游标推进并回绕。
func TestKeySelectorPollingSkipsDisabled(t *testing.T) {
	// 三把:启用/手动禁用/启用 → 轮询序列应为 #1,#3,#1,#3...
	sel := newTestSelector(common.KeyModePolling, common.StatusEnabled, common.StatusDisabled, common.StatusEnabled)
	expect := []int{1, 3, 1, 3, 1}
	for i, want := range expect {
		got, _, err := sel.Pick()
		if err != nil {
			t.Fatalf("pick[%d]: %v", i, err)
		}
		if got != want {
			t.Fatalf("pick[%d] = #%d, want #%d", i, got, want)
		}
	}
}

// TestKeySelectorRandomSkipsDisabled 随机:只在启用集合内取。
func TestKeySelectorRandomSkipsDisabled(t *testing.T) {
	// #2/#3 禁用,仅 #1 可选 → 恒 #1
	sel := newTestSelector(common.KeyModeRandom, common.StatusEnabled, common.StatusDisabled, common.StatusAutoDisabled)
	for i := 0; i < 20; i++ {
		got, _, err := sel.Pick()
		if err != nil || got != 1 {
			t.Fatalf("pick[%d] = #%d (err %v), want #1", i, got, err)
		}
	}
}

// TestKeySelectorNoEnabledKeys 全禁用/空池 → 明确错误。
func TestKeySelectorNoEnabledKeys(t *testing.T) {
	for _, sel := range []*KeySelector{
		newTestSelector(common.KeyModePolling),
		newTestSelector(common.KeyModePolling, common.StatusDisabled),
		newTestSelector(common.KeyModeRandom, common.StatusAutoDisabled, common.StatusDisabled),
	} {
		if _, _, err := sel.Pick(); err == nil {
			t.Fatal("expected error for empty/exhausted pool")
		}
		if sel.HasEnabledKeys() {
			t.Fatal("HasEnabledKeys should be false")
		}
	}
}

// TestKeySelectorBearerPrefix bearer 认证下头值带前缀。
func TestKeySelectorBearerPrefix(t *testing.T) {
	sel := newTestSelector(common.KeyModePolling, common.StatusEnabled)
	sel.bearer = true
	_, v, err := sel.Pick()
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if v != "Bearer "+sel.keys[0].Value {
		t.Fatalf("value = %q, want %q", v, "Bearer "+sel.keys[0].Value)
	}
}

// TestKeySelectorItemPoolPick 条目级选择器与服务级共用 Pick 路径,
// kind 只影响熔断落库表与日志文案;这里验证内存行为一致 + bearer 位生效。
func TestKeySelectorItemPoolPick(t *testing.T) {
	sel := &KeySelector{
		owner: 7, kind: selectorKindItem, svcName: "条目", target: "Authorization",
		bearer: true, mode: common.KeyModePolling,
		keys: []keyEntry{
			{ID: 201, SortOrder: 1, Value: "a", Status: common.StatusEnabled},
			{ID: 202, SortOrder: 2, Value: "b", Status: common.StatusEnabled},
		},
	}
	for i, want := range []int{1, 2, 1} {
		got, v, err := sel.Pick()
		if err != nil || got != want {
			t.Fatalf("pick[%d] = #%d (err %v), want #%d", i, got, err, want)
		}
		wantVal := "Bearer " + sel.keys[want-1].Value
		if v != wantVal {
			t.Fatalf("pick[%d] value = %q, want %q", i, v, wantVal)
		}
	}
}
