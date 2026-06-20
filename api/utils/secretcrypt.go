package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
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
//   - Если SECRET_ENCRYPTION_KEY не задан, шифрование выключено (passthrough)
//     с одноразовым предупреждением — чтобы существующие деплои не падали до
//     того, как оператор задаст ключ.
//   - EncryptSecret идемпотентна: уже-зашифрованное значение возвращается без
//     изменений (защита от двойного шифрования в GORM-хуках).

const secretEncPrefix = "enc:v1:"

var (
	secretKeyOnce sync.Once
	secretKey     []byte // 32 байта, либо nil если ключ не задан
	secretKeyWarn sync.Once
)

func loadSecretKey() {
	raw := os.Getenv("SECRET_ENCRYPTION_KEY")
	if raw == "" {
		return
	}
	sum := sha256.Sum256([]byte(raw))
	secretKey = sum[:]
}

func newGCM() (cipher.AEAD, error) {
	block, err := aes.NewCipher(secretKey)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// EncryptSecret шифрует значение для хранения в БД. Возвращает строку с
// префиксом "enc:v1:". Пустая строка и уже-зашифрованное значение возвращаются
// без изменений. Если ключ не задан — возвращает plaintext (шифрование off).
func EncryptSecret(plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, secretEncPrefix) {
		return plain, nil
	}
	secretKeyOnce.Do(loadSecretKey)
	if secretKey == nil {
		secretKeyWarn.Do(func() {
			// stderr — global.Logger недоступен в utils без цикла импортов.
			_, _ = os.Stderr.WriteString(
				"WARNING: SECRET_ENCRYPTION_KEY is not set — secrets (PAT, " +
					"permanent_password) are stored in plaintext (BUGS.md B-008)\n")
		})
		return plain, nil
	}
	gcm, err := newGCM()
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
	secretKeyOnce.Do(loadSecretKey)
	if secretKey == nil {
		return "", errors.New("value is encrypted but SECRET_ENCRYPTION_KEY is not set")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretEncPrefix))
	if err != nil {
		return "", err
	}
	gcm, err := newGCM()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
