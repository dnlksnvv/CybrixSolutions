# Auth (Logto OSS)

Self-hosted identity: **OIDC/OAuth**, **Management API**, **Admin Console**. Integrate your site via standard OIDC or Logto SDKs — not only a hosted login page.

## Stack

- `logto` — Core API (`3001`) + Admin Console UI (`3002`)
- `postgres` — database (persistent volume)

## Quick start

1. Edit `.env`: set `PG_PASSWORD`, `DB_URL` (same password/user/db as `PG_*`), and for production set `ENDPOINT` / `ADMIN_ENDPOINT` to your public HTTPS URLs.
2. Start:

   ```bash
   docker compose up -d
   docker compose ps
   ```

3. Open **Admin Console**: `ADMIN_ENDPOINT` (default `http://localhost:3002/`).
4. Complete the **one-time** admin signup (OSS allows a single initial admin).

**Core / OIDC** listens on `ENDPOINT` (default `http://localhost:3001/`).

## Integration (high level)

- **SPA / mobile / your backend**: use Logto as **OIDC provider** (Authorization Code + PKCE, tokens, userinfo). See [Logto docs — Integrations](https://docs.logto.io/).
- **Management API**: create/update users, reset credentials, etc. (API-first; suitable for custom flows on your domain).
- **Traefik in front**: set `TRUST_PROXY_HEADER=1` (already in compose) and terminate TLS on Traefik; keep `ENDPOINT` / `ADMIN_ENDPOINT` as the **public** URLs users see. Route traffic to `logto:3001` / `logto:3002` or a single hostname with path rules, per Logto’s deployment guide.

Logto does **not** ship the same “forwardAuth + magic headers” model as Authentik. Typical pattern: app completes OIDC with Logto, then calls your API with **access tokens** (JWT); the API validates JWTs (JWKS from Logto). Optional: put **oauth2-proxy** or similar in front if you need cookie SSO at the edge.

## Notes

- Official demo compose warns that **embedded Postgres without a named volume** loses data; this file uses a **named volume** `logto_postgres` for persistence.
- For production hardening, follow [Deployment and configuration](https://docs.logto.io/logto-oss/deployment-and-configuration) (external DB, backups, TLS, `ENDPOINT` correctness).
- If `DB_URL` passwords contain `@`, `:`, `/`, etc., URL-encode them or use a simpler password for Postgres.
