# Gateway (Traefik + Logto JWT validation)

This gateway protects backend routes at Traefik entrypoint using `forwardAuth`.

Flow:

1. Frontend sends `Authorization: Bearer <access_token>` to Traefik.
2. Traefik calls Go `jwt-auth` (`/auth/verify`) before forwarding.
3. `jwt-auth` validates JWT from Logto JWKS (`iss`, `aud`, signature, expiration, scopes).
4. If valid, Traefik forwards request to backend with trusted headers:
   - `X-User-Id` (JWT `sub`)
   - `X-User-Scopes` (JWT `scope` — org-scoped tokens carry org-related scopes from Logto)
   - `X-Organization-Id` when the JWT is organization-scoped (`organization_id` / `org_id` claim)

Organization tokens use `aud` like `urn:logto:organization:<org_id>`. Set `LOGTO_ORGANIZATION_AUDIENCE_PREFIX` (e.g. `urn:logto:organization:`) so `jwt-auth` accepts them alongside normal API `aud` values in `LOGTO_AUDIENCE`. Membership checks stay in your services (or cache), not in Traefik.

## 1) Configure environment

Docker Compose loads variables for `jwt-auth` from a file named **`.env`** in this folder (not `.env.example`).

```bash
cp .env.example .env
# edit .env if needed
```

Required values:

- `LOGTO_ISSUER` (example `https://tenant.logto.app/oidc`)
- `LOGTO_AUDIENCE` (your API resource indicator from Logto)

Optional:

- `REQUIRED_SCOPES` (space-delimited scopes required by middleware)
- `LOGTO_JWKS_URI` (auto-derives as `${LOGTO_ISSUER}/jwks`)

## 2) Start gateway

```bash
docker compose up -d --build
```

- Traefik entrypoint: `http://localhost:${TRAEFIK_HTTP_PORT}`
- Traefik dashboard: `http://localhost:${TRAEFIK_DASHBOARD_PORT}`

## 3) Attach middleware to protected services

In each backend service compose labels:

```yaml
labels:
  - traefik.enable=true
  - traefik.http.routers.api.rule=PathPrefix(`/api`)
  - traefik.http.routers.api.entrypoints=web
  - traefik.http.routers.api.middlewares=logto-jwt-auth@file
  - traefik.http.services.api.loadbalancer.server.port=3000
```

Also ensure the service joins the same docker network (`cs_gateway` by default).

## 4) Logto Cloud (no Traefik proxy here)

The SPA and backends talk to **Logto Cloud** directly (`https://<tenant>.logto.app/...`). This stack does **not** run a Logto reverse proxy in Docker.

If you still have `traefik/dynamic/99-logto-proxy.yml` on disk from an older setup, delete it so Traefik does not load stale routes.

Advanced: to put Logto behind Traefik yourself, add a static file under `traefik/dynamic/` and configure a **custom domain** in Logto; see Logto docs.

## 5) Public access via Cloudflare Tunnel (optional)

Expose Traefik on the internet **without** opening ports on your router: run Cloudflare’s `cloudflared` next to Traefik.

1. In [Cloudflare Zero Trust](https://one.dash.cloudflare.com/) → **Networks** → **Tunnels** → **Create a tunnel**.
2. Choose **Docker**, copy the **token**.
3. Put the token in Gateway `.env` (file is gitignored — **do not** paste tokens into `docker-compose.yml` or commits):

   ```text
   CLOUDFLARE_TUNNEL_TOKEN=eyJ...
   ```

   If a token was ever shared in chat or committed, **rotate it** in Zero Trust (revoke old / create new tunnel token).

4. In the tunnel configuration, add a **Public Hostname** (e.g. `gateway.example.com`):

   - **Service type**: HTTP
   - **URL**: `http://traefik:80`  
     (must be exactly this from inside Docker: hostname `traefik`, port `80`, same compose network as Traefik.)

5. Start with the profile:

   ```bash
   docker compose --profile cloudflare up -d
   ```

6. Update **Logto** (and your SPA) redirect / CORS / allowed origins to use the **public** `https://gateway.example.com` URLs where applicable.

`jwt-auth` is **not** exposed by default on a public hostname unless you route that path in the tunnel; usually you only publish Traefik (`80`). Keep `JWT_AUTH_HOST_PORT` for local debugging.

## 6) M2M secrets (backend, not Traefik)

`.env.example` includes placeholders:

- `LOGTO_M2M_CLIENT_ID` / `LOGTO_M2M_CLIENT_SECRET` — for **your** backend to call **Logto Management API** (`LOGTO_MANAGEMENT_API_RESOURCE`). Traefik does not read these; they are only stored next to other gateway env for convenience.

## Notes

- This implements "Traefik validates token" by enforcing auth through `forwardAuth`.
- Backend services should trust identity headers only from internal gateway network.
- If you need per-route scopes, run multiple middlewares with different `REQUIRED_SCOPES` (or extend `jwt-auth`).

## Troubleshooting

### `LOGTO_ISSUER is required` / compose warns variables are not set

You ran `docker compose` without a real **`.env`** file. Copy from `.env.example` first (see step 1).

### Logto JSON: `invalid_request` / `GET on /`

You opened or requested the **root URL** of the tenant (e.g. `https://w93wb3.logto.app/`) with **GET**. The OIDC app does not serve `/` that way. Use discovery instead:

`https://w93wb3.logto.app/oidc/.well-known/openid-configuration`

### Quick local check of JWT validation

With gateway up and `.env` filled:

```bash
curl -sS -i -H "Authorization: Bearer <access_token_jwt>" \
  "http://127.0.0.1:${JWT_AUTH_HOST_PORT:-8000}/auth/verify"
```
