# Changelog

## Unreleased

- Add MAIL-0 production verification/reset email through the official Resend Go
  SDK, preserving the Auth-owned ChallengeMailer and post-commit failure semantics.
- Require explicit production Resend configuration with validated From/Reply-To;
  preserve private local capture and disabled development/test delivery.
- Send text/minimal HTML with encoded fragment tokens and a five-second budget;
  flatten provider errors, with no retry/outbox/queue or database migration.
- Add offline config/adapter/security tests, API-key/recipient secret auditing and
  opt-in `smoke:mail:resend:dev` using only ignored private input and synthetic mail.

- Complete BRAND-0 repository branding for Tap4Furry (`deepfurry/tap4furry`),
  including product UI/mail/docs, the Go module and `@tap4furry/*` workspace scope.
- Rename Public/Admin/OAuth cookies, development-only secret defaults, runtime
  binaries and four `tap4furry-*:local` images; use `tap4furry.com` and
  `admin.tap4furry.com` for future production origin/callback contracts.
- Retain `gfp_*` PostgreSQL identifiers, the `gfp:` Redis namespace, applied
  migrations 1–5, private local inputs and the external easyhash dependency.
  No infrastructure/data migration, deployment or compatibility aliases are added.

- Complete P0-1D static moderator/editor/admin roles and operator-only bootstrap,
  with last-active-admin protection and transactional final-role session revocation.
- Add independent password-only Admin authentication, current-role authorization,
  Strict HttpOnly cookies, separate Origin/CSRF, 8h/1h sessions and session controls.
- Revoke both Public and Admin sessions atomically on password reset/change,
  retaining exactly one replacement Public session.
- Add HMAC-private Redis subject/global auth throttles and bounded failure events;
  Public local auth fails open on cache outage, Admin login/reauth fail closed.
- Add migration 5 with minimal Admin grants, generated Admin API/client, protected
  React Admin workspace on port 5173, disposable security/concurrency tests and a
  real-development Admin smoke with narrowly scoped temporary-fixture cleanup.
- Document remaining human/provider and production deployment gates; no new
  dependency, dynamic RBAC, application MFA or product domain is introduced.

- Add P0-1C Google OIDC and GitHub OAuth Web Flow with S256 PKCE, browser-bound
  one-use Redis state and fixed callbacks; provider tokens remain transient.
- Resolve accounts by verified provider subject, reject email auto-linking, and
  create OAuth-only accounts with an email identity and no password credential.
- Add explicit auth-method listing/linking, provider reauthentication and safe
  unlinking with 15-minute freshness, session rotation and provider-session revocation.
- Add migration 4 for OAuth events, session-method constraints and provider
  uniqueness; serialize authentication mutations on User rows, including OAuth-only
  accounts and OAuth callbacks racing password reset/change. Preserve migrations 1–3.
- Extend generated Public contracts/clients and account UI for provider sign-in,
  private method metadata, safe callback errors and remaining-method reauthentication.
- Add signed fake OIDC/provider, disposable Redis, security and concurrency tests,
  plus a redacted real-development OAuth configuration/capability smoke command.

- Add P0-1B migration 3 for single-use auth challenges and transactional security
  events, with minimal API privileges and no new dependency or product domain.
- Require session-bound HMAC CSRF and exact Origin for authenticated unsafe Public
  requests; keep raw session tokens HttpOnly and discard CSRF on rotation.
- Add email verification, password reset/change, reauthentication and owned public
  session listing/revocation, with atomic password/challenge/session/event updates.
- Add consumer-owned post-commit mail delivery and private local capture; production
  rejects local capture and no production mail provider is included.
- Add recovery/verification pages and account security controls using generated
  clients, memory-only fragment handling and no-referrer responses.
- Extend disposable security, privacy, concurrency and rollback tests, and the real
  development auth smoke with private capture and narrowly scoped fixture cleanup.

- Add P0-1A accounts, public profiles, email identities, password credentials and
  PostgreSQL public sessions through migration 2, with explicit runtime grants.
- Add standard-library UUIDv7 IDs, easyhash v1.2.0 explicit Argon2id hashing,
  dummy verification and race-safe login hash upgrades.
- Add Public register/login/logout/me/profile APIs, opaque HttpOnly cookies,
  bounded session resolution and exact-origin protection for unsafe requests.
- Add anonymous Astro login/register/account shells with React forms using the
  generated Public client; public profile responses exclude all private fields.
- Add disposable database/concurrency/HTTP/privacy tests and a real-development
  authentication smoke with narrowly scoped temporary-identity cleanup.
- Extend Agent contracts and secret/boundary audits for the implemented identity
  scope. OAuth and final auth hardening remain later phases.

- Establish P0-0 Agent context, engineering contracts and implementation guidance.
- Add pnpm workspace with Astro/React public SSR, React/Vite admin SPA, shared SCSS
  design tokens, and Public/Admin generated fetch-client facades.
- Add Go API/Admin/Worker runtimes, environment validation, structured redacted
  logging, bounded health checks and graceful shutdown.
- Add separate OpenAPI contracts, Fiber v3/oapi-codegen and Orval generation,
  Goose foundation and pgx/sqlc readiness query.
- Isolate official River migrations and Worker runtime in the `river` schema,
  with an explicit infrastructure probe and least-privilege object grants.
- Add real driver smoke commands, generated drift and secret/boundary checks,
  disposable-infrastructure CI, and four runtime Dockerfile definitions.
