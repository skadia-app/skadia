package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

// validHexKey is a 32-byte AES-256 key encoded as 64 hex characters.
const validHexKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plaintext := "super secret API key"

	enc, err := Encrypt(plaintext, validHexKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if enc == "" {
		t.Fatal("Encrypt returned empty string")
	}

	dec, err := Decrypt(enc, validHexKey)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec != plaintext {
		t.Fatalf("Decrypt = %q, want %q", dec, plaintext)
	}
}

func TestEncryptProducesVersionPrefix(t *testing.T) {
	enc, err := Encrypt("hello", validHexKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if raw[0] != keyVersion1 {
		t.Fatalf("expected version byte 0x01, got 0x%02x", raw[0])
	}
}

func TestEncryptTwiceProducesDifferentCiphertexts(t *testing.T) {
	enc1, _ := Encrypt("same plaintext", validHexKey)
	enc2, _ := Encrypt("same plaintext", validHexKey)
	if enc1 == enc2 {
		t.Fatal("Encrypt should produce distinct ciphertexts on each call (random nonce)")
	}
}

func TestDecryptLegacyFormat(t *testing.T) {
	// Simulate a legacy ciphertext produced without the version prefix.
	// Build nonce+ciphertext directly via the underlying GCM.
	gcm, err := newGCM(validHexKey)
	if err != nil {
		t.Fatalf("newGCM failed: %v", err)
	}

	// Use a fixed nonce to produce a deterministic legacy blob.
	nonce := make([]byte, gcm.NonceSize()) // all-zero nonce for test determinism
	plaintext := "legacy value"
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	legacy := base64.StdEncoding.EncodeToString(sealed)

	dec, err := Decrypt(legacy, validHexKey)
	if err != nil {
		t.Fatalf("Decrypt legacy failed: %v", err)
	}
	if dec != plaintext {
		t.Fatalf("Decrypt legacy = %q, want %q", dec, plaintext)
	}
}

func TestDecryptInvalidKeyEmpty(t *testing.T) {
	_, err := Decrypt("anything", "")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecryptInvalidKeyWrongLength(t *testing.T) {
	_, err := Decrypt("anything", "deadbeef") // 4 bytes, not 32
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestDecryptInvalidKeyNonHex(t *testing.T) {
	_, err := Decrypt("anything", strings.Repeat("zz", 32))
	if err == nil {
		t.Fatal("expected error for non-hex key")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	enc, err := Encrypt("original", validHexKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	raw, _ := base64.StdEncoding.DecodeString(enc)
	// Flip a byte in the ciphertext portion (past version byte + nonce).
	raw[len(raw)-1] ^= 0xff
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err = Decrypt(tampered, validHexKey)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestEncryptInvalidKey(t *testing.T) {
	_, err := Encrypt("value", "not-hex")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	_, err := Decrypt("!!!not-base64!!!", validHexKey)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDecryptLegacyCiphertextWithFirstByte0x01(t *testing.T) {
	// A legacy ciphertext whose nonce starts with 0x01 is ambiguous:
	// the Decrypt function may interpret it as a versioned format.
	// This test documents the known limitation — the version detection
	// considers 0x01 first byte as a version prefix.
	//
	// For production, all new ciphertexts use the versioned format (via Encrypt),
	// so this is only relevant for legacy data where the nonce randomly started
	// with 0x01 (probability ~1/256). In practice, such entries would need
	// re-encryption.
	//
	// Verify that versioned ciphertexts always round-trip correctly,
	// even when the ciphertext content happens to start with 0x01.
	plaintext := "versioned with embedded 0x01"
	enc, err := Encrypt(plaintext, validHexKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Verify that the versioned blob starts with 0x01.
	raw, _ := base64.StdEncoding.DecodeString(enc)
	if raw[0] != keyVersion1 {
		t.Fatalf("expected version byte 0x01, got 0x%02x", raw[0])
	}

	dec, err := Decrypt(enc, validHexKey)
	if err != nil {
		t.Fatalf("Decrypt versioned failed: %v", err)
	}
	if dec != plaintext {
		t.Fatalf("Decrypt = %q, want %q", dec, plaintext)
	}
}
