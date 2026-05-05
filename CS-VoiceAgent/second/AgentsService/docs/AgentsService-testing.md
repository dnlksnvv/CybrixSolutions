

# AgentsService — единая документация по тестированию

> Этот документ фиксирует итоговую стратегию тестирования `AgentService`.
>
> Его задача — собрать в одном месте всё, что относится к проверке сервиса: unit-тесты, API-тесты через `httptest`, E2E-тесты через реальный HTTP, тестовую MongoDB, pre-flight pipeline и ручные сценарии проверки.
>
> Документ можно использовать как шаблон для тестирования следующих микросервисов.

---

# 1. Главная цель тестирования

Тестирование `AgentService` должно подтвердить, что сервис не просто компилируется и запускается, а реально корректно работает по бизнес-правилам.

Проверяются:

- доменные валидаторы;
- лимиты строк, массивов, числовых полей и размера workflow;
- usecase-логика;
- запреты на редактирование и удаление опубликованной версии workflow;
- создание агента вместе с workflow v0 и PublishedWorkflow;
- публикация workflow;
- снятие публикации;
- поведение `agent.response_engine`;
- runtime API для микросервиса звонков;
- protected API для frontend/admin;
- обязательность `X-Workspace-Id` только для protected API;
- отсутствие требования `X-Workspace-Id` для runtime API;
- формат успешных JSON-ответов;
- формат ошибок;
- интеграция с MongoDB;
- работа MongoDB transactions;
- работа MongoDB indexes;
- работа Mongo aggregation для runtime-списков;
- полный пользовательский сценарий через HTTP.

---

# 2. Уровни тестирования

В сервисе используется несколько уровней тестирования:

```text
1. Unit tests
2. API tests через httptest
3. E2E tests через реальный HTTP + тестовую MongoDB
4. Ручные curl-сценарии для локальной отладки
```

Каждый уровень проверяет разные вещи. Их нельзя полностью заменить друг другом.

---

# 3. Unit tests

## 3.1. Назначение

Unit-тесты проверяют отдельные части сервиса без реального HTTP и без реальной MongoDB.

Они должны быть быстрыми, стабильными и запускаться локально командой:

```bash
go test ./... -count=1
```

## 3.2. Что проверяют unit-тесты

Unit-тесты проверяют:

- валидацию папок;
- валидацию workflow;
- лимит количества nodes;
- лимит размера workflow JSON;
- уникальность `Node.id`;
- корректность `start_node_id`;
- бизнес-логику публикации workflow;
- бизнес-логику удаления опубликованной версии;
- runtime usecase;
- перезапись `agent.response_engine` из PublishedWorkflow в runtime config;
- ошибку `published_workflow_not_found`, если PublishedWorkflow отсутствует.

## 3.3. Что unit-тесты не проверяют

Unit-тесты не проверяют:

- реальный Gin router;
- реальные HTTP routes;
- реальные JSON responses;
- реальную MongoDB;
- BSON-теги;
- Mongo aggregation;
- Mongo indexes;
- Mongo transactions;
- сетевое взаимодействие между контейнерами.

Для этого нужны API tests и E2E tests.

---

# 4. Существующие unit-тесты по файлам

| Файл | Что проверяет | Тип | Реальная MongoDB | HTTP / JSON |
|---|---|---|---|---|
| `internal/domain/validation/folder_validator_test.go` | Валидация имени папки, успешный случай и запрет `Template Agents` | Unit | Нет | Нет |
| `internal/domain/validation/workflow_validator_test.go` | Структура workflow, дубликаты `Node.id`, стартовая нода, превышение размера serialized JSON | Unit | Нет | Нет |
| `internal/app/usecase/conversation_flow_usecase_test.go` | `Publish` обновляет PublishedWorkflow и `agent.response_engine`; `DeleteVersion` для опубликованной версии возвращает `cannot_delete_published_version` | Unit | Нет, используются fake repositories | Нет |
| `internal/app/usecase/runtime_usecase_test.go` | Runtime config перезаписывает `response_engine` из PublishedWorkflow; при отсутствии PublishedWorkflow возвращается `published_workflow_not_found` | Unit | Нет, используются fake repositories | Нет |

---

# 5. API tests через httptest

## 5.1. Назначение

API-тесты через `httptest` поднимают Gin router в памяти и делают HTTP-запросы без реального сетевого порта.

Они проверяют связку:

```text
HTTP request -> router -> middleware -> handler -> usecase -> in-memory repository -> HTTP response
```

## 5.2. Что проверяют API tests

API tests проверяют:

- что routes зарегистрированы правильно;
- что runtime routes не требуют `X-Workspace-Id`;
- что protected routes требуют `X-Workspace-Id`;
- что middleware возвращает `missing_workspace_id`;
- что `POST /api/v1/agents` создаёт Agent + ConversationFlow v0 + PublishedWorkflow;
- что editor flow response возвращает `published` и `response_engine`;
- что runtime `published-config` возвращает склеенный config;
- что publish обновляет PublishedWorkflow и `agent.response_engine`;
- что unpublish удаляет только PublishedWorkflow;
- что после unpublish runtime config возвращает `published_workflow_not_found`;
- что опубликованную версию нельзя PATCH;
- что опубликованную версию нельзя DELETE;
- что агента с PublishedWorkflow нельзя DELETE;
- что workflow > 1000 nodes возвращает ошибку;
- что workflow JSON > 8MB возвращает `workflow_size_exceeded`.

## 5.3. Что API tests не проверяют

API tests через `httptest` не проверяют реальную MongoDB.

Вместо MongoDB используется in-memory store.

Поэтому API tests не ловят ошибки:

- BSON-тегов;
- MongoDB aggregation;
- MongoDB indexes;
- MongoDB transactions;
- подключения к реальной БД.

Для этого нужен E2E pipeline.

---

# 6. Добавленные файлы для API tests

## 6.1. `internal/transport/httpapi/mount.go`

Вынесенная регистрация middleware и маршрутов.

Нужна для того, чтобы один и тот же роутинг использовался:

- в обычном запуске сервиса;
- в `httptest` тестах.

Это снижает риск, что тесты проверяют не тот router, который используется в production-коде.

## 6.2. `internal/transport/httpapi/memory_store_api_test.go`

In-memory реализации repository interfaces:

- `FolderRepository`;
- `AgentRepository`;
- `ConversationFlowRepository`;
- `PublishedWorkflowRepository`;
- `TransactionManager`.

`memTx.WithTransaction` вызывает callback без реальной MongoDB transaction.

Это нормально для API-тестов, потому что их задача — проверить HTTP/API слой и usecase flow, а не MongoDB transaction engine.

## 6.3. `internal/transport/httpapi/api_integration_test.go`

Основной файл API-тестов через `httptest`.

---

# 7. API tests: список тестов

| Тест | Что проверяет |
|---|---|
| `TestAPI_RuntimeVsProtectedWorkspaceHeader` | Runtime endpoint без `X-Workspace-Id` возвращает 200, protected endpoint без `X-Workspace-Id` возвращает 400 `missing_workspace_id` |
| `TestAPI_FullHappyPath_AgentFlowAndRuntimeConfig` | Создание агента, получение списка версий, получение flow v0, проверка `published`, проверка `response_engine`, runtime `published-config` |
| `TestAPI_UnpublishPath` | `unpublish`, затем editor flow доступен с `published=false`, `agent.response_engine` сохраняется, runtime config возвращает `published_workflow_not_found` |
| `TestAPI_PublishedVersionProtectionAndAgentDeleteBlocked` | PATCH опубликованной версии запрещён, DELETE опубликованной версии запрещён, DELETE агента с PublishedWorkflow запрещён |
| `TestAPI_Validation_NodeLimitAndWorkflowSize` | Workflow > 1000 nodes возвращает `max_items_exceeded`; workflow JSON > 8MB возвращает `workflow_size_exceeded` |
| `TestAPI_PublishNewVersionUpdatesAgentResponseEngine` | Создание v1 из v0, публикация v1, проверка что `GET /agents/{id}` возвращает `response_engine.version == 1` |

---

# 8. E2E tests

## 8.1. Назначение

E2E tests проверяют сервис как настоящий внешний потребитель.

Они делают реальные HTTP-запросы к поднятому `agents-service-test` контейнеру и используют настоящую тестовую MongoDB.

E2E tests проверяют:

- что сервис реально стартует;
- что HTTP routes доступны по сети;
- что MongoDB подключается;
- что MongoDB transactions работают;
- что MongoDB indexes создаются;
- что PublishedWorkflow aggregation работает;
- что BSON mapping работает;
- что весь API-сценарий проходит на настоящей БД;
- что данные сохраняются и читаются между разными HTTP-запросами.

## 8.2. Где находятся E2E tests

E2E tests находятся в:

```text
tests/e2e/
```

Они запускаются только с build tag:

```text
e2e
```

Команда:

```bash
go test ./tests/e2e -tags=e2e -count=1 -v
```

Если запускать обычный `go test ./...` без тега `e2e`, пакет `tests/e2e` не выполняет HTTP E2E-сценарии.

---

# 9. Тестовая MongoDB

## 9.1. Unit/API tests

Unit и `httptest` API tests не используют реальную MongoDB.

Они используют fake/in-memory repositories.

## 9.2. E2E tests

E2E tests используют отдельную MongoDB в Docker Compose.

База:

```text
agents_service_e2e
```

URI внутри compose:

```text
mongodb://mongo-test:27017
```

Это изолированная тестовая база без production-данных.

## 9.3. Почему нужен replica set

В сервисе есть операции через `WithTransaction`.

MongoDB transactions не работают на standalone MongoDB.

Поэтому в `docker-compose.test.yml` MongoDB должна запускаться как однонодовый replica set:

```text
mongod --replSet rs0
```

Инициализация выполняется через:

```text
rs.initiate()
```

Если MongoDB запустить без replica set, создание агента может вернуть `500`, потому что `POST /api/v1/agents` выполняет транзакционную операцию:

```text
создать Agent + создать ConversationFlow v0 + создать PublishedWorkflow
```

---

# 10. Docker E2E pipeline

## 10.1. Компоненты

| Компонент | Назначение |
|---|---|
| `Dockerfile` | Multi-stage build: `base`, `build`, `test`, `production`, `runtime-test` |
| `docker-compose.test.yml` | Поднимает `mongo-test`, `agents-service-test`, `e2e-test` |
| `Makefile` | Команды `test`, `e2e`, `verify`, `run`, `build` |
| `tests/e2e/` | HTTP E2E tests с build tag `e2e` |
| `docs/tests/manual-api-smoke-test.md` | Ручной curl сценарий проверки API |
| `docs/tests/e2e-verify-guide.md` | Документация по pre-flight и Docker E2E |

## 10.2. Контейнеры

### `mongo-test`

Тестовая MongoDB.

Должна быть replica set.

Используется только для E2E.

### `agents-service-test`

Тестовый экземпляр сервиса.

Подключается к `mongo-test`.

### `e2e-test`

Контейнер, который запускает тесты.

Он выполняет:

```bash
go test ./... -count=1
go test ./tests/e2e -tags=e2e -v
```

---

# 11. Makefile команды

## 11.1. `make test`

Запускает unit и API tests без Docker и без реальной MongoDB.

```bash
make test
```

Эквивалент:

```bash
go test ./... -count=1
```

## 11.2. `make e2e`

Запускает полный Docker E2E pipeline.

```bash
make e2e
```

Эквивалентно:

```bash
docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from e2e-test
EXIT=$?
docker compose -f docker-compose.test.yml down -v --remove-orphans
exit $EXIT
```

После завершения test volume удаляется через `down -v`.

## 11.3. `make verify`

Полная локальная проверка перед запуском сервиса.

```bash
make verify
```

Она должна выполнять:

```text
1. make test
2. make e2e
```

Да, unit/API tests внутри Docker могут прогоняться второй раз. Это нормально, потому что так проверяется Linux/container окружение.

## 11.4. `make run`

Обычный локальный запуск приложения.

```bash
make run
```

Для него нужны рабочие `.env` значения:

```text
MONGO_URI
MONGO_DB
HTTP_PORT
```

## 11.5. `make build`

Сборка production image.

```bash
make build
```

---

# 12. Production container не запускает тесты

Production container не должен запускать E2E при старте.

Правильная схема:

```text
make verify -> если успешно -> запуск production service
```

Неправильная схема:

```text
production service starts -> runs tests -> then starts app
```

Почему нельзя запускать E2E при старте production:

- можно случайно подключиться к production MongoDB;
- можно случайно создать/удалить production data;
- production startup должен быть быстрым;
- startup не должен зависеть от тестовых сценариев;
- тесты и runtime — разные этапы жизненного цикла.

---

# 13. Переменные E2E

| Переменная | Значение по умолчанию |
|---|---|
| `AGENTS_SERVICE_BASE_URL` | `http://agents-service-test:8080` |
| `AGENTS_SERVICE_WORKSPACE_ID` | `ws_e2e_<timestamp>` или UnixNano, если явно не задано |

Пример ручного задания workspace:

```bash
E2E_WORKSPACE_ID=ws_manual_1 make e2e
```

Пример запуска E2E против уже запущенного локального сервиса:

```bash
export AGENTS_SERVICE_BASE_URL=http://localhost:9001
export AGENTS_SERVICE_WORKSPACE_ID=ws_test
go test ./tests/e2e -tags=e2e -count=1 -v
```

Важно: если запускать E2E против локального сервиса, нужно убедиться, что он подключен к тестовой базе, а не к production базе.

---

# 14. Почему workspace должен быть уникальным в E2E

В сервисе есть лимит:

```text
100 агентов на workspace
```

Если E2E много раз запускать на одном и том же workspace без очистки данных, можно упереться в лимит и получить ложное падение тестов.

Поэтому E2E использует уникальный workspace:

```text
ws_e2e_<timestamp>
```

или генерирует workspace через `UnixNano`.

---

# 15. HTTP body limit в E2E

В production лимит тела запроса может быть:

```text
8 MB
```

Но в E2E для проверки `workflow_size_exceeded` полезно поставить HTTP body limit больше, например:

```text
20 MB = 20971520 bytes
```

Зачем:

- если HTTP middleware обрежет запрос на 8MB, запрос не дойдёт до workflow validator;
- тогда нельзя проверить именно ошибку `workflow_size_exceeded`;
- поэтому в E2E body limit поднимается до 20MB, а validator всё равно проверяет лимит workflow JSON 8MB.

Это не противоречие, а отдельная политика тестового окружения.

---

# 16. Что значит “запрос падает”

Когда говорится:

```text
workflow > 1000 nodes падает
workflow > 8MB падает
нельзя редактировать опубликованный workflow
```

это не означает, что сервис должен крашнуться.

Правильное поведение:

```text
сервис возвращает нормальную API-ошибку и продолжает работать
```

Примеры:

- workflow > 1000 nodes -> `400 validation_error max_items_exceeded`;
- workflow > 8MB -> `400 validation_error workflow_size_exceeded`;
- PATCH опубликованной версии -> `400 business_error published_version_is_readonly`;
- DELETE опубликованной версии -> `400 business_error cannot_delete_published_version`;
- DELETE агента с PublishedWorkflow -> `400 business_error agent_has_published_workflow`.

---

# 17. Чеклист бизнес-сценариев

| Сценарий | Проверяется |
|---|---|
| Runtime routes не требуют `X-Workspace-Id` | API / E2E |
| Protected routes требуют `X-Workspace-Id` | API / E2E |
| `POST /agents` создаёт Agent + ConversationFlow v0 + PublishedWorkflow | API / E2E |
| `GET flow version=0` возвращает `published` и `response_engine` | API / E2E |
| Runtime `published-config` возвращает Agent + ConversationFlow + PublishedWorkflow | Unit / API / E2E |
| Publish новой версии обновляет PublishedWorkflow | Unit / API / E2E |
| Publish новой версии обновляет `agent.response_engine` | Unit / API / E2E |
| Unpublish удаляет PublishedWorkflow | API / E2E |
| Unpublish не очищает `agent.response_engine` | API |
| После unpublish runtime config возвращает `published_workflow_not_found` | Unit / API / E2E |
| Опубликованную версию нельзя PATCH | Unit / API / E2E |
| Опубликованную версию нельзя DELETE | Unit / API / E2E |
| Агента с PublishedWorkflow нельзя DELETE | API / E2E |
| Workflow > 1000 nodes возвращает ошибку | Unit / API / E2E |
| Workflow JSON > 8MB возвращает ошибку | Unit / API / E2E |
| Template Agents нельзя создать как обычную папку | Unit / E2E |
| Папки работают | E2E |
| Published agents grouped by workspace работают | E2E |

---

# 18. Ручной API smoke test

Ручной сценарий хранится в:

```text
docs/tests/manual-api-smoke-test.md
```

Он нужен для быстрой проверки сервиса через `curl`.

Ручной smoke test проверяет:

1. `/healthz`;
2. runtime endpoint без `X-Workspace-Id`;
3. protected endpoint без `X-Workspace-Id`;
4. создание агента;
5. получение `agent_id` и `conversation_flow_id`;
6. получение flow v0;
7. runtime `published-config` для v0;
8. создание версии v1;
9. публикацию v1;
10. runtime `published-config` для v1;
11. unpublish;
12. проверку, что frontend flow всё ещё открывается;
13. проверку, что runtime config после unpublish возвращает `published_workflow_not_found`.

---

# 19. Ручной curl-сценарий

```bash
#!/usr/bin/env bash
set -euo pipefail

BASE="http://localhost:9001"
WS="ws_test"
HDR=(-H "X-Workspace-Id: ${WS}" -H "Content-Type: application/json")

echo "=== 1) healthz ==="
curl -sS "${BASE}/healthz" | jq .

echo "=== 2) runtime без X-Workspace-Id (должен быть 200) ==="
curl -sS -w "\nHTTP %{http_code}\n" "${BASE}/api/v1/runtime/workspaces/published-agents" | head -c 2000
echo

echo "=== 3) protected без X-Workspace-Id (должен быть 400 missing_workspace_id) ==="
curl -sS -w "\nHTTP %{http_code}\n" "${BASE}/api/v1/agents"
echo

echo "=== 4–5) создать агента, взять agent_id и conversation_flow_id ==="
CREATE_RESP="$(curl -sS -X POST "${BASE}/api/v1/agents" "${HDR[@]}" -d '{}')"
echo "${CREATE_RESP}" | jq .
AGENT_ID="$(echo "${CREATE_RESP}" | jq -r '.data.agent_id')"
FLOW_ID="$(echo "${CREATE_RESP}" | jq -r '.data.response_engine.conversation_flow_id')"
echo "AGENT_ID=${AGENT_ID}"
echo "FLOW_ID=${FLOW_ID}"

echo "=== 6) flow v0: published + response_engine ==="
curl -sS "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}?version=0" "${HDR[@]}" | jq '.data | {published, response_engine, version}'

echo "=== 7) runtime published-config (ожидается v0 в опубликованном конфиге) ==="
curl -sS "${BASE}/api/v1/runtime/workspaces/${WS}/agents/${AGENT_ID}/published-config" | jq '.data | {agent: .agent.agent_id, cf_version: .conversation_flow.version, pw_version: .published_workflow.version}'

echo "=== 8) создать версию v1 (fromVersion=0) ==="
curl -sS -X POST "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}/versions?fromVersion=0" "${HDR[@]}" | jq '.data | {version, conversation_flow_id, published, response_engine}'

echo "=== 9) опубликовать v1 ==="
curl -sS -X POST "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}/publish?version=1" "${HDR[@]}" | jq .

echo "=== 10) runtime published-config должен отдавать v1 ==="
curl -sS "${BASE}/api/v1/runtime/workspaces/${WS}/agents/${AGENT_ID}/published-config" | jq '.data | {cf_version: .conversation_flow.version, pw_version: .published_workflow.version}'

echo "=== 11) unpublish (ожидается HTTP 204, тело пустое) ==="
curl -sS -o /dev/null -w "HTTP %{http_code}\n" -X POST "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/unpublish" "${HDR[@]}" -d '{}'

echo "=== 12) frontend flow v0 всё ещё открывается (ожидается 200, published=false) ==="
curl -sS "${BASE}/api/v1/agents/${AGENT_ID}/conversation-flows/${FLOW_ID}?version=0" "${HDR[@]}" | jq '.data | {published, response_engine, version}'

echo "=== 13) runtime published-config → 404 published_workflow_not_found ==="
curl -sS -w "\nHTTP %{http_code}\n" "${BASE}/api/v1/runtime/workspaces/${WS}/agents/${AGENT_ID}/published-config" | jq .
```

---

# 20. Важный момент для zsh

Если вставлять большой bash-скрипт прямо в zsh, можно получить ошибку вида:

```text
zsh: event not found: /usr/bin/env
```

Чтобы избежать проблем:

1. Сохранять скрипт в файл, например `scripts/api-smoke.sh`.
2. Запускать через bash:

```bash
bash ./scripts/api-smoke.sh
```

3. Не вставлять многострочный скрипт напрямую в zsh.

---

# 21. E2E сценарии из `docs/tests/manual-api-smoke-test.md`

E2E tests должны покрывать сценарии из ручного API-теста, но автоматически.

Минимальный E2E набор:

## 21.1. Healthcheck

```http
GET /healthz
```

Ожидается:

```text
200 OK
```

## 21.2. Runtime без workspace header

```http
GET /api/v1/runtime/workspaces/published-agents
```

Без `X-Workspace-Id`.

Ожидается:

```text
200 OK
```

## 21.3. Protected без workspace header

```http
GET /api/v1/agents
```

Без `X-Workspace-Id`.

Ожидается:

```text
400 missing_workspace_id
```

## 21.4. Создание агента

```http
POST /api/v1/agents
X-Workspace-Id: ws_e2e_xxx
Content-Type: application/json

{}
```

Ожидается:

- `201 Created`;
- есть `data.agent_id`;
- есть `data.response_engine.conversation_flow_id`;
- `data.response_engine.version == 0`.

## 21.5. Получение flow v0

```http
GET /api/v1/agents/{agentId}/conversation-flows/{flowId}?version=0
```

Ожидается:

- `200 OK`;
- `data.version == 0`;
- `data.published == true`;
- `data.response_engine.conversation_flow_id == flowId`;
- `data.response_engine.version == 0`.

## 21.6. Runtime published-config v0

```http
GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
```

Ожидается:

- `200 OK`;
- `data.agent.agent_id == agentId`;
- `data.conversation_flow.version == 0`;
- `data.published_workflow.version == 0`;
- `data.agent.response_engine.version == 0`.

## 21.7. Создание версии v1

```http
POST /api/v1/agents/{agentId}/conversation-flows/{flowId}/versions?fromVersion=0
```

Ожидается:

- `201 Created`;
- `data.version == 1`;
- `data.published == false`;
- `data.response_engine.version == 1`.

## 21.8. Runtime всё ещё отдаёт v0

До публикации v1 runtime должен продолжать отдавать v0.

Ожидается:

- `data.conversation_flow.version == 0`;
- `data.published_workflow.version == 0`.

## 21.9. Публикация v1

```http
POST /api/v1/agents/{agentId}/conversation-flows/{flowId}/publish?version=1
```

Ожидается:

- `200 OK`;
- `data.version == 1`;
- `data.published == true`.

## 21.10. Runtime отдаёт v1

После публикации v1 runtime должен отдавать v1.

Ожидается:

- `data.conversation_flow.version == 1`;
- `data.published_workflow.version == 1`;
- `data.agent.response_engine.version == 1`.

## 21.11. Запрет PATCH опубликованной v1

```http
PATCH /api/v1/agents/{agentId}/conversation-flows/{flowId}?version=1
```

Ожидается:

```text
400 published_version_is_readonly
```

## 21.12. Запрет DELETE опубликованной v1

```http
DELETE /api/v1/agents/{agentId}/conversation-flows/{flowId}?version=1
```

Ожидается:

```text
400 cannot_delete_published_version
```

## 21.13. Запрет DELETE агента с PublishedWorkflow

```http
DELETE /api/v1/agents/{agentId}
```

Ожидается:

```text
400 agent_has_published_workflow
```

## 21.14. Unpublish

```http
POST /api/v1/agents/{agentId}/conversation-flows/unpublish
```

Ожидается:

```text
204 No Content
```

## 21.15. После unpublish frontend flow открывается

```http
GET /api/v1/agents/{agentId}/conversation-flows/{flowId}?version=0
```

Ожидается:

- `200 OK`;
- `data.published == false`.

## 21.16. После unpublish runtime config не доступен

```http
GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
```

Ожидается:

```text
404 published_workflow_not_found
```

## 21.17. После unpublish агент может быть удалён

```http
DELETE /api/v1/agents/{agentId}
```

Ожидается:

```text
204 No Content
```

---

# 22. Проверка лимитов

## 22.1. Workflow > 1000 nodes

Сценарий:

1. Создать агента.
2. Снять публикацию, чтобы v0 стала редактируемой.
3. Отправить PATCH workflow с `nodes` длиной 1001.

Ожидается:

```text
400 validation_error max_items_exceeded
```

Важно: это не crash сервиса, а нормальный отказ принять невалидные данные.

## 22.2. Workflow JSON > 8MB

Сценарий:

1. Создать агента.
2. Снять публикацию.
3. Отправить PATCH workflow с огромным `instruction.text`, чтобы serialized workflow стал больше 8MB.

Ожидается:

```text
400 validation_error workflow_size_exceeded
```

В E2E окружении HTTP body limit может быть 20MB, чтобы запрос дошёл до validator.

---

# 23. Проверка папок

E2E должен проверять:

## 23.1. Создание папки

```http
POST /api/v1/folders
```

Ожидается:

```text
201 Created
```

## 23.2. Переименование папки

```http
PATCH /api/v1/folders/{folderId}
```

Ожидается:

```text
200 OK
```

## 23.3. Запрет Template Agents

```http
POST /api/v1/folders

{
  "name": "Template Agents"
}
```

Ожидается:

```text
400 template_folder_is_virtual
```

## 23.4. Удаление папки

```http
DELETE /api/v1/folders/{folderId}
```

Ожидается:

```text
204 No Content
```

Если в папке были агенты, у них должен сброситься `folder_id`.

---

# 24. Проверка runtime списков

## 24.1. Все workspace с published agents

```http
GET /api/v1/runtime/workspaces/published-agents
```

Ожидается:

- `200 OK`;
- в списке есть workspace текущего E2E;
- внутри workspace есть agent_id созданного опубликованного агента.

## 24.2. Published agents одного workspace

```http
GET /api/v1/runtime/workspaces/{workspaceId}/published-agents
```

Ожидается:

- `200 OK`;
- список содержит published agents workspace;
- после unpublish агент исчезает из списка.

---

# 25. Что было исправлено в процессе настройки E2E

## 25.1. Mongo без replica set

Проблема:

`CreateAgent` использует `WithTransaction`.

На standalone MongoDB транзакции не работают.

Симптом:

```text
500 на POST /api/v1/agents
```

Решение:

В `docker-compose.test.yml` для `mongo-test` добавлен запуск:

```text
mongod --replSet rs0
```

И healthcheck с `rs.initiate`.

## 25.2. Пустой data у published-agents

Проблема:

Для пустого списка JSON мог возвращать:

```json
"data": null
```

E2E учитывает это и трактует `nil` как пустой slice.

## 25.3. BSON-теги для runtime aggregation

Проблема:

`ListGroupedByWorkspace` возвращал документы с полями:

- `agent_id`;
- `conversation_flow_id`;
- `version`;
- `published_at`.

Но у DTO не было bson-тегов.

Mongo driver не маппил поля, и в ответе были пустые id.

Решение:

Добавить bson-теги в:

```text
internal/repository/interfaces/published_workflow_repository.go
```

## 25.4. Фиксированный workspace

Проблема:

Если постоянно использовать один workspace, можно упереться в лимит 100 агентов.

Решение:

`Makefile` для E2E задаёт:

```text
E2E_WORKSPACE_ID=ws_e2e_<timestamp>
```

Если переменная не задана, тест сам берёт уникальный id через UnixNano.

---

# 26. Команды проверки

## 26.1. Быстрая проверка без Docker

```bash
go test ./... -count=1
```

или:

```bash
make test
```

## 26.2. Проверка только API tests

```bash
go test ./internal/transport/httpapi/... -count=1 -v
```

## 26.3. Полная Docker E2E проверка

```bash
make e2e
```

или:

```bash
docker compose -f docker-compose.test.yml up --build --abort-on-container-exit --exit-code-from e2e-test
```

после завершения:

```bash
docker compose -f docker-compose.test.yml down -v --remove-orphans
```

## 26.4. Полная pre-flight проверка

```bash
make verify
```

---

# 27. Когда можно запускать сервис

Сервис можно считать готовым к локальному запуску или деплою после успешного:

```bash
make verify
```

Минимально перед локальной разработкой:

```bash
make test
```

Перед серьёзным запуском желательно:

```bash
make verify
```

После успешного verify считается, что:

- код компилируется;
- unit tests проходят;
- API tests проходят;
- сервис поднимается в Docker;
- тестовая MongoDB работает как replica set;
- транзакции MongoDB работают;
- E2E HTTP сценарий проходит;
- runtime API работает;
- protected API работает;
- лимиты работают;
- ошибки возвращаются в ожидаемом формате.

---

# 28. Что не проверяется текущими тестами

Даже если все текущие тесты проходят, это не означает, что проверено абсолютно всё.

Текущие тесты могут не покрывать:

- performance под нагрузкой;
- race conditions при одновременной публикации нескольких версий;
- реальные Traefik headers;
- авторизацию и scopes, если они появятся позже;
- production observability;
- реальные сценарии большого количества workspaces;
- долгоживущие миграции данных;
- совместимость с будущими версиями frontend;
- все возможные edge cases по Node-структурам Retell AI.

Это нормально. Эти проверки можно добавлять отдельными слоями позже.

---

# 29. Что обязательно добавить при расширении сервиса

Если в будущем добавляются новые endpoints или бизнес-правила, нужно сразу добавить:

1. Unit test на доменную/бизнес-логику.
2. API test через httptest на HTTP contract.
3. E2E test, если endpoint зависит от MongoDB или межзапросного состояния.
4. Обновление `docs/tests/manual-api-smoke-test.md`, если сценарий важен для ручной проверки.
5. Обновление этого документа.

Правило:

```text
Новая бизнес-логика без теста считается незавершённой.
```

---

# 30. Итоговая схема тестирования

```text
make test
  -> go test ./...
  -> unit tests
  -> httptest API tests
  -> no real MongoDB

make e2e
  -> build Docker images
  -> start mongo-test replica set
  -> start agents-service-test
  -> run go test ./...
  -> run go test ./tests/e2e -tags=e2e
  -> real HTTP
  -> real test MongoDB
  -> cleanup volumes

make verify
  -> make test
  -> make e2e
```

---

# 31. Краткий эталон того, что тесты должны подтверждать

```text
Runtime API не требует X-Workspace-Id.
Protected API требует X-Workspace-Id.
Создание агента создаёт Agent + ConversationFlow v0 + PublishedWorkflow.
Workflow v0 сразу опубликован.
Frontend получает конкретно запрошенную версию flow.
Frontend response содержит response_engine из URL version.
Runtime получает только опубликованную версию из PublishedWorkflow.
Publish переключает PublishedWorkflow.
Publish обновляет agent.response_engine.
Unpublish удаляет PublishedWorkflow.
Unpublish не очищает agent.response_engine.
После unpublish runtime published-config возвращает 404.
Опубликованную версию нельзя редактировать.
Опубликованную версию нельзя удалить.
Агента с PublishedWorkflow нельзя удалить.
Workflow больше 1000 nodes отклоняется.
Workflow JSON больше 8MB отклоняется.
Template Agents нельзя создать как обычную папку.
E2E использует только тестовую MongoDB.
Production контейнер не запускает E2E при старте.
```

---

# 32. Финальный вывод

Текущая тестовая стратегия правильная для этого этапа.

Она покрывает:

- быструю проверку бизнес-логики через unit tests;
- проверку HTTP contract через `httptest`;
- проверку реальной MongoDB-интеграции через Docker E2E;
- ручной smoke test через curl;
- pre-flight pipeline перед запуском сервиса.

Главное правило на будущее:

```text
Перед запуском сервиса выполнять make verify.
Для production не запускать E2E на старте контейнера.
Для E2E всегда использовать отдельную тестовую MongoDB.
```