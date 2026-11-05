package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestThrottleLocksAfterFiveFailures(t *testing.T) {
	s := newSessionManager("k", time.Hour, false)
	const ip = "1.2.3.4"

	for i := 0; i < 4; i++ {
		s.fail(ip)
		if s.throttle(ip) {
			t.Fatalf("locked too early at attempt %d", i+1)
		}
	}
	s.fail(ip) // 5th failure trips the lock
	if !s.throttle(ip) {
		t.Fatal("expected lock after 5 failures")
	}

	s.reset(ip)
	if s.throttle(ip) {
		t.Fatal("reset should clear the lock")
	}
}

func TestThrottleExpires(t *testing.T) {
	s := newSessionManager("k", time.Hour, false)
	const ip = "5.6.7.8"
	for i := 0; i < 5; i++ {
		s.fail(ip)
	}
	if !s.throttle(ip) {
		t.Fatal("expected lock")
	}
	// Force expiry
	s.mu.Lock()
	s.attempts[ip].until = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if s.throttle(ip) {
		t.Fatal("lock should expire")
	}
}

func TestLoginLockReturns429(t *testing.T) {
	e := newTestEnv(t)
	for i := 0; i < 5; i++ {
		e.do(t, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "bad"})
	}
	code, body := e.do(t, "POST", "/api/auth/login", map[string]string{"username": "admin", "password": "secret-pass"})
	if code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after 5 failures, got %d: %s", code, body)
	}
}

func TestSessionCookieRoundTrip(t *testing.T) {
	s := newSessionManager("k", time.Hour, false)
	rec := httptest.NewRecorder()
	s.issue(rec, "admin")

	res := rec.Result()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("cookie not set")
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	user, ok := s.user(req)
	if !ok || user != "admin" {
		t.Fatalf("round trip failed: %q %v", user, ok)
	}

	// Tampered token must be rejected
	bad := *cookie
	bad.Value = cookie.Value[:len(cookie.Value)-2] + "xy"
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(&bad)
	if _, ok := s.user(req2); ok {
		t.Fatal("tampered token accepted")
	}

	// Expired token must be rejected
	s2 := newSessionManager("k", time.Second, false)
	rec2 := httptest.NewRecorder()
	s2.issue(rec2, "admin")
	// exp has one-second resolution, so wait past the next boundary
	time.Sleep(2200 * time.Millisecond)
	for _, c := range rec2.Result().Cookies() {
		if c.Name == sessionCookie {
			req3 := httptest.NewRequest("GET", "/", nil)
			req3.AddCookie(c)
			if _, ok := s.user(req3); ok {
				t.Fatal("expired token accepted")
			}
		}
	}
}
