# P0-1B — Session Security, Verification & Recovery

**Repository:** `deepfurry/tap4furry`\
**Target branch:** `dev`  
**Release snapshot branch:** `main`  
**Prerequisite:** P0-1A complete

## 1. Goal

P0-1A established:

```text
Email + Password
      ↓
User / Profile
      ↓
Opaque Public Session
```

P0-1B turns that foundation into a durable account-security system.

This phase adds:

```text
Full session-bound CSRF
Email verification
Password reset
Password change
Re-authentication / session rotation
Active session management
Security Events
Safe development email delivery
```

At completion a user must be able to verify their email, recover/change a password, rotate authentication, list/revoke public sessions, and use a full session-bound CSRF token on authenticated unsafe requests.

P0-1B is still development-stage authentication. Google/GitHub OAuth, Admin auth, final abuse/rate-limit hardening, and a real production email provider remain later work.

---

## 2. Current P0-1A Baseline

Extend the actual current `dev` implementation. Do not rebuild auth.

Expected current state:

```text
00001_foundation.sql
00002_identity_local_auth.sql

app.users
app.user_profiles
app.auth_identities
app.password_credentials
app.sessions
```

Current Public API:

```text
POST  /auth/register
POST  /auth/login
POST  /auth/logout
GET   /me
PATCH /me/profile
GET   /users/{handle}
GET   /health/live
GET   /health/ready
```

Current security baseline:

```text
easyhash + explicit Argon2id
opaque 256-bit public session token
SHA-256 session lookup hash in PostgreSQL
30d absolute / 14d idle
10m touch interval
production __Host-tap4furry_session
exact PUBLIC_ORIGIN guard
```

Important existing packages:

```text
server/internal/auth
server/internal/identity
server/internal/transport/public
server/internal/database/sqlc
```

Do not edit migrations 00001 or 00002.

---

## 3. Branch / Execution Contract

Work only on `dev`.

Before editing:

```bash
git branch --show-current
git status --short
git log -5 --oneline --decorate
```

Rules:

- preserve unrelated user changes;
- do not rewrite existing commits/migrations;
- do not modify/merge `main`;
- do not tag/release;
- do not push unless explicitly instructed;
- after verification, commit locally on `dev`.

Recommended commit:

```text
feat: add session security and account recovery
```

---

## 4. Required Reading

Read:

1. `AGENTS.md`
2. this P0-1B spec
3. `.agents/architecture.md`
4. `.agents/playbook.md`
5. `contracts/architecture.md`
6. `contracts/database.md`
7. `contracts/development.md`
8. `contracts/generated-code.md`
9. `docs/product/PRODUCT.md`
10. `docs/product/domain-model.md`
11. `docs/architecture/security.md`
12. `docs/architecture/backend.md`
13. `docs/architecture/data.md`
14. `docs/architecture/frontend.md`
15. `docs/engineering/tech-stack.md`

Inspect the real current implementation, especially:

```text
server/internal/auth/*
server/internal/identity/*
server/internal/transport/public/*
server/db/queries/*
contracts/openapi/public.yaml
server/internal/config/config.go
apps/web/src/*
```

Re-check the current `github.com/gofurry/easyhash` API, especially:

```text
GenerateToken
HashToken
VerifyToken
Hash / Argon2id
VerifyAndUpgrade
```

Do not preload unrelated Resource/Discovery/Trust docs.

---

## 5. Scope

P0-1B includes:

```text
CSRF token contract
auth_challenges
security_events
Email Verification
Password Reset
Password Change
Re-authentication
Session Rotation
Session Listing
Single Session Revocation
Revoke Other Sessions
Development Mail Capture
Minimal Public Web security flows
```

P0-1B does not include:

```text
Google/GitHub OAuth
provider link/unlink
email-address change
Admin authentication
user_roles / RBAC
Turnstile
multi-dimensional Redis rate limiting
production mail provider
general notification system
general mail queue
JWT
NATS
MongoDB
pgvector
Resource domain
```

Do not create placeholder tables/packages for those.

---

## 6. Migration 00003

Create:

```text
server/db/migrations/00003_auth_security_recovery.sql
```

or the next sequential migration if repository history changed.

Introduce exactly:

```text
app.auth_challenges
app.security_events
```

Do not create other future P0-1 tables.

### 6.1 `app.auth_challenges`

Conceptual schema:

```text
id                uuid PRIMARY KEY
user_id           uuid NOT NULL
auth_identity_id  uuid NOT NULL
purpose           text NOT NULL
token_hash        text NOT NULL
created_at        timestamptz NOT NULL
expires_at        timestamptz NOT NULL
consumed_at       timestamptz NULL
invalidated_at    timestamptz NULL
```

FK:

```text
user_id          → app.users(id) RESTRICT
auth_identity_id → app.auth_identities(id) RESTRICT
```

Purpose values implemented now:

```text
email_verify
password_reset
```

Use DB CHECKs for current real values only.

Invariants:

```text
expires_at > created_at
NOT (consumed_at IS NOT NULL AND invalidated_at IS NOT NULL)
UNIQUE(token_hash)
```

A usable challenge requires:

```text
consumed_at IS NULL
invalidated_at IS NULL
expires_at > now
```

At most one non-consumed/non-invalidated challenge may exist per:

```text
auth_identity_id + purpose
```

Use an appropriate partial unique index.

Replacement issuance must invalidate the previous active challenge before creating a new one.

Challenge IDs are Go UUIDv7.

### 6.2 `app.security_events`

Security Events remain distinct from business Audit logs.

Conceptual schema:

```text
id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY
user_id      uuid NULL
session_id   uuid NULL
event_type   text NOT NULL
occurred_at  timestamptz NOT NULL
```

`user_id` may reference `app.users(id)` restrictively.

`session_id` is a historical identifier and should not create a FK that blocks future session cleanup.

Do not add JSON metadata unless current implementation truly needs it.

P0-1B event types:

```text
account_registered
login_succeeded
logout
email_verification_requested
email_verified
password_reset_requested
password_reset_completed
password_changed
reauthenticated
session_revoked
other_sessions_revoked
```

Do not add high-volume persistent `login_failure` yet; P0-1D owns failure/rate-limit hardening.

Security Events must never contain:

```text
email
password
password hash
session token/hash
CSRF token
challenge token/hash
cookies
authorization headers
```

### 6.3 Privileges

Extend only `gfp_api` rights actually required.

Expected new rights include:

```text
auth_challenges → SELECT/INSERT/UPDATE
security_events → INSERT
auth_identities → UPDATE verified_at/updated_at
```

Grant the identity-sequence permission needed for security-event insertion if PostgreSQL requires it.

Do not grant P0-1B DML to `gfp_worker` or `gfp_admin`.

`gfp_readonly` may receive SELECT according to the existing contract.

No superuser or ownership changes.

---

## 7. Challenge Token Contract

Use `easyhash`:

```go
token, err := easyhash.GenerateToken()
tokenHash, err := easyhash.HashToken(token)
```

Persist only `tokenHash`.

The raw token may exist only in:

```text
temporary Go memory
recipient email
private local mail capture
browser memory while consuming link
```

Never persist the raw token to:

```text
PostgreSQL
Redis
River
security_events
slog
tracked files
```

Never return it from normal API responses.

Use deterministic token-hash derivation for indexed lookup; do not scan rows and verify one-by-one.

Lifetimes:

```text
email verification = 24h
password reset      = 30m
```

Reissue cooldown:

```text
60s
```

During cooldown, return normal success but do not issue/send again.

This cooldown is not the final P0-1D abuse-rate-limit system.

---

## 8. Full Session-bound CSRF

Keep P0-1A exact-Origin enforcement and add:

```text
HttpOnly session cookie
+
session-bound CSRF token
+
exact Origin
```

Authenticated unsafe requests require both Origin and CSRF.

### 8.1 Derivation

Use standard-library HMAC-SHA256:

```text
csrf = Base64URL(
  HMAC-SHA256(CSRF_SECRET, raw_session_token)
)
```

Use constant-time comparison.

Do not store CSRF tokens in PostgreSQL/Redis.

`CSRF_SECRET` is not a session-authentication secret. Session authentication remains the PostgreSQL session lookup.

### 8.2 Config

Add:

```text
CSRF_SECRET
```

Production:

- required;
- at least 32 bytes of secret material;
- no built-in production default.

Development/test may use an explicit development-only default or injected test value.

Update `server/env/api.example` and `docs/development.md`.

### 8.3 Endpoint

Add:

```text
GET /auth/csrf
```

Requires valid public session.

Return:

```json
{"csrf_token":"..."}
```

Use `Cache-Control: no-store`.

### 8.4 Enforcement

Require `X-CSRF-Token` plus exact Origin on authenticated unsafe routes, including existing:

```text
POST  /auth/logout
PATCH /me/profile
```

and all new authenticated unsafe P0-1B routes.

Anonymous unsafe routes such as register/login/reset-consume/verification-consume continue to require exact Origin but no session CSRF.

Session rotation automatically invalidates the previous CSRF token.

Frontend must discard cached CSRF after:

```text
register
login
password reset
password change
reauthentication
current-session revocation
```

and fetch a new one when required.

---

## 9. Email Verification

`app.auth_identities.verified_at` already exists.

### Registration

Successful local registration should create an `email_verify` challenge and attempt delivery only after challenge state is durably committed.

Mail failure must not roll back a committed account.

Never send a link for a transaction that did not commit.

### Resend

Add authenticated:

```text
POST /auth/email/verification/request
```

Requires:

```text
public session
exact Origin
CSRF
active/non-deleted account
local email identity
```

If already verified: accepted/no-op.

Honor cooldown.

Replace old active verification challenge with a new one.

Return:

```text
202 Accepted
```

Never return raw token.

### Consume

Add anonymous:

```text
POST /auth/email/verification
```

Body:

```text
token
```

Requires exact Origin.

Flow:

1. derive token hash;
2. locate active `email_verify` challenge;
3. lock/revalidate;
4. require active/non-deleted user;
5. set `verified_at` exactly once;
6. consume challenge;
7. invalidate remaining active verification challenges for the identity;
8. insert `email_verified` Security Event in same transaction;
9. commit;
10. return `204`.

Invalid/expired/consumed/invalidated/wrong-purpose tokens use one generic public error.

---

## 10. Password Reset Request

Add anonymous:

```text
POST /auth/password/reset/request
```

Body:

```text
email
```

Requirements:

- reuse existing email normalization;
- same public response whether eligible account exists or not;
- if eligible active local-password account exists:
  - cooldown;
  - invalidate old active reset challenge;
  - create new challenge;
  - record `password_reset_requested`;
  - deliver reset link;
- otherwise do bounded dummy work and return the same response.

Always return:

```text
202 Accepted
```

with enumeration-resistant semantics such as:

> If an eligible account exists, a reset message has been sent.

Do not reveal nonexistent/OAuth-only/disabled/deleted states.

Multi-dimensional Redis rate limiting remains P0-1D.

---

## 11. Password Reset Consume

Add:

```text
POST /auth/password/reset
```

Body:

```text
token
new_password
```

Avoid holding a DB transaction during Argon2id.

Recommended flow:

1. validate/derive token hash;
2. perform initial challenge lookup;
3. validate password policy;
4. hash new password with explicit easyhash Argon2id outside transaction;
5. begin transaction;
6. lock/re-read and revalidate challenge;
7. update password credential;
8. consume current reset challenge;
9. invalidate all outstanding reset challenges;
10. revoke all active public sessions;
11. create one replacement public session;
12. record `password_reset_completed`;
13. commit;
14. set replacement cookie.

The replacement session may use:

```text
auth_method = password_reset
```

Generalize existing session creation only as much as necessary for explicit `auth_method`.

Password reset must **not** automatically set email `verified_at`.

A reset token is single-use.

---

## 12. Password Change

Add authenticated:

```text
POST /auth/password/change
```

Body:

```text
current_password
new_password
```

Requires:

```text
public session
Origin
CSRF
```

Verify current password using existing easyhash policy.

Validate/hash new password outside transaction.

Use compare-and-swap against the verified stored hash.

On success, coherently:

```text
update password
invalidate outstanding reset challenges
revoke all active public sessions
create one replacement public session
record password_changed
```

Set the new cookie.

Old current and other sessions must become invalid immediately.

Wrong current password returns a safe re-authentication error.

---

## 13. Re-authentication / Step-up Foundation

Add:

```text
POST /auth/reauthenticate
```

Body:

```text
password
```

Requires public session + Origin + CSRF.

Verify current local password, then:

```text
create replacement public session
authenticated_at = now
auth_method = password
revoke old current session
record reauthenticated
commit
set replacement cookie
return 204
```

This establishes the primitive P0-1C will use for sensitive provider link/unlink operations.

Future freshness target:

```text
15m
```

Do not implement OAuth re-auth here.

---

## 14. Session Management

### List

Add:

```text
GET /me/sessions
```

Return only own active public sessions.

Fields:

```text
id
auth_method
authenticated_at
created_at
last_seen_at
idle_expires_at
absolute_expires_at
current
```

Do not add IP/geolocation/device fingerprint/User-Agent persistence just for UI.

Never expose token/token_hash.

### Revoke one

Add:

```text
DELETE /me/sessions/{session_id}
```

Requires Origin + CSRF.

Target must belong to current user and be a public session.

Mark revoked; do not hard-delete.

Record `session_revoked`.

If target is current session, clear cookie.

Foreign and nonexistent session IDs use same not-found response.

### Revoke others

Add:

```text
POST /me/sessions/revoke-others
```

Requires Origin + CSRF.

Revoke all other active public sessions while preserving current session.

Record `other_sessions_revoked`.

Return `204`.

This protective action does not need an extra re-authentication step.

---

## 15. Session Rotation Invariants

Rotation must:

1. generate a fresh random >=256-bit token;
2. store only SHA-256 lookup hash;
3. create a new session row;
4. revoke superseded session(s);
5. commit before sending the cookie;
6. never expose raw token through JSON;
7. invalidate the old CSRF token.

P0-1B rotations:

```text
reauthentication
password change
password reset
```

---

## 16. Security Events in Existing Flows

Now extend P0-1A:

Registration transaction:

```text
account_registered
```

Login transaction:

```text
login_succeeded
```

Logout:

```text
revoke session + logout event
```

coherently.

Do not add a generic event bus.

Security Events are direct canonical PostgreSQL writes.

---

## 17. Mail Boundary

No production email provider is selected yet.

Implement a narrow consumer-owned interface, conceptually:

```go
type ChallengeMailer interface {
    SendEmailVerification(...)
    SendPasswordReset(...)
}
```

Auth/Application owns the interface; infrastructure implements it.

Do not make domain code depend on a provider SDK.

### Development local capture

Implement a local sender under `server/internal/mail` or equivalent.

Write only under ignored private local storage, preferably:

```text
.local/mail/
```

Requirements:

- filenames contain no email/token;
- use random/message IDs;
- message content may contain recipient/link because directory is private;
- never log message body/raw token/link;
- restrictive permissions where supported;
- tests use temp dirs/fake sender;
- no HTTP mail-browser endpoint.

Links:

```text
<PUBLIC_ORIGIN>/verify-email#token=<token>
<PUBLIC_ORIGIN>/reset-password#token=<token>
```

Use URL fragments intentionally so tokens are not sent to Astro/Web servers.

### Production

Do not add a real production provider in P0-1B.

`MAIL_MODE=local` must be rejected in production.

Do not add an arbitrary SMTP/provider dependency merely to satisfy this phase.

The project remains explicitly not production-auth-ready until a real provider and final hardening are completed.

---

## 18. River Boundary

Do **not** persist raw verification/reset tokens in River job payloads.

Do not create `mail.send.v1` carrying raw challenge tokens.

P0-1B uses the narrow direct challenge-mail sender because raw one-time tokens must not become durable River data.

If current docs say all email is asynchronous, document this security-sensitive exception.

General async email may be revisited later only with a secure token-handoff design.

---

## 19. Config

Add minimally:

```text
CSRF_SECRET
MAIL_MODE
MAIL_LOCAL_DIR
```

Recommended development behavior:

```text
APP_ENV=development
CSRF_SECRET    → development-only default allowed
MAIL_MODE      → local
MAIL_LOCAL_DIR → ../.local/mail
```

Production:

```text
CSRF_SECRET explicit
MAIL_MODE=local rejected
```

Do not add JWT signing secrets.

Update:

```text
server/env/api.example
docs/development.md
```

Avoid modifying ignored real `api.local` when safe dev defaults suffice.

---

## 20. Public API Contract

Extend `contracts/openapi/public.yaml`.

Keep Admin API unchanged.

Add:

```text
GET  /auth/csrf

POST /auth/email/verification/request
POST /auth/email/verification

POST /auth/password/reset/request
POST /auth/password/reset
POST /auth/password/change
POST /auth/reauthenticate

GET    /me/sessions
DELETE /me/sessions/{session_id}
POST   /me/sessions/revoke-others
```

Recommended status codes:

```text
GET  /auth/csrf                        200
POST /auth/email/verification/request  202
POST /auth/email/verification          204
POST /auth/password/reset/request      202
POST /auth/password/reset              200
POST /auth/password/change             200
POST /auth/reauthenticate              204
GET    /me/sessions                    200
DELETE /me/sessions/{id}               204
POST   /me/sessions/revoke-others      204
```

Model `X-CSRF-Token` explicitly on authenticated unsafe endpoints.

Recommended new error codes:

```text
CSRF_INVALID
AUTH_CHALLENGE_INVALID
AUTH_REAUTH_FAILED
AUTH_SESSION_NOT_FOUND
MAIL_UNAVAILABLE
```

Do not expose internal challenge state, session ownership, or account existence.

Generated OpenAPI DTOs remain transport-only.

Regenerate Go + Orval clients.

---

## 21. sqlc

Extend existing SQL files rather than creating repository wrappers.

Required capabilities include equivalents of:

### Challenges

```text
InvalidateActiveChallenges
CreateAuthChallenge
FindActiveChallengeByTokenHash
FindActiveChallengeByTokenHashForUpdate
ConsumeChallenge
GetLatestActiveChallenge
```

### Identity

```text
GetLocalEmailIdentityForUser
MarkEmailIdentityVerified
```

### Credentials

```text
GetPasswordCredentialByUser
CompareAndSwapPasswordHash
SetPasswordHashForReset
```

### Sessions

```text
CreateSession(auth_method)
ListActivePublicSessions
RevokePublicSessionByID
RevokeOtherPublicSessions
RevokeAllPublicSessions
```

### Events

```text
InsertSecurityEvent
```

No generic repository/event framework.

---

## 22. Transaction Boundaries

Challenge issuance:

```text
invalidate old
create new
security event
commit
then deliver email
```

Email verify:

```text
lock/revalidate
verify identity
consume/invalidate challenges
security event
commit
```

Password reset:

```text
hash outside tx
lock/revalidate challenge
set password
consume/invalidate reset challenges
revoke sessions
create replacement
security event
commit
```

Password change:

```text
verify/hash outside tx
CAS password
invalidate reset challenges
revoke all sessions
create replacement
security event
commit
```

Re-auth:

```text
verify outside tx
create replacement
revoke old
security event
commit
```

Never hold DB transactions during network/file mail delivery or Argon2id work.

---

## 23. Concurrency

Challenge consume must be race-safe:

```text
two concurrent consumes
→ exactly one success
```

Use row locking/state predicates.

Password change preserves CAS safety.

Session rotation must not leave partial replacement state.

DB constraints remain correctness boundaries.

---

## 24. Frontend

Extend Public Web only.

Add:

```text
/forgot-password
/reset-password
/verify-email
```

Extend `/account` with:

```text
email verification state
resend verification
change password
active sessions
revoke session
revoke others
logout
```

Do not redesign the product.

### CSRF client

Authenticated React islands:

1. fetch `/api/auth/csrf`;
2. hold token only in runtime/React memory;
3. send `X-CSRF-Token`;
4. discard it on session rotation;
5. fetch new token as needed.

Never store CSRF in localStorage/sessionStorage/IndexedDB.

### Verify/reset links

Token pages read token from `location.hash`, immediately clear the fragment with History API while keeping token only in memory, then consume it.

Use:

```text
Referrer-Policy: no-referrer
```

on these pages.

A page refresh after token removal may lose the token; reopening the email link is acceptable.

Forgot-password UI always shows the same generic confirmation.

---

## 25. Security-event Privacy

Event insertion should use typed fields only:

```text
event type
user id
session id
time
```

Do not accept arbitrary caller payload that can accidentally serialize secrets.

Add tests ensuring no Security Event schema/row contains email/token/password data.

---

## 26. Tests

Required test coverage:

### CSRF

```text
valid same-session token succeeds
missing token rejected
wrong token rejected
cross-session token rejected
wrong Origin rejected
old CSRF rejected after rotation
GET does not require CSRF
anonymous reset/verify consume uses Origin only
```

### Challenges

```text
only hash persisted
raw token absent from DB
expiry
single-use
replacement invalidates old
cooldown
wrong-purpose rejected
concurrent consume exactly once
```

### Verification

```text
register issues challenge
resend works
verified_at changes on valid consume only
/me email_verified updates
repeat consume fails
```

### Password reset

```text
existing/nonexistent same public response
old password fails after reset
all old sessions revoked
replacement session works
single-use reset token
does not auto-verify email
generic invalid/expired error
```

### Password change

```text
wrong current password rejected
new Argon2id hash
old password rejected
reset challenges invalidated
all old sessions revoked
replacement valid
```

### Re-auth

```text
correct password rotates
old session rejected
authenticated_at refreshed
old CSRF rejected
```

### Sessions

```text
list own active sessions only
current marker
revoke other
revoke current
foreign UUID privacy
revoke-others preserves current
```

### Security Events

Verify expected events for register/login/logout/verify/reset/change/reauth/revoke.

### Mail capture

```text
private configured directory only
filename has no email/token
links use #token=
no token/message-body logs
production rejects local mode
```

---

## 27. Real Development Infra Verification

After disposable tests pass, migrate real `gfp_dev` using `gfp_migrator`.

Run Public API with `gfp_api`.

No SSH and no manual role/ACL changes.

Use a new temporary development account and verify:

```text
register
verification mail capture
verify email
/me email_verified=true

create another login session
list sessions
revoke other session

request reset
reset mail capture
reset password
old sessions rejected
new session valid

change password
session rotation valid

CSRF enforcement valid
```

Do not include raw verification/reset/session/CSRF tokens in final report.

Do not print DSNs/passwords/Tailnet addresses.

---

## 28. CI

Existing P0-1A CI must remain green.

CI mail uses temp directory or fake sender, never network email.

CI must never access shared Infra.

Keep gates:

```text
pnpm check
pnpm generate
generated drift
fresh PostgreSQL migrations
Go tests
HTTP integration
frontend typecheck/build
Docker builds
secret/repository audit
```

No final Redis rate-limit tests are required in P0-1B.

---

## 29. Documentation

Update:

```text
CHANGELOG.md
docs/development.md
docs/architecture/security.md
```

Update `docs/engineering/jobs-automation.md` only if necessary to clarify that raw verification/reset tokens are not durable River payloads.

Document:

```text
CSRF mechanism
challenge storage
session rotation
development mail capture
production-mail limitation
```

Do not rewrite unrelated docs.

---

## 30. Security Boundaries

At completion:

- exact Origin remains;
- authenticated unsafe requests also require session-bound CSRF;
- CSRF token is bound to one session;
- no production CSRF default;
- raw challenge tokens are never persisted to DB/Redis/River/logs;
- reset request is enumeration-resistant;
- verification/reset tokens are short-lived and single-use;
- replacement challenge invalidates prior token;
- password reset/change revoke old sessions;
- re-auth rotates session;
- raw session token remains cookie/process-memory only;
- security events contain no email/secrets;
- local mail sink is prohibited in production;
- no broad CORS;
- no JWT/browser token store;
- no OAuth/Admin scope slips in.

Deferred to P0-1D:

```text
Redis multi-dimensional rate limiting
Turnstile
compromised-password service
login-failure retention
trusted-proxy/source-IP hardening
production mail provider readiness
```

---

## 31. Implementation Order

1. Audit current `dev`, P0-1A commit/CI/code.
2. Run baseline `pnpm check`.
3. Add migration 00003 and grants.
4. Add sqlc queries and regenerate.
5. Implement CSRF config/HMAC/API/enforcement.
6. Implement challenge issue/consume/invalidate/cooldown.
7. Add local mail boundary.
8. Integrate registration verification.
9. Implement verification resend/consume.
10. Implement password reset.
11. Implement password change.
12. Implement re-authentication.
13. Implement session management.
14. Integrate Security Events into new/existing flows.
15. Update OpenAPI and regenerate Go/Orval.
16. Extend Public Web.
17. Run unit/DB/HTTP/security/privacy tests.
18. Run disposable integration.
19. Apply migration + smoke against real development Infra.
20. Keep CI/images green.
21. Update docs/changelog.
22. Regenerate and inspect complete diff.
23. Secret/scope audit.
24. Commit locally on `dev`.
25. Do not push.

---

## 32. Acceptance Criteria

### Database

```text
00003 migration                    PASS
auth_challenges                    PASS
security_events                    PASS
fresh 00001→00002→00003           PASS
gfp_dev forward migration          PASS
```

### CSRF

```text
GET /auth/csrf                     PASS
session-bound HMAC                 PASS
Origin + CSRF                      PASS
missing/wrong/cross-session reject PASS
rotation invalidates old token     PASS
production secret requirement      PASS
```

### Verification

```text
registration challenge             PASS
resend/cooldown                    PASS
raw token never DB persisted       PASS
valid consume sets verified_at     PASS
single-use                         PASS
generic invalid/expired failure    PASS
```

### Reset

```text
enumeration-resistant request      PASS
single-use                         PASS
Argon2id new password              PASS
all old sessions revoked           PASS
replacement session                PASS
old password rejected              PASS
no auto email verification         PASS
```

### Change / Re-auth

```text
current password required          PASS
CAS-safe update                    PASS
reset challenges invalidated       PASS
session replacement                PASS
authenticated_at refresh           PASS
old session/CSRF rejected          PASS
```

### Sessions

```text
list own active sessions           PASS
current marker                     PASS
revoke one/current/others          PASS
foreign-session privacy            PASS
```

### Security Events

Required events written with no secret/email payload.

### Mail

```text
local development capture          PASS
#token= links                      PASS
safe filenames                     PASS
no raw-token logs                  PASS
local mode rejected production     PASS
```

### Generated / Existing

```text
OpenAPI                             PASS
oapi-codegen                        PASS
Orval                               PASS
sqlc                                PASS
pnpm generate no drift              PASS
P0-1A endpoints remain green        PASS
health endpoints remain green       PASS
latest dev CI successful            PASS
```

Forbidden scope audit confirms no OAuth, provider linking, email change, Admin auth, roles, Turnstile, production mail SDK, JWT, NATS, MongoDB, pgvector, or Resource domain.

---

## 33. Stop Conditions

Stop and report evidence if:

1. current `dev` materially differs from completed P0-1A;
2. P0-1A CI is failing and prevents a trustworthy baseline;
3. `gfp_migrator` cannot apply 00003/grants;
4. `gfp_api` cannot use new objects after repository-owned grants;
5. easyhash token API no longer provides safe Generate/Hash/Verify semantics;
6. full CSRF would require exposing raw session token to JS;
7. mail implementation would require persisting raw challenge tokens in River/Redis/PostgreSQL;
8. real dev verification requires SSH/privilege widening;
9. a production email provider is required to pass development acceptance;
10. a secret would need to be committed;
11. P0-1C OAuth or P0-1D Admin/RBAC becomes necessary to complete this phase.

For ordinary implementation details, choose the simplest solution consistent with existing contracts.

---

## 34. Final Verification / Commit

Before completion:

1. run `pnpm check`;
2. run P0-1B integration/security tests;
3. run fresh migrations;
4. run `pnpm generate` again;
5. verify no drift;
6. run real-development smoke;
7. inspect `git status --short`;
8. inspect `git diff --stat`;
9. inspect complete diff;
10. ensure `.local/mail` is ignored/untracked;
11. ensure no raw auth/challenge/CSRF token is tracked;
12. ensure no real DSN/Tailnet/password is tracked;
13. ensure no future P0-1 scope slipped in;
14. update `CHANGELOG.md`.

Then commit locally on `dev`.

Recommended:

```text
feat: add session security and account recovery
```

Do not push, merge `main`, tag, or release.

---

## 35. Final Codex Report

Return the final report in Chinese.

Include:

### Implemented
Migration/entities, CSRF, challenge model, verification, recovery/change, session management, frontend flows.

### Security
Explain CSRF binding, token storage, session rotation, enumeration resistance. Never print raw tokens.

### Mail
Report local capture behavior and confirm no production provider was added. Do not include captured message contents.

### Database
Report migration and real-development result.

### Verification
List every command actually executed and pass/fail.

### Deviations
Any intentional deviation and reason.

### Remaining P0-1
Only:

```text
P0-1C — Google/GitHub OAuth + Account Linking
P0-1D — Admin Auth + Roles + Final Auth Hardening
```

or unresolved blockers.

### Git

```text
branch
final commit SHA
git status
```

Do not push.
