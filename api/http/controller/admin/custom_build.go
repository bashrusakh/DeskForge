package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
	"rustdesk-server/api/utils"
)

type CustomBuild struct{}

func (ct *CustomBuild) List(c *gin.Context) {
	q := &admin.CustomBuildQuery{}
	if err := c.ShouldBindQuery(q); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.CustomBuildService.List(uint(q.Page), uint(q.PageSize))
	response.Success(c, res)
}

func (ct *CustomBuild) Detail(c *gin.Context) {
	id := c.Param("id")
	iid, _ := strconv.Atoi(id)
	u := service.AllService.CustomBuildService.Info(uint(iid))
	if u.Id > 0 {
		response.Success(c, u)
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

func (ct *CustomBuild) Create(c *gin.Context) {
	f := &admin.CustomBuildForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}

	user := service.AllService.UserService.CurUser(c)
	b := f.ToCustomBuild()
	b.UserId = user.Id
	b.Status = model.CustomBuildStatusPending
	b.DownloadKey = utils.RandomString(32)

	err := service.AllService.CustomBuildService.Create(b)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}

	ct.submitBuild(b)

	response.Success(c, b)
}

func (ct *CustomBuild) Delete(c *gin.Context) {
	f := &admin.CustomBuildForm{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	ex := service.AllService.CustomBuildService.Info(f.Id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.CustomBuildService.Delete(ex)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, nil)
}

func (ct *CustomBuild) DetailByKey(c *gin.Context) {
	key := c.Param("key")
	var builds []*model.CustomBuild
	global.DB.Where("download_key = ?", key).Find(&builds)
	if len(builds) > 0 {
		response.Success(c, builds[0])
		return
	}
	response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
}

// DownloadByKey — public: отдаёт zip с файлами собранного билда из
// /rdgen-data/output/{id}/. Capability URL (тот же download_key что у DetailByKey).
// Если статус не done или файлов нет — 409/404.
func (ct *CustomBuild) DownloadByKey(c *gin.Context) {
	key := c.Param("key")
	var build model.CustomBuild
	if err := global.DB.Where("download_key = ?", key).First(&build).Error; err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	if build.Status != model.CustomBuildStatusDone {
		c.JSON(409, gin.H{
			"code":    409,
			"message": "build is not ready (status=" + build.Status + ")",
		})
		return
	}

	dir := service.BuildOutputDir(build.Id)
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		c.JSON(404, gin.H{
			"code":    404,
			"message": "build artifacts not found on disk",
		})
		return
	}

	// Собираем zip в памяти.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		f, err := os.Open(full)
		if err != nil {
			continue
		}
		w, err := zw.Create(e.Name())
		if err != nil {
			f.Close()
			continue
		}
		_, _ = io.Copy(w, f)
		f.Close()
	}
	_ = zw.Close()

	// Имя файла: app_name или "rustqs".
	appName := build.AppName
	if appName == "" {
		appName = "rustqs"
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`, appName, time.Now().Format("20060102-150405")))
	c.Header("Content-Length", strconv.Itoa(buf.Len()))
	c.Data(200, "application/zip", buf.Bytes())
}

// submitBuild — направляет job в соответствующий backend:
//   - windows + настроенный GithubBuildConfig → workflow_dispatch + async polling (PLAN §8.8.5)
//   - иначе → файл-очередь rdgen-data/jobs (для linux/android агентов)
func (ct *CustomBuild) submitBuild(b *model.CustomBuild) {
	if b.Platform == "windows" {
		if ct.tryGithubDispatch(b) {
			return
		}
		// fallback: продолжаем в файл-очередь (если GitHub не настроен)
	}
	jobsDir := "/rdgen-data/jobs"
	if err := os.MkdirAll(jobsDir, 0755); err != nil {
		global.Logger.Warnf("failed to create rdgen jobs dir %s: %v", jobsDir, err)
		return
	}
	job := map[string]interface{}{
		"id":          b.Id,
		"platform":    b.Platform,
		"version":     b.Version,
		"app_name":    b.AppName,
		"custom_json": b.CustomJson,
		"host":        global.Config.Rustdesk.ApiServer,
		"key":         global.Config.Rustdesk.Key,
		"api_server":  global.Config.Rustdesk.ApiServer,
		"relay_server": global.Config.Rustdesk.RelayServer,
		"api_base":    global.Config.App.ApiBase,
	}
	data, _ := json.Marshal(job)
	jobFile := filepath.Join(jobsDir, fmt.Sprintf("%d.json", b.Id))
	if err := os.WriteFile(jobFile, data, 0644); err != nil {
		global.Logger.Errorf("failed to write build job file: %v", err)
	}
}

// tryGithubDispatch — пытается направить windows-сборку в GitHub Actions.
// Возвращает false если GithubBuildConfig не настроен (тогда вызывающий делает fallback).
func (ct *CustomBuild) tryGithubDispatch(b *model.CustomBuild) bool {
	gcfg, err := service.AllService.GithubBuildConfigService.Get()
	if err != nil || gcfg == nil || gcfg.Token == "" || gcfg.Repo == "" || gcfg.WorkflowFilename == "" || gcfg.PayloadKey == "" {
		return false
	}

	// Извлекаем параметры из CustomJson (произвольный JSON формы).
	params := map[string]any{
		"app_name": b.AppName,
	}
	if b.CustomJson != "" {
		var raw map[string]any
		if json.Unmarshal([]byte(b.CustomJson), &raw) == nil {
			// server: воркфлоу ждёт ключ `server`; форма хранит `server_ip` — поддержим оба.
			if v, ok := raw["server"]; ok {
				params["server"] = v
			} else if v, ok := raw["server_ip"]; ok {
				params["server"] = v
			}
			// Strip port — client appends ports automatically.
			if s, ok := params["server"].(string); ok {
				if i := strings.LastIndex(s, ":"); i > 0 {
					params["server"] = s[:i]
				}
			}
			if v, ok := raw["key"]; ok {
				params["key"] = v
			}
			// custom_txt: если задан напрямую (base64) — используем; иначе собираем из
			// отдельных полей формы (permanent_password, hide_cm, deny_lan, и т.п.) →
			// JSON → base64. Это нужно потому что allowCustom-patched клиент читает
			// custom_.txt как base64(JSON-настройки rdgen).
			if v, ok := raw["custom_txt"]; ok && fmt.Sprint(v) != "" {
				params["custom_txt"] = v
			} else {
				params["custom_txt"] = buildCustomTxtFromForm(raw)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	runId, err := service.AllService.GithubBuildConfigService.DispatchBuild(ctx, gcfg, params)
	if err != nil {
		global.Logger.Errorf("github dispatch failed for build %d: %v", b.Id, err)
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = "github dispatch error: " + err.Error()
		_ = service.AllService.CustomBuildService.Update(b)
		return true // мы попытались — fallback на файл не нужен
	}
	b.Status = model.CustomBuildStatusBuilding
	b.BuildLog = fmt.Sprintf("github run id: %d", runId)
	_ = service.AllService.CustomBuildService.Update(b)

	// Поллинг в фоне. Используем независимый context (запрос уйдёт раньше, чем сборка).
	go ct.pollAndDownload(b.Id, runId)
	return true
}

// pollAndDownload — асинхронно опрашивает статус рана GitHub до завершения,
// при успехе скачивает артефакт rustdesk-min-test-windows.zip, кладёт exe в
// /rdgen-data/output/{buildId}/{appname}.exe, обновляет статус CustomBuild.
func (ct *CustomBuild) pollAndDownload(buildId uint, runId int64) {
	// Паника в фоновой горутине роняет весь процесс — гасим её здесь.
	defer func() {
		if r := recover(); r != nil {
			global.Logger.Errorf("pollAndDownload panic for build %d: %v", buildId, r)
		}
	}()
	gcfg, err := service.AllService.GithubBuildConfigService.Get()
	if err != nil || gcfg == nil {
		return
	}
	deadline := time.Now().Add(90 * time.Minute) // защита от зависших ранов
	for time.Now().Before(deadline) {
		time.Sleep(30 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		status, conclusion, err := service.AllService.GithubBuildConfigService.RunStatus(ctx, gcfg, runId)
		cancel()
		if err != nil {
			continue
		}
		if status != "completed" {
			continue
		}
		// completed → финализируем
		b := service.AllService.CustomBuildService.Info(buildId)
		if b == nil || b.Id == 0 {
			return
		}
		if conclusion != "success" {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog += fmt.Sprintf("\nrun %d completed with conclusion=%s", runId, conclusion)
			_ = service.AllService.CustomBuildService.Update(b)
			return
		}
		// скачать артефакт
		dlCtx, dlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		zipBytes, err := service.AllService.GithubBuildConfigService.DownloadArtifact(dlCtx, gcfg, runId, "rustdesk-min-test-windows")
		dlCancel()
		if err != nil {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog += "\ndownload artifact: " + err.Error()
			_ = service.AllService.CustomBuildService.Update(b)
			return
		}
		// распаковка zip и извлечение exe
		appName := b.AppName
		if appName == "" {
			appName = "rustqs"
		}
		outDir := service.BuildOutputDir(buildId)
		_ = os.MkdirAll(outDir, 0755)
		zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
		if err != nil {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog += "\nunzip: " + err.Error()
			_ = service.AllService.CustomBuildService.Update(b)
			return
		}
		var exeWritten bool
		for _, zf := range zr.File {
			// артефакт — flat zip с rustqs.exe (или rustdesk.exe) + dll + custom_.txt
			name := filepath.Base(zf.Name)
			if name == appName+".exe" || name == "rustdesk.exe" {
				if n, e := extractZipFile(zf, filepath.Join(outDir, appName+".exe")); e == nil {
					b.FileSize = n
					exeWritten = true
				}
			}
			// дополнительно — custom_.txt и DLL рядом
			if name == "custom_.txt" || filepath.Ext(name) == ".dll" {
				_, _ = extractZipFile(zf, filepath.Join(outDir, name))
			}
		}
		if !exeWritten {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog += "\nexe not found in artifact"
			_ = service.AllService.CustomBuildService.Update(b)
			return
		}
		b.Status = model.CustomBuildStatusDone
		b.BuildLog += "\nartifact downloaded and extracted"
		_ = service.AllService.CustomBuildService.Update(b)
		return
	}
	// таймаут
	b := service.AllService.CustomBuildService.Info(buildId)
	if b != nil && b.Id != 0 && b.Status == model.CustomBuildStatusBuilding {
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog += "\npolling timeout (90 min)"
		_ = service.AllService.CustomBuildService.Update(b)
	}
}

// buildCustomTxtFromForm собирает base64-encoded JSON для custom_.txt из полей формы.
// Поля повторяют контракт rdgen-allowCustom-patched клиента: password, verification-method,
// hide-connection-management, deny-lan-discovery, и т.п. Пустые значения опускаются.
// Возвращает "" если ничего не задано (тогда L2 шаги в воркфлоу пропустятся).
func buildCustomTxtFromForm(raw map[string]any) string {
	cfg := map[string]any{}

	// password (постоянный) → password
	if v, ok := raw["permanent_password"]; ok {
		if s := fmt.Sprint(v); s != "" {
			cfg["password"] = s
		}
	}

	// security/permissions — все опциональны, типовая rdgen-схема
	type mapping struct {
		from, to string
	}
	bools := []mapping{
		{"deny_lan", "deny-lan-discovery"},
		{"enable_direct_ip", "direct-server"},
		{"hide_cm", "hide-connection-management"},
		{"remove_wallpaper", "remove-wallpaper"},
		{"allow_remote_config_modification", "allow-remote-config-modification"},
		{"disable_update", "disable-update"},
	}
	for _, m := range bools {
		if v, ok := raw[m.from]; ok {
			if b, isBool := v.(bool); isBool && b {
				cfg[m.to] = "Y"
			}
		}
	}

	// hide_cm определяет verification-method: с постоянным паролем достаточно одного
	if v, ok := raw["hide_cm"]; ok {
		if b, isBool := v.(bool); isBool && b {
			cfg["verification-method"] = "use-permanent-password"
		}
	}

	if len(cfg) == 0 {
		return ""
	}
	j, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(j)
}

// extractZipFile извлекает один файл из zip в dst, возвращает (записано байт, error).
func extractZipFile(zf *zip.File, dst string) (int64, error) {
	rc, err := zf.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	f, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return io.Copy(f, rc)
}
