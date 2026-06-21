package model

// ServerCmdAudit — журнал админских server-команд (BUGS.md AU-S-001).
// PR #20 закрыл группу /rustdesk/* за AdminPrivilege, но не вёл учёт того,
// КТО и КАКУЮ команду выполнял. Эта таблица + middleware (audit.go) пишут по
// записи на каждый мутирующий вызов: пользователь, метод/путь, тело запроса
// (усечённое), IP и HTTP-статус ответа.
type ServerCmdAudit struct {
	IdModel
	UserId   uint   `json:"user_id" gorm:"index;default:0;not null;"`
	Username string `json:"username" gorm:"size:128;default:'';not null;"`
	Method   string `json:"method" gorm:"size:8;default:'';not null;"`
	Path     string `json:"path" gorm:"size:255;default:'';not null;"`
	Params   string `json:"params" gorm:"type:text;"`
	Ip       string `json:"ip" gorm:"size:64;default:'';not null;"`
	Status   int    `json:"status" gorm:"default:0;not null;"`
	TimeModel
}

type ServerCmdAuditList struct {
	ServerCmdAudits []*ServerCmdAudit `json:"list"`
	Pagination
}
