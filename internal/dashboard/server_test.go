package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlashCookieRoundTrip(t *testing.T) {
	// Step 1: setFlash writes a Set-Cookie header.
	w1 := httptest.NewRecorder()
	setFlash(w1, "hello world")

	resp1 := w1.Result()
	cookies := resp1.Cookies()
	var flashCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "_flash" {
			flashCookie = c
			break
		}
	}
	if flashCookie == nil {
		t.Fatal("expected Set-Cookie _flash header, got none")
	}
	if flashCookie.MaxAge != 60 {
		t.Errorf("expected MaxAge 60, got %d", flashCookie.MaxAge)
	}

	// Step 2: getFlash reads the cookie, returns the message, and clears it.
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.AddCookie(flashCookie)

	got := getFlash(w2, r2)
	if got != "hello world" {
		t.Errorf("getFlash: want %q, got %q", "hello world", got)
	}

	// Verify a clearing Set-Cookie (MaxAge=-1) was emitted.
	resp2 := w2.Result()
	var clearCookie *http.Cookie
	for _, c := range resp2.Cookies() {
		if c.Name == "_flash" {
			clearCookie = c
			break
		}
	}
	if clearCookie == nil {
		t.Fatal("expected clearing Set-Cookie _flash header, got none")
	}
	if clearCookie.MaxAge != -1 {
		t.Errorf("expected clearing MaxAge -1, got %d", clearCookie.MaxAge)
	}
}
