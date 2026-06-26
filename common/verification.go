package common

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Email verification codes are stored in-memory (no Redis in this project).

type verificationValue struct {
	code string
	time time.Time
}

const (
	// EmailVerificationPurpose is the namespace key prefix for registration codes.
	EmailVerificationPurpose = "v"
	// PasswordResetPurpose is reserved for future password-reset flows.
	PasswordResetPurpose = "r"
	// EmailBindPurpose is the namespace key prefix for codes used when a user
	// binds/changes their own email.
	EmailBindPurpose = "b"
)

var verificationMutex sync.Mutex
var verificationMap map[string]verificationValue
var verificationMapMaxSize = 10

// VerificationValidMinutes is how long a verification code stays valid.
var VerificationValidMinutes = 10

// GenerateVerificationCode returns a random alphanumeric code of the given
// length (derived from a UUID with dashes stripped). length == 0 returns the
// full UUID.
func GenerateVerificationCode(length int) string {
	code := uuid.New().String()
	code = strings.Replace(code, "-", "", -1)
	if length == 0 {
		return code
	}
	return code[:length]
}

// RegisterVerificationCodeWithKey stores a code under purpose+key with the
// current timestamp. When the map grows past verificationMapMaxSize, expired
// entries are pruned.
func RegisterVerificationCodeWithKey(key string, code string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap[purpose+key] = verificationValue{
		code: code,
		time: time.Now(),
	}
	if len(verificationMap) > verificationMapMaxSize {
		removeExpiredPairs()
	}
}

// VerifyCodeWithKey reports whether the provided code matches the stored one
// for purpose+key and is still within its validity window.
func VerifyCodeWithKey(key string, code string, purpose string) bool {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[purpose+key]
	now := time.Now()
	if !okay || int(now.Sub(value.time).Seconds()) >= VerificationValidMinutes*60 {
		return false
	}
	return code == value.code
}

// DeleteKey removes the stored code for purpose+key (e.g. after a successful
// verification so it can't be reused).
func DeleteKey(key string, purpose string) {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	delete(verificationMap, purpose+key)
}

// no lock inside, so the caller must lock the verificationMap before calling!
func removeExpiredPairs() {
	now := time.Now()
	for key := range verificationMap {
		if int(now.Sub(verificationMap[key].time).Seconds()) >= VerificationValidMinutes*60 {
			delete(verificationMap, key)
		}
	}
}

func init() {
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	verificationMap = make(map[string]verificationValue)
}
