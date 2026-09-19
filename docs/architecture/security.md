# Tap4Furry — Authentication & Security Architecture

## Authentication Model

```text
Email + Password
Google OIDC
GitHub OAuth
       │
       ▼
auth_identities
       │
       ▼
User Account
       │
       ▼
Public Profile
```

Authentication identity is private and separate from public profile identity.

Implemented P0-1A/B/C/D scope includes local and Google/GitHub authentication, explicit
account linking, basic profiles, public sessions, email verification and recovery.
P0-1D adds static roles, Admin authentication and abuse controls; production deployment remains outside this phase.

## OAuth

Go backend owns OAuth flows.

Use Authorization Code flows with state and PKCE where applicable.

Temporary OAuth flow state belongs in Redis with short TTL and one-time consumption.

The `gfp:` Redis namespace remains a stable infrastructure contract after BRAND-0;
it is independent of the Tap4Furry product and browser-cookie names.

Do not retain provider access/refresh tokens unless a future feature explicitly requires provider API access.

## Account Linking

Do not auto-link accounts merely because provider emails match.

Link/unlink requires explicit user intent and re-authentication.

Do not unlink the last authentication method.

## Email / Password

Store passwords with:

```text
Argon2id
```

Favor length and password-manager compatibility over composition rules.

Email verification is required before high-cost contribution actions.

## Sessions

Use opaque server-side sessions.

Cookies:

```text
__Host-tap4furry_session
__Host-tap4furry_admin_session
```

Properties:

```text
Secure
HttpOnly
Path=/
No Domain
```

Public:

```text
SameSite=Lax
```

Admin:

```text
SameSite=Strict
```

Database stores only a hash of the raw session token.

PostgreSQL is the canonical session store.

## Lifetimes

Explicit application lifetime constants:

### Public
```text
absolute ≈ 30 days
idle     ≈ 14 days
```

### Admin
```text
absolute ≈ 8 hours
idle     ≈ 1 hour
```

Rotate sessions after login, password reset/change, recovery, or major auth changes.

## CSRF

Cookie-authenticated unsafe methods require:

```text
session-bound CSRF token
+
Origin verification
```

P0-1B uses base64url HMAC-SHA256(`CSRF_SECRET`, raw session token). GET `/auth/csrf`
returns only that HMAC with no-store; raw session tokens stay HttpOnly. Every
authenticated unsafe route checks exact Origin and `X-CSRF-Token`. Anonymous unsafe
routes check Origin only. Production requires a private secret of at least 32 bytes.
Browser mutation helpers request a fresh value each time; session rotation makes old
values invalid. CSRF never replaces PostgreSQL authentication or persists in storage.

## Auth Challenges

Email verification, password reset, and email change may use a unified challenge model.

Store only token hashes.

Tokens are random, short-lived, and single-use.

P0-1B implements only `email_verify` and `password_reset`. easyhash v1.2.0 supplies
GenerateToken, HashToken and VerifyToken; deterministic self-described SHA-256 hashes
support indexed lookup. Verification lasts 24 hours; reset lasts 30 minutes. A partial
unique index permits one outstanding identity/purpose challenge. Reissue first
invalidates the prior row and observes a 60-second cooldown. Consumption rechecks
purpose, state, expiry and active account inside the transaction.

User locking (then credential locking where needed) serializes login, challenge issuance/consumption and session
mutations; password KDFs run outside transactions. Reset/change revoke all Public/Admin
sessions and create one Public replacement atomically. Reset does not set email verified.
Reauthentication verifies the current password, replaces just the current session
and refreshes `authenticated_at`. Foreign/nonexistent session IDs return the same 404.

Challenge delivery is an explicit post-commit exception to general queued mail:
auth owns the mail interface; `internal/mail` writes private local captures. Raw tokens
never enter PostgreSQL, Redis, River, logs or normal API responses. Capture links use
fragments, browser pages immediately clear them, and Referrer-Policy is no-referrer.
Registration keeps its committed account/session even on mail failure. Local capture
is rejected in production; no production provider is included in P0-1B.

## Enumeration Resistance

Login and password-reset flows do not reveal account existence unnecessarily.

Reset requests perform bounded dummy Argon2id work and return the same 202/body for
existing, absent, OAuth-only, disabled and deleted accounts and mail failure. Delivery
failures do not serialize sender errors. Current login behavior and registration
duplicate-email behavior remain as specified in P0-1A.

Rate-limit by multiple dimensions rather than IP alone.

## Turnstile

Use selectively for:

- registration
- password reset
- repeated failed login
- suspicious Resource submission
- spam-risk reports

Do not challenge Save / Want / Have.

## Public / Admin Separation

Production deployment target (operator-provided; not a local implementation dependency):

```text
Cloudflare Access
+ mandatory MFA
+ Tap4Furry Admin Login
+ separate Admin Session
+ Tap4Furry Admin Authorization
```

Cloudflare identity is not Tap4Furry authorization.

Public sessions are never accepted as Admin sessions.

P0-1D implements password-only Admin login for active, non-deleted accounts with a
verified email identity, a password credential and a current static role. Moderator
grants AdminAccess/Moderation, editor grants AdminAccess/Editorial, admin grants all
of these plus Administration. Multiple roles are additive; no ordinary `user` row
or dynamic permission tables exist. `/me` returns only ID, email, roles and authentication
time. Unknown, wrong, unverified, roleless, OAuth-only, disabled or deleted accounts
share `ADMIN_INVALID_CREDENTIALS`; password verification/dummy work reuses easyhash.

Admin sessions occupy the existing table with `kind=admin` and `auth_method=password`.
They use 8h absolute/1h idle/5m touch and a separate Strict HttpOnly host cookie.
Every request checks canonical session/account state and current roles. User locking
serializes login/reauth with role and password mutations; KDFs run before the lock,
then credentials and eligibility are re-read before commit. A legacy CAS upgrade
changes the hash without changing the password epoch. Normal revocations stay
within their kind; password reset/change revoke every kind and replace only Public.

`ADMIN_ORIGIN` protects unsafe requests. Admin CSRF is base64url HMAC-SHA256 of its
raw session token using independent `ADMIN_CSRF_SECRET`, retrieved via Admin
`/auth/csrf` with no-store. Login needs Origin only; authenticated mutations require
both. Public CSRF, another Admin session's CSRF, and pre-rotation CSRF are rejected.
Production requires explicit private >=32-byte CSRF/throttle secrets. Password
hashes and raw session tokens never enter response DTOs or logs. CSRF is returned
only by its dedicated endpoint and never enters browser storage or logs.

Only owner `adminctl` grants/revokes static roles. A transaction advisory lock before
the User lock protects the last-active-admin invariant across concurrent operators.
Role events and final-privileged-role Admin-session revocation commit together.
Admin runtime has no role mutation privilege or route. Cloudflare identity, if
provisioned later, cannot replace these Tap4Furry role checks.

## Auth throttling (P0-1D)

Auth owns a narrow interface; `redisstore` implements atomic EVAL check/record and
subject clearing. HMAC-SHA256(`AUTH_THROTTLE_SECRET`, operation/dimension/normalized
subject) keys live under `gfp:auth:limit:`. Subject and global counters have TTLs,
saturate together and never hold raw emails, IDs, passwords, tokens, OAuth subjects
or IPs. Forwarded headers are ignored. Completed failure limits are Public login
10/subject + 500/global per 15m, Admin 5 + 100 per 15m. Registration/reset attempts
are 3 + 200 per hour. Password change/reauth verification uses 5/User + 500/global
per 15m; Admin reauth uses 5 + 100. Success clears only the subject failure bucket.

Public local auth fails open on cache outage with redacted warnings; failure events
are skipped when no healthy bounded counter admitted them. Admin login/reauth fails
closed; existing PostgreSQL sessions survive a Redis outage. Limited reset remains
the same 202/body without issuing mail, preserving enumeration resistance. Limits
are abuse controls, not permanent account lockouts. Production Turnstile/trusted
proxy decisions and deployment monitoring remain operator gates.

## Re-authentication

Require step-up re-authentication for sensitive account changes.

## Security Events

Security Events are separate from business Audit logs.

Examples:

```text
login_success
login_failure
oauth_failure
password_changed
password_reset
provider_linked
provider_unlinked
session_revoked
admin_login
challenge_failed
```

The implemented P0-1B event types are `account_registered`, `login_succeeded`,
`logout`, `email_verification_requested`, `email_verified`, `password_reset_requested`,
`password_reset_completed`, `password_changed`, `reauthenticated`, `session_revoked`
and `other_sessions_revoked`. P0-1C adds `oauth_login_succeeded`,
`oauth_identity_linked`, `oauth_identity_unlinked` and `oauth_reauthenticated`.
P0-1D adds bounded `login_failed`/`admin_login_failed`, `admin_login_succeeded`,
`admin_logout`, `admin_reauthenticated`, `admin_session_revoked`,
`admin_other_sessions_revoked`, and grant/revoke events for moderator/editor/admin.
The broader list above describes future events.
`app.security_events` allows only a generated bigint ID, user ID, historical session
ID, closed event type and timestamp. No metadata/email/IP/user-agent field exists.
Event insertion commits in the same transaction as the corresponding state change.

Do not log passwords, hashes, raw session tokens, CSRF tokens, OAuth codes/tokens, PKCE verifiers, reset tokens, cookies, or authorization headers.

## CORS

Browser API is same-origin:

```text
tap4furry.com/api/*
admin.tap4furry.com/api/*
```

Do not open broad CORS in P0.

## Proxy Trust

Go trusts forwarding/IP headers only through the controlled Cloudflare → Nginx path.

## Secrets

Use environment/Docker-secret style deployment.

No Vault requirement for P0.

## P0-1C OAuth boundary

Google identity is the verified OIDC `sub`; GitHub identity is its decimal numeric
user ID. Provider email/name/avatar is never an identity key. Known subjects keep
their User even when upstream email changes. New subjects never auto-link by email;
an existing canonical email requires explicit linking from an authenticated account.
New OAuth Users have User/Profile/email/provider identity rows and no password row.
Only verified Gmail/Workspace Google email is authoritative; other Google email
uses normal Tap4Furry verification. GitHub chooses primary verified email, then a
verified fallback, and refuses new-account creation without one.

Auth owns transactions plus provider/flow interfaces; the adapter returns validated
identity fields only. OAuth2 access/refresh/ID tokens never leave the callback adapter
or enter PostgreSQL, Redis, jobs, events, logs or frontend storage. Google signature,
issuer, audience and expiry use go-oidc; nonce and authorized party are checked too.
Both providers use S256 PKCE. Endpoint configuration is fixed, redirects are disabled
on outbound requests, and provider network operations precede database transactions.

One-use 256-bit state is stored by SHA-256 digest under `gfp:auth:oauth:flow:` for ten
minutes with NX and GETDEL. Its payload contains only provider, mode, PKCE verifier,
Google nonce, optional User/session IDs and creation time. A per-provider HttpOnly
cookie binds the callback to the initiating browser. Link/reauth additionally require
the exact initiating User/session, revalidated after the common User lock.

All existing-user auth writes serialize on the active User before other row locks.
Local login rereads its credential after that lock. OAuth login rejects flows older
than a password change/reset, preventing an in-flight callback from issuing a session
after revocation. If OAuth wins first, password reset/change revokes its session.
No credential row is required for verification, logout or session management.

Link/unlink require authentication within 15 minutes; activity never renews that
window. Link/unlink rotate cookies while preserving auth_method/authenticated_at.
Provider reauth matches an already-linked subject and rotates with fresh provider
authentication. Unlink cannot remove the last method or its current session method;
reauthenticate with a remaining method first. Unlink revokes all sessions using the
removed provider. Origin and CSRF remain mandatory for every unsafe owner operation.
OAuth callbacks use no-store/no-referrer and fixed same-origin destinations with
allowlisted error codes. Provider errors and arbitrary return URLs are never echoed.
