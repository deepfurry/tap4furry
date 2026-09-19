# P0-2A — Resource Domain & Schema

**Repository:** `deepfurry/tap4furry`
**Target branch:** `dev`
**Baseline:** `66c8abd556262b0a480be50d0b88905dd1ca86d5`
**Baseline commit:** `feat: add production transactional email`
**Baseline CI:** GitHub Actions Run #7 — success
**Prerequisites:** P0-0, P0-1, Human Acceptance, BRAND-0, MAIL-0 complete
**Status:** Codex implementation specification
**Date:** 2026-09-19

---

## 1. Goal

P0-2A establishes the canonical **Resource Graph data foundation**.

This phase answers:

> What is a Resource, how is it classified and localized, where can it be found, how can resources relate to one another, and what invariants make that knowledge trustworthy?

P0-2A is deliberately a **schema + domain-contract phase**.

It does **not** expose the Resource Graph over HTTP and does not build Resource/Admin UI.

After P0-2A the repository should have a stable relational model ready for:

```text
P0-2B Public Resource Read Surface
P0-2C Admin Resource Curation
P0-3  Contribution & Review
P0-4  Search & Discovery
```

---

## 2. Product Principles

Preserve:

```text
Resource First
Web First
Read First
Resource ≠ Source ≠ Distribution
Contribution over Administration
Complexity Must Be Earned
Ecosystem over Surveillance
```

The Resource Graph stores canonical knowledge.

It is not:

```text
a file-hosting model
a social graph
a recommendation graph
a payment/order model
a generic EAV model
a dumping ground for provider JSON
```

---

## 3. Scope

P0-2A introduces exactly these Resource Core tables:

```text
app.categories
app.category_localizations

app.tags
app.tag_localizations

app.resources
app.resource_localizations

app.resource_tags
app.resource_sources
app.resource_relations
app.resource_external_ids
```

Total:

```text
10 tables
```

It also introduces:

```text
server/internal/taxonomy
server/internal/resource

server/db/queries/taxonomy.sql
server/db/queries/resource.sql

Migration 00006
Resource Core database/domain tests
Disposable migration/privilege acceptance
pnpm smoke:resource:dev
```

No Category rows are seeded in migration 00006.

---

## 4. Explicit Non-goals

Do not implement:

```text
Public Resource HTTP API
Admin Resource CRUD HTTP API
OpenAPI Resource contracts
Astro /resources pages
Admin Resource UI

Contribution
Review workflow
Search
Discovery ranking
Collections
Save / Want / Have
Exchange
Claims
Organization
Discussion
Poll
Notification
Moderation
Trust score
Recommendation
Semantic/vector search
Media/R2
Resource import/synchronization
Source availability probing
```

Do not introduce:

```text
PostgreSQL ENUM
JSONB canonical fields
EAV
application triggers
stored-procedure business logic
hidden cascades
RLS
new Redis data
River jobs
new Worker Resource privileges
```

---

## 5. Existing Architecture Contracts

Continue:

```text
PostgreSQL canonical
Redis ephemeral
Goose schema source of truth
pgx/v5 + sqlc
UUIDv7 generated in Go
TEXT + CHECK for product states
timestamptz / UTC
explicit transactions
no trigger-maintained updated_at
FK RESTRICT / no business cascades
```

`00001` through `00005` are immutable.

Do not change their contents or hashes.

New schema goes only into:

```text
server/db/migrations/00006_resource_core.sql
```

---

## 6. Domain Package Boundary

P0-2A officially opens:

```text
server/internal/taxonomy/
server/internal/resource/
```

Update `scripts/audit-repository.mjs` so these two are no longer considered forbidden future-domain scaffolds.

Continue forbidding premature packages:

```text
collection
contribution
discovery
exchange
discussion
poll
trust
moderation
notification
analytics
```

Do not create placeholder directories for later phases.

---

## 7. Taxonomy Model

### Category

Category answers:

> What kind of Resource is this primarily?

A Resource has exactly one Category.

First product taxonomy for later curation:

```text
game
creative-work
tool
platform
community
event
knowledge
marketplace
service
other
```

P0-2A does not seed these rows.

### Tag

Tag answers:

> What characteristics does this Resource have?

Tags are many-to-many with Resource and remain flat in P0-2A.

Do not add:

```text
parent_id
tag hierarchy
aliases
implications
groups
weights
confidence
primary tag
magic IDs
```

---

## 8. Slug Contract

Category, Tag and Resource use stable ASCII slugs.

Canonical syntax:

```regex
^[a-z0-9]+(?:-[a-z0-9]+)*$
```

Lengths:

```text
Category.slug  1..64
Tag.slug       1..64
Resource.slug  1..80
```

Rules:

```text
lowercase ASCII
kebab-case
no Unicode slug
no automatic transliteration
no automatic collision suffix
no DB trigger generation
```

Category/Tag slug is immutable after creation.

Resource slug may change before first publication and becomes immutable through normal application behavior once `published_at` is non-null.

---

## 9. Localization Contract

Localization is first-class from P0.

Use separate localization tables.

Do not use:

```text
name_en
summary_zh
multilingual JSONB
```

Locales are BCP 47-style:

```text
en
zh-Hans
zh-Hant
ja
```

Go code must use:

```text
golang.org/x/text/language
```

for parse/canonicalization.

Examples:

```text
EN      → en
zh-hans → zh-Hans
JA      → ja
```

DB performs defensive shape validation only.

Every Category, Tag and Resource has:

```text
default_locale
```

Application transaction invariant:

```text
default_locale must have a matching localization row
```

Do not add a circular FK from parent entity to localization.

Creation must atomically create:

```text
entity
+
default localization
```

Changing default locale requires target localization to exist.

Deleting the current default localization must be rejected by Application logic.

Resource localization fields:

```text
name        required
summary     nullable
description nullable
```

Empty optional localized text normalizes to NULL.

Future P0-2B reads use field-level fallback:

```text
requested locale field
→ if NULL/missing use default locale field
```

---

## 10. Resource Model

Resource is a long-lived knowledge entity, not a URL/download location.

Publication state:

```text
draft
pending
published
restricted
removed
```

Lifecycle:

```text
active
inactive
discontinued
delisted
archived
unknown
```

Content rating:

```text
general
mature
explicit
```

These dimensions are independent.

`content_rating` has no default.

`publication_state` defaults to `draft`.

`lifecycle` defaults to `unknown`.

---

## 11. Resource Revision Model

`resources.version` is the canonical Resource revision anchor.

Definition:

> Version represents a revision of the Resource node and Resource-owned canonical knowledge, not the number of SQL statements executed.

Schema:

```text
version bigint NOT NULL
CHECK version >= 1
```

Creation:

```text
version = 1
```

No DB default.

A single logical transaction increments version exactly once for each affected Resource.

Resource-owned canonical changes include:

```text
Resource fields
ResourceLocalization
ResourceTag membership
ResourceSource
ResourceRelation
ResourceExternalID
publication/lifecycle/rating
default_locale
soft delete
```

Independent Category/Tag entity edits do not cascade version bumps.

Relation mutation affects both endpoints, so both Resource versions increment.

P0-2A provides minimal sqlc/CAS groundwork, not complete CRUD.

---

## 12. Soft Deletion

Use `deleted_at` on:

```text
categories
tags
resources
```

Semantics:

```text
state=retired
→ historically valid taxonomy, no longer for new bindings

publication_state=removed
→ Resource knowledge still exists but is not public

deleted_at != NULL
→ canonical entity is soft-deleted and excluded from normal business views
```

Do not add `deleted_at` to:

```text
resource_tags
resource_relations
resource_external_ids
```

Those relationship rows use physical deletion.

`resource_sources` uses `availability_state=removed`.

No runtime hard DELETE privilege is granted for Category, Tag, Resource or ResourceSource.

---

## 13. ResourceSource Model

ResourceSource means:

> An external location where a Resource can be accessed, found or learned about.

Source Type:

```text
official
store
archive
mirror
community
external
unknown
```

Availability State:

```text
active
unavailable
broken
removed
restricted
```

Rights Status:

```text
unknown
creator_provided
confirmed
disputed
rights_review
removed_by_request
```

Do not introduce legal labels such as `legal`, `illegal`, `pirated`.

---

## 14. ResourceSource URL Contract

Store a conservatively normalized URL.

Application normalization:

```text
trim outer whitespace
http/https only
host required
no userinfo
lowercase scheme
lowercase hostname
remove default :80 / :443
remove fragment
preserve path semantics
preserve query semantics
do not fetch URL
do not follow redirects
do not remove arbitrary query parameters
```

Maximum length:

```text
2048
```

Uniqueness:

```text
UNIQUE(resource_id, url)
```

The same URL may be associated with different Resources.

A Resource may have zero or one primary Source:

```text
UNIQUE(resource_id) WHERE is_primary
```

---

## 15. ResourceRelation Model

Store one canonical Resource→Resource edge.

Initial types:

```text
part_of
successor_of
derived_from
related_to
```

Do not add:

```text
predecessor_of
contains
mirror_of
official_site_of
store_of
created_by
owned_by
duplicate_of
alternative_to
```

Inverse semantics are derived by reads.

`related_to` is symmetric and canonicalized:

```text
smaller UUID → source_resource_id
larger UUID  → target_resource_id
```

Directed relations preserve direction.

Reject self-relations.

Cycle prevention for:

```text
part_of
successor_of
derived_from
```

is an Application concern for P0-2C, not a trigger in P0-2A.

---

## 16. ResourceExternalID Model

Examples:

```text
steam_app   / 123456
github_repo / owner/repo
itch_game   / ...
```

Rules:

```text
namespace lowercase stable identifier
external_id opaque string
UNIQUE(namespace, external_id)
```

Do not add provider metadata JSON, sync state or snapshots.

---

## 17. Migration 00006 — Exact Schema Shape

Create:

```text
server/db/migrations/00006_resource_core.sql
```

### app.categories

Columns:

```text
id              uuid PK
slug            text UNIQUE NOT NULL
default_locale  text NOT NULL
state           text NOT NULL DEFAULT active
created_at      timestamptz NOT NULL
updated_at      timestamptz NOT NULL
deleted_at      timestamptz NULL
```

Checks:

```text
slug length 1..64 and slug regex
locale length 2..64 and defensive locale-shape regex
state IN ('active','retired')
updated_at >= created_at
deleted_at IS NULL OR deleted_at >= created_at
```

Defensive locale regex:

```regex
^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)*$
```

### app.category_localizations

Columns:

```text
category_id
locale
name
description
created_at
updated_at
```

Constraints:

```text
FK category_id → categories(id) ON DELETE RESTRICT
PK(category_id, locale)

name trimmed and length 1..80
description NULL or nonblank, <=500
updated_at >= created_at
```

Case-insensitive locale unique index:

```text
UNIQUE(category_id, lower(locale))
```

### app.tags

Same identity/state model as Category.

Slug length:

```text
1..64
```

### app.tag_localizations

Columns:

```text
tag_id
locale
name
description
created_at
updated_at
```

Constraints parallel Category localization.

Name length:

```text
1..80
```

Case-insensitive locale uniqueness:

```text
UNIQUE(tag_id, lower(locale))
```

### app.resources

Columns:

```text
id
slug
default_locale
category_id
publication_state
lifecycle
content_rating
version
published_at
created_at
updated_at
deleted_at
```

Constraints:

```text
slug UNIQUE, length 1..80, canonical slug regex

category_id
→ categories(id)
ON DELETE RESTRICT

publication_state IN (
  draft,
  pending,
  published,
  restricted,
  removed
)

lifecycle IN (
  active,
  inactive,
  discontinued,
  delisted,
  archived,
  unknown
)

content_rating IN (
  general,
  mature,
  explicit
)

version >= 1

updated_at >= created_at
published_at IS NULL OR published_at >= created_at
deleted_at IS NULL OR deleted_at >= created_at

publication_state <> published
OR published_at IS NOT NULL
```

Defaults:

```text
publication_state = draft
lifecycle = unknown
```

No default for:

```text
content_rating
version
```

Index:

```text
resources_category_id_idx(category_id)
```

Do not add search/FTS/trigram indexes now.

### app.resource_localizations

Columns:

```text
resource_id
locale
name
summary
description
created_at
updated_at
```

Constraints:

```text
FK resource_id → resources(id) ON DELETE RESTRICT
PK(resource_id, locale)

name trimmed, length 1..160

summary NULL
or trimmed, length 1..500

description NULL
or nonblank

updated_at >= created_at
```

Case-insensitive locale uniqueness:

```text
UNIQUE(resource_id, lower(locale))
```

Do not impose a small VARCHAR-style limit on description.

### app.resource_tags

Columns:

```text
resource_id
tag_id
```

Constraints:

```text
PK(resource_id, tag_id)

resource_id → resources(id) RESTRICT
tag_id      → tags(id) RESTRICT
```

Index:

```text
resource_tags_tag_id_idx(tag_id)
```

No metadata columns.

### app.resource_sources

Columns:

```text
id
resource_id
url
label
source_type
availability_state
rights_status
is_primary
created_at
updated_at
```

Constraints:

```text
id uuid PK
resource_id → resources(id) RESTRICT

url trimmed
url length 1..2048
defensive http/https scheme check
no fragment
UNIQUE(resource_id, url)

label NULL or trimmed length 1..80

source_type IN (
  official,
  store,
  archive,
  mirror,
  community,
  external,
  unknown
)

availability_state IN (
  active,
  unavailable,
  broken,
  removed,
  restricted
)

rights_status IN (
  unknown,
  creator_provided,
  confirmed,
  disputed,
  rights_review,
  removed_by_request
)

availability_state DEFAULT active
rights_status DEFAULT unknown
is_primary DEFAULT false

updated_at >= created_at
```

Partial unique index:

```text
UNIQUE(resource_id) WHERE is_primary
```

Do not add redundant plain resource_id index if `(resource_id,url)` already covers it.

### app.resource_relations

Columns:

```text
id
source_resource_id
target_resource_id
relation_type
created_at
```

Constraints:

```text
id uuid PK

source_resource_id → resources(id) RESTRICT
target_resource_id → resources(id) RESTRICT

relation_type IN (
  part_of,
  successor_of,
  derived_from,
  related_to
)

UNIQUE(
  source_resource_id,
  target_resource_id,
  relation_type
)

source_resource_id <> target_resource_id

relation_type <> related_to
OR source_resource_id < target_resource_id
```

Incoming index:

```text
resource_relations_target_idx(
  target_resource_id,
  relation_type
)
```

Do not duplicate the outgoing prefix already provided by the unique index.

### app.resource_external_ids

Columns:

```text
resource_id
namespace
external_id
created_at
```

Constraints:

```text
resource_id → resources(id) RESTRICT

namespace lowercase
length 1..64
syntax:
^[a-z0-9]+(?:[._-][a-z0-9]+)*$

external_id trimmed
length 1..512

PK(resource_id, namespace, external_id)
UNIQUE(namespace, external_id)
```

No UUID is needed for this pure identity mapping.

---

## 18. Migration Down Order

Down is for disposable/recovery testing only.

Drop in this order:

```text
resource_external_ids
resource_relations
resource_sources
resource_tags
resource_localizations

resources

tag_localizations
tags

category_localizations
categories
```

Never run `00006 Down` on shared `gfp_dev`.

Do not expose a casual shared-development down command.

---

## 19. Database Privileges

Immediately after creating the new tables:

```text
REVOKE ALL
FROM PUBLIC,
     gfp_api,
     gfp_admin,
     gfp_worker,
     gfp_readonly
```

for all new Resource Core tables.

Then grant explicitly.

### gfp_api

```text
SELECT only
```

on all 10 tables.

No canonical Resource Graph DML.

### gfp_admin

Grant SELECT on all Resource Core tables.

Categories:

```text
INSERT
UPDATE(default_locale, state, updated_at, deleted_at)
```

No UPDATE(slug), no DELETE.

Category localizations:

```text
INSERT
DELETE
UPDATE(name, description, updated_at)
```

Tags:

```text
INSERT
UPDATE(default_locale, state, updated_at, deleted_at)
```

No UPDATE(slug), no DELETE.

Tag localizations:

```text
INSERT
DELETE
UPDATE(name, description, updated_at)
```

Resources:

```text
INSERT

UPDATE(
  slug,
  default_locale,
  category_id,
  publication_state,
  lifecycle,
  content_rating,
  version,
  published_at,
  updated_at,
  deleted_at
)
```

No hard DELETE.

Resource localizations:

```text
INSERT
DELETE
UPDATE(name, summary, description, updated_at)
```

Resource tags:

```text
INSERT
DELETE
```

Resource sources:

```text
INSERT
UPDATE(
  url,
  label,
  source_type,
  availability_state,
  rights_status,
  is_primary,
  updated_at
)
```

No hard DELETE.

Resource relations:

```text
INSERT
DELETE
```

Resource external IDs:

```text
INSERT
DELETE
```

### gfp_worker

No Resource Core privilege in P0-2A, including SELECT.

### gfp_readonly

```text
SELECT all Resource Core tables
```

No DML.

---

## 20. Product Role / Capability Contract

Do not add roles or dynamic permissions.

Existing roles:

```text
moderator
editor
admin
```

Existing capabilities:

```text
AdminAccess
Moderation
Editorial
Administration
```

P0-2 contract:

```text
editor
→ normal canonical Resource Graph editing

moderator
→ no canonical Resource Graph write

admin
→ editorial + high-impact governance changes
```

Future P0-2C:

Editorial can handle ordinary knowledge edits.

Administration is required for:

```text
retire Category/Tag
Resource restricted
Resource removed
soft-delete Category/Tag/Resource
rights_status mutation
```

P0-2A records this contract but does not build Admin CRUD HTTP APIs.

Remember:

```text
gfp_admin DB role != Tap4Furry role=admin
```

---

## 21. Domain Code

### server/internal/taxonomy

Implement ordinary Go types/functions for:

```text
CategoryState
slug validation
Locale parsing/canonicalization
localized text normalization
```

States:

```text
active
retired
```

Do not add ceremonial service/repository interfaces.

### server/internal/resource

Implement domain primitives for:

```text
PublicationState
Lifecycle
ContentRating
SourceType
SourceAvailabilityState
SourceRightsStatus
RelationType

Resource slug validation
Source URL normalization
Relation canonicalization
basic invariant validation
```

`related_to` canonicalizes UUID order.

Directed relations preserve direction.

Reject self relation.

Do not implement full Resource CRUD application use cases.

---

## 22. URL Normalization Tests

Verify at least:

```text
HTTPS://Example.COM:443/path?q=1#fragment
→ https://example.com/path?q=1

http://Example.COM:80/
→ http://example.com/

https://example.com/path?a=1&b=2
→ query preserved

userinfo URL
→ reject

ftp://
→ reject

missing host
→ reject

>2048 normalized URL
→ reject
```

Do not network-fetch input URLs.

---

## 23. sqlc Foundation

Add:

```text
server/db/queries/taxonomy.sql
server/db/queries/resource.sql
```

Only add queries needed by P0-2A and near-future mutation groundwork.

Expected categories:

```text
entity existence/state lookup
localization existence lookup

Resource id/version read
Resource row lock
Resource version compare/bump primitive

relation lookup primitives
fixture/smoke reads where appropriate
```

Do not pre-build complete P0-2B/P0-2C APIs.

Do not add homepage/search queries.

Generated sqlc code stays committed and must not be hand-edited.

---

## 24. Resource Version / CAS Groundwork

Provide a minimal reusable sqlc primitive for version conflict detection.

Conceptual form:

```sql
UPDATE app.resources
SET version = version + 1,
    updated_at = $now
WHERE id = $id
  AND version = $expected_version
  AND deleted_at IS NULL
RETURNING version;
```

Exact implementation may use row lock + checked version if that fits existing style better.

Requirement:

```text
stale expected_version must not silently overwrite current canonical state
```

P0-2A tests this primitive.

Full child-mutation orchestration belongs to P0-2C.

---

## 25. Localization Transaction Contract

P0-2A does not need complete CRUD, but tests must prove the intended transaction pattern.

Creation:

```text
entity
+
default localization
```

must commit atomically.

Change default locale:

```text
target localization must already exist
```

Delete localization:

```text
must reject deleting current default locale
```

Use narrow internal helpers/test transactions if needed.

Do not add circular FKs.

---

## 26. Repository Audit Update

Update `scripts/audit-repository.mjs`.

Allow:

```text
server/internal/resource/
server/internal/taxonomy/
```

Continue rejecting all other future domains.

Do not relax existing security/secret/dependency audits.

---

## 27. Disposable Database Tests

Use real PostgreSQL 18 disposable CI.

Verify PostgreSQL rejects:

```text
invalid Category/Tag/Resource slug
duplicate slug

invalid locale shape
same entity locale duplicated only by case

invalid Category/Tag state

invalid publication state
invalid lifecycle
invalid content rating

Resource version < 1

published Resource without published_at

duplicate ResourceTag

duplicate Source URL for same Resource

more than one primary Source

ResourceRelation self-edge

non-canonical related_to edge

duplicate ResourceExternalID namespace+external_id across resources
```

Prefer SQLSTATE assertions:

```text
23505 unique violation
23514 check violation
23503 foreign-key violation
42501 insufficient privilege
```

Also verify valid combinations:

```text
published + discontinued + explicit

primary Source that is unavailable

mirror Source with rights_status=unknown

same Source URL on different Resources

multiple external IDs in same namespace with distinct IDs
```

---

## 28. Privilege Integration Tests

Use actual disposable identities.

### gfp_api

```text
SELECT Resource Core               PASS
INSERT Resource                    FAIL 42501
UPDATE canonical graph             FAIL 42501
DELETE canonical graph             FAIL 42501
```

### gfp_admin

Expected DML succeeds.

Also verify:

```text
hard DELETE Resource               FAIL 42501
hard DELETE Category               FAIL 42501
UPDATE Category.slug               FAIL 42501
UPDATE Tag.slug                    FAIL 42501
```

### gfp_worker

```text
SELECT Resource Core               FAIL 42501
DML                                FAIL 42501
```

### gfp_readonly

```text
SELECT                             PASS
DML                                FAIL 42501
```

---

## 29. Disposable Migration Round-trip

Validate migration 00006:

```text
Up 00001..00006
↓
Down 00006 → version 5
↓
Up 00006 again
```

Allowed only with:

```text
CI=true
GFP_DISPOSABLE_INFRA=1
database = gfp_ci
```

Do not add a normal shared-development `down` command.

If needed, add a narrow guarded helper/test inside migration infrastructure.

---

## 30. Development Smoke

Add:

```text
pnpm smoke:resource:dev
```

It runs against prepared shared development infrastructure after:

```text
pnpm migrate:dev
```

Use:

```text
server/env/api.local
server/env/admin.local
server/env/worker.local
server/env/migrator.local
.local/readonly.env
```

Do not print DSNs/passwords.

`readonly` is not a runtime service.

Do not extend `config.Load()` with a fake readonly application service.

Expected shape:

```text
scripts/smoke-resource-dev.mjs
server/cmd/resource-smoke
```

Private DSNs may be passed via child-process env such as:

```text
RESOURCE_SMOKE_API_DATABASE_URL
RESOURCE_SMOKE_ADMIN_DATABASE_URL
RESOURCE_SMOKE_WORKER_DATABASE_URL
RESOURCE_SMOKE_MIGRATOR_DATABASE_URL
RESOURCE_SMOKE_READONLY_DATABASE_URL
```

Never log them.

---

## 31. Shared Dev Smoke Fixture

Generate random UUIDv7/slugs.

Using `gfp_admin`, create:

```text
Category + localization
Tag + localization

Resource A + default localization
Resource B + default localization

ResourceTag
Source
Relation
External ID
```

Then:

### gfp_api

```text
SELECT fixture PASS
canonical INSERT FAIL
```

### gfp_worker

```text
Resource Core SELECT FAIL
```

### gfp_readonly

```text
SELECT PASS
DML FAIL
```

### gfp_admin

Verify expected creation/update relation works.

Cleanup:

```text
gfp_migrator only
```

Delete only this run's random fixture in FK-safe reverse order.

No SSH, server administration, role creation or shared cluster changes.

---

## 32. CI Integration

Keep outer CI:

```text
pnpm check
pnpm generate
generated drift check
pnpm integration:ci
pnpm build:images
```

Extend `integration:ci` internally:

```text
fresh disposable PG/Redis
CI setup
migrations through 00006
00006 guarded Down/Up round-trip
existing driver smoke
existing P0-1 integration
P0-2A schema/privilege integration
P0-2A disposable resource smoke
```

No external provider calls.

Do not access:

```text
Tailscale
Resend
Google
GitHub OAuth
shared gfp_dev
developer .local files
```

---

## 33. Category Seed Policy

Do not seed Categories in migration 00006.

Document intended first taxonomy:

```text
game
creative-work
tool
platform
community
event
knowledge
marketplace
service
other
```

Actual creation belongs to P0-2C/operator curation.

Do not add magic fixed UUIDs.

Do not create a taxonomy bootstrap CLI in P0-2A unless an existing repository requirement proves it necessary.

---

## 34. Documentation Updates

Update at least:

```text
README.md
CHANGELOG.md

AGENTS.md if domain boundary guidance needs it

contracts/architecture.md
contracts/database.md

docs/product/domain-model.md
docs/architecture/backend.md
docs/architecture/data.md
docs/development.md
```

Add this specification under:

```text
docs/implementation/p0-2a-resource-domain-schema.md
```

Document:

```text
P0-2A scope
10-table Resource Core
multilingual localization model
Resource.version
Source three-axis model
Relation canonical edge model
External IDs
runtime DB privileges
no Public/Admin Resource API yet
P0-2B/P0-2C remain next
```

Remove stale text that says Resource/Taxonomy packages are still future/forbidden.

Do not rewrite unrelated P0-1 material.

---

## 35. Required Test Layers

Acceptance includes:

```text
ordinary Go unit tests
domain validation tests
URL normalization tests
locale canonicalization tests
relation canonicalization tests

sqlc generation
database schema integration
database privilege integration
Resource version/CAS test
localization transaction-pattern test

guarded migration round-trip
shared dev resource smoke
existing P0-1 regression
image build
```

---

## 36. Acceptance Criteria

### Baseline

```text
dev starts from 66c8abd or newer equivalent baseline
MAIL-0 CI remains green
00001..00005 untouched
```

### Migration

```text
00006_resource_core.sql                   PASS
10 Resource Core tables                   PASS
guarded disposable Down/Up                PASS
no shared-dev down path                   PASS
```

### Taxonomy

```text
Category schema/state                     PASS
Tag schema/state                          PASS
stable slug contract                      PASS
localizations                             PASS
no hierarchy/alias scaffold               PASS
```

### Resource

```text
independent publication/lifecycle/rating  PASS
content_rating explicit                   PASS
version >= 1                              PASS
soft deletion                             PASS
default localization contract             PASS
```

### Source

```text
type / availability / rights separated    PASS
URL normalization                         PASS
single primary Source                     PASS
same URL allowed across Resources         PASS
```

### Relation

```text
four relation types                       PASS
self-edge rejected                        PASS
related_to canonical ordering             PASS
no duplicate inverse storage              PASS
```

### External IDs

```text
namespace/external_id                     PASS
global namespace+external_id unique       PASS
```

### Localization

```text
x/text language canonicalization          PASS
case-duplicate locale rejected            PASS
default localization creation pattern     PASS
default switch requires target locale     PASS
default localization deletion rejected    PASS
```

### DB Roles

```text
gfp_api SELECT-only                       PASS
gfp_admin expected DML                    PASS
gfp_worker no Resource privilege          PASS
gfp_readonly SELECT-only                  PASS
```

### Repository Boundary

```text
taxonomy package allowed                  PASS
resource package allowed                  PASS
future domains still rejected             PASS
no transport/Redis/River in domain        PASS
```

### Shared Dev

```text
pnpm migrate:dev                          PASS
pnpm smoke:resource:dev                   PASS
fixture cleanup                           PASS
```

### Full Repository

```text
pnpm generate                             PASS
pnpm check                                PASS
generated drift                           PASS
pnpm integration:ci                       PASS
pnpm build:images                         PASS
```

---

## 37. Stop Conditions

Stop and report instead of broadening scope if:

1. P0-2A appears to require Public/Admin Resource HTTP APIs;
2. a new Redis key or River job appears necessary;
3. a trigger/stored procedure appears necessary for business behavior;
4. a circular FK is proposed for `default_locale`;
5. migration 00001–00005 would need editing;
6. Worker appears to require Resource privilege without a real Worker use case;
7. Category seed data would require magic UUIDs;
8. Resource model begins accumulating type-specific columns;
9. JSONB/EAV is proposed for canonical tags/translations/sources;
10. test infrastructure would need shared DB down migration;
11. shared infrastructure roles/ACLs appear to require widening;
12. P0-2B/P0-2C UI/API work starts leaking into this phase.

---

## 38. Implementation Order

1. Confirm `dev` baseline and clean worktree.
2. Run baseline `pnpm check`.
3. Add this implementation document to `docs/implementation`.
4. Create `00006_resource_core.sql`.
5. Add guarded disposable 00006 migration round-trip support.
6. Add `taxonomy` domain package.
7. Add `resource` domain package.
8. Promote `golang.org/x/text` to direct dependency if imported directly.
9. Add minimal `taxonomy.sql` / `resource.sql` sqlc primitives.
10. Run generation and commit generated sqlc changes.
11. Update repository domain audit.
12. Add domain/unit tests.
13. Add disposable schema/constraint tests.
14. Add disposable privilege tests.
15. Add Resource version/CAS tests.
16. Add localization transaction-pattern tests.
17. Add `pnpm smoke:resource:dev`.
18. Update docs/contracts/changelog.
19. Run `pnpm generate`.
20. Run `pnpm check`.
21. Run `pnpm integration:ci`.
22. Run `pnpm build:images`.
23. Apply `pnpm migrate:dev`.
24. Run `pnpm smoke:resource:dev`.
25. Verify migration `00001..00005` hashes are unchanged.
26. Inspect diff and repository audit.
27. Commit locally on `dev`.
28. Do not push.

---

## 39. Commit Contract

Recommended commit:

```text
feat: add resource domain foundation
```

Do not:

```text
push
merge main
tag
release
deploy production
```

---

## 40. Final Codex Report

Report in Chinese.

Include:

### Implemented

```text
migration 00006
10 Resource Core tables
taxonomy/resource domain
localization
version/CAS
Source model
Relation model
External IDs
sqlc groundwork
DB privileges
resource dev smoke
```

### Migration Safety

Confirm:

```text
00001..00005 unchanged
00006 disposable Down/Up result
no shared gfp_dev down performed
```

### DB Privileges

Report PASS/FAIL separately for:

```text
gfp_api
gfp_admin
gfp_worker
gfp_readonly
```

### Verification

List every command actually run and result, including:

```text
pnpm generate
pnpm check
pnpm integration:ci
pnpm build:images
pnpm migrate:dev
pnpm smoke:resource:dev
```

Do not claim a command passed if it was not run.

### Scope

Confirm no implementation of:

```text
Public Resource API
Admin Resource CRUD API
Astro Resource UI
Admin Resource UI
Contribution/Search/etc.
```

### Git

Report:

```text
branch
local commit SHA
git status
```

Do not push.

### Next

If every gate passes:

```text
P0-2A complete
Ready for P0-2B — Public Resource Read Surface
```
