# Architecture

Public/Admin are distinct transport and security boundaries, sharing one Go module.

```text
apps/web / apps/admin → generated API clients → Public/Admin transport
                                                ↓
                                       auth / identity
                                                ↓
                                           sqlc / pgx
                                                ↓
                                           PostgreSQL
worker → jobs adapter → shared Application/Domain (when implemented)
```

P0-1A/B/C/D add local/OAuth auth, PostgreSQL sessions, recovery, basic profiles and isolated Admin auth to P0-0 infrastructure.
Only `auth` and `identity` are product packages. `cmd/*` composes dependencies, signals and
bounded cleanup; reusable behavior lives in `internal/`.

Auth owns the challenge-mail interface; `internal/mail` implements post-commit local
capture. Challenges persist only easyhash token hashes; security events accept only
IDs, a closed event type and time. Public unsafe authenticated routes require exact
Origin and a session-bound HMAC CSRF value; rotation invalidates the previous value.

Auth owns the narrow OAuth provider and ephemeral-flow interfaces. `oauthprovider`
uses OAuth2/OIDC libraries and returns validated identity fields, never provider
tokens. `redisstore` implements one-use, ten-minute flows under `gfp:auth:oauth:flow:`.
Account resolution uses provider subjects; email collisions require explicit linking.
All existing-user auth mutations take the User lock before credential/identity/
session/challenge locks. Provider network I/O and KDFs stay outside transactions.

Admin uses password-only `kind=admin` sessions (8h absolute, 1h idle, 5m touch),
Strict cookies and independent Origin/CSRF. Current static roles are read on every
request. `adminctl` role writes use the owner, an operator advisory lock then User
lock, with last-active-admin protection. Reset/change revoke both session kinds.
Auth owns subject/global throttle policies; Redis stores HMAC-keyed counters/TTLs.
Public local auth fails open on Redis outage; Admin login/reauth fail closed.

Worker never calls Public/Admin HTTP. Application/domain code must not import Fiber,
transport DTOs, Redis or River. River types stay inside Jobs infrastructure and its
objects live in `river`; business data lives in `app`. PostgreSQL is canonical,
Redis holds disposable state. No ORM, AutoMigrate, vectors, MongoDB or message broker.

OpenAPI owns Go transport and TypeScript clients. Goose migrations plus SQL queries
own sqlc output. Generated files are committed, reviewed, and never manually edited.

Public web uses anonymous Astro Node SSR with isolated React interaction; Admin is
a React SPA. Apps use API-client facades, never deep generated imports or direct DB
access. Tailwind v4 owns layout; shared SCSS and SCSS Modules own visual appearance.
