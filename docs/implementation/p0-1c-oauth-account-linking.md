# P0-1C — Google/GitHub OAuth + Account Linking

**Repository:** `deepfurry/tap4furry`\
**Target branch:** `dev`\
**Prerequisite:** P0-1B complete\
**Status:** Codex implementation specification

## 1. Goal

P0-1C adds:

```text
Google OIDC
GitHub OAuth
Explicit account linking/unlinking
Provider re-authentication
OAuth-authenticated Public Sessions
Redis OAuth flow state
```

The final account model remains:

```text
Authentication Identity
        ↓
     User Account
        ↓
    Public Profile
```

Supported authentication methods after this phase:

```text
Password
Google
GitHub
```

A user may have any valid combination, but an account must never be left with zero authentication methods.

P0-1C must preserve all P0-1A/P0-1B behavior and security guarantees.

---

## 2. Verified P0-1B Baseline

Extend the actual current `dev` implementation.

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

Current provider CHECK already permits:

```text
email
google
github
```

Current security baseline:

```text
explicit easyhash Argon2id
opaque Public Session
SHA-256 session lookup hash only
30d absolute / 14d idle
session-bound HMAC CSRF
exact Origin on unsafe requests
email verification/recovery
password change/reset
reauthentication
session management
security events
```

Do not edit migrations `00001`, `00002`, or `00003`.

---

## 3. Development OAuth Configuration Already Exists

The developer has already created dedicated Dev OAuth clients and stored their real credentials in ignored:

```text
server/env/api.local
```

Expected private variables:

```text
GOOGLE_OAUTH_CLIENT_ID
GOOGLE_OAUTH_CLIENT_SECRET

GITHUB_OAUTH_CLIENT_ID
GITHUB_OAUTH_CLIENT_SECRET
```

Never print, commit, rewrite, or expose these values.

### Frozen callbacks

Development:

```text
http://localhost:4321/api/auth/oauth/google/callback
http://localhost:4321/api/auth/oauth/github/callback
```

Future production:

```text
https://tap4furry.com/api/auth/oauth/google/callback
https://tap4furry.com/api/auth/oauth/github/callback
```

Do not introduce redirect-URI env vars.

Derive redirects from:

```text
PUBLIC_ORIGIN + fixed /api/auth/oauth/.../callback path
```

Google Dev scopes are frozen:

```text
openid
email
profile
```

GitHub Dev OAuth App:

```text
owner = deepfurry
wildcard callback = disabled
Device Flow = disabled
expiring access tokens = enabled
```

GitHub requested scopes:

```text
read:user
user:email
```

---

## 4. Branch / Execution Rules

Work only on `dev`.

Before editing:

```bash
git branch --show-current
git status --short
git log -5 --oneline --decorate
```

Rules:

- preserve unrelated work;
- do not rewrite applied migrations;
- do not modify/merge `main`;
- do not tag/release;
- do not push unless explicitly requested;
- after verification, commit locally on `dev`.

Recommended commit:

```text
feat: add oauth authentication and account linking
```

---

## 5. Required Reading

Read only relevant context:

```text
AGENTS.md
this P0-1C spec
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

Inspect current:

```text
server/internal/auth/*
server/internal/identity/*
server/internal/redisstore/*
server/internal/transport/public/*
server/internal/config/*
server/db/migrations/*
server/db/queries/*
contracts/openapi/public.yaml
apps/web/src/*
```

Before provider implementation, check current official Google/GitHub OAuth/OIDC documentation.

Do not hand-roll OAuth/OIDC cryptography or JWT/JWKS validation.

---

## 6. Non-goals

Do not implement:

```text
Admin authentication
Admin sessions
Cloudflare Access
MFA
roles/RBAC
account merge
email-address change
password removal
GitHub repo access
Google API access beyond identity
long-term provider tokens
provider background jobs
Google One Tap
GitHub Device Flow
production OAuth clients
Turnstile
final auth rate limiting
JWT Tap4Furry sessions
NATS
MongoDB
pgvector
```

---

## 7. Dependencies

Add:

```text
golang.org/x/oauth2
```

for OAuth Authorization Code flows.

For Google OIDC ID-token verification, use a focused maintained verifier compatible with current Go 1.27, such as:

```text
github.com/coreos/go-oidc/v3/oidc
```

if current review confirms it remains appropriate.

Do not manually implement:

```text
JWT signature validation
JWKS fetching/caching
issuer/audience validation
```

Do not add a large Google SDK solely for sign-in if a focused verifier is sufficient.

---

## 8. Stable Provider Identity

Email is never a provider identity key.

Google:

```text
provider         = google
provider_subject = verified OIDC `sub`
```

GitHub:

```text
provider         = github
provider_subject = decimal string of numeric GitHub user ID
```

Never use:

```text
Google email/name/picture
GitHub login/email/profile URL
```

as stable subject keys.

If an existing `(provider, provider_subject)` reports a different email later, the stable provider subject still selects the same Tap4Furry account.

---

## 9. Account Email Invariant

Every newly created OAuth account must also have the existing account-email identity:

```text
provider = email
provider_subject = normalized email
email = normalized email
```

So a new OAuth-only account looks like:

```text
User
├── email identity
└── Google or GitHub identity
```

It has:

```text
NO password_credentials row
```

unless the user originally registered with a password.

Therefore:

> `provider=email` is the account email/contact identity. It is a login method only when `password_credentials` exists.

Do not create a fake password credential for OAuth accounts.

The existing unique `(provider, provider_subject)` constraint remains authoritative for account email uniqueness.

---

## 10. No Automatic Linking by Email

Hard rule:

```text
same email != same account
```

When OAuth subject is new:

### Email unused

Create a new OAuth account.

### Email already belongs to an existing Tap4Furry account

Do not:

```text
auto-link
auto-login the existing account
merge
move identities
create a duplicate account-email identity
```

Make no canonical mutation.

Redirect to a stable account-link-required UI state, e.g.:

```text
/login?oauth_error=account_link_required
```

Message:

> An account already exists for this email. Sign in to that account first, then explicitly link this provider.

Do not put email/User ID/provider subject in the URL.

---

## 11. Provider Email Policy

### GitHub

After token exchange call:

```text
GET /user
GET /user/emails
```

Select:

1. primary + verified email;
2. otherwise any verified email;
3. otherwise reject new-account creation safely.

Provider subject is still numeric user ID.

### Google

Validate OIDC ID token and require:

```text
sub
email
```

Read:

```text
email_verified
hd
name
picture
```

Do not persist picture/avatar in P0-1C.

For a new account, automatically mark the local account-email identity verified only when Google is authoritative:

```text
email_verified == true
AND
(
  email is @gmail.com
  OR hd is non-empty
)
```

Otherwise create the local account-email identity unverified and preserve the existing P0-1B email-verification flow.

Do not block OAuth sign-up solely because a third-party Google-account email requires Tap4Furry verification.

---

## 12. Profile Initialization

For a brand-new OAuth-created account:

```text
handle = NULL
bio = NULL
search_engine_indexing = false
```

`display_name` may use the provider's name if available and valid.

Do not:

```text
derive handle from GitHub username
derive handle from Google name
store avatar
overwrite existing profile data during later OAuth login
```

---

## 13. Migration 00004

Create:

```text
server/db/migrations/00004_oauth_identity.sql
```

No OAuth flow table is needed; temporary OAuth state belongs in Redis.

### Extend Security Events

Preserve existing event values and add:

```text
oauth_login_succeeded
oauth_identity_linked
oauth_identity_unlinked
oauth_reauthenticated
```

Do not edit migration 00003.

### Session auth-method invariant

Add a CHECK allowing only currently implemented session methods:

```text
password
password_reset
google
github
```

Do not add future methods.

### Do not add OAuth credential columns

Never add to PostgreSQL:

```text
access_token
refresh_token
id_token
state
PKCE verifier
nonce
```

---

## 14. General Account Auth Lock

P0-1B uses password-credential locking for several auth mutations.

OAuth-only users have no password credential.

Introduce one shared account-auth mutation lock, preferably:

```text
SELECT user row FOR UPDATE
```

Conceptual query:

```text
LockUserAuthState
```

Use a consistent lock order:

```text
User auth lock
→ password credential when needed
→ identities/sessions/challenges
```

Audit/refactor existing:

```text
local login completion
password reset
password change
password reauth
verification/challenges
session revocation
```

and new:

```text
OAuth login
OAuth link
OAuth unlink
OAuth reauth
```

Required invariant:

> OAuth login racing password reset/change must not create a session that escapes revocation.

---

## 15. Generalize Session Creation

Current local session creation depends on password state.

P0-1C must support:

```text
password
password_reset
google
github
```

Session insertion should require:

```text
active/non-deleted User
explicit auth_method
```

but must not require a password credential for OAuth sessions.

Removing password dependence from session INSERT must not weaken local login race safety.

Local login must lock/re-read the password credential after acquiring the User auth lock before creating the session.

Preserve all existing opaque-token, expiry, touch, revocation, and CSRF semantics.

---

## 16. Actor Model

Extend authenticated Actor with:

```text
AuthMethod
```

Result:

```text
UserID
SessionID
SessionKind
AuthMethod
AuthenticatedAt
```

Never expose provider subject.

---

## 17. Redis OAuth Flow Store

OAuth temporary state is ephemeral Redis data.

Add a narrow application-owned flow-store interface backed by existing `redisstore`.

Flow modes:

```text
login
link
reauth
```

TTL:

```text
10 minutes
```

State:

```text
>=256 random bits
```

Use a digest of state for the Redis key:

```text
gfp:auth:oauth:flow:<state-digest>
```

Payload should contain only required ephemeral values:

```text
provider
mode
PKCE verifier
Google nonce when applicable
user_id        # link/reauth
session_id     # link/reauth
created_at
```

Do not store:

```text
email
provider access/refresh/ID token
Tap4Furry session token
CSRF token
client secret
```

Store with NX + TTL.

Consume atomically, e.g. Redis `GETDEL`.

Callback replay must fail.

Redis failure disables OAuth flow but must not break local password authentication or canonical PostgreSQL sessions.

---

## 18. PKCE / State / Nonce

### GitHub

PKCE S256 is mandatory.

Authorization request includes:

```text
state
code_challenge
code_challenge_method=S256
```

Token exchange includes:

```text
code_verifier
```

### Google

Use backend Authorization Code/OIDC.

Use:

```text
state
client-secret protected code exchange
OIDC ID-token verification
nonce when supported/used by the selected OIDC flow
```

Use PKCE S256 if current official Google Web OAuth flow and selected library support it correctly.

Do not invent non-standard Google parameters.

Codex must verify current official behavior before implementation.

---

## 19. Google OIDC Validation

Do not trust decoded claims without signature validation.

Validate:

```text
signature
issuer
audience == GOOGLE_OAUTH_CLIENT_ID
expiry/not-before as supported
nonce when issued
sub non-empty
```

Use OIDC discovery/JWKS through the maintained verifier.

Do not use a generic tokeninfo HTTP endpoint as the primary verifier when standard local OIDC verification is available.

After extracting the validated identity, discard provider tokens.

---

## 20. GitHub Identity Retrieval

After Authorization Code exchange:

1. fetch authenticated `/user`;
2. require numeric user ID;
3. fetch `/user/emails`;
4. choose verified email per this spec;
5. discard access token and any refresh token.

Use current recommended GitHub REST headers/version.

No repo APIs.

No provider token persistence.

---

## 21. Provider Adapter Boundary

Provider network details stay outside core account logic.

Use an explicit adapter boundary, conceptually:

```go
type OAuthProvider interface {
    AuthorizationURL(...)
    Exchange(...)
}
```

Return a Tap4Furry-owned identity result such as:

```text
Provider
Subject
Email
Email assurance
DisplayName candidate
```

Do not pass provider token objects into Domain/Application APIs.

Suggested infrastructure location:

```text
server/internal/oauthprovider/
```

Do not build a generic plugin framework.

Only Google and GitHub exist.

---

## 22. OAuth Login Start

Endpoint:

```text
GET /auth/oauth/{provider}/start
```

Provider enum:

```text
google
github
```

Flow:

1. validate provider enabled;
2. generate state;
3. generate PKCE verifier/challenge where applicable;
4. generate Google nonce when applicable;
5. store Redis flow `mode=login`;
6. redirect to provider.

Do not accept arbitrary `return_to`.

Fixed success destination:

```text
/account
```

Fixed failure destination:

```text
/login?oauth_error=<stable-code>
```

---

## 23. OAuth Callback

Endpoint template:

```text
GET /auth/oauth/{provider}/callback
```

This matches the frozen external URLs through `/api` proxying.

Flow:

1. validate provider;
2. require state;
3. atomically consume flow;
4. require flow provider matches callback provider;
5. handle provider denial safely;
6. exchange code;
7. verify/fetch provider identity;
8. discard provider tokens;
9. resolve/create/link/reauth according to flow mode;
10. set replacement/new Tap4Furry Public Session cookie when required;
11. redirect to fixed same-origin UI.

Headers:

```text
Cache-Control: no-store
Referrer-Policy: no-referrer
```

Do not put provider code/state/token in outgoing Location.

---

## 24. Login — Existing Provider Identity

If `(provider, provider_subject)` already exists:

1. begin transaction;
2. lock User auth state;
3. re-read provider identity;
4. require active/non-deleted account;
5. optionally refresh private provider email metadata;
6. create Public Session:
   - `auth_method=google`, or
   - `auth_method=github`;
7. record `oauth_login_succeeded`;
8. commit;
9. set session cookie;
10. redirect `/account`.

Do not rewrite public profile.

Do not select another account because provider email changed.

---

## 25. Login — New OAuth Account

If provider subject is unknown:

1. normalize provider email;
2. query existing account-email identity;
3. if email exists → account-link-required, no mutation;
4. if unused:
   - create UUIDv7 User;
   - create Profile;
   - create `provider=email` identity;
   - create Google/GitHub identity;
   - create no password credential;
   - create provider-authenticated Public Session;
   - record `account_registered`;
   - record `oauth_login_succeeded`;
   - commit.

Apply provider email verification policy.

If local email remains unverified, existing P0-1B verification must work for the OAuth-only account.

---

## 26. Auth Methods API

Add:

```text
GET /me/auth-methods
```

Owner-only response concept:

```json
{
  "password": true,
  "providers": [
    {
      "provider": "google",
      "email": "user@example.com",
      "linked_at": "..."
    }
  ]
}
```

Never expose:

```text
provider_subject
tokens
OAuth state
PKCE
```

Password presence is determined by `password_credentials`.

The email identity alone is not a password method.

---

## 27. Fresh Authentication

Sensitive auth-method mutations use the P0-1B `AuthenticatedAt`.

Freeze:

```text
fresh auth window = 15 minutes
```

Freshness required for:

```text
start provider linking
provider unlink
```

Normal session activity does not refresh `AuthenticatedAt`.

If stale:

```text
AUTH_REAUTH_REQUIRED
```

---

## 28. Link Provider

Endpoint:

```text
POST /me/auth-methods/{provider}/link
```

Requires:

```text
valid Public Session
exact Origin
CSRF
AuthenticatedAt <=15m old
```

Store flow:

```text
mode=link
provider
user_id
session_id
PKCE/nonce data
```

Return:

```json
{"authorization_url":"..."}
```

Frontend redirects with `window.location.assign`.

### Link callback

Require current session cookie and exact match:

```text
actor.UserID == flow.user_id
actor.SessionID == flow.session_id
```

Then:

- verify provider identity;
- lock User auth state;
- revalidate session/freshness;
- if provider subject belongs to another User → reject;
- if already linked to current User → idempotent success;
- otherwise insert provider identity.

Explicit linking does not require provider email to equal account email.

Do not:

```text
change account email
create another email identity
merge accounts
overwrite profile
```

Record:

```text
oauth_identity_linked
```

Rotate current Public Session because auth configuration changed.

Replacement session preserves current `AuthMethod`.

Old session and CSRF become invalid.

---

## 29. Provider Re-authentication

OAuth-only users need step-up without a password.

Endpoint:

```text
POST /me/auth-methods/{provider}/reauthenticate
```

Requires:

```text
current valid session
Origin
CSRF
provider already linked to current User
```

It does not require freshness.

Store:

```text
mode=reauth
provider
user_id
session_id
```

Callback must require:

- same User/session;
- returned provider subject exactly matches the already-linked provider identity.

If user selects a different provider account, fail.

On success:

```text
rotate current session
auth_method = provider
authenticated_at = now
record oauth_reauthenticated
```

Do not relink or move identities during reauth.

---

## 30. Unlink Provider

Endpoint:

```text
DELETE /me/auth-methods/{provider}
```

Requires:

```text
session
Origin
CSRF
fresh auth <=15m
```

Authentication-method count:

```text
password credential ? 1 : 0
+ Google linked ? 1 : 0
+ GitHub linked ? 1 : 0
```

Reject removal if zero methods would remain:

```text
AUTH_LAST_METHOD
```

Also reject if:

```text
actor.AuthMethod == provider being removed
```

with:

```text
AUTH_REAUTH_REQUIRED
```

The user must reauthenticate using another remaining method first.

Transaction:

1. lock User auth state;
2. revalidate current session/freshness;
3. verify provider linked;
4. enforce last-method/current-method rules;
5. delete provider identity;
6. revoke all active sessions whose `auth_method` is removed provider;
7. rotate current session preserving its remaining auth method;
8. record `oauth_identity_unlinked`;
9. commit;
10. set replacement cookie.

Do not revoke the user's Google/GitHub grant through provider APIs in P0-1C.

---

## 31. Existing Password Re-auth Compatibility

Keep:

```text
POST /auth/reauthenticate
```

For password-capable accounts.

After P0-1C:

```text
password method → password reauth
OAuth method    → provider reauth
```

Do not force OAuth-only accounts to create passwords.

---

## 32. Provider Token Safety

Provider tokens may exist only in Go process memory during callback exchange/identity lookup.

Never persist to:

```text
PostgreSQL
Redis
River
logs
Security Events
cookies
frontend
browser storage
```

GitHub may return expiring access + refresh tokens because the Dev app has token expiration enabled.

Use the access token only for `/user` and `/user/emails`, then discard both access and refresh tokens.

Do not request GitHub `offline_access`.

Google:

- do not request offline access;
- discard access/ID/refresh tokens after identity extraction.

---

## 33. Logging / Error Safety

Never log:

```text
authorization code
state
PKCE verifier/challenge
nonce
access token
refresh token
ID token
client secret
Authorization header
raw callback query
```

Use stable UI errors only:

```text
provider_denied
provider_unavailable
provider_invalid
account_link_required
provider_already_linked
reauth_required
reauth_failed
```

Do not forward provider `error_description`.

No arbitrary open redirect.

---

## 34. Public OpenAPI

Extend:

```text
contracts/openapi/public.yaml
```

Add:

```text
GET /auth/oauth/{provider}/start
GET /auth/oauth/{provider}/callback

GET    /me/auth-methods
POST   /me/auth-methods/{provider}/link
POST   /me/auth-methods/{provider}/reauthenticate
DELETE /me/auth-methods/{provider}
```

Provider enum:

```text
google
github
```

Authenticated mutation endpoints require existing:

```text
PublicSession
X-CSRF-Token
Origin
```

Add only needed error codes:

```text
AUTH_PROVIDER_UNAVAILABLE
AUTH_PROVIDER_INVALID
AUTH_PROVIDER_ALREADY_LINKED
AUTH_PROVIDER_NOT_LINKED
AUTH_ACCOUNT_LINK_REQUIRED
AUTH_REAUTH_REQUIRED
AUTH_LAST_METHOD
```

Generated DTOs remain transport-only.

Regenerate Go + Orval.

---

## 35. sqlc

Add/extend focused queries for:

```text
LockUserAuthState
FindProviderIdentity
FindProviderIdentityForUpdate
CreateProviderIdentity
UpdateProviderIdentityMetadata
DeleteProviderIdentity
ListProviderIdentitiesForUser
FindEmailIdentityBySubject
CreateEmailIdentityForOAuthAccount
HasPasswordCredential
CountLinkedProviderIdentities
CreateSession without password dependency
RevokeSessionsByAuthMethod
```

Do not create ceremonial Repository wrappers.

Transactions remain in auth/application use cases.

---

## 36. Frontend

Extend Public Web only.

### Login/Register

Add ordinary links:

```text
Continue with Google
Continue with GitHub
```

to:

```text
/api/auth/oauth/google/start
/api/auth/oauth/github/start
```

No Google JS SDK.

No GitHub JS SDK.

No popup flow.

### Account Security

Display:

```text
Password: configured/not configured
Google: linked/not linked
GitHub: linked/not linked
```

For linked provider, owner-only provider email may be shown.

Do not show provider subject.

Implement:

```text
link
provider reauth
unlink
```

with generated API client.

Read stable `oauth_error` query values on `/login` and `/account`.

Never display raw provider errors.

---

## 37. No Browser OAuth Token Storage

Never store provider tokens/temporary secrets in:

```text
localStorage
sessionStorage
IndexedDB
persistent React state
Astro cookies
URL fragments
```

The frontend only sees:

```text
provider authorization URL
provider redirects
Tap4Furry HttpOnly session cookie
stable non-secret error code
```

PKCE/state/nonce are server-controlled through Redis.

---

## 38. Provider Network Timeouts

Use bounded contexts for:

```text
token exchange
Google OIDC discovery/JWKS
GitHub /user
GitHub /user/emails
```

Do not use unlimited default network clients.

Do not blindly retry authorization-code exchange.

---

## 39. Fake Provider CI

CI must not use real Google/GitHub credentials.

Use injectable provider adapters or local fake provider HTTP servers.

Test:

```text
authorization redirect
state
PKCE
callback exchange
Google verified identity
GitHub profile/email
account creation
existing-account login
link
reauth
unlink
```

Do not weaken production verification just for tests.

---

## 40. Required Security Tests

### State / Redis

```text
>=256-bit state
10m TTL
one-time consume
replay rejected
expired rejected
provider mismatch rejected
Redis keys use gfp:
```

### GitHub PKCE

```text
S256 challenge
correct verifier
wrong/missing verifier rejected
```

### Google OIDC

```text
signature valid
issuer valid
audience valid
expired token rejected
nonce mismatch rejected when used
sub required
email required
```

### GitHub identity

```text
numeric ID required
primary verified email preferred
verified fallback
unverified-only new-account rejected
```

### Token persistence

Assert provider access/refresh/ID tokens never appear in:

```text
PostgreSQL
Redis after callback
Security Events
logs
API responses
frontend fixtures
```

---

## 41. Account Resolution Tests

Test:

```text
known Google subject → same User
known GitHub subject → same User
provider email changes → same User

unused email → new OAuth User
email identity exists
provider identity exists
no password credential
Public Session exists

existing local email + new OAuth subject
→ no auto-link
→ no duplicate account
→ account_link_required

provider identity linked to User A
+ link attempt from User B
→ rejected
```

Provider login must not overwrite user-edited profile.

---

## 42. OAuth-only Regression Tests

An OAuth-only User must support:

```text
GET /me
profile update
session list/revoke
provider reauth
link second provider
last-method rule
```

If local email remains unverified:

```text
email verification resend/consume works
```

Password reset remains enumeration-safe/no-op when no password credential exists.

Password reauth fails safely rather than crashing.

---

## 43. Link / Unlink Tests

Link:

```text
fresh password session + Google succeeds
fresh GitHub session + Google succeeds
stale session rejected
linked-elsewhere rejected
email mismatch allowed for explicit link
session rotated
old CSRF rejected
```

Unlink:

```text
Password + Google:
password reauth → unlink Google

Google + GitHub:
GitHub reauth → unlink Google

only Google → reject
only GitHub → reject
current auth provider == target → reauth required
removed-provider sessions revoked
current remaining-method session rotated
```

---

## 44. Concurrency Tests

At minimum:

```text
OAuth login vs password reset
OAuth login vs password change
two link callbacks same provider
link vs unlink
two callbacks same provider subject
two new-account callbacks same email
```

Required:

```text
no duplicate identities
no duplicate account email
no provider identity moves users
no session escapes reset/change revocation
```

---

## 45. OAuth Config Contract

Extend `server/env/api.example` with placeholders:

```text
GOOGLE_OAUTH_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_SECRET=
GITHUB_OAUTH_CLIENT_ID=
GITHUB_OAUTH_CLIENT_SECRET=
```

No redirect vars.

Each provider pair must be:

```text
both present
or
both absent
```

Partial config is an error.

Development/test may run with a provider disabled when both values are absent.

Real developer acceptance expects both configured.

Never include values in errors/logs.

---

## 46. Real Development Verification

The real Dev credentials already exist locally.

Add a safe smoke that verifies without printing secrets:

```text
credential pairs are present
callback derivation is exact
Redis OAuth flow works
provider start URL can be generated
```

Do not print authorization URLs containing state/PKCE values.

### Interactive provider E2E

Real provider authorization requires user/browser consent.

If Codex can execute it safely, run it.

If not, report:

```text
NOT RUN — requires human provider consent
```

Do not fake a PASS.

Manual Google acceptance:

```text
pnpm dev:api
pnpm dev:web
open http://localhost:4321/login
Continue with Google
→ consent/test account
→ callback
→ /account
→ /me works
→ Google appears in auth methods
```

Manual GitHub acceptance is equivalent.

Then manually test linking from an existing password account.

---

## 47. CI

Existing P0-1B CI remains green.

CI uses:

```text
disposable PostgreSQL
disposable Redis
fake providers
```

Never real OAuth secrets.

Required:

```text
pnpm check
pnpm generate
generated drift
fresh migrations 00001→00004
OAuth fake-provider integration
auth concurrency tests
frontend typecheck/build
Docker builds
secret audit
```

---

## 48. Documentation

Update:

```text
CHANGELOG.md
docs/development.md
docs/architecture/security.md
docs/engineering/tech-stack.md
```

Document:

```text
provider subject keys
callback derivation
Redis state/PKCE/nonce
no token persistence
no auto-link by email
explicit link/unlink
15m freshness
OAuth-only account semantics
```

Do not rewrite unrelated docs.

---

## 49. Implementation Order

1. Audit current `dev` and run baseline `pnpm check`.
2. Verify current Google/GitHub official OAuth docs.
3. Add migration 00004.
4. Add general User auth lock and refactor P0-1B auth mutations.
5. Generalize session creation and Actor.
6. Add OAuth config/callback derivation.
7. Add Redis flow store.
8. Add provider adapter boundary.
9. Implement GitHub Authorization Code + PKCE.
10. Implement Google Authorization Code/OIDC.
11. Implement OAuth login/account creation/no-auto-link.
12. Add auth-method listing.
13. Implement link.
14. Implement provider reauth.
15. Implement unlink.
16. Add Security Events.
17. Extend OpenAPI/sqlc and regenerate.
18. Extend Public Web.
19. Add fake-provider/security/concurrency tests.
20. Run disposable integration.
21. Run safe real-config smoke.
22. Run interactive provider E2E if environment supports it.
23. Keep all P0-1A/B tests green.
24. Update docs/changelog.
25. Re-run generation/drift/secret/scope audits.
26. Commit locally on `dev`.
27. Do not push.

---

## 50. Acceptance Criteria

### Database / locking

```text
00004 applies                         PASS
existing event values preserved       PASS
OAuth event values added              PASS
session auth-method CHECK              PASS
general User auth lock                 PASS
fresh 00001→00004                     PASS
```

### OAuth flow

```text
Redis 10m one-time state              PASS
GitHub PKCE S256                      PASS
Google OIDC validation                PASS
no provider tokens persisted          PASS
callback paths exact                  PASS
```

### Identity

```text
Google subject = sub                  PASS
GitHub subject = numeric ID string    PASS
existing subject selects same User    PASS
same-email auto-link absent           PASS
new OAuth account gets email identity PASS
no fake password credential           PASS
```

### Sessions

```text
google auth_method                    PASS
github auth_method                    PASS
OAuth login/reset race safe           PASS
rotation preserves CSRF/session rules PASS
```

### Linking / reauth / unlink

```text
15m freshness                         PASS
Origin + CSRF                         PASS
explicit link                         PASS
linked-elsewhere rejected             PASS
provider reauth                       PASS
last method cannot unlink             PASS
current provider cannot self-unlink   PASS
removed-provider sessions revoked     PASS
session rotation                      PASS
```

### Frontend

```text
Google/GitHub login actions           PASS
auth-method view                      PASS
link UI                               PASS
provider reauth UI                    PASS
unlink UI                             PASS
safe OAuth errors                     PASS
no browser provider-token storage     PASS
```

### Existing auth

All P0-1A/B flows remain green.

### Real provider verification

Automated fake-provider E2E must pass.

Real Google/GitHub interactive E2E must be reported honestly as:

```text
PASS
```

or:

```text
NOT RUN — human provider consent required
```

---

## 51. Stop Conditions

Stop and report instead of changing architecture if:

1. current `dev` materially differs from completed P0-1B;
2. P0-1B baseline CI is failing;
3. current Google flow cannot safely support backend Authorization Code/OIDC;
4. current GitHub OAuth Web Flow no longer supports PKCE S256;
5. Google validation would require hand-written JWT/JWKS cryptography;
6. real Dev credentials are partial/missing when real smoke is attempted;
7. provider callbacks differ from the frozen URLs;
8. OAuth-only accounts would require fake password credentials;
9. implementation would auto-link by email;
10. provider tokens would need to be persisted;
11. shared Infra requires SSH/privilege widening;
12. secrets would need to be committed;
13. P0-1D Admin/RBAC is required to complete this phase.

---

## 52. Final Verification / Commit

Before completion:

```text
pnpm check
OAuth/provider integration tests
auth concurrency tests
fresh migration test
pnpm generate
generated drift check
safe Dev OAuth config smoke
interactive OAuth E2E if available
full diff review
secret audit
scope audit
```

Update `CHANGELOG.md`.

Then commit locally on `dev`:

```text
feat: add oauth authentication and account linking
```

Do not push, merge `main`, tag, or release.

---

## 53. Final Codex Report

Return the report in Chinese.

Include:

### Implemented
Google/GitHub provider adapters, Redis flow state, OAuth login/account creation, auth-method listing, linking, reauth, unlink, frontend.

### Identity Safety
Explain Google `sub`, GitHub numeric ID, no auto-link by email, OAuth-only email identity.

### Token Safety
Confirm no provider access/refresh/ID token persisted and no state/PKCE/nonce logged.

### Database
Report migration 00004, common auth lock, session refactor.

### Verification
Separate:

```text
automated fake-provider E2E
safe real-provider config smoke
interactive Google E2E
interactive GitHub E2E
```

Never claim an interactive PASS if user consent was not actually completed.

### Deviations
Any intentional deviation and reason.

### Remaining P0-1

```text
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
