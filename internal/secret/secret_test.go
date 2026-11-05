package secret

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	box, err := New("test-key-material", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := box.Encrypt("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cipher, "enc:v1:") {
		t.Fatalf("missing prefix: %q", cipher)
	}
	if strings.Contains(cipher, "hunter2") {
		t.Fatal("plaintext leaked")
	}
	plain, err := box.Decrypt(cipher)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "hunter2" {
		t.Fatalf("round trip = %q", plain)
	}
}

func TestEncryptEmpty(t *testing.T) {
	box, _ := New("k", t.TempDir())
	c, err := box.Encrypt("")
	if err != nil || c != "" {
		t.Fatalf("empty should stay empty: %q %v", c, err)
	}
	p, err := box.Decrypt("")
	if err != nil || p != "" {
		t.Fatalf("empty decrypt: %q %v", p, err)
	}
}

func TestDecryptPlaintextPassthrough(t *testing.T) {
	box, _ := New("k", t.TempDir())
	p, err := box.Decrypt("legacy-plain")
	if err != nil || p != "legacy-plain" {
		t.Fatalf("legacy: %q %v", p, err)
	}
}

func TestWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	box1, _ := New("key-one", dir)
	c, _ := box1.Encrypt("secret")

	box2, _ := New("key-two", dir)
	if _, err := box2.Decrypt(c); err == nil {
		t.Fatal("decrypt with a different key must fail")
	}
}

func TestKeyFilePersistence(t *testing.T) {
	dir := t.TempDir()
	box1, err := New("", dir)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := box1.Encrypt("persisted")

	box2, err := New("", dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := box2.Decrypt(c)
	if err != nil || p != "persisted" {
		t.Fatalf("key file not reused: %q %v", p, err)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "secret.key")); err != nil {
		t.Fatal(err)
	}
}
