package crypto

import (
	"crypto/aes"
	"encoding/base64"
	"strings"
	"testing"
)

// testKey32 is a valid 32-byte AES-256 key.
var testKey32 = []byte("0123456789abcdef0123456789abcdef")

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext string
		key       []byte
	}{
		{"simple ASCII", "hello world", testKey32},
		{"empty plaintext", "", testKey32},
		{"long plaintext", strings.Repeat("a", 4096), testKey32},
		{"unicode plaintext", "hello 世界 🚀 日本語", testKey32},
		{"special characters", `{"key":"value","num":42}` + "\n\t\r", testKey32},
		{"numeric string", "1234567890", testKey32},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encrypted, err := EncryptStringAESGCM(tc.plaintext, tc.key)
			if err != nil {
				t.Fatalf("EncryptStringAESGCM failed: %v", err)
			}
			if encrypted == "" {
				t.Fatal("encrypted output should not be empty")
			}
			decrypted, err := DecryptStringAESGCM(encrypted, tc.key)
			if err != nil {
				t.Fatalf("DecryptStringAESGCM failed: %v", err)
			}
			if decrypted != tc.plaintext {
				t.Errorf("round-trip mismatch: got %q, want %q", decrypted, tc.plaintext)
			}
		})
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	plaintext := "secret message"
	correctKey := testKey32
	wrongKey := []byte("abcdefghijklmnopqrstuvwxyz012345")

	encrypted, err := EncryptStringAESGCM(plaintext, correctKey)
	if err != nil {
		t.Fatalf("EncryptStringAESGCM failed: %v", err)
	}

	_, err = DecryptStringAESGCM(encrypted, wrongKey)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key, got nil")
	}
}

func TestDecrypt_CorruptedCiphertextFails(t *testing.T) {
	plaintext := "important data"
	encrypted, err := EncryptStringAESGCM(plaintext, testKey32)
	if err != nil {
		t.Fatalf("EncryptStringAESGCM failed: %v", err)
	}

	// Decode the base64 to raw bytes so we can corrupt it.
	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}

	// Corrupt a byte in the ciphertext body (skip the nonce prefix).
	nonceSize := 12 // gcm.NonceSize() for AES-GCM is always 12
	if len(raw) <= nonceSize {
		t.Fatal("ciphertext too short to corrupt")
	}
	raw[len(raw)-1] ^= 0xff

	corrupted := base64.StdEncoding.EncodeToString(raw)
	_, err = DecryptStringAESGCM(corrupted, testKey32)
	if err == nil {
		t.Fatal("expected error decrypting corrupted ciphertext, got nil")
	}
}

func TestDecrypt_InvalidBase64ReturnsError(t *testing.T) {
	// Not valid base64 (contains characters outside the alphabet and bad padding).
	_, err := DecryptStringAESGCM("!!!not-base64!!!", testKey32)
	if err == nil {
		t.Fatal("expected error for invalid base64 input, got nil")
	}
}

func TestDecrypt_EmptyStringReturnsError(t *testing.T) {
	_, err := DecryptStringAESGCM("", testKey32)
	if err == nil {
		t.Fatal("expected error for empty ciphertext string, got nil")
	}
}

func TestDecrypt_TooShortReturnsError(t *testing.T) {
	// A valid base64 string that decodes to fewer bytes than the nonce size.
	// gcm.NonceSize() for AES-GCM is 12, so 4 bytes is too short.
	short := base64.StdEncoding.EncodeToString([]byte("abcd"))
	_, err := DecryptStringAESGCM(short, testKey32)
	if err == nil {
		t.Fatal("expected error for ciphertext shorter than nonce size, got nil")
	}
}

func TestNormalizeKey32_PadsShortKey(t *testing.T) {
	shortKey := []byte("short")
	out := normalizeKey32(shortKey)
	if len(out) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(out))
	}
	for i, b := range out {
		if i < len(shortKey) {
			if b != shortKey[i] {
				t.Errorf("byte %d: expected %q, got %q", i, shortKey[i], b)
			}
		} else {
			if b != 0 {
				t.Errorf("padding byte %d: expected 0, got %q", i, b)
			}
		}
	}
}

func TestNormalizeKey32_TruncatesLongKey(t *testing.T) {
	longKey := []byte("this is a key that is way longer than 32 bytes for sure")
	out := normalizeKey32(longKey)
	if len(out) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(out))
	}
	for i, b := range out {
		if b != longKey[i] {
			t.Errorf("byte %d: expected %q, got %q", i, longKey[i], b)
		}
	}
}

func TestNormalizeKey32_Exactly32Bytes(t *testing.T) {
	out := normalizeKey32(testKey32)
	if len(out) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(out))
	}
	for i, b := range out {
		if b != testKey32[i] {
			t.Errorf("byte %d: expected %q, got %q", i, testKey32[i], b)
		}
	}
}

func TestNormalizeKey32_EmptyKey(t *testing.T) {
	out := normalizeKey32([]byte{})
	if len(out) != 32 {
		t.Fatalf("expected 32 zero bytes, got %d", len(out))
	}
	for _, b := range out {
		if b != 0 {
			t.Fatal("expected all zero bytes for empty key input")
		}
	}
}

func TestEncrypt_NilKeyProducesValidCipher(t *testing.T) {
	// A nil key should be normalized to 32 zero bytes and still produce valid output.
	encrypted, err := EncryptStringAESGCM("data", nil)
	if err != nil {
		t.Fatalf("EncryptStringAESGCM with nil key failed: %v", err)
	}
	// Decrypting with an all-zero 32-byte key should recover the plaintext.
	zeroKey := make([]byte, 32)
	decrypted, err := DecryptStringAESGCM(encrypted, zeroKey)
	if err != nil {
		t.Fatalf("DecryptStringAESGCM with derived key failed: %v", err)
	}
	if decrypted != "data" {
		t.Errorf("expected %q, got %q", "data", decrypted)
	}
}

func TestEncrypt_DeterministicStructure(t *testing.T) {
	// Each encryption should produce a different ciphertext (random nonce).
	first, _ := EncryptStringAESGCM("same input", testKey32)
	second, _ := EncryptStringAESGCM("same input", testKey32)
	if first == second {
		t.Fatal("encrypting the same plaintext twice should produce different ciphertexts due to random nonce")
	}
}

func TestAesCipher_ValidKey32(t *testing.T) {
	// Sanity check: a 32-byte key must be accepted by aes.NewCipher.
	_, err := aes.NewCipher(testKey32)
	if err != nil {
		t.Fatalf("aes.NewCipher with 32-byte key failed: %v", err)
	}
}

func TestDecrypt_WrongKeyProducesNonEmptyError(t *testing.T) {
	// Confirm decryption with a wrong key returns a non-empty error (not a panic or nil).
	plaintext := "classified"
	encrypted, err := EncryptStringAESGCM(plaintext, testKey32)
	if err != nil {
		t.Fatalf("EncryptStringAESGCM failed: %v", err)
	}
	wrongKey := []byte("X123456789abcdef0123456789abcdef")
	_, err = DecryptStringAESGCM(encrypted, wrongKey)
	if err == nil {
		t.Fatal("expected error with wrong key")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}
