package model

import (
	"rustdesk-server/api/utils"

	"gorm.io/gorm"
)

// GithubBuildConfig — настройки GitHub-интеграции для сборки Windows-клиента (§8.8.5).
// Singleton: всегда одна запись с id=1. Используется service/custom_build.go для
// `platform=windows` — вместо локальной job-очереди дёргает workflow_dispatch в форке.
//
// Заполняется через админ-UI ("Build Settings"), не через .env — PAT хранится в БД
// как admin-only настройка инсталляции (по требованию владельца).
//
// PayloadKey — симметричный AES-ключ для шифрования inputs (см. (5) в PLAN §8.8.3b).
// Должен совпадать с GitHub Secret `WORKFLOW_PAYLOAD_KEY` в форке. Автосинхронизация
// доступна через PUT /repos/.../actions/secrets/WORKFLOW_PAYLOAD_KEY (требует scope
// `Secrets: write` у fine-grained PAT).
type GithubBuildConfig struct {
	IdModel
	Repo             string `json:"repo"              gorm:"size:128;default:'';not null;"`      // owner/name, напр. "bashrusakh/rustdesk"
	WorkflowFilename string `json:"workflow_filename" gorm:"size:128;default:'';not null;"`      // напр. "rustqs-windows-min-test.yml"
	Branch           string `json:"branch"            gorm:"size:128;default:'master';not null;"` // ветка, на которой запускать
	Token            string `json:"token,omitempty"   gorm:"type:text;"`                         // PAT (fine-grained); в API-ответах ОПУСКАЕМ
	PayloadKey       string `json:"payload_key,omitempty" gorm:"type:text;"`                     // AES-passphrase; в API-ответах ОПУСКАЕМ
	TimeModel
}

// SafeView — версия для возврата в UI без секретов. Поля Token и PayloadKey
// замещены booleanами "has_*", чтобы UI знал, заданы ли они, но не получал значений.
type GithubBuildConfigSafe struct {
	IdModel
	Repo             string `json:"repo"`
	WorkflowFilename string `json:"workflow_filename"`
	Branch           string `json:"branch"`
	HasToken         bool   `json:"has_token"`
	HasPayloadKey    bool   `json:"has_payload_key"`
	TimeModel
}

func (c *GithubBuildConfig) Safe() *GithubBuildConfigSafe {
	return &GithubBuildConfigSafe{
		IdModel:          c.IdModel,
		Repo:             c.Repo,
		WorkflowFilename: c.WorkflowFilename,
		Branch:           c.Branch,
		HasToken:         c.Token != "",
		HasPayloadKey:    c.PayloadKey != "",
		TimeModel:        c.TimeModel,
	}
}

// --- BUGS.md B-008: прозрачное шифрование секретов at rest ---------------------
// Token (PAT) и PayloadKey шифруются перед записью и расшифровываются при чтении,
// так что вызывающий код работает с открытыми значениями, а в БД лежит шифртекст.

func (c *GithubBuildConfig) encryptSecrets() error {
	var err error
	if c.Token, err = utils.EncryptSecret(c.Token); err != nil {
		return err
	}
	c.PayloadKey, err = utils.EncryptSecret(c.PayloadKey)
	return err
}

func (c *GithubBuildConfig) decryptSecrets() error {
	var err error
	if c.Token, err = utils.DecryptSecret(c.Token); err != nil {
		return err
	}
	c.PayloadKey, err = utils.DecryptSecret(c.PayloadKey)
	return err
}

func (c *GithubBuildConfig) BeforeSave(tx *gorm.DB) error { return c.encryptSecrets() }

// AfterSave возвращает структуру в открытый вид: GORM не расшифровывает поля
// автоматически после записи, а вызывающий код ожидает plaintext.
func (c *GithubBuildConfig) AfterSave(tx *gorm.DB) error { return c.decryptSecrets() }

func (c *GithubBuildConfig) AfterFind(tx *gorm.DB) error { return c.decryptSecrets() }
