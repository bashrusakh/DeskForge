package admin

import (
	"time"

	"github.com/gin-gonic/gin"
	"rustdesk-server/api/global"
	"rustdesk-server/api/http/request/admin"
	"rustdesk-server/api/http/response"
	"rustdesk-server/api/model"
	"rustdesk-server/api/service"
)

type Rustdesk struct {
}

type RustdeskCmd struct {
	Cmd    string `json:"cmd"`
	Option string `json:"option"`
	Target string `json:"target"`
}

func (r *Rustdesk) CmdList(c *gin.Context) {
	q := &admin.PageQuery{}
	if err := c.ShouldBindQuery(q); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	res := service.AllService.ServerCmdService.List(q.Page, 9999)

	list := make([]*model.ServerCmd, 0)
	list = append(list, model.SysIdServerCmds...)
	list = append(list, model.SysRelayServerCmds...)
	list = append(list, res.ServerCmds...)
	res.ServerCmds = list
	response.Success(c, res)
}

// CmdAuditList — журнал выполненных server-команд (BUGS.md AU-S-001), новейшие
// сверху, постранично. Только для просмотра; пишется middleware.ServerCmdAudit.
func (r *Rustdesk) CmdAuditList(c *gin.Context) {
	q := &admin.PageQuery{}
	if err := c.ShouldBindQuery(q); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	page := q.Page
	if page == 0 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize == 0 {
		pageSize = 20
	}
	res := &model.ServerCmdAuditList{}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)
	global.DB.Model(&model.ServerCmdAudit{}).Count(&res.Total)
	global.DB.Model(&model.ServerCmdAudit{}).
		Order("id desc").
		Offset(int((page - 1) * pageSize)).
		Limit(int(pageSize)).
		Find(&res.ServerCmdAudits)
	response.Success(c, res)
}

func (r *Rustdesk) CmdDelete(c *gin.Context) {
	f := &model.ServerCmd{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if f.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	ex := service.AllService.ServerCmdService.Info(f.Id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}

	err := service.AllService.ServerCmdService.Delete(ex)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, nil)
}
func (r *Rustdesk) CmdCreate(c *gin.Context) {
	f := &model.ServerCmd{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	err := service.AllService.ServerCmdService.Create(f)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, nil)
}

func (r *Rustdesk) CmdUpdate(c *gin.Context) {
	f := &model.ServerCmd{}
	if err := c.ShouldBindJSON(f); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	errList := global.Validator.ValidStruct(c, f)
	if len(errList) > 0 {
		response.Fail(c, 101, errList[0])
		return
	}
	ex := service.AllService.ServerCmdService.Info(f.Id)
	if ex.Id == 0 {
		response.Fail(c, 101, response.TranslateMsg(c, "ItemNotFound"))
		return
	}
	err := service.AllService.ServerCmdService.Update(f)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	response.Success(c, nil)
}

func (r *Rustdesk) SendCmd(c *gin.Context) {
	rc := &RustdeskCmd{}
	if err := c.ShouldBindJSON(rc); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if rc.Cmd == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if rc.Target == "" {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if rc.Target != model.ServerCmdTargetIdServer && rc.Target != model.ServerCmdTargetRelayServer {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}

	port := 0
	switch rc.Target {
	case model.ServerCmdTargetIdServer:
		port = global.Config.Admin.IdServerPort - 1
	case model.ServerCmdTargetRelayServer:
		port = global.Config.Admin.RelayServerPort
	}

	res, err := service.AllService.ServerCmdService.SendCmd(port, rc.Cmd, rc.Option)
	if err != nil {
		response.Fail(c, 101, err.Error())
		return
	}
	// AU-C-001: запоминаем применённую set-команду, чтобы восстановить её после
	// рестарта (PersistCmd сам игнорирует read-команды без option).
	if perr := service.AllService.ServerCmdService.PersistCmd(rc.Target, rc.Cmd, rc.Option); perr != nil {
		global.Logger.Warnf("PersistCmd(%s %s): %v", rc.Target, rc.Cmd, perr)
	}
	response.Success(c, res)
}

// ReplayServerCmds — стартап-хук (BUGS.md AU-C-001). Переприменяет сохранённые
// server-команды (RELAY_SERVERS / ALWAYS_USE_RELAY / MUST_LOGIN / blocklist и т.п.),
// которые иначе откатываются к env/файлам при рестарте контейнера. Best-effort:
// если hbbs/hbbr ещё не подняли сокет — логируем и идём дальше. Небольшая задержка
// даёт локальным серверам время забиндиться. Должен вызываться из cmd/apimain.go
// ПОСЛЕ AutoMigrate.
func ReplayServerCmds() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				global.Logger.Errorf("ReplayServerCmds panic: %v", r)
			}
		}()
		time.Sleep(3 * time.Second)
		states := service.AllService.ServerCmdService.AllCmdStates()
		for _, st := range states {
			port := 0
			switch st.Target {
			case model.ServerCmdTargetIdServer:
				port = global.Config.Admin.IdServerPort - 1
			case model.ServerCmdTargetRelayServer:
				port = global.Config.Admin.RelayServerPort
			default:
				continue
			}
			if _, err := service.AllService.ServerCmdService.SendCmd(port, st.Cmd, st.Option); err != nil {
				global.Logger.Warnf("ReplayServerCmds: %s %s failed: %v", st.Target, st.Cmd, err)
			}
		}
	}()
}
