package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	wsPingInterval = 30 * time.Second
	wsPongTimeout  = 90 * time.Second
	wsWriteTimeout = 10 * time.Second
	wsReadLimit    = 32 << 20
)

// NewWebSocketAdapter dials an upstream that serves raw JSON-RPC text frames.
func NewWebSocketAdapter(serviceID int64, endpoint string, headers map[string]string) *SDKAdapter {
	_ = serviceID
	h := make(http.Header, len(headers))
	for name, value := range headers {
		h.Set(name, value)
	}
	a := newSDKAdapter(TypeWebSocket)
	a.transport = &webSocketTransport{endpoint: endpoint, headers: h}
	return a
}

// NewPassiveWSAdapter uses an authenticated inbound socket. New MCP remains the
// MCP client: it initializes and discovers the local server behind the bridge.
func NewPassiveWSAdapter(serviceID int64, conn *websocket.Conn) *SDKAdapter {
	_ = serviceID
	a := newSDKAdapter(TypePassiveWS)
	a.transport = &webSocketTransport{conn: conn}
	return a
}

type webSocketTransport struct {
	endpoint string
	headers  http.Header
	conn     *websocket.Conn
}

func (t *webSocketTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	conn := t.conn
	if conn == nil {
		if t.endpoint == "" {
			return nil, fmt.Errorf("missing WebSocket endpoint or accepted connection")
		}
		dialer := *websocket.DefaultDialer
		dialer.HandshakeTimeout = 10 * time.Second
		var err error
		var resp *http.Response
		conn, resp, err = dialer.DialContext(ctx, t.endpoint, t.headers)
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			return nil, fmt.Errorf("dial WebSocket: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	rw := newWebSocketStream(conn)
	return (&mcp.IOTransport{Reader: rw, Writer: rw}).Connect(ctx)
}

// webSocketStream adds NDJSON delimiters only within the SDK's IO stream. The
// wire carries one raw JSON-RPC payload per text frame.
// The SDK serializes Read and Write; control writes use gorilla's safe API.
type webSocketStream struct {
	conn      *websocket.Conn
	readBuf   *bytes.Reader
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func newWebSocketStream(conn *websocket.Conn) *webSocketStream {
	w := &webSocketStream{conn: conn, done: make(chan struct{})}
	conn.SetReadLimit(wsReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
	})
	// The default ping handler already sends pong with WriteControl.
	go w.keepalive()
	return w
}

func (w *webSocketStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for w.readBuf == nil || w.readBuf.Len() == 0 {
		typ, data, err := w.conn.ReadMessage()
		if err != nil {
			return 0, err
		}
		if typ != websocket.TextMessage {
			return 0, fmt.Errorf("MCP WebSocket requires JSON text frames")
		}
		_ = w.conn.SetReadDeadline(time.Now().Add(wsPongTimeout))
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			return 0, fmt.Errorf("empty MCP WebSocket message")
		}
		w.readBuf = bytes.NewReader(append(data, '\n'))
	}
	return w.readBuf.Read(p)
}

func (w *webSocketStream) Write(p []byte) (int, error) {
	data := bytes.TrimSpace(p)
	if patched := withEmptyArgumentsIfMissing(data); patched != nil {
		data = patched
	}
	if err := w.conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return 0, err
	}
	if err := w.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *webSocketStream) Close() error {
	w.closeOnce.Do(func() {
		close(w.done)
		w.closeErr = w.conn.Close()
	})
	return w.closeErr
}

func (w *webSocketStream) keepalive() {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			if err := w.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsWriteTimeout)); err != nil {
				_ = w.Close()
				return
			}
		}
	}
}

var _ io.ReadWriteCloser = (*webSocketStream)(nil)
