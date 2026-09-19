# P0-1A — Identity Core + Email/Password + Public Session

**Repository:** `deepfurry/tap4furry`\
**Target branch:** `dev`  
**Release snapshot branch:** `main`  
**Phase:** P0-1A  
**Status:** Codex implementation specification

---

## 1. Goal

P0-1A is the first real product-domain phase after P0-0.

Its goal is to establish the minimum durable identity and local-authentication system required for later Tap4Furry product work:

```text
Auth Identity
    ↓
User Account
    ↓
Public Profile
```

and:

```text
Email + Password
        ↓
Public Session
        ↓
Authenticated /me
```

When P0-1A is complete, a user must be able to:

- register with email + password;
- receive an authenticated public session;
- log in with email + password;
- log out;
- retrieve the current authenticated account/profile;
- edit the basic public profile;
- resolve a public profile by handle;
- keep all private authentication data out of public profile responses.

This phase must preserve all P0-0 architecture and engineering contracts.

P0-1A is **not production-auth complete**. Email verification, password reset, OAuth, full session-management UI, synchronizer CSRF, rate-limit hardening, security-event logging, Admin auth, and roles remain later P0-1 phases.

---

## 2. Branch / Execution Contract

Work only on:

```text
dev
```

`main` remains the stable release snapshot branch.

Before editing:

```bash
git branch --show-current
git status --short
git log -5 --oneline --decorate
```

Rules:

- do not modify or merge `main`;
- do not tag or release;
- do not push unless explicitly instructed;
- preserve unrelated user work;
- do not rewrite P0-0 released/shared-environment-applied migration history;
- create new migrations only;
- after all verification passes, commit locally on `dev`.

Recommended final commit:

```text
feat: add identity and local authentication foundation
```

---

## 3. Context Discipline

Do not preload the entire documentation tree.

### Required reading

Read in this order:

1. `AGENTS.md`
2. active P0-1A implementation specification
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

Also inspect the current implementation before making assumptions.

### easyhash required reading

Inspect the current API of:

```text
https://github.com/gofurry/easyhash
```

At minimum:

```text
README.md
docs/usage.md
policy.go
token.go
```

P0-1A must use the actual current API rather than guessing from prior documentation.

### Do not preload

Do not read these merely for completeness:

```text
docs/product/discovery.md
docs/product/content-policy.md
docs/product/trust-safety.md
docs/product/roadmap.md
docs/engineering/jobs-automation.md
docs/engineering/backup-restore.md
```

Consult them only if the implementation genuinely crosses into those areas.

---

## 4. Existing P0-0 Baseline

P0-1A must extend the existing foundation rather than rebuild it.

Current important facts include:

```text
Go 1.27+
Fiber v3
OpenAPI spec-first
oapi-codegen
Orval
pgx/v5 + pgxpool
sqlc
goose
go-redis/v9
River
slog
```

Current transport boundary:

```text
server/internal/transport/public
server/internal/transport/admin
```

Current database source of truth:

```text
server/db/migrations/
server/db/queries/
```

Current application schemas:

```text
app
river
```

Current P0-0 application migration:

```text
00001_foundation.sql
```

Do not edit `00001_foundation.sql`.

Create a new P0-1A migration.

Public/Admin OpenAPI contracts remain separate.

P0-1A expands only the **Public API** contract. Admin API remains health-only.

---

## 5. Scope

P0-1A includes:

```text
User Account
Public Profile
Email Auth Identity
Password Credential
Public Session
Register
Login
Logout
Current User
Profile Update
Public Profile Read
```

It also introduces:

- the first real Application/Domain packages;
- local password policy;
- password hashing through `gofurry/easyhash`;
- public-session cookie handling;
- authenticated-request resolution;
- a minimal same-origin unsafe-request guard;
- minimal Public Web register/login/account/profile UI.

---

## 6. Explicit Non-goals

Do **not** implement in P0-1A:

```text
email verification flow
password reset
email change
auth_challenges
Google OAuth/OIDC
GitHub OAuth
provider link/unlink
active-session management UI
revoke other sessions
password change
session list
full synchronizer CSRF token system
Turnstile
login/register rate limiting
security_events
Admin authentication
user_roles
Moderator / Editor / Admin authorization
Trust / Risk / Budget / Distribution
avatar upload
Resource relationships
verified creator relationships
follow/notifications
MongoDB
NATS
pgvector
```

Do not add placeholder tables/packages for these future features.

Do not create `auth_challenges`, `security_events`, or `user_roles` early.

---

## 7. Domain Boundaries

P0-1A may introduce:

```text
server/internal/auth/
server/internal/identity/
```

### `identity`

Owns account/profile concepts and profile rules.

Examples:

```text
User
Profile
Handle validation
Public profile view
Update profile use case
```

### `auth`

Owns:

```text
Email identity normalization
Password hashing/verifying
Register
Login
Public session creation/resolution/revocation
Authentication middleware/context
```

Do not create ceremonial Service/Repository/DAO layers.

Prefer concrete application types that directly use generated sqlc queries and explicit pgx transactions.

Transport must depend on Application/Domain code.

Application/Domain must not import:

```text
Fiber
OpenAPI generated DTOs
Redis
River
```

unless a real boundary explicitly requires an infrastructure adapter.

P0-1A local authentication does not require River.

---

## 8. Database Migration

Create:

```text
server/db/migrations/00002_identity_local_auth.sql
```

or the next migration number consistent with repository conventions.

Do not edit migration 1.

The migration owns only P0-1A schema.

### 8.1 `app.users`

Conceptual schema:

```text
id              uuid PRIMARY KEY
account_state   text NOT NULL DEFAULT 'active'
created_at      timestamptz NOT NULL
updated_at      timestamptz NOT NULL
deleted_at      timestamptz NULL
```

Constraints:

```text
account_state IN ('active', 'disabled')
```

Notes:

- UUID is generated in Go, not by PostgreSQL.
- `disabled` is an account-security state, not moderation/restriction.
- `deleted_at` is separate from account state.
- P0-1A does not implement account deletion yet.
- no email/password/profile fields belong directly in `users`.

### 8.2 `app.user_profiles`

Conceptual schema:

```text
user_id                 uuid PRIMARY KEY
handle                  text NULL
display_name            text NULL
bio                     text NULL
search_engine_indexing  boolean NOT NULL DEFAULT false
created_at              timestamptz NOT NULL
updated_at              timestamptz NOT NULL
```

FK:

```text
user_id → app.users(id)
```

Use explicit restrictive/no-action FK behavior. Do not use business cascades.

Handle rules:

```text
nullable
lowercase ASCII only
3..32 characters
allowed: a-z 0-9 _ -
```

Recommended DB check:

```text
handle IS NULL OR handle ~ '^[a-z0-9][a-z0-9_-]{2,31}$'
```

Also ensure stored handles are lowercase.

Unique:

```text
UNIQUE(handle)
```

Do not make deleted-account handles automatically reusable.

Display name:

```text
nullable
Unicode allowed
max 80 characters
```

Bio:

```text
nullable
Unicode allowed
max 500 characters
```

`search_engine_indexing` defaults to `false`.

Do not add avatar/media columns yet.

### 8.3 `app.auth_identities`

Conceptual schema:

```text
id                 uuid PRIMARY KEY
user_id            uuid NOT NULL
provider           text NOT NULL
provider_subject   text NOT NULL
email              text NULL
verified_at        timestamptz NULL
created_at         timestamptz NOT NULL
updated_at         timestamptz NOT NULL
```

FK:

```text
user_id → app.users(id)
```

Provider constraint may include:

```text
email
google
github
```

P0-1A only creates/uses `email` identities.

Critical uniqueness:

```text
UNIQUE(provider, provider_subject)
```

Do **not** create a global unique constraint on `email` across OAuth identities.

For the local email identity:

```text
provider = 'email'
provider_subject = normalized email
email = normalized/presentation email
verified_at = NULL
```

Email verification arrives in P0-1B.

### 8.4 `app.password_credentials`

Conceptual schema:

```text
user_id              uuid PRIMARY KEY
password_hash        text NOT NULL
password_updated_at  timestamptz NOT NULL
created_at           timestamptz NOT NULL
updated_at           timestamptz NOT NULL
```

FK:

```text
user_id → app.users(id)
```

Do not store:

```text
plaintext password
salt column
algorithm column
Argon2 parameter columns
```

`easyhash` output is self-describing.

### 8.5 `app.sessions`

Conceptual schema:

```text
id                   uuid PRIMARY KEY
user_id              uuid NOT NULL
kind                 text NOT NULL
auth_method          text NOT NULL
token_hash           bytea NOT NULL
authenticated_at     timestamptz NOT NULL
created_at           timestamptz NOT NULL
last_seen_at         timestamptz NOT NULL
idle_expires_at      timestamptz NOT NULL
absolute_expires_at  timestamptz NOT NULL
revoked_at           timestamptz NULL
```

FK:

```text
user_id → app.users(id)
```

Constraints:

```text
kind IN ('public', 'admin')
octet_length(token_hash) = 32
```

P0-1A only creates:

```text
kind = 'public'
auth_method = 'password'
```

Unique:

```text
UNIQUE(token_hash)
```

Indexes should support:

```text
token lookup
active sessions by user
expiry cleanup later
```

Do not add IP/User-Agent/security metadata in P0-1A.

### 8.6 Privileges

The migration must explicitly grant only P0-1A-required rights.

`gfp_api` needs the minimum DML needed for the P0-1A tables.

P0-1A should not grant identity DML to:

```text
gfp_worker
gfp_admin
```

unless current code proves an actual requirement.

`gfp_readonly` may receive SELECT according to the database contract.

Do not grant ownership/DDL to runtime roles.

Do not require PostgreSQL superuser.

CI disposable database setup must create the required logical roles before applying migrations if explicit grants require them.

---

## 9. UUID Contract

Business IDs must use the selected Go 1.27+ standard-library UUIDv7 capability.

Use UUIDv7 for:

```text
users.id
auth_identities.id
sessions.id
```

Do not add a third-party UUID library for application IDs.

If an indirect dependency already includes `github.com/google/uuid`, do not use it for Tap4Furry business ID generation.

Inspect the actual installed Go 1.27 standard-library UUID API and use the real supported interface rather than guessing an import path/function name.

UUIDs are generated before persistence in Go.

PostgreSQL must not generate these IDs through defaults, triggers, `uuid-ossp`, or `pgcrypto`.

---

## 10. Email Normalization

Local email identity lookup must be deterministic.

Normalization rules:

1. trim surrounding whitespace;
2. validate that the input is a plain email address, not a display-name address;
3. lowercase the canonical lookup value;
4. do not remove dots;
5. do not strip `+tag`;
6. do not implement provider-specific Gmail/Outlook normalization.

The normalized value becomes:

```text
auth_identities.provider_subject
```

for provider `email`.

Do not expose email through public-profile APIs.

---

## 11. Password Policy

P0-1A password policy:

```text
minimum: 15 Unicode code points
maximum: 128 Unicode code points
```

Rules:

- allow spaces;
- allow Unicode;
- allow paste/password managers;
- no mandatory uppercase/lowercase/digit/symbol composition;
- do not trim passwords;
- do not Unicode-normalize passwords;
- reject inputs beyond the maximum before invoking an expensive KDF.

Common/compromised-password screening is deferred to later hardening.

---

## 12. easyhash Integration

Add:

```text
github.com/gofurry/easyhash
```

as a real P0-1A dependency.

### 12.1 New password hashing

Do **not** call:

```go
easyhash.Hash(password)
```

because the current high-level default is bcrypt.

Tap4Furry explicitly requires Argon2id.

Use the current equivalent of:

```go
easyhash.Hash(password, easyhash.WithArgon2id())
```

or the actual current Argon2id option exposed by the library.

### 12.2 Login verification / rehash

Use the library's high-level migration path.

Build a Tap4Furry password policy based on the current easyhash policy API:

```text
PreferredAlgorithm = Argon2id
Argon2id parameters = easyhash current default Argon2id parameters
```

Do **not** use `easyhash.DefaultPolicy()` unmodified because its current preferred algorithm is bcrypt.

Do **not** use `StrongPolicy()` unmodified for the same reason.

Use `VerifyAndUpgrade` or the current equivalent.

When login verifies and `upgraded == true`, update the stored password hash.

The update must be race-safe.

Prefer compare-and-swap semantics:

```text
UPDATE ...
SET password_hash = new
WHERE user_id = ?
  AND password_hash = old
```

Do not overwrite a newer concurrent hash.

### 12.3 Missing-user timing behavior

Login must return the same user-visible error for:

```text
unknown email
wrong password
```

Use an Argon2id dummy verification path for nonexistent identities so the obvious password-verification timing gap is reduced.

Do not log whether a login email exists.

---

## 13. Public Session Model

P0-1A uses opaque server-side sessions.

### 13.1 Raw token

Generate at least 256 bits of random entropy using `crypto/rand`.

Encode the browser token using URL-safe unpadded encoding.

The raw token exists only in:

```text
browser cookie
temporary process memory
```

Do not store the raw token in PostgreSQL.

### 13.2 Storage hash

Persist:

```text
SHA-256(raw session token)
```

as the 32-byte `sessions.token_hash`.

This deterministic lookup hash is intentionally different from user password hashing.

Do not use a slow KDF for session lookup.

### 13.3 Lifetime

Initial public-session policy:

```text
absolute lifetime = 30 days
idle lifetime     = 14 days
touch interval    ≈ 10 minutes
```

It is acceptable to keep these as explicit well-named constants in P0-1A.

On authenticated request:

- reject revoked session;
- reject absolute expiry;
- reject idle expiry;
- reject deleted/disabled account;
- throttle `last_seen_at` / idle-expiry extension rather than writing every request.

### 13.4 Cookie

Production public cookie:

```text
__Host-tap4furry_session
Secure
HttpOnly
SameSite=Lax
Path=/
No Domain
```

Development/test over local HTTP cannot use a Secure `__Host-` cookie.

Use a separate development cookie name, for example:

```text
tap4furry_session
```

Production configuration must never silently downgrade Secure/`__Host-` semantics.

Do not expose the token in JSON responses.

---

## 14. Basic Unsafe-request Origin Guard

Full session-bound CSRF belongs to P0-1B.

P0-1A must still avoid unrestricted cookie-authenticated unsafe requests.

Add a small exact-origin guard for relevant Public API unsafe routes.

Required public origin:

```text
PUBLIC_ORIGIN
```

Behavior:

- production: `PUBLIC_ORIGIN` is required;
- development may use a safe local default matching the existing Public Web dev origin if current repository conventions already define one;
- test may inject an explicit value.

For:

```text
POST
PUT
PATCH
DELETE
```

on P0-1A auth/profile endpoints, require a browser `Origin` that exactly matches the configured public origin.

Do not enable broad CORS.

Do not mistake this for the final P0-1B CSRF system.

---

## 15. Authentication Context

Add one public-auth resolution boundary.

Conceptually:

```text
Cookie
  ↓
Session token hash
  ↓
sessions
  ↓
users
  ↓
Authenticated Actor
```

Transport may resolve authentication, but authorization/application behavior remains outside transport.

Use request-scoped Fiber context/local storage.

The resolved actor should minimally contain:

```text
UserID
SessionID
SessionKind
AuthenticatedAt
```

Do not expose password/auth identity internals.

---

## 16. Application Use Cases

### Register

Input:

```text
email
password
```

Flow:

1. validate and normalize email;
2. validate password policy;
3. hash password with easyhash Argon2id;
4. generate UUIDv7 IDs;
5. begin transaction;
6. create `users`;
7. create `user_profiles`;
8. create `auth_identities(provider=email)`;
9. create `password_credentials`;
10. create public password-auth session;
11. commit;
12. return current-user view;
13. set session cookie.

Hash before opening the DB transaction.

### Login

Input:

```text
email
password
```

Flow:

1. normalize email;
2. lookup local identity + user + credential;
3. unknown identity follows dummy Argon2 verification path;
4. verify with Tap4Furry easyhash Argon2id policy;
5. reject disabled/deleted account;
6. CAS-upgrade hash when required;
7. create public session;
8. set cookie;
9. return current-user view.

Unknown email and wrong password both return:

```text
AUTH_INVALID_CREDENTIALS
```

### Logout

Authenticated session required.

```text
revoke current session
clear cookie
return 204
```

### Get Current User

Return account + profile + the user's own email/verification state.

Never return password/session/auth internals.

### Update My Profile

Allowed:

```text
handle
display_name
bio
search_engine_indexing
```

Email/password are not changed here.

### Get Public Profile

```text
GET /users/{handle}
```

Return only:

```text
handle
display_name
bio
```

Do not expose email, verification state, sessions, auth identities, account security state, or the indexing preference.

---

## 17. Public API Contract

Extend:

```text
contracts/openapi/public.yaml
```

Recommended endpoints:

```text
POST  /auth/register
POST  /auth/login
POST  /auth/logout

GET   /me
PATCH /me/profile

GET   /users/{handle}
```

Keep existing health endpoints.

Recommended success codes:

```text
register       201
login          200
logout         204
get me         200
update profile 200
public profile 200
```

Introduce one stable API error schema:

```json
{
  "code": "AUTH_INVALID_CREDENTIALS",
  "message": "..."
}
```

Required error-code equivalents:

```text
VALIDATION_ERROR
AUTH_EMAIL_ALREADY_REGISTERED
AUTH_INVALID_CREDENTIALS
AUTH_UNAUTHENTICATED
AUTH_ACCOUNT_DISABLED
PROFILE_HANDLE_UNAVAILABLE
PROFILE_NOT_FOUND
```

Do not expose raw SQL/pgx/easyhash errors.

Generated OpenAPI DTOs remain transport-only.

Regenerate Public Go and Orval clients from the spec.

Admin OpenAPI stays health-only.

---

## 18. sqlc Queries

Add focused query source such as:

```text
identity.sql
auth.sql
session.sql
```

Queries should cover at minimum:

```text
CreateUser
CreateProfile
CreateEmailIdentity
CreatePasswordCredential
FindLocalCredentialByEmailSubject
CompareAndSwapPasswordHash
CreateSession
FindActiveSessionByTokenHash
TouchSession
RevokeSession
GetMe
UpdateProfile
GetPublicProfileByHandle
```

Do not create repository wrappers that merely mirror sqlc.

Transactions belong in auth/identity application use cases.

---

## 19. Frontend Scope

P0-1A should provide a minimal vertical slice in Public Web.

Admin remains unchanged apart from generated/build compatibility.

Add minimal routes:

```text
/login
/register
/account
```

Use Astro shells with React islands for private/authenticated behavior.

Do not convert Public Web into a global React application.

### Register

Minimal form:

```text
email
password
submit
login link
```

### Login

Minimal form:

```text
email
password
submit
register link
```

### Account

React island loads:

```text
GET /api/me
```

Allows editing:

```text
handle
display name
bio
search indexing preference
```

and logout.

Do not SSR private account/session state into cacheable HTML.

A minimal public:

```text
/users/[handle]
```

page may be added while preserving Web-first/public-content architecture.

Use:

```text
@tap4furry/api-client/public
```

Do not hand-write duplicate DTOs.

Do not store session/JWT/password values in browser storage.

---

## 20. Config Changes

Extend API config minimally with:

```text
PUBLIC_ORIGIN
```

No JWT signing secret is required.

No session signing secret is required because the session is opaque/server-side and looked up by hash.

Update:

```text
server/env/api.example
docs/development.md
```

Do not print or rewrite ignored real local credentials.

Prefer a safe development default for non-secret local origin configuration if compatible with the current dev ports.

---

## 21. Transaction Boundaries

### Register

One DB transaction for:

```text
User
Profile
Email Identity
Password Credential
Session
```

Argon2 hashing occurs before transaction begin.

### Login

Do not hold DB transaction during Argon2 verification.

After verification use the smallest required write transaction for CAS rehash/session creation.

### Profile update

Explicit, minimal write path.

No hidden transaction behavior.

---

## 22. Concurrency / Constraints

Database constraints are authoritative for:

```text
email identity uniqueness
profile handle uniqueness
session token hash uniqueness
```

Application prechecks may improve UX but are not correctness boundaries.

Translate expected constraint races into stable application errors.

Never expose PostgreSQL constraint names.

---

## 23. Tests

### Unit

At minimum:

```text
email normalization
password length policy
handle validation
session token hashing
cookie policy by environment
auth error mapping
```

### easyhash

Verify:

```text
new hash identifies as Argon2id
correct password succeeds
wrong password fails
bcrypt/legacy successful login upgrades to Argon2id
equal/stronger Argon2id is not downgraded
```

### PostgreSQL integration

Using disposable PostgreSQL:

```text
fresh migrations
create account
duplicate email blocked
handle uniqueness
session lookup
revoked rejection
idle expiry
absolute expiry
public/private profile projection
CAS rehash update
```

Do not use SQLite.

### HTTP integration

Test:

```text
register
login
invalid login
unauthenticated /me
authenticated /me
profile update
handle conflict
public profile
logout
post-logout rejection
Origin rejection
cookie attributes
```

### Privacy

Explicitly assert public profile never includes:

```text
email
password hash
auth identity subject
session state/token/hash
```

and `/me` never returns credential/session secrets.

---

## 24. Development Infra Verification

After disposable tests pass, apply P0-1A to the prepared shared development DB using normal local configuration.

Allowed:

```text
gfp_migrator → Goose migration
gfp_api      → Public API runtime
```

Do not SSH.

Verify through actual application paths:

```text
migration applies
gfp_api DML works
register
login
/me
profile update
logout
```

Use clearly temporary development identities.

Do not expose real DSNs/passwords/Tailnet addresses.

Do not widen database roles or Redis ACLs.

---

## 25. CI

Existing P0-0 CI must remain green.

Extend disposable CI for P0-1A:

```text
pnpm check
pnpm generate
generated drift
Go tests
frontend typecheck/build
fresh PostgreSQL migration
P0-1A DB/HTTP integration
Docker builds
secret audit
```

CI must never connect to shared Infra.

No OAuth/email/Turnstile service is needed in P0-1A CI.

---

## 26. Documentation

Update where appropriate:

```text
CHANGELOG.md
docs/development.md
docs/engineering/tech-stack.md
```

`tech-stack.md` should now explicitly name:

```text
github.com/gofurry/easyhash
Argon2id
```

Do not rewrite unrelated design documents.

---

## 27. Security Boundaries That Must Already Hold

Even before P0-1B/C/D:

- passwords never logged;
- plaintext passwords never persisted;
- new password hashes explicitly use Argon2id;
- unknown email and wrong password return the same public login error;
- raw session tokens never reach PostgreSQL;
- session token hashes never reach frontend;
- cookies are HttpOnly;
- production cookie is Secure + `__Host-`;
- public/private profile data are separate;
- disabled/deleted users cannot authenticate;
- unsafe P0-1A routes enforce the exact-origin baseline;
- no JWT/localStorage auth;
- no broad CORS;
- no secret values in slog/errors.

Document that P0-1B/P0-1D are still required before production authentication is considered complete.

---

## 28. Implementation Order

1. Audit current `dev`, P0-0 baseline, docs, easyhash API.
2. Run baseline `pnpm check`.
3. Add migration 2.
4. Add sqlc queries and regenerate.
5. Create `internal/identity` and `internal/auth`.
6. Implement password/session/application use cases.
7. Extend Public OpenAPI.
8. Regenerate Go + Orval.
9. Implement Public transport/auth context/origin guard.
10. Add minimal Public Web login/register/account/profile flows.
11. Add unit + DB + HTTP/privacy tests.
12. Run disposable integration.
13. Apply migration/run smoke against real development Infra with no SSH.
14. Keep CI/images green.
15. Update docs/changelog.
16. Re-run generation and full verification.
17. Inspect complete diff and secret/dependency scope.
18. Commit locally on `dev`.
19. Do not push.

---

## 29. Acceptance Criteria

### Schema

```text
users                  PASS
user_profiles          PASS
auth_identities        PASS
password_credentials   PASS
sessions               PASS
```

No future P0-1 tables exist.

### Password

```text
new hashes = Argon2id                           PASS
wrong password rejected                         PASS
unknown identity same public error              PASS
legacy/bcrypt successful login upgrades hash    PASS
no downgrade                                    PASS
```

### Sessions

```text
raw token has >=256 bits entropy       PASS
only SHA-256 lookup hash stored        PASS
HttpOnly cookie                        PASS
production __Host-/Secure semantics   PASS
revoked rejected                       PASS
idle expired rejected                  PASS
absolute expired rejected              PASS
disabled/deleted rejected              PASS
touch throttled                        PASS
```

### API

```text
POST  /auth/register   PASS
POST  /auth/login      PASS
POST  /auth/logout     PASS
GET   /me              PASS
PATCH /me/profile      PASS
GET   /users/{handle}  PASS
```

Health endpoints remain green.

### Privacy

No public profile response exposes private auth/account fields.

### Generation

```text
pnpm generate             PASS
second generate no drift  PASS
Go OpenAPI generated      PASS
Orval generated           PASS
sqlc generated            PASS
```

### Verification

Existing `pnpm check` and CI remain green.

Fresh DB migrates from 00001 → 00002.

Existing `gfp_dev` migrates forward without editing 00001.

### Forbidden scope

No:

```text
OAuth
email verification/reset
auth_challenges
security_events
Admin auth
user_roles
Turnstile
NATS
MongoDB
pgvector
JWT
Resource domain
```

is introduced.

---

## 30. Stop Conditions

Stop and report evidence rather than changing architecture if:

1. current `dev` materially differs from the expected P0-0 baseline;
2. easyhash current API cannot provide explicit Argon2id + verify/upgrade;
3. Go 1.27 standard-library UUIDv7 cannot satisfy the selected ID contract;
4. `gfp_migrator` cannot apply the new schema/grants;
5. `gfp_api` cannot perform required runtime DML after migration;
6. production cookie semantics would need to be weakened;
7. OpenAPI/Fiber generation becomes incompatible;
8. real dev verification would require SSH or widened shared-Infra privileges;
9. a secret would need to be tracked;
10. P0-1B/C/D functionality is unexpectedly required for this phase to work.

For ordinary implementation details, choose the simplest solution consistent with repository contracts.

---

## 31. Final Verification

Before completion:

1. run full repository verification;
2. run P0-1A integration tests;
3. run generation again;
4. verify no drift;
5. inspect `git status --short`;
6. inspect `git diff --stat`;
7. inspect complete diff;
8. verify no `.local` file is tracked;
9. verify no live Tailnet address/DSN/password appears in tracked files;
10. verify no future P0-1 scope slipped in;
11. update `CHANGELOG.md`.

Do not claim unexecuted checks passed.

---

## 32. Commit Contract

After all acceptance gates pass:

```text
branch: dev
```

Recommended commit:

```text
feat: add identity and local authentication foundation
```

Do not push.

Do not merge `main`.

Do not tag/release.

---

## 33. Final Codex Report

Return the report in Chinese.

Include:

### Implemented

- DB entities;
- Application/Domain packages;
- Public API endpoints;
- Public Web flows.

### Password Security

- easyhash API/version used;
- explicit Argon2id policy;
- rehash behavior.

Never include password hashes.

### Session Security

- cookie behavior;
- server-side token-hash model;
- expiry behavior.

Never include session tokens/hashes.

### Database

- migration number;
- sqlc generation;
- real development migration result.

### Verification

List each command actually executed and pass/fail.

### Deviations

Any intentional deviation and why.

### Remaining P0-1 Work

Only:

```text
P0-1B
P0-1C
P0-1D
```

or unresolved blockers.

### Git

Report:

```text
branch
final commit SHA
git status
```

Do not push.
