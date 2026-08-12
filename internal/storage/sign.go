package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"

	"github.com/mujkjk/newmcp/common"
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

// SignURL returns the hex HMAC-SHA256 token binding a storage key to an expiry.
// The signed payload is "key|expires" with expires as a decimal unix timestamp,
// so changing either the key or the expiry invalidates the token.
func SignURL(key string, expires int64) string {
	m := hmac.New(sha256.New, signSecret())
	m.Write([]byte(key))
	m.Write([]byte("|"))
	m.Write([]byte(strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyURL checks a token against the recomputed HMAC in constant time. It does
// NOT check expiry — callers must compare expires against time.Now() themselves
// so both checks stay explicit and testable.
func VerifyURL(key string, expires int64, token string) bool {
	want := SignURL(key, expires)
	// Length differs → reject without hashing-comparing, then constant-time.
	if len(token) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
}

// IsSecretConfigured reports whether SESSION_SECRET was explicitly set. Callers
// (router InitStorage) warn at startup when it isn't, since signed URLs would
// otherwise fall back to the public weak default.
func IsSecretConfigured() bool {
	return common.SessionSecret != ""
}

// SignURLFor is like SignURL but binds a purpose tag (e.g. "PUT") into the signed
// payload, so a token issued for one handler cannot be replayed against another
// sharing the same key path. The presigned-PUT direct-upload endpoint lives on
// the same /api/v1/vision/files/*key path as the GET serve endpoint; without
// purpose-binding, a leaked GET token (handed to third-party upstreams) could be
// replayed as a PUT to overwrite the object. The signed payload is
// "purpose|key|expires", disjoint from SignURL's "key|expires".
func SignURLFor(purpose, key string, expires int64) string {
	m := hmac.New(sha256.New, signSecret())
	m.Write([]byte(purpose))
	m.Write([]byte("|"))
	m.Write([]byte(key))
	m.Write([]byte("|"))
	m.Write([]byte(strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyURLFor checks a purpose-bound token in constant time. Like VerifyURL it
// does NOT check expiry — callers compare expires against time.Now() themselves.
func VerifyURLFor(purpose, key string, expires int64, token string) bool {
	want := SignURLFor(purpose, key, expires)
	if len(token) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
}
