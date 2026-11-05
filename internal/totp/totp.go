// Package totp implements RFC 6238 time-based one-time passwords.
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Period is the TOTP step in seconds.
const Period = 30

// Digits is the number of digits produced.
const Digits = 6

// Normalize strips whitespace, padding and otpauth:// prefixes from a secret.
func Normalize(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "otpauth://") {
		if u, err := url.Parse(s); err == nil {
			if v := u.Query().Get("secret"); v != "" {
				s = v
			}
		}
	}
	s = strings.ToUpper(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimRight(s, "=")
	return s
}

func decode(secret string) ([]byte, error) {
	s := Normalize(secret)
	if s == "" {
		return nil, errors.New("empty secret")
	}
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid base32 secret: %w", err)
	}
	return b, nil
}

// CodeAt returns the TOTP code for the given time.
func CodeAt(secret string, t time.Time) (string, error) {
	key, err := decode(secret)
	if err != nil {
		return "", err
	}
	counter := uint64(t.Unix() / Period)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", Digits, code%1_000_000), nil
}

// Now returns the current code and the seconds left in the current step.
func Now(secret string) (code string, remaining int, err error) {
	now := time.Now()
	code, err = CodeAt(secret, now)
	if err != nil {
		return "", 0, err
	}
	remaining = Period - int(now.Unix()%Period)
	if remaining == 0 {
		remaining = Period
	}
	return code, remaining, nil
}

// Validate checks a code against the current step with a +/-1 step window.
func Validate(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return false
	}
	for _, skew := range []int64{-1, 0, 1} {
		want, err := CodeAt(secret, time.Now().Add(time.Duration(skew*Period)*time.Second))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// FormatCounter is a helper used in tests and debugging.
func FormatCounter(t time.Time) string {
	return strconv.FormatInt(t.Unix()/Period, 10)
}
