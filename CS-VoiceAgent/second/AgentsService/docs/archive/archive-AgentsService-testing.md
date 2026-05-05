# AgentsService: покрытие тестами и API-интеграция

Документ фиксирует, какие тесты есть в сервисе, что они проверяют, как соотносятся с бизнес-сценариями, и как устроены новые `httptest`-тесты без реальной MongoDB.

---

## 1. Существующие тесты (по файлам)

| Файл | Что проверяет | Тип | Реальная MongoDB | HTTP / JSON ответы |
|------|----------------|-----|------------------|---------------------|
| `internal/domain/validation/folder_validator_test.go` | Валидация имени папки (успех и запрет «Template Agents») | Unit | Нет | Нет |
| `internal/domain/validation/workflow_validator_test.go` | Структура workflow: дубликаты id, стартовая нода, превышение размера сериализованного JSON (огромный `instruction.text`) | Unit | Нет | Нет |
| `internal/app/usecase/conversation_flow_usecase_test.go` | `Publish` обновляет PublishedWorkflow и `agent.response_engine`; `DeleteVersion` для опубликованной версии → `cannot_delete_published_version` | Unit | Нет — **fake**-репозитории (`fakeAgentsRepo`, `fakeFlowsRepo`, `fakePublishedRepo`) | Нет |
| `internal/app/usecase/runtime_usecase_test.go` | Runtime config перезаписывает `response_engine` из PublishedWorkflow; при отсутствии PW → `published_workflow_not_found` | Unit | Нет — **fake**-репозитории | Нет |

До добавления слоя `httptest` не было тестов, которые поднимали Gin-router и вызывали реальные маршруты с проверкой JSON.

---

## 2. Чеклист сценариев: было и стало

| Сценарий | Раньше | Сейчас |
|----------|--------|--------|
| Runtime-маршруты не требуют `X-Workspace-Id` | Автотеста не было | Да — `TestAPI_RuntimeVsProtectedWorkspaceHeader` |
| Protected-маршруты требуют заголовок | Автотеста не было | Да — тот же тест (`GET /api/v1/agents` без заголовка → `missing_workspace_id`) |
| `POST /api/v1/agents` создаёт Agent + ConversationFlow v0 + PublishedWorkflow | Только по коду | Да — косвенно через happy-path и runtime `published-config` (без трёх сущностей ответ не сойдётся) |
| `GET .../conversation-flows/{id}?version=0` возвращает `published` и `response_engine` | Нет | Да — `TestAPI_FullHappyPath_AgentFlowAndRuntimeConfig` |
| `POST .../publish` обновляет PublishedWorkflow и `agent.response_engine` | Unit на usecase | Да — unit + **API** `TestAPI_PublishNewVersionUpdatesAgentResponseEngine` |
| `POST .../unpublish` удаляет только PublishedWorkflow, не очищает `agent.response_engine` | E2E не было | Да — `TestAPI_UnpublishPath` (проверка `GET /api/v1/agents/{id}` после unpublish) |
| Runtime `published-config` — склейка Agent + ConversationFlow + PublishedWorkflow | Только unit runtime | Да — API-тест |
| После unpublish runtime `published-config` → `published_workflow_not_found` | Только unit | Да — API-тест |
| Workflow JSON > 8MB → `workflow_size_exceeded` | Unit в `workflow_validator_test` | Да — дополнительно **PATCH по HTTP** (лимит тела в тесте 20MB, валидация 8MB) |
| Опубликованную версию нельзя PATCH | Не через HTTP | Да — `TestAPI_PublishedVersionProtectionAndAgentDeleteBlocked` |
| Опубликованную версию нельзя DELETE | Unit только delete | Да — тот же API-тест |
| Агента с PublishedWorkflow нельзя DELETE | Не было | Да — API-тест |
| Workflow > 1000 nodes → ошибка валидации (`max_items_exceeded`) | Не через HTTP | Да — `TestAPI_Validation_NodeLimitAndWorkflowSize` (после unpublish, чтобы v0 был редактируем) |

---

## 3. Добавленные файлы и тесты

### Код

- **`internal/transport/httpapi/mount.go`** — вынесенная регистрация middleware и маршрутов (`MountAPI`). Используется из **`router.go`** и из тестов.
- **`internal/transport/httpapi/memory_store_api_test.go`** — in-memory реализации `repo.FolderRepository`, `AgentRepository`, `ConversationFlowRepository`, `PublishedWorkflowRepository` и `memTx` (`WithTransaction` вызывает колбэк без реальной БД).

### API-тесты (`httptest`)

Файл: **`internal/transport/httpapi/api_integration_test.go`**

| Тест | Назначение |
|------|------------|
| `TestAPI_RuntimeVsProtectedWorkspaceHeader` | **A:** `GET /api/v1/runtime/workspaces/published-agents` без `X-Workspace-Id` → 200; `GET /api/v1/agents` без заголовка → 400 `missing_workspace_id` |
| `TestAPI_FullHappyPath_AgentFlowAndRuntimeConfig` | **B:** `POST /agents`, список версий, `GET` flow `version=0` (`published`, `response_engine`), runtime `published-config` с тремя блоками в `data` |
| `TestAPI_UnpublishPath` | **C:** unpublish → `GET` flow v0 с `published=false` → сохранение `response_engine` у агента → runtime `published-config` → 404 `published_workflow_not_found` |
| `TestAPI_PublishedVersionProtectionAndAgentDeleteBlocked` | **D:** PATCH опубликованной v0 → `published_version_is_readonly`; DELETE версии → `cannot_delete_published_version`; DELETE агента → `agent_has_published_workflow` |
| `TestAPI_Validation_NodeLimitAndWorkflowSize` | **E:** >1000 nodes → `validation_error` / `max_items_exceeded`; огромный workflow → `workflow_size_exceeded` |
| `TestAPI_PublishNewVersionUpdatesAgentResponseEngine` | Создание v1 из v0, `POST .../publish?version=1`, `GET` агента — `response_engine.version == 1` |

---

## 4. Запуск тестов

Из каталога сервиса:

```bash
go test ./... -count=1
```

Пакет с API-тестами:

```bash
go test ./internal/transport/httpapi/... -count=1 -v
```

---

## 5. Реальная MongoDB

В текущих тестах **реальная MongoDB не используется**.

- **Unit usecase** — ручные fakes в `*_usecase_test.go`.
- **API-интеграция** — полные реализации интерфейсов репозиториев в памяти (`testMemoryStore`), тот же путь: handlers → usecase → репозитории, без драйвера Mongo.

Отдельный прогон против живой БД (например, build tag `integration`, testcontainers или `docker compose` + `MONGO_URI`) в репозитории не настроен; при необходимости его можно добавить поверх `MountAPI` и `NewRouter`.

---

## 6. Примечание по unit-тесту размера workflow

В `internal/domain/validation/workflow_validator_test.go` тест `TestValidateConversationFlow_SizeExceeded` проверяет, что `ValidateConversationFlow` возвращает ошибку на слишком большой JSON; отдельная проверка кода `workflow_size_exceeded` в нём не зафиксирована — этот код явно проверяется в **`TestAPI_Validation_NodeLimitAndWorkflowSize`** на уровне HTTP-ответа.
