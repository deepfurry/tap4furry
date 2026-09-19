# Local development

## Toolchain and commands

Use Go 1.27.1+ (one `server/go.mod`, no `go.work`), Node 24 LTS and pnpm 10.11.0.
Go module tools pin oapi-codegen/sqlc; Goose and River migrators use their pinned
official Go APIs. No global Go tools, psql or redis-cli are needed.

| Command | Purpose |
| --- | --- |
| `pnpm install --frozen-lockfile` | Install the committed workspace dependency graph |
| `pnpm generate` | OpenAPI → Fiber v3/TypeScript; Goose/queries → sqlc |
| `pnpm lint` | ESLint, gofmt verification and go vet |
| `pnpm typecheck` | Strict TypeScript and Astro diagnostics |
| `pnpm test` | Native Node generated-client tests and Go behavior tests |
| `pnpm build` | Public SSR, Admin SPA and all three Go runtime binaries |
| `pnpm check` | Secret/boundary audit, generation drift, lint/types/tests/build |
| `pnpm check:generated` | Regenerate and compare exact bytes and file membership |
| `pnpm audit:repository` | Secret, forbidden dependency and architecture checks |
| `pnpm migrate:dev` | Explicit Goose → official River → Worker object grants |
| `pnpm smoke:dev` | Real pgx/Redis and River execution checks with private config |
| `pnpm smoke:auth:dev` | Temporary account through real Public HTTP/application handlers, with fixture cleanup |
| `pnpm smoke:oauth:dev` | Check prepared OAuth pairs, fixed callbacks, PKCE URLs and Redis one-use flows without consent or token exchange |
| `pnpm smoke:admin:dev` | Real Admin identity/grants, login/CSRF/session/role isolation with temporary fixture cleanup |
| `pnpm smoke:mail:resend:dev` | Opt-in real Resend send using private smoke input, without DB state |
| `pnpm adminctl:dev` | Operator-only static role grant/revoke/list using the prepared migrator |
| `pnpm integration:ci` | Fresh, guarded loopback disposable PostgreSQL/Redis tests |
| `pnpm build:images` | Build four local Docker images, without publishing |

`pnpm check` never implicitly migrates or accesses private Infra. Run the separate
integration/image gates when the active spec requires them. Generated source is
committed and never edited manually. sqlc reads the Goose migrations directly;
there is no second schema definition. Tool-only transitive dependencies (for example
sqlc's MySQL/SQLite parsers and gRPC client) are not application architecture/runtime
integrations. No observability framework is configured or imported by application code.

## Private launch inputs

Preserve the existing ignored files:

```text
server/env/api.local
server/env/admin.local
server/env/worker.local
server/env/migrator.local
.local/readonly.env
```

The four `server/env/*.example` files show the public contract using localhost
placeholders. For a new workstation, obtain private credentials through the operator
path and create missing local files only. Never overwrite existing credentials.
Do not print, commit, log or paste these files or real addresses/connection URLs.
The readonly operator input is not an application launch input.

Runtime binaries read process environment only and validate required values. Node's
standard `parseEnv` loads `.local` only in the developer launcher, which refuses CI.
For older prepared files, it supplies missing non-secret defaults: API
`127.0.0.1:8080`, Admin API `127.0.0.1:8081`, Worker/Migrator `RIVER_SCHEMA=river`.
Explicit values win and are still validated. No private file is rewritten.

Start separate terminals:

```text
pnpm dev:api
pnpm dev:admin-api
pnpm dev:worker
pnpm dev:web
pnpm dev:admin
```

Web development serves on port 4321; Admin serves on 5173. Development `/api/*`
proxies strip `/api` and forward to the corresponding Go API. Browser clients use
the generated facade. Both APIs expose GET `/health/live` and `/health/ready`.
Live does not fan out; readiness requires PostgreSQL and reports Redis degradation.
Anonymous public SSR never reads per-user state; `/foundation` is prerendered.

## Local authentication (P0-1A/B/C/D)

Public Web provides `/register`, `/login` and `/account` as anonymous Astro shells
with React islands. Account data is fetched in the browser through `/api/me`; it
never enters shared SSR HTML. The public profile lookup is GET `/api/users/{handle}`.
There is no public profile HTML route yet. Admin provides its own password login and protected workspace.

Public API adds POST `/auth/register`, `/auth/login`, `/auth/logout`, GET `/me`,
PATCH `/me/profile` and GET `/users/{handle}`. PATCH retains omitted fields; null
clears handle/display name/bio. Indexing is an explicit boolean and defaults false.
Public profile JSON contains only handle, display name and bio.

API configuration includes `PUBLIC_ORIGIN`. Development defaults to
`http://localhost:4321`; tests supply an explicit value. Production requires an
HTTPS origin without path/query/fragment/userinfo. Unsafe auth/profile requests
must carry that exact Origin. Do not configure broad CORS. Existing local files
need no edits to use the development default. Other processes do not need it.

easyhash v1.2.0 hashes new passwords with explicit `WithArgon2id()`: 64 MiB memory,
time cost 3, parallelism 2, library-default salt/key lengths. Its default Hash and
DefaultPolicy prefer bcrypt, so Tap4Furry overrides the policy to Argon2id before
`VerifyAndUpgrade`. Successful legacy/bcrypt verification upgrades with a CAS;
concurrent upgrades retry verification once. Equal/stronger current Argon2id hashes
are retained. Rehash does not change the actual password-change timestamp.
Unknown identities perform dummy Argon2id verification. Passwords accept 15–128
Unicode code points, including spaces, without trimming or normalization.

Business IDs use Go 1.27's standard `uuid.NewV7()`. Browser sessions contain 32
random bytes encoded with unpadded base64url; PostgreSQL stores only the SHA-256
hash of that encoded token. Public sessions expire after 30 days absolute or 14
days idle. Activity is touched at most every 10 minutes and never extends absolute
expiry. Revoked/expired sessions and disabled/deleted accounts are rejected.
Production uses `__Host-tap4furry_session`, Secure, HttpOnly, SameSite=Lax, Path=/,
without Domain. Local HTTP uses `tap4furry_session`. Login always issues a new session;
logout revokes the current session and expires the same cookie. No browser storage
contains credentials or session tokens. Auth/profile database work has a bounded
request context; private responses use `Cache-Control: no-store`.

P0-1B adds `/forgot-password`, `/reset-password` and `/verify-email`, plus account
verification resend, password change and session management. Link pages read
`#token=` into React memory and immediately remove it with `history.replaceState`.
Reloading loses that value. No token enters browser storage, SSR state or query
strings. Account shells send `Referrer-Policy: no-referrer` and `Cache-Control: no-store`.

Authenticated unsafe requests require both exact Origin and `X-CSRF-Token`. Obtain
the latter from GET `/auth/csrf`; it is base64url HMAC-SHA256 of the raw session token
using `CSRF_SECRET`, not an authentication credential. Browser helpers fetch a fresh
value before each mutation, so no cache survives a cookie rotation. Anonymous unsafe
requests need Origin only. Session authentication remains the PostgreSQL lookup.

| API-only setting | Development/test | Production |
| --- | --- | --- |
| `CSRF_SECRET` | Explicit public development default if omitted | Private value of at least 32 bytes required; development default rejected |
| `MAIL_MODE` | `local` by default; `disabled` and `resend` allowed | Must explicitly be `resend`; missing/local/disabled rejected |
| `MAIL_LOCAL_DIR` | Defaults to `../.local/mail` when launched from `server`; used only in local mode | Unused |
| `RESEND_API_KEY` | Private non-empty value required only for resend | Private non-empty value required |
| `MAIL_FROM` / `MAIL_REPLY_TO` | Valid single RFC mailbox/address required only for resend | Required; canonical values below |

Existing ignored credentials need no edits. Local captures must stay below the
repository `.local` directory; `os.Root` confines nested paths/symlinks. New capture
directories/files use 0700/0600 where supported; existing broad directory permissions
are rejected on Unix. Windows uses the account's filesystem ACLs. Captures contain
private JSON messages (`To`, `Subject`, `Link`) with random UUID filenames. Inspect
them privately on your workstation, never paste their contents into logs/reports or
expose them through an HTTP route. Links target `PUBLIC_ORIGIN` and use a fragment.

BRAND-0 renames session and OAuth browser-binding cookies without accepting legacy
aliases. Sign in again after updating a local checkout. Existing PostgreSQL sessions
are not migrated or purged. `gfp_*` database/role names and Redis `gfp:` keys remain
stable infrastructure identifiers; no private input needs rewriting.

Auth issues a challenge and commits before calling its consumer-owned mail interface.
Only an easyhash hash is persisted. Verification expires in 24 hours, reset in 30
minutes; issuing the same identity/purpose is limited to once per 60 seconds,
including after consumption. Replacement invalidates the previous challenge.
Registration still succeeds and returns its session cookie if delivery fails.
Reset requests always return the same accepted response for eligible/ineligible
accounts and delivery failure; an authenticated resend may return `MAIL_UNAVAILABLE`.
Delivery failures use static safe logs, without sender errors or private values.

Password reset/change atomically revoke all Public/Admin sessions and issue one Public replacement;
reset does not auto-verify email. Reauthentication replaces only the current session
and refreshes `authenticated_at`. Session management exposes only the user's active
public sessions. Security events persist only IDs, event type and time, in the same
transaction as the corresponding write. MAIL-0 adds production Resend delivery;
SMTP, automatic retries and durable raw-token queues remain out of scope.
Production deployment sign-off is separate from application/human acceptance.

## Transactional mail (MAIL-0)

Only Public API owns mail configuration and delivery. `ChallengeMailer` remains
Auth-owned; the official `github.com/resend/resend-go/v3` SDK is confined to
`internal/mail`. Admin, Worker and Migrator do not read mail settings.

Set production process environment through the existing secret-management path:

```dotenv
MAIL_MODE=resend
MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>
MAIL_REPLY_TO=support@tap4furry.com
```

Supply `RESEND_API_KEY` privately. Never put it in tracked files, shell command
arguments or reports. Production refuses to start without explicit Resend mode,
a non-empty key and valid From/Reply-To addresses. No local directory is required
for Resend. Existing development `.local` inputs need no changes.

Verification/reset mail includes text and minimal HTML, with 24-hour/30-minute
expiration and fragment-only links at `PUBLIC_ORIGIN`. The auth transaction commits
before the single five-second delivery attempt. The adapter uses a fixed Resend
HTTPS endpoint, disallows redirects, and flattens provider/network errors. It does
not print recipients, tokens, URLs, provider bodies or headers. Registration keeps
its account/session on failure; authenticated resend returns `MAIL_UNAVAILABLE`;
reset requests retain the same accepted response regardless of account/delivery.

Open and click tracking remain disabled in the externally prepared Resend domain
settings. No tracking markup, remote images, custom metadata, CC/BCC, attachment,
template system, webhook, SMTP, River mail job, retry or outbox is added. Tokens
remain non-durable in application storage; the existing private development
capture is an explicit local debugging facility, never a production path.

For a real send, privately prepare the ignored `.local/resend-smoke.env` with a
restricted `RESEND_API_KEY` and `RESEND_TEST_RECIPIENT` controlled by the operator.
The file may optionally override `MAIL_FROM`, `MAIL_REPLY_TO` and `PUBLIC_ORIGIN`.
Defaults are the canonical sender/reply address above and `http://localhost:4321`.
Then run from the repository root:

```text
pnpm smoke:mail:resend:dev
```

The command refuses CI, requires the private file, generates a random synthetic
token in memory, and calls the real adapter exactly once. It creates no User or DB
challenge and needs no database/Redis credentials. The emailed link is intentionally
not redeemable. Only a safe PASS/failure summary is printed; PASS means Resend
accepted the message, not proof of inbox delivery. There is no automatic retry.
If networking requires a local proxy, provide normal process-scoped HTTP(S) proxy
settings; never change the shared Infra or weaken TLS. Do not paste any private
file or email content into diagnostics. CI uses only injected fake senders and
local temporary capture; it never sends real mail.

## Admin authentication and role operations (P0-1D)

Admin SPA serves `http://localhost:5173`; its `/api/*` proxy targets Admin API port
8081. `/login` accepts email/password; `/` requires Admin `/me` and redirects to
login on 401 or lost role access. The workspace displays current roles and Admin
sessions, with reauthentication, current/other session revocation and logout.
Passwords and CSRF exist only transiently in memory; session cookies remain HttpOnly.

| Setting | Development/test | Production |
| --- | --- | --- |
| `ADMIN_ORIGIN` | Development defaults to `http://localhost:5173`; test sets it explicitly | Exact HTTPS origin, normally `https://admin.tap4furry.com` |
| `ADMIN_CSRF_SECRET` | Separate public development default | Explicit private >=32 bytes, different from Public CSRF |
| `AUTH_THROTTLE_SECRET` | Public development default | Explicit private >=32 bytes, shared between API and Admin |

Only active, non-deleted Users with a verified local email, password credential and
at least one static privileged role may log in. No role means ordinary User; no
`user` role row exists. Multiple roles are additive:

| Role | Capabilities |
| --- | --- |
| `moderator` | AdminAccess, Moderation |
| `editor` | AdminAccess, Editorial |
| `admin` | AdminAccess, Moderation, Editorial, Administration |

Use the existing verified local account, then run from the repository root:

```text
pnpm adminctl:dev grant-role -email operator@example.invalid -role admin
pnpm adminctl:dev list-roles -email operator@example.invalid
pnpm adminctl:dev revoke-role -email operator@example.invalid -role moderator
```

The address above is an example, not a seeded account. There is no default Admin
password or automatic grant. `adminctl` checks actual `gfp_migrator`/`gfp_dev` and
uses standard flags; the Admin HTTP runtime cannot change roles. Grants are
idempotent. A transaction advisory lock serializes operators before the target
User lock. Revoking the last active `admin` fails. Revoking a User's final privileged
role also revokes every Admin session in the same transaction. Roles are re-read
on each request, so a session never serves as a cached role grant.

Admin shares `app.sessions` but requires `kind=admin`, `auth_method=password`.
Absolute expiry is 8h, idle 1h, touch at most every 5m. Production cookie is
`__Host-tap4furry_admin_session` (Secure, HttpOnly, Strict, Path=/, no Domain); local
HTTP uses `tap4furry_admin_session`. No Public session exchange or OAuth login exists
on Admin. Normal logout/revocation affects its own kind only. Password reset/change
revoke both kinds atomically and issue exactly one replacement Public session.

Auth throttle policy constants:

| Operation | Subject | Global | Window |
| --- | --- | --- | --- |
| Public login failures | 10 | 500 | 15m |
| Admin login failures | 5 | 100 | 15m |
| Registration attempts | 3 | 200 | 1h |
| Password reset requests | 3 | 200 | 1h |
| Public password verification failures (change/reauth) | 5 per User | 500 | 15m |
| Admin reauth failures | 5 per User | 100 | 15m |

Keys are `gfp:auth:limit:` plus HMAC-SHA256 of operation/dimension/normalized email
or User ID. Values are only counters with TTLs. Global fingerprints contain no
identity. No IP/forwarding header is used. EVAL atomically checks and saturates both
dimensions using GET/INCR/EXPIRE/TTL; DEL clears only the successful subject. The
limits count completed failures; a pre-check avoids KDFs once a bucket is exhausted.
Already admitted concurrent requests can finish, but cannot exceed the bounded
failure-event count. Blocked attempts never prolong expiry. No KEYS/SCAN is used.

Redis failure leaves Public local login/register/reset/password verification open
with safe static warnings; no unbounded PostgreSQL failure events are written.
Admin login and reauth fail closed (503 `INTERNAL_ERROR`); existing canonical Admin
sessions remain usable. Limited login/register/reauth returns 429 `AUTH_RATE_LIMITED`.
Limited reset still returns the same 202/body and sends no challenge. OAuth retains
its independent fail-closed one-use flow dependency. CI grants only the required
Redis commands to its disposable runtime user; shared ACLs are never changed.

`pnpm smoke:admin:dev` validates `gfp_admin`, `gfp_api`, `gfp_migrator` on `gfp_dev`
and inspects role-table grants. It registers a random account through the Public
application, captures its post-commit verification token only in process memory,
verifies it, grants a temporary moderator through the operator path, and runs Admin
HTTP login/me/CSRF/rotation/session revocation/cookie isolation. Finally it removes
the role, checks immediate access loss and cleans only that generated account and
dependent rows. Separate `smoke:auth:dev` validates private filesystem mail capture.

P0-1 application implementation can be complete while human sign-off remains pending:
real Google/GitHub login/callback/link/reauth/unlink and an operator-led Admin browser
walkthrough. Production also needs Cloudflare Access with enforced MFA, real OAuth
registrations/callbacks, private Resend configuration and delivery sign-off, separate private CSRF/throttle
secrets, deployment secret management, trusted proxy policy, and a Turnstile decision.
No Cloudflare provisioning, application TOTP/WebAuthn, dynamic RBAC or public role
editor is included. Local/private mail capture is never a production delivery path.

## Migrations and shared Infra

Shared development Infra is accessed only using private configuration. No SSH,
server/container administration, cluster-role changes or Redis ACL changes are part
of ordinary work. Migrator may apply repository-owned changes only to `gfp_dev`.

`pnpm migrate:dev` checks the migrator identity/database, ensures Goose's bookkeeping
namespace `app`, runs Goose with `app.goose_db_version`, then official `rivermigrate`
with `Schema: river`, and grants Worker only pinned River runtime object access.
Goose owns namespace/extension foundation; River owns all SQL inside `river`.
Migration 1 retains pre-existing schemas/extensions on down; recovery uses new
forward migrations. Once applied to shared Infra, do not edit it in place.
Migration 2 adds the five identity/auth tables with restrictive FKs, CHECK/UNIQUE
constraints and explicit API DML. Worker gets no identity DML; Admin gains only migration 5 grants. Always pass
disposable migration and auth tests before applying new migrations to shared dev.
Migration 3 adds challenges and security events with minimal API grants and readonly
SELECT. Security event identity insertion requires no direct sequence grant. Migration
history 1–5 is immutable once applied to shared development. Migration 4 adds OAuth
event/session constraints, one identity per User/provider, and minimal owned-object
grants for User locking, provider email metadata and explicit unlinking.

River 0.47.0 refuses to start with zero registered workers (`client.go`, `Start`).
The only P0-0 job is `infrastructure.probe.v1`: an explicit smoke request that runs
the system readiness query. Normal worker startup does not enqueue or schedule it.
Smoke waits for completion, removes its own job, and checks bounded Worker shutdown.
Public/Admin have no River enqueue grants. No future product jobs are scaffolded.

The official paths used are [oapi-codegen Fiber v3](https://github.com/oapi-codegen/oapi-codegen/blob/v2.8.0/docs/fiber-v3-server.md)
and [River explicit alternate schema](https://riverqueue.com/docs/alternate-schema).

`pnpm smoke:dev` verifies all four prepared PostgreSQL identities, schema/extension
state and sqlc query, Redis PING and a unique TTL-bound `gfp:*` SET/GET/DEL, then
Worker River enqueue/execution/completion/cleanup. Driver errors retain only safe
operation labels and PostgreSQL SQLSTATE. Missing privileges are a stop condition.

After disposable acceptance, `pnpm smoke:auth:dev` checks `gfp_api`/`gfp_dev`, then
registers, verifies email through private capture, resets/changes passwords, rotates
authentication, lists/revokes sessions, checks CSRF, reads/updates a profile and logs out through the actual Fiber
handlers/application code with real pgx connections. It does not require an already
running API listener. No email/password/cookie/hash/URL is printed. The prepared
`gfp_migrator` connection deletes only that process's randomly named temporary
identity, challenges, events and dependent records in a restrictive-FK-safe transaction;
the run also removes only its own randomly named capture subdirectory. No runtime
DELETE grant, shared server administration or production user deletion is involved.

## Disposable CI and containers

GitHub Actions provisions fresh PostgreSQL 18 and Redis 8 service containers. It
never loads private files or uses Tailscale. `integration:ci` requires `CI=true` and
`GFP_DISPOSABLE_INFRA=1`, uses fixed loopback endpoints and creates `gfp_ci` plus
restricted roles. Its published passwords are disposable fixtures, not developer
credentials. Running it against an existing initialized CI database fails deliberately;
use fresh containers for each acceptance run. Only this explicitly guarded fixture
setup creates cluster roles, on the disposable server.

CI reuses `pnpm check`, repeats generation with Git drift/untracked-file checks,
runs fresh migrations twice, driver smoke and P0-1A/B/C/D auth/database/HTTP/privacy tests,
then builds all four images. Third
party Actions are pinned to commit SHAs. No deployment, tag, release or image push.

See [deploy/README.md](../deploy/README.md) for images and runtime ports. Local image
acceptance needs a working Docker engine; lack of Docker must be reported as an
unexecuted gate, never a passing build.

To rerun only auth integration against an already initialized disposable fixture,
set `CI=true`, `GFP_DISPOSABLE_INFRA=1`, `GFP_AUTH_INTEGRATION=1` and run
`go -C server test -count=1 -run TestIntegration ./internal/transport/public ./internal/redisstore ./internal/oauthprovider`.
Tests hard-code the disposable loopback database and never read developer URLs.
The ordinary Go test suite skips these integration tests until explicitly enabled.
CI mail tests use fake delivery or `t.TempDir`, never network email or developer captures.

## Google/GitHub OAuth and account linking (P0-1C)

Configure only `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`,
`GITHUB_OAUTH_CLIENT_ID`, `GITHUB_OAUTH_CLIENT_SECRET` in the existing private API
launch input. Each pair is both present or both absent; partial pairs fail without
printing values. Disabled providers do not disable local password authentication.
No callback environment variables or browser provider SDKs are used.

| Provider | Development callback | Future production callback |
| --- | --- | --- |
| Google | `http://localhost:4321/api/auth/oauth/google/callback` | `https://tap4furry.com/api/auth/oauth/google/callback` |
| GitHub | `http://localhost:4321/api/auth/oauth/github/callback` | `https://tap4furry.com/api/auth/oauth/github/callback` |

Both callbacks are derived exclusively from `PUBLIC_ORIGIN`. Configure those exact
URLs privately with the corresponding provider. Google requests only `openid email
profile`; GitHub requests only `read:user user:email`. Both use Authorization Code
with S256 PKCE. Google additionally uses a nonce and verified OIDC ID tokens. No
offline access or refresh-token storage is requested. API callback work has a bounded
15-second context; individual outbound HTTP requests time out after five seconds.

Redis needs `SET` with NX/EX and `GETDEL` within the prepared `gfp:*` namespace.
The runtime never changes ACLs. CI adds GETDEL only to its disposable runtime role;
TTL inspection/forced expiry use its separate local test inspector. State has 256
random bits, keys contain its SHA-256 digest and flows expire in ten minutes. A
short-lived HttpOnly SameSite=Lax cookie binds each provider flow to its browser.
Binding cookies are `tap4furry_oauth_google` / `tap4furry_oauth_github` locally;
production adds the `__Host-` prefix and Secure. Callback consumes state
once, including denial/invalid-flow paths, and redirects only to `/account` or
`/login?oauth_error=<fixed-code>` (account errors remain on `/account`). Do not add
access logs containing OAuth callback query strings or authorization headers.

Start login at `/api/auth/oauth/{provider}/start`. Account security lists private
sign-in methods, supports explicit linking, provider/password reauthentication and
unlinking. Links require authentication within 15 minutes at start and callback.
Unlinking requires another method and a current session authenticated by a remaining
method. All mutations require exact Origin and a session-bound CSRF header.
Link/unlink rotations preserve the method and original authentication time;
reauthentication refreshes both. Removing a provider revokes its public sessions.

Provider subjects, not email, select accounts. Email collisions require signing into
the existing account and linking explicitly. An OAuth-only account has an ordinary
email identity but no password. Its password reset request is an enumeration-safe
no-op; it can verify email, edit its profile and manage sessions normally. Google
third-party emails remain locally unverified until Tap4Furry verification succeeds;
verified Gmail/Workspace emails and selected verified GitHub emails are trusted.
Provider profile updates never overwrite the user's edited profile.

After disposable acceptance, phases adding migrations run `pnpm migrate:dev`.
BRAND-0 applies no shared migration. Run `pnpm smoke:dev`, `pnpm smoke:auth:dev`,
`pnpm smoke:oauth:dev` and `pnpm smoke:admin:dev`. The OAuth smoke prints only provider
names and boolean outcomes: it neither prints URLs/credentials nor exchanges real
codes. Missing pairs/callback or Redis capabilities are stop conditions. No shared
server administration or cluster-role changes are permitted.

For interactive verification, run API and Web, open `/login`, choose the provider,
complete human consent and check `/account`. Then explicitly link from a password
account, reauthenticate and unlink using another method. Report Google and GitHub
separately as PASS only after actual consent; otherwise record
`NOT RUN — human provider consent required`. Automated fake providers, local signed
OIDC/JWKS fixtures and concurrency tests never use real credentials or Internet IdPs.

Official behavior verified for this phase:
[Google OIDC](https://developers.google.com/identity/openid-connect/openid-connect),
[Google S256 discovery metadata](https://accounts.google.com/.well-known/openid-configuration),
[Google authoritative email policy](https://developers.google.com/identity/gsi/web/guides/verify-google-id-token),
[GitHub Web Flow and PKCE](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps),
[GitHub email API](https://docs.github.com/en/rest/users/emails?apiVersion=2026-03-10).
