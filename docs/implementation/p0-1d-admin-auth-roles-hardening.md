# P0-1D — Admin Authentication, Roles & Final Auth Hardening

**Repository:** `deepfurry/gofurry-platform`\
**Target branch:** `dev`\
**Release snapshot branch:** `main`\
**Prerequisite:** P0-1C implementation complete\
**Status:** Codex implementation specification

## 1. Goal

P0-1D is the final Identity/Auth implementation phase.

It completes the application-level authentication foundation by adding:

```text
Static system roles
Separate Admin authentication
Separate Admin sessions
Admin CSRF / Origin protection
Admin session management
Admin re-authentication
Operator role bootstrap
Redis-backed auth throttling
Final cross-session / credential hardening
```

The architectural split must remain:

```text
Public Web / Public API
        │
        ├── Public Session
        └── User-facing auth

Admin Web / Admin API
        │
        ├── Admin Session
        ├── privileged role authorization
        └── future governance workflows
```

At completion:

- Public sessions are never accepted by Admin API.
- Admin sessions are never accepted by Public API.
- Only privileged users may establish an Admin session.
- Admin auth uses a separate cookie, lifetime, CSRF secret, Origin, and OpenAPI contract.
- Roles remain static; no configurable RBAC.
- Role mutation is an operator action, not an Admin product feature.
- Password reset/change revokes Admin sessions as well as Public sessions.
- Public/Admin auth has Redis-backed abuse controls.
- P0-1 automated implementation acceptance is green.

P0-1D completes **application Identity/Auth**, not production deployment.

---

## 2. Current P0-1C Baseline

Extend the actual current `dev`.

Completed phases:

```text
P0-1A  Identity + Email/Password + Public Session
P0-1B  CSRF + Verification + Recovery + Session Security
P0-1C  Google/GitHub OAuth + Explicit Account Linking
```

Current persistent entities:

```text
app.users
app.user_profiles
app.auth_identities
app.password_credentials
app.sessions
app.auth_challenges
app.security_events
```

Current session methods:

```text
password
password_reset
google
github
```

Current session kinds already support:

```text
public
admin
```

Current Public security includes:

```text
opaque server-side sessions
SHA-256 session-token lookup hash
session-bound HMAC CSRF
exact Origin validation
30d absolute / 14d idle
email verification/recovery
password change/reset
Google/GitHub OAuth
Redis one-time OAuth state
15m auth freshness
```

Current Admin API remains health-only.
Current Admin Web remains a foundation shell.

Do not edit migrations:

```text
00001_foundation.sql
00002_identity_local_auth.sql
00003_auth_security_recovery.sql
00004_oauth_identity.sql
```

Create a new migration only.

---

## 3. P0-1C Human OAuth Acceptance

Interactive real-provider OAuth is not a blocker for starting P0-1D.

Treat status as:

```text
P0-1C implementation          complete
Disposable provider E2E       complete
Real OAuth config capability  available
Google interactive E2E        pending human verification
GitHub interactive E2E        pending human verification
```

Retain those human gates for final P0-1 sign-off.

Never report interactive OAuth PASS unless it was actually executed.

---

## 4. Branch / Execution Contract

Work only on:

```text
dev
```

Before editing:

```bash
git branch --show-current
git status --short
git log -5 --oneline --decorate
```

Rules:

- preserve unrelated user work;
- do not rewrite applied migrations;
- do not modify/merge `main`;
- do not tag/release;
- do not push unless explicitly requested;
- after all applicable verification, commit locally on `dev`.

Recommended commit:

```text
feat: add admin authentication and auth hardening
```

---

## 5. Context Discipline

Read:

```text
AGENTS.md
this P0-1D specification
.agents/architecture.md
.agents/playbook.md
contracts/architecture.md
contracts/database.md
contracts/development.md
contracts/generated-code.md
docs/product/PRODUCT.md
docs/product/domain-model.md
docs/architecture/security.md
docs/architecture/backend.md
docs/architecture/data.md
docs/architecture/frontend.md
docs/engineering/tech-stack.md
```

Inspect current implementation, especially:

```text
server/internal/auth/*
server/internal/identity/*
server/internal/redisstore/*
server/internal/config/*
server/internal/transport/public/*
server/internal/transport/admin/*
server/db/migrations/*
server/db/queries/*
contracts/openapi/public.yaml
contracts/openapi/admin.yaml
apps/admin/src/*
apps/web/src/*
scripts/*
```

Do not preload unrelated Resource/Discovery/Exchange implementation docs.

---

## 6. Explicit Non-goals

Do not implement:

```text
dynamic RBAC builder
permission tables
custom roles
role editor UI
web role administration
resource moderation
contribution review
reports
Trust/Risk/Budget
business AuditLog
Cloudflare Access JWT verification
Cloudflare Access provisioning
application TOTP/WebAuthn
production email provider
production OAuth clients
Turnstile widget/provider
account merge
email change
password removal
general user-management console
NATS
MongoDB
pgvector
```

---

## 7. Production Boundary

The frozen production Admin model remains:

```text
Cloudflare Access
+ mandatory MFA
+ GoFurry Admin Login
+ separate Admin Session
+ GoFurry Admin Authorization
```

P0-1D implements the latter three.

Cloudflare Access remains a deployment/pre-production gate because production infrastructure is not being provisioned yet.

Do not treat any `CF-*` header as a role.
Cloudflare identity is not GoFurry authorization.

---

## 8. Static Role Model

Introduce exactly:

```text
moderator
editor
admin
```

A normal user has no `user` role row; the User account itself is the base identity.

Do not introduce DB-driven permission machinery.

---

## 9. Static Authorization Policy

Authorization is explicit Go code.

Capabilities:

```text
AdminAccess
Editorial
Moderation
Administration
```

Policy:

```text
moderator:
  AdminAccess
  Moderation

editor:
  AdminAccess
  Editorial

admin:
  AdminAccess
  Editorial
  Moderation
  Administration
```

`admin` is therefore the explicit super-role in code.

Do not persist the capability matrix.

P0-1D real Admin routes only require `AdminAccess`; future P0 phases may consume the other helpers.

---

## 10. Migration 00005

Create:

```text
server/db/migrations/00005_admin_auth_roles.sql
```

or the next sequential number if history changed.

Introduce:

```text
app.user_roles
```

and extend Security Event values.

No other business table is required.

---

## 11. `app.user_roles`

Conceptual schema:

```text
user_id     uuid NOT NULL
role        text NOT NULL
created_at  timestamptz NOT NULL

PRIMARY KEY (user_id, role)
```

FK:

```text
user_id → app.users(id) ON DELETE RESTRICT
```

CHECK:

```text
role IN ('moderator','editor','admin')
```

Do not add:

```text
permission JSON
scope JSON
expires_at
custom role
granted_by metadata
```

This table is current authorization state only.

---

## 12. Security Event Extension

Preserve all existing P0-1 events.

Add at minimum:

```text
login_failed

admin_login_succeeded
admin_login_failed
admin_logout
admin_reauthenticated
admin_session_revoked
admin_other_sessions_revoked

role_moderator_granted
role_moderator_revoked
role_editor_granted
role_editor_revoked
role_admin_granted
role_admin_revoked
```

Do not add arbitrary metadata to `security_events`.

Unknown-account login failure may remain unpersisted; do not create unbounded event volume.

Security Events never contain:

```text
email
IP
User-Agent
password/hash
session token/hash
CSRF
OAuth secret/token
rate-limit key
```

---

## 13. Database Privileges

### `gfp_admin`

Grant only runtime rights needed by Admin auth.

Expected shape:

```text
USAGE ON SCHEMA app

SELECT:
  users
  auth_identities
  password_credentials
  user_roles
  sessions

INSERT:
  sessions
  security_events

UPDATE:
  sessions.last_seen_at
  sessions.idle_expires_at
  sessions.revoked_at
```

If `SELECT FOR UPDATE` requires UPDATE privilege on `users`, grant only the minimal harmless column pattern, e.g.:

```text
UPDATE(updated_at)
```

Do not grant `gfp_admin` role mutation DML.

### Other roles

`gfp_readonly` may receive `SELECT user_roles`.

Do not grant `gfp_worker` Admin/role DML.

Do not widen `gfp_api` role privileges unless a concrete Public path needs them.

No superuser/ownership changes.

---

## 14. Role Assignment Is Operator-only

Add:

```text
server/cmd/adminctl
```

Use standard Go CLI/flag handling; do not add a CLI framework only for this.

Supported:

```text
grant-role
revoke-role
list-roles
```

Conceptual usage:

```text
go run ./cmd/adminctl grant-role -email <email> -role admin
go run ./cmd/adminctl revoke-role -email <email> -role editor
go run ./cmd/adminctl list-roles -email <email>
```

A Windows-friendly root helper may wrap `migrator.local`.

The command uses migrator/owner credentials, not `gfp_admin`.

---

## 15. Role Bootstrap Safety

For P0-1D privileged login, `grant-role` requires an existing:

```text
active, non-deleted User
local email identity
verified email
password credential
```

This ensures the target can actually use Admin password login.

`grant-role` is idempotent.

Refuse removing the final active:

```text
role=admin
```

assignment.

When role removal leaves no privileged role:

```text
revoke all active Admin sessions
```

in the same operator transaction.

Record role-specific Security Event.

---

## 16. Admin Authentication Method

Admin login uses:

```text
email + password
```

only.

Do not add Google/GitHub OAuth on the Admin origin in P0-1D.

An OAuth-only user cannot establish an Admin session in this phase.

Do not create a fake password credential.

---

## 17. Admin Login Eligibility

Admin login succeeds only if:

```text
email identity exists
email verified
password credential exists
password verifies
User active
User not deleted
at least one privileged role
```

Unknown email, wrong password, unverified email, no role, OAuth-only, disabled, and deleted must all use:

```text
ADMIN_INVALID_CREDENTIALS
```

Do not reveal the failing predicate.

Reuse the existing Argon2id and dummy-verification policy.

---

## 18. Admin Session Model

Reuse:

```text
app.sessions
```

with:

```text
kind = admin
auth_method = password
```

Do not create another session table.

Admin lifetime:

```text
absolute = 8 hours
idle     = 1 hour
touch    ≈ 5 minutes
```

Admin resolution requires:

```text
kind=admin
active/non-expired/non-revoked session
active/non-deleted User
at least one current privileged role
```

If all privileged roles disappear, the session immediately becomes invalid.

---

## 19. Admin Cookie Contract

Production:

```text
__Host-gofurry_admin_session
Secure
HttpOnly
SameSite=Strict
Path=/
No Domain
```

Development/test HTTP:

```text
gofurry_admin_session
```

Never silently downgrade production semantics.

Do not accept/use the Public cookie name.

---

## 20. Public/Admin Session Isolation

Hard invariant:

```text
Public API accepts kind=public only
Admin API accepts kind=admin only
```

Explicitly test:

```text
Public cookie → Admin API = 401
Admin cookie  → Public API = 401
```

Do not create Public→Admin session exchange.

Admin login always requires password entry.

---

## 21. Admin Origin

Add:

```text
ADMIN_ORIGIN
```

Development default:

```text
http://localhost:5173
```

Production:

```text
https://admin.gofurry.com
```

Use the same exact-origin validation constraints as `PUBLIC_ORIGIN`.

Do not open broad CORS.

Preserve Admin Vite `/api` proxy to `127.0.0.1:8081`.

---

## 22. Admin CSRF

Add separate:

```text
ADMIN_CSRF_SECRET
```

Do not reuse Public `CSRF_SECRET`.

Production requires explicit private >=32-byte value.

Development/test may use a clearly development-only default.

Derive:

```text
Base64URL(HMAC-SHA256(ADMIN_CSRF_SECRET, raw_admin_session_token))
```

Use constant-time comparison.

Do not persist CSRF.

Add:

```text
GET /auth/csrf
```

for valid Admin session.

Every authenticated unsafe Admin request requires:

```text
exact ADMIN_ORIGIN
+
X-CSRF-Token
```

Login requires Origin only.

---

## 23. Admin Actor

Conceptually:

```text
UserID
SessionID
AuthenticatedAt
Roles
```

Do not include password/provider/session internals.

Authorization policy remains outside transport DTOs.

---

## 24. Admin Auth Application Boundary

Prefer extending shared auth/session primitives rather than duplicating password security.

Required capabilities conceptually:

```text
AdminLogin
ResolveAdmin
AdminLogout
AdminReauthenticate
AdminSessions
RevokeAdminSession
RevokeOtherAdminSessions
```

A separate `adminauth` package is acceptable only if it reuses shared password/session primitives and avoids duplication/cycles.

---

## 25. Admin Login Transaction

Flow:

1. normalize email;
2. throttle pre-check;
3. lookup candidate;
4. run dummy/generic behavior for unknown/ineligible accounts;
5. verify password outside transaction;
6. begin transaction;
7. lock common User auth state;
8. re-read credential;
9. require verified email;
10. require privileged role;
11. require active/non-deleted User;
12. CAS-upgrade password hash if needed;
13. create `kind=admin` session with 8h/1h policy;
14. record `admin_login_succeeded`;
15. commit;
16. clear subject failure throttle;
17. set Admin cookie.

Known-account wrong password may record bounded `admin_login_failed`.

Do not hold DB transaction during Argon2id.

---

## 26. Admin Re-authentication

Add:

```text
POST /auth/reauthenticate
```

Requires Admin session + Origin + Admin CSRF + password.

On success:

```text
verify current password
lock User auth state
revalidate role/session
revoke current Admin session
create replacement Admin session
authenticated_at = now
record admin_reauthenticated
commit
set replacement cookie
```

This is the future high-impact Admin step-up primitive.

---

## 27. Admin Session Management

Add:

```text
GET    /me/sessions
DELETE /me/sessions/{session_id}
POST   /me/sessions/revoke-others
```

List only own active:

```text
kind=admin
```

sessions.

Return:

```text
id
authenticated_at
created_at
last_seen_at
idle_expires_at
absolute_expires_at
current
```

Do not add IP/device fingerprints.

Revoke one:

- target must be own Admin session;
- foreign/nonexistent/Public session IDs share safe not-found behavior;
- current revoke clears Admin cookie;
- record `admin_session_revoked`.

Revoke others:

- preserve current Admin session;
- affect Admin sessions only;
- record `admin_other_sessions_revoked`.

---

## 28. Password Reset/Change Must Revoke Admin Sessions

P0-1B predates real Admin sessions.

P0-1D must change successful:

```text
password reset
password change
```

to revoke:

```text
all Public sessions
+
all Admin sessions
```

before creating one replacement **Public** session.

Do not automatically recreate an Admin session.

Privileged users must log in to Admin again.

---

## 29. Other Session Separation Rules

Existing Public logout/revoke/revoke-others remain:

```text
kind=public only
```

Admin logout/revoke/revoke-others remain:

```text
kind=admin only
```

OAuth provider unlink revokes matching Public provider sessions only; P0-1D Admin sessions are password-authenticated.

Password reset/change is the cross-kind revocation exception.

---

## 30. Admin OpenAPI

Expand:

```text
contracts/openapi/admin.yaml
```

Add:

```text
POST /auth/login
POST /auth/logout
POST /auth/reauthenticate
GET  /auth/csrf

GET  /me

GET    /me/sessions
DELETE /me/sessions/{session_id}
POST   /me/sessions/revoke-others

GET /health/live
GET /health/ready
```

Do not add business moderation/resource endpoints.

---

## 31. Admin OpenAPI Errors

Required equivalents:

```text
VALIDATION_ERROR
ADMIN_INVALID_CREDENTIALS
ADMIN_UNAUTHENTICATED
ADMIN_FORBIDDEN
ADMIN_SESSION_NOT_FOUND
ORIGIN_FORBIDDEN
CSRF_INVALID
AUTH_RATE_LIMITED
INTERNAL_ERROR
```

Do not expose SQL/easyhash/Redis errors.

Generated DTOs remain transport-only.

---

## 32. Admin `/me`

Return only:

```text
id
email
roles
authenticated_at
```

Roles enum:

```text
moderator
editor
admin
```

Do not expose public/private profile internals, password state, provider subjects, or Public sessions.

---

## 33. Admin Web

Replace the current foundation-only shell with a minimal authenticated SPA.

Use existing:

```text
React 19
Vite
TanStack Router
TanStack Query
generated Admin API client
Tailwind layout
SCSS
```

Routes:

```text
/login
/
```

Do not build Review Queue/Moderation/User Management yet.

---

## 34. Admin Login UI

Form:

```text
email
password
```

Session arrives only in HttpOnly Admin cookie.

No token store.

Protected root loads:

```text
GET /api/me
```

401 redirects to `/login`.

Display:

```text
GoFurry Admin
roles
Admin session controls
reauth
logout
```

No fake dashboard metrics.

---

## 35. Admin Frontend CSRF

Mutation helper:

1. fetch `/api/auth/csrf`;
2. hold token only in runtime memory;
3. send `X-CSRF-Token`;
4. discard after Admin session rotation;
5. refetch on demand.

Never persist Admin session/CSRF/password in browser storage.

---

## 36. Redis-backed Auth Throttling

Add a narrow auth throttle boundary using existing Redis `gfp:*`.

Do not build a generic rate-limit framework.

Needed operations conceptually:

```text
check
record attempt/failure
clear subject
TTL
```

Use atomic Redis behavior where practical.

No `KEYS`/`SCAN`.

Throttle state is ephemeral and not PostgreSQL truth.

---

## 37. Rate-limit Key Privacy

Add:

```text
AUTH_THROTTLE_SECRET
```

Use keyed HMAC over normalized identity dimensions.

Redis key prefix:

```text
gfp:auth:limit:
```

Do not place raw email, password, OAuth subject, session token, or IP in keys/values.

Development/test may use a dev-only default.

Production requires explicit private >=32-byte secret.

---

## 38. Throttle Dimensions

Do not rely on IP alone.

P0-1D must use at least:

```text
normalized account/email fingerprint
+
global operation bucket
```

This intentionally avoids depending on production proxy forwarding.

Do not trust arbitrary `X-Forwarded-For` or `X-Real-IP`.

---

## 39. Initial Throttle Policies

Keep as explicit code constants.

Recommended:

```text
Public login failures:
  subject 10 / 15m
  global  500 / 15m

Admin login failures:
  subject 5 / 15m
  global  100 / 15m

Registration attempts:
  subject 3 / 1h
  global  200 / 1h

Password reset requests:
  subject 3 / 1h
  global  200 / 1h

Password/Admin reauth failures:
  user 5 / 15m
```

Small evidence-based adjustments are allowed if documented.

---

## 40. Throttle Behavior

### Public login

- pre-check failure buckets;
- invalid credentials record subject/global failure;
- success clears subject failure bucket;
- limited requests return `AUTH_RATE_LIMITED`.

### Admin login

Same pattern with stricter limits.

If throttle Redis is unavailable:

```text
Admin login fails closed
```

### Registration

Count attempts before KDF/insert.

Limited:

```text
429 AUTH_RATE_LIMITED
```

### Password reset

Preserve enumeration resistance.

If limited:

```text
return same 202/body
do not issue/send challenge
```

### Public throttle Redis failure

Public canonical auth must not depend on Redis.

Prefer documented fail-open behavior for Public local login/register/reset with safe warning logs.

OAuth remains separately Redis-dependent as already implemented.

---

## 41. Login Failure Security Events

Now add bounded known-account failures:

```text
login_failed
admin_login_failed
```

Unknown-account failures need not be persisted.

Never store submitted email in events.

---

## 42. Password / Compromised Password Scope

Do not pretend a tiny local denylist is a comprehensive breach service.

Preserve:

```text
15–128 Unicode code points
password-manager friendly
Argon2id
no composition rules
```

A network-backed compromised-password service is a pre-production enhancement, not a P0-1D blocker.

Do not add an external breach API without a separate decision.

---

## 43. Turnstile Scope

Do not add fake Turnstile code in P0-1D.

Security architecture still recommends selective Turnstile for registration/reset/repeated failures, but actual site/secret configuration belongs to pre-production/deployment hardening.

P0-1D supplies Redis abuse controls first.

---

## 44. Proxy Trust Scope

Do not trust arbitrary forwarded IP headers.

Production rule remains:

```text
trust forwarding/IP headers only through controlled Cloudflare → Nginx
```

P0-1D throttling must not require forwarded-IP correctness.

---

## 45. Admin / Credential Concurrency

Admin login must use the common User auth lock introduced in P0-1C.

Required races:

```text
Admin login vs password reset
Admin login vs password change
Admin login vs role revoke
Admin reauth vs password reset/change
```

Required results:

```text
no Admin session escapes reset/change
no removed role remains usable
old Admin session invalid after reauth
```

---

## 46. Role Revocation Concurrency

Operator role revoke transaction:

1. lock User auth state;
2. enforce last-admin rule where applicable;
3. delete role;
4. query remaining privileged roles;
5. if none, revoke all Admin sessions;
6. record role event;
7. commit.

Admin session resolution also checks current roles.

---

## 47. Regression: Password Reset/Change

Update tests:

```text
password reset:
  revoke all Public sessions
  revoke all Admin sessions
  create one Public replacement

password change:
  revoke all Public sessions
  revoke all Admin sessions
  create one Public replacement
```

Existing Public session-management tests must continue proving Admin sessions are untouched by normal Public session revocation.

---

## 48. Admin sqlc

Add focused queries equivalent to:

```text
FindAdminCredentialByEmail
ListUserRoles
HasPrivilegedRole
CreateAdminSession
FindActiveAdminSessionByTokenHash
TouchAdminSession
ListActiveAdminSessions
RevokeAdminSession
RevokeOtherAdminSessions
RevokeAllAdminSessions
```

Reuse/generalize current session queries where clearer.

No Repository wrappers around sqlc.

---

## 49. Operator Role Queries

Need capabilities equivalent to:

```text
FindUserByEmailForRoleOperation
ListUserRoles
GrantUserRole
RevokeUserRole
CountActiveAdmins
CountPrivilegedRoles
RevokeAllAdminSessions
```

Operator transactions are explicit.

No role mutation HTTP endpoints.

---

## 50. Admin Transport

`server/internal/transport/admin` owns:

```text
Admin cookie
Admin Origin guard
Admin CSRF guard
Admin actor resolution
generated Admin DTO mapping
safe error mapping
```

Do not import Public handlers.

Application/security policy remains outside generated types.

---

## 51. Public Transport Hardening

Change Public transport only for P0-1D requirements:

```text
rate-limit error mapping
known login failure events
password reset/change cross-kind revocation
```

Do not redesign Public API/OAuth.

---

## 52. Configuration Contract

Extend Admin:

```text
ADMIN_ORIGIN
ADMIN_CSRF_SECRET
AUTH_THROTTLE_SECRET
```

Extend Public API:

```text
AUTH_THROTTLE_SECRET
```

Development defaults:

```text
ADMIN_ORIGIN=http://localhost:5173
ADMIN_CSRF_SECRET=<dev-only default allowed>
AUTH_THROTTLE_SECRET=<dev-only default allowed>
```

Production requires explicit:

```text
ADMIN_ORIGIN=https://admin.gofurry.com
ADMIN_CSRF_SECRET >=32 bytes
AUTH_THROTTLE_SECRET >=32 bytes
```

Update:

```text
server/env/api.example
server/env/admin.example
docs/development.md
```

Do not expose values in errors.

---

## 53. Generated Admin Client

Admin OpenAPI remains authoritative:

```text
Admin OpenAPI
→ oapi-codegen
→ Orval Admin client
```

Generated files are committed and never manually edited.

---

## 54. Static Policy Tests

Verify:

```text
no role:
  AdminAccess false

moderator:
  AdminAccess true
  Moderation true
  Editorial false
  Administration false

editor:
  AdminAccess true
  Editorial true
  Moderation false
  Administration false

admin:
  all true
```

Multiple roles are additive.

---

## 55. Admin Auth Integration Tests

Disposable PostgreSQL/Redis:

```text
verified password + moderator → PASS
verified password + editor    → PASS
verified password + admin     → PASS

no role        → generic failure
unverified     → generic failure
OAuth-only     → generic failure
wrong password → generic failure
unknown email  → generic failure
disabled       → generic failure
deleted        → generic failure
```

Verify easyhash upgrade remains safe.

---

## 56. Session Isolation Tests

Prove:

```text
Public cookie on Admin /me → 401
Admin cookie on Public /me → 401

Admin logout:
  Admin revoked
  Public unaffected

Public logout:
  Public revoked
  Admin unaffected

Admin revoke-others:
  Admin only

Public revoke-others:
  Public only
```

---

## 57. Admin CSRF / Origin Tests

Test:

```text
Admin CSRF requires Admin session
same-session token works
Public CSRF fails
other Admin session CSRF fails
missing/wrong token fails
wrong ADMIN_ORIGIN fails
Public Origin fails
GET does not require CSRF
old CSRF fails after Admin reauth
production cookie/secret semantics
```

---

## 58. Role Bootstrap Tests

Test:

```text
grant moderator/editor/admin
idempotent grant
revoke role
last active Admin revoke rejected
remove one role while another privileged remains → Admin session may remain
remove final privileged role → Admin sessions revoked
role Security Event written
```

Use disposable fixtures only.

---

## 59. Throttle Tests

Test:

```text
raw email absent from Redis keys
subject limit
global limit
TTL expiry
clear on successful login
Public login limit
Admin stricter limit
reset still same 202 when limited
Public Redis outage fail-open
Admin Redis outage fail-closed
```

Avoid sleep-heavy tests.

---

## 60. Security Event Tests

Verify:

```text
login_failed
admin_login_succeeded
admin_login_failed
admin_logout
admin_reauthenticated
admin_session_revoked
admin_other_sessions_revoked
role grant/revoke events
```

No private/secret payloads.

---

## 61. Concurrency Tests

At minimum:

```text
Admin login vs password reset
Admin login vs password change
Admin login vs role revoke
Admin reauth vs password reset
role revoke vs Admin request
```

Keep P0-1C OAuth concurrency green.

---

## 62. Real Development Admin Smoke

Add:

```text
pnpm smoke:admin:dev
```

Use prepared ignored local env + Tailscale, no SSH.

Suggested smoke:

1. verify `gfp_admin` and `gfp_migrator` expected identities;
2. create a narrow temporary password account through normal application/runtime paths;
3. prepare verified email for the fixture;
4. grant temporary privileged role through operator/owner path;
5. Admin login via `gfp_admin`;
6. Admin `/me`;
7. Admin CSRF;
8. Admin reauth/session rotation;
9. Admin session list/revoke;
10. Public/Admin cookie isolation;
11. revoke role;
12. confirm Admin access immediately fails;
13. clean only the temporary fixture.

Never print real DSNs/passwords/session/CSRF.

Do not widen shared Infra privileges.

---

## 63. P0-1 Final Human Sign-off

After P0-1D, retain:

```text
pnpm smoke:oauth:dev

Google:
  real login → callback → /account → auth method visible

GitHub:
  real login → callback → /account → auth method visible

Password account:
  link Google
  provider reauth
  safe unlink

Password account:
  link GitHub
  provider reauth
  safe unlink

Admin browser:
  grant role
  Admin login
  /me
  CSRF
  logout
```

These are final human acceptance gates, not Codex implementation blockers.

---

## 64. CI

Extend existing disposable CI:

```text
fresh migrations 00001→00005
Redis throttle integration
Public auth regression
OAuth fake-provider regression
Admin auth integration
role/operator integration
Public/Admin session isolation
Admin CSRF
concurrency
frontend typecheck/build
Docker builds
secret audit
generated drift
```

Never use:

```text
real OAuth secrets
shared development Infra
real Admin users
Cloudflare Access
```

in CI.

---

## 65. Documentation Updates

Update:

```text
CHANGELOG.md
docs/development.md
docs/architecture/security.md
docs/engineering/tech-stack.md
contracts/architecture.md
contracts/database.md
contracts/development.md
```

Document:

```text
static roles
operator role bootstrap
Admin session separation
Admin cookie/CSRF
8h/1h policy
cross-kind password revocation
auth throttling
remaining production gates
P0-1 human OAuth sign-off
```

Do not rewrite unrelated docs.

---

## 66. Remaining Production Gates

P0-1D completion does not mean production deployment readiness.

Still required before production:

```text
Cloudflare Access configuration
mandatory Access MFA
production mail provider
production OAuth clients/callbacks
real Google/GitHub browser acceptance
production Admin/Throttle secrets
trusted Cloudflare → Nginx proxy/IP configuration
selective Turnstile decision/integration
deployment secret management
```

These are deployment/pre-production tasks, not reasons to keep Identity/Auth implementation open.

---

## 67. Implementation Order

1. Audit current `dev` and P0-1C CI.
2. Run baseline `pnpm check`.
3. Add migration `00005_admin_auth_roles.sql`.
4. Add role model/static policy.
5. Add operator role command.
6. Generalize session primitives for Public/Admin policy.
7. Implement Admin session resolver/creation/revocation.
8. Make password reset/change revoke all session kinds.
9. Add Admin Origin/CSRF/config.
10. Expand Admin OpenAPI and regenerate.
11. Implement Admin transport.
12. Implement Admin login/logout/reauth.
13. Implement Admin session management.
14. Add Redis auth throttle.
15. Integrate Public login/register/reset throttling.
16. Integrate Admin login/reauth throttling.
17. Add failure/Admin/role Security Events.
18. Replace Admin foundation UI with minimal authenticated shell.
19. Add role/session/CSRF/throttle/concurrency tests.
20. Run disposable integration.
21. Apply migration + real Admin dev smoke.
22. Keep all P0-1A/B/C regressions green.
23. Update docs/changelog/contracts.
24. Re-run generation and drift.
25. Inspect full diff.
26. Secret/scope audit.
27. Commit locally on `dev`.
28. Do not push.

---

## 68. Acceptance Criteria

### Database / roles

```text
00005 applies                               PASS
fresh 00001→00005                          PASS
user_roles                                 PASS
role CHECK                                 PASS
no dynamic permission tables               PASS
least-privilege gfp_admin grants            PASS
```

### Static policy

```text
moderator                                  PASS
editor                                     PASS
admin                                      PASS
multi-role additive                        PASS
```

### Operator

```text
grant role                                 PASS
idempotent grant                           PASS
revoke role                                PASS
last active Admin protected                PASS
final privileged removal revokes Admin     PASS
```

### Admin login/session

```text
verified password + role                   PASS
generic ineligible failure                 PASS
kind=admin                                 PASS
8h absolute                                PASS
1h idle                                    PASS
touch throttled                            PASS
separate Strict cookie                     PASS
production Secure/__Host-                  PASS
```

### Isolation

```text
Public → Admin rejected                    PASS
Admin → Public rejected                    PASS
Public revocation ignores Admin            PASS
Admin revocation ignores Public            PASS
```

### Admin CSRF

```text
separate ADMIN_CSRF_SECRET                 PASS
session-bound HMAC                         PASS
exact ADMIN_ORIGIN                         PASS
cross-session/Public token rejected        PASS
rotation invalidates old CSRF              PASS
```

### Credential compromise

```text
password reset revokes Public+Admin        PASS
password change revokes Public+Admin       PASS
replacement is Public only                 PASS
```

### Admin reauth/session management

```text
reauth rotation                            PASS
session list                               PASS
revoke one/current/others                  PASS
foreign/Public session privacy             PASS
```

### Throttling

```text
Redis ephemeral                            PASS
no raw email keys                          PASS
subject + global dimensions                PASS
Public login limit                         PASS
Admin stricter limit                       PASS
reset enumeration resistance               PASS
Public fail-open                           PASS
Admin fail-closed                          PASS
```

### Existing P0-1

All existing local auth, recovery, Public session security, OAuth fake-provider E2E, linking/unlinking, provider reauth, profiles and health remain green.

### CI / Dev

```text
latest disposable CI                       PASS
pnpm smoke:admin:dev                       PASS
```

Interactive Google/GitHub browser acceptance may remain explicitly pending human sign-off.

---

## 69. Stop Conditions

Stop and report if:

1. current `dev` materially differs from completed P0-1C;
2. P0-1C CI is failing for unrelated reasons;
3. `gfp_admin` cannot receive required runtime grants through migration;
4. Admin separation requires a new session table;
5. Admin login would need a Public session;
6. role implementation would require dynamic permission tables;
7. reset/change cannot atomically revoke Admin sessions;
8. Redis throttle requires raw identity/secret keys;
9. Admin security requires trusting arbitrary forwarded IP headers;
10. real dev verification requires SSH/Infra privilege widening;
11. a secret would need to be committed;
12. Cloudflare Access provisioning is required just to test P0-1D.

---

## 70. Final Verification / Commit

Before completion:

```text
pnpm check
Admin auth/role/throttle tests
all existing P0-1 integration tests
fresh migrations
pnpm generate
generated drift check
pnpm smoke:admin:dev
safe existing dev smokes as applicable
full diff review
secret audit
scope audit
```

Update `CHANGELOG.md`.

Then commit locally on `dev`:

```text
feat: add admin authentication and auth hardening
```

Do not push, merge `main`, tag, or release.

---

## 71. Final Codex Report

Return the report in Chinese.

Include:

### Implemented
Role table/static policy, operator bootstrap, Admin login/session/CSRF, Admin reauth/session management, auth throttling, cross-kind credential revocation, Admin Web.

### Security Boundaries
Explain Public/Admin isolation, Admin roles, Admin CSRF, role removal behavior, Redis throttle privacy.

Never print secrets/tokens.

### Database
Report migration `00005` and `gfp_admin` privilege changes.

### Verification
List every command actually run.

Separate:

```text
disposable CI/integration
real dev Admin smoke
interactive Google OAuth
interactive GitHub OAuth
```

Interactive OAuth may honestly remain:

```text
NOT RUN — deferred human acceptance
```

### Remaining Production Gates
Only genuine deployment/pre-production work.

### P0-1 Status

If all automated/application gates pass, report:

```text
P0-1 implementation complete
Human/provider deployment sign-off pending
```

### Git

```text
branch
final commit SHA
git status
```

Do not push.
