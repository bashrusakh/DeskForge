# NEXT-STEPS — поднятие docker и проверка GitHub-pipeline через UI

> Состояние на 2026-06-12: §8.8.5 склеен по коду, не проверен компиляцией.
> Локальный docker очищен (тома + образы). Win-build standalone заморожен.

## 0. Что приготовить заранее

- [ ] **GitHub PAT** (fine-grained) для `bashrusakh/rustdesk`. Scope:
  - `Actions: Read and write`
  - `Secrets: Read and write` (нужен для `Push to GitHub Secrets`)
  - `Metadata: Read-only` (выдаётся автоматически)
- [ ] Скопировать существующий ключ из локального файла (если хочешь не менять секрет в форке):
  - путь: `offline-kit/artifacts/workflow-payload.key` (43 символа base64). Иначе на шаге 5 нажмёшь "Generate" и получишь новый.
- [ ] Старый Windows-агент **НЕ поднимать** — `docker-compose.yml` оставить без `build-win` (либо `docker compose up server linux-build`).

## 1. Поднять docker

```powershell
cd E:\_projects\full_Server\docker
docker compose build server     # пересборка из-за нового Go-кода (model+service+controller)
docker compose up -d server      # без build-win!
docker compose logs -f server    # следить за стартом
```

- [ ] Логи должны показать `Migrating....268` (DatabaseVersion = 268). Если нет — миграция не прошла.
- [ ] Если падает на компиляции Go → читать ошибку, чинить, повторять. Самые вероятные косяки:
  - опечатка в импорте (`gorm.io/gorm`, `golang.org/x/crypto/nacl/box`, `golang.org/x/crypto/pbkdf2`)
  - забытая регистрация в `service/service.go` Service struct
  - `archive/zip` / `bytes` / `time` в `custom_build.go` — проверить
- [ ] Healthcheck: `docker compose ps` → server `Up (healthy)`.

## 2. Открыть admin-ui

- [ ] Браузер: `http://localhost:21114/admin/`
- [ ] Залогиниться (admin)
- [ ] Nav слева: **Server → GitHub Build** (новый пункт, иконка Connection)

## 3. Заполнить форму

- [ ] Repository: `bashrusakh/rustdesk`
- [ ] Workflow filename: `rustqs-windows-min-test.yml`
- [ ] Branch: `rustqs/min-test`
- [ ] GitHub Token: вставить PAT из шага 0
- [ ] Encryption key: оставить пусто (нажмём Generate ниже) ИЛИ вставить старый из файла
- [ ] **Save**

## 4. Test connection

- [ ] Нажать **Test connection**
- [ ] Ожидается: зелёная плашка "ok"
- [ ] Если "HTTP 401/403" — PAT неправильный или нет прав
- [ ] Если "HTTP 404" — опечатка в repo

## 5. Encryption key

Вариант A (новый ключ — рекомендую):
- [ ] **Generate new key** → появится поле с base64
- [ ] **Push to GitHub Secrets** → плашка "WORKFLOW_PAYLOAD_KEY synced"
- [ ] Проверить: github.com/bashrusakh/rustdesk/settings/secrets/actions → `WORKFLOW_PAYLOAD_KEY` обновлён "less than a minute ago"

Вариант B (использовать ключ из offline-kit/artifacts/workflow-payload.key — он уже в форке):
- [ ] Вставить значение в поле Encryption key → Save
- [ ] Push to Secrets можно пропустить

## 6. Trigger test build (sanity-check workflow_dispatch)

- [ ] Нажать **Trigger test build**
- [ ] Ожидается: "Run started: id=...", ссылка на GitHub
- [ ] Открыть ссылку, увидеть свежий ран `rustqs windows min test`
- [ ] (не дожидаться завершения — это 25-30 мин)

## 7. Полный e2e через Custom Client

- [ ] Nav: **Custom Client → New Build**
- [ ] Platform: **Windows**
- [ ] App name: `rustqs`
- [ ] Custom JSON (важно — формат строгий): сейчас Go ожидает поля `server`, `key`, `custom_txt` в корне CustomJson. Что-то вроде:
  ```json
  {"server":"твой.сервер:21116","key":"твой_RS_PUB_KEY_base64","custom_txt":"eyJwYXNzd29yZCI6InRlc3QifQ=="}
  ```
  (`custom_txt` — это base64 от `{"password":"..."}`)
- [ ] Create → в списке появится запись со статусом `building`
- [ ] В Go-логе: `github run id: <число>` — это запустился `workflow_dispatch`
- [ ] Ждать ~30 мин (поллинг каждые 30 сек на стороне сервера)
- [ ] Статус → `done`, появится Download

## 8. Проверить артефакт

- [ ] На сервере (внутри контейнера): `ls /rdgen-data/output/<id>/`
- [ ] Должны быть: `rustqs.exe`, `custom_.txt`, несколько `.dll`
- [ ] Скачать через UI → запустить на Windows → должен подключиться к вшитому серверу
  с вшитым паролем

## Если что-то падает

| Симптом | Где смотреть | Вероятная причина |
|---|---|---|
| `Migrating....` отсутствует / падает | `docker compose logs server` | Go-код не собрался; смотреть `docker compose build` |
| Test connection → 401/403 | UI alert | PAT неверный/без прав |
| Test connection → 404 | UI alert | опечатка в `repo` |
| Push to Secrets → 403 | UI alert | у PAT нет `Secrets: write` |
| Build стоит в `building` >40 мин | `docker compose logs server`; GitHub Actions UI | поллер упал, или ран висит |
| Build → `failed` сразу | `b.BuildLog` в записи Custom Client | dispatch error: бывает invalid `enc_payload` |
| Build → `failed` после рана | BuildLog | exe не найден в zip, или 90-мин таймаут |

## После успеха

- [ ] Скоммитить локальный код в свой репо `full_Server` (git init если не было).
- [ ] Закрыть `[~] 8.8.5` в PLAN.md как `[x]`.
- [ ] Решить: оставлять ли `[Debug]` плейн-input'ы в воркфлоу (сейчас они там для отладки), или убрать (тогда только enc_payload).
