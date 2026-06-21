package model

// ServerCmdState — персистентное состояние server-команд (BUGS.md AU-C-001).
// SendCmd по сокету только проксирует команду в hbbs/hbbr и НИЧЕГО не хранит,
// поэтому RELAY_SERVERS / ALWAYS_USE_RELAY / MUST_LOGIN / blocklist, выставленные
// через админку, терялись при рестарте контейнера (откатывались к env/файлам).
// Здесь храним применённые set-команды и реаплеим их на старте.
//
// Семантика хранения:
//   - replace-команды (rs/aur/ml и произвольные с option) — одна строка на
//     (target, cmd), последнее значение побеждает;
//   - аддитивные `<x>-add` / `<x>-remove` (blocklist/blacklist) — строка на
//     каждый активный add; remove удаляет соответствующий add. Так таблица
//     всегда отражает текущий набор, а реаплей его точно воспроизводит.
type ServerCmdState struct {
	IdModel
	Target string `json:"target" gorm:"size:16;index:idx_target_cmd;default:'';not null;"`
	Cmd    string `json:"cmd" gorm:"size:64;index:idx_target_cmd;default:'';not null;"`
	Option string `json:"option" gorm:"type:text;"`
	TimeModel
}
