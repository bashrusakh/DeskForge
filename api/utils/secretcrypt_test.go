package utils

import (
	"strings"
	"sync"
	"testing"
)

func resetSecretState() {
	secretKeyOnce = sync.Once{}
	secretKey = nil
}

func TestSecretRoundTripWithKey(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "unit-test-key-123")
	resetSecretState()

	plain := "ghp_supersecretPAT_value"
	enc, err := EncryptSecret(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, secretEncPrefix) {
		t.Fatalf("expected prefix %q, got %q", secretEncPrefix, enc)
	}
	if strings.Contains(enc, plain) {
		t.Fatalf("ciphertext leaks plaintext")
	}
	// idempotent: re-encrypting an encrypted value is a no-op
	enc2, _ := EncryptSecret(enc)
	if enc2 != enc {
		t.Fatalf("EncryptSecret not idempotent")
	}
	dec, err := DecryptSecret(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", dec, plain)
	}
}

func TestEmptyAndLegacyPassthrough(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "k")
	resetSecretState()
	if v, _ := EncryptSecret(""); v != "" {
		t.Fatalf("empty should stay empty")
	}
	// legacy plaintext (no prefix) decrypts to itself
	if v, _ := DecryptSecret("legacy-plain"); v != "legacy-plain" {
		t.Fatalf("legacy passthrough failed: %q", v)
	}
}

func TestDisabledWhenKeyUnset(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "")
	resetSecretState()
	enc, err := EncryptSecret("secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc != "secret" {
		t.Fatalf("with no key, should passthrough, got %q", enc)
	}
}
