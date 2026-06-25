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

// defaultWindowsArtifactName — имя GitHub-артефакта, который продюсит
// windows-min-test workflow. Вынесено из inline-строки (BUGS.md AU-L-011);
// DownloadArtifact дополнительно умеет взять единственный артефакт рана, если
// имя не совпало, так что смена воркфлоу не ломает скачивание.
const defaultWindowsArtifactName = "rustdesk-min-test-windows"

// defaultLinuxWorkflowFilename — имя GitHub-workflow для Linux-сборки (B-012).
// Файл запушен в форк как .github/workflows/rustqs-linux.yml на rustqs/min-test.
// Пока константа; вынести в GithubBuildConfig когда workflow будет green.
const defaultLinuxWorkflowFilename = "rustqs-linux.yml"

// defaultAndroidWorkflowFilename — имя GitHub-workflow для Android-сборки (B-012).
// Файл запушен в форк как .github/workflows/rustqs-android.yml на rustqs/min-test.
// Пока константа; вынести в GithubBuildConfig когда workflow будет green.
const defaultAndroidWorkflowFilename = "rustqs-android.yml"

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
	// BUGS.md B-006: capability-ссылка должна протухать. TTL из конфига,
	// дефолт 7 дней если не задан/невалиден.
	ttl := global.Config.App.DownloadKeyTTL
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	b.DownloadKeyExpiresAt = time.Now().Add(ttl).Unix()

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

// findBuildByDownloadKey — единая точка валидации capability-ключа для всех
// публичных эндпоинтов (DetailByKey/DownloadByKey). Проверяет и существование,
// и срок жизни (BUGS.md B-006), чтобы протухание нельзя было забыть проверить
// в одном из обработчиков. Возвращает (build, httpStatus, ok), где httpStatus =
// 404 для ненайденного ключа, 410 для протухшего, 200 для валидного.
func findBuildByDownloadKey(key string) (*model.CustomBuild, int, bool) {
	var build model.CustomBuild
	if err := global.DB.Where("download_key = ?", key).First(&build).Error; err != nil {
		return nil, 404, false
	}
	if build.DownloadKeyExpiresAt > 0 && time.Now().Unix() > build.DownloadKeyExpiresAt {
		return nil, 410, false
	}
	return &build, 200, true
}

func (ct *CustomBuild) DetailByKey(c *gin.Context) {
	key := c.Param("key")
	build, status, ok := findBuildByDownloadKey(key)
	if !ok {
		if status == 410 {
			c.JSON(410, gin.H{"code": 410, "message": "download link has expired"})
			return
		}
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	response.Success(c, build)
}

// DownloadByKey — public: отдаёт zip с файлами собранного билда из
// /rdgen-data/output/{id}/. Capability URL (тот же download_key что у DetailByKey).
// Если статус не done или файлов нет — 409/404.
//
// Zip собирается стримом прямо в response writer (BUGS.md B-007). Раньше билд
// складывался в `bytes.Buffer` целиком — это OOM-риск, когда артефакт включает
// flutter runtime + dll'ки + portable packer (~20+ МБ × n параллельных скачиваний).
// Content-Length не отдаём — длину знаем только после Close(), стрим уйдёт chunked.
func (ct *CustomBuild) DownloadByKey(c *gin.Context) {
	key := c.Param("key")
	build, status, ok := findBuildByDownloadKey(key)
	if !ok {
		if status == 410 {
			c.JSON(410, gin.H{"code": 410, "message": "download link has expired"})
			return
		}
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

	appName := build.AppName
	if appName == "" {
		appName = "rustqs"
	}

	// Заголовки ДО первого Write — gin/net-http запретит их менять после.
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.zip"`,
		appName, time.Now().Format("20060102-150405")))
	c.Status(200)

	zw := zip.NewWriter(c.Writer)
	defer zw.Close()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		f, openErr := os.Open(full)
		if openErr != nil {
			global.Logger.Warnf("DownloadByKey: skip %s: %v", full, openErr)
			continue
		}
		w, createErr := zw.Create(e.Name())
		if createErr != nil {
			f.Close()
			global.Logger.Warnf("DownloadByKey: zw.Create(%s): %v", e.Name(), createErr)
			continue
		}
		if _, copyErr := io.Copy(w, f); copyErr != nil {
			// клиент отвалился — zw.Close() в defer тоже упадёт, это OK
			f.Close()
			global.Logger.Warnf("DownloadByKey: copy %s: %v", e.Name(), copyErr)
			return
		}
		f.Close()
	}
}

// submitBuild — направляет job в соответствующий backend:
//   - windows/linux/android + настроенный GithubBuildConfig → workflow_dispatch + async polling
//   - иначе → файл-очередь rdgen-data/jobs (для агентов без GitHub)
func (ct *CustomBuild) submitBuild(b *model.CustomBuild) {
	// windows/linux/android (B-012) маршрутизируются в GitHub Actions; остальное — файл-очередь.
	if b.Platform == "windows" || b.Platform == "linux" || b.Platform == "android" {
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
	if err != nil || gcfg == nil || gcfg.Token == "" || gcfg.Repo == "" || gcfg.PayloadKey == "" {
		return false
	}
	// B-012: выбираем workflow по платформе. windows — настраиваемый
	// gcfg.WorkflowFilename; linux — пока константа (см. defaultLinuxWorkflowFilename).
	workflow := gcfg.WorkflowFilename
	switch b.Platform {
	case "linux":
		workflow = defaultLinuxWorkflowFilename
	case "android":
		workflow = defaultAndroidWorkflowFilename
	}
	if workflow == "" {
		return false
	}
	// Копия конфига: подменяем workflow (зависит от платформы) и нормализуем branch.
	// Branch в БД мог остаться "master" от старой установки — форсируем rustqs/min-test,
	// потому что воркфлоу rustqs-* живут только на этой ветке.
	dispatchCfg := *gcfg
	dispatchCfg.WorkflowFilename = workflow
	dispatchCfg.Branch = "rustqs/min-test"

	// Извлекаем параметры из CustomJson (произвольный JSON формы).
	// ВАЖНО: b.Version НЕ передаётся в workflow — фактическая версия клиента
	// определяется кодом на rustqs/min-test ветке форка. Версия в форме —
	// только метка на записи билда (см. github-build/README.md).
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
	runId, err := service.AllService.GithubBuildConfigService.DispatchBuild(ctx, &dispatchCfg, params)
	if err != nil {
		global.Logger.Errorf("github dispatch failed for build %d: %v", b.Id, err)
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog = "github dispatch error: " + err.Error()
		_ = service.AllService.CustomBuildService.Update(b)
		return true // мы попытались — fallback на файл не нужен
	}
	b.Status = model.CustomBuildStatusBuilding
	b.GithubRunId = runId
	b.BuildLog = fmt.Sprintf("github run id: %d", runId)
	_ = service.AllService.CustomBuildService.Update(b)

	// Поллинг в фоне. Используем независимый context (запрос уйдёт раньше, чем сборка).
	go ct.pollAndDownload(b.Id, runId)
	return true
}

// ResumePendingPolls — стартап-хук. Находит билды со статусом `building` и
// сохранённым GithubRunId, перезапускает для них pollAndDownload. Без этого после
// рестарта api все in-flight GitHub сборки зависают навсегда (BUGS.md B-003).
//
// Должен вызываться один раз из cmd/apimain.go ПОСЛЕ AutoMigrate.
func ResumePendingPolls() {
	defer func() {
		if r := recover(); r != nil {
			global.Logger.Errorf("ResumePendingPolls panic: %v", r)
		}
	}()
	ct := &CustomBuild{}
	var builds []*model.CustomBuild
	if err := global.DB.Where("status = ? AND github_run_id > 0", model.CustomBuildStatusBuilding).
		Find(&builds).Error; err != nil {
		global.Logger.Warnf("ResumePendingPolls: query failed: %v", err)
		return
	}
	for _, b := range builds {
		global.Logger.Infof("ResumePendingPolls: resuming build %d (run %d)", b.Id, b.GithubRunId)
		go ct.pollAndDownload(b.Id, b.GithubRunId)
	}
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
		// CustomBuildService.Info never returns nil; Id==0 means row not found.
		if b.Id == 0 {
			return
		}
		if conclusion != "success" {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog += fmt.Sprintf("\nrun %d completed with conclusion=%s", runId, conclusion)
			_ = service.AllService.CustomBuildService.Update(b)
			return
		}
		// скачать артефакт. Имя зависит от платформы (B-012); DownloadArtifact
		// дополнительно фолбэчит на единственный артефакт рана, если имя не совпало.
		artifactName := defaultWindowsArtifactName
		switch b.Platform {
		case "linux":
			artifactName = "rustdesk-min-test-linux"
		case "android":
			artifactName = "rustdesk-min-test-android"
		}
		dlCtx, dlCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		zipBytes, err := service.AllService.GithubBuildConfigService.DownloadArtifact(dlCtx, gcfg, runId, artifactName)
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
		var extracted bool
		if b.Platform == "linux" || b.Platform == "android" {
			// Linux/Android-артефакт (B-012): плоский набор файлов (бинарь/.deb/.apk
			// + custom_.txt). Извлекаем всё; FileSize — размер самого крупного файла.
			for _, zf := range zr.File {
				if zf.FileInfo().IsDir() {
					continue
				}
				name := filepath.Base(zf.Name)
				if name == "" {
					continue
				}
				if n, e := extractZipFile(zf, outDir, name); e == nil {
					extracted = true
					if n > b.FileSize {
						b.FileSize = n
					}
				}
			}
		} else {
			// Windows-артефакт — flat zip с rustqs.exe (или rustdesk.exe) + dll + custom_.txt.
			for _, zf := range zr.File {
				name := filepath.Base(zf.Name)
				if name == appName+".exe" || name == "rustdesk.exe" {
					if n, e := extractZipFile(zf, outDir, appName+".exe"); e == nil {
						b.FileSize = n
						extracted = true
					}
				}
				if name == "custom_.txt" || filepath.Ext(name) == ".dll" {
					_, _ = extractZipFile(zf, outDir, name)
				}
			}
		}
		if !extracted {
			b.Status = model.CustomBuildStatusFailed
			b.BuildLog += "\nno usable files found in artifact"
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
	if b.Id != 0 && b.Status == model.CustomBuildStatusBuilding {
		b.Status = model.CustomBuildStatusFailed
		b.BuildLog += "\npolling timeout (90 min)"
		_ = service.AllService.CustomBuildService.Update(b)
	}
}

// buildCustomTxtFromForm собирает base64-encoded JSON для custom_.txt из полей формы.
// Контракт rdgen-allowCustom-patched клиента: общие настройки + 13 permission-флагов,
// каждый — это строка "Y"/"N" (отсутствие ключа = client default).
//
// История бага (BUGS.md B-004/B-005): предыдущая версия мапила всего 6 полей и содержала
// два ключа, которых форма не шлёт (`allow_remote_config_modification`, `disable_update`).
// Из-за этого все permission-тоглы, branding-URL'ы, theme, direction и т.п. молча
// терялись на windows-via-GitHub пути. Теперь маппинг покрывает весь PRESET_FIELDS из
// admin-ui/src/views/custom-client/index.vue. Если расширяешь форму — добавь ключ сюда.
//
// Возвращает "" если ничего не задано (тогда L2 шаги в воркфлоу пропустятся).
func buildCustomTxtFromForm(raw map[string]any) string {
	cfg := map[string]any{}

	// --- скаляры (string) ---
	stringFields := []struct{ from, to string }{
		{"permanent_password", "password"},
		{"pass_approve_mode", "approve-mode"},
		{"direction", "direction"},
		{"theme", "theme"},
		{"permissions_type", "permissions-mode"},
		{"company_name", "company-name"},
		{"download_url", "download-url"},
		// сетевые координаты — сервер/ключ ушли в L1 (config.rs), а api/relay
		// остаются обычной runtime-настройкой клиента
		{"api_server", "custom-rendezvous-api-server"},
		{"relay_server", "relay-server"},
		// branding URLs — клиент допускает абсолютные или относительные пути
		{"app_icon_url", "app-icon-url"},
		{"app_logo_url", "app-logo-url"},
		{"privacy_screen_url", "privacy-screen-url"},
		// android package id
		{"android_app_id", "android-app-id"},
	}
	for _, m := range stringFields {
		if v, ok := raw[m.from]; ok {
			if s := fmt.Sprint(v); s != "" && s != "<nil>" {
				cfg[m.to] = s
			}
		}
	}

	// --- булевы — все опциональны, "Y" если true; отсутствие или false = опустить ---
	boolFields := []struct{ from, to string }{
		// security
		{"deny_lan", "deny-lan-discovery"},
		{"enable_direct_ip", "direct-server"},
		{"hide_cm", "hide-connection-management"},
		{"auto_close", "auto-close-incoming-sessions"},
		// theme / UX
		{"remove_wallpaper", "remove-wallpaper"},
		{"remove_new_version_notif", "remove-new-version-notif"},
		{"cycle_monitor", "cycle-monitor"},
		{"x_offline", "x-offline"},
		// permissions — клиент читает только при permissions-mode=custom,
		// но передаём всегда (упрощает отладку)
		{"enable_keyboard", "enable-keyboard"},
		{"enable_clipboard", "enable-clipboard"},
		{"enable_file_transfer", "enable-file-transfer"},
		{"enable_audio", "enable-audio"},
		{"enable_tcp", "enable-tunnel"},
		{"enable_remote_restart", "enable-remote-restart"},
		{"enable_recording", "enable-record-session"},
		{"enable_blocking_input", "enable-block-input"},
		{"enable_remote_modi", "allow-remote-config-modification"}, // B-005 fix
		{"enable_printer", "enable-printer"},
		{"enable_camera", "enable-camera"},
		{"enable_terminal", "enable-terminal"},
	}
	for _, m := range boolFields {
		if v, ok := raw[m.from]; ok {
			if b, isBool := v.(bool); isBool && b {
				cfg[m.to] = "Y"
			}
		}
	}

	// hide_cm + постоянный пароль → verification-method=use-permanent-password
	// (нужно явно, иначе клиент ждёт пароль с экрана подтверждения, который скрыт)
	if hide, _ := raw["hide_cm"].(bool); hide {
		if pw, ok := raw["permanent_password"]; ok && fmt.Sprint(pw) != "" {
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

// extractZipFile извлекает один файл из zip в outDir/name, возвращает (записано байт, error).
// Проверяет, что итоговый путь остаётся внутри outDir (защита от Zip Slip).
func extractZipFile(zf *zip.File, outDir, name string) (int64, error) {
	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return 0, err
	}
	dst := filepath.Join(absOut, filepath.Base(name))
	if !strings.HasPrefix(dst+string(os.PathSeparator), absOut+string(os.PathSeparator)) {
		return 0, fmt.Errorf("zip slip: path %q escapes output directory", dst)
	}
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
