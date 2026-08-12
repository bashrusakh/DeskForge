package utils

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSecretRoundTripWithKey(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "unit-test-key-123")

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

func TestReservedPrefixPlaintextIsRejected(t *testing.T) {
	t.Setenv(SecretEncryptionKeyEnv, "reserved-prefix-key")
	reservedPlaintext := secretEncPrefix + "this-is-not-ciphertext"

	got, err := EncryptSecret(reservedPlaintext)
	if err == nil {
		t.Fatal("reserved-prefix plaintext was accepted")
	}
	if got != "" {
		t.Fatalf("reserved-prefix plaintext returned %q", got)
	}
	var ciphertextErr *SecretCiphertextError
	if !errors.As(err, &ciphertextErr) {
		t.Fatalf("error = %T %v, want SecretCiphertextError", err, err)
	}

	got, err = EncryptCustomBuilderJSON(reservedPlaintext)
	if err == nil || got != "" {
		t.Fatalf("custom JSON reserved-prefix input = %q, %v; want fail closed", got, err)
	}
}

func TestReservedPrefixEncryptedValueRemainsIdempotent(t *testing.T) {
	t.Setenv(SecretEncryptionKeyEnv, "reserved-prefix-key")
	plain := `{"permanent_password":"secret"}`
	ciphertext, err := EncryptSecret(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := EncryptSecret(ciphertext)
	if err != nil {
		t.Fatalf("idempotent encrypt: %v", err)
	}
	if got != ciphertext {
		t.Fatalf("idempotent encrypt changed valid ciphertext")
	}
}

func TestEmptyAndLegacyPassthrough(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "k")
	if v, _ := EncryptSecret(""); v != "" {
		t.Fatalf("empty should stay empty")
	}
	// legacy plaintext (no prefix) decrypts to itself
	if v, _ := DecryptSecret("legacy-plain"); v != "legacy-plain" {
		t.Fatalf("legacy passthrough failed: %q", v)
	}
}

func TestMissingKeyFailsClosedWithoutPlaintext(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "")
	enc, err := EncryptSecret("secret")
	var keyErr *SecretEncryptionKeyError
	if !errors.As(err, &keyErr) {
		t.Fatalf("encrypt error = %T %v, want SecretEncryptionKeyError", err, err)
	}
	if enc != "" {
		t.Fatalf("missing key returned plaintext/ciphertext %q", enc)
	}
	if HasSecretEncryptionKey() {
		t.Fatal("HasSecretEncryptionKey() = true with empty canonical key")
	}
	t.Setenv("SECRET_CRYPT_KEY", "legacy-alias-must-not-work")
	if _, err := EncryptSecret("secret"); !errors.As(err, &keyErr) {
		t.Fatalf("alias unexpectedly enabled encryption: %T %v", err, err)
	}
}

func TestEncryptedReadRequiresKeyButLegacyPlaintextDoesNot(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "read-key")
	ciphertext, err := EncryptSecret("legacy-readable")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", "")
	if got, err := DecryptSecret("legacy-readable"); err != nil || got != "legacy-readable" {
		t.Fatalf("legacy plaintext read = %q, %v", got, err)
	}
	if _, err := DecryptSecret(ciphertext); err == nil {
		t.Fatal("encrypted read without key succeeded")
	}
}

func TestMalformedAndRotatedCiphertextFailClosed(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "original-key")
	ciphertext, err := EncryptSecret("rotated-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := DecryptSecret(secretEncPrefix + "not-base64"); err == nil {
		t.Fatal("malformed ciphertext succeeded")
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", "rotated-key")
	if _, err := DecryptSecret(ciphertext); err == nil {
		t.Fatal("ciphertext decrypted with rotated key")
	}
}

func TestCustomBuilderJSONSecretBoundary(t *testing.T) {
	t.Setenv("SECRET_ENCRYPTION_KEY", "")
	for _, value := range []string{"", `{}`, `{"enable_audio":true}`} {
		if err := RequireSecretEncryptionForCustomBuilderJSON(value); err != nil {
			t.Errorf("non-secret JSON %q rejected without key: %v", value, err)
		}
	}
	if err := RequireSecretEncryptionForCustomBuilderJSON(`{"token":"secret"}`); err == nil {
		t.Fatal("other secret-bearing JSON accepted without key")
	}
	if err := RequireSecretEncryptionForCustomBuilderJSON(`{"permanent_password":"secret"}`); err == nil {
		t.Fatal("secret JSON accepted without key")
	}
	t.Setenv("SECRET_ENCRYPTION_KEY", "custom-json-key")
	ciphertext, err := EncryptCustomBuilderJSON(`{"permanent_password":"secret"}`)
	if err != nil || !IsEncryptedSecret(ciphertext) {
		t.Fatalf("secret JSON encryption = %q, %v", ciphertext, err)
	}
}

func TestRedactCustomBuilderJSONCatchesUnknownSecretLikeKeys(t *testing.T) {
	legacy := `{"endpoint":"https://api.example","server":"id.example:21116","public_key":"public-key","app_name":"RustDesk","enable_audio":true,"API-KEY":"api-secret","accessKey":"access-secret","auth":"auth-secret","authorization":"authorization-secret","AUTH-TOKEN":"token-secret","PAT":"pat-secret","password":"password-secret","legacyCredential":"credential-secret","private-value":"private-secret","misc":"unknown-legacy-value","notes":"unknown-legacy-value"}`
	redacted := RedactCustomBuilderJSON(legacy)
	for _, secret := range []string{"api-secret", "access-secret", "auth-secret", "authorization-secret", "token-secret", "pat-secret", "password-secret", "credential-secret", "private-secret", "unknown-legacy-value"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted JSON leaked %q: %s", secret, redacted)
		}
	}
	for _, public := range []string{"https://api.example", "id.example:21116", "public-key", "RustDesk", "enable_audio"} {
		if !strings.Contains(redacted, public) {
			t.Fatalf("redacted JSON removed public value %q: %s", public, redacted)
		}
	}
}

func TestRedactCustomBuilderJSONDropsTypeSmuggledPublicFields(t *testing.T) {
	legacy := `{"endpoint":"https://api.example","server":["id.example:21116"],"public_key":{"value":"public-key"},"app_name":["RustDesk"],"enable_audio":"true","enable_keyboard":false,"theme":{"name":"dark"},"server_url":"https://server.example"}`
	redacted := RedactCustomBuilderJSON(legacy)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(redacted), &decoded); err != nil {
		t.Fatalf("redacted JSON is invalid: %v", err)
	}
	for _, key := range []string{"server", "public_key", "app_name", "enable_audio", "theme"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("type-smuggled public field %q survived redaction: %s", key, redacted)
		}
	}
	if decoded["endpoint"] != "https://api.example" || decoded["server_url"] != "https://server.example" || decoded["enable_keyboard"] != false {
		t.Fatalf("valid canonical public fields were not preserved: %#v", decoded)
	}
}
