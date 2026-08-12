package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"
)

const (
	DefaultIdServerPort    = 21116
	DefaultRelayServerPort = 21117
	rustDeskPublicKeyBytes = 32
)

type Rustdesk struct {
	IdServer        string `mapstructure:"id-server"`
	IdServerPort    int    `mapstructure:"-"`
	RelayServer     string `mapstructure:"relay-server"`
	RelayServerPort int    `mapstructure:"-"`
	ApiServer       string `mapstructure:"api-server"`
	Key             string `mapstructure:"key"`
	KeyFile         string `mapstructure:"key-file"`
	Personal        int    `mapstructure:"personal"`
	//webclient-magic-queryonline
	WebclientMagicQueryonline int    `mapstructure:"webclient-magic-queryonline"`
	WsHost                    string `mapstructure:"ws-host"`
	keyLoadErr                error
}

// PublicKeyConfigurationError reports a key-file/configuration problem without
// exposing key material. A missing key is allowed during process startup, but
// callers entering a custom-build capability must handle this error.
type PublicKeyConfigurationError struct {
	Path   string
	Reason string
	Cause  error
}

func (e *PublicKeyConfigurationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("public key %q is unavailable: %s", e.Path, e.Reason)
	}
	return "public key is unavailable: " + e.Reason
}

func (e *PublicKeyConfigurationError) Unwrap() error { return e.Cause }

// NormalizePublicKey removes only the line terminator emitted by the
// RustDesk server's id_ed25519.pub file. Other whitespace remains material and
// is rejected by ValidatePublicKeyMaterial.
func NormalizePublicKey(value string) string {
	return strings.TrimRight(value, "\r\n")
}

// ValidatePublicKeyMaterial validates the public-key format consumed by the
// RustDesk client: canonical padded standard base64 encoding of a 32-byte
// Ed25519 public key. The key bytes are never included in the returned error.
func ValidatePublicKeyMaterial(value string) error {
	value = NormalizePublicKey(value)
	if value == "" {
		return &PublicKeyConfigurationError{Reason: "public key is missing"}
	}
	if strings.TrimSpace(value) == "" {
		return &PublicKeyConfigurationError{Reason: "public key is whitespace-only"}
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return &PublicKeyConfigurationError{Reason: "public key contains unsafe control characters"}
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return &PublicKeyConfigurationError{Reason: "public key is not valid RustDesk base64"}
	}
	if len(decoded) != rustDeskPublicKeyBytes || base64.StdEncoding.EncodeToString(decoded) != value {
		return &PublicKeyConfigurationError{Reason: "public key must be canonical base64 for 32 bytes"}
	}
	return nil
}

func (rd *Rustdesk) LoadKeyFile() error {
	rd.keyLoadErr = nil
	// Load key file
	if rd.Key != "" {
		rd.Key = NormalizePublicKey(rd.Key)
		if rd.Key == "" {
			rd.keyLoadErr = &PublicKeyConfigurationError{Reason: "configured value is empty"}
			return rd.keyLoadErr
		}
		return nil
	}
	if rd.KeyFile != "" {
		// Load key from file
		b, err := os.ReadFile(rd.KeyFile)
		if err != nil {
			rd.keyLoadErr = &PublicKeyConfigurationError{Path: rd.KeyFile, Reason: "file cannot be read", Cause: err}
			return rd.keyLoadErr
		}
		// The generated public-key file is line-oriented. Remove only its
		// record terminator; internal content and other whitespace remain part
		// of the value for the BuildSpec boundary to validate.
		rd.Key = NormalizePublicKey(string(b))
		if rd.Key == "" {
			rd.keyLoadErr = &PublicKeyConfigurationError{Path: rd.KeyFile, Reason: "file is empty"}
			return rd.keyLoadErr
		}
		return nil
	}
	return nil
}

// RequirePublicKey is the capability-time check. It intentionally does not
// make startup fail for deployments that configure the key later.
func (rd *Rustdesk) RequirePublicKey() error {
	if rd == nil {
		return &PublicKeyConfigurationError{Reason: "configuration is missing"}
	}
	if rd.keyLoadErr != nil {
		return rd.keyLoadErr
	}
	rd.Key = NormalizePublicKey(rd.Key)
	if rd.Key == "" {
		if rd.KeyFile != "" {
			return &PublicKeyConfigurationError{Path: rd.KeyFile, Reason: "key file did not provide a key"}
		}
		return &PublicKeyConfigurationError{Reason: "no key or key-file is configured"}
	}
	if err := ValidatePublicKeyMaterial(rd.Key); err != nil {
		if rd.KeyFile != "" {
			var keyErr *PublicKeyConfigurationError
			if errors.As(err, &keyErr) {
				keyErr.Path = rd.KeyFile
			}
		}
		return err
	}
	return nil
}

// KeyLoadError exposes the non-secret load result for diagnostics/tests while
// keeping key bytes out of error values.
func (rd *Rustdesk) KeyLoadError() error {
	if rd == nil {
		return errors.New("rustdesk configuration is missing")
	}
	return rd.keyLoadErr
}
