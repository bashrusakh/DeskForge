package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKeyFileRemovesOnlyTrailingLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519.pub")
	if err := os.WriteFile(path, []byte("public\nkey\r\n"), 0600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	rd := Rustdesk{KeyFile: path}
	rd.LoadKeyFile()
	if rd.Key != "public\nkey" {
		t.Fatalf("loaded key = %q, want only trailing CR/LF removed", rd.Key)
	}
}

func TestLoadKeyNormalizesConfiguredTrailingLineEndings(t *testing.T) {
	const valid = "5Qbwsde3unUcJBtrx9ZkvUmwFNoExHzpryHuPUdqlWM="
	rd := Rustdesk{Key: valid + "\r\n"}
	rd.LoadKeyFile()
	if rd.Key != valid {
		t.Fatalf("configured key = %q, want trailing CR/LF removed", rd.Key)
	}
	if err := rd.RequirePublicKey(); err != nil {
		t.Fatalf("RequirePublicKey() error = %v, want normalized RustDesk key accepted", err)
	}
}

func TestLoadKeyFileFailsClosedOnlyAtCapabilityTime(t *testing.T) {
	for _, test := range []struct {
		name    string
		setup   func(*Rustdesk)
		wantErr string
	}{
		{name: "missing file", setup: func(rd *Rustdesk) { rd.KeyFile = filepath.Join(t.TempDir(), "missing.pub") }, wantErr: "file cannot be read"},
		{name: "empty file", setup: func(rd *Rustdesk) {
			rd.KeyFile = filepath.Join(t.TempDir(), "empty.pub")
			if err := os.WriteFile(rd.KeyFile, []byte("\r\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}, wantErr: "file is empty"},
		{name: "empty configured value", setup: func(rd *Rustdesk) { rd.Key = "\r\n" }, wantErr: "configured value is empty"},
		{name: "not configured", setup: func(*Rustdesk) {}, wantErr: "no key or key-file is configured"},
	} {
		t.Run(test.name, func(t *testing.T) {
			rd := Rustdesk{}
			test.setup(&rd)
			if err := rd.LoadKeyFile(); err != nil && !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("LoadKeyFile() error = %v, want %q", err, test.wantErr)
			}
			err := rd.RequirePublicKey()
			var keyErr *PublicKeyConfigurationError
			if !errors.As(err, &keyErr) || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("RequirePublicKey() error = %T %v, want PublicKeyConfigurationError containing %q", err, err, test.wantErr)
			}
		})
	}
}

func TestValidatePublicKeyMaterialUsesRustDeskEd25519Base64Format(t *testing.T) {
	const valid = "5Qbwsde3unUcJBtrx9ZkvUmwFNoExHzpryHuPUdqlWM="
	for _, test := range []struct {
		name    string
		value   string
		wantErr string
	}{
		{name: "valid", value: valid},
		{name: "valid trailing line endings", value: valid + "\r\n"},
		{name: "missing", value: "", wantErr: "missing"},
		{name: "whitespace-only", value: " \t\r\n", wantErr: "whitespace-only"},
		{name: "control", value: valid[:10] + "\x00" + valid[10:], wantErr: "control"},
		{name: "malformed base64", value: "not-a-rustdesk-key", wantErr: "base64"},
		{name: "wrong decoded length", value: "cHVibGljLWtleQ==", wantErr: "32 bytes"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePublicKeyMaterial(test.value)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidatePublicKeyMaterial() error = %v, want valid key", err)
				}
				return
			}
			var keyErr *PublicKeyConfigurationError
			if !errors.As(err, &keyErr) || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("ValidatePublicKeyMaterial() error = %T %v, want PublicKeyConfigurationError containing %q", err, err, test.wantErr)
			}
		})
	}
}

func TestRequirePublicKeyFailsClosedForMalformedConfiguredMaterial(t *testing.T) {
	for _, value := range []string{" \t", "public\x00key", "public-key", "cHVibGljLWtleQ=="} {
		t.Run(value, func(t *testing.T) {
			rd := Rustdesk{Key: value}
			if err := rd.LoadKeyFile(); err != nil {
				t.Fatalf("LoadKeyFile() error = %v, want deferred capability validation", err)
			}
			var keyErr *PublicKeyConfigurationError
			if err := rd.RequirePublicKey(); !errors.As(err, &keyErr) {
				t.Fatalf("RequirePublicKey() error = %T %v, want typed public-key error", err, err)
			}
		})
	}
}
