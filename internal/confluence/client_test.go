package confluence

import (
	"net/http"
	"testing"
	"time"
)

func TestShouldRetryStatus(t *testing.T) {
	cases := map[int]bool{
		200: false,
		201: false,
		400: false,
		401: false,
		404: false,
		429: true,
		500: false, // 500 is treated as a permanent server error, not retryable
		502: true,
		503: true,
		504: true,
	}
	for status, want := range cases {
		if got := shouldRetryStatus(status); got != want {
			t.Errorf("shouldRetryStatus(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestRetryAfterParsesDeltaSeconds(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "3")
	if got := retryAfter(resp); got != 3*time.Second {
		t.Errorf("retryAfter = %v, want 3s", got)
	}
}

func TestRetryAfterCapsAtMax(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "9999")
	if got := retryAfter(resp); got != maxRetryAfter {
		t.Errorf("retryAfter = %v, want %v (capped)", got, maxRetryAfter)
	}
}

func TestRetryAfterEmpty(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	if got := retryAfter(resp); got != 0 {
		t.Errorf("retryAfter (empty) = %v, want 0", got)
	}
}

func TestRetryAfterMalformed(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "Wed, 21 Oct 2026 07:28:00 GMT") // HTTP-date form not supported
	if got := retryAfter(resp); got != 0 {
		t.Errorf("retryAfter (HTTP-date) = %v, want 0", got)
	}
}

func TestBackoffDelayGrowsExponentially(t *testing.T) {
	d1 := backoffDelay(1, 0)
	d2 := backoffDelay(2, 0)
	d3 := backoffDelay(3, 0)
	if d1 != baseBackoff {
		t.Errorf("attempt 1 = %v, want %v", d1, baseBackoff)
	}
	if d2 <= d1 || d3 <= d2 {
		t.Errorf("backoff not growing: %v, %v, %v", d1, d2, d3)
	}
}

func TestBackoffDelayCapsAtMax(t *testing.T) {
	if got := backoffDelay(20, 0); got != maxBackoff {
		t.Errorf("attempt 20 = %v, want %v (capped)", got, maxBackoff)
	}
}

func TestBackoffDelayHonoursLargerHint(t *testing.T) {
	hint := 5 * time.Second
	got := backoffDelay(1, hint)
	if got != hint {
		t.Errorf("hint=%v, attempt=1, got %v, want %v", hint, got, hint)
	}
}

func TestBackoffDelayIgnoresSmallerHint(t *testing.T) {
	hint := 1 * time.Millisecond
	got := backoffDelay(1, hint)
	if got != baseBackoff {
		t.Errorf("hint=%v should be ignored when smaller than schedule, got %v", hint, got)
	}
}

func TestNewClientPrecomputesAuthHeader(t *testing.T) {
	c := NewClient("https://x/wiki/", "user@example.com", "tok")
	// "user@example.com:tok" base64 = "dXNlckBleGFtcGxlLmNvbTp0b2s="
	want := "Basic dXNlckBleGFtcGxlLmNvbTp0b2s="
	if c.authHeader != want {
		t.Errorf("authHeader = %q, want %q", c.authHeader, want)
	}
	if c.BaseURL != "https://x/wiki" {
		t.Errorf("BaseURL trailing slash not stripped: %q", c.BaseURL)
	}
}
