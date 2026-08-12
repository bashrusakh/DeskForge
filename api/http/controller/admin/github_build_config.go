package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rustdesk-server/api/global"
	requestadmin "rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

// GithubBuildConfig — HTTP контроллер для настроек GitHub-сборки (PLAN.md §8.8.5).
// Все эндпоинты под /admin/github_build_config/* (admin-only).
type GithubBuildConfig struct{}

func failGithubConfigError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if global.Logger != nil {
		global.Logger.Errorf("GitHub workflow/provider operation failed: %s", service.GithubErrorDetail(err))
	}
	if status, handled := service.GithubErrorHTTPStatus(err); handled {
		response.FailStatus(c, status, 101, service.GithubSafeErrorMessage(err))
		return true
	}
	response.FailStatus(c, http.StatusInternalServerError, 101, service.GithubSafeErrorMessage(err))
	return true
}

func dispatchTestResponse(dispatch *service.GithubDispatchResult) gin.H {
	return gin.H{
		"run_id":   dispatch.WorkflowRunID,
		"run_url":  dispatch.RunURL,
		"html_url": dispatch.HTMLURL,
		"status":   "dispatched",
		"message":  fmt.Sprintf("Smoke-test build dispatched. Check status at %s", dispatch.HTMLURL),
	}
}

// GET /admin/github_build_config/get → возвращает SafeView (без секретов).
// @Tags GithubBuildConfig
// @Summary Get GitHub build configuration
// @Description Admin-only secret-free GitHub build configuration view. Credentials are represented only by has_token and has_payload_key flags.
// @Produce json
// @Success 200 {object} response.Response{data=model.GithubBuildConfigSafe} "Secret-free GitHub build configuration envelope"
// @Failure 500 {object} response.Response "GitHub build configuration is unavailable"
// @Router /admin/github_build_config/get [get]
// @Security token
func (h *GithubBuildConfig) Get(c *gin.Context) {
	cfg, err := service.AllService.GithubBuildConfigService.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	response.Success(c, cfg.Safe())
}

// POST /admin/github_build_config/save
// body: { repo, token?, payload_key? }
// Пустые token / payload_key — не затирают существующие значения.
// @Tags GithubBuildConfig
// @Summary Save GitHub build configuration
// @Description Admin-only configuration update. Repository is user-authored; workflow selectors and resolved provider identities remain system/provider-derived. Empty secrets preserve stored values, and the response is secret-free.
// @Accept json
// @Produce json
// @Param body body requestadmin.GithubBuildConfigForm true "GitHub build configuration"
// @Success 200 {object} response.Response{data=model.GithubBuildConfigSafe} "Secret-free GitHub build configuration envelope"
// @Failure 400 {object} response.Response "Invalid configuration request"
// @Failure 500 {object} response.Response "GitHub build configuration could not be saved"
// @Failure 503 {object} response.Response "Secret encryption is unavailable"
// @Router /admin/github_build_config/save [post]
// @Security token
func (h *GithubBuildConfig) Save(c *gin.Context) {
	var form requestadmin.GithubBuildConfigForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, "params error")
		return
	}
	in := &model.GithubBuildConfig{Repo: form.Repo, Token: form.Token, PayloadKey: form.PayloadKey}
	if err := service.AllService.GithubBuildConfigService.Save(in); err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	cur, err := service.AllService.GithubBuildConfigService.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	response.Success(c, cur.Safe())
}

// POST /admin/github_build_config/approve_workflow_ref
// body: { confirm: true, workflow_tag: "provider-derived-tag-label" }
// The tag label must come from the safe workflow_tags catalog; raw refs and
// SHAs are not accepted as normal input.
//
// @Tags GithubBuildConfig
// @Summary Approve a provider-derived workflow tag
// @Description Admin confirmation for a provider-derived, verified, protected workflow tag. The workflow_tag must be selected from workflow_tags; raw refs and SHAs are not accepted. The response is the secret-free GithubBuildConfigSafe view.
// @Accept json
// @Produce json
// @Param body body requestadmin.WorkflowRefApprovalForm true "Confirm and provider-derived workflow tag label"
// @Success 200 {object} response.Response{data=model.GithubBuildConfigSafe} "Safe GitHub build configuration envelope"
// @Failure 400 {object} response.Response "Invalid selector or approval request"
// @Failure 500 {object} response.Response "Workflow approval failed"
// @Failure 502 {object} response.Response "Provider rejected or could not verify the workflow tag"
// @Failure 503 {object} response.Response "GitHub provider configuration or transport is unavailable"
// @Router /admin/github_build_config/approve_workflow_ref [post]
// @Security token
func (h *GithubBuildConfig) ApproveWorkflowRef(c *gin.Context) {
	var form requestadmin.WorkflowRefApprovalForm
	if err := c.ShouldBindJSON(&form); err != nil {
		response.Fail(c, 101, "params error")
		return
	}
	if !form.Confirm {
		response.Fail(c, 101, "workflow reference approval requires confirm=true")
		return
	}
	cfg, err := service.AllService.GithubBuildConfigService.ApproveWorkflowTag(form.WorkflowTag)
	if err != nil {
		var approvalErr *service.WorkflowRefApprovalError
		if errors.As(err, &approvalErr) {
			response.FailStatus(c, http.StatusBadRequest, 101, "workflow reference is not an approved selector")
			return
		}
		if failGithubConfigError(c, err) {
			return
		}
		response.Fail(c, 101, "workflow reference approval failed")
		return
	}
	response.Success(c, cfg.Safe())
}

// GET /admin/github_build_config/workflow_tags
// Returns only provider-derived safe tag labels that pass provider verification and
// current workflow readiness checks. PATs, payload keys, refs, and SHAs never
// cross this admin DTO boundary.
//
// @Tags GithubBuildConfig
// @Summary List provider-derived workflow tag options
// @Description Admin-only capability catalog. Each WorkflowTagOption is a safe provider-derived label; credentials, raw refs, SHAs, and provider verification objects remain server-side. Approval must select one of these options.
// @Produce json
// @Success 200 {object} response.Response{data=map[string][]service.WorkflowTagOption} "Safe workflow tag options under data.tags"
// @Failure 500 {object} response.Response "Workflow tag options are unavailable"
// @Failure 502 {object} response.Response "Provider rejected or returned invalid workflow data"
// @Failure 503 {object} response.Response "GitHub provider configuration or transport is unavailable"
// @Router /admin/github_build_config/workflow_tags [get]
// @Security token
func (h *GithubBuildConfig) WorkflowTags(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	config, err := svc.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		response.Fail(c, 101, "workflow tag options unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	options, err := svc.ListWorkflowTagOptions(ctx, config)
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		response.Fail(c, 101, "workflow tag options unavailable")
		return
	}
	response.Success(c, gin.H{"tags": options})
}

// POST /admin/github_build_config/generate_key
// Генерит свежий 43-char base64 ключ и СОХРАНЯЕТ его в конфиг.
// Возвращает ключ В ОТКРЫТУЮ — чтобы юзер скопировал и положил в GitHub Secrets форка
// как WORKFLOW_PAYLOAD_KEY. Это единственный момент когда секрет показывается; потом
// /get вернёт только has_payload_key=true.
// @Tags GithubBuildConfig
// @Summary Generate a workflow payload key
// @Description Admin-only operation that creates and stores a new workflow payload key. The generated key is returned once so it can be copied to the provider secret store; later configuration reads are secret-free.
// @Produce json
// @Success 200 {object} response.Response "Generated workflow payload key"
// @Failure 500 {object} response.Response "Payload key could not be generated"
// @Failure 503 {object} response.Response "Secret encryption is unavailable"
// @Router /admin/github_build_config/generate_key [post]
// @Security token
func (h *GithubBuildConfig) GenerateKey(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	key, err := svc.GeneratePayloadKey()
	if err != nil {
		failGithubConfigError(c, err)
		return
	}
	cur, err := svc.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	cur.PayloadKey = key
	if err := service.AllService.GithubBuildConfigService.Save(cur); err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	response.Success(c, gin.H{"payload_key": key})
}

// POST /admin/github_build_config/test
// Проверяет PAT + доступ к репо. Не светит токен в ответе.
// @Tags GithubBuildConfig
// @Summary Test GitHub provider access
// @Description Admin-only read-only provider connectivity check. The stored PAT is used server-side and is never returned.
// @Produce json
// @Success 200 {object} response.Response "Provider access is available"
// @Failure 502 {object} response.Response "Provider rejected the configured credentials or repository"
// @Failure 503 {object} response.Response "GitHub provider configuration or transport is unavailable"
// @Router /admin/github_build_config/test [post]
// @Security token
func (h *GithubBuildConfig) Test(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	if err := svc.TestConnectionError(cur); err != nil {
		failGithubConfigError(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true, "message": "ok"})
}

// POST /admin/github_build_config/sync_secret
// One-click sealed-box sync: кладёт текущий PayloadKey в GitHub Secrets форка как
// WORKFLOW_PAYLOAD_KEY. Удобно после GenerateKey — больше не надо лезть в Settings.
// @Tags GithubBuildConfig
// @Summary Sync the workflow payload key
// @Description Admin-only provider mutation that synchronizes the stored workflow payload key to the configured GitHub Actions secret. The key is not returned.
// @Produce json
// @Success 200 {object} response.Response "Workflow payload key synchronized"
// @Failure 502 {object} response.Response "Provider rejected the secret update"
// @Failure 503 {object} response.Response "GitHub provider configuration or transport is unavailable"
// @Router /admin/github_build_config/sync_secret [post]
// @Security token
func (h *GithubBuildConfig) SyncSecret(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	if err := svc.SetWorkflowSecret(cur); err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		return
	}
	response.Success(c, gin.H{"ok": true, "message": "WORKFLOW_PAYLOAD_KEY synced to GitHub Secrets"})
}

// POST /admin/github_build_config/dispatch_test
// Диспетчит workflow_dispatch и возвращает run_id. Статус — в GitHub Actions
// (длинный poll здесь не держим, чтобы не ловить обрыв прокси).
//
// BUGS.md B-009: раньше слался ПУСТОЙ payload (`map[string]any{}`) → реальная
// сборка с пустыми server/key/app_name падала поздно или давала непригодный
// артефакт, впустую тратя минуты Actions и засоряя историю. Теперь:
//   - запуск требует явного подтверждения (`{"confirm": true}` в теле) — это
//     реальный билд, а не дешёвая проверка (для read-only есть /test);
//   - payload заполняется реальными server/key самого сервера и понятным
//     app_name "deskforge-smoketest", так что smoke-сборка валидна и пригодна.
//
// @Tags GithubBuildConfig
// @Summary Dispatch a GitHub smoke-test build
// @Description Admin-only provider mutation that dispatches the configured Windows workflow with server-derived parameters after explicit confirmation. The response contains provider run metadata, not credentials.
// @Accept json
// @Produce json
// @Param body body map[string]bool false "Explicit build confirmation"
// @Success 200 {object} response.Response "Provider workflow dispatch metadata"
// @Failure 412 {object} response.Response "Workflow reference approval is required"
// @Failure 502 {object} response.Response "Provider rejected the workflow dispatch"
// @Failure 503 {object} response.Response "GitHub provider configuration or transport is unavailable"
// @Router /admin/github_build_config/dispatch_test [post]
// @Security token
func (h *GithubBuildConfig) DispatchTest(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.Fail(c, 101, "params error")
		return
	} // тело опционально; нас интересует только confirm
	if !req.Confirm {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+
			"this triggers a REAL GitHub Actions build and consumes Actions minutes; "+
			"resend with confirm=true to proceed (use /test for a read-only check)")
		return
	}

	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	if err := svc.RequireWorkflowRefApproval(cur); err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		response.FailStatus(c, http.StatusPreconditionFailed, 101, "workflow reference approval is required")
		return
	}
	if err := service.RequireConfiguredPublicKey(); err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		response.FailStatus(c, http.StatusServiceUnavailable, 101, "configured public key is unavailable")
		return
	}

	// Select the newest validated catalog identity. The endpoint has no raw ref
	// input; it follows the same fixed Windows workflow and immutable identity
	// contract as normal builds.
	versionsCtx, versionsCancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	versions, err := svc.GetAvailableVersions(versionsCtx)
	versionsCancel()
	if err != nil || len(versions) == 0 {
		if err == nil {
			err = fmt.Errorf("configured repository has no available build versions")
		}
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	identity := versions[0].VersionIdentity

	// Реальные параметры сервера, чтобы smoke-артефакт был рабочим, а не пустым.
	server := global.Config.Rustdesk.IdServer
	if server == "" {
		server = global.Config.Rustdesk.ApiServer
	}
	params, err := normalizeDispatchTestParams(server, global.Config.Rustdesk.Key, identity.DisplayVersion)
	if err != nil {
		response.Fail(c, 101, "invalid configured build parameters")
		return
	}

	dispatchCtx, dispatchCancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer dispatchCancel()
	dispatch, err := svc.DispatchBuild(dispatchCtx, cur, identity, string(service.PlatformWindows), params)
	if err != nil {
		if failGithubConfigError(c, err) {
			return
		}
		failGithubConfigError(c, err)
		return
	}
	response.Success(c, dispatchTestResponse(dispatch))
}

func normalizeDispatchTestParams(server, key, version string) (map[string]any, error) {
	normalized, err := service.NormalizeCustomBuild(map[string]any{
		"server_ip": server,
		"key":       key,
	}, service.BuildRecordContext{
		Platform: string(service.PlatformWindows),
		AppName:  "deskforge-smoketest",
		Version:  version,
	})
	if err != nil {
		return nil, err
	}
	return normalized.DispatchParams, nil
}
