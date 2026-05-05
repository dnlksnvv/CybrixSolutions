# Локальная проверка перед запуском сервиса (verify / E2E)

Цель: **одной командой** убедиться, что код собирается, unit/API-тесты проходят, сервис поднимается на **отдельной тестовой MongoDB** (`agents_service_e2e`), HTTP E2E сценарии из `docs/api-test.md` выполняются.

Production-контейнер **не** запускает тесты при старте. Проверка — отдельный pre-flight шаг (`make verify` или compose).

---

## Что используется

| Компонент | Назначение |
|-----------|------------|
| `Dockerfile` | Multi-stage: `base`, `build`, `test`, `production` (distroless), `runtime-test` (Alpine + curl для healthcheck) |
| `docker-compose.test.yml` | `mongo-test` (однонодовый replica set) → `agents-service-test` → `e2e-test` |
| `Makefile` | `test`, `e2e`, `verify`, `run`, `build` |
| `tests/e2e/` | HTTP E2E, только с `-tags=e2e` |

База в compose: **`MONGO_DB=agents_service_e2e`**, URI **`mongodb://mongo-test:27017`** (без auth, только для изолированного compose).

Mongo запускается с **`--replSet rs0`** и инициализацией в healthcheck: операции **`WithTransaction`** (создание агента и др.) на обычном standalone MongoDB в Docker дают 500.

Volume **`mongo-test-data`** удаляется при `docker compose ... down -v` (см. `make e2e`).

---

## Команды

### Только unit/API тесты (без Docker, без реальной MongoDB)

```bash
make test
# или
go test ./... -count=1
```

Пакет `tests/e2e` без тега `e2e` не выполняет HTTP-тесты (только `doc.go`).

### Полный pipeline в Docker (unit + E2E с реальной MongoDB)

```bash
make e2e
```

Эквивалентно:

```bash
docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from e2e-test; \
  EXIT=$?; \
  docker compose -f docker-compose.test.yml down -v --remove-orphans; \
  exit $EXIT
```

Контейнер **`e2e-test`** последовательно выполняет:

1. `go test ./... -count=1` — в том числе `internal/transport/httpapi` (httptest + in-memory репозитории).
2. `go test ./tests/e2e -tags=e2e -v` — запросы к **`agents-service-test`** по сети compose.

### Локальные тесты + Docker pipeline

```bash
make verify
```

Сначала `make test`, затем `make e2e` (unit-тесты внутри Docker прогоняются второй раз — дополнительная проверка в Linux-окружении).

### Обычный локальный запуск приложения

```bash
make run
```

Нужны рабочие `MONGO_URI` / `MONGO_DB` в `.env` (не тот же compose, что для E2E, если не настраивать специально).

### Сборка production-образа (distroless)

```bash
make build
```

---

## Переменные E2E

| Переменная | По умолчанию (в compose) |
|------------|---------------------------|
| `AGENTS_SERVICE_BASE_URL` | `http://agents-service-test:8080` |
| `AGENTS_SERVICE_WORKSPACE_ID` | из **`E2E_WORKSPACE_ID`** на хосте; `make e2e` подставляет `ws_e2e_<unix_ts>`. Если не задано — тест генерирует уникальный id сам (`UnixNano`). |

Переопределение workspace:

```bash
E2E_WORKSPACE_ID=ws_manual_1 make e2e
```

Локальный прогон E2E против уже запущенного сервиса:

```bash
export AGENTS_SERVICE_BASE_URL=http://localhost:9001
export AGENTS_SERVICE_WORKSPACE_ID=ws_test
go test ./tests/e2e -tags=e2e -count=1 -v
```

---

## Когда можно запускать «боевой» сервис

После успешного **`make verify`** (или как минимум **`make test`** + ручная проверка) считается, что:

- валидаторы и usecase-логика в порядке;
- HTTP слой с middleware и маршрутизацией проверен httptest-тестами;
- интеграция с MongoDB и основной сценарий API подтверждены E2E в изолированном compose.

Дальше можно поднимать сервис обычной командой (`make run` или свой `docker compose` прод-стека). **Не** подключайте E2E к production MongoDB.

---

## Ограничение HTTP body в E2E

В `docker-compose.test.yml` для `agents-service-test` задано **`HTTP_BODY_LIMIT_BYTES=20971520`**, чтобы запрос PATCH для сценария `workflow_size_exceeded` доходил до валидатора (8MB по полю workflow). В проде лимит может оставаться 8MB — это отдельная политика деплоя.

---

## Покрытие сценариев `api-test.md`

E2E в `tests/e2e/smoke_test.go` покрывает сценарии **1–24**, **22** (списки после unpublish), **25–26** (лимиты), **27–28** (папки + Template Agents). Детали и ручные curl — в `docs/api-test.md` и `docs/AgentsService-testing.md`.
