package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const sessionCookie = "gm_session"

const (
	// maxLoginAttempts is the number of failures before an IP is locked out.
	maxLoginAttempts = 5
	// lockoutWindow is how long the lockout lasts.
	lockoutWindow = 5 * time.Minute
)

// sessionManager issues and validates stateless signed session cookies.
type sessionManager struct {
	key      []byte
	ttl      time.Duration
	secure   bool
	mu       sync.Mutex
	attempts map[string]*attempt
}

type attempt struct {
	count int
	until time.Time
}

func newSessionManager(secret string, ttl time.Duration, secure bool) *sessionManager {
	key := []byte(secret)
	if len(key) == 0 {
		b := make([]byte, 32)
		rand.Read(b)
		key = b
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &sessionManager{key: key, ttl: ttl, secure: secure, attempts: map[string]*attempt{}}
}

func (s *sessionManager) sign(payload string) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *sessionManager) issue(w http.ResponseWriter, user string) {
	exp := time.Now().Add(s.ttl).Unix()
	nonce := make([]byte, 12)
	rand.Read(nonce)
	payload := user + "|" + base64.RawURLEncoding.EncodeToString(nonce) + "|" + itoa(exp)
	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + s.sign(payload)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(s.ttl),
		MaxAge:   int(s.ttl.Seconds()),
	})
}

func (s *sessionManager) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *sessionManager) user(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	if !hmac.Equal([]byte(s.sign(string(payload))), []byte(parts[1])) {
		return "", false
	}
	fields := strings.Split(string(payload), "|")
	if len(fields) != 3 {
		return "", false
	}
	exp, err := atoi(fields[2])
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	return fields[0], true
}

// throttle reports whether the client is locked out.
func (s *sessionManager) throttle(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.attempts[ip]
	if !ok {
		return false
	}
	if a.until.IsZero() {
		return false
	}
	if time.Now().Before(a.until) {
		return true
	}
	delete(s.attempts, ip)
	return false
}

// fail records one failed login and locks the client after maxAttempts.
func (s *sessionManager) fail(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.attempts[ip]
	if !ok || (!a.until.IsZero() && time.Now().After(a.until)) {
		a = &attempt{}
		s.attempts[ip] = a
	}
	a.count++
	if a.count >= maxLoginAttempts {
		a.until = time.Now().Add(lockoutWindow)
		a.count = 0
	}
}

func (s *sessionManager) reset(ip string) {
	s.mu.Lock()
	delete(s.attempts, ip)
	s.mu.Unlock()
}

var errBadToken = errors.New("bad token")

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func atoi(s string) (int64, error) {
	var n int64
	if s == "" {
		return 0, errBadToken
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errBadToken
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}
