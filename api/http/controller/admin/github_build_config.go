package admin

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"

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

// POST /admin/github_build_config/dispatch_test
// Сухой тест: триггерит workflow_dispatch с пустым enc_payload (т.е. workflow пойдёт по
// debug-режиму). Полезно для проверки что воркфлоу видится и токен имеет нужные права.
// Поллит статус до завершения (или таймаута 90 мин), возвращает финальный результат.
func (h *GithubBuildConfig) DispatchTest(c *gin.Context) {
	svc := service.AllService.GithubBuildConfigService
	cur, err := svc.Get()
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	dispatchCtx, dispatchCancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer dispatchCancel()
	// пустой payload → расшифровка не пойдёт, debug pass-through сработает
	runId, err := svc.DispatchBuild(dispatchCtx, cur, map[string]any{})
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	// Поллим до завершения (макс 90 мин). Каждые 30 сек запрашиваем статус.
	// Используем НЕзависимый от запроса context — сам запрос клиента уйдёт раньше.
	pollCtx, pollCancel := context.WithTimeout(context.Background(), 90*time.Minute)
	defer pollCancel()
	var finalStatus, finalConclusion string
	for {
		select {
		case <-pollCtx.Done():
			response.Success(c, gin.H{
				"run_id":   runId,
				"status":   "timeout",
				"conclusion": "polling timeout (90 min)",
				"message":  "Build still running after 90 min. Check GitHub Actions for status.",
			})
			return
		case <-time.After(30 * time.Second):
		}
		statusCtx, statusCancel := context.WithTimeout(context.Background(), 20*time.Second)
		st, concl, err := svc.RunStatus(statusCtx, cur, runId)
		statusCancel()
		if err != nil {
			continue
		}
		finalStatus = st
		finalConclusion = concl
		if st == "completed" {
			break
		}
	}
	ok := finalConclusion == "success"
	msg := "Test build completed: " + finalConclusion
	if ok {
		msg = "Test build successful ✅"
	} else {
		msg = "Test build failed: " + finalConclusion
	}
	response.Success(c, gin.H{
		"run_id":     runId,
		"status":     finalStatus,
		"conclusion": finalConclusion,
		"ok":         ok,
		"message":    msg,
	})
}
