

Окей, я посмотрел отчёт по фактическому поведению.

Нужно доработать реализацию согласно актуальному ТЗ в файле:

`CS-VoiceAgent/second/AgentsService/docs/AgentsService-doc.md`

Ориентируйся на разделы:

1. `## Заголовки запросов (обязательно)`
2. `## API Contract`
3. `## Эндпоинты (/api/v1/)`
4. `## API Contract: Runtime endpoints для микросервиса звонков`
5. `## Публикация workflow и переключение версий`
6. `### Снятие публикации и удаление`

Сейчас по твоему отчёту видно, что основная логика Agent / ConversationFlow / PublishedWorkflow реализована нормально, но runtime API для микросервиса звонков отсутствует. Нужно исправить именно это.

---

## 1. Runtime endpoints должны быть реализованы отдельно

Сейчас по фактическому коду:

- `/api/v1/runtime/...` routes отсутствуют;
- runtime handlers отсутствуют;
- runtime usecase отсутствует;
- runtime repository methods отсутствуют;
- `RequireWorkspaceID` висит на всей группе `/api/v1`.

Это нужно исправить.

Добавь endpoints:

```http
GET /api/v1/runtime/workspaces/published-agents
GET /api/v1/runtime/workspaces/{workspaceId}/published-agents
GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config
```

Эти endpoints описаны в ТЗ в разделе:

```md
## API Contract: Runtime endpoints для микросервиса звонков
```

---

## 2. Runtime endpoints НЕ должны требовать пользовательские headers

Сейчас в коде используется логика примерно такого вида:

```go
api := r.Group("/api/v1")
api.Use(middleware.RequireWorkspaceID())
```

Из-за этого все `/api/v1/*` требуют `X-Workspace-Id`.

По актуальному ТЗ это неверно для runtime routes.

Нужно сделать так:

- обычные frontend/admin endpoints продолжают требовать `X-Workspace-Id`;
- runtime endpoints НЕ требуют:
  - `X-Workspace-Id`;
  - `X-User-Id`;
  - `X-User-Scopes`.

Для runtime endpoints `workspaceId` приходит из path params.

Предпочтительный вариант реализации:

```go
api := r.Group("/api/v1")

runtime := api.Group("/runtime")
// runtime routes без RequireWorkspaceID

protected := api.Group("")
protected.Use(middleware.RequireWorkspaceID())
// обычные frontend/admin routes
```

Можно сделать иначе, но важно, чтобы `/api/v1/runtime/*` не попадали под обязательную проверку `X-Workspace-Id`.

---

## 3. Реализовать GET /api/v1/runtime/workspaces/published-agents

Назначение:

Получить список всех workspace, в которых есть опубликованные агенты.

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
- возвращать только workspaces, у которых есть минимум один PublishedWorkflow;
- внутри workspace возвращать только опубликованных агентов;
- не возвращать полный Agent;
- не возвращать полный ConversationFlow;
- endpoint только читает данные.

Нужно добавить repository method для агрегации PublishedWorkflow по workspace.

Например:

```go
ListGroupedByWorkspace(ctx context.Context) ([]PublishedAgentsByWorkspace, error)
```

---

## 4. Реализовать GET /api/v1/runtime/workspaces/{workspaceId}/published-agents

Назначение:

Получить список опубликованных агентов в конкретном workspace.

Источник данных:

```text
PublishedWorkflow by workspace_id
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

Правила:

- если у агента нет PublishedWorkflow, он не попадает в список;
- не возвращать полный Agent;
- не возвращать полный ConversationFlow;
- endpoint только читает данные.

Нужно добавить repository method:

```go
ListByWorkspace(ctx context.Context, workspaceID string) ([]PublishedWorkflow, error)
```

---

## 5. Реализовать GET /api/v1/runtime/workspaces/{workspaceId}/agents/{agentId}/published-config

Это главный endpoint для микросервиса звонков.

Микросервис звонков передаёт только:

```text
workspaceId
agentId
```

Он НЕ передаёт:

```text
conversation_flow_id
version
```

AgentService сам выбирает опубликованную версию через PublishedWorkflow.

Алгоритм строго такой:

1. Найти Agent по `workspace_id + agent_id`.
2. Найти PublishedWorkflow по `workspace_id + agent_id`.
3. Из PublishedWorkflow взять:
   - `conversation_flow_id`;
   - `version`.
4. Найти ConversationFlow по:
   - `workspace_id`;
   - `agent_id`;
   - `conversation_flow_id`;
   - `version`.
5. В response динамически заполнить:

```json
"agent": {
  "response_engine": {
    "type": "conversation-flow",
    "conversation_flow_id": "<из PublishedWorkflow>",
    "version": "<из PublishedWorkflow>"
  }
}
```

6. Вернуть склеенный JSON:

```json
{
  "data": {
    "agent": {},
    "conversation_flow": {},
    "published_workflow": {}
  }
}
```

Пример response:

```json
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
      "response_engine": {
        "type": "conversation-flow",
        "conversation_flow_id": "conversation_flow_c530702321b8",
        "version": 2
      }
    },
    "conversation_flow": {
      "conversation_flow_id": "conversation_flow_c530702321b8",
      "agent_id": "agent_9bb2ac714ff6733eabdc922bdc",
      "workspace_id": "ws_abc123xyz",
      "version": 2,
      "published": true,
      "nodes": [],
      "start_node_id": "start-node-1777982620634",
      "start_speaker": "agent",
      "global_prompt": "..."
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
```

Ошибки:

- если Agent не найден → `agent_not_found`;
- если PublishedWorkflow не найден → `published_workflow_not_found`;
- если ConversationFlow не найден → `conversation_flow_not_found`.

Важно:

Runtime `published-config` должен брать `conversation_flow_id` и `version` только из PublishedWorkflow.

Нельзя брать version из request.

Нельзя брать version из старого `agent.response_engine` как из источника истины.

---

## 6. Frontend/editor endpoints не ломать, но нужно проверить response_engine

Сейчас по отчёту:

```text
GET /api/v1/agents/{agentId}/conversation-flows/{conversationFlowId}?version=0
```

возвращает ConversationFlow + `published`, но НЕ возвращает `response_engine`.

По актуальной логике ТЗ для frontend/editor response нужно, чтобы при запросе конкретной версии flow в response был `response_engine`, где:

```json
"response_engine": {
  "type": "conversation-flow",
  "conversation_flow_id": "<явно запрошенный conversationFlowId>",
  "version": "<явно запрошенная version>"
}
```

То есть:

- если фронт запросил `conversationFlowId=conversation_flow_xxx&version=0`;
- response должен содержать `response_engine.conversation_flow_id = conversation_flow_xxx`;
- response должен содержать `response_engine.version = 0`;
- это должно работать даже если workflow не опубликован;
- `published` при этом будет `false`, если PublishedWorkflow отсутствует.

Проверь актуальный `AgentsService-doc.md` и реализуй так, если это поле уже описано в контракте.

---

## 7. Unpublish не менять по логике кода

Сейчас по отчёту:

```text
Unpublish удаляет только PublishedWorkflow.
agent.response_engine не меняется.
```

Это соответствует актуальной логике.

Но нужно поправить старые фразы в `AgentsService-doc.md`, если они ещё есть.

Старая неверная формулировка:

```md
очищает agent.response_engine
```

Актуальная формулировка:

```md
После unpublish удаляется только PublishedWorkflow.
agent.response_engine не очищается автоматически.
Для runtime endpoints значения conversation_flow_id и version берутся только из PublishedWorkflow.
Если PublishedWorkflow отсутствует, production-запуск невозможен.
Frontend/editor endpoints продолжают работать по явно запрошенной версии flow.
```

---

## 8. Workflow JSON size limit

По отчёту:

- body limit 8MB реализован;
- nodes <= 1000 реализован;
- отдельной проверки serialized workflow JSON <= 8MB нет.

В ТЗ указано:

```md
Максимальный размер одного workflow после сериализации в JSON — 8 MB.
```

Нужно реализовать отдельную проверку в workflow validator/usecase:

```go
json.Marshal(conversationFlow)
len(bytes) <= MaxWorkflowJSONBytes
```

При превышении вернуть validation_error:

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

## 9. После доработки дай отчёт

После реализации напиши:

1. Какие файлы изменены.
2. Какие runtime routes добавлены.
3. Как runtime routes исключены из `RequireWorkspaceID`.
4. Какие repository methods добавлены.
5. Какие usecase methods добавлены.
6. Как работает `published-config`.
7. Что возвращается после unpublish при вызове `published-config`.
8. Добавлен ли `response_engine` в frontend/editor flow response.
9. Реализован ли отдельный лимит workflow JSON <= 8MB.
10. Результат `go test ./...`.