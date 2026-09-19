# P0-1A implementation and validation

Validated on 2026-09-08, Windows with Go 1.27.1, Node 24.15.0 and pnpm 10.11.0.
Baseline: `1142eb1` on `dev`. The only initial untracked file was the user-supplied
P0-1A specification; it is included unchanged with this implementation.

## Implemented

- Migration `00002_identity_local_auth.sql` adds `app.users`, `user_profiles`,
  `auth_identities`, `password_credentials` and `sessions`. Migration 1 is unchanged.
  sqlc reads the migrations and focused auth/identity/session queries directly.
- Concrete `internal/auth` and `internal/identity` application code owns transactions
  and policy. It imports no Fiber, generated HTTP DTOs, Redis or River.
- Public endpoints: POST `/auth/register`, `/auth/login`, `/auth/logout`, GET `/me`,
  PATCH `/me/profile`, GET `/users/{handle}`. Public/Admin health remain intact;
  Admin OpenAPI and generated client/server files are unchanged.
- Public Web `/register`, `/login`, `/account` use anonymous Astro shells and React
  islands through the Public client facade. Account state is fetched client-side.
  Nullable PATCH fields distinguish omission, clearing and replacement.

## Password and session security

Inspected easyhash v1.2.0 (`4eea85ee2512dfb3296cefca38038c2310431b92`), including
README, usage guide, policy and token implementation. New passwords explicitly use
`Hash(password, WithArgon2id())`. `VerifyAndUpgrade` receives an Argon2id policy with
the library's default Argon2 parameters: 64 MiB, time cost 3, parallelism 2.
Bcrypt upgrades use CAS; concurrent successful upgrades retry verification once.
Equal/stronger current Argon2id hashes remain unchanged. Unknown identities take a
dummy Argon2id verification path; login errors do not distinguish them from wrong
passwords. Passwords are neither trimmed nor Unicode-normalized.

Inspected the installed Go standard library `src/uuid/uuid.go`: `uuid.NewV7()`
returns `uuid.UUID`. Business IDs use this API, including in the Linux image build.
No third-party application ID generator was added.

Sessions use 32 cryptographically random bytes, unpadded base64url browser encoding,
and a 32-byte SHA-256 lookup hash in PostgreSQL. No raw token enters database or
JSON responses. Current names after BRAND-0 are Secure/HttpOnly
`__Host-tap4furry_session` in production, Path=/, SameSite=Lax, no Domain, and
`tap4furry_session` in explicit development/test. These names were updated after
the original P0-1A validation; the cookie security properties are unchanged.
Absolute/idle lifetimes are 30/14 days; touches are throttled to 10 minutes and
bounded by absolute expiry. Revoked, expired, disabled and deleted cases fail.
Exact `PUBLIC_ORIGIN` protects unsafe routes; production requires HTTPS.

## Commands executed

| Command | Result |
| --- | --- |
| `git branch --show-current`, `git status --short`, `git log` | Confirmed `dev`, existing P0-0, preserved supplied specification |
| `go version`, `go env GOROOT`, `go list std` and installed UUID source inspection | Go 1.27.1 standard UUIDv7 confirmed |
| `git ls-remote https://github.com/gofurry/easyhash.git HEAD refs/heads/main refs/tags/v*` | Verified current v1.2.0 source revision |
| `go -C server mod download -json github.com/gofurry/easyhash@v1.2.0` | Inspected the selected published module |
| `pnpm check` before implementation | PASS: P0-0 baseline |
| `go -C server tool sqlc generate -f db/sqlc.yaml` | PASS after qualifying an initially ambiguous session query column |
| `go -C server get github.com/gofurry/easyhash@v1.2.0` | Added the used password dependency |
| `go -C server get github.com/oapi-codegen/runtime@latest` | Resolved/pinned v1.7.0, required by generated handle parameter binding |
| `go -C server mod tidy` | PASS; no unrelated selected dependency upgrades |
| `pnpm install --frozen-lockfile` | PASS; workspace lockfile unchanged |
| `pnpm generate` | PASS: Go OpenAPI, Orval and sqlc generated outputs |
| `pnpm lint`, `pnpm typecheck`, `go -C server test ./...` | PASS; final Astro diagnostics have no errors/warnings/hints |
| `CI=true GFP_DISPOSABLE_INFRA=1 pnpm integration:ci` | PASS from fresh disposable PostgreSQL/Redis, migrations 1→2, repeat-up, role/driver/River smoke and auth integration |
| `CI=true GFP_DISPOSABLE_INFRA=1 GFP_AUTH_INTEGRATION=1 go -C server test -count=1 -run TestIntegration ./internal/transport/public` | PASS after request-deadline integration |
| `CI=true GFP_DISPOSABLE_INFRA=1 GFP_AUTH_INTEGRATION=1 go -C server test -race -count=1 -timeout=3m ./internal/auth ./internal/identity ./internal/transport/public` | PASS, including database concurrency/HTTP tests |
| `pnpm migrate:dev` | PASS, after disposable acceptance; prepared migrator applied migration 2 to `gfp_dev` |
| `pnpm smoke:dev` | PASS: real pgx, Redis and River checks |
| `pnpm smoke:auth:dev` | PASS: real API role register/login/me/profile/public-profile/logout and owner-scoped temporary identity cleanup |
| `pnpm dev:web` with API pointed only at disposable `gfp_ci` | Browser registration, profile editing, logout, login and persisted profile confirmed |
| `pnpm build:images` | PASS: Server, Web, Admin and PostgreSQL images built locally using the isolated WSL Docker engine; no publishing |
| Final `pnpm check` | PASS: repository audit, generation drift, lint, types, native Node/Go tests and application builds |
| Final `pnpm generate`, `pnpm check:generated` | PASS: byte-identical files and unchanged generated file membership |
| `git diff --check`, complete working/staged diff inspection, `pnpm audit:repository` | PASS for implementation; no secret/local-config/Tailnet tracking or forbidden scope |
| `git diff --cached --check -- . ':(exclude)docs/implementation/p0-1a-identity-local-auth.md'` | PASS; the supplied specification retains its four intentional Markdown hard line breaks |

Environment assignments above use shell-neutral notation; Windows runs set the
corresponding `$env:` variables. CI uses fixed disposable loopback endpoints and
never loads the private launch files. Image names retain the foundation's local
`p0-0-local` tags and contain the current implementation.

The first integration run exposed an incorrect test expectation: PostgreSQL 18
reports `23001` for an explicit `ON DELETE RESTRICT` violation. The database correctly
rejected deletion. The expectation was fixed, then the complete integration was
rerun successfully against freshly recreated disposable containers. No schema or
architecture workaround was used.

The unfiltered staged whitespace check flags only four existing two-space Markdown
line breaks in the supplied specification header. These were preserved intentionally;
the implementation-only staged whitespace check passes.

## Coverage and limits

Tests exercise email/password/handle validation, Unicode/space handling, Argon2id,
bcrypt upgrades, no downgrade, token encoding/hash, cookies in all environments,
exact Origin, stable error mapping, fresh schema, least-privilege grants, database
CHECK/FK/UNIQUE authority, concurrent duplicate email/handle races, rollback integrity,
stale hash CAS, all HTTP flows, public field allowlists, disabled/deleted accounts,
revocation, both expiry limits and touch throttling. Node tests check generated
client status/options, nullable PATCH and empty logout handling.

No material specification deviations. Optional public profile HTML was omitted;
the required public profile API is implemented. No future product domains or P0-1
placeholder tables were introduced. Private files were never printed, edited,
deleted or tracked; shared Infra had no SSH, role changes or Redis ACL changes.

P0-1B, P0-1C and P0-1D remain. Email verification/recovery, OAuth, full synchronizer
CSRF, rate limits, security events, Admin authentication and roles are not complete;
P0-1A must not be represented as production-auth complete.

The final local commit and clean `dev` status are reported in the task response.
No push, main merge, Git tag or release is part of this work.
