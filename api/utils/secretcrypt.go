package utils

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// Шифрование секретов «at rest» (BUGS.md B-008).
//
// GitHub PAT и permanent_password раньше лежали в БД открытым текстом — любой
// с доступом к базе читал их напрямую. Здесь — симметричное AES-256-GCM
// шифрование под ключом из окружения. Ключ НЕ переиспользует WORKFLOW_PAYLOAD_KEY
// (тот кластерно-общий и едет в GitHub Secrets) — берётся отдельный
// SECRET_ENCRYPTION_KEY, известный только деплою.
//
// Совместимость:
//   - Зашифрованные значения помечаются префиксом "enc:v1:". DecryptSecret
//     отдаёт значения без префикса как есть (legacy-плейнтекст), так что старые
//     строки продолжают читаться, а при следующей записи шифруются.
//   - Если SECRET_ENCRYPTION_KEY не задан, новые секреты не записываются:
//     EncryptSecret возвращает typed configuration error вместо plaintext.
//   - EncryptSecret идемпотентна: уже-зашифрованное значение возвращается без
//     изменений (защита от двойного шифрования в GORM-хуках).

const secretEncPrefix = "enc:v1:"
const SecretEncryptionKeyEnv = "SECRET_ENCRYPTION_KEY"

// SecretEncryptionKeyError reports that a new secret cannot be persisted
// because the canonical at-rest encryption key is not configured.
type SecretEncryptionKeyError struct{}

func (e *SecretEncryptionKeyError) Error() string {
	return "secret encryption key is not configured"
}

// SecretCiphertextError reports malformed or unauthentic encrypted data
// without including the ciphertext or plaintext in the error.
type SecretCiphertextError struct {
	Cause string
}

func (e *SecretCiphertextError) Error() string {
	return "secret ciphertext is invalid: " + e.Cause
}

// IsEncryptedSecret reports whether stored uses the versioned ciphertext
// representation. It is intentionally a format check, not a decryption check.
func IsEncryptedSecret(stored string) bool {
	return strings.HasPrefix(stored, secretEncPrefix)
}

func secretKey() ([]byte, error) {
	raw := os.Getenv(SecretEncryptionKeyEnv)
	if raw == "" {
		return nil, &SecretEncryptionKeyError{}
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:], nil
}

// HasSecretEncryptionKey reports whether the canonical at-rest key is set.
// It reads the environment on every call so tests and key changes do not
// depend on a process-global sync.Once cache.
func HasSecretEncryptionKey() bool {
	return os.Getenv(SecretEncryptionKeyEnv) != ""
}

// RequireSecretEncryptionKey fails without revealing any secret value.
func RequireSecretEncryptionKey() error {
	_, err := secretKey()
	return err
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptSecret шифрует значение для хранения в БД. Возвращает строку с
// префиксом "enc:v1:". Пустая строка и уже-зашифрованное значение возвращаются
// без изменений. Для нового непустого plaintext без ключа возвращается ошибка.
func EncryptSecret(plain string) (string, error) {
	if plain == "" {
		return plain, nil
	}
	if strings.HasPrefix(plain, secretEncPrefix) {
		// The prefix is reserved for ciphertext. Validate it with the canonical
		// key before treating the value as an idempotent write; otherwise an
		// attacker could persist arbitrary plaintext that merely starts with the
		// reserved marker.
		if _, err := DecryptSecret(plain); err != nil {
			return "", err
		}
		return plain, nil
	}
	key, err := secretKey()
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretEncPrefix + base64.StdEncoding.EncodeToString(ct), nil
}

// DecryptSecret обращает EncryptSecret. Значения без префикса возвращаются как
// есть (legacy-плейнтекст / шифрование выключено).
func DecryptSecret(stored string) (string, error) {
	if !strings.HasPrefix(stored, secretEncPrefix) {
		return stored, nil
	}
	key, err := secretKey()
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretEncPrefix))
	if err != nil {
		return "", &SecretCiphertextError{Cause: "malformed encoding"}
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", &SecretCiphertextError{Cause: "ciphertext is too short"}
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", &SecretCiphertextError{Cause: "authentication failed"}
	}
	return string(pt), nil
}

// CustomBuilderJSONContainsSecret identifies the user-authored secret in the
// typed custom-builder JSON. Invalid JSON is treated conservatively as secret-
// bearing so a direct model write cannot bypass encryption accidentally.
func CustomBuilderJSONContainsSecret(value string) bool {
	if value == "" || IsEncryptedSecret(value) {
		return false
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return true
	}
	return customBuilderJSONValueContainsSecret(decoded)
}

// CustomBuilderJSONHasPermanentPassword reports only whether the canonical
// top-level permanent_password field contains a non-empty value. It never
// returns the password and treats ciphertext, malformed JSON, and non-object
// values as absent.
func CustomBuilderJSONHasPermanentPassword(value string) bool {
	if value == "" || IsEncryptedSecret(value) {
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &fields); err != nil || fields == nil {
		return false
	}
	var password string
	if err := json.Unmarshal(fields["permanent_password"], &password); err != nil {
		return false
	}
	return strings.TrimSpace(password) != ""
}

func customBuilderJSONValueContainsSecret(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			if isSecretCustomBuilderKey(key) && !isEmptyJSONValue(nested) {
				return true
			}
			if customBuilderJSONValueContainsSecret(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if customBuilderJSONValueContainsSecret(nested) {
				return true
			}
		}
	}
	return false
}

func isEmptyJSONValue(value any) bool {
	stringValue, ok := value.(string)
	return ok && stringValue == ""
}

// RequireSecretEncryptionForCustomBuilderJSON checks the custom-builder
// persistence boundary without changing the caller's plaintext representation.
func RequireSecretEncryptionForCustomBuilderJSON(value string) error {
	if IsEncryptedSecret(value) {
		_, err := EncryptSecret(value)
		return err
	}
	if !CustomBuilderJSONContainsSecret(value) {
		return nil
	}
	return RequireSecretEncryptionKey()
}

// ValidateCustomBuilderJSONFields enforces the canonical persisted custom
// field boundary for direct model writes. Service normalization remains the
// primary typed request path; this second boundary prevents a caller that
// saves a model directly from placing an unknown or neutral field in the
// database as plaintext. Legacy rows remain readable because this validator is
// applied only by the save hook.
func ValidateCustomBuilderJSONFields(value string) error {
	if value == "" || IsEncryptedSecret(value) {
		return nil
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return fmt.Errorf("custom builder JSON is invalid: %w", err)
	}
	if decoded == nil {
		return fmt.Errorf("custom builder JSON must be an object")
	}
	for key, raw := range decoded {
		fieldType, ok := canonicalCustomBuilderFieldTypes[key]
		if !ok {
			return fmt.Errorf("unsupported custom field %q", key)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("custom field %q has invalid %s value", key, fieldType)
		}
		if fieldType == canonicalCustomBuilderBool {
			var target bool
			if err := json.Unmarshal(raw, &target); err != nil {
				return fmt.Errorf("custom field %q has invalid %s value", key, fieldType)
			}
			continue
		}
		var target string
		if err := json.Unmarshal(raw, &target); err != nil {
			return fmt.Errorf("custom field %q has invalid %s value", key, fieldType)
		}
	}
	return nil
}

type canonicalCustomBuilderFieldType string

const (
	canonicalCustomBuilderString canonicalCustomBuilderFieldType = "string"
	canonicalCustomBuilderBool   canonicalCustomBuilderFieldType = "boolean"
)

// canonicalCustomBuilderFieldTypes is the storage boundary for new direct
// model writes. Keep this allowlist exact: response redaction intentionally
// understands compact legacy names, but accepting those names on write would
// let aliases or nested values bypass the typed BuildSpec contract.
var canonicalCustomBuilderFieldTypes = map[string]canonicalCustomBuilderFieldType{
	"server_ip":                canonicalCustomBuilderString,
	"key":                      canonicalCustomBuilderString,
	"api_server":               canonicalCustomBuilderString,
	"relay_server":             canonicalCustomBuilderString,
	"direction":                canonicalCustomBuilderString,
	"permanent_password":       canonicalCustomBuilderString,
	"pass_approve_mode":        canonicalCustomBuilderString,
	"theme":                    canonicalCustomBuilderString,
	"permissions_type":         canonicalCustomBuilderString,
	"company_name":             canonicalCustomBuilderString,
	"download_url":             canonicalCustomBuilderString,
	"android_app_id":           canonicalCustomBuilderString,
	"app_icon_url":             canonicalCustomBuilderString,
	"app_logo_url":             canonicalCustomBuilderString,
	"privacy_screen_url":       canonicalCustomBuilderString,
	"deny_lan":                 canonicalCustomBuilderBool,
	"enable_direct_ip":         canonicalCustomBuilderBool,
	"auto_close":               canonicalCustomBuilderBool,
	"hide_cm":                  canonicalCustomBuilderBool,
	"remove_wallpaper":         canonicalCustomBuilderBool,
	"remove_new_version_notif": canonicalCustomBuilderBool,
	"cycle_monitor":            canonicalCustomBuilderBool,
	"x_offline":                canonicalCustomBuilderBool,
	"enable_remote_modi":       canonicalCustomBuilderBool,
	"enable_keyboard":          canonicalCustomBuilderBool,
	"enable_clipboard":         canonicalCustomBuilderBool,
	"enable_file_transfer":     canonicalCustomBuilderBool,
	"enable_audio":             canonicalCustomBuilderBool,
	"enable_tcp":               canonicalCustomBuilderBool,
	"enable_remote_restart":    canonicalCustomBuilderBool,
	"enable_recording":         canonicalCustomBuilderBool,
	"enable_blocking_input":    canonicalCustomBuilderBool,
	"enable_printer":           canonicalCustomBuilderBool,
	"enable_camera":            canonicalCustomBuilderBool,
	"enable_terminal":          canonicalCustomBuilderBool,
}

// EncryptCustomBuilderJSON encrypts custom_json only when it contains a
// non-empty secret-bearing field. Empty and non-secret typed records retain
// their existing representation and remain usable without the at-rest key.
func EncryptCustomBuilderJSON(value string) (string, error) {
	if IsEncryptedSecret(value) {
		return EncryptSecret(value)
	}
	if err := ValidateCustomBuilderJSONFields(value); err != nil {
		return "", err
	}
	if err := RequireSecretEncryptionForCustomBuilderJSON(value); err != nil {
		return "", err
	}
	if !CustomBuilderJSONContainsSecret(value) {
		return value, nil
	}
	return EncryptSecret(value)
}

// RedactCustomBuilderJSON returns the canonical custom-builder settings that
// are safe for normal API/UI responses. Secret-bearing fields are removed and
// malformed or ciphertext values are omitted so a response can never become a
// storage or secret transport boundary.
func RedactCustomBuilderJSON(value string) string {
	if value == "" || IsEncryptedSecret(value) {
		return ""
	}
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return ""
	}
	if _, ok := decoded.(map[string]any); !ok {
		return ""
	}
	redacted, ok := redactCustomBuilderJSONValue(decoded)
	if !ok {
		return ""
	}
	encoded, err := json.Marshal(redacted)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func redactCustomBuilderJSONValue(value any) (any, bool) {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			compact := compactCustomBuilderKey(key)
			// Safe/public responses use an allowlist rather than attempting to
			// prove that an arbitrary legacy value is harmless. Legacy storage
			// remains readable internally, but unknown keys are omitted here.
			if !isSafePublicCustomBuilderKey(compact) || isSecretCustomBuilderKey(key) || !isCanonicalPublicCustomBuilderValue(compact, nested) {
				delete(value, key)
				continue
			}
		}
		return value, true
	}
	return nil, false
}

// isCanonicalPublicCustomBuilderValue keeps safe responses aligned with the
// typed BuildSpec contract. A legacy key is public only when its value has the
// scalar type that the normal form accepts; arrays, objects, and JSON type
// smuggling are omitted rather than recursively exposed.
func isCanonicalPublicCustomBuilderValue(key string, value any) bool {
	if isCanonicalPublicCustomBuilderStringKey(key) {
		_, ok := value.(string)
		return ok
	}
	if isCanonicalPublicCustomBuilderBoolKey(key) {
		_, ok := value.(bool)
		return ok
	}
	return false
}

func isCanonicalPublicCustomBuilderStringKey(key string) bool {
	switch key {
	case "server", "serverip", "serverurl", "endpoint", "endpointurl", "idserver", "relayserver", "apiserver", "apiserverurl",
		"key", "publickey", "publickeyfile", "appname", "companyname", "downloadurl", "androidappid", "appiconurl", "applogourl", "privacyscreenurl",
		"direction", "passapprovemode", "theme", "permissionstype", "conntype", "accessmode", "approvemode":
		return true
	default:
		return false
	}
}

func isCanonicalPublicCustomBuilderBoolKey(key string) bool {
	switch key {
	case "denylan", "enabledirectip", "autoclose", "hidecm", "removewallpaper", "removenewversionnotif", "cyclemonitor", "xoffline",
		"enableremotemodi", "enablekeyboard", "enableclipboard", "enablefiletransfer", "enableaudio", "enabletcp", "enableremoterestart",
		"enablerecording", "enableblockinginput", "enableprinter", "enablecamera", "enableterminal", "enablelandiscovery", "allowautodisconnect",
		"allowhidecm", "allowremovewallpaper", "allowremoteconfigmodification", "enabletunnel", "enablerecordsession", "enableblockinput", "enableremoteprinter":
		return true
	default:
		return false
	}
}

func isSecretCustomBuilderKey(key string) bool {
	compact := compactCustomBuilderKey(key)
	if isExplicitPublicCustomBuilderKey(compact) {
		return false
	}
	return compact == "auth" || compact == "authorization" ||
		strings.Contains(compact, "auth") ||
		strings.Contains(compact, "password") ||
		strings.Contains(compact, "secret") ||
		strings.Contains(compact, "token") ||
		strings.Contains(compact, "credential") ||
		strings.Contains(compact, "private") ||
		strings.Contains(compact, "ciphertext") ||
		strings.Contains(compact, "payloadkey") ||
		strings.Contains(compact, "apikey") ||
		strings.Contains(compact, "accesskey") ||
		compact == "pat" || strings.HasSuffix(compact, "pat") ||
		compact == "encpayload"
}

func compactCustomBuilderKey(key string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return unicode.ToLower(char)
		}
		return -1
	}, strings.ToLower(strings.TrimSpace(key)))
}

// isExplicitPublicCustomBuilderKey is deliberately a small allowlist. These
// names are part of the documented public server/key contract; every other
// auth-like or credential-like legacy key is redacted conservatively.
func isExplicitPublicCustomBuilderKey(compact string) bool {
	if strings.Contains(compact, "auth") || strings.Contains(compact, "token") ||
		strings.Contains(compact, "secret") || strings.Contains(compact, "private") ||
		strings.Contains(compact, "password") || strings.Contains(compact, "credential") {
		return false
	}
	switch compact {
	case "key", "publickey", "publickeyfile", "endpoint", "endpointurl", "server", "serverurl", "serverip", "idserver", "relayserver", "apiserver":
		return true
	default:
		return strings.HasSuffix(compact, "publickey")
	}
}

// isSafePublicCustomBuilderKey is the response allowlist for the documented
// BuildSpec fields and the generated native settings that the UI can display.
// It intentionally does not include arbitrary legacy keys: omission is safer
// than exposing a value whose meaning is no longer known.
func isSafePublicCustomBuilderKey(compact string) bool {
	if isSecretCustomBuilderKey(compact) {
		return false
	}
	switch compact {
	case "server", "serverip", "serverurl", "endpoint", "endpointurl", "idserver", "relayserver", "apiserver", "apiserverurl",
		"key", "publickey", "publickeyfile", "appname",
		"companyname", "downloadurl", "androidappid", "appiconurl", "applogourl", "privacyscreenurl",
		"direction", "passapprovemode", "theme", "permissionstype",
		"denylan", "enabledirectip", "autoclose", "hidecm", "removewallpaper", "removenewversionnotif", "cyclemonitor", "xoffline",
		"enableremotemodi", "enablekeyboard", "enableclipboard", "enablefiletransfer", "enableaudio", "enabletcp", "enableremoterestart",
		"enablerecording", "enableblockinginput", "enableprinter", "enablecamera", "enableterminal",
		"defaultsettings", "overridesettings", "conntype", "enablelandiscovery", "allowautodisconnect", "accessmode", "approvemode",
		"directserver", "allowhidecm", "verificationmethod", "allowremovewallpaper", "allowremoteconfigmodification", "enabletunnel",
		"enablerecordsession", "enableblockinput", "enableremoteprinter":
		return true
	default:
		return false
	}
}
