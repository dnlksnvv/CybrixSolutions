## AgentService - техническое задание

Ты — профессиональный Go Senior/Architect высшего уровня с 10+ летним опытом создания высоконагруженных микросервисов. Ты всегда выбираешь максимально чистые, простые, поддерживаемые и масштабируемые решения. Ты строго следуешь принципам Clean Architecture + Lean Architecture, избегаешь over-engineering, дублирования кода и нескольких способов сделать одну и ту же вещь.

Твоя задача — полностью реализовать сервис **AgentService**.

---

## Назначение сервиса

AgentService — это центральный сервис управления AI-агентами и их Conversation Flow (workflow). Он отвечает за создание, хранение, редактирование, версионирование, публикацию и удаление агентов и связанных с ними workflow. Сервис является частью большой микросервисной архитектуры, общается только по HTTP и не должен ходить в другие сервисы.

---

## Требования к архитектуре и стилю кода

- Использовать Clean Architecture в сочетании с Lean-подходом.
- Код должен быть максимально читаемым, предсказуемым и легко модифицируемым в будущем.
- Никаких нескольких способов сделать одну вещь — только один стандартизированный, чистый способ.
- Максимально использовать готовые библиотеки и пакеты вместо написания своего кода.
- Все повторяющиеся куски выносить в helper-функции и утилиты.
- Код должен быть строгим: все входные данные обязательно валидируются.
- Никакого over-engineering. Простота, читаемость и эффективность — приоритет.
- Покрыть сервис unit-тестами (go test + testify).
- Весь код должен быть подробно покрыт комментариями на русском языке.
- Комментарии должны объяснять не только "что делает код", но и "зачем это сделано именно так".
- Комментарии обязательны для доменных моделей, DTO, usecase-методов, repository-методов, валидаторов, middleware, helper-функций ошибок и сложной бизнес-логики.
- Все публичные структуры, интерфейсы, методы и функции должны иметь комментарии на русском языке.
- В местах, где есть важные бизнес-правила (публикация workflow, запрет редактирования опубликованной версии, создание новой версии, снятие публикации, удаление агента, проверка workspace_id), комментарии должны подробно пояснять правило и причину его существования.

---

## Технологический стек

- Go (последняя стабильная версия)
- MongoDB + официальный MongoDB Go Driver (go.mongodb.org/mongo-driver)
- Gin (или Echo — выбери сам, что лучше подходит)
- Viper для конфигурации
- Zerolog для логирования
- Testify + стандартные go test

---

## Архитектура проекта

Сервис должен быть реализован по Clean Architecture с Lean-подходом.

Главное правило зависимостей:

```text
handler → usecase → repository interface → repository implementation
```

Запрещено:

- вызывать MongoDB напрямую из HTTP handlers;
- размещать бизнес-логику публикации, версионирования, удаления и валидации в handlers;
- использовать DTO HTTP-запросов как доменные модели;
- смешивать transport, business logic и database logic в одном пакете.

Разрешено:

- держать структуру простой;
- не создавать лишние абстракции без необходимости;
- использовать interfaces только там, где они реально отделяют бизнес-логику от инфраструктуры.

---

## Рекомендуемая структура проекта

```text
cmd/agent-service/
  main.go

internal/
  config/
    config.go

  transport/http/
    router.go
    middleware.go
    handlers/
      agent_handler.go
      folder_handler.go
      conversation_flow_handler.go

  app/
    usecase/
      agent_usecase.go
      folder_usecase.go
      conversation_flow_usecase.go
      publication_usecase.go

  domain/
    models/
      agent.go
      folder.go
      conversation_flow.go
      published_workflow.go
      node.go
    errors/
      validation_error.go
      business_error.go
    validation/
      agent_validator.go
      folder_validator.go
      workflow_validator.go

  repository/
    interfaces/
      agent_repository.go
      folder_repository.go
      conversation_flow_repository.go
      published_workflow_repository.go
    mongo/
      agent_repository.go
      folder_repository.go
      conversation_flow_repository.go
      published_workflow_repository.go

  templates/
    agent_template.go
    default_conversation_flow.go

  logger/
    logger.go

  server/
    server.go
```

---

## Ответственность слоёв

### transport/http

Отвечает только за HTTP:

- прочитать headers;
- распарсить query/path/body;
- вызвать нужный usecase;
- вернуть JSON response;
- преобразовать ошибку в HTTP response.

Handlers не должны содержать бизнес-логику.

### app/usecase

Содержит бизнес-логику сервиса:

- создание агента;
- создание базового workflow;
- создание новой версии workflow;
- публикация workflow;
- снятие публикации;
- запрет редактирования опубликованной версии;
- запрет удаления опубликованной версии;
- удаление агента;
- перемещение агента между папками.

Usecase работает только через repository interfaces.

### domain/models

Содержит чистые доменные модели:

- Agent;
- Folder;
- ConversationFlow;
- PublishedWorkflow;
- Node.

Доменные модели не должны зависеть от Gin, MongoDB, HTTP или внешней инфраструктуры.

### domain/validation

Содержит валидацию входных данных и workflow:

- лимиты строк;
- лимиты массивов;
- лимиты числовых полей;
- проверки уникальности Node.id;
- проверки start_node_id;
- проверки destination_node_id;
- проверки enum variables;
- проверки transcript roles.

Все ошибки валидации должны создаваться через общий helper.

### repository/interfaces

Содержит интерфейсы репозиториев, которые используются usecase-слоем.

### repository/mongo

Содержит реализацию репозиториев через MongoDB Go Driver.

Только этот слой имеет право напрямую работать с MongoDB.

### templates

Содержит базовый шаблон агента и базовый шаблон ConversationFlow.

Шаблон должен быть легко редактируемым без изменения бизнес-логики.

---

## Правила DTO и моделей

Нужно разделять:

- HTTP request DTO;
- HTTP response DTO;
- domain models;
- MongoDB persistence models, если это необходимо.

Нельзя напрямую использовать request body как доменную модель без валидации.

---

## Транзакционность и консистентность

---

## Комментарии в коде

Код должен быть максимально понятным для будущей поддержки. Поэтому все ключевые части сервиса должны быть подробно прокомментированы на русском языке.

Требования к комментариям:

- Комментарии пишутся на русском языке.
- Комментарии должны быть понятными для разработчика, который впервые открыл проект.
- Комментарии должны объяснять назначение структуры, функции, метода или бизнес-правила.
- В сложных местах комментарий должен объяснять, почему выбран именно такой подход.
- Комментарии не должны быть формальными и бесполезными. Плохой комментарий: `// сохраняем агента`. Хороший комментарий: `// Сохраняем агента только после создания workflow v0, чтобы Agent сразу имел валидную опубликованную версию для запуска`.

Обязательно комментировать:

- доменные модели;
- request DTO и response DTO;
- repository interfaces;
- MongoDB repository implementations;
- usecase-методы;
- валидаторы;
- helper-функции ошибок;
- middleware чтения `X-Workspace-Id`;
- бизнес-правила публикации workflow;
- бизнес-правила снятия публикации;
- запрет редактирования опубликованной версии;
- запрет удаления опубликованной версии;
- запрет удаления агента с опубликованным workflow;
- создание агента с дефолтным workflow-шаблоном;
- создание новой версии workflow через `fromVersion`;
- вычисление поля `published` при выдаче списка версий.

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

Операции, которые меняют несколько коллекций, должны выполняться атомарно на уровне бизнес-логики.

К таким операциям относятся:

- создание агента + создание workflow v0 + создание PublishedWorkflow;
- публикация workflow + обновление PublishedWorkflow + обновление agent.response_engine;
- снятие публикации + удаление PublishedWorkflow + очистка agent.response_engine;
- удаление агента + удаление всех workflow-версий.

Если MongoDB transactions доступны в окружении, использовать transaction/session для этих операций.

Если transactions недоступны, код должен быть написан так, чтобы минимизировать риск рассинхронизации и возвращать понятную ошибку при частичном сбое.

---

## Заголовки запросов (обязательно)

Все запросы приходят через Traefik и содержат заголовки:

- X-User-Id
- X-Workspace-Id (обязательный)
- X-User-Scopes (опционально)

Сервис обязан проверять наличие X-Workspace-Id и использовать его во всех обычных frontend/admin API-операциях.

Исключение: отдельные runtime endpoints для микросервиса звонков. Они предназначены только для внутреннего межсервисного использования, получают `workspace_id` и `agent_id` через path/query параметры и не требуют пользовательских headers `X-User-Id`, `X-Workspace-Id`, `X-User-Scopes`.
---

## Стандартизированный формат обновления workflow (строго один способ)

Обновление workflow делается **только** через PATCH и только top-level полями полностью. Никаких точечных обновлений внутри нод.

Важно: Если какое-то top-level поле (nodes, global_prompt, model_choice, start_node_id и т.д.) **не пришло** в теле PATCH-запроса — оно **не должно обновляться**.

Обновляются только те поля, которые явно присутствуют в запросе.

Общее правило для PATCH-запросов: request body должен иметь ту же структуру, что и полный объект ресурса в response, но все поля в PATCH являются опциональными. Клиент передаёт только те поля, которые хочет изменить.

Например, если `GET /agents/{agentId}` возвращает полный Agent с вложенными `tts`, `stt`, `response_engine` и другими объектами, то `PATCH /agents/{agentId}` принимает такой же формат Agent, но можно передать только `tts`, только `stt`, только `folder_id`, только `name` или любую другую часть Agent.

Если `GET /conversation-flows/{conversationFlowId}?version=1` возвращает полный ConversationFlow, то `PATCH /conversation-flows/{conversationFlowId}?version=1` принимает такой же формат ConversationFlow, но можно передать только `nodes`, только `global_prompt`, только `model_choice` и т.д.

Поля `tts` и `stt` относятся к Agent и обновляются только через `PATCH /agents/{agentId}`, а не через PATCH workflow.

Пример тела PATCH-запроса для workflow:

```json
{
  "nodes": [
    { "полная нода": 0 },
    { "полная нода": 1 }
  ],
  "global_prompt": "новый текст",
  "model_choice": { },
  "start_node_id": "xxx",
  "start_speaker": "agent",
  "flex_mode": true
}
```

Если поле не пришло — оно не обновляется.

---

# Полные структуры данных

########################################################

## Особенности виртуальной папки "Template Agents"

- "Template Agents" — это **виртуальная** папка. Она **не хранится** как документ в коллекции Folder.
- Все агенты **всегда** отображаются в папке "Template Agents", независимо от того, есть ли у них `folder_id`.
- Если у агента указан `folder_id` — он отображается **одновременно** и в своей реальной папке, **и** в "Template Agents".
- Папку "Template Agents" **нельзя** создать, переименовать или удалить через API.
- При удалении обычной папки у всех агентов из неё поле `folder_id` сбрасывается. После этого агенты остаются отображаться в виртуальной папке "Template Agents".

---

## Перемещение агентов между папками

Перемещение делается **точно как в Retell AI**:

**PATCH /agents/{agentId}**

Тело запроса может содержать поле:

```json
{
  "folder_id": "folder_xxx"
}
```

`folder_id: null` → у агента сбрасывается привязка к обычной папке, и он остаётся отображаться в виртуальной папке "Template Agents".

Если поле `folder_id` отсутствует в PATCH-запросе — папка агента не изменяется.

При DELETE /folders/{folderId} — у всех агентов из этой папки `folder_id` сбрасывается. После этого агенты остаются отображаться в виртуальной папке "Template Agents".

---

## Folder

- folder_id (string, префикс "folder_")
- workspace_id
- name
- created_at, updated_at

---

## Folder — полная структура

Папка для группировки агентов. Структура должна быть максимально простой.

Важно: в Folder **не хранится** массив `agentIds`. Источник истины для принадлежности агента к папке — только поле `Agent.folder_id`.

Количество агентов в папке и список агентов в папке считаются через запрос по `Agent.folder_id`. Это проще, чище и исключает риск рассинхронизации данных.

```jsonc
{
  "folder_id": "string",        // обязательно, префикс "folder_", пример: "folder_5072e604e6063f693c10de4a"
  "workspace_id": "string",     // обязательно, ID воркспейса. Все операции с папками и агентами строго привязаны к workspace
  "name": "string",             // обязательно, человекочитаемое название папки, пример: "Sales Agents"
  "created_at": "number",       // Unix timestamp в миллисекундах, когда папка была создана
  "updated_at": "number"        // Unix timestamp в миллисекундах, последнее изменение папки
}
```

Пояснения по полям:

- folder_id — уникальный идентификатор папки. Генерируется автоматически с префиксом folder_.
- workspace_id — строго обязательное поле. Все операции с папками и агентами привязаны к воркспейсу.
- name — название папки, которое видит пользователь на фронтенде.
- created_at / updated_at — таймстампы в миллисекундах (как в Retell AI).
- agentIds в Folder не хранится. Список агентов в папке и количество агентов вычисляются запросом по `Agent.folder_id`.

Пример реального документа в MongoDB:

```json
{
  "folder_id": "folder_5072e604e6063f693c10de4a",
  "workspace_id": "ws_abc123xyz",
  "name": "Sales Agents",
  "created_at": 1774384312548,
  "updated_at": 1777982029068
}
```

Важно: это пример обычной папки. Виртуальная папка "Template Agents" в MongoDB не хранится и через Folder API не создаётся.

---

## Особенности работы с Folder

### Перемещение агентов между папками

Должны быть методы:

- Присвоить агенту folder_id (переместить в папку)
- Убрать folder_id у агента (сбросить привязку к обычной папке)
- При удалении папки — у всех агентов этой папки folder_id сбрасывается

### Виртуальная папка "Template Agents"

Все агенты всегда видны в виртуальной папке "Template Agents". Она не создаётся в базе и не может быть удалена.

########################################################

## Agent — полная структура

Важно: `tts`, `stt`, `voice_id`, `language` и другие настройки Agent не входят в ConversationFlow и не версионируются вместе с workflow.

Workflow никогда не должен содержать, обновлять или переопределять `tts` и `stt`. Эти настройки принадлежат только Agent.

Если пользователь меняет `tts` или `stt` через `PATCH /agents/{agentId}`, изменение применяется сразу ко всем последующим запускам агента, независимо от того, какая версия workflow опубликована.

```jsonc
{
  "agent_id": "string",                    // обязательно, префикс "agent_", пример: "agent_9bb2ac714ff6733eabdc922bdc"
  "workspace_id": "string",                // обязательно, ID воркспейса. Нужен для строгой изоляции данных
  "folder_id": "string | null",            // null = агент не привязан к обычной папке, но отображается в виртуальной папке "Template Agents"

  "name": "string",                        // название агента, которое видит пользователь
  "channel": "voice" | "chat",             // тип канала: голосовой агент или чат-агент

  "voice_id": "string",                    // основной voice_id агента, пример: "retell-Cimo", "minimax-Cimo", "retell-Marissa"
  "language": "string",                    // основной язык агента, пример: "ru-RU", "en-US", "es-419"

  "tts": {                                  // настройки Text-To-Speech. Обновляются через PATCH /agents/{agentId}
    "voice_id": "string",                  // ID голоса для синтеза речи
    "language": "string",                  // язык синтеза речи, пример: "ru-RU", "en-US", "es-419"
    "emotion": "string | null",            // опциональная эмоция голоса, если поддерживается провайдером
    "speed": "number | null"               // скорость речи, например 1.0 = нормальная скорость
  },

  "stt": {                                  // настройки Speech-To-Text. Обновляются через PATCH /agents/{agentId}
    "model_id": "string",                  // ID модели распознавания речи
    "language": "string"                   // язык распознавания речи, пример: "ru-RU", "en-US", "es-419"
  },

  "interruption_sensitivity": "number",    // чувствительность к перебиванию пользователем, обычно от 0.0 до 1.0, 0.9 по умолчанию
  "max_call_duration_ms": "number",        // максимальная длительность звонка в миллисекундах
  "normalize_for_speech": "boolean",       // нормализовать ли текст перед озвучкой
  "allow_user_dtmf": "boolean",            // разрешены ли DTMF-нажатия пользователя
  "user_dtmf_options": { },                 // объект с настройками DTMF

  "response_engine": {                      // движок ответа и ссылка на ConversationFlow в контексте конкретной выдачи данных
    "type": "conversation-flow",           // тип движка ответа. Для этого сервиса всегда "conversation-flow"
    "conversation_flow_id": "string",      // ID ConversationFlow. Для frontend response соответствует явно запрошенной версии, для runtime response соответствует опубликованной версии
    "version": "number"                    // версия ConversationFlow. Для frontend response соответствует явно запрошенной версии, для runtime response соответствует опубликованной версии
  },

  "handbook_config": {                      // дополнительные настройки поведения агента
    "default_personality": "boolean",      // использовать ли дефолтную личность агента
    "ai_disclosure": "boolean"             // раскрывать ли пользователю, что он общается с AI
  },

  "pii_config": {                           // настройки обработки персональных данных
    "mode": "post_call" | "none",          // режим обработки PII после звонка или отключение обработки
    "categories": [ ]                       // массив категорий персональных данных
  },

  "data_storage_setting": "everything" | "none" | "metadata_only", // политика хранения данных разговоров

  "last_modified": "number",               // Unix timestamp в миллисекундах, последнее значимое изменение агента
  "created_at": "number",                  // Unix timestamp в миллисекундах, когда агент был создан
  "updated_at": "number"                   // Unix timestamp в миллисекундах, последнее обновление документа
}
```

Пояснения по полям Agent:

- agent_id — уникальный идентификатор агента (генерируется автоматически).
- workspace_id — обязательный идентификатор воркспейса. Все операции должны фильтроваться по workspace_id.
- folder_id — может быть null. В этом случае агент не привязан к обычной папке, но всё равно отображается в виртуальной папке "Template Agents".
- voice_id и language — основные настройки голоса и языка агента.
- tts и stt — объекты, которые нужно хранить на уровне Agent. Они обновляются целиком через `PATCH /agents/{agentId}`.
- interruption_sensitivity, max_call_duration_ms, normalize_for_speech, allow_user_dtmf, user_dtmf_options — настройки разговора, которые применяются к агенту целиком.
- response_engine — объект движка ответа. Его поля `conversation_flow_id` и `version` заполняются в зависимости от контекста response. Для frontend/editor endpoints конкретной версии flow они должны соответствовать явно запрошенным `conversationFlowId` и `version`. Для runtime endpoints микросервиса звонков они должны соответствовать опубликованной версии из PublishedWorkflow. Обычный `GET /agents/{agentId}` может возвращать опубликованную связку, если она есть, но production-запуск всё равно должен опираться на PublishedWorkflow, а не на Agent как единственный источник истины.
- handbook_config — настройки личности агента и disclosure, если нужно сообщать пользователю, что он общается с AI.
- pii_config — настройки обработки персональных данных.
- data_storage_setting — политика хранения данных разговоров.
- last_modified, created_at, updated_at — хранятся в миллисекундах, как в Retell AI.

Важно: сущность Agent отдельно не версионируется. Версионирование применяется только к ConversationFlow. Agent хранит текущие настройки агента и ссылку на опубликованную версию workflow через `response_engine`.

Важно: если пользователь изменяет настройки Agent, например `tts`, `stt`, `voice_id`, `language`, `interruption_sensitivity` и другие поля агента, эти изменения сразу влияют на запуск голосового агента. Это нормально, потому что Agent не имеет версионирования. Версионирование защищает только ConversationFlow, то есть логику разговора.

Пример реального документа Agent в MongoDB:

```json
{
  "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
  "workspace_id": "ws_abc123xyz",
  "folder_id": "folder_5072e604e6063f693c10de4a",
  "name": "Customer Support RU",
  "channel": "voice",
  "voice_id": "retell-Cimo",
  "language": "ru-RU",
  "tts": {
    "voice_id": "retell-Cimo",
    "language": "ru-RU",
    "speed": 1.0
  },
  "stt": {
    "model_id": "default",
    "language": "ru-RU"
  },
  "interruption_sensitivity": 0.9,
  "max_call_duration_ms": 3600000,
  "response_engine": {
    "type": "conversation-flow",
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "version": 0
  },
  "handbook_config": {
    "default_personality": true,
    "ai_disclosure": true
  },
  "last_modified": 1777983018802,
  "created_at": 1777982621600,
  "updated_at": 1777983018802
}
```

---

## Создание агента и базовый шаблон workflow

При создании агента через `POST /agents` сервис должен автоматически создать:

1. Документ Agent.
2. Начальный ConversationFlow версии 0.
3. Запись PublishedWorkflow для этого агента и workspace.
4. Связь с опубликованной версией workflow в `agent.response_engine`.

Пользователь не должен попадать в абсолютно пустого агента. При создании агента должен создаваться базовый workflow-шаблон с минимальным набором нод, стартовой нодой, глобальным промптом и базовыми настройками модели.

Базовый шаблон создания агента должен находиться на бекенде в одном понятном месте, например в отдельном пакете или файле:

- `internal/templates/agent_template.go`
- или `internal/templates/default_conversation_flow.go`

Требования к базовому шаблону:

- Шаблон должен быть легко редактируемым на бекенде без изменения бизнес-логики сервиса.
- Логика создания агента должна брать дефолтные значения из этого шаблона.
- В шаблоне должны быть дефолтные значения для Agent, TTS, STT, ConversationFlow и стартовых nodes.
- Если в `POST /agents` пользователь передал свои значения, они могут переопределить дефолтные значения шаблона.
- Если значение не передано — используется дефолтное значение из шаблона.
- Базовый workflow должен сразу становиться опубликованным для созданного агента.
- При создании агента версия создаваемого ConversationFlow всегда равна `0`, а Agent сразу получает ссылку на опубликованную версию в `response_engine`.

---

########################################################

## ConversationFlow (workflow)

```jsonc
{
  "conversation_flow_id": "string",         // обязательно, префикс "conversation_flow_", пример: "conversation_flow_7f7944f06518"
  "agent_id": "string",                     // обязательно, ID агента, которому принадлежит workflow
  "workspace_id": "string",                 // обязательно, ID воркспейса для изоляции данных
  "version": "number",                      // версия workflow. Начинается с 0, увеличивается при создании новой версии

  "nodes": [ "Node", "Node" ],             // массив полных нод. Это основная логика conversation flow

  "start_node_id": "string",                // ID стартовой ноды
  "start_speaker": "agent" | "user",        // кто начинает разговор: агент или пользователь

  "global_prompt": "string",                // глобальный промпт для всего workflow. Может содержать {{current_time}}, {{user_name}} и другие переменные

  "model_choice": {                          // выбор LLM-модели
    "type": "cascading",                    // обычно "cascading"
    "model": "string",                      // например "gpt-4.1", "gpt-5.4"
    "high_priority": "boolean"              // опциональный флаг высокого приоритета
  },

  "model_temperature": "number",            // температура модели, например 0.7

  "flex_mode": "boolean",                   // гибкий режим, возможность вызова глобальных нод из любой точки графа
  "tool_call_strict_mode": "boolean",       // строгий режим вызова инструментов

  "kb_config": {                             // настройки базы знаний / Retrieval / RAG
    "top_k": "number",                      // сколько релевантных фрагментов доставать, например 3
    "filter_score": "number"                // минимальный score релевантности, например 0.6
  },

  "begin_tag_display_position": {            // позиция стартового блока на canvas фронтенда
    "x": "number",
    "y": "number"
  },

  "is_transfer_cf": "boolean",              // является ли workflow отдельным conversation flow для перевода звонка

  "created_at": "number",                   // Unix timestamp в миллисекундах, когда workflow был создан
  "updated_at": "number"                    // Unix timestamp в миллисекундах, последнее изменение workflow
}
```

Пояснения по полям ConversationFlow:

- conversation_flow_id — уникальный идентификатор workflow.
- agent_id — ID агента, которому принадлежит workflow.
- workspace_id — ID воркспейса. Все операции с workflow должны проверять workspace_id.
- version — версия workflow. При создании новой версии создаётся копия версии, указанной в `fromVersion`, с увеличенным номером версии.
- В ConversationFlow не хранится поле `is_published`. Статус публикации вычисляется через коллекцию PublishedWorkflow.
- nodes — самое важное поле. Всегда содержит полный массив нод. При обновлении клиент присылает полный массив nodes.
- start_node_id — ID ноды, с которой начинается выполнение workflow.
- start_speaker — кто начинает разговор в workflow.
- global_prompt — глобальный системный промпт, который применяется ко всему workflow. Поддерживает динамические переменные {{}}.
- model_choice — выбор модели LLM.
- model_temperature — температура генерации ответа модели.
- flex_mode — включает гибкий режим, возможность вызова глобальных нод из любой точки.
- tool_call_strict_mode — строгий режим вызова инструментов.
- kb_config — настройки Retrieval / RAG.
- begin_tag_display_position — позиция стартового блока на canvas фронтенда.
- is_transfer_cf — признак workflow для передачи звонка.
- created_at / updated_at — таймстампы в миллисекундах.

Важно: ConversationFlow хранит версии логики поведения агента. Пользователь может создавать новые версии workflow, редактировать их и тестировать, не влияя на опубликованную версию агента. Сам документ ConversationFlow не знает, опубликован он или нет. Единственный источник истины для публикации — PublishedWorkflow.

Пример реального ConversationFlow:

```json
{
  "conversation_flow_id": "conversation_flow_c530702321b8",
  "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
  "workspace_id": "ws_abc123",
  "version": 0,
  "nodes": [ ],
  "start_node_id": "start-node-1777982620634",
  "start_speaker": "agent",
  "global_prompt": "Ты дружелюбный помощник поддержки. {{current_time}}",
  "model_choice": {
    "type": "cascading",
    "model": "gpt-5.4",
    "high_priority": true
  },
  "model_temperature": 0.7,
  "flex_mode": true,
  "tool_call_strict_mode": true,
  "kb_config": {
    "top_k": 3,
    "filter_score": 0.6
  },
  "begin_tag_display_position": {
    "x": 120,
    "y": 80
  },
  "created_at": 1777982621600,
  "updated_at": 1777983690814
}
```

---

## PublishedWorkflow

PublishedWorkflow — отдельная простая коллекция для быстрого получения опубликованной версии workflow без перебора всех версий workflow в базе.

В один момент времени у одного агента может быть только одна опубликованная версия workflow.

Единственный источник истины для опубликованной версии workflow — PublishedWorkflow. Поле `agent.response_engine` является денормализованной копией для удобства чтения Agent и должно обновляться только в операции публикации/снятия публикации.

ConversationFlow не хранит `is_published`. При выдаче списка версий API вычисляет поле `published: true/false` сравнением каждой версии с записью PublishedWorkflow.

Рекомендуется хранить и `agent_id`, и `conversation_flow_id`, и `version`, потому что:

- `agent_id` нужен для быстрого поиска опубликованного workflow конкретного агента.
- `conversation_flow_id` нужен для прямого получения документа workflow.
- `version` нужен, потому что у одного workflow есть несколько версий, и опубликованной является только одна конкретная версия.
- `workspace_id` нужен для строгой изоляции данных по workspace.

```jsonc
{
  "workspace_id": "string",                 // обязательно, ID воркспейса
  "agent_id": "string",                     // обязательно, ID агента
  "conversation_flow_id": "string",         // обязательно, ID опубликованного ConversationFlow
  "version": "number",                      // обязательно, опубликованная версия ConversationFlow
  "published_at": "number",                 // Unix timestamp в миллисекундах, когда версия была опубликована
  "created_at": "number",                   // Unix timestamp в миллисекундах, когда запись была создана
  "updated_at": "number"                    // Unix timestamp в миллисекундах, последнее изменение записи
}
```

Для коллекции PublishedWorkflow должен быть уникальный индекс по:

```text
workspace_id + agent_id
```

Это гарантирует, что у одного агента в одном workspace не может быть больше одного опубликованного workflow.

---

## Публикация workflow и переключение версий

Публикация workflow означает, что выбранная версия ConversationFlow становится опубликованной и используется при запуске конкретного агента.

При публикации workflow сервис должен:

1. Проверить, что agent принадлежит текущему workspace.
2. Проверить, что conversation_flow_id принадлежит этому agent и workspace.
3. Проверить, что указанная version существует.
4. Обновить или создать запись в PublishedWorkflow для `workspace_id + agent_id`.
5. Обновить `agent.response_engine.conversation_flow_id` и `agent.response_engine.version` как денормализованную копию PublishedWorkflow.
6. Обновить `agent.updated_at` и `agent.last_modified`.

`PublishedWorkflow` и `agent.response_engine` должны всегда быть синхронизированы.

После успешной публикации выбранной версии должны выполняться два условия:

1. В `PublishedWorkflow` записаны `workspace_id`, `agent_id`, `conversation_flow_id` и `version` выбранной версии.
2. В `agent.response_engine` записаны тот же `conversation_flow_id` и та же `version`.

Если хотя бы одно из этих условий нарушено, это считается ошибкой консистентности данных.

У пользователя должна быть возможность создавать новые версии ConversationFlow, редактировать их и публиковать нужную версию.

Версионирование применяется **только к ConversationFlow**. Сущность Agent отдельно не версионируется.

Agent хранит текущие настройки агента и объект `response_engine`. Значения `response_engine.conversation_flow_id` и `response_engine.version` должны заполняться динамически в зависимости от контекста выдачи данных: для frontend/editor endpoints — явно запрошенная версия ConversationFlow, для runtime endpoints микросервиса звонков — опубликованная версия из PublishedWorkflow.
ConversationFlow хранит версии логики поведения агента. Пользователь может создавать новые версии workflow, редактировать их и тестировать, не влияя на опубликованную версию агента.

Редактирование неопубликованной версии workflow не должно влиять на опубликованную версию агента. Агент при запуске всегда использует только версию, указанную в PublishedWorkflow и `agent.response_engine`.
- В ConversationFlow не хранится поле `is_published`. Статус публикации вычисляется через коллекцию PublishedWorkflow.

Правила версионирования:

- При создании агента автоматически создаётся ConversationFlow версии 0.
- При создании новой версии workflow сервис копирует версию, указанную в query-параметре `fromVersion`, и создаёт новый документ с `version = max(version) + 1` для этого agent и conversation_flow_id. Новая версия не получает статуса публикации, потому что статус публикации хранится только в PublishedWorkflow.
- Новая версия не становится опубликованной автоматически. PublishedWorkflow продолжает указывать на ранее опубликованную версию.
- Опубликованной может быть только одна версия workflow на одного агента. Это гарантируется уникальным индексом PublishedWorkflow по `workspace_id + agent_id`.
- Переключение опубликованной версии делается отдельным endpoint публикации.
- Редактировать можно только неопубликованные версии по `conversationFlowId` и `version`. Опубликованную версию редактировать напрямую нельзя. Для изменения опубликованного workflow нужно создать новую версию через `fromVersion`, отредактировать её и затем опубликовать.

Пример сценария работы с версиями:

1. У агента есть опубликованная версия workflow `version = 0`.
2. Пользователь создаёт новую версию через `POST /agents/{agentId}/conversation-flows/{conversationFlowId}/versions?fromVersion=0`.
3. Сервис создаёт новую версию, например `version = 1`. PublishedWorkflow остаётся указывать на `version = 0`, поэтому для фронтенда `version = 1` будет иметь `published = false`.
4. Пользователь редактирует `version = 1` через `PATCH /agents/{agentId}/conversation-flows/{conversationFlowId}?version=1`.
5. Агент всё это время продолжает работать на опубликованной версии `version = 0`.
6. Когда пользователь явно публикует `version = 1`, сервис обновляет PublishedWorkflow и `agent.response_engine`. После этого для фронтенда `version = 0` будет иметь `published = false`, а `version = 1` — `published = true`.

### Запреты для опубликованной версии

- Опубликованную версию ConversationFlow нельзя редактировать через PATCH.
- Опубликованную версию ConversationFlow нельзя удалить.
- Для изменения опубликованной версии нужно создать новую версию через `fromVersion`, отредактировать её и затем опубликовать.
- Если пользователь пытается редактировать опубликованную версию, сервис возвращает `400 Bad Request` с кодом `published_version_is_readonly`.
- Если пользователь пытается удалить опубликованную версию, сервис возвращает `400 Bad Request` с кодом `cannot_delete_published_version`.

### Снятие публикации и удаление

- Удалить опубликованную версию нельзя, пока она записана в PublishedWorkflow.
- Удалить агента с опубликованным workflow нельзя без предварительного снятия публикации.
- Для снятия публикации используется отдельный endpoint `POST /agents/{agentId}/conversation-flows/unpublish`.
<<<<<<< Current (Your changes)
- При снятии публикации сервис удаляет запись PublishedWorkflow. `agent.response_engine` не очищается автоматически и не используется как единственный источник истины для production-запуска. Для runtime endpoints значения `conversation_flow_id` и `version` подставляются динамически из PublishedWorkflow. Если PublishedWorkflow отсутствует, production-запуск невозможен.- После снятия публикации агент не должен запускаться, пока не будет опубликована новая версия workflow.
=======
- При снятии публикации сервис удаляет **только** запись PublishedWorkflow. `agent.response_engine` не очищается автоматически, потому что он может использоваться фронтендом/редактором для отображения последней выбранной версии workflow.
- После снятия публикации агент не должен запускаться, пока не будет опубликована новая версия workflow.
>>>>>>> Incoming (Background Agent changes)
- Если пользователь пытается удалить агента с опубликованным workflow, сервис возвращает `400 Bad Request` с кодом `agent_has_published_workflow`.

---

## Валидация пользовательского контента workflow

Пользователь может редактировать названия нод, промпты, transitions, условия переходов, правила вызова глобальных нод, описания переменных, примеры диалогов и реплики. Все эти данные обязательно должны валидироваться на бекенде.

Валидация должна быть централизованной. Нельзя писать отдельную функцию ошибки под каждый лимит. Должна быть единая helper-функция для ошибок валидации, например:

```go
func ValidationError(field string, code string, message string, limit any) error
```

Ответ API при ошибке валидации должен быть единообразным и понятным фронтенду:

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

Требования к ошибкам:

- HTTP status: `400 Bad Request`.
- `field` должен указывать точный путь до проблемного поля.
- `code` должен быть машинно-читаемым кодом ошибки.
- `message` должен быть понятным человеку.
- `limit` должен передавать ограничение, если ошибка связана с лимитом.
- Все ошибки валидации должны создаваться через общий helper, чтобы формат ошибок был одинаковым во всём сервисе.

### Общие лимиты строк

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
- `Node.finetune_conversation_examples[].transcript[].content` — максимум 10 000 символов на одну реплику.
- `Node.finetune_transition_examples[].transcript[].content` — максимум 10 000 символов на одну реплику.
- `Node.global_node_setting.positive_finetune_examples[].transcript[].content` — максимум 10 000 символов на одну реплику.
- `Node.global_node_setting.negative_finetune_examples[].transcript[].content` — максимум 10 000 символов на одну реплику.

### Лимиты массивов

- `nodes` — максимум 1000 нод в одном workflow.
- `Node.edges` — максимум 50 переходов на одну ноду.
- `Node.variables` — максимум 100 переменных на одну ноду.
- `Node.variables[].choices` — максимум 100 вариантов enum.
- `Node.finetune_conversation_examples` — максимум 50 примеров на одну ноду.
- `Node.finetune_transition_examples` — максимум 50 примеров на одну ноду.
- `Node.global_node_setting.positive_finetune_examples` — максимум 50 примеров на одну ноду.
- `Node.global_node_setting.negative_finetune_examples` — максимум 50 примеров на одну ноду.
- `transcript` внутри любого примера — максимум 100 реплик.

### Лимиты числовых полей

- `Agent.interruption_sensitivity` — от 0.0 до 1.0.
- `Agent.max_call_duration_ms` — от 60 000 до 14 400 000 миллисекунд.
- `Agent.tts.speed` — от 0.5 до 2.0, если поле передано.
- `ConversationFlow.model_temperature` — от 0.0 до 2.0.
- `ConversationFlow.kb_config.top_k` — от 1 до 20.
- `ConversationFlow.kb_config.filter_score` — от 0.0 до 1.0.
- `Node.voice_speed` — от 0.5 до 2.0, если поле передано.
- `Node.responsiveness` — от 0.0 до 1.0, если поле передано.
- `Node.global_node_setting.cool_down` — от 0 до 100.
- `display_position.x`, `display_position.y`, `begin_tag_display_position.x`, `begin_tag_display_position.y` — числа в диапазоне от -100000 до 100000.

### Обязательные проверки структуры workflow

- Все `Node.id` внутри одного workflow должны быть уникальными.
- `start_node_id` должен ссылаться на существующую ноду из `nodes`.
- Каждый `edges[].destination_node_id`, если он передан, должен ссылаться на существующую ноду из `nodes`.
- `Node.type` не должен быть пустым.
- `Node.name` не должен быть пустым.
- `Node.instruction.type` должен быть только `static_text` или `prompt`, если `instruction` передан.
- `start_speaker` должен быть только `agent` или `user`.
- `transcript[].role` должен быть только `user` или `agent`.
- `variables[].type` должен быть только `string`, `number`, `boolean` или `enum`.
- Если `variables[].type = enum`, должен быть передан непустой массив `choices`.

### Лимиты размера JSON

- Максимальный размер JSON-тела запроса — 8 MB.
- Максимальный размер одного workflow после сериализации в JSON — 8 MB.

Если пользователь превышает любой лимит, сервис должен вернуть `400 Bad Request` с точным `field`, `code`, `message` и `limit`.

Для бизнес-ограничений публикации и удаления использовать тот же единый формат ошибки. Примеры кодов: `published_version_is_readonly`, `cannot_delete_published_version`, `agent_has_published_workflow`, `published_workflow_not_found`.

---

## Node — полная структура

Каждая нода в массиве `nodes` **должна точно соответствовать** формату Retell AI. Сервис обязан хранить и возвращать данные **точно в том виде**, в котором их отправляет/принимает Retell AI.

```jsonc
{
  "id": "string",                                      // обязательно, пример: "start-node-1777982620634" или "node-1777985599819"
  "type": "string",                                    // обязательно: "conversation" | "end" | "extract_dynamic_variables" | другие типы Retell AI
  "name": "string",                                    // человекочитаемое название ноды

  "instruction": {
    "type": "static_text" | "prompt",                  // тип инструкции: статический текст или prompt
    "text": "string"                                   // текст промпта. Может содержать динамические переменные {{current_time}}, {{user_name}} и т.д.
  },

  "edges": [                                           // массив переходов из текущей ноды
    {
      "id": "string",                                  // ID перехода
      "destination_node_id": "string",                 // ID следующей ноды
      "transition_condition": {                         // условие перехода
        "type": "prompt",                              // тип условия, обычно prompt
        "prompt": "string"                              // текстовое условие перехода
      }
    }
  ],

  "global_node_setting": {                              // глобальные настройки ноды
    "condition": "string",                             // условие, при котором нода может быть вызвана из любой точки графа
    "go_back_conditions": [                             // условия возврата обратно в основной сценарий
      {
        "id": "string",
        "transition_condition": {
          "type": "prompt",
          "prompt": "string"
        }
      }
    ],
    "positive_finetune_examples": [                    // примеры положительных диалогов
      {
        "transcript": [
          { "role": "user", "content": "Здравствуйте! Какая у вас есть пицца?" },
          { "role": "agent", "content": "Одну минутку, уточняю" }
        ]
      }
    ],
    "negative_finetune_examples": [                    // примеры отрицательных диалогов
      {
        "transcript": [
          { "role": "user", "content": "..." },
          { "role": "agent", "content": "..." }
        ]
      }
    ],
    "cool_down": "number"                              // количество следующих нод/вызовов, после которых глобальную ноду можно вызвать снова. Это не секунды
  },

  "variables": [                                       // только для типа ноды extract_dynamic_variables
    {
      "name": "string",                                // имя переменной, например "имя_пользователя"
      "description": "string",                         // описание переменной
      "type": "string" | "number" | "boolean" | "enum" // тип переменной. При enum может быть дополнительное поле choices
    }
  ],

  "voice_speed": "number",                             // скорость речи в этой ноде, 1.0 = нормальная скорость
  "responsiveness": "number",                          // скорость реакции агента в этой ноде
  "finetune_conversation_examples": [                  // примеры диалогов для улучшения ответов ноды
    {
      "transcript": [
        { "role": "user", "content": "..." },
        { "role": "agent", "content": "..." }
      ]
    }
  ],
  "finetune_transition_examples": [                    // примеры переходов между нодами
    {
      "id": "string",
      "transcript": [
        { "role": "user", "content": "..." },
        { "role": "agent", "content": "..." }
      ],
      "destination_node_id": "string"
    }
  ],

  "display_position": {                                // позиция ноды на холсте фронтенда
    "x": "number",
    "y": "number"
  },

  "start_speaker": "agent" | "user"                    // кто начинает говорить в этой ноде
}
```

Ключевые правила по Node:

- Динамические переменные {{current_time}}, {{user_name}} и т.п. — это просто текст внутри instruction.text. Сервис их не обрабатывает и не подставляет — хранит как есть.
- При обновлении workflow клиент всегда присылает полный массив nodes целиком.
- Структура должна быть максимально близка к оригинальной Retell AI, чтобы в будущем не было проблем с совместимостью.

---

## Эндпоинты (/api/v1/)

### Folders

- GET /folders
- POST /folders
- PATCH /folders/{folderId}
- DELETE /folders/{folderId}

### Agents

- GET /agents?page=1&limit=20&folderId=... (лёгкий список: id, name, voice_id, last_modified, folder_id; поддержка пагинации, limit по умолчанию 20, максимум 100)
- POST /agents → создать агента, автоматически создать ConversationFlow версии 0 из backend-шаблона, создать PublishedWorkflow и записать опубликованную версию в `agent.response_engine`
- GET /agents/{agentId}
- PATCH /agents/{agentId} → частичное обновление top-level полей агента, включая `tts`, `stt` и `folder_id` для перемещения между папками. Если поле `folder_id` отсутствует — папка агента не изменяется. Если `folder_id: null` — у агента сбрасывается привязка к обычной папке, и он остаётся отображаться в виртуальной папке "Template Agents".
- DELETE /agents/{agentId} → удалить агента и все его версии workflow только если у агента нет опубликованного workflow. Если PublishedWorkflow существует, вернуть ошибку `agent_has_published_workflow` и потребовать сначала снять публикацию

### Conversation Flows

- GET /agents/{agentId}/conversation-flows → получить лёгкий список всех версий workflow агента без полного массива `nodes`. Ответ должен содержать минимум: `conversation_flow_id`, `version`, `created_at`, `updated_at`, `published`. Поле `published` вычисляется через PublishedWorkflow и не хранится в ConversationFlow.
- GET /agents/{agentId}/conversation-flows/published → получить опубликованный workflow агента по PublishedWorkflow
- GET /agents/{agentId}/conversation-flows/{conversationFlowId}?version=0
- PATCH /agents/{agentId}/conversation-flows/{conversationFlowId}?version=0 ← использует формат top-level обновления
- POST /agents/{agentId}/conversation-flows/{conversationFlowId}/versions?fromVersion=0 → создать новую версию как копию версии, указанной в `fromVersion`. Новая версия не становится опубликованной автоматически
- POST /agents/{agentId}/conversation-flows/{conversationFlowId}/publish?version=0 → опубликовать указанную версию workflow. Если ранее была опубликована другая версия, PublishedWorkflow переключается на новую версию
- POST /agents/{agentId}/conversation-flows/unpublish → снять публикацию с текущего опубликованного workflow агента: удалить **только** запись PublishedWorkflow (production-публикацию). Для runtime-эндпоинтов `conversation_flow_id` и `version` берутся только из PublishedWorkflow; если PublishedWorkflow отсутствует — production-запуск невозможен. Frontend/editor эндпоинты продолжают работать по явно запрошенной версии flow.
- DELETE /agents/{agentId}/conversation-flows/{conversationFlowId}?version=0 → удалить конкретную неопубликованную версию workflow. Если версия опубликована, вернуть ошибку `cannot_delete_published_version`

### Runtime endpoints для микросервиса звонков

- GET /runtime/workspaces/published-agents → получить список всех workspace, в которых есть опубликованные агенты, и список `agent_id` внутри каждого workspace
- GET /runtime/workspaces/{workspaceId}/published-agents → получить список `agent_id`, у которых есть PublishedWorkflow и которые готовы к production-запуску в конкретном workspace
- GET /runtime/workspaces/{workspaceId}/agents/{agentId}/published-config → получить полный склеенный runtime config: Agent + опубликованный ConversationFlow + PublishedWorkflow summary. Версия workflow выбирается автоматически через PublishedWorkflow, микросервис звонков version не передаёт

---


---

## API Contract

Все endpoints находятся под префиксом `/api/v1`.

Все обычные frontend/admin API-запросы должны содержать заголовок `X-Workspace-Id`. Если заголовок отсутствует, сервис возвращает `400 Bad Request`.

Все обычные frontend/admin API-операции должны фильтровать данные по `workspace_id`. Нельзя возвращать или изменять данные другого workspace.

Исключение: runtime endpoints для микросервиса звонков описаны отдельно. Они являются внутренними endpoint'ами между микросервисами, не требуют пользовательских headers и получают `workspace_id`/`agent_id` напрямую в path/query параметрах.

### Общий формат успешного ответа

Для одиночных объектов:

```json
{
  "data": { }
}
```

Для списков:

```json
{
  "data": [ ],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 100
  }
}
```

Если endpoint не использует пагинацию, `meta` можно не возвращать.

### Правило симметрии request/response

Для endpoints, которые создают или обновляют ресурс, request body должен повторять структуру соответствующего ресурса из response.

Главное правило:

- `POST` может принимать часть полной структуры ресурса + использовать backend defaults.
- `PATCH` принимает ту же структуру ресурса, что возвращается в response, но все поля опциональны.
- В PATCH клиент передаёт только тот объект или поле, которое хочет изменить.
- Response после `POST` и `PATCH` должен возвращать полную актуальную структуру созданного или обновлённого ресурса, если endpoint не описан как action endpoint.

Примеры:

- `GET /agents/{agentId}` возвращает полный Agent.
- `PATCH /agents/{agentId}` принимает частичный Agent и возвращает полный обновлённый Agent.
- `GET /agents/{agentId}/conversation-flows/{conversationFlowId}?version=1` возвращает полный ConversationFlow.
- `PATCH /agents/{agentId}/conversation-flows/{conversationFlowId}?version=1` принимает частичный ConversationFlow и возвращает полный обновлённый ConversationFlow.

Для лёгких списков можно возвращать сокращённые объекты:

- `GET /agents` возвращает лёгкий список агентов.
- `GET /agents/{agentId}/conversation-flows` возвращает лёгкий список версий workflow без полного массива `nodes`.

### Правило вложенных объектов

Если PATCH-запрос содержит вложенный объект, например `tts`, `stt`, `model_choice`, `kb_config`, то этот вложенный объект обновляется целиком.

Пример:

```json
{
  "tts": {
    "voice_id": "retell-Marissa",
    "language": "en-US",
    "emotion": null,
    "speed": 1.1
  }
}
```

Такой запрос полностью заменяет объект `tts` у Agent.

Если нужно изменить только `tts.speed`, клиент всё равно должен отправить полный объект `tts`, чтобы не было неоднозначных partial merge внутри вложенных объектов.

### Правило полноты response

Для `GET /agents/{agentId}`, `POST /agents` и `PATCH /agents/{agentId}` response должен возвращать полную структуру Agent.

Для `GET /agents/{agentId}/conversation-flows/{conversationFlowId}?version=0`, `PATCH /agents/{agentId}/conversation-flows/{conversationFlowId}?version=0` и `POST /agents/{agentId}/conversation-flows/{conversationFlowId}/versions?fromVersion=0` response должен возвращать полную структуру ConversationFlow.

Для action endpoints можно возвращать короткий результат действия:

- `POST /publish` возвращает PublishedWorkflow summary.
- `POST /unpublish` возвращает `204 No Content`.
- `DELETE` возвращает `204 No Content`.

### Общий формат ошибки

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

Поля ошибки:

- `type` — тип ошибки: `validation_error`, `business_error`, `not_found`, `internal_error`.
- `field` — путь до поля, если ошибка связана с конкретным полем. Если поле неприменимо, использовать `null`.
- `code` — машинно-читаемый код ошибки.
- `message` — понятное человеку описание ошибки.
- `limit` — лимит, если ошибка связана с лимитом. Если лимита нет, использовать `null`.

### Общие HTTP status codes

- `200 OK` — успешное получение или обновление данных.
- `201 Created` — успешное создание ресурса.
- `204 No Content` — успешное удаление или действие без тела ответа.
- `400 Bad Request` — ошибка валидации или бизнес-ограничение.
- `404 Not Found` — ресурс не найден в текущем workspace.
- `409 Conflict` — конфликт состояния, например попытка создать дубликат папки, если будет введено ограничение уникальности имени.
- `500 Internal Server Error` — внутренняя ошибка сервиса.

---

## API Contract: Folders

### GET /api/v1/folders

Получить список обычных папок workspace. Виртуальная папка "Template Agents" не хранится в MongoDB, но может быть добавлена в response как виртуальный элемент, если это удобно фронтенду.

Headers:

```http
X-Workspace-Id: ws_abc123xyz
X-User-Id: user_123
X-User-Scopes: agents:read
```

Query params: отсутствуют.

Response `200 OK`:

```json
{
  "data": [
    {
      "folder_id": "folder_5072e604e6063f693c10de4a",
      "workspace_id": "ws_abc123xyz",
      "name": "Sales Agents",
      "created_at": 1774384312548,
      "updated_at": 1777982029068
    }
  ]
}
```

Errors:

- `400 missing_workspace_id` — отсутствует `X-Workspace-Id`.

---

### POST /api/v1/folders

Создать обычную папку. Папку "Template Agents" создать нельзя.

Request body:

```json
{
  "name": "Sales Agents"
}
```

Validation:

- `name` обязателен;
- `name` максимум 255 символов;
- `name` не может быть `Template Agents`.

Response `201 Created`:

```json
{
  "data": {
    "folder_id": "folder_5072e604e6063f693c10de4a",
    "workspace_id": "ws_abc123xyz",
    "name": "Sales Agents",
    "created_at": 1774384312548,
    "updated_at": 1774384312548
  }
}
```

Errors:

- `400 validation_error`;
- `400 folder_limit_exceeded`;
- `400 template_folder_is_virtual`.

---

### PATCH /api/v1/folders/{folderId}

Переименовать обычную папку. Виртуальную папку "Template Agents" переименовать нельзя.

Path params:

- `folderId` — ID папки.

Request body:

```json
{
  "name": "Updated Sales Agents"
}
```

Response `200 OK`:

```json
{
  "data": {
    "folder_id": "folder_5072e604e6063f693c10de4a",
    "workspace_id": "ws_abc123xyz",
    "name": "Updated Sales Agents",
    "created_at": 1774384312548,
    "updated_at": 1777982029068
  }
}
```

Errors:

- `400 validation_error`;
- `404 folder_not_found`;
- `400 template_folder_is_virtual`.

---

### DELETE /api/v1/folders/{folderId}

Удалить обычную папку. При удалении папки у всех агентов этой папки сбрасывается `folder_id`.

Path params:

- `folderId` — ID папки.

Response `204 No Content`.

Errors:

- `404 folder_not_found`;
- `400 template_folder_is_virtual`.

---

## API Contract: Agents

### GET /api/v1/agents

Получить лёгкий список агентов workspace.

Query params:

- `page` — номер страницы, по умолчанию `1`.
- `limit` — размер страницы, по умолчанию `20`, максимум `100`.
- `folderId` — опциональный фильтр по обычной папке.

Response `200 OK`:

```json
{
  "data": [
    {
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "name": "Customer Support RU",
      "voice_id": "retell-Cimo",
      "language": "ru-RU",
      "folder_id": "folder_5072e604e6063f693c10de4a",
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

Errors:

- `400 validation_error`.

---

### POST /api/v1/agents

Создать агента. Сервис автоматически создаёт ConversationFlow версии 0 из backend-шаблона, создаёт PublishedWorkflow и записывает опубликованную версию в `agent.response_engine`.

Request body имеет ту же форму, что и Agent, но при создании можно передать только часть полей. Остальные значения берутся из backend-шаблона.

Request body example:

```json
{
  "name": "Customer Support RU",
  "folder_id": null,
  "channel": "voice",
  "voice_id": "retell-Cimo",
  "language": "ru-RU",
  "tts": {
    "voice_id": "retell-Cimo",
    "language": "ru-RU",
    "emotion": null,
    "speed": 1.0
  },
  "stt": {
    "model_id": "default",
    "language": "ru-RU"
  },
  "interruption_sensitivity": 0.9,
  "max_call_duration_ms": 3600000,
  "normalize_for_speech": true,
  "allow_user_dtmf": false,
  "user_dtmf_options": { },
  "handbook_config": {
    "default_personality": true,
    "ai_disclosure": true
  },
  "pii_config": {
    "mode": "post_call",
    "categories": [ ]
  },
  "data_storage_setting": "everything"
}
```

Response `201 Created` возвращает полную структуру Agent:

```json
{
  "data": {
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "folder_id": null,
    "name": "Customer Support RU",
    "channel": "voice",
    "voice_id": "retell-Cimo",
    "language": "ru-RU",
    "tts": {
      "voice_id": "retell-Cimo",
      "language": "ru-RU",
      "emotion": null,
      "speed": 1.0
    },
    "stt": {
      "model_id": "default",
      "language": "ru-RU"
    },
    "interruption_sensitivity": 0.9,
    "max_call_duration_ms": 3600000,
    "normalize_for_speech": true,
    "allow_user_dtmf": false,
    "user_dtmf_options": { },
    "response_engine": {
      "type": "conversation-flow",
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 0
    },
    "handbook_config": {
      "default_personality": true,
      "ai_disclosure": true
    },
    "pii_config": {
      "mode": "post_call",
      "categories": [ ]
    },
    "data_storage_setting": "everything",
    "created_at": 1777982621600,
    "updated_at": 1777982621600,
    "last_modified": 1777982621600
  }
}
```

Errors:

- `400 validation_error`;
- `400 agent_limit_exceeded`;
- `404 folder_not_found`.

---

### GET /api/v1/agents/{agentId}

Получить полный Agent.

Path params:

- `agentId` — ID агента.

Response `200 OK`:

```json
{
  "data": {
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "folder_id": null,
    "name": "Customer Support RU",
    "channel": "voice",
    "voice_id": "retell-Cimo",
    "language": "ru-RU",
    "tts": {
      "voice_id": "retell-Cimo",
      "language": "ru-RU",
      "emotion": null,
      "speed": 1.0
    },
    "stt": {
      "model_id": "default",
      "language": "ru-RU"
    },
    "interruption_sensitivity": 0.9,
    "max_call_duration_ms": 3600000,
    "normalize_for_speech": true,
    "allow_user_dtmf": false,
    "user_dtmf_options": { },
    "response_engine": {
      "type": "conversation-flow",
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 0
    },
    "handbook_config": {
      "default_personality": true,
      "ai_disclosure": true
    },
    "pii_config": {
      "mode": "post_call",
      "categories": [ ]
    },
    "data_storage_setting": "everything",
    "created_at": 1777982621600,
    "updated_at": 1777983018802,
    "last_modified": 1777983018802
  }
}
```

Errors:

- `404 agent_not_found`.

---

### PATCH /api/v1/agents/{agentId}

Частично обновить top-level поля Agent. Через этот endpoint обновляются `tts`, `stt`, `voice_id`, `language`, `folder_id` и другие настройки агента.

Важно: Agent не версионируется. Изменения Agent применяются сразу ко всем последующим запускам агента.

Path params:

- `agentId` — ID агента.

Request body имеет ту же форму, что и полный Agent, но все поля опциональны. Клиент передаёт только те поля, которые хочет изменить.

Request body example:

```json
{
  "name": "Updated Support Agent",
  "folder_id": "folder_5072e604e6063f693c10de4a",
  "voice_id": "retell-Marissa",
  "language": "en-US",
  "tts": {
    "voice_id": "retell-Marissa",
    "language": "en-US",
    "emotion": null,
    "speed": 1.1
  },
  "stt": {
    "model_id": "default",
    "language": "en-US"
  }
}
```

Правила `folder_id`:

- если `folder_id` отсутствует — папка агента не изменяется;
- если `folder_id: null` — у агента сбрасывается привязка к обычной папке;
- если передан `folder_id` — агент перемещается в указанную папку.

Response `200 OK` возвращает полную обновлённую структуру Agent:

```json
{
  "data": {
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "folder_id": "folder_5072e604e6063f693c10de4a",
    "name": "Updated Support Agent",
    "channel": "voice",
    "voice_id": "retell-Marissa",
    "language": "en-US",
    "tts": {
      "voice_id": "retell-Marissa",
      "language": "en-US",
      "emotion": null,
      "speed": 1.1
    },
    "stt": {
      "model_id": "default",
      "language": "en-US"
    },
    "interruption_sensitivity": 0.9,
    "max_call_duration_ms": 3600000,
    "normalize_for_speech": true,
    "allow_user_dtmf": false,
    "user_dtmf_options": { },
    "response_engine": {
      "type": "conversation-flow",
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 0
    },
    "handbook_config": {
      "default_personality": true,
      "ai_disclosure": true
    },
    "pii_config": {
      "mode": "post_call",
      "categories": [ ]
    },
    "data_storage_setting": "everything",
    "created_at": 1777982621600,
    "updated_at": 1777984000000,
    "last_modified": 1777984000000
  }
}
```

Errors:

- `400 validation_error`;
- `404 agent_not_found`;
- `404 folder_not_found`.

---

### DELETE /api/v1/agents/{agentId}

Удалить агента и все его версии workflow только если у агента нет опубликованного workflow.

Path params:

- `agentId` — ID агента.

Response `204 No Content`.

Errors:

- `404 agent_not_found`;
- `400 agent_has_published_workflow`.

---

## API Contract: Conversation Flows

### GET /api/v1/agents/{agentId}/conversation-flows

Получить лёгкий список всех версий workflow агента без полного массива `nodes`.

Path params:

- `agentId` — ID агента.

Response `200 OK`:

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

`published` вычисляется через PublishedWorkflow и не хранится в ConversationFlow.

Errors:

- `404 agent_not_found`.

---

### GET /api/v1/agents/{agentId}/conversation-flows/published

Получить опубликованный workflow агента.

Response `200 OK`:

```json
{
  "data": {
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "version": 0,
    "published": true,
    "nodes": [ ],
    "start_node_id": "start-node-1777982620634",
    "start_speaker": "agent",
    "global_prompt": "Ты дружелюбный помощник поддержки. {{current_time}}"
  }
}
```

Errors:

- `404 agent_not_found`;
- `404 published_workflow_not_found`.

---

### GET /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0

Получить конкретную версию workflow полностью.

Query params:

- `version` — номер версии workflow.

Response `200 OK`:

```json
{
  "data": {
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "version": 1,
    "published": false,
    "nodes": [ ],
    "start_node_id": "start-node-1777982620634",
    "start_speaker": "agent",
    "global_prompt": "Ты дружелюбный помощник поддержки. {{current_time}}",
    "model_choice": {
      "type": "cascading",
      "model": "gpt-5.4",
      "high_priority": true
    },
    "model_temperature": 0.7,
    "flex_mode": true,
    "tool_call_strict_mode": true,
    "kb_config": {
      "top_k": 3,
      "filter_score": 0.6
    },
    "begin_tag_display_position": {
      "x": 120,
      "y": 80
    },
    "is_transfer_cf": false,
    "created_at": 1777983900000,
    "updated_at": 1777984000000
  }
}
```

Errors:

- `404 agent_not_found`;
- `404 conversation_flow_not_found`.

---


## API Contract: Runtime endpoints для микросервиса звонков

### GET /api/v1/runtime/workspaces/published-agents

Получить список всех workspaces, в которых есть хотя бы один опубликованный агент, и список опубликованных `agent_id` внутри каждого workspace.

Этот endpoint нужен микросервису звонков для общего discovery: сначала получить все workspace с опубликованными агентами, затем по конкретному `workspace_id + agent_id` получить полный runtime config через `/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config`.

Request headers: не требуются.

Query params: отсутствуют.

Response `200 OK`:

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
        },
        {
          "agent_id": "agent_f7f09c6e4ca4a5133116a20e70",
          "conversation_flow_id": "conversation_flow_b1743a9d8e12",
          "version": 0,
          "published_at": 1777987100000
        }
      ]
    },
    {
      "workspace_id": "ws_def456qwe",
      "agents": [
        {
          "agent_id": "agent_111111111111111111111111",
          "conversation_flow_id": "conversation_flow_222222222222",
          "version": 1,
          "published_at": 1777987200000
        }
      ]
    }
  ]
}
```

Правила:

- Endpoint агрегирует данные из PublishedWorkflow.
- В response попадают только workspaces, у которых есть минимум один PublishedWorkflow.
- Внутри каждого workspace возвращаются только агенты, у которых есть PublishedWorkflow.
- Endpoint не возвращает полный Agent и не возвращает полный ConversationFlow.
- Endpoint нужен только для discovery. Для получения настроек конкретного агента нужно вызвать `GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config`.
- Endpoint только читает данные и ничего не изменяет в базе.

Errors:

- `500 internal_error` — если не удалось прочитать PublishedWorkflow.

---

Эти endpoints предназначены только для внутреннего микросервиса звонков.

Они не используются фронтендом и не являются частью обычного пользовательского API.

Для этих endpoints не требуется передавать пользовательские headers:

- `X-User-Id`;
- `X-Workspace-Id`;
- `X-User-Scopes`.

`workspace_id` и `agent_id` передаются напрямую через path/query параметры.

Runtime endpoints ничего не создают и не изменяют. Они только отдают данные, необходимые микросервису звонков для запуска опубликованного агента.

### GET /api/v1/runtime/workspaces/{workspaceId}/published-agents

Получить список `agent_id`, которые имеют опубликованный workflow и готовы к production-запуску в указанном workspace.

Этот endpoint нужен микросервису звонков, чтобы быстро получить список агентов, доступных для запуска.

После получения `agent_id` из этого списка микросервис звонков может получить полный runtime config конкретного агента через `GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config`.

Path params:

- `workspaceId` — ID workspace.

Response `200 OK`:

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

Правила:

* В список попадают только агенты, у которых есть запись PublishedWorkflow.
* Если у агента нет PublishedWorkflow, он не готов к production-запуску и не должен попадать в список.
* Endpoint не возвращает полный Agent и не возвращает полный ConversationFlow. Он отдаёт только список опубликованных связок для быстрого discovery.

Errors:

* 404 workspace_not_found, если такая проверка workspace реализована отдельно.

---

GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
Получить полный runtime config для production-запуска агента микросервисом звонков.

Микросервис звонков передаёт только workspaceId и agentId. Версию workflow он не передаёт.

AgentService сам выбирает опубликованную версию через PublishedWorkflow.

Path params:

* workspaceId — ID workspace.
* agentId — ID агента.

Алгоритм работы endpoint:

1. Найти Agent по workspace_id + agent_id.
2. Найти PublishedWorkflow по workspace_id + agent_id.
3. Из PublishedWorkflow получить conversation_flow_id и version.
4. Найти конкретный ConversationFlow по workspace_id + agent_id + conversation_flow_id + version.
5. Динамически заполнить agent.response_engine.conversation_flow_id и agent.response_engine.version опубликованными значениями из PublishedWorkflow.
6. Вернуть единый склеенный JSON: настройки Agent + настройки опубликованного ConversationFlow + PublishedWorkflow summary.

Response 200 OK:
{
  "data": {
    "agent": {
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "workspace_id": "ws_abc123xyz",
      "folder_id": null,
      "name": "Customer Support RU",
      "channel": "voice",
      "voice_id": "retell-Cimo",
      "language": "ru-RU",
      "tts": {
        "voice_id": "retell-Cimo",
        "language": "ru-RU",
        "emotion": null,
        "speed": 1.0
      },
      "stt": {
        "model_id": "default",
        "language": "ru-RU"
      },
      "interruption_sensitivity": 0.9,
      "max_call_duration_ms": 3600000,
      "normalize_for_speech": true,
      "allow_user_dtmf": false,
      "user_dtmf_options": { },
      "response_engine": {
        "type": "conversation-flow",
        "conversation_flow_id": "conversation_flow_c530702321b8",
        "version": 2
      },
      "handbook_config": {
        "default_personality": true,
        "ai_disclosure": true
      },
      "pii_config": {
        "mode": "post_call",
        "categories": [ ]
      },
      "data_storage_setting": "everything",
      "created_at": 1777982621600,
      "updated_at": 1777983018802,
      "last_modified": 1777983018802
    },
    "conversation_flow": {
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "workspace_id": "ws_abc123xyz",
      "version": 2,
      "published": true,
      "nodes": [ ],
      "start_node_id": "start-node-1777982620634",
      "start_speaker": "agent",
      "global_prompt": "Ты дружелюбный помощник поддержки. {{current_time}}",
      "model_choice": {
        "type": "cascading",
        "model": "gpt-5.4",
        "high_priority": true
      },
      "model_temperature": 0.7,
      "flex_mode": true,
      "tool_call_strict_mode": true,
      "kb_config": {
        "top_k": 3,
        "filter_score": 0.6
      },
      "begin_tag_display_position": {
        "x": 120,
        "y": 80
      },
      "is_transfer_cf": false,
      "created_at": 1777983900000,
      "updated_at": 1777985000000
    },
    "published_workflow": {
      "workspace_id": "ws_abc123xyz",
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "version": 2,
      "published_at": 1777987000000
    }
  }
}

Правила:

* Микросервис звонков не передаёт conversation_flow_id и version.
* Версия workflow выбирается автоматически через PublishedWorkflow.
* Runtime config всегда должен содержать именно опубликованную версию ConversationFlow.
* agent.response_engine.conversation_flow_id и agent.response_engine.version в этом response должны быть динамически заполнены опубликованными значениями из PublishedWorkflow.
* Response является склеенным runtime JSON и содержит всё необходимое для запуска звонка: настройки Agent, настройки опубликованного ConversationFlow и PublishedWorkflow summary.
* Endpoint только читает данные и ничего не изменяет в базе.

Errors:

* 404 agent_not_found;
* 404 published_workflow_not_found;
* 404 conversation_flow_not_found.


---

### PATCH /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0

Обновить конкретную неопубликованную версию workflow top-level полями.

Query params:

- `version` — номер версии workflow.

Request body имеет ту же форму, что и полный ConversationFlow, но все поля опциональны. Клиент передаёт только те поля, которые хочет изменить.

Request body:

```json
{
  "nodes": [ ],
  "global_prompt": "Новый глобальный промпт",
  "model_choice": {
    "type": "cascading",
    "model": "gpt-5.4",
    "high_priority": true
  },
  "model_temperature": 0.7,
  "start_node_id": "start-node-1777982620634",
  "start_speaker": "agent",
  "flex_mode": true,
  "tool_call_strict_mode": true,
  "kb_config": {
    "top_k": 3,
    "filter_score": 0.6
  }
}
```

Rules:

- Если поле отсутствует в PATCH body — оно не обновляется.
- `nodes`, если передан, заменяет полный массив нод целиком.
- Опубликованную версию редактировать нельзя.

Response `200 OK`:

```json
{
  "data": {
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "version": 1,
    "published": false,
    "nodes": [ ],
    "start_node_id": "start-node-1777982620634",
    "start_speaker": "agent",
    "global_prompt": "Новый глобальный промпт",
    "model_choice": {
      "type": "cascading",
      "model": "gpt-5.4",
      "high_priority": true
    },
    "model_temperature": 0.7,
    "flex_mode": true,
    "tool_call_strict_mode": true,
    "kb_config": {
      "top_k": 3,
      "filter_score": 0.6
    },
    "begin_tag_display_position": {
      "x": 120,
      "y": 80
    },
    "is_transfer_cf": false,
    "created_at": 1777983900000,
    "updated_at": 1777985000000
  }
}
```

Errors:

- `400 validation_error`;
- `400 published_version_is_readonly`;
- `404 conversation_flow_not_found`.

---

### POST /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/versions?fromVersion=0

Создать новую версию workflow как копию версии, указанной в `fromVersion`.

Query params:

- `fromVersion` — версия, которую нужно скопировать.

Request body: пустой объект или отсутствует.

```json
{ }
```

Response `201 Created`:

```json
{
  "data": {
    "conversation_flow_id": "conversation_flow_c530702321b8",
    "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
    "workspace_id": "ws_abc123xyz",
    "version": 2,
    "published": false,
    "nodes": [ ],
    "start_node_id": "start-node-1777982620634",
    "start_speaker": "agent",
    "global_prompt": "Ты дружелюбный помощник поддержки. {{current_time}}",
    "model_choice": {
      "type": "cascading",
      "model": "gpt-5.4",
      "high_priority": true
    },
    "model_temperature": 0.7,
    "flex_mode": true,
    "tool_call_strict_mode": true,
    "kb_config": {
      "top_k": 3,
      "filter_score": 0.6
    },
    "begin_tag_display_position": {
      "x": 120,
      "y": 80
    },
    "is_transfer_cf": false,
    "created_at": 1777986000000,
    "updated_at": 1777986000000
  }
}
```

Errors:

- `400 workflow_version_limit_exceeded`;
- `404 conversation_flow_not_found`;
- `404 source_version_not_found`.

---

### POST /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}/publish?version=0

Опубликовать указанную версию workflow. PublishedWorkflow переключается на выбранную версию.

Query params:

- `version` — версия, которую нужно опубликовать.

Request body: пустой объект или отсутствует.

```json
{ }
```

Response `200 OK`:

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

Errors:

- `404 agent_not_found`;
- `404 conversation_flow_not_found`;
- `404 workflow_version_not_found`.

---

### POST /api/v1/agents/{agentId}/conversation-flows/unpublish

Снять публикацию с текущего опубликованного workflow агента.

Request body: пустой объект или отсутствует.

```json
{ }
```

Response `204 No Content`.

Errors:

- `404 agent_not_found`;
- `404 published_workflow_not_found`.

---

### DELETE /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0

Удалить конкретную неопубликованную версию workflow.

Query params:

- `version` — версия, которую нужно удалить.

Response `204 No Content`.

Errors:

- `404 conversation_flow_not_found`;
- `400 cannot_delete_published_version`.

На один workspace:

- Максимум 100 агентов
- Максимум 50 обычных папок (Template Agents не считается)
- Максимум 100 версий workflow на один agent
- Максимум 1000 нод в одном workflow
- Максимальный размер одного workflow (JSON) — 8 МБ

При превышении лимита возвращать ошибку 400 Bad Request с понятным текстом.

Дополнительные защиты:

- Максимальный размер JSON-тела запроса — 8MB.
- Максимальная длина строки name — 255 символов.
- Максимальная длина global_prompt и instruction.text — 50 000 символов.
- Подробные лимиты для нод, промптов, transitions, условий, переменных и примеров диалогов описаны в разделе "Валидация пользовательского контента workflow".
- При попытке создать/обновить больше лимита — возвращать `400 Bad Request` через общий helper ошибки валидации.

---

## Правила разработки (строго соблюдать)

- Сначала создай файл **RULES.md** в корне проекта.
- В RULES.md подробно опиши все модели данных, форматы запросов/ответов, архитектуру, правила валидации и все эндпоинты.
- Код должен быть покрыт unit-тестами (go test + testify).
- Все входные данные обязательно валидируются.
- Повторяющийся код выносить в helper-функции.
- Использовать готовые решения там, где это возможно.
- Код должен быть максимально чистым, читаемым и легко модифицируемым в будущем.
- Никакого over-engineering.

---

## Этапы работы

1. Создай файл **RULES.md** со всеми структурами данных, форматом обновления workflow, описанием архитектуры и всеми эндпоинтами.
2. Покажи мне содержимое RULES.md.
3. Только после моего подтверждения начинай реализовывать весь сервис по этому файлу.

Начинай работу прямо сейчас.