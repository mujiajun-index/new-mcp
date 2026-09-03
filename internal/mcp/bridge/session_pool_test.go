package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/internal/mcp/transport"
	"github.com/mujkjk/newmcp/model"
)

// fakeAdapter 仅满足池逻辑测试:常连的 stdio 适配器,工具/资源均为空。
type fakeAdapter struct{}

func (f *fakeAdapter) Connect(ctx context.Context) error { return nil }
func (f *fakeAdapter) Close() error                      { return nil }
func (f *fakeAdapter) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAdapter) IsConnected() bool                            { return true }
func (f *fakeAdapter) GetType() transport.TransportType             { return transport.TypeStdio }
func (f *fakeAdapter) GetTools() []transport.Tool                   { return nil }
func (f *fakeAdapter) GetProtocolVersion() string                   { return "" }
func (f *fakeAdapter) GetServerInfo() *transport.ServerInfo         { return nil }
func (f *fakeAdapter) GetStdioProcess() *transport.StdioProcessInfo { return nil }
func (f *fakeAdapter) ListResources(ctx context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"resources":[]}`), nil
}
func (f *fakeAdapter) ListResourceTemplates(ctx context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"resourceTemplates":[]}`), nil
}
func (f *fakeAdapter) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAdapter) ListPrompts(ctx context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"prompts":[]}`), nil
}
func (f *fakeAdapter) GetPrompt(ctx context.Context, name string, arguments map[string]string) (json.RawMessage, error) {
	return nil, errors.New("not implemented")
}

// setOption 临时覆写内存 OptionMap 并在测试结束后还原。
func setOption(t *testing.T, key, value string) {
	t.Helper()
	model.OptionMapMutex.Lock()
	old, had := model.OptionMap[key]
	model.OptionMap[key] = value
	model.OptionMapMutex.Unlock()
	t.Cleanup(func() {
		model.OptionMapMutex.Lock()
		if had {
			model.OptionMap[key] = old
		} else {
			delete(model.OptionMap, key)
		}
		model.OptionMapMutex.Unlock()
	})
}

// TestAcquireSharedStdioConcurrencyLimit 共享条目会话(itemID≠0)租约数达到
// SharedStdioMaxConcurrency 后应返回 ErrServiceBusy,释放后可再租。
func TestAcquireSharedStdioConcurrencyLimit(t *testing.T) {
	setOption(t, "SharedStdioMaxConcurrency", "2")
	p := NewSessionPool()
	sess := &McpSession{key: sessionKey{itemID: 42}, Adapter: &fakeAdapter{}}
	p.sessions[sess.key] = sess

	r1, ok, err := p.AcquireSession(sess)
	if !ok || err != nil || r1 == nil {
		t.Fatalf("first acquire: ok=%v err=%v", ok, err)
	}
	r2, ok, err := p.AcquireSession(sess)
	if !ok || err != nil || r2 == nil {
		t.Fatalf("second acquire: ok=%v err=%v", ok, err)
	}

	if _, ok, err := p.AcquireSession(sess); !ok || !errors.Is(err, ErrServiceBusy) {
		t.Fatalf("third acquire should hit ErrServiceBusy, got ok=%v err=%v", ok, err)
	}

	r1() // 释放一个租约后应恢复可租
	if r, ok, err := p.AcquireSession(sess); !ok || err != nil || r == nil {
		t.Fatalf("acquire after release: ok=%v err=%v", ok, err)
	}
}

// TestAcquireUnlimitedForNonSharedSessions 自有/独占会话(serviceID 键控)
// 不受共享并发上限约束。
func TestAcquireUnlimitedForNonSharedSessions(t *testing.T) {
	setOption(t, "SharedStdioMaxConcurrency", "1")
	p := NewSessionPool()
	sess := &McpSession{key: sessionKey{serviceID: 7}, Adapter: &fakeAdapter{}}
	p.sessions[sess.key] = sess

	for i := 0; i < 5; i++ {
		if r, ok, err := p.AcquireSession(sess); !ok || err != nil || r == nil {
			t.Fatalf("acquire #%d should never be limited: ok=%v err=%v", i+1, ok, err)
		}
	}
}

// TestAcquireZeroLimitUnlimited 上限配置为 0 时共享会话同样不限流。
func TestAcquireZeroLimitUnlimited(t *testing.T) {
	setOption(t, "SharedStdioMaxConcurrency", "0")
	p := NewSessionPool()
	sess := &McpSession{key: sessionKey{itemID: 42}, Adapter: &fakeAdapter{}}
	p.sessions[sess.key] = sess

	for i := 0; i < 5; i++ {
		if r, ok, err := p.AcquireSession(sess); !ok || err != nil || r == nil {
			t.Fatalf("acquire #%d with zero limit: ok=%v err=%v", i+1, ok, err)
		}
	}
}

// TestConnectLockPerKeySerialization per-key 建连锁:同键串行、异键并行,
// 引用计数归零后锁条目从池中清空。
func TestConnectLockPerKeySerialization(t *testing.T) {
	p := NewSessionPool()
	ka := sessionKey{serviceID: 1}
	kmA := p.lockConnect(ka)

	// 同键第二个加锁者应阻塞
	blocked := make(chan struct{})
	go func() {
		km2 := p.lockConnect(ka)
		p.unlockConnect(ka, km2)
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatal("per-key lock failed to serialize same-key connectors")
	case <-time.After(50 * time.Millisecond):
	}

	// 异键不受影响
	free := make(chan struct{})
	kb := sessionKey{serviceID: 2}
	go func() {
		kmB := p.lockConnect(kb)
		p.unlockConnect(kb, kmB)
		close(free)
	}()
	select {
	case <-free:
	case <-time.After(time.Second):
		t.Fatal("different-key connector must not be blocked")
	}

	p.unlockConnect(ka, kmA)
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("same-key connector should proceed after unlock")
	}

	if n := len(p.connectLocks); n != 0 {
		t.Fatalf("connect locks should be cleaned up after full release, remain %d", n)
	}
}
