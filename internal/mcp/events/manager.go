package events

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	softTTL                    = 5 * time.Minute
	ringLimit                  = 256
	feedLimit                  = 1024
	principalFeedLimit         = 64
	principalSubscriptionLimit = 32
	principalStreamLimit       = 32
	maxPayloadBytes            = 256 << 10
	maxFeedBytes               = 1 << 20
	maxBufferBytes             = 16 << 20
)

type storedEvent struct {
	Event
	seq   uint64
	at    time.Time
	bytes int
}

type feed struct {
	key, epoch string
	principal  Principal
	name       string
	arguments  json.RawMessage
	ctx        context.Context
	cancel     context.CancelFunc
	stop       func()
	ready      chan struct{}
	err        error
	lastUsed   time.Time
	refs       int
	seq        uint64
	ring       []storedEvent
	wake       chan struct{}
	dead       bool
	bytes      int
	pollLease  time.Time
}

type Manager struct {
	mu                                                   sync.Mutex
	webhookMu                                            sync.Mutex
	ctx                                                  context.Context
	cancel                                               context.CancelFunc
	closed                                               bool
	feeds                                                map[string]*feed
	subscriptions                                        map[string]*subscription
	streams                                              map[*feed]int
	verification                                         map[string]time.Time
	verificationHosts                                    map[string]time.Time
	wg                                                   sync.WaitGroup
	ttl, retention, heartbeat, sweepInterval, retryDelay time.Duration
	ringSize                                             int
	webhooks                                             *webhookSender
	bufferBytes                                          int
}

func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx: ctx, cancel: cancel, feeds: map[string]*feed{}, subscriptions: map[string]*subscription{},
		streams: map[*feed]int{}, verification: map[string]time.Time{}, verificationHosts: map[string]time.Time{},
		ttl: softTTL, retention: softTTL, heartbeat: 25 * time.Second, sweepInterval: time.Second, retryDelay: time.Second,
		ringSize: ringLimit, webhooks: newWebhookSender(),
	}
	m.wg.Add(1)
	go m.sweepLoop()
	return m
}

func nonce() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("events: secure randomness unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func feedKey(p Principal, req request) string {
	b, _ := json.Marshal([]string{p.ID, req.Name, string(req.Arguments)})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) Handle(ctx context.Context, p Principal, method string, raw json.RawMessage, source Source) (any, error) {
	if err := check(ctx, p); err != nil {
		return nil, err
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, &RPCError{Code: -32603, Message: "Event manager closed"}
	}
	if method == "events/list" {
		var params struct {
			Cursor *string `json:"cursor"`
		}
		if err := decode(raw, &params); err != nil {
			return nil, err
		}
		if params.Cursor != nil {
			return nil, invalid("event catalog has no additional pages")
		}
		return map[string]any{"events": catalog()}, nil
	}
	if method != "events/poll" && method != "events/subscribe" && method != "events/unsubscribe" {
		return nil, &RPCError{Code: -32601, Message: "Method not found"}
	}
	req, err := parseRequest(raw)
	if err != nil {
		return nil, err
	}
	switch method {
	case "events/subscribe":
		return m.subscribe(ctx, p, req, source)
	case "events/unsubscribe":
		return m.unsubscribe(ctx, p, req)
	}
	if req.Delivery != nil || len(req.TTLMS) != 0 {
		return nil, invalid("poll does not accept delivery or ttlMs")
	}
	f, err := m.getFeed(ctx, p, req, source)
	if err != nil {
		return nil, err
	}
	if err = check(ctx, p); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if f.dead {
		m.mu.Unlock()
		return nil, sanitized(f.err)
	}
	f.lastUsed = time.Now()
	f.pollLease = time.Now().Add(m.ttl)
	batch, pos, truncated, more := m.readLocked(f, req.Cursor, req.MaxAgeMS, limit(req.MaxEvents))
	m.mu.Unlock()
	for _, e := range batch {
		if err := m.checkFeedEvent(ctx, p, f, e); err != nil {
			return nil, err
		}
	}
	return map[string]any{"events": batch, "cursor": cursor(f, pos), "truncated": truncated, "hasMore": more, "nextPollMs": 1000}, nil
}

func limit(n *int) int {
	if n == nil {
		return 50
	}
	if *n > ringLimit {
		return ringLimit
	}
	return *n
}

func (m *Manager) getFeed(ctx context.Context, p Principal, req request, source Source) (*feed, error) {
	key := feedKey(p, req)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: "Event manager closed"}
	}
	if f := m.feeds[key]; f != nil {
		f.lastUsed = time.Now()
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-f.ready:
			m.mu.Lock()
			err, dead := f.err, f.dead
			m.mu.Unlock()
			if dead || err != nil {
				return nil, sanitized(err)
			}
			return f, nil
		}
	}
	if source == nil {
		m.mu.Unlock()
		return nil, &RPCError{Code: -32014, Message: "Unsupported", Data: map[string]any{"feature": "eventSource"}}
	}
	n := 0
	for _, f := range m.feeds {
		if f.principal.ID == p.ID {
			n++
		}
	}
	if n >= principalFeedLimit {
		m.mu.Unlock()
		return nil, exhausted("eventSources", principalFeedLimit)
	}
	if len(m.feeds) >= feedLimit {
		m.mu.Unlock()
		return nil, exhausted("eventSources", feedLimit)
	}
	feedCtx, cancel := context.WithCancel(m.ctx)
	f := &feed{key: key, epoch: nonce(), principal: p, name: req.Name, arguments: append(json.RawMessage(nil), req.Arguments...), ctx: feedCtx, cancel: cancel, ready: make(chan struct{}), wake: make(chan struct{}), lastUsed: time.Now()}
	f.ctx = context.WithValue(feedCtx, sourceFailureContextKey{}, func(err error) bool { return m.failFeed(f, err) })
	m.feeds[key] = f
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()
	stop, err := source(f.ctx, req.Name, req.Arguments, func(e Event) { m.publish(f, e) })
	m.mu.Lock()
	f.stop = stop
	if err != nil {
		f.err = err
	} else if f.dead {
		err = f.err
	}
	if err != nil {
		f.dead = true
		// An asynchronous failure can detach this feed before setup returns.
		// Never remove a recovered feed created under the same subscription key.
		if m.feeds[key] == f {
			delete(m.feeds, key)
			m.bufferBytes -= f.bytes
			f.bytes = 0
			f.ring = nil
		}
		cancel()
	}
	dead := f.dead || m.closed
	close(f.ready)
	m.mu.Unlock()
	if dead && stop != nil {
		stop()
	}
	if err != nil {
		return nil, sanitized(err)
	}
	if dead {
		return nil, &RPCError{Code: -32603, Message: "Event source closed"}
	}
	return f, nil
}

func (m *Manager) failFeed(f *feed, err error) bool {
	m.mu.Lock()
	if m.closed || f.dead || m.feeds[f.key] != f {
		m.mu.Unlock()
		return false
	}
	f.dead = true
	f.err = sanitized(err)
	// A failed source must not prevent an immediate reconnect under the same
	// key. Existing workers keep this old object and observe its terminal error;
	// a new feed receives its own epoch so old cursors correctly signal a gap.
	delete(m.feeds, f.key)
	m.bufferBytes -= f.bytes
	f.bytes = 0
	f.ring = nil
	f.cancel()
	close(f.wake)
	f.wake = make(chan struct{})
	stop := f.stop
	f.stop = nil
	if stop != nil {
		m.wg.Add(1)
	}
	m.mu.Unlock()
	if stop != nil {
		go func() { defer m.wg.Done(); stop() }()
	}
	return true
}

func (m *Manager) feedFailure(f *feed) error {
	m.mu.Lock()
	dead, err := f.dead || m.closed, f.err
	m.mu.Unlock()
	if dead {
		return sanitized(err)
	}
	return nil
}

// Cached batches belong to their original feed. Check both sides of the
// gateway's potentially blocking permission callback so a source failure while
// that callback runs cannot resume delivery from a detached feed.
func (m *Manager) checkFeedEvent(ctx context.Context, p Principal, f *feed, e Event) error {
	if err := m.feedFailure(f); err != nil {
		return err
	}
	if err := checkEvent(ctx, p, e); err != nil {
		return err
	}
	return m.feedFailure(f)
}

func (m *Manager) publish(f *feed, e Event) {
	if e.Name != "" && e.Name != f.name {
		return
	}
	e.Name = f.name
	if e.EventID == "" {
		e.EventID = "evt_" + nonce()
	}
	if len(e.EventID) > 512 || strings.ContainsAny(e.EventID, "\r\n") {
		return
	}
	if _, err := time.Parse(time.RFC3339Nano, e.Timestamp); err != nil {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if e.Data == nil {
		e.Data = map[string]any{}
	}
	// Snapshot the emitter's mutable data and impose a hard per-event memory cap.
	raw, err := json.Marshal(e.Data)
	if err != nil || len(raw) > maxPayloadBytes-2048 || len(raw) == 0 || raw[0] != '{' {
		return
	}
	e.Data = json.RawMessage(raw)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || f.dead || f.ctx.Err() != nil {
		return
	}
	f.seq++
	c := cursor(f, f.seq)
	e.Cursor = &c
	size := len(raw) + len(e.EventID) + len(e.Timestamp) + 512
	f.ring = append(f.ring, storedEvent{Event: e, seq: f.seq, at: time.Now(), bytes: size})
	f.bytes += size
	m.bufferBytes += size
	m.trimLocked(f)
	close(f.wake)
	f.wake = make(chan struct{})
}

func cursor(f *feed, seq uint64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(f.epoch + "." + strconv.FormatUint(seq, 10)))
}

func parseCursor(f *feed, token string) (uint64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return 0, false
	}
	epoch, number, ok := strings.Cut(string(raw), ".")
	if !ok || epoch != f.epoch {
		return 0, false
	}
	n, err := strconv.ParseUint(number, 10, 64)
	return n, err == nil
}

func (m *Manager) trimLocked(f *feed) {
	floor := time.Now().Add(-m.retention)
	n := 0
	for n < len(f.ring) && (f.ring[n].at.Before(floor) || len(f.ring)-n > m.ringSize || f.bytes > maxFeedBytes || m.bufferBytes > maxBufferBytes) {
		f.bytes -= f.ring[n].bytes
		m.bufferBytes -= f.ring[n].bytes
		n++
	}
	if n > 0 {
		copy(f.ring, f.ring[n:])
		for i := len(f.ring) - n; i < len(f.ring); i++ {
			f.ring[i] = storedEvent{}
		}
		f.ring = f.ring[:len(f.ring)-n]
	}
}

// readLocked returns intermediate cursors for capped batches; it never advances
// beyond undelivered matching events. All ring data is scoped to one principal.
func (m *Manager) readLocked(f *feed, token *string, maxAge *int64, max int) ([]Event, uint64, bool, bool) {
	m.trimLocked(f)
	batch := []Event{}
	if token == nil {
		return batch, f.seq, false, false
	}
	pos, ok := parseCursor(f, *token)
	if !ok || pos > f.seq {
		return batch, f.seq, true, false
	}
	truncated := false
	first := f.seq + 1
	if len(f.ring) > 0 {
		first = f.ring[0].seq
	}
	if pos+1 < first {
		pos = first - 1
		truncated = true
	}
	var floor time.Time
	if maxAge != nil {
		age := m.retention
		if *maxAge < age.Milliseconds() {
			age = time.Duration(*maxAge) * time.Millisecond
		}
		floor = time.Now().Add(-age)
	}
	for _, e := range f.ring {
		if e.seq <= pos {
			continue
		}
		occurred, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
		if !floor.IsZero() && occurred.Before(floor) {
			pos = e.seq
			truncated = true
			continue
		}
		if len(batch) >= max {
			return batch, pos, truncated, true
		}
		copyEvent := e.Event
		data := e.Data.(json.RawMessage)
		copyEvent.Data = append(json.RawMessage(nil), data...)
		c := *e.Cursor
		copyEvent.Cursor = &c
		batch = append(batch, copyEvent)
		pos = e.seq
	}
	return batch, f.seq, truncated, false
}

// Stream leaves subscription-id correlation and the final JSON-RPC response to
// the transport, which knows the parent request's ID. stop cancels and waits for
// this stream's worker, ensuring no callback can write after it returns.
func (m *Manager) Stream(ctx context.Context, p Principal, raw json.RawMessage, source Source) (<-chan Notification, func(), error) {
	if err := check(ctx, p); err != nil {
		return nil, nil, err
	}
	req, err := parseRequest(raw)
	if err != nil {
		return nil, nil, err
	}
	if req.Delivery != nil || len(req.TTLMS) != 0 || req.MaxEvents != nil {
		return nil, nil, invalid("push does not accept delivery, ttlMs or maxEvents")
	}
	f, err := m.getFeed(ctx, p, req, source)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	n := 0
	for sf, count := range m.streams {
		if sf.principal.ID == p.ID {
			n += count
		}
	}
	if n >= principalStreamLimit {
		m.mu.Unlock()
		return nil, nil, exhausted("streams", principalStreamLimit)
	}
	if m.closed || f.dead {
		m.mu.Unlock()
		return nil, nil, &RPCError{Code: -32603, Message: "Event source closed"}
	}
	f.refs++
	m.streams[f]++
	batch, pos, truncated, _ := m.readLocked(f, req.Cursor, req.MaxAgeMS, m.ringSize)
	startPos := pos
	if len(batch) > 0 {
		startPos, _ = parseCursor(f, *batch[0].Cursor)
		startPos--
	}
	m.wg.Add(1)
	m.mu.Unlock()
	streamCtx, cancel := context.WithCancel(ctx)
	out := make(chan Notification, 1)
	done := make(chan struct{})
	go func() {
		defer m.wg.Done()
		defer close(done)
		defer close(out)
		defer func() {
			m.mu.Lock()
			f.refs--
			m.streams[f]--
			if m.streams[f] == 0 {
				delete(m.streams, f)
			}
			f.lastUsed = time.Now()
			stop := m.releaseFeedLocked(f)
			m.mu.Unlock()
			if stop != nil {
				stop()
			}
		}()
		ticker := time.NewTicker(m.heartbeat)
		defer ticker.Stop()
		send := func(n Notification) bool {
			select {
			case out <- n:
				return true
			case <-streamCtx.Done():
				return false
			case <-m.ctx.Done():
				return false
			}
		}
		terminate := func(err error) {
			timer := time.NewTimer(time.Second)
			defer timer.Stop()
			select {
			case out <- Notification{Method: "notifications/events/terminated", Params: map[string]any{"error": sanitized(err)}}:
			case <-streamCtx.Done():
			case <-timer.C:
			}
		}
		if err := check(streamCtx, p); err != nil {
			terminate(err)
			return
		}
		if !send(Notification{Method: "notifications/events/active", Params: map[string]any{"cursor": cursor(f, startPos), "truncated": truncated}}) {
			return
		}
		for _, e := range batch {
			if err := m.checkFeedEvent(streamCtx, p, f, e); err != nil {
				terminate(err)
				return
			}
			if !send(Notification{Method: "notifications/events/event", Params: e}) {
				return
			}
		}
		token := cursor(f, pos)
		for {
			m.mu.Lock()
			batch, pos, gap, _ := m.readLocked(f, &token, nil, m.ringSize)
			wake, dead, feedErr := f.wake, f.dead, f.err
			m.mu.Unlock()
			if dead {
				terminate(sanitized(feedErr))
				return
			}
			if err := check(streamCtx, p); err != nil {
				terminate(err)
				return
			}
			if gap {
				gapPos := pos
				if len(batch) > 0 {
					gapPos, _ = parseCursor(f, *batch[0].Cursor)
					gapPos--
				}
				if !send(Notification{Method: "notifications/events/active", Params: map[string]any{"cursor": cursor(f, gapPos), "truncated": true}}) {
					return
				}
			}
			for _, e := range batch {
				if err := m.checkFeedEvent(streamCtx, p, f, e); err != nil {
					terminate(err)
					return
				}
				if !send(Notification{Method: "notifications/events/event", Params: e}) {
					return
				}
			}
			token = cursor(f, pos)
			select {
			case <-streamCtx.Done():
				return
			case <-m.ctx.Done():
				terminate(&RPCError{Code: -32603, Message: "Server shutting down"})
				return
			case <-wake:
			case <-ticker.C:
				if err := check(streamCtx, p); err != nil {
					terminate(err)
					return
				}
				if !send(Notification{Method: "notifications/events/heartbeat", Params: map[string]any{"cursor": token}}) {
					return
				}
			}
		}
	}()
	var once sync.Once
	return out, func() { once.Do(func() { cancel(); <-done }) }, nil
}

func (m *Manager) releaseFeedLocked(f *feed) func() {
	if f.dead || f.refs != 0 || time.Now().Before(f.pollLease) {
		return nil
	}
	if m.feeds[f.key] != f {
		return nil
	}
	delete(m.feeds, f.key)
	f.dead = true
	f.cancel()
	close(f.wake)
	f.wake = make(chan struct{})
	m.bufferBytes -= f.bytes
	f.bytes = 0
	f.ring = nil
	stop := f.stop
	f.stop = nil
	return stop
}

func (m *Manager) sweepLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case now := <-ticker.C:
			m.sweep(now)
		}
	}
}

func (m *Manager) sweep(now time.Time) {
	var stops []func()
	m.mu.Lock()
	for key, s := range m.subscriptions {
		if !now.Before(s.expires) {
			delete(m.subscriptions, key)
			s.cancel()
		}
	}
	for key, f := range m.feeds {
		m.trimLocked(f)
		if f.refs == 0 && now.Sub(f.lastUsed) >= m.ttl {
			delete(m.feeds, key)
			f.dead = true
			m.bufferBytes -= f.bytes
			f.bytes = 0
			f.ring = nil
			f.cancel()
			close(f.wake)
			f.wake = make(chan struct{})
			if f.stop != nil {
				stops = append(stops, f.stop)
			}
		}
	}
	for key, expiry := range m.verification {
		if !now.Before(expiry) {
			delete(m.verification, key)
		}
	}
	for host, last := range m.verificationHosts {
		if now.Sub(last) >= m.ttl {
			delete(m.verificationHosts, host)
		}
	}
	m.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.cancel()
	var stops []func()
	for _, s := range m.subscriptions {
		s.cancel()
	}
	for _, f := range m.feeds {
		f.dead = true
		f.cancel()
		if f.stop != nil {
			stops = append(stops, f.stop)
		}
	}
	m.feeds = map[string]*feed{}
	m.bufferBytes = 0
	m.subscriptions = map[string]*subscription{}
	m.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
	m.wg.Wait()
}

func (m *Manager) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return fmt.Sprintf("events.Manager{sources:%d,webhooks:%d}", len(m.feeds), len(m.subscriptions))
}
