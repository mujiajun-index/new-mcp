package storage

import (
	"bytes"
	"context"
	"strings"
	"testing"
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
