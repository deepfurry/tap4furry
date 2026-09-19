# Tap4Furry — Data Architecture

## Core Principle

> **Database enforces invariants; application performs behavior.**

PostgreSQL is the only canonical database.

## PostgreSQL Runtime

Baseline:

```text
PostgreSQL 18.x
```

Tap4Furry maintains a reproducible PostgreSQL runtime image:

```text
deploy/postgres/
└── Dockerfile
```

The image defines PostgreSQL and any required external extensions.
PGDATA remains independent from the container image.

## UUID

Go is locked to **1.27+**.

Generate business IDs in Go using standard-library UUIDv7 support:

```text
uuid.NewV7()
```

Database:

```sql
id uuid PRIMARY KEY
```

Do not rely on database defaults/triggers for business ID generation.

### ID Strategy

Business entities:

```text
UUIDv7
```

Pure relationship tables:

```text
composite primary keys
```

High-volume internal append-only records:

```text
BIGINT identity where appropriate
```

## Schema

Use one application schema:

```text
app.*
```

Do not mirror every Go domain as a separate PostgreSQL schema.

River maintains separate infrastructure tables/schema.

## Naming

```text
tables  → plural_snake_case
columns → snake_case
```

Use `*_state` for lifecycle states and `*_at` for timestamps.

## Time

Use:

```text
timestamptz
UTC
```

No trigger-maintained `updated_at`; writes update it explicitly.

## States

Prefer:

```text
text + CHECK
+
Go typed strings
```

Avoid PostgreSQL ENUM for evolving product states.

## Minimal Database Magic

P0:

- no application triggers;
- no stored-procedure business logic;
- no hidden business cascades;
- FK default NO ACTION / RESTRICT;
- explicit Application transactions perform state changes.

## Soft Delete

Important long-lived entities support soft delete:

```text
deleted_at timestamptz NULL
```

Business state and soft deletion are separate dimensions.

Relationship tables such as resource tags, saves, wants, haves, and collection items normally use physical row deletion.

Physical purge is a separate maintenance workflow, not implicit cascade behavior.

## Multilingual Data

Multilingual support is first-class from P0.

Use localization tables, not `summary_en` columns or multilingual JSONB blobs.

Example:

```text
resources
resource_localizations
```

`resource_localizations`:

```text
resource_id
locale
name
summary
description
PRIMARY KEY(resource_id, locale)
```

Use BCP 47-style locale values:

```text
en
zh-Hans
zh-Hant
ja
```

The same pattern may be used for Tag / Category / Organization / platform-curated Collection content.

UGC may simply store original language and original text.

## Resource Core

Keep the core Resource table generic.

Do not continuously add resource-type-specific columns.

No generic EAV system in P0.

If a resource type later needs stable dedicated fields, add an explicit extension table.

## ResourceSource

Resource and Source are independent entities.

Restricting/removing a Source does not remove the Resource knowledge entity.

## External IDs

Use a dedicated table:

```text
resource_external_ids
```

Conceptually:

```text
resource_id
namespace
external_id
UNIQUE(namespace, external_id)
```

Useful for Steam, GitHub, itch, import matching, and duplicate detection.

## Relations

Use explicit relation tables and real foreign keys.

Avoid polymorphic foreign keys when referential integrity matters.

Prefer separate Resource↔User and Resource↔Organization relation tables.

## JSONB

Use JSONB only for flexible/historical/external payloads:

- Contribution patches
- Audit before/after
- external metadata snapshots
- provider-specific metadata
- evidence metadata
- limited job/support payloads

Do not use JSONB for canonical tags, languages, sources, permissions, roles, translations, or content rating.

## Contribution History

Contribution is the revision/change envelope for public knowledge.

Canonical entities remain relational.

## Concurrency

Important entities may use:

```text
version bigint
```

for optimistic concurrency.

High-impact transitions may use:

```sql
SELECT ... FOR UPDATE
```

Default isolation:

```text
READ COMMITTED
```

## Constraints

Enforce important invariants in both:

```text
Go Application/Domain
+
PostgreSQL PK/FK/UNIQUE/CHECK
```

## Indexing

P0 performance strategy:

- PK
- UNIQUE
- FK indexes
- real filter/sort indexes
- partial indexes where useful
- PostgreSQL FTS
- `pg_trgm`

Do not preemptively add projection systems, partitioning, materialized views, or read replicas.

## Search

P0 uses normalized tables + indexes + FTS + `pg_trgm`.

No dedicated search projection is required initially.

## pgvector

Development infrastructure may include pgvector, but P0 does not depend on it.

Future semantic-search rollout:

```text
1. Update Tap4Furry PostgreSQL runtime image to include pgvector.
2. Validate in staging.
3. Reuse compatible PGDATA.
4. Goose: CREATE EXTENSION vector.
5. Add embedding tables/indexes.
6. Deploy API/Worker support.
7. Backfill embeddings asynchronously.
```

PostgreSQL major upgrades remain separate database-maintenance events.

## MongoDB / NATS

MongoDB is not part of current architecture.

NATS is reserved for a future distributed multi-consumer event architecture.
