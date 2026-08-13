package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/mujkjk/newmcp/common"
	"github.com/mujkjk/newmcp/model"
)

// signSecret returns the HMAC key for signed URLs. It deliberately mirrors the
// fallback in middleware/auth.go (GenerateToken/ParseToken), so signed URLs and
// JWTs share the same secret lifecycle — rotating SESSION_SECRET invalidates
// both, and a missing secret degrades to the same weak default with a startup
// warning rather than a hard crash.
func signSecret() []byte {
	if common.SessionSecret == "" {
		return []byte("default-secret-change-me")
	}
	return []byte(common.SessionSecret)
}

// IsSecretConfigured reports whether SESSION_SECRET was explicitly set. Callers
// (router InitStorage) warn at startup when it isn't, since signed URLs would
// otherwise fall back to the public weak default.
func IsSecretConfigured() bool {
	return common.SessionSecret != ""
}

// shortTokenBytes is the truncated MAC length (bytes) for short file URLs.
// 12 bytes = 96 bits, comfortably above the RFC 2104 / NIST SP 800-107 floor
// (≥80 / ≥64). Forged ONLINE (one HTTP request per guess) the success probability
// is ~2^-76 after 2^20 attempts; the ~71-bit unguessable short_id is the primary
// defense, and this MAC covers the leaked-id scenario. Encodes to 16 base64url
// chars — so a short URL's path+query is a fixed 32 chars regardless of backend.
const shortTokenBytes = 12

// SignShort returns a method-bound, truncated HMAC for a short file URL. The
// payload is "method|shortID" — there is NO expiry in the URL because the DB row
// (created_at + retention / pending-TTL) is the sole expiry authority.
// Method-binding ("GET" vs "PUT") keeps the signature spaces disjoint, so a GET
// token cannot be replayed as a PUT. Truncated and base64url-encoded WITHOUT
// padding (RawURLEncoding) so the URL needs no %-encoding.
func SignShort(method, shortID string) string {
	m := hmac.New(sha256.New, signSecret())
	m.Write([]byte(method))
	m.Write([]byte("|"))
	m.Write([]byte(shortID))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:shortTokenBytes])
}

// VerifyShort recomputes the method-bound MAC and constant-time compares. It does
// NOT check expiry — the caller enforces that via the DB row's created_at + TTL,
// so both checks stay explicit.
func VerifyShort(method, shortID, token string) bool {
	want := SignShort(method, shortID)
	if len(token) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
}

// ShortURL builds the public short file URL: {ServerAddress}/u/{shortID}?s={mac}.
// ServerAddress is read fresh each call so an admin change takes effect on the
// next upload without a restart. The URL is backend-independent (local and S3
// both resolve /u/<sid> server-side), which is why this is a package function
// rather than a Storage interface method.
func ShortURL(method, shortID string) string {
	base := strings.TrimRight(model.GetOptionString("ServerAddress"), "/")
	return fmt.Sprintf("%s/u/%s?s=%s", base, shortID, SignShort(method, shortID))
}

const shortIDCharset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// isValidShortID validates a short id: base62, length 1..16 (matches the column
// size and the 12-char generated length, with headroom).
func isValidShortID(sid string) bool {
	if len(sid) == 0 || len(sid) > 16 {
		return false
	}
	for _, r := range sid {
		if !strings.ContainsRune(shortIDCharset, r) {
			return false
		}
	}
	return true
}

// OwnsShortURL reports whether rawurl is one of this server's own short file
// URLs — same host as ServerAddress and the /u/ path prefix. Backend-independent
// (the short URL always points at this server regardless of local/S3), so it is a
// package function, not a Storage interface method. Used to gate the own-storage
// reverse-fetch in analyze_image before the row lookup.
func OwnsShortURL(rawurl string) bool {
	sa := model.GetOptionString("ServerAddress")
	if sa == "" {
		return false
	}
	su, err := url.Parse(sa)
	if err != nil {
		return false
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return false
	}
	return u.Host == su.Host && strings.HasPrefix(u.Path, "/u/")
}

// ShortIDFromURL extracts the short id from one of this server's own short file
// URLs ({ServerAddress}/u/<sid>?s=…). analyze_image uses it to resolve the handle
// to a row (model.GetUploadedImageByShortID) and read the bytes back as base64,
// so the upstream never has to reach ServerAddress (works behind localhost).
// ok=false for anything that is not a valid own short-URL path; the caller then
// passes the URL through to the upstream instead.
func ShortIDFromURL(rawurl string) (string, bool) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", false
	}
	sid := strings.Trim(strings.TrimPrefix(u.Path, "/u/"), "/")
	if !isValidShortID(sid) {
		return "", false
	}
	return sid, true
}
