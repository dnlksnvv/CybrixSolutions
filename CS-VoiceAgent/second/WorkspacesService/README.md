# WorkspacesService

Owns the **Workspace** bounded context: creating workspaces, listing them, fetching details. A workspace is **a Logto Organization**, and Logto is the **single source of truth**: this service does not persist a local copy of names, members, or roles. All reads go straight to the Logto Management API on every request, so the response always reflects the current state in Logto.

## Architecture

Clean / Hexagonal:

- `internal/domain/` — `Workspace` projection, `OrganizationRole`, request validation, domain errors.
- `internal/application/ports/` — `IdentityProvider` interface (the only outbound port for now).
- `internal/application/usecase/` — `CreateWorkspace`, `ListMyWorkspaces`, `GetWorkspace`, `UpdateWorkspace`, `ListWorkspaceMembers`, `ListWorkspaceRoles`, `LeaveWorkspace`, `CreateWorkspaceRole`, `InviteByEmail`, `ListInvitations`, `RevokeInvitation`, `ResendInvitation`, `GetInvitation`, `AcceptInvitation`.
- `internal/adapters/identity/logto/` — Logto Management API client (M2M token cache, organizations endpoints).
- `internal/adapters/transport/http/` — `chi` router, handlers, middleware.
- `cmd/workspaces/` — wiring & lifecycle (slog, graceful shutdown).

> No database adapter. When product-specific data (billing, agents, configs, FK targets) appears, we will add a thin Postgres projection via a new port — it will store **only product data + a link to `organization_id`**, never duplicate identity fields.

## Trust model

The service expects to live **behind the Traefik gateway** in the internal `cs_gateway` docker network. Traefik runs `forwardAuth` against `jwt-auth`, validates the user JWT, and forwards the request with trusted headers:

- `X-User-Id` — Logto `sub` of the caller.
- `X-User-Scopes` — space-delimited scopes.
- `X-Organization-Id` — optional (only when the token has organization context).

WorkspacesService MUST NOT be exposed publicly bypassing Traefik. Without `X-User-Id` it returns `401`.

## Logto prerequisites

In Logto Console:

1. **Machine-to-Machine app** assigned the role **`Logto Management API access`** (built-in, has `all` permission for the Management API resource `https://<tenant>.logto.app/api`).
2. **Organization template** with the role used as workspace owner (default: `Owner`). Adjust `WORKSPACE_OWNER_ROLE` if you use a different name. Any role you want to assign via invitations must also exist in the template.
3. **Email connector** configured (SMTP, SendGrid, AWS SES, etc.). Without it Logto cannot deliver invitation emails. Test it from Console → Connectors.
4. **Email template `OrganizationInvitation`** populated with subject + body. The body MUST include the `{{link}}` template variable — Logto substitutes it with the SPA URL we send in `messagePayload.link`.

## Configuration

Copy `.env.example` to `.env` and fill in real values:

```bash
cp .env.example .env
```

Required variables:

- `LOGTO_ISSUER` (e.g. `https://<tenant>.logto.app/oidc`)
- `LOGTO_MANAGEMENT_BASE_URL` (e.g. `https://<tenant>.logto.app`)
- `LOGTO_MANAGEMENT_RESOURCE` (e.g. `https://<tenant>.logto.app/api`)

Optional:

- `LOGTO_TENANT_ID` — same string as the `<tenant>` subdomain on Logto Cloud. If omitted, it is **derived** from `LOGTO_MANAGEMENT_BASE_URL` when the host matches `*.logto.app`. Required for self-hosted / custom domains where derivation does not apply.

The Management API **`POST /api/organizations`** requires `tenantId` in the JSON body; the service fills it from this value.
- `LOGTO_M2M_CLIENT_ID`
- `LOGTO_M2M_CLIENT_SECRET`
- `WORKSPACE_OWNER_ROLE` (default `Owner`)
- `INVITATION_LINK_BASE_URL` — SPA URL prefix for invitation landing pages. The service appends the invitation id as the last path segment when building `messagePayload.link`. E.g. `https://app.cybrix.local/invitations` → email link becomes `https://app.cybrix.local/invitations/<id>`.

## API

All endpoints expect `X-User-Id` set by Traefik. The user identity is taken from there; the body never carries the owner.

### POST /workspaces

Body:

```json
{ "name": "Acme", "description": "Acme corp workspace" }
```

Response `201`:

```json
{
  "id": "logto-org-id",
  "name": "Acme",
  "description": "Acme corp workspace"
}
```

Effect (orchestrated against Logto Management API):

1. `POST /api/organizations` → creates organization.
2. `POST /api/organizations/{id}/users` → adds the caller as a member.
3. `POST /api/organizations/{id}/users/{userId}/roles` → assigns the configured owner role.

> No saga/outbox yet. If step 2 or 3 fails after step 1 succeeds, the freshly created organization may be left without a member/owner. Acceptable for MVP; revisit before production.

### GET /workspaces

Returns workspaces the caller is a member of (live from Logto):

```json
{
  "items": [
    { "id": "logto-org-id", "name": "Acme", "description": "" }
  ]
}
```

Implemented via `GET /api/users/{sub}/organizations`.

### GET /workspaces/{id}

Returns a workspace by Logto organization id, **only if** the caller is a member. Membership is verified against Logto on every call. Non-member or non-existent ids both return `404` (we do not leak existence).

```json
{ "id": "logto-org-id", "name": "Acme", "description": "" }
```

### PATCH /workspaces/{id}

Update mutable fields. Caller must be a member.

```json
{ "name": "New name", "description": "optional" }
```

Response `200`: `{ "id", "name", "description" }`.

Either `name` or `description` may be omitted; both nil → `400`. Backed by Logto `PATCH /api/organizations/{id}`.

### GET /workspaces/{id}/members

List members of a workspace with the caller’s primary role per user.

```json
{
  "items": [
    {
      "id": "user_abc",
      "email": "alex@example.com",
      "name": "Alex",
      "role": "Admin",
      "isCurrentUser": true
    }
  ]
}
```

Backed by `GET /api/organizations/{id}/users` (Logto returns embedded `organizationRoles`). `role` is the first role; `isCurrentUser` is `member.id == X-User-Id`.

### DELETE /workspaces/{id}/members/me

Caller leaves the workspace. Response `204`. Backed by `DELETE /api/organizations/{id}/users/{userId}`.

### GET /workspaces/{id}/roles

Roles available in the tenant organization template (same set across all orgs). Caller must be a member.

```json
{
  "items": [
    { "id": "role_001", "name": "Admin", "description": "...", "type": "User" }
  ]
}
```

Backed by `GET /api/organization-roles`. `type` passes through Logto’s value (`User` or `MachineToMachine`).

### POST /workspaces/{id}/roles

Create a new role in the tenant organization template (visible to all workspaces). Caller must be a member.

```json
{ "name": "Billing Manager", "description": "Can manage billing only" }
```

Response `201`: same shape as `roles` items. Backed by `POST /api/organization-roles`.

`409` when a role with the same name already exists.

## Invitations

Standard email-based invitation flow backed by Logto's organization invitations API. Logto's email connector delivers the message; our SPA hosts the landing page that finalises acceptance. Logto does not auto-link a fresh sign-up to a pending invitation — the SPA reads the invitation id from the URL and calls `POST /invitations/{id}/accept` once the user is signed in.

### High-level flow

1. Admin → `POST /workspaces/{id}/invitations { email, role }`. We create a Pending invitation in Logto, then ask Logto to send the email with link `INVITATION_LINK_BASE_URL/<invitationId>`.
2. Invitee opens the link → SPA `/invitations/:id`:
   - If not authenticated → drive Logto Sign-In Experience (sign-in or sign-up); after redirect the SPA returns to the same `/invitations/:id`.
   - SPA fetches `GET /invitations/{id}` to display the workspace + assigned role.
   - On confirm → SPA calls `POST /invitations/{id}/accept`. The backend verifies the caller's primary email matches the invitee, then transitions the invitation to `Accepted` (Logto adds the user to the organization with the saved roles).
3. Admin can `DELETE /workspaces/{id}/invitations/{invitationId}` (revoke) or `POST .../resend` while the invitation is `Pending`.

### POST /workspaces/{id}/invitations

Caller must be a workspace member. Body:

```json
{ "email": "new@example.com", "role": "Member" }
```

Response `201`:

```json
{
  "id": "inv_abc",
  "organizationId": "org_xyz",
  "email": "new@example.com",
  "status": "Pending",
  "role": "Member",
  "roles": ["Member"],
  "inviterId": "user_caller",
  "expiresAt": "2026-05-08T10:00:00Z",
  "createdAt": "2026-05-01T10:00:00Z"
}
```

Backed by Logto: `GET /api/organization-roles` (resolve role name → id), `POST /api/organization-invitations`, `POST /api/organization-invitations/{id}/message` (sends email with `messagePayload.link`).

Errors: `400` when role is missing from the template (`organization_role.role_names_not_found` from Logto); `502` when the email connector / template is not configured in Logto.

### GET /workspaces/{id}/invitations

Caller must be a workspace member. Returns all invitations for the workspace (Pending, Accepted, Revoked, Expired):

```json
{ "items": [ { "id": "inv_abc", "email": "...", "status": "Pending", "role": "Member", "expiresAt": "..." } ] }
```

Backed by `GET /api/organization-invitations?organizationId=...`.

### DELETE /workspaces/{id}/invitations/{invitationId}

Cancels a pending invitation. Response `204`. Backed by `PUT /api/organization-invitations/{id}/status { "status": "Revoked" }`.

### POST /workspaces/{id}/invitations/{invitationId}/resend

Re-sends the invitation email (only when status is `Pending`). Response `204`. Backed by `POST /api/organization-invitations/{id}/message`.

### GET /invitations/{id}

Invitee-facing lookup: returns invitation details so the SPA landing page can render workspace name + role. Caller must be either:

- the invitee (primary email matches `invitee` exactly), or
- already a member of the workspace.

Anyone else gets `404` (we do not leak the invitation's existence).

```json
{
  "id": "inv_abc",
  "organizationId": "org_xyz",
  "organizationName": "Acme",
  "email": "new@example.com",
  "status": "Pending",
  "role": "Member",
  "roles": ["Member"],
  "expiresAt": "2026-05-08T10:00:00Z"
}
```

### POST /invitations/{id}/accept

Finalises acceptance. The backend:

1. Loads the invitation; rejects with `400` if status is not `Pending`.
2. Loads the caller's user record (`GET /api/users/{sub}`) and compares `primaryEmail` to `invitee` (case-insensitive). On mismatch returns `403 email does not match invitation`.
3. Calls `PUT /api/organization-invitations/{id}/status { "status": "Accepted", "acceptedUserId": "<sub>" }`. Logto adds the user to the organization with the invitation's roles.

Response `200`: the updated invitation (status `Accepted`).

Notes:

- The endpoint requires a valid user JWT (Traefik forwards `X-User-Id`); the invitee must therefore already be signed in via Logto when the SPA calls accept.
- A Logto user account is created during the standard sign-up flow when the invitee did not exist. Sign-up uses Logto's normal Sign-In Experience and does not depend on the invitation; the link between user and workspace is established only when accept succeeds.

| Source                                              | HTTP status |
|-----------------------------------------------------|-------------|
| Invalid name / owner / role / email                 | 400         |
| Missing organization roles in Logto template        | 400 (body includes Logto message) |
| Invitation no longer pending (revoked/expired/used) | 400         |
| Missing `X-User-Id`                                 | 401         |
| Caller email does not match invitation              | 403         |
| Workspace / user / invitation not found             | 404         |
| Caller is not a member (looks like 404 to client)   | 404         |
| Already a member / role name exists                 | 409         |
| Logto Management API non-2xx (other)                | 502         |
| Anything else                                       | 500         |

### Troubleshooting `502 identity provider error`

1. **M2M token**: ensure `LOGTO_M2M_CLIENT_ID` / `LOGTO_M2M_CLIENT_SECRET` are correct and the app has the **Logto Management API access** role.
2. **`tenantId` on create org**: `POST /api/organizations` requires `tenantId`; this service sends it from `LOGTO_TENANT_ID` or derives it from `*.logto.app`.
3. **Organization roles**: assigning `WORKSPACE_OWNER_ROLE` (default `Owner`) fails with Logto code `organization.role_names_not_found` if that role does not exist under **Authorization → Organization template → Organization roles**. Create it first (name must match **exactly**, case-sensitive), then retry `POST /workspaces`.

## Run

```bash
docker compose up -d --build
```

The service joins the external `cs_gateway` network from the Gateway compose. Traefik picks up labels via the docker provider and exposes the route under `/workspaces`.

## Local check

Через Traefik (рекомендуется, проверяется JWT):

```bash
curl -i \
  -H "Authorization: Bearer <user_jwt>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme"}' \
  http://127.0.0.1/workspaces

curl -i -H "Authorization: Bearer <user_jwt>" http://127.0.0.1/workspaces
curl -i -H "Authorization: Bearer <user_jwt>" http://127.0.0.1/workspaces/<organization_id>
```
