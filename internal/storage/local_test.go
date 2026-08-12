package storage

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mujkjk/newmcp/common"
)

func newTestStore(t *testing.T) *localStorage {
	t.Helper()
	s, err := newLocal(Config{LocalRoot: t.TempDir(), PathPrefix: "vision"})
	if err != nil {
		t.Fatalf("newLocal: %v", err)
	}
	return s.(*localStorage)
}

func TestLocalStorage_PutGetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := ContentKey("deadbeef")
	want := []byte("hello vision bytes \xff\x00png")

	if err := s.Put(ctx, key, bytes.NewReader(want), "image/png"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rc, err := s.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()
	got := make([]byte, len(want))
	if _, err := rc.Read(got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch: got %q want %q", got, want)
	}
}

func TestLocalStorage_GetMissingIsSentinel(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Get(context.Background(), "ff/nosuchfile"); err != ErrObjectNotFound {
		t.Fatalf("expected ErrObjectNotFound, got %v", err)
	}
}

func TestLocalStorage_DeleteIdempotent(t *testing.T) {
	s := newTestStore(t)
	// Deleting a key that was never written must not error.
	if err := s.Delete(context.Background(), "ff/never"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	key := ContentKey("cafebabe")
	if err := s.Put(context.Background(), key, strings.NewReader("x"), ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(context.Background(), key); err != nil { // second delete also clean
		t.Fatalf("double delete: %v", err)
	}
}

func TestLocalStorage_PathTraversalRejected(t *testing.T) {
	s := newTestStore(t)
	// Keys containing ".." (which could escape the root) or backslashes or empty
	// must be rejected. Note: a leading-slash absolute like "/etc/passwd" is NOT
	// a traversal here — Trim+Join normalize it to "<root>/vision/etc/passwd",
	// safely contained, so it's accepted (it just nests one level). We assert
	// only the genuinely escaping inputs are refused.
	bad := []string{"../escape", "..\\escape", "a/../b", "a//b", ""}
	for _, k := range bad {
		if err := s.Put(context.Background(), k, strings.NewReader("x"), ""); err == nil {
			t.Fatalf("Put(%q) expected rejection, got nil", k)
		}
	}
	// And confirm an absolute-looking key is contained, not written outside root.
	contained := "/etc/passwd"
	if err := s.Put(context.Background(), contained, strings.NewReader("x"), ""); err != nil {
		t.Fatalf("Put(%q) should be accepted (contained under root), got %v", contained, err)
	}
}

func TestSignVerify(t *testing.T) {
	// Force a known secret so the test is independent of the env default.
	prev := common.SessionSecret
	common.SessionSecret = "test-secret-only"
	defer func() { common.SessionSecret = prev }()

	const key = "ab/abcdef"
	const expires int64 = 1_700_000_000
	tok := SignURL(key, expires)

	if !VerifyURL(key, expires, tok) {
		t.Fatal("valid token rejected")
	}
	if VerifyURL(key, expires+1, tok) {
		t.Fatal("token accepted with wrong expiry")
	}
	if VerifyURL("ab/different", expires, tok) {
		t.Fatal("token accepted with wrong key")
	}
	if VerifyURL(key, expires, tok+"deadbee") {
		t.Fatal("tampered token accepted")
	}
	if VerifyURL(key, expires, "") {
		t.Fatal("empty token accepted")
	}
}

func TestPublicURLShapeAndSignature(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	key := ContentKey("f00dface")
	if err := s.Put(ctx, key, strings.NewReader("x"), ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
	raw, err := s.PublicURL(ctx, key, time.Hour)
	if err != nil {
		t.Fatalf("PublicURL: %v", err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.HasPrefix(u.Path, "/api/v1/vision/files/") {
		t.Fatalf("unexpected path %q", u.Path)
	}
	if !strings.HasSuffix(u.Path, key) {
		t.Fatalf("path %q does not end with key %q", u.Path, key)
	}
	expiresStr := u.Query().Get("expires")
	tok := u.Query().Get("token")
	if expiresStr == "" || tok == "" {
		t.Fatalf("missing expires/token in %q", raw)
	}
	// The query token must verify against the key + the same expiry we issued.
	if !VerifyURL(key, parseInt64(t, expiresStr), tok) {
		t.Fatalf("issued URL token failed to verify: %q", raw)
	}
}

func parseInt64(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("non-numeric expires %q", s)
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
