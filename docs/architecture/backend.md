# Tap4Furry — Backend Architecture

## Runtime

```text
Go 1.27+
Fiber v3
```

Three binaries:

```text
tap4furry-api
tap4furry-admin
tap4furry-worker
```

One Go module and one shared backend codebase.

## Call Direction

```text
Transport
   ↓
Application Use Case
   ↓
Domain Policy / State Transition
   ↓
sqlc
   ↓
pgx / PostgreSQL
```

Worker path:

```text
River Job Adapter
   ↓
Application Use Case
   ↓
Domain / sqlc
```

## Style

> Pragmatic Modular Monolith + Use-case Application Layer + Lightweight Domain Model + SQL-first persistence.

Do not mechanically build Handler / Service / Repository / DAO layers if they only rename one another.

## Transport

Fiber and generated OpenAPI types stay in Transport.

Application/Domain must not depend on:

```text
Fiber
HTTP
OpenAPI generated DTOs
River
Redis
```

## Application

Application owns user/business intentions:

```text
SubmitResource
ApproveContribution
ClaimResource
SaveResource
SetWant
CreateCollection
RestrictUser
MergeResources
```

Application owns transaction boundaries.

## Domain

Domain code is ordinary Go:

```text
struct
typed string
method
function
policy
error
```

Avoid ceremonial DDD unless real complexity justifies it.

## Commands vs Queries

### Commands
Use Application/Domain rules and explicit transactions.

### Queries
May directly use purpose-built sqlc read queries when reconstructing domain objects adds no value.

This is a CQRS mindset without CQRS infrastructure.

## Transaction Rule

```text
Handler     → never owns business transaction
Application → owns transaction
Domain      → transaction-agnostic
sqlc        → executes with provided pool/tx
```

## Repository Rule

Do not wrap every sqlc query.

Create a repository/store abstraction only when it hides meaningful persistence complexity.

## Interface Rule

Concrete types by default.

Interfaces at real boundaries:

```text
Mailer
ObjectStorage
OAuthProvider
Clock
```

Avoid ceremonial `ResourceServiceInterface` or `RepositoryInterface`.

## Cross-domain Workflows

A use case may coordinate several domains but must have one clear owner.

Avoid cyclic Application dependencies.

Add a top-level workflow/usecase package only when a real workflow requires it.

## Trust & Safety

Do not create one giant TrustSafety middleware.

Transport middleware handles transport concerns:

```text
Request ID
Recovery
Access Log
Security Headers
Body Limit
CSRF
Session resolution
basic rate limiting
```

Business policy remains Application/Domain logic.

## OpenAPI

Contract-first:

```text
contracts/openapi/public.yaml
contracts/openapi/admin.yaml
```

Generate Fiber server interfaces with `oapi-codegen`.

Manually map transport DTOs to Application commands/results.

## Errors

Domain/Application errors map to stable API error codes and HTTP semantics.

Example:

```text
ErrBudgetExceeded
→ CONTRIBUTION_BUDGET_EXCEEDED
→ HTTP 429
```

## Database Stack

```text
pgx/v5 + pgxpool
sqlc
goose SQL migrations
```

No GORM / AutoMigrate / ORM.

## Logging

Use `log/slog` structured logging.

A full observability/alerting architecture is intentionally out of scope.
