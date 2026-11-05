package totp

import (
	"testing"
	"time"
)

// RFC 6238 test vectors (SHA1, 8 digits truncated to 6 by taking the tail).
func TestCodeAtKnownVectors(t *testing.T) {
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // "12345678901234567890"
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
	}
	for _, c := range cases {
		got, err := CodeAt(secret, time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("CodeAt(%d): %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("CodeAt(%d) = %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"jbswy3dpehpk3pxp":                                  "JBSWY3DPEHPK3PXP",
		" JBSW Y3DP EHPK 3PXP ":                             "JBSWY3DPEHPK3PXP",
		"JBSWY3DPEHPK3PXP====":                              "JBSWY3DPEHPK3PXP",
		"otpauth://totp/a?secret=JBSWY3DPEHPK3PXP&issuer=x": "JBSWY3DPEHPK3PXP",
		"": "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNowAndValidate(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	code, remaining, err := Now(secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code length %d", len(code))
	}
	if remaining < 1 || remaining > Period {
		t.Fatalf("remaining out of range: %d", remaining)
	}
	if !Validate(secret, code) {
		t.Fatal("current code should validate")
	}
	if Validate(secret, "000000") && code != "000000" {
		t.Fatal("wrong code must not validate")
	}
	if Validate(secret, "12345") {
		t.Fatal("short code must not validate")
	}
}

func TestInvalidSecret(t *testing.T) {
	if _, _, err := Now("not-base32!!!"); err == nil {
		t.Fatal("expected error for invalid base32")
	}
	if _, _, err := Now(""); err == nil {
		t.Fatal("expected error for empty secret")
	}
}
