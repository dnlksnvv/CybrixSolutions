# Приглашения в workspace (организацию Logto)

Документ описывает эндпоинты, полный flow и роли фронтенда. Актуально для WorkspacesService за Traefik + `jwt-auth`.

---

## Страница на Logto или в приложении?

**В текущей интеграции — в вашем SPA**, не отдельный «готовый экран Logto» для конечного пользователя.

По [документации Logto — приглашение участников организации](https://docs.logto.io/end-user-flows/organization-experience/invite-organization-members): в письме используется плейсхолдер **`{{link}}`**, а **URL задаётся вами** через `messagePayload.link` в Management API (например `https://your-app.com/.../{invitation-id}`). Нужна **своя лендинг-страница**, где пользователь после входа завершает приглашение.

Logto предоставляет **Sign-In Experience** (логин/регистрация), но маршрут приложения (`/invitations/:id` и вызовы gateway) — это **ваш фронт + ваш бэкенд**.

Для новых версий Logto в доке упоминается **magic link (one-time token)** — ссылка всё равно ведёт на **ваш домен** (с токеном в query), а не заменяет полностью страницу в продукте. Сейчас WorkspacesService собирает ссылку как **`INVITATION_LINK_BASE_URL` + `/` + `invitationId`** (id в path).

---

## Куда «отправляется» ссылка из письма

**С фронтенда ссылку ни в какой API передавать не нужно.**

1. Админ вызывает **`POST /workspaces/{workspaceId}/invitations`** с `email` и `role`.
2. **WorkspacesService** через Logto Management API создаёт приглашение и вызывает отправку письма с  
   `messagePayload: { "link": "https://ваш-спа/invitations/<invitationId>" }`.
3. Logto подставляет этот URL в шаблон вместо **`{{link}}`** и отправляет email.

URL формирует **только бэкенд** из переменной **`INVITATION_LINK_BASE_URL`**. Фронт реализует страницу по этому URL и вызывает **`GET` / `POST /invitations/...`** на gateway.

---

## Полный flow инвайта

| Шаг | Кто | Действие |
|-----|-----|----------|
| 1 | Админ (уже член workspace) | В UI: пригласить по email и роли → **`POST /workspaces/{id}/invitations`**. |
| 2 | WorkspacesService | Создаёт инвайт в Logto, резолвит имя роли в id, отправляет письмо с `link` = `INVITATION_LINK_BASE_URL` + `/` + `invitationId`. |
| 3 | Приглашённый | Открывает письмо → переход на **ваш SPA** по `/invitations/{invitationId}`. |
| 4 | SPA | Нет сессии → **Logto sign-in / sign-up**; **redirect_uri** должен вернуть на тот же `/invitations/{id}`. |
| 5 | SPA | **`GET /invitations/{id}`** с Bearer — показать организацию, email, роль, статус. |
| 6 | SPA | **`POST /invitations/{id}/accept`** — бэкенд проверяет, что **primary email** пользователя в Logto совпадает с **invitee** в инвайте; затем принимает инвайт в Logto → пользователь в org с ролями из приглашения. |
| 7 | SPA | Редирект в приложение; сброс кэша списка воркспейсов. |

**Важно:** при несовпадении email на шаге accept бэкенд отвечает **403**. Для `GET` чужой аккаунт часто получает **404** (намеренно, без утечки деталей).

---

## Базовый URL API

Все пути ниже — относительно **`https://&lt;gateway&gt;`** (тот же хост, что для `/workspaces`).

Требуется заголовок **`Authorization: Bearer &lt;JWT&gt;`** (после Traefik бэкенд получает `X-User-Id`). Браузерный preflight **OPTIONS** обрабатывается на gateway отдельно.

---

## Эндпоинты: админ / настройки

Вызываются из приложения пользователем, который **уже член** данного `workspaceId` (id организации Logto).

| Метод | Путь | Назначение |
|--------|------|------------|
| `POST` | `/workspaces/{workspaceId}/invitations` | Создать приглашение и отправить письмо. Тело: `{"email":"...","role":"Member"}`. **201** — объект приглашения. Без `INVITATION_LINK_BASE_URL` на бэке — **503**. |
| `GET` | `/workspaces/{workspaceId}/invitations` | Список приглашений (все статусы). |
| `DELETE` | `/workspaces/{workspaceId}/invitations/{invitationId}` | Отменить приглашение. **204**. |
| `POST` | `/workspaces/{workspaceId}/invitations/{invitationId}/resend` | Повторная отправка письма (только **Pending**). **204**. |

---

## Эндпоинты: приглашённый (лендинг из письма)

| Метод | Путь | Назначение |
|--------|------|------------|
| `GET` | `/invitations/{invitationId}` | Детали для экрана «вас пригласили». Доступ: email пользователя = **invitee** ИЛИ пользователь уже **член** этой организации. Иначе часто **404**. |
| `POST` | `/invitations/{invitationId}/accept` | Принять приглашение. **200** — обновлённый объект; **403** — email не совпадает с инвайтом; **400** — инвайт не в **Pending**. |

`invitationId` — тот же id, что в URL письма (суффикс после `INVITATION_LINK_BASE_URL`).

---

## Формат ответа (приглашение)

Ориентировочные поля JSON (см. хендлеры сервиса):

- `id`, `organizationId`, `organizationName`
- `email` (invitee)
- `status` — например `Pending`, `Accepted`, `Revoked`, `Expired`
- `role`, `roles`
- `inviterId`
- `expiresAt`, `createdAt` (ISO 8601, если есть)

Ошибки в JSON: `{ "error": "..." }` (коды **400 / 401 / 403 / 404 / 503 / 502** в зависимости от случая).

---

## Что реализовать на фронтенде

| Зона | Функционал |
|------|------------|
| Настройки / пользователи | `POST` создать инвайт, `GET` список, `DELETE` отмена, `POST` resend. |
| Страница из письма | Роут, совпадающий с `INVITATION_LINK_BASE_URL`; Logto login с возвратом на тот же URL; `GET` детали; кнопка `POST` accept; обработка 401/403/404/400. |

Не вызывать с клиента **Logto Management API** и не собирать `messagePayload.link` — это ответственность WorkspacesService.

---

## Краткий итог

Ссылка в письме ведёт на **ваш фронт**. Фронт **не** регистрирует эту ссылку в API: её в Logto передаёт **бэкенд** при создании/ресенде инвайта. Страница приглашения **не** «целиком на Logto»: Logto используется для **аутентификации**, принятие — через **`GET` / `POST /invitations/...`** на gateway.

---

## Ссылки Logto

- [Invite organization members](https://docs.logto.io/end-user-flows/organization-experience/invite-organization-members)
- [Built-in email service — шаблоны и `messagePayload.link`](https://docs.logto.io/connectors/email-connectors/built-in-email-service#unified-email-templates)
- [OpenAPI: resend invitation message](https://openapi.logto.io/operation/operation-createorganizationinvitationmessage) — тело запроса с полем **`link` на верхнем уровне**, не `messagePayload.link`.

---

## Устранение: в письме видно сырой `{{link}}`

Причины:

1. **Неверное тело `POST .../organization-invitations/{id}/message`**: Logto ожидает JSON вида `{"link":"https://..."}`, а не `{"messagePayload":{"link":"..."}}`. Иначе плейсхолдер в шаблоне не заполняется.
2. **Письмо ушло при создании инвайта без `link`**: при `POST /api/organization-invitations` без корректного `messagePayload` или с пустым объектом шаблон может отработать без подстановки. Сервис задаёт **`messagePayload: false`** при создании (без немедленной отправки), затем вызывает **`POST .../message`** с полным URL.

После исправления на бэкенде пересоберите образ и отправьте инвайт заново (или **resend**).
