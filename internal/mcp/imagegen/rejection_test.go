package imagegen

import "testing"

func TestImageQueueFullRejection(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		status        int
		want          bool
	}{
		{"gateway message", "image queue is full, please retry later (request id: 20261017082618341048268EJforpaI)", 503, true},
		{"without ID", "image queue is full, please retry later", 503, true},
		{"nested JSON", `{"error":{"code":"image_queue_full","message":"image queue is full, please retry later"}}`, 503, true},
		{"generic 503", "service unavailable", 503, false},
		{"wrong status", "image queue is full, please retry later", 504, false},
		{"incomplete message", "image queue is full, please retry", 503, false},
		{"unrelated suffix", "image queue is full, please retry later; submission accepted", 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (&APIError{StatusCode: tc.status, Message: tc.message}).QueueFull(); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
