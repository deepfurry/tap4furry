# P0-0 validation record

Historical evidence: local environment names below record the original execution;
current product/build contracts use Tap4Furry. BRAND-0 does not rename that environment.

Executed locally on 2026-09-08 on `dev`. Initial HEAD was
`7c122c3` and the working tree was clean. Existing product/architecture/engineering
documents were preserved. The supplied implementation specification was copied to
its requested repository path, with only newline/trailing-whitespace normalization.

## Implemented boundaries

The pnpm monorepo has Public Astro Node SSR with React 19 islands and a prerendered
foundation page, Admin React 19/Vite with TanStack Router/Query, generated client
facades, and shared SCSS design tokens. Go API/Admin/Worker share one Go 1.27 module.
Only health HTTP behavior is implemented; there are no product domains or tables.

OpenAPI generates Fiber v3 transport and Orval fetch clients; Goose migrations and
queries generate pgx/sqlc. Outputs are committed. Migration code lives in dedicated
database/Jobs subpackages and is absent from runtime binary dependency graphs.
River types stay in Jobs; PostgreSQL is canonical and Redis keys use `gfp:`.

## Commands and observed results

Commands below describe the final successful executions. Go commands ran from
`server`, or equivalently used `go -C server`. Root scripts show their underlying
verification commands in `scripts/tasks.mjs`.

| Command / check | Observed result |
| --- | --- |
| `git branch --show-current`, `git status --short`, `git log -5 --oneline --decorate` | `dev`, clean initial baseline; existing design docs only |
| `go version` | Go 1.27.1, Windows amd64 |
| `node --version`, `pnpm --version` | Node 24.15.0, pnpm 10.11.0 |
| `git check-ignore -- server/env/*.local .local/readonly.env` (explicit file list) | All five real inputs ignored and not tracked |
| `pnpm install` | Dependency graph and lockfile created |
| `pnpm install --frozen-lockfile` | Passed |
| `go generate ./...`, `pnpm generate` | Fiber v3, sqlc and both Orval clients generated successfully |
| `pnpm check:generated` | Regeneration byte-identical, including file membership |
| `git diff --exit-code --` generated output directories | No unstaged generated drift against reviewed staged outputs |
| `pnpm lint` | ESLint, `gofmt -l` verification and `go vet ./...` passed |
| `pnpm typecheck` | Root Orval config, API-client, Admin TypeScript and Astro diagnostics passed |
| `pnpm test` | Four native Node client tests and all Go tests passed |
| `go test ./...` | Config, secret-safe errors/logs, namespace, both health transports and HTTP shutdown tests passed |
| `pnpm build` | Astro SSR/prerender, Vite SPA and three independent Go binary builds passed |
| `go build -o bin/... ./cmd/api`, `./cmd/admin`, `./cmd/worker` | All passed via the root build script |
| `pnpm check` | Complete repository verification passed |
| `go mod verify` | All modules verified |
| `go list -deps ./cmd/api ./cmd/admin ./cmd/worker` | No Goose, River migrator, pgx stdlib bridge or prohibited runtime integration |
| `go run ./cmd/preflight -service ...` with the private launcher | All four prepared PostgreSQL identities verified, no values exposed |
| `pnpm migrate:dev` | Goose foundation, official River migrations and owned-object Worker grants passed; repeated up passed |
| `pnpm smoke:dev` | All four pgx identities/sqlc queries; Redis PING and unique TTL-bound SET/GET/DEL; actual River execution/completion/cleanup passed |
| `CI=true GFP_DISPOSABLE_INFRA=1 pnpm integration:ci` (process env on Windows) | Fresh disposable PostgreSQL 18 / Redis 8: restricted role/ACL fixtures, Goose up, official River up twice and all driver smoke checks passed |
| `pnpm build:images` | Final Server, Web, Admin and PostgreSQL Docker images built successfully |
| Native HTTP checks using Node `fetch` | Both real-config APIs returned correct live/ready responses; Public production SSR/island markup/prerender and Admin asset entry passed |
| Container HTTP checks using Node `fetch` | Public/Admin live/ready, Public SSR and Admin SPA/history fallback passed against disposable Infra |
| `docker stop --timeout ...` / `docker inspect` | API/Admin/Worker exited 0; final Web image exited promptly with expected SIGTERM code 143; Admin static runtime exited 0 |
| `pnpm audit:repository` | Private-value/Tailnet scan, ignored-file rules, staged secret scan, domain/dependency and runtime boundary checks passed |
| `git diff --check`, `git diff --cached --check` and complete diff review | Passed; no unrelated design-doc edits or private configuration included |

`pnpm test` uses Node's built-in runner rather than a frontend test framework. Go
health tests cover PostgreSQL failure, Redis degradation, shutdown readiness, and
dependency-free liveness for both transports. Container SIGTERM checks exercised the
actual Linux runtime binaries in addition to the context-driven HTTP unit test.

## Database and real development Infra

The prepared migrator has schema creation capability but not CREATE on `public`.
Goose bookkeeping therefore uses `app.goose_db_version`; the explicit migrator
ensures that namespace before running Goose. `pg_trgm` was already enabled and
`vector` absent in shared development; fresh disposable migration also proved the
extension bootstrap. No product tables were created.

Official River v0.47.0 migrations 1–7 ran in `river`. Worker received runtime object
permissions without schema CREATE/ownership or migration-history writes. No shared
cluster role, Redis ACL, server, Tailscale or container configuration was modified.
There was no shared Infra SSH.

Real `gfp_api`, `gfp_admin`, `gfp_worker` and `gfp_migrator` connections all passed.
Redis checks authenticated as `gfp_runtime` and removed their own temporary keys.
River's infrastructure probe was enqueued, executed, completed and removed under
the Worker role; normal startup does not schedule it.

## Adaptations and resolved verification failures

- The requested spec path was absent; the user-supplied attachment supplied it.
- Prepared private files lacked HTTP_ADDR/RIVER_SCHEMA. The first strict preflight
  failed before a connection. Developer launchers now supply only missing non-secret
  defaults, preserving every existing file and explicit setting.
- River rejects an empty worker registry. The spec-authorized infrastructure probe
  is the only registered job and exists to test real execution.
- oapi-codegen 2.8.0's documented `fiber-v3-server` path generated and compiled
  directly; no framework substitution or custom server generator was used.
- Orval 8 uses `formatter`; this was corrected and root tool configuration added to
  TypeScript checks. Generated files were regenerated, never patched by hand.
- Initial formatting/path invocations were corrected; final lint and diff checks
  passed. The copied spec's four trailing Markdown spaces were normalized.
- Docker was initially unavailable. An isolated temporary WSL distribution and
  Docker engine were created locally. Initial direct registry access timed out;
  a temporary relay restricted to that WSL adapter/client used the workstation's
  existing proxy. No shared Infra or repository runtime architecture was changed.
  Test containers were removed and the daemon/relay/distro were stopped. Automated
  approval review rejected deletion of the temporary distro/cache with only
  `blocked by policy`; the stopped `GoFurry-P0-0-Verify` distro and ignored
  `.cache/p0-0-infra` files remain locally and are not committed.
- The initial Web container needed a forced stop when Node ran as PID 1. Adding
  tini fixed signal delivery; the final image was rebuilt and retested successfully.
- TypeScript 6 and compatible ESLint packages were selected because the installed
  Astro checker and Node 24.15 toolchain support them. All direct versions are pinned.
  Tool/test transitive checksums in go.sum do not imply an observability deployment;
  runtime dependency checks confirm no such integration.

## CI and remaining work

The committed workflow uses disposable PostgreSQL/Redis, immutable third-party
Action SHAs, generation drift checks, native tests and image builds. Its commands
were exercised locally. GitHub Actions itself was not triggered because no push
was authorized. No production deployment, image publishing, tag or release occurred.

P0-1 authentication and product-domain work remain deliberately deferred. There are
no unresolved P0-0 acceptance blockers. The final local commit SHA is reported in
the task response, avoiding a self-referential SHA in this committed record.
