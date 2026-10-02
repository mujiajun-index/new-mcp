package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The production sender has no option allowing private destinations. Tests may
// replace these package-private transport hooks while preserving public DNS
// validation. ProxyFromEnvironment is deliberately not used.
type webhookSender struct {
	lookup    func(context.Context, string) ([]net.IPAddr, error)
	dial      func(context.Context, string, string) (net.Conn, error)
	tlsConfig *tls.Config
	timeout   time.Duration
}

func newWebhookSender() *webhookSender {
	d := &net.Dialer{Timeout: 5 * time.Second}
	return &webhookSender{lookup: net.DefaultResolver.LookupIPAddr, dial: d.DialContext, timeout: 5 * time.Second}
}

var forbiddenPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("224.0.0.0/4"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"), netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"), netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"), netip.MustParsePrefix("fe80::/10"), netip.MustParsePrefix("ff00::/8"),
}

func publicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	// Current IPv6 Internet unicast allocation is 2000::/3. Reject deprecated
	// site-local and unallocated space conservatively as well as IANA specials.
	if a.Is6() && !netip.MustParsePrefix("2000::/3").Contains(a) {
		return false
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(a) {
			return false
		}
	}
	return true
}

func callbackURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || len(raw) > 4096 {
		return nil, invalid("delivery.url must be an absolute HTTPS URL without credentials or fragment")
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, invalid("invalid callback port")
		}
	}
	if strings.Contains(u.Hostname(), "%") {
		return nil, invalid("callback IP zones are unsupported")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return nil, invalid("callback destination must be publicly routable")
	}
	return u, nil
}

func secret(raw string) ([]byte, error) {
	if !strings.HasPrefix(raw, "whsec_") || strings.ContainsAny(raw, " \r\n\t") {
		return nil, invalid("delivery.secret must be a whsec_ secret")
	}
	b, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(raw, "whsec_"))
	if err != nil || len(b) < 24 || len(b) > 64 {
		return nil, invalid("delivery.secret must encode 24 to 64 bytes")
	}
	return b, nil
}

func sign(key []byte, id, timestamp string, body []byte) string {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(id + "." + timestamp + "."))
	_, _ = h.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(h.Sum(nil))
}

type callbackFailure struct {
	reason string
	status int
}

func (e *callbackFailure) Error() string { return e.reason }
func callbackError(err error) error {
	reason := "connection_refused"
	var failure *callbackFailure
	if errors.As(err, &failure) {
		reason = failure.reason
	}
	return &RPCError{Code: -32015, Message: "CallbackEndpointError", Data: map[string]any{"reason": reason}}
}

func failureCategory(err error) string {
	var certErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var recordErr tls.RecordHeaderError
	if errors.As(err, &certErr) || errors.As(err, &unknownAuthority) || errors.As(err, &recordErr) {
		return "tls_error"
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return "timeout"
	}
	return "connection_refused"
}

// post resolves and validates all addresses afresh for EVERY attempt, then pins
// the connection to those validated IPs. TLS SNI and Host retain the original
// hostname. Disabling reuse ensures later attempts cannot bypass revalidation.
func (s *webhookSender) post(ctx context.Context, rawURL, id, subID string, keys [][]byte, body []byte) ([]byte, error) {
	u, err := callbackURL(rawURL)
	if err != nil {
		return nil, &callbackFailure{reason: "connection_refused"}
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	ips, err := s.lookup(ctx, u.Hostname())
	if err != nil {
		return nil, &callbackFailure{reason: failureCategory(err)}
	}
	if len(ips) == 0 {
		return nil, &callbackFailure{reason: "connection_refused"}
	}
	for _, addr := range ips {
		if addr.Zone != "" || !publicIP(addr.IP) {
			return nil, &callbackFailure{reason: "connection_refused"}
		}
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	tr := &http.Transport{TLSHandshakeTimeout: s.timeout, DisableKeepAlives: true, TLSClientConfig: s.tlsConfig}
	tr.DialContext = func(c context.Context, network, address string) (net.Conn, error) {
		var last error
		for _, addr := range ips {
			conn, err := s.dial(c, network, net.JoinHostPort(addr.IP.String(), port))
			if err == nil {
				return conn, nil
			}
			last = err
		}
		return nil, last
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, &callbackFailure{reason: "connection_refused"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", id)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	req.Header.Set("webhook-timestamp", timestamp)
	signatures := make([]string, 0, len(keys))
	for _, key := range keys {
		signatures = append(signatures, sign(key, id, timestamp, body))
	}
	req.Header.Set("webhook-signature", strings.Join(signatures, " "))
	req.Header.Set("X-MCP-Subscription-Id", subID)
	resp, err := client.Do(req)
	if err != nil {
		return nil, &callbackFailure{reason: failureCategory(err)}
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		category := "http_4xx"
		if resp.StatusCode >= 500 {
			category = "http_5xx"
		}
		return nil, &callbackFailure{reason: category, status: resp.StatusCode}
	}
	if err != nil {
		return nil, &callbackFailure{reason: failureCategory(err)}
	}
	if len(response) > 4096 {
		return nil, &callbackFailure{reason: "challenge_failed"}
	}
	return response, nil
}

type subscription struct {
	key, id, url     string
	principal        Principal
	feed             *feed
	ctx              context.Context
	cancel           context.CancelFunc
	done             chan struct{}
	expires          time.Time
	keyBytes, oldKey []byte
	oldKeyUntil      time.Time
	cursor           string
	lastDeliveryAt   *string
	lastError        *string
	failedSince      *string
	gapSinceRefresh  bool
}

func subscriptionKey(p Principal, req request) string {
	b, _ := json.Marshal([]string{p.ID, req.Delivery.URL, req.Name, string(req.Arguments)})
	sum := sha256.Sum256(b)
	return "sub_" + hex.EncodeToString(sum[:])
}

func (m *Manager) verify(ctx context.Context, p Principal, u, id string, key []byte) error {
	b, _ := json.Marshal([]string{p.ID, u})
	sum := sha256.Sum256(b)
	cacheKey := hex.EncodeToString(sum[:])
	m.mu.Lock()
	if expiry := m.verification[cacheKey]; time.Now().Before(expiry) {
		m.verification[cacheKey] = time.Now().Add(m.ttl)
		m.mu.Unlock()
		return nil
	}
	parsed, _ := url.Parse(u)
	host := parsed.Hostname()
	if last := m.verificationHosts[host]; time.Since(last) < 2*time.Second {
		m.mu.Unlock()
		return exhausted("callbackVerificationRate", 1)
	}
	// Bound verification cache/host state as well as subscriptions, including
	// unsuccessful requests by principals that vary their URL arguments.
	if _, exists := m.verificationHosts[host]; !exists && len(m.verificationHosts) >= feedLimit {
		m.mu.Unlock()
		return exhausted("callbackHosts", feedLimit)
	}
	m.verificationHosts[host] = time.Now()
	m.mu.Unlock()
	challenge := nonce()
	body, _ := json.Marshal(map[string]any{"type": "verification", "challenge": challenge})
	response, err := m.webhooks.post(ctx, u, "msg_verification_"+nonce(), id, [][]byte{key}, body)
	if err != nil {
		return callbackError(err)
	}
	var echoed struct {
		Challenge string `json:"challenge"`
	}
	if json.Unmarshal(response, &echoed) != nil || subtle.ConstantTimeCompare([]byte(echoed.Challenge), []byte(challenge)) != 1 {
		return callbackError(&callbackFailure{reason: "challenge_failed"})
	}
	if err = check(ctx, p); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return &RPCError{Code: -32603, Message: "Event manager closed"}
	}
	if _, exists := m.verification[cacheKey]; !exists && len(m.verification) >= feedLimit {
		return exhausted("callbackVerifications", feedLimit)
	}
	m.verification[cacheKey] = time.Now().Add(m.ttl)
	return nil
}

func (m *Manager) grantTTL(raw json.RawMessage) (time.Duration, error) {
	grant := m.ttl
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return grant, nil
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err != nil || n < 0 {
		return 0, invalid("ttlMs must be a nonnegative integer or null")
	}
	// The protocol permits a documented floor to avoid renewal storms. Test
	// managers with shorter caps use their cap as the floor.
	floor := time.Second
	if grant < floor {
		floor = grant
	}
	if n < grant.Milliseconds() {
		grant = time.Duration(n) * time.Millisecond
	}
	if grant < floor {
		grant = floor
	}
	return grant, nil
}

func (m *Manager) subscribe(ctx context.Context, p Principal, req request, source Source) (any, error) {
	if p.ID == "" {
		return nil, forbidden()
	}
	if req.Delivery == nil || req.Delivery.Mode != "webhook" {
		return nil, invalid("subscribe requires delivery.mode webhook")
	}
	if _, err := callbackURL(req.Delivery.URL); err != nil {
		return nil, err
	}
	keyBytes, err := secret(req.Delivery.Secret)
	if err != nil {
		return nil, err
	}
	grant, err := m.grantTTL(req.TTLMS)
	if err != nil {
		return nil, err
	}
	if req.MaxEvents != nil {
		return nil, invalid("webhook does not accept maxEvents")
	}
	// Mutating subscriptions is serialized so simultaneous refresh/unsubscribe
	// cannot attach an orphaned worker or multiply verification POSTs.
	m.webhookMu.Lock()
	defer m.webhookMu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	detach := context.AfterFunc(m.ctx, cancel)
	defer detach()
	key := subscriptionKey(p, req)
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: "Event manager closed"}
	}
	existing := m.subscriptions[key]
	if existing != nil && !time.Now().Before(existing.expires) {
		delete(m.subscriptions, key)
		existing.cancel()
		existing = nil
	}
	n := 0
	for _, s := range m.subscriptions {
		if s.principal.ID == p.ID {
			n++
		}
	}
	if existing == nil && n >= principalSubscriptionLimit {
		m.mu.Unlock()
		return nil, exhausted("subscriptions", principalSubscriptionLimit)
	}
	if existing == nil && len(m.subscriptions) >= feedLimit {
		m.mu.Unlock()
		return nil, exhausted("subscriptions", feedLimit)
	}
	m.mu.Unlock()
	if err = m.verify(ctx, p, req.Delivery.URL, key, keyBytes); err != nil {
		return nil, err
	}
	if err = check(ctx, p); err != nil {
		return nil, err
	}
	if existing != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed || m.subscriptions[key] != existing {
			return nil, &RPCError{Code: -32603, Message: "Subscription closed during refresh"}
		}
		if !hmac.Equal(existing.keyBytes, keyBytes) {
			existing.oldKey = existing.keyBytes
			existing.oldKeyUntil = time.Now().Add(30 * time.Second)
		}
		existing.keyBytes = keyBytes
		existing.expires = time.Now().Add(grant)
		result := m.subscriptionResultLocked(existing, existing.gapSinceRefresh)
		existing.gapSinceRefresh = false
		return result, nil
	}
	f, err := m.getFeed(ctx, p, req, source)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed || f.dead {
		m.mu.Unlock()
		return nil, &RPCError{Code: -32603, Message: "Event source closed"}
	}
	batch, pos, truncated, _ := m.readLocked(f, req.Cursor, req.MaxAgeMS, m.ringSize)
	if len(batch) > 0 {
		pos, _ = parseCursor(f, *batch[0].Cursor)
		pos--
	}
	subCtx, subCancel := context.WithCancel(m.ctx)
	s := &subscription{key: key, id: key, url: req.Delivery.URL, principal: p, feed: f, ctx: subCtx, cancel: subCancel, done: make(chan struct{}), expires: time.Now().Add(grant), keyBytes: keyBytes, cursor: cursor(f, pos)}
	m.subscriptions[key] = s
	f.refs++
	m.wg.Add(1)
	result := m.subscriptionResultLocked(s, truncated)
	m.mu.Unlock()
	go m.deliverLoop(s)
	return result, nil
}

func (m *Manager) subscriptionResultLocked(s *subscription, truncated bool) map[string]any {
	return map[string]any{"id": s.id, "refreshBefore": s.expires.UTC().Format(time.RFC3339Nano), "cursor": s.cursor, "truncated": truncated,
		"deliveryStatus": map[string]any{"active": true, "lastDeliveryAt": s.lastDeliveryAt, "lastError": s.lastError, "failedSince": s.failedSince}}
}

func (m *Manager) unsubscribe(ctx context.Context, p Principal, req request) (any, error) {
	if p.ID == "" {
		return nil, forbidden()
	}
	if req.Delivery == nil || req.Delivery.URL == "" || req.Delivery.Secret != "" || (req.Delivery.Mode != "" && req.Delivery.Mode != "webhook") || req.Cursor != nil || req.MaxAgeMS != nil || req.MaxEvents != nil || len(req.TTLMS) != 0 {
		return nil, invalid("unsubscribe requires name, arguments and delivery.url")
	}
	if _, err := callbackURL(req.Delivery.URL); err != nil {
		return nil, err
	}
	m.webhookMu.Lock()
	defer m.webhookMu.Unlock()
	if err := check(ctx, p); err != nil {
		return nil, err
	}
	key := subscriptionKey(p, req)
	m.mu.Lock()
	s := m.subscriptions[key]
	if s != nil {
		delete(m.subscriptions, key)
		s.cancel()
	}
	m.mu.Unlock()
	if s == nil {
		return nil, &RPCError{Code: -32011, Message: "NotFound", Data: map[string]any{"kind": "subscription"}}
	}
	<-s.done
	return map[string]any{}, nil
}

func (m *Manager) deliveryKeys(s *subscription) [][]byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := [][]byte{append([]byte(nil), s.keyBytes...)}
	if time.Now().Before(s.oldKeyUntil) {
		keys = append(keys, append([]byte(nil), s.oldKey...))
	}
	return keys
}

func (m *Manager) deliveryAuthorized(s *subscription) error {
	m.mu.Lock()
	live := !m.closed && m.subscriptions[s.key] == s && time.Now().Before(s.expires)
	m.mu.Unlock()
	if !live {
		return context.Canceled
	}
	if err := check(s.ctx, s.principal); err != nil {
		return err
	}
	return m.feedFailure(s.feed)
}

func (m *Manager) recordDelivery(s *subscription, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err == nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		s.lastDeliveryAt = &now
		s.lastError = nil
		s.failedSince = nil
		return
	}
	var failure *callbackFailure
	reason := "connection_refused"
	if errors.As(err, &failure) {
		reason = failure.reason
	}
	s.lastError = &reason
	if s.failedSince == nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		s.failedSince = &now
	}
}

// Events are delivered serially. A payload's cursor is the last accepted or
// abandoned position BEFORE this event, which is safe even if a receiver saves
// the cursor before its ACK reaches us. A refresh returns the newer watermark.
func (m *Manager) sendWithRetry(s *subscription, id string, body []byte, event *Event) (bool, error) {
	if len(body) > maxPayloadBytes {
		return false, &callbackFailure{reason: "http_4xx", status: 413}
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := m.deliveryAuthorized(s); err != nil {
			return false, err
		}
		if event != nil {
			if err := m.checkFeedEvent(s.ctx, s.principal, s.feed, *event); err != nil {
				return false, err
			}
		}
		_, err := m.webhooks.post(s.ctx, s.url, id, s.id, m.deliveryKeys(s), body)
		m.recordDelivery(s, err)
		if err == nil {
			return true, nil
		}
		last = err
		var failure *callbackFailure
		if errors.As(err, &failure) && (failure.status == 410 || failure.status == 413) {
			break
		}
		if attempt < 2 {
			timer := time.NewTimer(m.retryDelay * time.Duration(1<<attempt))
			select {
			case <-s.ctx.Done():
				timer.Stop()
				return false, s.ctx.Err()
			case <-timer.C:
			}
		}
	}
	return false, last
}

func (m *Manager) terminated(s *subscription, err error) {
	// Revocation stops event payloads. One signed control envelope informs the
	// previously verified endpoint, with a bounded timeout and no retry flood.
	m.mu.Lock()
	live := !m.closed && m.subscriptions[s.key] == s && time.Now().Before(s.expires)
	if live {
		// The endpoint may immediately re-subscribe as soon as this envelope
		// arrives, before its HTTP ACK. Remove the terminal instance first so
		// that refresh creates a new worker instead of reviving this one.
		delete(m.subscriptions, s.key)
	}
	m.mu.Unlock()
	if !live || s.ctx.Err() != nil {
		return
	}
	body, _ := json.Marshal(map[string]any{"type": "terminated", "error": sanitized(err)})
	_, _ = m.webhooks.post(s.ctx, s.url, "msg_terminated_"+nonce(), s.id, m.deliveryKeys(s), body)
}

func (m *Manager) deliverLoop(s *subscription) {
	defer m.wg.Done()
	defer close(s.done)
	defer func() {
		s.cancel()
		m.mu.Lock()
		if m.subscriptions[s.key] == s {
			delete(m.subscriptions, s.key)
		}
		s.feed.refs--
		s.feed.lastUsed = time.Now()
		stop := m.releaseFeedLocked(s.feed)
		m.mu.Unlock()
		if stop != nil {
			stop()
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := m.deliveryAuthorized(s); err != nil {
			if !errors.Is(err, context.Canceled) {
				m.terminated(s, err)
			}
			return
		}
		m.mu.Lock()
		batch, pos, gap, _ := m.readLocked(s.feed, &s.cursor, nil, m.ringSize)
		wake, dead := s.feed.wake, s.feed.dead
		if len(batch) == 0 && !gap {
			s.cursor = cursor(s.feed, pos)
		}
		m.mu.Unlock()
		if dead {
			m.terminated(s, &RPCError{Code: -32603, Message: "Event source closed"})
			return
		}
		if gap {
			m.mu.Lock()
			s.gapSinceRefresh = true
			m.mu.Unlock()
			reset := pos
			if len(batch) > 0 {
				reset, _ = parseCursor(s.feed, *batch[0].Cursor)
				reset--
			}
			fresh := cursor(s.feed, reset)
			body, _ := json.Marshal(map[string]any{"type": "gap", "cursor": fresh})
			if _, err := m.sendWithRetry(s, "msg_gap_"+nonce(), body, nil); err != nil {
				if _, ok := err.(*RPCError); ok {
					m.terminated(s, err)
					return
				}
				if s.ctx.Err() != nil {
					return
				}
			}
			m.mu.Lock()
			s.cursor = fresh
			m.mu.Unlock()
		}
		for _, event := range batch {
			original := *event.Cursor
			m.mu.Lock()
			watermark := s.cursor
			m.mu.Unlock()
			event.Cursor = &watermark
			body, _ := json.Marshal(event)
			_, err := m.sendWithRetry(s, event.EventID, body, &event)
			if err != nil {
				if _, ok := err.(*RPCError); ok {
					m.terminated(s, err)
					return
				}
				if s.ctx.Err() != nil || errors.Is(err, context.Canceled) {
					return
				}
			}
			// Accepted or exhausted: the position can now be safely advanced.
			m.mu.Lock()
			s.cursor = original
			m.mu.Unlock()
		}
		select {
		case <-s.ctx.Done():
			return
		case <-wake:
		case <-ticker.C:
		}
	}
}
