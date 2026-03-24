// Package crypto provides shared AES-256-GCM encryption helpers used across
// the Skadia API service.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

const keyVersion1 = byte(0x01)

// Encrypt encrypts plaintext with AES-256-GCM using the given hex-encoded key.
// The returned string is base64-encoded and contains a 1-byte version prefix
// (0x01) followed by the random nonce and the GCM ciphertext+tag.
func Encrypt(plaintext, hexKey string) (string, error) {
	gcm, err := newGCM(hexKey)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Layout: [version(1)] [nonce(12)] [ciphertext+tag]
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	out := make([]byte, 1+len(sealed))
	out[0] = keyVersion1
	copy(out[1:], sealed)

	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt decrypts a base64-encoded ciphertext produced by Encrypt.
// It handles both the versioned format (0x01 prefix) used by Encrypt and the
// legacy format (no version byte) used by older callers.
func Decrypt(encoded, hexKey string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}

	gcm, err := newGCM(hexKey)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()

	// Detect version byte: if first byte is 0x01 and remaining bytes are long
	// enough to hold nonce + at least one byte of ciphertext, treat as versioned.
	// Otherwise fall back to legacy (nonce starts at byte 0).
	var payload []byte
	if len(data) > 1+nonceSize && data[0] == keyVersion1 {
		payload = data[1:]
	} else {
		payload = data
	}

	if len(payload) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}

	plaintext, err := gcm.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}
	return string(plaintext), nil
}

// newGCM decodes hexKey and returns an AES-256-GCM cipher.AEAD.
func newGCM(hexKey string) (cipher.AEAD, error) {
	if hexKey == "" {
		return nil, fmt.Errorf("crypto: encryption key not configured")
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("crypto: encryption key must be a 32-byte hex string (64 hex chars)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new GCM: %w", err)
	}
	return gcm, nil
}
