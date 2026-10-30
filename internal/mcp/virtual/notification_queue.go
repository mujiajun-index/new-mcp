package virtual

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/mujkjk/newmcp/internal/mcp/transport"
)

// Invalidation messages are coalesced by method/URI. A slow client cannot block
// the service observer or cause an unbounded event backlog.
type notificationQueue struct {
	mu      sync.Mutex
	pending map[string]transport.Notification
	order   []string
	wake    chan struct{}
	ctx     context.Context
}

func newNotificationQueue(ctx context.Context, done <-chan struct{}, emit func(transport.Notification)) *notificationQueue {
	q := &notificationQueue{ctx: ctx, pending: map[string]transport.Notification{}, wake: make(chan struct{}, 1)}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-q.wake:
			}
			for {
				q.mu.Lock()
				if len(q.order) == 0 {
					q.mu.Unlock()
					break
				}
				key := q.order[0]
				q.order = q.order[1:]
				n := q.pending[key]
				delete(q.pending, key)
				q.mu.Unlock()
				if ctx.Err() != nil {
					return
				}
				select {
				case <-done:
					return
				default:
				}
				emit(n)
			}
		}
	}()
	return q
}
func (q *notificationQueue) enqueue(n transport.Notification) {
	if q.ctx.Err() != nil {
		return
	}
	key := n.Method
	if n.Method == "notifications/resources/updated" {
		var p struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(n.Params, &p)
		key += "\x00" + p.URI
	}
	q.mu.Lock()
	if _, exists := q.pending[key]; !exists {
		q.order = append(q.order, key)
	}
	q.pending[key] = n
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}
