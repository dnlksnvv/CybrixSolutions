# WorkspacesService — HTTP API (для фронтенда)

Базовый URL задаётся gateway (например `https://gateway.example.com`). Пути **без** префикса `/api`: сервис монтируется так, что корень — это уже `/workspaces` и `/invitations`.

---

## Аутентификация

| Требование | Описание |
|------------|----------|
| **Заголовок** | `Authorization: Bearer <access_token>` — JWT от Logto (или opaque-токен не подойдёт, если gateway ожидает JWT). |
| **Идентификация пользователя** | За reverse-proxy обычно выставляется `X-User-Id` (Logto `sub`). Сервис в Docker ожидает этот заголовок от Traefik после `forwardAuth`. **Браузер** шлёт только `Authorization`; gateway дописывает `X-User-Id` upstream’у. |
| **Публичные исключения** | `GET /healthz` — без авторизации. `OPTIONS` для путей `/workspaces*` и `/invitations*` — ответ 204 без тела (CORS preflight). |

Тип ответа ошибок: JSON `{"error": "<текст>"}`.

---

## Общие модели

### Workspace (воркспейс = организация в Logto)

```json
{
  "id": "string",
  "name": "string",
  "description": "string"
}
```

Поле `description` может отсутствовать в ответе, если пустое.

### Список воркспейсов

```json
{
  "items": [ /* workspaceResponse */ ]
}
```

### Участник воркспейса

```json
{
  "id": "string",
  "name": "string",
  "username": "string",
  "email": "string",
  "avatar": "string",
  "role": "string",
  "isCurrentUser": true
}
```

- `id` — Logto user id (тот же идентификатор, что в JWT `sub` / `X-User-Id`).
- `role` — первая роль из списка ролей в организации (удобное отображение).
- Пустые строки часто опускаются (`omitempty`).

### Роль каталога (шаблон организации в тенанте)

```json
{
  "id": "string",
  "name": "string",
  "description": "string",
  "type": "string"
}
```

`type` в Logto: например `"User"` или `"MachineToMachine"`.

### Приглашение

```json
{
  "id": "string",
  "organizationId": "string",
  "organizationName": "string",
  "email": "string",
  "status": "Pending | Accepted | Revoked | Expired",
  "role": "string",
  "roles": ["string"],
  "inviterId": "string",
  "expiresAt": "2026-05-01T12:00:00Z",
  "createdAt": "2026-05-01T12:00:00Z"
}
```

- `email` — адрес приглашённого (`invitee` в Logto).
- `role` — первая роль из `roles` для UI.
- Даты в **RFC3339** (UTC), опциональны, если нет значения.

### Создание воркспейса (тело запроса)

```json
{
  "name": "string",
  "description": "string"
}
```

### Обновление воркспейса (тело запроса)

Хотя бы одно поле должно быть задано.

```json
{
  "name": "string",
  "description": "string"
}
```

Оба поля опциональны; передаются только изменяемые (`null` не используется — либо ключ есть, либо нет).

### Создание приглашения (тело запроса)

```json
{
  "email": "user@example.com",
  "role": "Member"
}
```

`role` — имя роли из шаблона организации Logto (например `Owner`, `Admin`, `Member` — как настроено в тенанте).

### Создание роли в шаблоне тенанта (тело запроса)

```json
{
  "name": "string",
  "description": "string"
}
```

---

## Типовые коды ответа (ошибки бизнес-логики)

| HTTP | `error` (пример) | Когда |
|------|------------------|--------|
| 400 | текст валидации / `invalid …` | Невалидное имя, роль, email, приглашение не Pending и т.д. |
| 400 | `remove yourself via DELETE /workspaces/{id}/members/me` | Пытаются выгнать себя через `DELETE .../members/{userId}` |
| 401 | `missing user identity` / `missing X-User-Id header` | Нет контекста пользователя |
| 403 | `email does not match invitation` | Accept приглашения под другим email |
| 403 | `forbidden` | Нет прав (например Member вызывает kick) |
| 404 | `workspace not found` | Нет доступа / нет воркспейса |
| 404 | `invitation not found` | Нет приглашения или нет прав смотреть |
| 404 | `member not found in workspace` | Удаляемый user id не в организации |
| 409 | текст конфликта | Дубликат (Logto / роль) |
| 502 | `identity provider error` | Ошибка Logto Management API |
| 503 | `set INVITATION_LINK_BASE_URL…` | На бэкенде не настроена база ссылок для писем |

У gateway до сервиса возможны отдельно **401/403** от `jwt-auth` (например `missing scopes: …`) — тело может быть `text/plain`.

---

## Эндпоинты

### Здоровье

| Метод | Путь | Тело | Ответ |
|-------|------|------|--------|
| `GET` | `/healthz` | — | `200` `{"status":"ok"}` |

---

### Воркспейсы

#### Создать воркспейс

| | |
|---|---|
| **Метод / путь** | `POST /workspaces` |
| **Тело** | `createWorkspaceRequest` |
| **Успех** | `201` + `Workspace` |
| **Пояснение** | Владелец — текущий пользователь; в Logto создаётся организация, пользователь добавляется с ролью owner (см. env `WORKSPACE_OWNER_ROLE`, по умолчанию `Owner`). |

#### Список моих воркспейсов

| | |
|---|---|
| **Метод / путь** | `GET /workspaces` |
| **Успех** | `200` + `{ "items": [ Workspace ] }` |
| **Пояснение** | Только организации, где пользователь состоит членом. |

#### Получить воркспейс

| | |
|---|---|
| **Метод / путь** | `GET /workspaces/{id}` |
| **Параметры** | `id` — id организации / воркспейса |
| **Успех** | `200` + `Workspace` |
| **Пояснение** | Нужно быть участником этого воркспейса. |

#### Обновить воркспейс

| | |
|---|---|
| **Метод / путь** | `PATCH /workspaces/{id}` |
| **Тело** | `updateWorkspaceRequest` (частичное) |
| **Успех** | `200` + `Workspace` |
| **Пояснение** | Нужно быть участником. |

---

### Участники

#### Список участников

| | |
|---|---|
| **Метод / путь** | `GET /workspaces/{id}/members` |
| **Успех** | `200` + `{ "items": [ memberResponse ] }` |
| **Пояснение** | Только участники воркспейса. |

#### Покинуть воркспейс (самостоятельно)

| | |
|---|---|
| **Метод / путь** | `DELETE /workspaces/{id}/members/me` |
| **Тело** | — |
| **Успех** | `204` без тела |
| **Пояснение** | Снимает **текущего** пользователя с членства в организации. |

#### Удалить другого участника (kick)

| | |
|---|---|
| **Метод / путь** | `DELETE /workspaces/{id}/members/{userId}` |
| **Параметры** | `userId` — тот же `id`, что в списке участников (Logto user id) |
| **Тело** | — |
| **Успех** | `204` без тела |
| **Пояснение** | **Owner** или **Admin** (роли в организации). **Admin не может** удалить пользователя с ролью **Owner** — только Owner может. Удалить себя этим методом нельзя — использовать `.../members/me`. |

---

### Роли (каталог шаблона организации в тенанте)

#### Список ролей

| | |
|---|---|
| **Метод / путь** | `GET /workspaces/{id}/roles` |
| **Успех** | `200` + `{ "items": [ roleResponse ] }` |
| **Пояснение** | Каталог доступен участнику воркспейса (проверка членства). |

#### Создать роль в шаблоне

| | |
|---|---|
| **Метод / путь** | `POST /workspaces/{id}/roles` |
| **Тело** | `createWorkspaceRoleRequest` |
| **Успех** | `201` + `roleResponse` |
| **Пояснение** | Нужно быть участником; роль создаётся на уровне **тенанта** Logto (organization-roles), не только одной организации. |

---

### Приглашения — сторона админа / участника воркспейса

Все маршруты ниже требуют, чтобы вызывающий был **участником** `{id}`.

#### Создать приглашение и отправить письмо

| | |
|---|---|
| **Метод / путь** | `POST /workspaces/{id}/invitations` |
| **Тело** | `createInvitationRequest` |
| **Успех** | `201` + `invitationResponse` |
| **Пояснение** | На бэкенде должна быть настроена база ссылок для писем (`INVITATION_LINK_BASE_URL`), иначе `503`. |

#### Список приглашений воркспейса

| | |
|---|---|
| **Метод / путь** | `GET /workspaces/{id}/invitations` |
| **Успех** | `200` + `{ "items": [ invitationResponse ] }` |

#### Отозвать приглашение

| | |
|---|---|
| **Метод / путь** | `DELETE /workspaces/{id}/invitations/{invitationId}` |
| **Успех** | `204` без тела |

#### Повторно отправить письмо по приглашению

| | |
|---|---|
| **Метод / путь** | `POST /workspaces/{id}/invitations/{invitationId}/resend` |
| **Успех** | `204` без тела |
| **Пояснение** | Обычно только для статуса `Pending`; иначе возможна ошибка валидации. |

---

### Приглашения — сторона приглашённого пользователя

#### Получить приглашение по id (лендинг)

| | |
|---|---|
| **Метод / путь** | `GET /invitations/{id}` |
| **Успех** | `200` + `invitationResponse` |
| **Пояснение** | Доступ, если **email** текущего пользователя совпадает с приглашением **или** пользователь уже **участник** этого воркспейса. Иначе часто `404`. |

#### Принять приглашение

| | |
|---|---|
| **Метод / путь** | `POST /invitations/{id}/accept` |
| **Тело** | — |
| **Успех** | `200` + `invitationResponse` (статус станет `Accepted`) |
| **Пояснение** | Приглашение должно быть `Pending`. **Primary email / username / identities** в Logto должны согласовываться с `email` приглашения (проверка на бэкенде). |

---

## Краткая сводка путей

```
GET    /healthz
POST   /workspaces
GET    /workspaces
GET    /workspaces/{id}
PATCH  /workspaces/{id}
GET    /workspaces/{id}/members
DELETE /workspaces/{id}/members/me
DELETE /workspaces/{id}/members/{userId}
GET    /workspaces/{id}/roles
POST   /workspaces/{id}/roles
POST   /workspaces/{id}/invitations
GET    /workspaces/{id}/invitations
DELETE /workspaces/{id}/invitations/{invitationId}
POST   /workspaces/{id}/invitations/{invitationId}/resend
GET    /invitations/{id}
POST   /invitations/{id}/accept
```

---

## TypeScript-подсказки (опционально)

```typescript
type Workspace = {
  id: string;
  name: string;
  description?: string;
};

type WorkspaceMember = {
  id: string;
  name?: string;
  username?: string;
  email?: string;
  avatar?: string;
  role?: string;
  isCurrentUser: boolean;
};

type OrgRole = {
  id: string;
  name: string;
  description?: string;
  type?: string;
};

type InvitationStatus = "Pending" | "Accepted" | "Revoked" | "Expired";

type Invitation = {
  id: string;
  organizationId: string;
  organizationName?: string;
  email: string;
  status: InvitationStatus;
  role?: string;
  roles?: string[];
  inviterId?: string;
  expiresAt?: string;
  createdAt?: string;
};

type ApiError = { error: string };
```

Файл сгенерирован по текущему коду сервиса; при смене gateway URL или middleware ошибки до API могут отличаться.
