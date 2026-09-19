# Tap4Furry — Jobs & Automation

## Engine

Use **River OSS** as the PostgreSQL-backed durable job system.

River is a Go dependency plus PostgreSQL infrastructure tables, not a new external middleware service.

## Anti-lock-in Boundary

River types must not spread through product domains.

Keep River inside Jobs infrastructure.

Application code should call business-level enqueue helpers such as:

```text
EnqueueResourceMetadataRefreshTx(...)
EnqueueSourceHealthCheckTx(...)
EnqueueMailTx(...)
```

rather than manipulating generic River objects throughout the codebase.

Business truth remains in `app.*`, never in River tables.

## Worker

`tap4furry-worker` is a separate process but shares Application/Domain code.

```text
River Job Adapter
   ↓
Application Use Case
   ↓
Domain / sqlc
```

Worker does not call Public/Admin APIs over HTTP.

## Synchronous vs Async

Synchronously commit:

- canonical business state
- contribution state
- authorization result
- audit record
- durable job enqueue when required by the transaction

Async:

- email
- notification delivery
- image processing
- source health
- metadata refresh
- cleanup
- snapshots
- reconciliation

Rule:

> **Business state synchronously; side effects asynchronously.**

P0-1B has one explicit exception: authentication verification/reset challenge mail
is delivered directly after the auth transaction commits. Raw single-use tokens may
exist only in memory, private local capture and the recipient's browser/mail; they
must never be persisted in River arguments or another queue. Auth owns this mail
interface, and `internal/mail` supplies local capture only. No general mail queue or
production provider is introduced by this exception. Other business mail continues
to follow the asynchronous rule above.

## Transactional Enqueue

Use River transaction enqueue when the job must exist if and only if the business transaction commits.

No separate Outbox in P0/P1.

Add a real Outbox only if future architecture needs durable event fan-out across independent services.

## Job Contracts

Stable and versioned:

```text
resource.metadata_refresh.v1
source.health_check.v1
media.process.v1
mail.send.v1
maintenance.cleanup_sessions.v1
analytics.daily_snapshot.v1
```

Use ID-first payloads.

Do not serialize large canonical business snapshots into jobs.

## Semantics

Assume **at-least-once** execution.

Every job must be retry-safe.

Use:

- current-state checks
- UNIQUE constraints
- UPSERT
- version comparison
- idempotency keys where needed

River Unique Jobs are an optimization, not the source of correctness.

## Queues

P0:

```text
default
mail
network
media
maintenance
```

Avoid one queue per domain.

## Retry

### Transient
Retry:

- network timeout
- HTTP 429
- 502/503
- temporary provider/database problems

### Permanent
Stop retrying:

- invalid media
- unsupported MIME
- permanently invalid input

### No Longer Needed
Return success:

- Resource deleted
- Source disabled
- Poll already closed
- media already ready

## Timeouts

Each job type has an explicit timeout and respects context cancellation.

## Periodic Work

Use River recurring jobs for application automation.

Prefer scheduler + fan-out for bulk work.

Example:

```text
SourceHealthScheduler
→ select due Sources
→ enqueue many source.health_check.v1 jobs
```

Avoid one huge `CheckAllSources` job.

## Reconciliation

Important automation must also be discoverable from canonical state.

Examples:

```text
active listing past expires_at
active poll past end_at
stale source
pending media
stale metadata
```

Periodic reconciliation repairs missed work.

> **Events make the system fast; reconciliation makes the system reliable.**

## Infrastructure Automation

Do not put in River:

- PostgreSQL backup
- server filesystem maintenance
- Docker image cleanup
- infrastructure certificate/DNS operations

Those belong to host/CI infrastructure automation.

## River Risk Management

- pin River version
- review changelog before upgrades
- run integration tests
- keep River migrations separate
- do not require River Pro for core architecture
- keep business truth out of River tables
- preserve reconciliation escape paths

## Future NATS Path

NATS is not required for P0/P1.

If future independent services need durable multi-consumer event fan-out, revisit:

```text
Transactional Outbox
+
NATS JetStream
```
