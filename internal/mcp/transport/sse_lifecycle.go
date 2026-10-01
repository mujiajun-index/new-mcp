package transport

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The SDK SSE GET retains its Connect context for the entire stream. Detach
// after endpoint discovery so cancellation of one request does not close a
// pooled connection, while still allowing callers to cancel initial discovery.
type sseLifecycleTransport struct {
	inner *mcp.SSEClientTransport
}

func (t *sseLifecycleTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	lifetimeCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(ctx, cancel)
	conn, err := t.inner.Connect(lifetimeCtx)
	stop()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		cancel()
		_ = conn.Close()
		return nil, err
	}
	return &sseLifecycleConnection{Connection: conn, cancel: cancel}, nil
}

type sseLifecycleConnection struct {
	mcp.Connection
	cancel context.CancelFunc
	once   sync.Once
	err    error
}

func (c *sseLifecycleConnection) Close() error {
	c.once.Do(func() {
		c.cancel()
		c.err = c.Connection.Close()
	})
	return c.err
}
