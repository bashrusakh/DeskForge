package model

import (
	"strings"

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
	Repo string `json:"repo" gorm:"size:128;default:'';not null;"` // configured owner/name
	// WorkflowFilename is retained as an unread legacy column for migration
	// safety. Branch is retained as a storage-compatibility slot; the service
	// normalizes only known legacy owned values to rustqs/min-test. Workflow
	// selectors are branch/tag names; resolved commit SHAs are stored on builds.
	WorkflowFilename    string `json:"-" gorm:"size:128;default:'';not null;"`
	Branch              string `json:"-" gorm:"size:128;default:'';not null;"`
	WorkflowRefApproved bool   `json:"-" gorm:"default:false;not null;"`
	// WorkflowRefProviderVerified records that the last explicit approval had
	// provider-reported verification and protection evidence. It is status
	// metadata only, not a cryptographic signer allowlist; build preparation and
	// dispatch always revalidate GitHub evidence.
	WorkflowRefProviderVerified bool `json:"-" gorm:"default:false;not null;"`
	// WorkflowRefApprovalSHA is the provider-resolved commit recorded with the
	// explicit tag approval. Builds compare it with a fresh provider resolution
	// so a moved tag cannot reuse an old approval.
	WorkflowRefApprovalSHA string `json:"-" gorm:"size:64;default:'';not null;"`
	Token                  string `json:"-" gorm:"type:text;"` // PAT (fine-grained); доступен только provider boundary
	PayloadKey             string `json:"-" gorm:"type:text;"` // AES-passphrase; доступен только provider boundary
	TimeModel
}

// SafeView — версия для возврата в UI без секретов. Поля Token и PayloadKey
// замещены booleanами "has_*", чтобы UI знал, заданы ли они, но не получал значений.
type GithubBuildConfigSafe struct {
	IdModel
	Repo                   string `json:"repo"`
	WorkflowRef            string `json:"workflow_ref"`
	WorkflowRefApproved    bool   `json:"workflow_ref_approved"`
	WorkflowRefStatus      string `json:"workflow_ref_status"`
	WorkflowRefTrustStatus string `json:"workflow_ref_trust_status"`
	HasToken               bool   `json:"has_token"`
	HasPayloadKey          bool   `json:"has_payload_key"`
	TimeModel
}

func (c *GithubBuildConfig) Safe() *GithubBuildConfigSafe {
	return &GithubBuildConfigSafe{
		IdModel:                c.IdModel,
		Repo:                   c.Repo,
		WorkflowRef:            safeWorkflowRef(c.Branch),
		WorkflowRefApproved:    c.workflowRefStatus() == WorkflowRefStatusApproved,
		WorkflowRefStatus:      c.workflowRefStatus(),
		WorkflowRefTrustStatus: c.workflowRefTrust(),
		HasToken:               c.Token != "",
		HasPayloadKey:          c.PayloadKey != "",
		TimeModel:              c.TimeModel,
	}
}

const (
	WorkflowRefStatusApprovalRequired         = "approval-required"
	WorkflowRefStatusProviderPolicyUnverified = "provider-policy-unverified"
	WorkflowRefStatusApproved                 = "approved"
	WorkflowRefTrustProviderReported          = "provider-reported"
	WorkflowRefTrustUnverified                = "unverified"
)

func (c *GithubBuildConfig) workflowRefStatus() string {
	if c == nil || !c.WorkflowRefApproved {
		return WorkflowRefStatusApprovalRequired
	}
	if !c.WorkflowRefProviderVerified || safeWorkflowTag(c.Branch) == "" || !validWorkflowApprovalSHA(c.WorkflowRefApprovalSHA) {
		return WorkflowRefStatusProviderPolicyUnverified
	}
	if _, err := approvedWorkflowRefForSafeStatus(c.Branch); err != nil {
		return WorkflowRefStatusProviderPolicyUnverified
	}
	return WorkflowRefStatusApproved
}

func validWorkflowApprovalSHA(sha string) bool {
	if len(sha) < 40 || len(sha) > 64 {
		return false
	}
	for _, char := range sha {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

// approvedWorkflowRefForSafeStatus mirrors the closed provider selector policy
// without importing the service package into the model package. Safe status is
// never "approved" for a mutable, malformed, or raw-SHA selector.
func approvedWorkflowRefForSafeStatus(ref string) (string, error) {
	const tagPrefix = "refs/tags/"
	if !strings.HasPrefix(ref, tagPrefix) {
		return "", gorm.ErrInvalidData
	}
	tag := strings.TrimPrefix(ref, tagPrefix)
	if safeWorkflowTag(ref) == "" || validWorkflowApprovalSHA(tag) {
		return "", gorm.ErrInvalidData
	}
	return ref, nil
}

func (c *GithubBuildConfig) workflowRefTrust() string {
	if c != nil && c.WorkflowRefProviderVerified {
		return WorkflowRefTrustProviderReported
	}
	return WorkflowRefTrustUnverified
}

// safeWorkflowRef exposes only the selected tag label. Mutable compatibility
// values and raw refs are deliberately never serialized to an API client.
func safeWorkflowRef(ref string) string {
	ref = strings.TrimSpace(ref)
	return safeWorkflowTag(ref)
}

func safeWorkflowTag(ref string) string {
	const tagPrefix = "refs/tags/"
	if !strings.HasPrefix(ref, tagPrefix) {
		return ""
	}
	tag := strings.TrimPrefix(ref, tagPrefix)
	if tag == "" || len(ref) > 128 || strings.HasSuffix(tag, "/") || strings.Contains(tag, "..") || strings.Contains(tag, "//") {
		return ""
	}
	for _, char := range tag {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && !strings.ContainsRune("._/-", char) {
			return ""
		}
	}
	return tag
}

// --- BUGS.md B-008: прозрачное шифрование секретов at rest ---------------------
// Token (PAT) и PayloadKey шифруются перед записью и расшифровываются при чтении,
// так что вызывающий код работает с открытыми значениями, а в БД лежит шифртекст.

func (c *GithubBuildConfig) encryptSecrets() error {
	token, err := utils.EncryptSecret(c.Token)
	if err != nil {
		return err
	}
	payloadKey, err := utils.EncryptSecret(c.PayloadKey)
	if err != nil {
		return err
	}
	// Assign only after both values have been encrypted successfully. A failed
	// hook must not leave the caller's in-memory model half-mutated.
	c.Token = token
	c.PayloadKey = payloadKey
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
