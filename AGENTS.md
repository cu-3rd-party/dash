# AGENTS.md

Go-сервер (chi + xorm, html/template) + htmx, та же раскладка, что у cu3rd/paste и cu3rd/db-access:
слои, Makefile как единая точка входа, YAML-фикстуры, интеграционные тесты без сокета и e2e на Playwright,
запускаемые из Go. Предметная область и запуск — `README.md`.

## Раскладка

- `main.go`, `cmd/` — CLI (`urfave/cli/v3`): `web` (по умолчанию), `migrate`. `bindata.go` (тег `bindata`)
  встраивает `templates/` и `public/` в бинарь; без тега они читаются с диска (`DASH_STATIC_ROOT`).
- `models/<feature>/` — xorm-структура + функции доступа, ошибки-типы с `Unwrap()` → `db.ErrNotExist` /
  `db.ErrInvalidArgument`. Импорт в чужих пакетах — алиасом `<feature>_model` (importas).
  Фичи: `member` (доступ и роли), `category`, `expense` (статьи и их оплаты), `donation`, `sitesetting`, `audit`.
- `models/db/` — движок, `GetEngine(ctx)`, `WithTx(ctx, fn)` (вложенные вызовы переиспользуют транзакцию).
- `models/migrations/` — версионные миграции, только добавлять в конец; миграция объявляет свою копию структуры.
- `models/fixtures/<table>.yml` — фикстуры для всех уровней тестов. Даты пишутся как `"RAW='2026-01-01'"`:
  иначе testfixtures превратит строку в timestamp. Участники: `boss` (владелец), `ed` (редактор), `vi` (наблюдатель).
- `modules/money` — суммы (в минорных единицах), курсы, периоды, сдвиг срока оплаты. Даты — строки `YYYY-MM-DD`
  в московском времени: оплачивают дни, а не моменты.
- `services/auth` — сессия (HMAC cookie) и Forgejo OAuth2; `services/finance` — сводки и запись оплаты со сдвигом срока;
  `services/rates` — курсы ЦБ РФ раз в сутки (таймер в `cmd/web.go`), пишет их в `sitesetting`.
- `services/context/` — `*context.Context` для хендлеров: `Data`, `HTML`, `Doer()`, `Flash`, `RedirectWithFlash`,
  `NotFoundOrServerError` (404/422/500, тело — текст для тоста).
- `routers/web/web.go` — все маршруты и проверки ролей (`reqRole`); `routers/web/<feature>/` — хендлеры,
  `routers/api/` — `/api/summary` и вебхук донатов. Роутеры не трогают xorm (depguard).
- `templates/` — `*.tmpl`, имя = путь без расширения. `base/head` + `base/footer`, фрагменты рядом (`admin/due`).
- `web_src/js` (TypeScript, webpack + esbuild) → `public/assets/js/index.js`; `web_src/css` (Tailwind 3) →
  `public/assets/css/index.css`, раздаются по `/assets/`. Палитра темы cu3rd — `tailwind.config.ts`.

## htmx-конвенции

- Каждая форма работает без JS (`method="post" action=…`) и с htmx (`hx-post`). Успех — `RedirectWithFlash`
  (303 или `HX-Redirect`), сообщение показывается на следующей странице.
- Ошибки 4xx/5xx не свопаются, текст ответа показывается тостом. Форма статьи расхода отправляется без htmx
  и при ошибке перерисовывается со списком ошибок и введёнными значениями.

## Тесты (три уровня)

- Юнит: `make test-backend`, `make test-frontend` (vitest + happy-dom).
- Интеграционные: `tests/integration/` — весь роутер через `httptest`; сессия — `as(req, "ed")`.
  `make test-sqlite`, `make test-pgsql` (`TEST_PGSQL_DSN`).
- E2E: `tests/e2e/*.test.e2e.ts`, `make test-e2e`. Харнесс включает dev-вход, Playwright входит через форму.

## Конвенции

- Перед коммитом: `make fmt lint test test-sqlite` (CI: `make pr-go`, `make pr-frontend`, `test-pgsql`).
- Go: gofumpt, golangci-lint v2 (`.golangci.yml`), логирование — `log/slog`, `fmt.Print*` запрещён.
- Новая фича: миграция → `models/<f>/` + фикстура + тесты модели → хендлеры + маршруты → шаблоны →
  интеграционные тесты → e2e.
- npm-версии точные (`.npmrc save-exact`).
