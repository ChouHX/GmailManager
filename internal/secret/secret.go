// Package secret provides authenticated encryption for credentials at rest.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const prefix = "enc:v1:"

// Box encrypts and decrypts short strings (passwords, TOTP secrets).
type Box struct {
	aead cipher.AEAD
}

// New builds a Box. When rawKey is empty a key file inside dataDir is used,
// generated on first start.
func New(rawKey, dataDir string) (*Box, error) {
	key, err := loadKey(rawKey, dataDir)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(key)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func loadKey(rawKey, dataDir string) ([]byte, error) {
	if rawKey != "" {
		return []byte(rawKey), nil
	}
	if dataDir == "" {
		dataDir = "."
	}
	path := filepath.Join(dataDir, "secret.key")
	b, err := os.ReadFile(path)
	if err == nil && len(b) >= 16 {
		return b, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt returns a self-describing ciphertext, or "" for empty input.
func (b *Box) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := b.aead.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Decrypt reverses Encrypt. Plaintext values (legacy) are returned unchanged.
func (b *Box) Decrypt(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, prefix) {
		return value, nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("secret too short")
	}
	plain, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", errors.New("cannot decrypt secret (wrong GM_SECRET?)")
	}
	return string(plain), nil
}
