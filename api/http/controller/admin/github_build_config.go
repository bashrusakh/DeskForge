package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"rustdesk-server/api/global"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

// GithubBuildConfig — HTTP контроллер для настроек GitHub-сборки (PLAN.md §8.8.5).
// Все эндпоинты под /admin/github_build_config/* (admin-only).
type GithubBuildConfig struct{}

// GET /admin/github_build_config/get → возвращает SafeView (без секретов).
func (h *GithubBuildConfig) Get(c *gin.Context) {
	cfg, err := service.AllService.GithubBuildConfigService.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, cfg.Safe())
}

// POST /admin/github_build_config/save
// body: { repo, workflow_filename, branch, token?, payload_key? }
// Пустые token / payload_key — не затирают существующие значения.
func (h *GithubBuildConfig) Save(c *gin.Context) {
	var in model.GithubBuildConfig
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Fail(c, 101, "params error: "+err.Error())
		return
	}
	if err := service.AllService.GithubBuildConfigService.Save(&in); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	cur, _ := service.AllService.GithubBuildConfigService.Get()
	response.Success(c, cur.Safe())
}

// POST /admin/github_build_config/generate_key
// Генерит свежий 43-char base64 ключ и СОХРАНЯЕТ его в конфиг.
// Возвращает ключ В ОТКРЫТУЮ — чтобы юзер скопировал и положил в GitHub Secrets форка
// как WORKFLOW_PAYLOAD_KEY. Это единственный момент когда секрет показывается; потом
// /get вернёт только has_payload_key=true.
func (h *GithubBuildConfig) GenerateKey(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	key, err := svc.GeneratePayloadKey()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	cur, err := svc.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	cur.PayloadKey = key
	if err := service.AllService.GithubBuildConfigService.Save(cur); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"payload_key": key})
}

// POST /admin/github_build_config/test
// Проверяет PAT + доступ к репо. Не светит токен в ответе.
func (h *GithubBuildConfig) Test(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	ok, msg := svc.TestConnection(cur)
	if ok {
		response.Success(c, gin.H{"ok": true, "message": msg})
	} else {
		response.Success(c, gin.H{"ok": false, "message": msg})
	}
}

// POST /admin/github_build_config/sync_secret
// One-click sealed-box sync: кладёт текущий PayloadKey в GitHub Secrets форка как
// WORKFLOW_PAYLOAD_KEY. Удобно после GenerateKey — больше не надо лезть в Settings.
func (h *GithubBuildConfig) SyncSecret(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if err := svc.SetWorkflowSecret(cur); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true, "message": "WORKFLOW_PAYLOAD_KEY synced to GitHub Secrets"})
}

// POST /admin/github_build_config/sync_pat
// One-click sealed-box sync: кладёт текущий PAT в GitHub Secrets DeskForge как GH_PAT.
// Нужен для sync-workflows.yml (доступ к форку из CI DeskForge).
func (h *GithubBuildConfig) SyncPat(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	if err := svc.SetSyncPatSecret(cur); err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true, "message": "GH_PAT synced to DeskForge GitHub Secrets"})
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
func (h *GithubBuildConfig) DispatchTest(c *gin.Context) {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&req) // тело опционально; нас интересует только confirm
	if !req.Confirm {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+
			"this triggers a REAL GitHub Actions build and consumes Actions minutes; "+
			"resend with confirm=true to proceed (use /test for a read-only check)")
		return
	}

	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}

	// Реальные параметры сервера, чтобы smoke-артефакт был рабочим, а не пустым.
	server := global.Config.Rustdesk.IdServer
	if server == "" {
		server = global.Config.Rustdesk.ApiServer
	}
	params := map[string]any{
		"app_name":   "deskforge-smoketest",
		"server":     server,
		"key":        global.Config.Rustdesk.Key,
		"custom_txt": "",
	}

	dispatchCtx, dispatchCancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer dispatchCancel()
	runId, err := svc.DispatchBuild(dispatchCtx, cur, params)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, gin.H{
		"run_id":  runId,
		"status":  "dispatched",
		"message": fmt.Sprintf("Smoke-test build dispatched. Check status at https://github.com/%s/actions/runs/%d", cur.Repo, runId),
	})
}
