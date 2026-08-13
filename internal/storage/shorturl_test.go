package storage

import (
	"strings"
	"testing"

	"github.com/mujkjk/newmcp/common"
)

// TestSignShort_RoundTripAndMethodBinding covers the security-critical pieces
// of the short-URL MAC: valid tokens verify, tampered/empty tokens reject, the
// GET and PUT signature spaces are disjoint (a GET token cannot be replayed as a
// PUT), and the token is the expected 16-char base64url form (no padding).
func TestSignShort_RoundTripAndMethodBinding(t *testing.T) {
	common.SessionSecret = "short-url-test-secret"
	t.Cleanup(func() { common.SessionSecret = "" })

	sid := "8QkP2mR9xY4a"
	getTok := SignShort("GET", sid)
	putTok := SignShort("PUT", sid)

	// 12 bytes → 16 base64url chars, unpadded and URL-safe.
	if len(getTok) != 16 {
		t.Fatalf("token length=%d want 16 (got %q)", len(getTok), getTok)
	}
	if strings.ContainsAny(getTok, "+/=") {
		t.Fatalf("token not URL-safe/unpadded: %q", getTok)
	}

	if !VerifyShort("GET", sid, getTok) {
		t.Fatalf("VerifyShort rejected a valid GET token")
	}
	if VerifyShort("GET", sid, "deadbeefdeadbeef") {
		t.Fatalf("VerifyShort accepted a tampered token")
	}
	if VerifyShort("GET", sid, "") {
		t.Fatalf("VerifyShort accepted an empty token")
	}

	// Method binding: GET and PUT token spaces must be disjoint.
	if VerifyShort("PUT", sid, getTok) {
		t.Fatalf("GET token verified as PUT — method spaces not disjoint")
	}
	if VerifyShort("GET", sid, putTok) {
		t.Fatalf("PUT token verified as GET — method spaces not disjoint")
	}

	// A different sid yields a different token and does not cross-verify.
	other := SignShort("GET", "Z9xY4a8QkP2m")
	if other == getTok {
		t.Fatalf("different sid produced the same token")
	}
	if VerifyShort("GET", "Z9xY4a8QkP2m", getTok) {
		t.Fatalf("token verified against a different sid")
	}
}

// TestShortIDFromURL covers path extraction and the base62/length validation that
// gates the own-URL reverse-fetch.
func TestShortIDFromURL(t *testing.T) {
	cases := []struct {
		name   string
		url    string
		wantOK bool
		want   string
	}{
		{"plain", "http://h/u/8QkP2mR9xY4a?s=abc", true, "8QkP2mR9xY4a"},
		{"no query", "https://h:3000/u/AbC123xyz09", true, "AbC123xyz09"},
		{"trailing slash", "http://h/u/8QkP2mR9xY4a/", true, "8QkP2mR9xY4a"},
		{"wrong prefix", "http://h/api/v1/vision/files/ab/abcd", false, ""},
		{"empty sid", "http://h/u/", false, ""},
		{"bad charset", "http://h/u/has-dash-123", false, ""},
		{"too long", "http://h/u/" + strings.Repeat("a", 20), false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ShortIDFromURL(c.url)
			if ok != c.wantOK || (ok && got != c.want) {
				t.Fatalf("ShortIDFromURL(%q) = (%q,%v) want (%q,%v)", c.url, got, ok, c.want, c.wantOK)
			}
		})
	}
}
