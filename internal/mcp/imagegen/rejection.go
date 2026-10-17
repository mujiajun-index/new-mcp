package imagegen

import (
	"net/http"
	"regexp"
	"strings"
)

var queueFullMessage = regexp.MustCompile(`(?i)^image queue is full, please retry later(?:\s+\(request id: [a-z0-9_-]+\))?\.?$`)

// QueueFull identifies the explicit gateway rejection. Compatible image APIs
// sometimes expose only this message, without preserving a structured error code.
// Other 503 responses still have an unknown submission outcome.
func (e *APIError) QueueFull() bool {
	return e.StatusCode == http.StatusServiceUnavailable && queueFullMessage.MatchString(strings.TrimSpace(errorMessage([]byte(e.Message))))
}
