

# AgentService — единая prompt-документация сервиса

> Этот документ является итоговой единой документацией и одновременно мастер-промптом для реализации `AgentService`.
>
> Его назначение:
>
> 1. Зафиксировать, что именно должен реализовывать сервис.
> 2. Зафиксировать архитектурные правила, модели данных, API-контракт и бизнес-логику.
> 3. Сохранить итоговую версию требований после всех доработок по runtime API, публикации workflow, `response_engine`, валидации, лимитам и проверкам.
> 4. Использовать этот документ как шаблон для разработки следующих микросервисов в таком же стиле.
>
> Документ написан так, чтобы по нему можно было восстановить логику сервиса без переписки, черновиков и промежуточных уточнений.

---

# 1. Роль исполнителя

Ты — профессиональный Go Senior / Architect высшего уровня с 10+ летним опытом создания высоконагруженных микросервисов.

Ты всегда выбираешь:

- максимально чистые решения;
- простую поддерживаемую архитектуру;
- предсказуемый код;
- строгую валидацию данных;
- минимальное количество способов сделать одну и ту же вещь;
- отсутствие over-engineering;
- понятные контракты API;
- явные бизнес-правила;
- изоляцию данных по workspace;
- читаемые комментарии на русском языке.

Ты строго следуешь принципам:

- Clean Architecture;
- Lean Architecture;
- HTTP-only microservice communication;
- repository interfaces;
- единый формат ошибок;
- явные DTO для request/response;
- запрет смешивания транспортного слоя, бизнес-логики и работы с БД.

Твоя задача — полностью реализовать сервис `AgentService` согласно этому документу.

---

# 2. Назначение сервиса

`AgentService` — центральный сервис управления AI-агентами и их Conversation Flow.

Сервис отвечает за:

- создание AI-агентов;
- хранение AI-агентов;
- редактирование настроек AI-агентов;
- удаление AI-агентов;
- создание ConversationFlow версии `0` при создании агента;
- хранение ConversationFlow;
- редактирование ConversationFlow;
- версионирование ConversationFlow;
- публикацию выбранной версии ConversationFlow;
- снятие публикации;
- выдачу опубликованной runtime-конфигурации для микросервиса звонков;
- выдачу списков опубликованных агентов для runtime-сервисов;
- работу с папками агентов;
- валидацию пользовательского контента workflow;
- защиту опубликованных версий от случайного изменения;
- поддержку frontend/editor API и runtime API.

Сервис является частью большой микросервисной архитектуры.

Сервис общается только по HTTP.

Сервис не должен ходить в другие сервисы.

Сервис не должен выполнять логику звонков, LLM-инференса, TTS, STT или исполнения ConversationFlow. Он только хранит и отдаёт конфигурации.

---

# 3. Главная идея домена

В системе есть три основные сущности:

1. `Agent` — текущие настройки AI-агента.
2. `ConversationFlow` — версия логики разговора агента.
3. `PublishedWorkflow` — указатель на опубликованную версию workflow для агента.

Важно понимать разделение ответственности:

- `Agent` не версионируется.
- `ConversationFlow` версионируется.
- `PublishedWorkflow` является единственным источником истины для production/runtime запуска.
- `agent.response_engine` является удобной денормализованной копией и response-полем, но runtime-запуск не должен полагаться только на него.

---

# 4. Ключевые бизнес-правила

## 4.1. Agent не версионируется

`Agent` хранит текущие настройки агента:

- имя;
- канал;
- голос;
- язык;
- TTS;
- STT;
- чувствительность к перебиванию;
- длительность звонка;
- DTMF;
- настройки персональных данных;
- настройки хранения данных;
- ссылку на workflow через `response_engine`.

Если пользователь изменяет настройки агента через `PATCH /agents/{agentId}`, изменения применяются сразу ко всем последующим запускам агента.

Это нормальное поведение, потому что версионируется только логика разговора, то есть `ConversationFlow`.

Пример:

- опубликована версия workflow `v1`;
- пользователь изменил `tts.voice_id` у агента;
- следующий звонок будет использовать опубликованный workflow `v1`, но уже с новым голосом.

## 4.2. ConversationFlow версионируется

`ConversationFlow` хранит логику разговора:

- nodes;
- transitions;
- prompts;
- start node;
- model settings;
- flex mode;
- KB/RAG config;
- display positions;
- глобальные настройки нод.

Пользователь может:

- создать новую версию из существующей;
- редактировать неопубликованную версию;
- тестировать неопубликованную версию;
- опубликовать нужную версию.

Пока новая версия не опубликована, production/runtime сервисы продолжают использовать ранее опубликованную версию.

## 4.3. PublishedWorkflow — источник истины публикации

В `ConversationFlow` не хранится поле `is_published`.

Статус публикации не хранится внутри документа `ConversationFlow`.

Единственный источник истины публикации — отдельная коллекция `PublishedWorkflow`.

Это нужно для того, чтобы:

- у одного агента была только одна опубликованная версия;
- не было рассинхронизации между разными версиями workflow;
- быстро получать опубликованный workflow без перебора всех версий;
- runtime-сервис мог быстро получить production-ready конфигурацию.

`PublishedWorkflow` хранит:

- `workspace_id`;
- `agent_id`;
- `conversation_flow_id`;
- `version`;
- `published_at`;
- `created_at`;
- `updated_at`.

На коллекции `PublishedWorkflow` должен быть уникальный индекс:

```text
workspace_id + agent_id
```

Это гарантирует, что один агент в одном workspace не может иметь больше одной опубликованной версии workflow.

## 4.4. response_engine имеет разный контекст

`response_engine` — объект движка ответа.

Для этого сервиса `response_engine.type` всегда:

```json
"conversation-flow"
```

Но поля `conversation_flow_id` и `version` зависят от контекста response.

### Frontend/editor context

Когда frontend запрашивает конкретную версию workflow:

```http
GET /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=N
```

response должен содержать:

```json
"response_engine": {
  "type": "conversation-flow",
  "conversation_flow_id": "<conversationFlowId из URL>",
  "version": N
}
```

Даже если эта версия не опубликована.

Это нужно, чтобы frontend/editor понимал, с какой конкретной версией flow он сейчас работает.

### Runtime context

Когда микросервис звонков запрашивает production-ready конфиг:

```http
GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
```

`response_engine` должен заполняться строго из `PublishedWorkflow`:

```json
"response_engine": {
  "type": "conversation-flow",
  "conversation_flow_id": "<из PublishedWorkflow>",
  "version": "<из PublishedWorkflow>"
}
```

Runtime никогда не передаёт version сам.

Runtime не выбирает версию.

AgentService сам выбирает опубликованную версию через `PublishedWorkflow`.

---

# 5. Технологический стек

- Go, последняя стабильная версия.
- MongoDB.
- Официальный MongoDB Go Driver: `go.mongodb.org/mongo-driver`.
- Gin для HTTP API.
- Viper для конфигурации.
- Zerolog для логирования.
- Testify + стандартный `go test`.
- Docker / Docker Compose для локального и тестового окружения.

Важно: MongoDB transactions требуют replica set. В тестовом Docker Compose MongoDB должна запускаться как однонодовый replica set, например `--replSet rs0`.

---

# 6. Архитектура проекта

Сервис должен быть реализован по Clean Architecture с Lean-подходом.

Главное правило зависимостей:

```text
handler -> usecase -> repository interface -> repository implementation
```

## 6.1. Запрещено

- вызывать MongoDB напрямую из HTTP handlers;
- размещать бизнес-логику публикации в handlers;
- размещать бизнес-логику версионирования в handlers;
- размещать бизнес-логику удаления в handlers;
- размещать валидацию доменной модели только в handlers;
- использовать DTO HTTP-запросов как доменные модели без валидации;
- смешивать transport, business logic и database logic в одном пакете;
- делать несколько альтернативных способов выполнить одну и ту же операцию;
- писать сложные универсальные абстракции без необходимости.

## 6.2. Разрешено

- держать структуру простой;
- использовать интерфейсы только там, где они реально отделяют бизнес-логику от инфраструктуры;
- использовать готовые библиотеки вместо написания своих решений;
- выносить повторяющийся код в helper-функции;
- разделять DTO, domain models и persistence models.

---

# 7. Рекомендуемая структура проекта

```text
cmd/agent-service/
  main.go

internal/
  config/
    config.go

  transport/httpapi/
    router.go
    mount.go
    middleware/
      workspace.go
      body_limit.go
    handlers/
      api.go
      agents.go
      folders.go
      conversation_flows.go
      runtime.go

  app/
    usecase/
      agent_usecase.go
      folder_usecase.go
      conversation_flow_usecase.go
      runtime_usecase.go
      publication_usecase.go

  domain/
    models/
      agent.go
      folder.go
      conversation_flow.go
      published_workflow.go
      node.go
    errors/
      api_error.go
      validation_error.go
      business_error.go
    validation/
      limits.go
      agent_validator.go
      folder_validator.go
      workflow_validator.go

  repository/
    interfaces/
      agent_repository.go
      folder_repository.go
      conversation_flow_repository.go
      published_workflow_repository.go
      transaction_manager.go
    mongo/
      indexes.go
      agent_repository.go
      folder_repository.go
      conversation_flow_repository.go
      published_workflow_repository.go
      transaction_manager.go

  templates/
    agent_template.go
    default_conversation_flow.go

  logger/
    logger.go

  server/
    server.go

docs/
  AgentsService-doc.md
  AgentsService-testing.md
  tests/manual-api-smoke-test.md
  tests/e2e-verify-guide.md
```

Названия пакетов могут отличаться, но ответственность слоёв должна сохраняться.

---

# 8. Ответственность слоёв

## 8.1. transport/httpapi

Отвечает только за HTTP:

- регистрацию routes;
- middleware;
- чтение headers;
- чтение path/query params;
- парсинг request body;
- вызов usecase;
- возврат JSON response;
- преобразование ошибок в HTTP response.

Handlers не должны содержать бизнес-логику.

## 8.2. app/usecase

Содержит бизнес-логику сервиса:

- создание агента;
- создание базового workflow;
- создание новой версии workflow;
- публикация workflow;
- снятие публикации;
- запрет редактирования опубликованной версии;
- запрет удаления опубликованной версии;
- запрет удаления агента с опубликованным workflow;
- удаление агента;
- перемещение агента между папками;
- сборка runtime published-config;
- получение списков опубликованных агентов;
- orchestration нескольких repository calls.

Usecase работает только через repository interfaces.

## 8.3. domain/models

Содержит чистые доменные модели:

- `Agent`;
- `Folder`;
- `ConversationFlow`;
- `PublishedWorkflow`;
- `Node`;
- вложенные структуры Agent;
- вложенные структуры ConversationFlow;
- response_engine.

Доменные модели не должны зависеть от Gin, HTTP, MongoDB или внешней инфраструктуры.

## 8.4. domain/validation

Содержит валидацию:

- лимитов строк;
- лимитов массивов;
- лимитов числовых полей;
- уникальности Node.id;
- корректности start_node_id;
- корректности destination_node_id;
- enum variables;
- transcript roles;
- размера JSON workflow;
- запрета `Template Agents` как обычной папки.

Все ошибки валидации должны создаваться через общий helper.

## 8.5. repository/interfaces

Содержит интерфейсы репозиториев, которые используются usecase-слоем.

Usecase не должен знать, что под ним MongoDB.

## 8.6. repository/mongo

Содержит реализацию repository interfaces через MongoDB Go Driver.

Только этот слой имеет право напрямую работать с MongoDB.

## 8.7. templates

Содержит базовый шаблон агента и базовый шаблон ConversationFlow.

Шаблон должен быть легко редактируемым без изменения бизнес-логики сервиса.

---

# 9. Комментарии в коде

Код должен быть подробно покрыт комментариями на русском языке.

Комментарии должны объяснять не только что делает код, но и почему выбран именно такой подход.

Обязательно комментировать:

- доменные модели;
- DTO request/response;
- repository interfaces;
- MongoDB repository implementations;
- usecase-методы;
- валидаторы;
- helper-функции ошибок;
- middleware чтения `X-Workspace-Id`;
- middleware лимита тела запроса;
- runtime routes;
- бизнес-правила публикации workflow;
- бизнес-правила снятия публикации;
- запрет редактирования опубликованной версии;
- запрет удаления опубликованной версии;
- запрет удаления агента с опубликованным workflow;
- создание агента с дефолтным workflow-шаблоном;
- создание новой версии workflow через `fromVersion`;
- вычисление поля `published` при выдаче списка версий;
- сборку runtime published-config;
- причину использования PublishedWorkflow как источника истины;
- причину, почему `ConversationFlow` не хранит `is_published`.

Пример хорошего комментария:

```go
// PublishWorkflow публикует выбранную версию workflow для агента.
//
// В ConversationFlow не хранится поле is_published. Единственным источником истины
// является коллекция PublishedWorkflow. Такой подход исключает рассинхронизацию между
// несколькими версиями workflow и гарантирует, что у одного агента может быть только
// одна опубликованная версия.
func (uc *ConversationFlowUseCase) PublishWorkflow(ctx context.Context, input PublishWorkflowInput) error {
    // ...
}
```

Пример плохого комментария:

```go
// Публикуем workflow
func (uc *ConversationFlowUseCase) PublishWorkflow(ctx context.Context, input PublishWorkflowInput) error {
    // ...
}
```

---

# 10. Транзакционность и консистентность

Операции, которые меняют несколько коллекций, должны выполняться атомарно на уровне бизнес-логики.

К таким операциям относятся:

- создание агента + создание workflow v0 + создание PublishedWorkflow;
- публикация workflow + обновление PublishedWorkflow + обновление `agent.response_engine`;
- снятие публикации + удаление PublishedWorkflow;
- удаление агента + удаление всех workflow-версий;
- удаление папки + сброс `folder_id` у агентов этой папки.

Если MongoDB transactions доступны, использовать transaction/session.

Если transactions недоступны, код должен быть написан так, чтобы минимизировать риск рассинхронизации и возвращать понятную ошибку при частичном сбое.

Важно: для MongoDB transactions требуется replica set. Даже локальная тестовая MongoDB в Docker должна запускаться как однонодовый replica set.

---

# 11. Headers и группы API

## 11.1. Frontend/admin API

Обычные frontend/admin endpoints требуют header:

```http
X-Workspace-Id: <workspace_id>
```

Дополнительно через Traefik могут приходить:

```http
X-User-Id: <user_id>
X-User-Scopes: <scopes>
```

Но обязательным для изоляции данных является `X-Workspace-Id`.

Если `X-Workspace-Id` отсутствует, сервис должен вернуть:

```json
{
  "error": {
    "type": "validation_error",
    "field": "X-Workspace-Id",
    "code": "missing_workspace_id",
    "message": "X-Workspace-Id header is required",
    "limit": null
  }
}
```

HTTP status:

```text
400 Bad Request
```

## 11.2. Runtime API

Runtime endpoints предназначены для внутреннего микросервиса звонков.

Они не требуют пользовательских headers:

- `X-Workspace-Id`;
- `X-User-Id`;
- `X-User-Scopes`.

Runtime endpoints получают `workspace_id` и `agent_id` из path params.

Runtime endpoints должны быть отделены от protected frontend/admin routes.

Пример структуры router:

```go
api := r.Group("/api/v1")

runtime := api.Group("/runtime")
// runtime routes без RequireWorkspaceID

protected := api.Group("")
protected.Use(middleware.RequireWorkspaceID())
// frontend/admin routes
```

Важно по безопасности: runtime API не должен быть публично открыт без защиты на уровне инфраструктуры. В production его нужно закрывать внутренней сетью, Traefik rules, service-to-service token или mTLS. Пользовательские headers не требуются, но это не значит, что endpoint должен быть доступен всему интернету.

---

# 12. Стандартизированный формат обновления

Обновление workflow делается только через PATCH.

Обновление делается только top-level полями.

Никаких точечных обновлений внутри нод.

Если поле не пришло в PATCH body, оно не обновляется.

Если поле пришло, оно заменяет соответствующее поле целиком.

Пример PATCH workflow:

```json
{
  "nodes": [
    { "полная нода": 0 },
    { "полная нода": 1 }
  ],
  "global_prompt": "новый текст",
  "model_choice": {},
  "start_node_id": "xxx",
  "start_speaker": "agent",
  "flex_mode": true
}
```

Если `nodes` пришёл, весь массив nodes заменяется целиком.

Если `model_choice` пришёл, весь объект `model_choice` заменяется целиком.

Если `kb_config` пришёл, весь объект `kb_config` заменяется целиком.

Если `tts` пришёл в `PATCH /agents/{agentId}`, весь объект `tts` заменяется целиком.

Если нужно изменить только `tts.speed`, frontend должен отправить полный объект `tts`.

---

# 13. Правило симметрии request/response

Для endpoints создания и обновления request body должен повторять структуру соответствующего ресурса из response.

Главное правило:

- `POST` может принимать часть полной структуры ресурса + использовать backend defaults.
- `PATCH` принимает ту же структуру ресурса, что возвращается в response, но все поля опциональны.
- В PATCH клиент передаёт только объект или поле, которое хочет изменить.
- Response после `POST` и `PATCH` возвращает полную актуальную структуру созданного или обновлённого ресурса, кроме action endpoints.

Примеры:

- `GET /agents/{agentId}` возвращает полный Agent.
- `PATCH /agents/{agentId}` принимает частичный Agent и возвращает полный обновлённый Agent.
- `GET /agents/{agentId}/conversation-flows/{conversationFlowId}?version=1` возвращает полный ConversationFlow.
- `PATCH /agents/{agentId}/conversation-flows/{conversationFlowId}?version=1` принимает частичный ConversationFlow и возвращает полный обновлённый ConversationFlow.

Лёгкие списки могут возвращать сокращённые объекты:

- `GET /agents` возвращает лёгкий список агентов.
- `GET /agents/{agentId}/conversation-flows` возвращает лёгкий список версий workflow без полного массива nodes.

---

# 14. Folder

## 14.1. Назначение

`Folder` — обычная папка для группировки агентов в рамках workspace.

Папка не является владельцем агентов.

Источник истины принадлежности агента к папке — поле:

```text
Agent.folder_id
```

В `Folder` не хранится массив `agentIds`.

Это сделано для простоты и чтобы избежать рассинхронизации данных.

## 14.2. Виртуальная папка Template Agents

`Template Agents` — виртуальная папка.

Она не хранится в MongoDB.

Все агенты всегда отображаются в `Template Agents`, независимо от `folder_id`.

Если у агента указан `folder_id`, он отображается одновременно:

- в своей обычной папке;
- в виртуальной папке `Template Agents`.

Папку `Template Agents` нельзя:

- создать;
- переименовать;
- удалить через API.

При удалении обычной папки у всех агентов из неё сбрасывается `folder_id`.

После этого агенты остаются отображаться в виртуальной папке `Template Agents`.

## 14.3. Структура Folder

```jsonc
{
  "folder_id": "string",        // обязательно, префикс "folder_"
  "workspace_id": "string",     // обязательно, ID workspace
  "name": "string",             // название папки
  "created_at": "number",       // Unix timestamp в миллисекундах
  "updated_at": "number"        // Unix timestamp в миллисекундах
}
```

## 14.4. Пример Folder

```json
{
  "folder_id": "folder_5072e604e6063f693c10de4a",
  "workspace_id": "ws_abc123xyz",
  "name": "Sales Agents",
  "created_at": 1774384312548,
  "updated_at": 1777982029068
}
```

## 14.5. Перемещение агента в папку

Перемещение делается через:

```http
PATCH /api/v1/agents/{agentId}
```

Body:

```json
{
  "folder_id": "folder_xxx"
}
```

Правила:

- если `folder_id` отсутствует, папка агента не изменяется;
- если `folder_id: null`, у агента сбрасывается привязка к обычной папке;
- если `folder_id` передан, агент перемещается в указанную папку;
- если папка не найдена в текущем workspace, вернуть `folder_not_found`.

---

# 15. Agent

## 15.1. Назначение Agent

`Agent` — сущность AI-агента.

Agent хранит текущие настройки, которые применяются ко всем последующим запускам агента.

Agent не хранит версии.

Agent не содержит историю изменений workflow.

Agent не исполняет workflow.

Agent не содержит полную бизнес-логику разговора. Она хранится в `ConversationFlow`.

## 15.2. Структура Agent

```jsonc
{
  "agent_id": "string",                    // обязательно, префикс "agent_"
  "workspace_id": "string",                // обязательно, ID workspace
  "folder_id": "string | null",            // null = нет обычной папки

  "name": "string",                        // название агента
  "channel": "voice" | "chat",             // тип канала

  "voice_id": "string",                    // основной voice_id агента
  "language": "string",                    // основной язык агента

  "tts": {
    "voice_id": "string",
    "language": "string",
    "emotion": "string | null",
    "speed": "number | null"
  },

  "stt": {
    "model_id": "string",
    "language": "string"
  },

  "interruption_sensitivity": "number",
  "max_call_duration_ms": "number",
  "normalize_for_speech": "boolean",
  "allow_user_dtmf": "boolean",
  "user_dtmf_options": {},

  "response_engine": {
    "type": "conversation-flow",
    "conversation_flow_id": "string",
    "version": "number"
  },

  "handbook_config": {
    "default_personality": "boolean",
    "ai_disclosure": "boolean"
  },

  "pii_config": {
    "mode": "post_call" | "none",
    "categories": []
  },

  "data_storage_setting": "everything" | "none" | "metadata_only",

  "last_modified": "number",
  "created_at": "number",
  "updated_at": "number"
}
```

## 15.3. Поля Agent

### agent_id

Уникальный идентификатор агента.

Генерируется автоматически.

Должен иметь префикс:

```text
agent_
```

### workspace_id

Идентификатор workspace.

Все операции с Agent должны фильтроваться по `workspace_id`.

Нельзя вернуть или изменить агента из другого workspace.

### folder_id

Может быть `null`.

Если `null`, агент не привязан к обычной папке, но всё равно отображается в виртуальной папке `Template Agents`.

### name

Название агента, которое видит пользователь.

Максимум 255 символов.

### channel

Тип канала:

- `voice`;
- `chat`.

### voice_id и language

Основные настройки голоса и языка агента.

### tts

Настройки Text-To-Speech.

Хранятся только на уровне Agent.

Обновляются через:

```http
PATCH /api/v1/agents/{agentId}
```

Не входят в ConversationFlow.

Не версионируются вместе с workflow.

### stt

Настройки Speech-To-Text.

Хранятся только на уровне Agent.

Обновляются через:

```http
PATCH /api/v1/agents/{agentId}
```

Не входят в ConversationFlow.

### interruption_sensitivity

Чувствительность к перебиванию пользователем.

Диапазон:

```text
0.0 - 1.0
```

### max_call_duration_ms

Максимальная длительность звонка в миллисекундах.

Диапазон:

```text
60000 - 14400000
```

### response_engine

Объект движка ответа.

В обычном Agent response может отражать опубликованную связку.

В frontend/editor response для конкретной версии flow должен соответствовать явно запрошенным `conversationFlowId` и `version`.

В runtime response должен строго соответствовать `PublishedWorkflow`.

### handbook_config

Настройки поведения агента:

- дефолтная личность;
- раскрытие факта общения с AI.

### pii_config

Настройки обработки персональных данных.

### data_storage_setting

Политика хранения данных разговоров.

Возможные значения:

- `everything`;
- `none`;
- `metadata_only`.

---

# 16. Создание агента и базовый workflow

При создании агента через:

```http
POST /api/v1/agents
```

сервис должен автоматически создать:

1. Документ `Agent`.
2. Начальный `ConversationFlow` версии `0`.
3. Запись `PublishedWorkflow`.
4. Связь с опубликованной версией workflow в `agent.response_engine`.

Пользователь не должен попадать в абсолютно пустого агента.

При создании должен создаваться базовый workflow-шаблон:

- минимальный набор нод;
- стартовая нода;
- глобальный промпт;
- базовые настройки модели;
- begin tag display position;
- default TTS/STT/Agent settings.

Базовый шаблон должен находиться на backend в одном понятном месте:

```text
internal/templates/agent_template.go
internal/templates/default_conversation_flow.go
```

Требования:

- шаблон легко редактируется без изменения бизнес-логики;
- создание агента берёт defaults из шаблона;
- значения из request могут переопределять defaults;
- если значение не передано, используется default;
- базовый workflow сразу публикуется;
- версия создаваемого workflow всегда `0`;
- Agent сразу получает `response_engine` на опубликованную версию.

---

# 17. ConversationFlow

## 17.1. Назначение

`ConversationFlow` хранит версию логики поведения агента.

Это workflow, который frontend может редактировать и который runtime может исполнять после публикации.

## 17.2. Важное правило

`ConversationFlow` не хранит `is_published`.

Статус публикации вычисляется через `PublishedWorkflow`.

В API response может возвращаться поле:

```json
"published": true
```

Но это вычисляемое поле response, а не поле MongoDB-документа `ConversationFlow`.

## 17.3. Структура ConversationFlow

```jsonc
{
  "conversation_flow_id": "string",
  "agent_id": "string",
  "workspace_id": "string",
  "version": "number",

  "nodes": ["Node"],

  "start_node_id": "string",
  "start_speaker": "agent" | "user",

  "global_prompt": "string",

  "model_choice": {
    "type": "cascading",
    "model": "string",
    "high_priority": "boolean"
  },

  "model_temperature": "number",
  "flex_mode": "boolean",
  "tool_call_strict_mode": "boolean",

  "kb_config": {
    "top_k": "number",
    "filter_score": "number"
  },

  "begin_tag_display_position": {
    "x": "number",
    "y": "number"
  },

  "is_transfer_cf": "boolean",

  "created_at": "number",
  "updated_at": "number"
}
```

## 17.4. Поля ConversationFlow

### conversation_flow_id

Уникальный идентификатор workflow.

Генерируется автоматически.

Префикс:

```text
conversation_flow_
```

### agent_id

ID агента, которому принадлежит workflow.

### workspace_id

ID workspace.

Все операции должны проверять workspace_id.

### version

Номер версии.

Начинается с `0`.

При создании новой версии сервис копирует выбранную существующую версию и создаёт новый документ с:

```text
version = max(version) + 1
```

### nodes

Основная логика conversation flow.

Всегда хранится полный массив нод.

При обновлении frontend отправляет полный массив `nodes` целиком.

### start_node_id

ID стартовой ноды.

Должен ссылаться на существующую ноду из `nodes`.

### start_speaker

Кто начинает разговор:

- `agent`;
- `user`.

### global_prompt

Глобальный системный промпт workflow.

Может содержать динамические переменные:

```text
{{current_time}}
{{user_name}}
```

Сервис не подставляет эти переменные.

Сервис хранит их как обычный текст.

### model_choice

Выбор LLM-модели.

### model_temperature

Температура модели.

Диапазон:

```text
0.0 - 2.0
```

### flex_mode

Гибкий режим, позволяющий вызывать глобальные ноды из любой точки графа.

### tool_call_strict_mode

Строгий режим вызова инструментов.

### kb_config

Настройки базы знаний / Retrieval / RAG.

### begin_tag_display_position

Позиция стартового блока на canvas frontend.

### is_transfer_cf

Признак workflow для передачи звонка.

---

# 18. Node

Каждая нода в массиве `nodes` должна быть максимально близка к формату Retell AI.

Сервис обязан хранить и возвращать Node в том виде, в котором её отправляет frontend.

## 18.1. Структура Node

```jsonc
{
  "id": "string",
  "type": "string",
  "name": "string",

  "instruction": {
    "type": "static_text" | "prompt",
    "text": "string"
  },

  "edges": [
    {
      "id": "string",
      "destination_node_id": "string",
      "transition_condition": {
        "type": "prompt",
        "prompt": "string"
      }
    }
  ],

  "global_node_setting": {
    "condition": "string",
    "go_back_conditions": [
      {
        "id": "string",
        "transition_condition": {
          "type": "prompt",
          "prompt": "string"
        }
      }
    ],
    "positive_finetune_examples": [
      {
        "transcript": [
          { "role": "user", "content": "..." },
          { "role": "agent", "content": "..." }
        ]
      }
    ],
    "negative_finetune_examples": [
      {
        "transcript": [
          { "role": "user", "content": "..." },
          { "role": "agent", "content": "..." }
        ]
      }
    ],
    "cool_down": "number"
  },

  "variables": [
    {
      "name": "string",
      "description": "string",
      "type": "string" | "number" | "boolean" | "enum",
      "choices": ["string"]
    }
  ],

  "voice_speed": "number",
  "responsiveness": "number",

  "finetune_conversation_examples": [
    {
      "transcript": [
        { "role": "user", "content": "..." },
        { "role": "agent", "content": "..." }
      ]
    }
  ],

  "finetune_transition_examples": [
    {
      "id": "string",
      "transcript": [
        { "role": "user", "content": "..." },
        { "role": "agent", "content": "..." }
      ],
      "destination_node_id": "string"
    }
  ],

  "display_position": {
    "x": "number",
    "y": "number"
  },

  "start_speaker": "agent" | "user"
}
```

## 18.2. Правила Node

- Динамические переменные внутри `instruction.text` — это просто текст.
- Сервис не обрабатывает `{{current_time}}` и другие переменные.
- При обновлении workflow frontend присылает полный массив nodes целиком.
- Все `Node.id` внутри одного workflow должны быть уникальными.
- `start_node_id` должен ссылаться на существующую ноду.
- `edges[].destination_node_id`, если передан, должен ссылаться на существующую ноду.

---

# 19. PublishedWorkflow

## 19.1. Назначение

`PublishedWorkflow` — отдельная коллекция для быстрого получения опубликованной версии workflow.

Это главный источник истины для runtime запуска агента.

## 19.2. Структура

```jsonc
{
  "workspace_id": "string",
  "agent_id": "string",
  "conversation_flow_id": "string",
  "version": "number",
  "published_at": "number",
  "created_at": "number",
  "updated_at": "number"
}
```

## 19.3. Индекс

Уникальный индекс:

```text
workspace_id + agent_id
```

## 19.4. Зачем хранить conversation_flow_id и version

`agent_id` нужен для быстрого поиска опубликованного workflow конкретного агента.

`conversation_flow_id` нужен для прямого получения документа workflow.

`version` нужен, потому что у одного workflow есть несколько версий.

`workspace_id` нужен для изоляции данных.

---

# 20. Публикация workflow

Публикация workflow означает, что выбранная версия `ConversationFlow` становится production/runtime версией для агента.

Endpoint:

```http
POST /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/publish?version=N
```

Алгоритм:

1. Проверить, что Agent принадлежит текущему workspace.
2. Проверить, что ConversationFlow принадлежит этому Agent и workspace.
3. Проверить, что указанная version существует.
4. Обновить или создать `PublishedWorkflow` для `workspace_id + agent_id`.
5. Обновить `agent.response_engine` как денормализованную копию PublishedWorkflow.
6. Обновить `agent.updated_at` и `agent.last_modified`.

После успешной публикации:

- `PublishedWorkflow` указывает на выбранную версию;
- `agent.response_engine` содержит тот же `conversation_flow_id` и `version`;
- frontend список версий показывает `published=true` только у выбранной версии;
- runtime `published-config` отдаёт выбранную версию.

---

# 21. Снятие публикации

Endpoint:

```http
POST /api/v1/agents/{agentId}/conversation-flows/unpublish
```

Снятие публикации удаляет только запись `PublishedWorkflow`.

`agent.response_engine` не очищается автоматически.

Это осознанное правило.

Причина:

- production/runtime запуск всё равно не должен брать версию из `agent.response_engine` как из источника истины;
- runtime должен искать `PublishedWorkflow`;
- если `PublishedWorkflow` отсутствует, production-запуск невозможен;
- frontend/editor endpoints продолжают работать по явно запрошенной версии flow.

После unpublish:

- runtime `published-config` возвращает `404 published_workflow_not_found`;
- frontend может открывать версии workflow по `conversationFlowId + version`;
- все версии в списке имеют `published=false`;
- агента можно удалить, если нет PublishedWorkflow.

---

# 22. Запреты для опубликованной версии

Опубликованную версию ConversationFlow нельзя редактировать.

Опубликованную версию ConversationFlow нельзя удалить.

Агента с PublishedWorkflow нельзя удалить.

Правильный сценарий изменения опубликованного workflow:

1. Создать новую версию из опубликованной через `fromVersion`.
2. Отредактировать новую неопубликованную версию.
3. Протестировать новую версию.
4. Опубликовать новую версию.
5. После этого старая версия становится неопубликованной и может быть удалена.

Ошибки:

- `published_version_is_readonly`;
- `cannot_delete_published_version`;
- `agent_has_published_workflow`.

---

# 23. Runtime API для микросервиса звонков

Runtime API предназначен только для внутреннего микросервиса звонков.

Runtime API отдаёт production-ready данные.

Runtime API не требует пользовательские headers.

Runtime API не должен использовать request version.

Runtime API выбирает version только через `PublishedWorkflow`.

## 23.1. Список всех workspace с опубликованными агентами

```http
GET /api/v1/runtime/workspaces/published-agents
```

Назначение:

Получить список workspace, у которых есть опубликованные агенты.

Источник данных:

```text
PublishedWorkflow
```

Response:

```json
{
  "data": [
    {
      "workspace_id": "ws_abc123xyz",
      "agents": [
        {
          "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
          "conversation_flow_id": "conversation_flow_c530702321b8",
          "version": 2,
          "published_at": 1777987000000
        }
      ]
    }
  ]
}
```

Правила:

- брать данные только из PublishedWorkflow;
- возвращать только workspace с минимум одним published agent;
- не возвращать полный Agent;
- не возвращать полный ConversationFlow.

## 23.2. Список опубликованных агентов workspace

```http
GET /api/v1/runtime/workspaces/{workspaceId}/published-agents
```

Response:

```json
{
  "data": [
    {
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 2,
      "published_at": 1777987000000
    }
  ]
}
```

## 23.3. Published config агента

```http
GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
```

Это главный endpoint для микросервиса звонков.

Алгоритм:

1. Найти Agent по `workspace_id + agent_id`.
2. Найти PublishedWorkflow по `workspace_id + agent_id`.
3. Из PublishedWorkflow взять `conversation_flow_id` и `version`.
4. Найти ConversationFlow по `workspace_id + agent_id + conversation_flow_id + version`.
5. В response перезаписать `agent.response_engine` значениями из PublishedWorkflow.
6. Вернуть склеенный JSON.

Response:

```json
{
  "data": {
    "agent": {},
    "conversation_flow": {
      "published": true
    },
    "published_workflow": {}
  }
}
```

Ошибки:

- `agent_not_found`;
- `published_workflow_not_found`;
- `conversation_flow_not_found`.

---

# 24. Валидация пользовательского контента workflow

Пользователь может редактировать:

- названия нод;
- prompts;
- transitions;
- условия переходов;
- правила вызова глобальных нод;
- описания переменных;
- примеры диалогов;
- реплики;
- display positions;
- model settings.

Все эти данные должны валидироваться на backend.

Валидация должна быть централизованной.

Нельзя писать отдельную функцию ошибки под каждый лимит.

Должен быть единый helper ошибок:

```go
func ValidationError(field string, code string, message string, limit any) error
```

Ответ API при ошибке:

```json
{
  "error": {
    "type": "validation_error",
    "field": "nodes[0].instruction.text",
    "code": "max_length_exceeded",
    "message": "Instruction text must be no longer than 50000 characters",
    "limit": 50000
  }
}
```

---

# 25. Лимиты

## 25.1. Workspace limits

На один workspace:

- максимум 100 агентов;
- максимум 50 обычных папок;
- максимум 100 версий workflow на одного agent;
- максимум 1000 нод в одном workflow;
- максимальный размер одного workflow после сериализации в JSON — 8 MB.

## 25.2. Body size

Максимальный размер JSON-тела запроса:

```text
8 MB
```

В E2E окружении может быть установлен больший HTTP body limit, например `20 MB`, чтобы запрос дошёл до workflow validator и можно было проверить `workflow_size_exceeded`.

## 25.3. String limits

- `Agent.name` — максимум 255 символов.
- `Folder.name` — максимум 255 символов.
- `Node.name` — максимум 255 символов.
- `ConversationFlow.global_prompt` — максимум 50 000 символов.
- `Node.instruction.text` — максимум 50 000 символов.
- `Node.edges[].transition_condition.prompt` — максимум 10 000 символов.
- `Node.global_node_setting.condition` — максимум 10 000 символов.
- `Node.global_node_setting.go_back_conditions[].transition_condition.prompt` — максимум 10 000 символов.
- `Node.variables[].name` — максимум 128 символов.
- `Node.variables[].description` — максимум 2 000 символов.
- `Node.variables[].choices[]` — максимум 255 символов на один вариант enum.
- `transcript[].content` — максимум 10 000 символов на одну реплику.

## 25.4. Array limits

- `nodes` — максимум 1000 нод в одном workflow.
- `Node.edges` — максимум 50 переходов на одну ноду.
- `Node.variables` — максимум 100 переменных на одну ноду.
- `Node.variables[].choices` — максимум 100 вариантов enum.
- `Node.finetune_conversation_examples` — максимум 50 примеров на одну ноду.
- `Node.finetune_transition_examples` — максимум 50 примеров на одну ноду.
- `Node.global_node_setting.positive_finetune_examples` — максимум 50 примеров на одну ноду.
- `Node.global_node_setting.negative_finetune_examples` — максимум 50 примеров на одну ноду.
- `transcript` внутри любого примера — максимум 100 реплик.

## 25.5. Numeric limits

- `Agent.interruption_sensitivity` — от 0.0 до 1.0.
- `Agent.max_call_duration_ms` — от 60 000 до 14 400 000.
- `Agent.tts.speed` — от 0.5 до 2.0, если поле передано.
- `ConversationFlow.model_temperature` — от 0.0 до 2.0.
- `ConversationFlow.kb_config.top_k` — от 1 до 20.
- `ConversationFlow.kb_config.filter_score` — от 0.0 до 1.0.
- `Node.voice_speed` — от 0.5 до 2.0, если поле передано.
- `Node.responsiveness` — от 0.0 до 1.0, если поле передано.
- `Node.global_node_setting.cool_down` — от 0 до 100.
- `display_position.x`, `display_position.y`, `begin_tag_display_position.x`, `begin_tag_display_position.y` — от -100000 до 100000.

## 25.6. Workflow JSON size

После применения PATCH или создания версии итоговый `ConversationFlow` должен сериализоваться в JSON.

Размер JSON не должен превышать:

```text
8 MB = 8388608 bytes
```

Проверка:

```go
json.Marshal(conversationFlow)
len(bytes) <= MaxWorkflowJSONBytes
```

При превышении:

```json
{
  "error": {
    "type": "validation_error",
    "field": "workflow",
    "code": "workflow_size_exceeded",
    "message": "Workflow JSON size must be no larger than 8 MB",
    "limit": 8388608
  }
}
```

---

# 26. Обязательные проверки структуры workflow

- Все `Node.id` внутри одного workflow уникальны.
- `start_node_id` ссылается на существующую ноду.
- `edges[].destination_node_id`, если передан, ссылается на существующую ноду.
- `Node.type` не пустой.
- `Node.name` не пустой.
- `Node.instruction.type` только `static_text` или `prompt`, если instruction передан.
- `start_speaker` только `agent` или `user`.
- `transcript[].role` только `user` или `agent`.
- `variables[].type` только `string`, `number`, `boolean` или `enum`.
- Если `variables[].type = enum`, должен быть непустой `choices`.

---

# 27. Общий формат API response

## 27.1. Одиночный объект

```json
{
  "data": {}
}
```

## 27.2. Список

```json
{
  "data": [],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 100
  }
}
```

Если endpoint не использует пагинацию, `meta` можно не возвращать.

---

# 28. Общий формат ошибки

Все ошибки должны возвращаться в едином формате:

```json
{
  "error": {
    "type": "validation_error",
    "field": "nodes[0].instruction.text",
    "code": "max_length_exceeded",
    "message": "Instruction text must be no longer than 50000 characters",
    "limit": 50000
  }
}
```

Поля:

- `type` — `validation_error`, `business_error`, `not_found`, `internal_error`.
- `field` — путь до поля или `null`.
- `code` — машинно-читаемый код.
- `message` — человекочитаемое описание.
- `limit` — лимит или `null`.

HTTP status codes:

- `200 OK` — успешное получение или обновление;
- `201 Created` — успешное создание;
- `204 No Content` — успешное удаление или действие без тела;
- `400 Bad Request` — validation/business error;
- `404 Not Found` — ресурс не найден в текущем workspace;
- `409 Conflict` — конфликт состояния;
- `500 Internal Server Error` — внутренняя ошибка.

---

# 29. API endpoints

Все frontend/admin endpoints находятся под:

```text
/api/v1
```

Все runtime endpoints находятся под:

```text
/api/v1/runtime
```

## 29.1. Folders

```http
GET    /api/v1/folders
POST   /api/v1/folders
PATCH  /api/v1/folders/{folderId}
DELETE /api/v1/folders/{folderId}
```

## 29.2. Agents

```http
GET    /api/v1/agents?page=1&limit=20&folderId=...
POST   /api/v1/agents
GET    /api/v1/agents/{agentId}
PATCH  /api/v1/agents/{agentId}
DELETE /api/v1/agents/{agentId}
```

## 29.3. Conversation Flows

```http
GET    /api/v1/agents/{agentId}/conversation-flows
GET    /api/v1/agents/{agentId}/conversation-flows/published
GET    /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0
PATCH  /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0
POST   /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/versions?fromVersion=0
POST   /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/publish?version=0
POST   /api/v1/agents/{agentId}/conversation-flows/unpublish
DELETE /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0
```

## 29.4. Runtime

```http
GET /api/v1/runtime/workspaces/published-agents
GET /api/v1/runtime/workspaces/{workspaceId}/published-agents
GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
```

---

# 30. API Contract: Agents

## 30.1. GET /api/v1/agents

Лёгкий список агентов workspace.

Query params:

- `page` — по умолчанию 1;
- `limit` — по умолчанию 20, максимум 100;
- `folderId` — опциональный фильтр.

Response:

```json
{
  "data": [
    {
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "name": "Customer Support RU",
      "voice_id": "retell-Cimo",
      "language": "ru-RU",
      "folder_id": null,
      "last_modified": 1777983018802,
      "created_at": 1777982621600,
      "updated_at": 1777983018802
    }
  ],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 1
  }
}
```

## 30.2. POST /api/v1/agents

Создаёт агента, workflow v0 и PublishedWorkflow.

Request может быть пустым:

```json
{}
```

Если поля не переданы, используются backend defaults.

Response `201 Created` возвращает полный Agent.

## 30.3. GET /api/v1/agents/{agentId}

Возвращает полный Agent.

## 30.4. PATCH /api/v1/agents/{agentId}

Обновляет top-level поля Agent.

Через этот endpoint обновляются:

- `name`;
- `folder_id`;
- `voice_id`;
- `language`;
- `tts`;
- `stt`;
- `interruption_sensitivity`;
- остальные настройки Agent.

Agent не версионируется.

## 30.5. DELETE /api/v1/agents/{agentId}

Удаляет агента и все версии workflow только если у агента нет PublishedWorkflow.

Если PublishedWorkflow существует:

```text
400 agent_has_published_workflow
```

---

# 31. API Contract: Conversation Flows

## 31.1. GET /api/v1/agents/{agentId}/conversation-flows

Возвращает лёгкий список версий workflow.

Response:

```json
{
  "data": [
    {
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 0,
      "created_at": 1777982621600,
      "updated_at": 1777983690814,
      "published": true
    },
    {
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 1,
      "created_at": 1777983900000,
      "updated_at": 1777984000000,
      "published": false
    }
  ]
}
```

`published` вычисляется через PublishedWorkflow.

## 31.2. GET /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=N

Возвращает конкретную версию workflow полностью.

Response должен содержать:

- полный ConversationFlow;
- `published`;
- `response_engine` из явно запрошенных `conversationFlowId + version`.

Пример:

```json
{
  "data": {
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "version": 1,
    "published": false,
    "response_engine": {
      "type": "conversation-flow",
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 1
    },
    "nodes": [],
    "start_node_id": "start-node-1777982620634",
    "start_speaker": "agent",
    "global_prompt": "Ты дружелюбный помощник поддержки. {{current_time}}"
  }
}
```

## 31.3. PATCH workflow version

```http
PATCH /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=N
```

Правила:

- обновлять только явно переданные поля;
- `nodes` заменяется целиком;
- опубликованную версию нельзя редактировать;
- response возвращает полный ConversationFlow + `published` + `response_engine`.

## 31.4. POST versions

```http
POST /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/versions?fromVersion=N
```

Создаёт новую версию как копию `fromVersion`.

Новая версия не публикуется автоматически.

Response возвращает полный ConversationFlow новой версии + `published=false` + `response_engine` новой версии.

## 31.5. POST publish

```http
POST /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/publish?version=N
```

Публикует указанную версию.

Response:

```json
{
  "data": {
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "version": 1,
    "published": true,
    "published_at": 1777987000000
  }
}
```

## 31.6. POST unpublish

```http
POST /api/v1/agents/{agentId}/conversation-flows/unpublish
```

Удаляет PublishedWorkflow.

Не очищает `agent.response_engine`.

Response:

```text
204 No Content
```

## 31.7. DELETE workflow version

```http
DELETE /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=N
```

Удаляет только неопубликованную версию.

Опубликованную версию удалить нельзя.

---

# 32. API Contract: Runtime

## 32.1. GET /api/v1/runtime/workspaces/published-agents

Возвращает все workspace, у которых есть опубликованные агенты.

Не требует `X-Workspace-Id`.

## 32.2. GET /api/v1/runtime/workspaces/{workspaceId}/published-agents

Возвращает опубликованных агентов одного workspace.

Не требует `X-Workspace-Id`.

## 32.3. GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config

Возвращает склеенный runtime JSON:

```json
{
  "data": {
    "agent": {},
    "conversation_flow": {},
    "published_workflow": {}
  }
}
```

Runtime response должен использовать version из PublishedWorkflow.

Если PublishedWorkflow отсутствует:

```text
404 published_workflow_not_found
```

---

# 33. Индексы MongoDB

Обязательные индексы:

## 33.1. folders

```text
workspace_id + folder_id
```

## 33.2. agents

```text
workspace_id + agent_id
workspace_id + folder_id
```

## 33.3. conversation_flows

```text
workspace_id + agent_id + conversation_flow_id + version
workspace_id + agent_id
```

Уникальный индекс на конкретную версию:

```text
workspace_id + agent_id + conversation_flow_id + version
```

## 33.4. published_workflows

Уникальный индекс:

```text
workspace_id + agent_id
```

Для runtime списков:

```text
workspace_id
```

---

# 34. Конфигурация

Сервис должен получать конфигурацию через Viper из env / config files.

Минимальные параметры:

```env
HTTP_PORT=9001
HTTP_BODY_LIMIT_BYTES=8388608
MONGO_URI=mongodb://localhost:27017
MONGO_DB=agents_service
LOG_LEVEL=info
ENV=local
```

Для E2E окружения может использоваться:

```env
HTTP_BODY_LIMIT_BYTES=20971520
MONGO_URI=mongodb://mongo-test:27017
MONGO_DB=agents_service_e2e
```

---

# 35. Runtime / protected router rule

Router должен явно разделять runtime и protected groups.

Пример:

```go
api := r.Group("/api/v1")

runtime := api.Group("/runtime")
{
    runtime.GET("/workspaces/published-agents", api.RuntimeListPublishedAgentsByWorkspace)
    runtime.GET("/workspaces/:workspaceId/published-agents", api.RuntimeListPublishedAgentsForWorkspace)
    runtime.GET("/workspaces/:workspaceId/agents/:agentId/published-config", api.RuntimeGetPublishedConfig)
}

protected := api.Group("")
protected.Use(middleware.RequireWorkspaceID())
{
    protected.GET("/agents", api.ListAgents)
    protected.POST("/agents", api.CreateAgent)
    // остальные frontend/admin routes
}
```

Runtime routes не должны попадать под `RequireWorkspaceID`.

---

# 36. Логика runtime published-config

Usecase:

```text
RuntimeUsecase.GetPublishedConfig(workspaceID, agentID)
```

Алгоритм:

```text
1. agent = AgentRepository.GetByID(workspaceID, agentID)
2. pw = PublishedWorkflowRepository.Get(workspaceID, agentID)
3. cf = ConversationFlowRepository.GetVersion(workspaceID, agentID, pw.conversation_flow_id, pw.version)
4. agent.response_engine = ResponseEngine{
     type: "conversation-flow",
     conversation_flow_id: pw.conversation_flow_id,
     version: pw.version,
   }
5. return {agent, conversation_flow: cf + published=true, published_workflow: pw}
```

Запрещено:

- брать version из request;
- брать version из `agent.response_engine` как из источника истины;
- возвращать неопубликованную версию в runtime response;
- запускать runtime без PublishedWorkflow.

---

# 37. Логика frontend/editor flow response

Endpoint:

```http
GET /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=N
```

Должен вернуть flow, который явно запрошен в URL.

Даже если он не опубликован.

`response_engine` в этом response должен быть сформирован из URL:

```json
{
  "type": "conversation-flow",
  "conversation_flow_id": "<conversationFlowId>",
  "version": N
}
```

`published` вычисляется отдельно через PublishedWorkflow.

---

# 38. Логика delete и unpublish

## 38.1. Нельзя удалить агента с PublishedWorkflow

Если PublishedWorkflow существует:

```text
DELETE /api/v1/agents/{agentId}
```

должен вернуть:

```text
400 agent_has_published_workflow
```

## 38.2. После unpublish агента можно удалить

Если PublishedWorkflow удалён, агент может быть удалён вместе со всеми версиями workflow.

## 38.3. Нельзя удалить опубликованную версию

Если `PublishedWorkflow` указывает на эту версию:

```text
400 cannot_delete_published_version
```

## 38.4. Можно удалить неопубликованную версию

Если версия не опубликована, её можно удалить.

---

# 39. Правила безопасности

- Все frontend/admin операции обязаны фильтроваться по `workspace_id`.
- Нельзя вернуть данные другого workspace.
- Runtime endpoints должны быть доступны только внутренним сервисам на уровне инфраструктуры.
- Runtime endpoints не требуют пользовательских headers, но это не означает публичный доступ из интернета.
- В production runtime endpoints нужно закрыть через private network, Traefik rules, service token или mTLS.
- E2E-тесты никогда не должны использовать production MongoDB.
- Production container не должен запускать destructive E2E-тесты при старте.

---

# 40. Тестирование и pre-flight

Подробная тестовая документация должна быть в отдельном файле.

Но базовые правила для реализации сервиса:

- `go test ./...` должен проходить.
- Unit tests проверяют валидаторы и usecase на fake repositories.
- API tests через `httptest` проверяют routes, middleware, status codes, JSON responses.
- E2E tests проверяют реальный HTTP API и реальную тестовую MongoDB.
- E2E запускается отдельной командой и не запускается при старте production container.

Pre-flight перед запуском сервиса:

```bash
make verify
```

`make verify` должен:

1. Запустить `go test ./...`.
2. Собрать тестовое окружение Docker.
3. Поднять отдельную MongoDB test replica set.
4. Поднять AgentService test container.
5. Запустить E2E по HTTP.
6. Удалить test volume после завершения.

Если `make verify` успешен, можно запускать сервис.

---

# 41. Ожидаемый результат реализации

После реализации сервиса должно быть выполнено:

- сервис стартует;
- `/healthz` работает;
- frontend/admin API требует `X-Workspace-Id`;
- runtime API не требует `X-Workspace-Id`;
- создание агента создаёт Agent + ConversationFlow v0 + PublishedWorkflow;
- базовый workflow публикуется сразу;
- frontend может получить flow v0;
- frontend response содержит `published` и `response_engine`;
- можно создать v1 из v0;
- v1 можно редактировать, пока она не опубликована;
- runtime до публикации v1 продолжает отдавать v0;
- после публикации v1 runtime отдаёт v1;
- опубликованную версию нельзя PATCH;
- опубликованную версию нельзя DELETE;
- агента с PublishedWorkflow нельзя DELETE;
- unpublish удаляет PublishedWorkflow;
- после unpublish runtime published-config возвращает `published_workflow_not_found`;
- после unpublish frontend всё ещё может открыть v0/v1 по явной версии;
- после unpublish агент может быть удалён;
- лимит 1000 nodes работает;
- лимит workflow JSON 8MB работает;
- папки работают;
- Template Agents нельзя создать как обычную папку;
- Mongo indexes создаются;
- unit/API/E2E тесты проходят.

---

# 42. Этапы работы исполнителя

1. Создать или обновить `RULES.md` / основную документацию по этому контракту.
2. Показать содержимое документации.
3. После подтверждения реализовать доменные модели.
4. Реализовать валидаторы.
5. Реализовать repository interfaces.
6. Реализовать Mongo repositories.
7. Реализовать usecases.
8. Реализовать handlers.
9. Разделить runtime и protected router groups.
10. Реализовать templates.
11. Реализовать Mongo indexes.
12. Реализовать unit tests.
13. Реализовать API httptest tests.
14. Реализовать E2E test pipeline отдельно от production.
15. Прогнать `go test ./...`.
16. Прогнать `make verify`.
17. Выдать отчёт по реализованным файлам, endpoints, тестам и результатам.

---

# 43. Краткий эталон поведения сервиса

```text
AgentService хранит агентов и версии workflow.
Agent не версионируется.
ConversationFlow версионируется.
PublishedWorkflow определяет, какая версия опубликована.
Runtime берёт версию только из PublishedWorkflow.
Frontend может открывать любую явно запрошенную версию.
Опубликованную версию нельзя редактировать и удалять.
Чтобы изменить опубликованный workflow, нужно создать новую версию, изменить её и опубликовать.
Unpublish удаляет PublishedWorkflow, но не очищает agent.response_engine.
Если PublishedWorkflow отсутствует, runtime-запуск невозможен.
Все frontend/admin операции изолированы по X-Workspace-Id.
Runtime API предназначен для внутреннего микросервиса звонков.
```