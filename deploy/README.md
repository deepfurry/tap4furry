# Runtime artifacts

Run `pnpm build:images` from the repository root with a local Docker engine. Builds
are local only and never push. CI builds the same four Dockerfiles.

| Image | Runtime |
| --- | --- |
| `tap4furry-server:local` | `tap4furry-api` (default), `tap4furry-admin`, or `tap4furry-worker` |
| `tap4furry-web:local` | Astro Node SSR, port 4321 |
| `tap4furry-admin:local` | Unprivileged Nginx SPA, port 8080 |
| `tap4furry-postgres:local` | PostgreSQL 18 with pg_trgm available |

All backend binaries read injected process environment. Images contain no private
config. `.dockerignore` excludes local secrets and generated build artifacts.
Server/Web run as non-root; Admin uses an unprivileged runtime. PostgreSQL follows
its upstream entrypoint and keeps PGDATA independent from image contents.
Web uses tini to forward container signals to Node and reap child processes.

The server image's command selects a runtime binary; API and Admin require separate
HTTP_ADDR values when sharing a network namespace. Migrations are explicit developer
or CI commands and never run at application startup.
Public API production configuration also requires an HTTPS `PUBLIC_ORIGIN`; it
always uses the Secure `__Host-tap4furry_session` cookie. Inject this through runtime
environment, never through image build arguments or checked-in local files.
MAIL-0 additionally requires explicit `MAIL_MODE=resend`, a private
`RESEND_API_KEY`, `MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>` and
`MAIL_REPLY_TO=support@tap4furry.com` in the Public API runtime only.
Mail credentials are never image build inputs; Admin and Worker do not read them.

The `local` tags contain the current P0-1A/B/C/D implementation. Future origins are
`https://tap4furry.com` and `https://admin.tap4furry.com`. Same-origin `/api/*`
forwarding belongs to a future deployment reverse proxy; the Admin static container
returns 503 on that prefix until
routing is configured. No VPS, Cloudflare, certificates, production credentials,
image publishing or deployment is provisioned here.

OAuth uses API-only Google/GitHub credential pairs injected at runtime. Callback
URLs derive from `PUBLIC_ORIGIN`; no client secret belongs in Web/Admin builds.
The future reverse proxy must suppress OAuth query strings, cookies and authorization
headers in access/error logging and preserve all Set-Cookie headers on callbacks.
