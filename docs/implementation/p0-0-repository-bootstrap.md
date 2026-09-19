# P0-0 — Repository Bootstrap & Engineering Foundation

**Repository:** `deepfurry/tap4furry`
**Target branch:** `dev`
**Release snapshot branch:** `main`
**Phase:** P0-0
**Status:** Implementation specification

## 1. Goal

P0-0 establishes the executable engineering foundation for Tap4Furry before any product-domain implementation begins.

At completion the repository must have:

- Agent-oriented repository contracts and workflow guidance;
- a pnpm monorepo;
- Astro + React public web foundation;
- React + Vite admin foundation;
- one Go module with `api`, `admin`, and `worker` binaries;
- separate Public/Admin OpenAPI contracts;
- oapi-codegen + Orval generation;
- PostgreSQL + pgx + sqlc + goose foundations;
- Redis connectivity using the `gfp:*` namespace;
- River isolated as PostgreSQL-backed job infrastructure;
- generated-code drift checks;
- CI using disposable infrastructure;
- buildable container images;
- real driver-level connectivity checks against the prepared development Infra.

P0-0 is **not** a product-feature phase.

Do not implement Resource, Identity, Search, Collections, Exchange, Discussions, Polls, Trust, moderation workflows, or other product domains.

---

## 2. Execution Contract

### 2.1 Branches

```text
dev
└── current development/integration branch

main
└── stable release snapshot branch
```

For this task:

- work from local `dev`;
- do not work directly on `main`;
- do not merge `dev` into `main`;
- do not tag or release;
- do not push unless the user explicitly asks;
- finish with coherent local commit(s) on `dev`.

Before editing:

```bash
git branch --show-current
git status --short
git log -5 --oneline --decorate
```

If unrelated user changes exist, preserve them. Do not reset, discard, overwrite, or silently absorb them.

### 2.2 Audit before editing

Before implementation:

1. inspect the actual repository;
2. confirm `dev`;
3. inspect existing design docs;
4. verify the ignored local environment files exist without printing their values;
5. establish the current build/check baseline.

If live repository evidence materially conflicts with this specification, surface the conflict instead of forcing the plan.

### 2.3 Shared Infra boundary

Ordinary P0-0 work must **not** SSH into the shared development Infra server.

Do not:

- SSH to `gofurry-dev-infra`;
- modify its Docker services;
- modify PostgreSQL cluster roles;
- modify Redis ACLs;
- touch databases outside `gfp_dev`;
- change Tailscale/server/security-group configuration.

P0-0 **may** use the prepared `gfp_migrator` credentials to run repository-owned Goose and River migrations against `gfp_dev`.

If a cluster/server-level privilege is required, stop and report the exact missing capability.

### 2.4 Secrets

Ignored local files may exist:

```text
server/env/api.local
server/env/admin.local
server/env/worker.local
server/env/migrator.local
.local/readonly.env
```

They contain real private development credentials.

Rules:

- never print their complete contents;
- never commit them;
- never replace them with examples;
- never remove their ignore rules;
- never expose real DSNs, passwords, Tailnet addresses, or tokens in docs, logs, test output, generated files, or final reports.

---

## 3. Context Discipline

Do **not** preload the whole `docs/` tree.

### Required reading

Read in order:

1. this implementation specification;
2. `docs/product/PRODUCT.md`;
3. `docs/architecture/ARCHITECTURE.md`;
4. `docs/architecture/repository-layout.md`;
5. `docs/architecture/backend.md`;
6. `docs/architecture/frontend.md`;
7. `docs/architecture/data.md`;
8. `docs/engineering/tech-stack.md`;
9. `docs/engineering/jobs-automation.md`;
10. `docs/engineering/ci-cd.md`.

Also inspect the reference repository:

```text
https://github.com/deepfurry/agent-engineering-template
```

At minimum:

```text
AGENTS.md
.agents/architecture.md
.agents/playbook.md
contracts/
```

Use it as a pattern reference only. Do not mechanically copy it.

### Optional reading

Read only if directly relevant:

```text
docs/architecture/security.md
docs/architecture/deployment.md
docs/engineering/operations.md
docs/engineering/backup-restore.md
```

### Do not preload for P0-0

Do not read these merely for completeness:

```text
docs/product/domain-model.md
docs/product/discovery.md
docs/product/content-policy.md
docs/product/trust-safety.md
docs/product/information-architecture.md
docs/product/roadmap.md
```

Consult them only if implementation truly crosses into their domain.

The new `AGENTS.md` must include the same context-discipline rule: read the active implementation spec first and only load related detailed docs.

---

## 4. Hard Technology Decisions

Do not reopen these choices unless a real compatibility blocker is proven.

### Frontend

```text
Node.js 24 LTS
pnpm workspace

Public:
Astro
React 19 Islands
Astro Node SSR
selective prerender

Admin:
React 19
Vite SPA
TanStack Router
TanStack Query v5

Styling:
Tailwind CSS v4 → layout
SCSS / SCSS Modules → visual styling
```

Base UI is selected for future headless primitives, but do not add it if P0-0 has no meaningful primitive using it.

Also defer Zustand, React Hook Form, Zod, Lucide, and an i18n framework until actually used.

### Backend

```text
Go 1.27+
Fiber v3
OpenAPI spec-first
oapi-codegen
pgx/v5 + pgxpool
sqlc
goose
go-redis/v9
River OSS
log/slog
```

No GORM, ORM, or AutoMigrate.

### Password hashing

Selected for P0-1:

```text
github.com/gofurry/easyhash
→ Argon2id
```

P0-0 does not implement password auth, so do not add `easyhash` unless real P0-0 code uses it.

Also defer OAuth, S3, email, and Turnstile dependencies.

### Data

```text
PostgreSQL 18.x
PostgreSQL = canonical source of truth
Redis = ephemeral state/cache/counters
pg_trgm = enabled
pgvector/vector = not enabled for P0-0
```

Do not introduce:

```text
MongoDB
NATS
Kafka
RabbitMQ
Elasticsearch/OpenSearch
ClickHouse
vector database services
```

### Jobs

River is the P0/P1 durable-job implementation.

River types must remain inside the Jobs infrastructure boundary.

### Observability

Only implement:

```text
structured slog logging
/health/live
/health/ready
```

Do not add a full observability or alerting stack.

---

## 5. Prepared Development Infrastructure

### PostgreSQL

Database:

```text
gfp_dev
```

Prepared schema:

```text
app
```

Enabled:

```text
pg_trgm
```

Not enabled:

```text
vector
```

Prepared roles:

```text
gfp_migrator
gfp_api
gfp_admin
gfp_worker
gfp_readonly
```

Purpose:

```text
gfp_migrator → Goose + River migration
gfp_api      → Public API runtime
gfp_admin    → Admin API runtime
gfp_worker   → Worker + River runtime
gfp_readonly → operator inspection only
```

The pre-created development schema is not the repository source of truth. Migrations must bootstrap a fresh compatible database too.

### Redis

Prepared user:

```text
gfp_runtime
```

Allowed keys:

```text
gfp:*
```

Application configuration must use:

```text
REDIS_KEY_PREFIX=gfp:
```

### Connectivity

TCP connectivity from the developer workstation through Tailscale has already been verified.

P0-0 must perform protocol-level verification with:

```text
pgx/v5
go-redis/v9
```

Local `psql` and `redis-cli` are not required development dependencies.

---

## 6. Application Configuration Contract

Runtime binaries read process environment variables. They must not know Infra-secret storage variable names.

### Public API

```text
APP_ENV
HTTP_ADDR
DATABASE_URL
REDIS_URL
REDIS_KEY_PREFIX
```

### Admin API

```text
APP_ENV
HTTP_ADDR
DATABASE_URL
REDIS_URL
REDIS_KEY_PREFIX
```

### Worker

```text
APP_ENV
DATABASE_URL
REDIS_URL
REDIS_KEY_PREFIX
RIVER_SCHEMA
```

### Migrator

```text
APP_ENV
DATABASE_URL
RIVER_SCHEMA
```

Use:

```text
RIVER_SCHEMA=river
```

Rules:

- use a thin standard-library-oriented config package;
- do not use Viper;
- validate required values on startup;
- errors/logs must not expose secrets;
- runtime binaries read process env only;
- `.local` files are developer launch inputs, not production runtime behavior.

Create public templates:

```text
server/env/api.example
server/env/admin.example
server/env/worker.example
server/env/migrator.example
```

Preserve existing ignored `.local` files.

---

## 7. Target Repository Shape

Approximate target:

```text
tap4furry/
│
├── .agents/
│   ├── architecture.md
│   └── playbook.md
├── .github/
│   └── workflows/
│       └── ci.yml
├── apps/
│   ├── web/
│   └── admin/
├── packages/
│   ├── api-client/
│   ├── design/
│   └── react-ui/          # only if it contains a real shared primitive
├── server/
│   ├── cmd/
│   │   ├── api/
│   │   ├── admin/
│   │   └── worker/
│   ├── internal/
│   │   ├── config/
│   │   ├── transport/
│   │   │   ├── public/
│   │   │   └── admin/
│   │   ├── database/
│   │   │   └── sqlc/
│   │   ├── redisstore/
│   │   ├── jobs/
│   │   └── runtime/
│   ├── db/
│   │   ├── migrations/
│   │   ├── queries/
│   │   └── sqlc.yaml
│   ├── codegen/
│   │   └── oapi/
│   │       ├── public.yaml
│   │       └── admin.yaml
│   ├── env/
│   │   ├── api.example
│   │   ├── admin.example
│   │   ├── worker.example
│   │   └── migrator.example
│   ├── go.mod
│   └── go.sum
├── contracts/
│   ├── architecture.md
│   ├── database.md
│   ├── development.md
│   ├── generated-code.md
│   └── openapi/
│       ├── public.yaml
│       └── admin.yaml
├── deploy/
│   ├── docker/
│   ├── postgres/
│   │   └── Dockerfile
│   └── README.md
├── docs/
│   ├── product/
│   ├── architecture/
│   ├── engineering/
│   ├── implementation/
│   │   └── p0-0-repository-bootstrap.md
│   ├── decisions/
│   │   └── README.md
│   └── development.md
├── AGENTS.md
├── CHANGELOG.md
├── README.md
├── package.json
├── pnpm-workspace.yaml
└── pnpm-lock.yaml
```

Do not create empty future domains such as `auth`, `identity`, `resource`, `taxonomy`, `collection`, `contribution`, `discovery`, `exchange`, `discussion`, `poll`, `trust`, `moderation`, `notification`, or `analytics`.

Create a domain only when its phase begins.

P0-0 must create and use:

```text
packages/api-client
packages/design
```

Create `packages/react-ui` only when it contains a real shared primitive used by both apps.

Do not create an empty `packages/i18n`.

---

## 8. Agent Engineering Foundation

Adapt `deepfurry/agent-engineering-template`.

### AGENTS.md

Keep concise. Include:

- repository purpose;
- `dev` / `main` branch contract;
- context-reading discipline;
- repository map;
- source-of-truth hierarchy;
- generated-code rule;
- migration rule;
- secret rule;
- shared Infra boundary;
- repository-wide verification command;
- final diff/report expectations.

Agents must not merge/push/tag/release unless explicitly instructed.

### `.agents/architecture.md`

Compact mental model and invariants:

```text
apps/web / apps/admin
        ↓
generated API clients
        ↓
Public/Admin Go transport
        ↓
future Application/Domain
        ↓
sqlc/pgx
        ↓
PostgreSQL

worker
  ↓
jobs adapter
  ↓
shared application/domain
```

Also capture:

- Public/Admin are security/transport boundaries;
- Worker does not call Public/Admin over HTTP;
- PostgreSQL canonical, Redis ephemeral;
- River isolated;
- generated code ownership;
- no speculative future domain tree.

### `.agents/playbook.md`

Adapt the procedural model from the reference repo.

Do not use `make check`.

Repository-wide final verification should be:

```text
pnpm check
```

or an equivalent single cross-platform root command.

### Contracts

Create concise:

```text
contracts/architecture.md
contracts/database.md
contracts/generated-code.md
contracts/development.md
```

`contracts/database.md` must state:

- PostgreSQL source of truth;
- Goose-only application migrations;
- no AutoMigrate/ORM;
- sqlc generated ownership;
- `app` schema;
- `river` River infrastructure schema;
- `pg_trgm` required;
- `vector` prohibited in P0-0;
- released migrations are not modified in place;
- development role purposes.

`contracts/generated-code.md` must encode:

```text
contracts/openapi/*.yaml → Go transport + TS clients
server/db/migrations + server/db/queries → sqlc
```

Generated outputs are committed and never manually edited.

`contracts/development.md` must encode local secrets, branch policy, shared Infra boundaries, and the rule that CI never uses shared Infra.

### ADR index

Create only `docs/decisions/README.md` explaining when a future ADR is required.

Do not retroactively create many ADRs.

---

## 9. Root Workspace

Use pnpm workspace.

Create:

```text
package.json
pnpm-workspace.yaml
pnpm-lock.yaml
```

Workspace patterns:

```yaml
packages:
  - apps/*
  - packages/*
```

Expose:

```text
pnpm generate
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm check
```

`pnpm check` is the preferred final verification entry point.

Root scripts orchestrate real tools; they do not implement generator logic.

Do not use Turbo/Nx.

Pin pnpm in `packageManager`.

Declare Node 24 as the supported major.

Use cross-platform commands; do not make the default workflow Bash-only.

---

## 10. Public Web

Create `apps/web` with:

```text
Astro
React 19
Astro Node adapter
Tailwind CSS v4
SCSS
strict TypeScript
```

Requirements:

- Node SSR output;
- React integration;
- no global React app/provider tree;
- minimal page only;
- shared design tokens from `packages/design`;
- no product UI implementation;
- if an API-health island exists, use the generated Public client;
- development `/api/*` may proxy to local Public API;
- no user-specific SSR.

---

## 11. Admin Web

Create `apps/admin` with:

```text
React 19
Vite
strict TypeScript
TanStack Router
TanStack Query v5
Tailwind CSS v4
SCSS / SCSS Modules
```

Requirements:

- SPA;
- minimal route tree;
- no dashboard/product implementation;
- shared design tokens;
- generated Admin API client for simple health request;
- development `/api/*` proxy to Admin API;
- no Zustand/form stack without a real need.

---

## 12. Shared Frontend Packages

### `packages/api-client`

OpenAPI is authoritative.

Generate:

```text
src/generated/public/
src/generated/admin/
```

Expose stable facades:

```text
@tap4furry/api-client/public
@tap4furry/api-client/admin
```

Apps must not import deep generated paths.

Use Orval and prefer simple fetch clients in P0-0.

### `packages/design`

Keep small:

```text
tokens.scss
typography.scss
mixins.scss
global.scss
```

Tailwind is layout-oriented; SCSS owns visual styling.

### `packages/react-ui`

Only create if at least one genuine shared primitive is used by both apps.

Do not build a design-system catalog.

---

## 13. Go Backend

Create one module:

```text
server/go.mod
```

Module path:

```text
github.com/deepfurry/tap4furry/server
```

Go 1.27 minimum.

Do not create `go.work`.

### Binaries

```text
server/cmd/api
server/cmd/admin
server/cmd/worker
```

All must build independently.

`cmd/*` owns composition, config, dependency construction, startup, signals, and graceful shutdown.

Reusable code belongs in `internal/`.

### Config

Use a thin env-based config package.

No Viper.

### PostgreSQL

Use `pgx/v5 + pgxpool`.

Explicitly construct/close pools.

Do not introduce `database/sql` merely for symmetry.

### Redis

Use `go-redis/v9`.

Provide a small key-boundary/helper ensuring project-created keys begin with `REDIS_KEY_PREFIX`.

Do not build a generic cache framework.

### Logging

Use `log/slog`.

Minimally include:

```text
service
environment
```

### Graceful shutdown

API/Admin/Worker must handle process termination with bounded cleanup.

---

## 14. OpenAPI & Codegen

Authoritative specs:

```text
contracts/openapi/public.yaml
contracts/openapi/admin.yaml
```

P0-0 endpoints only:

```text
GET /health/live
GET /health/ready
```

Keep specs separate.

### Health semantics

`live`:

- process-alive only;
- no dependency fan-out.

`ready`:

- PostgreSQL is hard readiness dependency;
- Redis may be reported degraded without necessarily failing readiness.

### Go

Generate with oapi-codegen into:

```text
server/internal/transport/public/generated/
server/internal/transport/admin/generated/
```

Keep hand-written handlers outside generated directories.

Generated DTOs are transport-only.

Use the current documented Fiber v3-compatible oapi-codegen path.

If current oapi-codegen cannot correctly support Fiber v3, stop and report evidence. Do not silently switch framework or abandon spec-first generation.

### TypeScript

Generate both clients with Orval.

---

## 15. Go Tooling & Generation

Use the Go 1.27 module tool mechanism where supported.

Pin Go generator tooling such as:

```text
oapi-codegen
sqlc
goose
River CLI/migrator tooling as appropriate
```

Prefer `go generate ./...` for Go-side outputs.

Root:

```text
pnpm generate
```

must cover:

```text
OpenAPI → Go
OpenAPI → TypeScript
migrations/queries → sqlc
```

Generated outputs are committed and never manually edited.

Generation must be idempotent.

---

## 16. PostgreSQL Foundation

Layout:

```text
server/db/migrations/
server/db/queries/
server/db/sqlc.yaml
```

Goose migrations are schema source of truth.

Do not maintain duplicate `schema.sql`.

### Foundation migration

Create one minimal migration safe for the existing `gfp_dev` and a fresh compatible DB.

Responsibilities may include:

```text
CREATE SCHEMA IF NOT EXISTS app;
CREATE SCHEMA IF NOT EXISTS river;
CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;
```

Goose may own namespace creation, but River owns the objects inside `river` through River's own migrations.

Do not enable `vector`.

Do not create product tables.

### sqlc

Use Goose migrations as schema input.

If a query is needed to prove the pipeline, use a legitimate non-product system/readiness query rather than a fake business table.

Generate to:

```text
server/internal/database/sqlc/
```

### Startup

Application startup must never run Goose automatically.

Migrations remain explicit.

---

## 17. River Foundation

Use River OSS with pgx.

Use:

```text
RIVER_SCHEMA=river
```

River migrations are separate from Goose migrations.

Expected order:

```text
Goose foundation
→ river schema exists
→ official River migrate-up in schema river
```

Use River's official custom-schema mechanism. Do not rewrite River migration SQL.

River imports belong in:

```text
server/internal/jobs/
```

Do not expose River types to future product domains.

Do not invent future Resource/Mail/Media jobs in P0-0.

If River can run with an empty worker registry, use that. If the current version requires registration to start, use only the smallest legitimate infrastructure mechanism and document the reason.

### Permissions

River migrations use `gfp_migrator`.

Grant `gfp_worker` only the runtime permissions required for River objects in `river`.

Use permissions owned by application/migrator-created objects; never request superuser.

Public/Admin transactional enqueue permissions are deferred until those processes actually enqueue jobs.

---

## 18. Real Development Infra Smoke

Required acceptance gate.

Using ignored `.local` files and actual drivers, verify:

```text
api.local      → gfp_api / gfp_dev
admin.local    → gfp_admin / gfp_dev
worker.local   → gfp_worker / gfp_dev
migrator.local → gfp_migrator / gfp_dev
```

Do not print DSNs/passwords.

Apply Goose foundation with `migrator.local`.

Apply River migrations with `migrator.local`.

Verify Worker River runtime access with `worker.local`.

Redis via `gfp_runtime`:

- PING succeeds;
- temporary `gfp:*` set/get/delete succeeds;
- key creation uses configured prefix;
- do not widen ACL.

A safe foreign-namespace negative test is acceptable.

No SSH.

A small developer connectivity-check command/script is acceptable if it has a clear purpose and never leaks secrets.

Do not make local `psql`/`redis-cli` required.

---

## 19. CI

Use GitHub Actions.

Trigger at least:

```text
pull_request
push: dev
push: main
```

CI must never access shared Tailscale Infra or `.local` files.

Use disposable PostgreSQL 18 and Redis 8.

### Go

```text
gofmt verification
go vet ./...
go test ./...
go build ./cmd/api
go build ./cmd/admin
go build ./cmd/worker
```

### Frontend

```text
pnpm install --frozen-lockfile
pnpm lint
pnpm typecheck
pnpm test
pnpm build
```

Do not add a heavyweight frontend test framework solely to make `test` non-empty.

### Generated code

```text
pnpm generate
git diff --exit-code
```

### Database

A fresh disposable PostgreSQL must prove:

```text
Goose up from empty compatible DB
River migration in schema river
application DB smoke
```

Do not emulate PostgreSQL with SQLite.

### Redis

Use disposable Redis where integration tests need it.

### Docker

Build all P0-0 images. PR CI does not need to push.

Pin third-party Actions to immutable commit SHAs where practical.

Do not add production secrets or auto-deployment.

---

## 20. Docker / Deployment Skeleton

P0-0 creates buildable artifacts, not production deployment.

### Server

One multi-stage image containing:

```text
tap4furry-api
tap4furry-admin
tap4furry-worker
```

Runtime command chooses the binary.

Use non-root runtime where practical.

### Public Web

Build Astro Node SSR into a production runtime image.

### Admin

Build Vite static assets into a minimal static runtime image.

### PostgreSQL Runtime

Create:

```text
deploy/postgres/Dockerfile
```

Represent the future Tap4Furry-controlled PostgreSQL runtime.

P0-0 requires PostgreSQL 18.x and `pg_trgm` availability.

Do not add pgvector.

### No fake production

Do not provision/configure:

- VPS;
- Cloudflare;
- certificates;
- production credentials;
- public deployment.

A concise `deploy/README.md` is sufficient until real deployment work begins.

---

## 21. Documentation

### Root README

Keep concise:

- product summary;
- development status;
- repository map;
- quick start;
- branch model;
- verification commands;
- links to design/implementation docs;
- AGPL-3.0-only.

### `docs/development.md`

Document:

- Go/Node/pnpm requirements;
- local env paths;
- secret rules;
- running API/Admin/Worker;
- running Web/Admin;
- migrations;
- generation;
- checks;
- shared Infra is accessed only through private local config;
- CI uses disposable infrastructure.

Never include live Tailnet IPs or credentials.

---

## 22. `.gitignore`

Preserve:

```gitignore
/server/env/*.local
/.local/
```

Expand for actual outputs such as `node_modules`, build output, coverage, and editor/temp files.

Do not ignore generated source that is intentionally committed.

---

## 23. Install Only Used Dependencies

Expected P0-0 dependencies may include:

```text
Astro / React 19 / Vite
Astro Node adapter
Tailwind v4 / Sass
TanStack Router / Query where actually used
Fiber v3
pgx/v5
go-redis/v9
River
oapi-codegen
sqlc
goose
Orval
```

Defer until actual use:

```text
github.com/gofurry/easyhash
golang.org/x/oauth2
AWS SDK v2 S3
Turnstile
mail provider SDK
Zustand
React Hook Form
Zod
Base UI if unused
Lucide if unused
i18n framework
pgvector
MongoDB client
NATS client
```

Do not add dependency placeholders.

---

## 24. P0-0 Prohibitions

P0-0 MUST NOT:

- implement authentication or User/Profile models;
- implement Resource/ResourceSource;
- implement taxonomy;
- implement Search/Discovery;
- implement Collections/Save/Want/Have;
- implement Contribution/Review;
- implement Exchange;
- implement Discussions/Polls;
- implement Trust/Risk/Budget/Distribution;
- implement moderation;
- create product tables;
- enable pgvector;
- introduce MongoDB or NATS;
- add external search infrastructure;
- introduce microservices/gRPC/Kubernetes;
- add full observability/alerting;
- provision production Infra;
- SSH to shared dev Infra;
- alter shared PostgreSQL roles or Redis ACLs;
- hand-edit generated code;
- create empty future-domain trees;
- add unused dependencies;
- merge/push `main`;
- tag/release.

---

## 25. Implementation Order

1. **Audit**
   - branch/status/history;
   - docs;
   - ignored local env presence;
   - toolchain versions;
   - Agent template.

2. **Agent engineering**
   - `AGENTS.md`;
   - `.agents/*`;
   - contracts;
   - `CHANGELOG.md`;
   - ADR index.

3. **Root workspace**
   - pnpm workspace;
   - version contract;
   - root scripts.

4. **Frontend**
   - Public;
   - Admin;
   - used shared packages.

5. **Go runtime**
   - module;
   - config;
   - DB;
   - Redis;
   - shutdown/logging.

6. **OpenAPI/codegen**
   - health specs;
   - oapi-codegen;
   - Orval;
   - root generation.

7. **PostgreSQL/sqlc**
   - foundation migration;
   - sqlc config/query;
   - generated output.

8. **River**
   - `river` schema;
   - official migrations;
   - jobs boundary;
   - worker runtime permissions.

9. **Health transport**
   - Public live/ready;
   - Admin live/ready.

10. **Worker**
    - foundation startup/shutdown.

11. **Real dev Infra smoke**
    - pgx;
    - Redis;
    - Goose;
    - River.

12. **CI**
    - disposable PG/Redis;
    - drift checks;
    - builds/tests.

13. **Container builds**

14. **README/development docs**

15. **Full verification and diff review**

16. **Local commit on dev; do not push**

---

## 26. Acceptance Criteria

### Repository

```text
branch = dev
no secret files tracked
no unrelated changes lost
Agent contracts present
design docs preserved
```

### Toolchain

```text
Go 1.27+
Node 24
pnpm workspace
one server/go.mod
no go.work
```

### Generation

```text
pnpm generate                         PASS
second generation produces no drift  PASS
generated Go/TS/sqlc source committed
```

### Go

```text
gofmt verification      PASS
go vet ./...            PASS
go test ./...           PASS
go build ./cmd/api      PASS
go build ./cmd/admin    PASS
go build ./cmd/worker   PASS
```

### Frontend

```text
pnpm lint       PASS
pnpm typecheck  PASS
pnpm test       PASS
pnpm build      PASS
```

### Database

```text
fresh Goose migration       PASS
pg_trgm available           PASS
vector not enabled          PASS
sqlc generation             PASS
River migration in river    PASS
```

### Real dev Infra

```text
gfp_api pgx          PASS
gfp_admin pgx        PASS
gfp_worker pgx       PASS
gfp_migrator pgx     PASS
gfp_runtime Redis    PASS
gfp:* smoke key      PASS
River worker access  PASS
```

### HTTP

Both Public/Admin APIs:

```text
/health/live
/health/ready
```

behave according to contract.

### Docker

```text
server image                PASS
web image                   PASS
admin image                 PASS
Tap4Furry PostgreSQL runtime  PASS
```

### Forbidden dependency audit

No P0-0 dependency on:

```text
MongoDB
NATS
pgvector/vector
GORM/ORM
Elasticsearch/OpenSearch
Kubernetes
observability/alerting stacks
```

---

## 27. Stop Conditions

Stop and report evidence rather than inventing a new architecture if:

1. `dev` contains substantial unexpected implementation conflicting with this plan;
2. local env files are missing and real Infra verification cannot be performed;
3. a secret is tracked by Git;
4. current oapi-codegen cannot correctly support Fiber v3;
5. River custom-schema/runtime needs cannot be satisfied with prepared DB privileges;
6. a required action needs shared Infra SSH/admin access;
7. a selected dependency materially conflicts with Go 1.27 or Node 24;
8. CI can only pass by using shared development Infra;
9. the task appears to require a prohibited component.

For non-consequential details, use the simplest solution consistent with repository contracts.

---

## 28. Final Verification

Before completion:

1. run repository-wide `pnpm check`;
2. run generation again and verify no drift;
3. inspect `git status --short`;
4. inspect `git diff --stat`;
5. inspect the full diff;
6. verify no `.local`/secret files are tracked;
7. verify no live Tailnet address is tracked;
8. verify no product-domain implementation slipped in;
9. verify no prohibited dependency was introduced;
10. update `CHANGELOG.md` under `Unreleased`.

---

## 29. Commit Contract

After verification:

- make one coherent commit or a small number of logical local commits;
- remain on `dev`;
- do not push.

Recommended message:

```text
chore: bootstrap platform engineering foundation
```

Do not amend unrelated pre-existing commits.

---

## 30. Final Codex Report

Return:

### Implemented
What was created and the resulting repository shape.

### Architecture
Important boundaries established.

### Generated contracts
OpenAPI/oapi-codegen/Orval/sqlc status.

### Database / River
Migration status and schema boundaries.

### Development Infra
Which real driver-level smoke checks passed.

Never expose DSNs, passwords, Tailnet addresses, or credentials.

### Verification
Every command actually run and result.

### Deviations
Any intentional difference from this spec and why.

### Remaining work
Only legitimate P0-1 follow-up or unresolved blockers.

### Git

```text
branch
final commit SHA
git status
```

Do not claim checks passed unless they were actually executed.
