# Tap4Furry — Deployment Architecture

## Topology

```text
                         Internet
                            │
                       Cloudflare
              DNS / CDN / WAF / Turnstile
                  Access for Admin
                            │
                            ▼
                    Host Nginx :443
                            │
          ┌─────────────────┼─────────────────┐
          │                 │                 │
       Astro Web        Public API         Admin
          │                 │          ┌─────┴─────┐
          │                 │          │           │
          │                 │     Admin Web    Admin API
          │                 │
──────────────────────── Docker Compose ────────────────────────
          │                 │               │
          └────────────── Shared backend ───┘
                            │
                      ┌─────┴─────┐
                      ▼           ▼
                 PostgreSQL      Redis
                      │
                    River
```

Worker accesses PostgreSQL / Redis / R2 / Email / Internet.

## Region

Preferred:

```text
Hong Kong VPS
```

Fallback:

```text
Tokyo VPS
```

Choose based on real mainland-China and international routing quality.

## Host Nginx

Nginx runs on the VPS host, not in Compose.

Responsibilities:

- :443 origin gateway
- hostname routing
- Cloudflare Origin TLS
- maintenance/fallback page
- reverse proxy
- same-origin `/api/*`

Do not add a second page-cache layer in Nginx.

## Containers

P0:

```text
web
admin-web
api
admin-api
worker
postgres
redis
river-ui (optional)
```

Backend uses one server image with different commands for API/Admin/Worker.

## Ports

Only Host Nginx is public.

Application ports bind to loopback where host access is needed.

PostgreSQL and Redis are internal-only.

## Routing

```text
tap4furry.com/
→ Astro

tap4furry.com/api/*
→ Public API

admin.tap4furry.com/
→ Admin Web

admin.tap4furry.com/api/*
→ Admin API

river.tap4furry.com
→ River UI
```

Admin and River UI use Cloudflare Access.

## Public SSR Boundary

Astro HTML must remain anonymous/cacheable.

Nginx may strip public session cookies before proxying non-API requests to Astro.

Private user state remains API/React-Island responsibility.

## Cloudflare

Cloudflare is CDN/WAF/bot/Turnstile/Admin-Access infrastructure.

It is not application runtime.

## Origin Protection

Production origin should:

- allow 443 from Cloudflare only;
- use Full (strict);
- use Authenticated Origin Pulls where practical;
- reject unknown hostnames;
- restrict SSH to management access.

No Cloudflare Tunnel in P0.

## Cache

Hashed assets: long immutable cache.

Public HTML: short edge TTL initially.

Public API: no CDN cache by default.

Admin: `no-store`.

## PostgreSQL Runtime Image

Maintain:

```text
tap4furry-postgres:<runtime-version>
```

Image and PGDATA have separate lifecycles.

Future pgvector is added by producing a new runtime image while reusing compatible PGDATA.

## Persistence

Critical local persistence:

```text
PostgreSQL PGDATA
```

Redis is ephemeral.

Media lives in R2/S3.

## Migration

No AutoMigrate on application startup.

Deployment explicitly runs:

```text
goose migrations
River migrations
```

Use Expand / Migrate / Contract.

## Deploy Order

```text
1. Pull new images
2. Run app migrations
3. Run River migrations
4. Update Worker
5. Update Public API
6. Update Admin API
7. Update Public Web
8. Update Admin Web
9. Health checks
10. Record release
```

## Images

Tag application images by Git SHA.

Never depend on `latest`.

## Rollback

Application:

```text
deploy previous image SHA
```

Database:

```text
forward-fix by default
```

Production only pulls images; it does not build source.
