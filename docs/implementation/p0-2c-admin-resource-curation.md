# P0-2C — Admin Resource Curation

**Repository:** `deepfurry/tap4furry`  
**Target branch:** `dev`  
**Baseline:** `5874ffa4c374250909062f538ec6201fa7cef2a0`  
**Baseline commit:** `feat: add public resource read surface`  
**Baseline CI:** GitHub Actions Run #9 — success  
**Prerequisites:** P0-2A Resource Domain & Schema, P0-2B Public Resource Read Surface complete  
**Status:** Codex implementation specification  
**Date:** 2026-09-20

---

## 1. Goal

P0-2C establishes the canonical **Admin Resource Curation system**.

This phase answers:

> How can moderators, editors and administrators safely inspect, edit and govern the canonical Resource Graph while preserving capability boundaries, optimistic concurrency, localization integrity, graph invariants and Public visibility semantics?

P0-2C closes the first complete Resource lifecycle:

```text
Admin curator
    ↓
Category / Tag
    ↓
Resource + Localization
    ↓
Tags / Sources / Relations / External IDs
    ↓
Publication
    ↓
Public API
    ↓
Astro Resource Page
```

P0-2C is a **canonical mutation + Admin API + Admin UI** phase.

It is not Contribution, Search, Exchange or Trust/Moderation workflow work.

---

## 2. Product / Architecture Principles

Preserve:

```text
Resource First
Web First
Read First
Contribution over Administration
Complexity Must Be Earned
Resource ≠ Source ≠ Distribution
SQL-first persistence
Explicit transactions
Human authorization in Go
```

Mutation path:

```text
Admin React
    ↓
Admin OpenAPI / Fiber
    ↓
curation.App
    ↓
resource / taxonomy Domain
    ↓
sqlc
    ↓
pgx / PostgreSQL
```

Read path:

```text
Admin GET
    ↓
purpose-built sqlc read model
```

Do not force reads through mutation/domain reconstruction when it adds no value.

---

## 3. Database Freeze

P0-2C must keep:

```text
Goose schema version = 6
```

Existing migrations remain immutable:

```text
00001_foundation.sql
00002_identity_local_auth.sql
00003_auth_security_recovery.sql
00004_oauth_identity.sql
00005_admin_auth_roles.sql
00006_resource_core.sql
```

Do not create:

```text
00007_*.sql
```

Do not modify existing Resource Core schema, indexes, constraints or grants.

If implementation appears to require schema expansion:

> Stop and report the conflict instead of silently creating a migration.

---

## 4. Explicit Non-goals

Do not implement:

```text
Contribution
Review workflow
User-submitted Resource changes
Search / fuzzy search / FTS
Discovery ranking
Recommendation
Collections
Save / Want / Have
Exchange
Claims
Organizations
Discussions
Polls
Notifications
Trust scoring
Moderation cases
AuditLog domain
Media / R2
Image gallery
Source probing
Import/sync pipelines
Semantic/vector search
Bulk Resource editing
Bulk publish/delete
Restore workflow
```

Do not add:

```text
Redis Resource cache
River Resource jobs
Worker Resource privileges
new roles
dynamic RBAC
generic repository/service abstraction stacks
```

---

## 5. Existing Roles / Capabilities

Keep existing roles:

```text
moderator
editor
admin
```

Existing capability mapping remains authoritative:

```text
moderator
→ AdminAccess
→ Moderation

editor
→ AdminAccess
→ Editorial

admin
→ AdminAccess
→ Moderation
→ Editorial
→ Administration
```

Do not add new roles or dynamic permissions.

---

## 6. P0-2C Capability Contract

### Moderator

May:

```text
enter Admin
read Resource Graph
read Category/Tag
read Source rights state
read canonical Resource detail
```

May not mutate canonical Resource knowledge.

### Editor

May perform ordinary curation:

```text
create Resource Draft
edit Resource core fields
edit Resource localizations
change default locale
set Tags
create/edit Sources
set Source availability / primary
create/delete Relations
set External IDs
change lifecycle
change content rating
move draft/pending/published
create/edit Category/Tag localization
create Category/Tag
```

### Administrator

Includes Editorial plus high-impact governance:

```text
retire / reactivate Category or Tag
restricted / removed publication transitions
leave restricted / removed
Source rights_status changes
Resource soft delete
Category soft delete
Tag soft delete
```

Backend is authoritative.

Frontend capability helpers are UX only.

---

## 7. Transactional Capability Revalidation

Curation mutation authorization must not rely only on roles captured when the Admin session was first resolved.

Every canonical mutation transaction must revalidate the actor.

Required pattern:

```text
BEGIN

lock actor user row
↓
verify active Admin session
↓
re-read current PostgreSQL roles
↓
require capability
↓
lock target canonical entities
↓
perform mutation
↓
COMMIT
```

The actor `users` row must be locked consistently with RoleOperator behavior so role grant/revoke and curation mutation serialize correctly.

Required concurrency semantics:

```text
mutation acquires actor lock first
→ concurrent role revoke waits
→ current mutation may finish under the authority that was valid when it acquired the lock

role revoke commits first
→ later mutation re-reads roles
→ mutation returns 403 ADMIN_FORBIDDEN
```

Implement a small reusable auth primitive, e.g.:

```text
RequireAdminCapabilityTx
```

or equivalent.

Do not build a second authentication system.

Reuse existing Admin session / role logic.

---

## 8. Timestamp Source

Canonical curation transactions should use PostgreSQL transaction time consistently:

```sql
transaction_timestamp()
```

Avoid mixing server host `time.Now()` with database `now()` for Resource graph mutation timestamps.

The same transaction timestamp should be used for:

```text
created_at
updated_at
published_at first-write
deleted_at
```

where applicable.

---

## 9. New Application Package

Add:

```text
server/internal/curation/
```

Suggested structure:

```text
curation/
├─ app.go
├─ taxonomy.go
├─ resource.go
├─ source.go
└─ relation.go
```

Keep it small and pragmatic.

Do not add ceremonial directories such as:

```text
services/
repositories/
ports/
adapters/
commands/
handlers/
```

unless a real boundary later justifies them.

`curation.App` owns mutation transactions.

`resource` / `taxonomy` remain ordinary domain packages without pgx/sqlc/Fiber dependencies.

---

## 10. SQL Files

Add purpose-built curation SQL, suggested:

```text
server/db/queries/admin_resource.sql
server/db/queries/curation.sql
```

### admin_resource.sql

Owns Admin read models:

```text
ListAdminResources
GetAdminResource
ListAdminCategories
GetAdminCategory
ListAdminTags
GetAdminTag
Resource child reads
```

### curation.sql

Owns mutation primitives:

```text
lock actor / Resource / taxonomy
insert/update/delete localizations
replace Resource tags
Source insert/update
relation insert/delete / cycle query
External ID diff primitives
publication/core updates
soft delete
taxonomy in-use checks
```

Do not add a repository wrapper that only renames sqlc.

---

## 11. Admin OpenAPI Scope

Modify:

```text
contracts/openapi/admin.yaml
```

Add canonical curation endpoints.

All endpoints require Admin session.

All unsafe endpoints require:

```text
exact ADMIN_ORIGIN
+
X-CSRF-Token
```

Admin API remains:

```text
https://admin.tap4furry.com/api/*
```

---

## 12. Admin Resource Read Endpoints

Implement:

```text
GET /resources
GET /resources/{resource_id}
```

Admin uses UUID path identity.

Do not use public slug as Admin entity identity.

---

## 13. Admin Resource List

`GET /resources`

Supported filters only:

```text
page
page_size
publication_state
category_id
slug
```

`slug` is exact canonical slug equality only.

It is not fuzzy Search.

Defaults:

```text
page      = 1
page_size = 50
max       = 100
```

Sort exactly:

```text
updated_at DESC
id DESC
```

No:

```text
q
ranking
fuzzy
full text
tag search
custom sort
```

---

## 14. Admin Resource List Item

Return:

```text
id
slug
name
default_locale

category

publication_state
lifecycle
content_rating

version

published_at
updated_at
```

`name` is the canonical default-localization name.

Admin list does not need requested-locale rendering.

---

## 15. Admin Resource Detail

Return canonical editing snapshot:

```text
id
slug
default_locale

category

publication_state
lifecycle
content_rating

version

localizations[]
tags[]
sources[]
relations[]
external_ids[]

published_at
created_at
updated_at
```

Normal Admin reads exclude soft-deleted Resource entities.

Do not add a P0-2C deleted/restore browser.

---

## 16. Admin Category / Tag Read Endpoints

Implement:

```text
GET /categories
GET /categories/{category_id}

GET /tags
GET /tags/{tag_id}
```

Normal browse includes:

```text
active
retired
```

but excludes soft-deleted taxonomy.

Category/Tag detail:

```text
id
slug
default_locale
state
localizations[]
created_at
updated_at
```

No restore surface.

---

## 17. Mutation Error Contract

Retain existing Admin errors and add only:

```text
CURATION_NOT_FOUND
RESOURCE_VERSION_CONFLICT
CURATION_CONFLICT
CURATION_IN_USE
CURATION_RELATION_CYCLE
```

Mapping:

```text
400 VALIDATION_ERROR
401 ADMIN_UNAUTHENTICATED
403 ADMIN_FORBIDDEN
404 CURATION_NOT_FOUND
409 RESOURCE_VERSION_CONFLICT
409 CURATION_CONFLICT
409 CURATION_IN_USE
409 CURATION_RELATION_CYCLE
500 INTERNAL_ERROR
```

Do not expose:

```text
SQLSTATE
constraint names
pgx errors
SQL text
internal DB identifiers
```

Map known constraint conflicts safely.

---

## 18. Resource Revision / CAS Contract

Every Resource-owned canonical mutation requires:

```text
?expected_version=N
```

where:

```text
N >= 1
int64
required
```

Applicable to:

```text
PATCH Resource core
PUT/DELETE Resource localization
PUT Resource tags
POST/PATCH Source
PUT Source rights
POST/DELETE Relation
PUT External IDs
PUT Publication
DELETE Resource
```

Stale expected version:

```text
409 RESOURCE_VERSION_CONFLICT
```

A stale transaction must roll back all child writes.

---

## 19. No-op Semantics

If canonical state does not change:

```text
version unchanged
updated_at unchanged
```

Examples:

```text
PATCH lifecycle=active when already active
PUT identical localization
PUT identical Tag set
PUT identical External ID set
PATCH identical Source
PUT publication=published when already published
```

Return success and current revision where applicable.

`version` is canonical revision, not request count.

---

## 20. Resource Revision Response

Resource mutation response may use a compact DTO:

```text
ResourceRevision
├─ id
└─ version
```

Example:

```json
{
  "id": "019...",
  "version": 18
}
```

Admin UI should invalidate/refetch canonical detail after success.

Do not return entire Resource detail from every mutation.

---

## 21. Create Resource

Endpoint:

```text
POST /resources
```

Capability:

```text
Editorial
```

Body:

```json
{
  "slug": "untamed-kingdom",
  "default_locale": "en",
  "category_id": "uuid",
  "content_rating": "explicit",
  "lifecycle": "unknown",
  "localization": {
    "name": "Untamed Kingdom",
    "summary": null,
    "description": null
  }
}
```

`lifecycle` may default to `unknown`.

Required:

```text
slug
default_locale
category_id
content_rating
localization.name
```

Transaction:

```text
require Editorial
validate Category active + not deleted
validate domain input
INSERT Resource
  publication_state=draft
  version=1
  published_at=NULL
INSERT default localization
COMMIT
```

Client must not provide publication state or version.

All Resources are created as Draft.

---

## 22. Resource Core PATCH

Endpoint:

```text
PATCH /resources/{resource_id}?expected_version=N
```

Capability:

```text
Editorial
```

Allowed fields:

```text
slug
default_locale
category_id
lifecycle
content_rating
```

Do not allow this endpoint to change:

```text
name
summary
description
publication_state
version
published_at
deleted_at
```

PATCH is true partial update:

```text
omitted = preserve
null = validation error for non-null fields
```

Do not introduce generic JSON Merge Patch semantics.

---

## 23. Resource Slug Freeze

Resource slug is mutable only while:

```text
published_at IS NULL
```

After first publication:

```text
published_at IS NOT NULL
→ slug immutable forever through normal curation
```

This remains true even if current publication state later becomes:

```text
draft
restricted
removed
```

---

## 24. Category Change

When Category ID does not change:

```text
retired current Category may remain
```

When changing Category:

```text
target Category must be active and not deleted
```

Do not permit active migration into a retired Category.

---

## 25. Resource Localization

Endpoints:

```text
PUT    /resources/{resource_id}/localizations/{locale}?expected_version=N
DELETE /resources/{resource_id}/localizations/{locale}?expected_version=N
```

Capability:

```text
Editorial
```

PUT body:

```json
{
  "name": "...",
  "summary": null,
  "description": null
}
```

PUT semantics:

```text
missing localization → INSERT
existing localization → UPDATE
```

Canonicalize locale using existing taxonomy locale parser.

Changed localization:

```text
Resource version +1 exactly once
```

Identical PUT:

```text
no version bump
```

---

## 26. Localization Validation

Resource:

```text
name        required, <=160 Unicode chars
summary     nullable, <=500 Unicode chars
description nullable, <=50,000 Unicode chars
```

Optional blank values normalize to NULL.

Category/Tag:

```text
name        required, <=80 Unicode chars
description nullable, <=500 Unicode chars
```

Do not add a new DB VARCHAR limit for Resource description.

Application/OpenAPI owns the P0 curation limit.

---

## 27. Default Locale Invariant

Changing Resource/Category/Tag `default_locale` requires target localization to exist first.

Deleting current default localization:

```text
400 VALIDATION_ERROR
```

Required workflow:

```text
create target localization
↓
change default_locale
↓
delete old localization
```

Lock parent row during this decision.

---

## 28. Resource Tag Set Replacement

Endpoint:

```text
PUT /resources/{resource_id}/tags?expected_version=N
```

Body:

```json
{
  "tag_ids": ["uuid-a", "uuid-b"]
}
```

Capability:

```text
Editorial
```

Application:

```text
load current set
dedupe input
calculate diff
DELETE removed
INSERT added
bump Resource once if changed
```

No change:

```text
no bump
```

---

## 29. Retired Tag Binding Semantics

Existing retired Tag binding may remain.

New retired Tag binding is forbidden.

Example:

```text
current = [A active, B retired]

submit [A, B]
→ PASS / no new retired binding

submit [A]
→ PASS / B removed

submit [A, B] later
→ FAIL
```

Only newly-added tags must satisfy:

```text
active
not deleted
```

---

## 30. External ID Set Replacement

Endpoint:

```text
PUT /resources/{resource_id}/external-ids?expected_version=N
```

Body:

```json
{
  "items": [
    {
      "namespace": "steam_app",
      "external_id": "123456"
    }
  ]
}
```

Capability:

```text
Editorial
```

Application:

```text
normalize
validate
dedupe
diff
delete removed
insert added
bump Resource once if changed
```

Identical set:

```text
no bump
```

Global identity conflict:

```text
(namespace, external_id)
already belongs to another Resource
→ 409 CURATION_CONFLICT
```

Do not auto-transfer external identities.

---

## 31. Source Create

Endpoint:

```text
POST /resources/{resource_id}/sources?expected_version=N
```

Capability:

```text
Editorial
```

Body:

```json
{
  "url": "https://example.com",
  "label": "Official website",
  "source_type": "official",
  "availability_state": "active",
  "is_primary": true
}
```

Do not accept `rights_status`.

Create with:

```text
rights_status = unknown
```

Normalize URL using existing Resource domain rules.

Changed graph:

```text
Resource version +1
```

---

## 32. Source Edit

Endpoint:

```text
PATCH /resources/{resource_id}/sources/{source_id}?expected_version=N
```

Capability:

```text
Editorial
```

Editor may modify:

```text
url
label
source_type
availability_state
is_primary
```

May not modify:

```text
rights_status
```

No-op:

```text
version unchanged
```

---

## 33. Source Primary Auto-switch

If another Source is currently primary and target becomes primary:

```text
old primary → false
target      → true
```

in the same transaction.

Do not leak partial unique-index `23505` to the user.

---

## 34. Source Retention

P0-2C does not expose a Source DELETE endpoint.

To remove a Source from Public:

```text
availability_state = removed
```

Source row remains for historical knowledge.

Do not hard-delete ResourceSource from Admin product behavior.

---

## 35. Source Rights Governance

Endpoint:

```text
PUT /resources/{resource_id}/sources/{source_id}/rights?expected_version=N
```

Capability:

```text
Administration
```

Body:

```json
{
  "rights_status": "confirmed"
}
```

Allowed states:

```text
unknown
creator_provided
confirmed
disputed
rights_review
removed_by_request
```

Changed state:

```text
Resource version +1
```

Editor may see rights state but cannot mutate it.

---

## 36. Relation Add

Endpoint:

```text
POST /resources/{resource_id}/relations?expected_version=N
```

Capability:

```text
Editorial
```

Body:

```json
{
  "target_resource_id": "uuid",
  "relation_type": "successor_of"
}
```

Path Resource is the anchor Resource, not necessarily stored SQL source after `related_to` canonicalization.

---

## 37. Relation Mutation Transaction

Required flow:

```text
BEGIN

actor capability validation

fixed pg_advisory_xact_lock(resource_relation_graph_key)

load/validate endpoint IDs

lock both Resource rows in deterministic UUID order

verify anchor expected_version

validate:
- both exist
- neither soft-deleted
- no self-edge
- relation type valid

canonicalize related_to

if directed type:
  recursive CTE cycle check for same relation_type only

INSERT / DELETE relation

bump endpoint A exactly once
bump endpoint B exactly once

COMMIT
```

Use one fixed advisory transaction lock for graph mutation.

This is acceptable because Admin relation edits are low-frequency and correctness matters more than concurrency.

---

## 38. Relation Cycle Semantics

Directed DAG types are independent:

```text
part_of
successor_of
derived_from
```

Cycle check for `part_of` must follow only `part_of`.

Same for the others.

Do not treat mixed relation semantics as one DAG.

`related_to` may form ordinary graph cycles.

---

## 39. Relation Concurrency Acceptance

Must test a concurrent cycle race.

Example:

```text
Tx A proposes:
A successor_of B

Tx B proposes:
B successor_of A
```

Both transactions starting concurrently must not both commit.

Expected:

```text
one succeeds
one returns CURATION_RELATION_CYCLE or equivalent safe conflict
```

The fixed advisory transaction lock should serialize the cycle decision.

---

## 40. Relation Delete

Endpoint:

```text
DELETE /resources/{anchor_id}/relations/{relation_id}?expected_version=N
```

Capability:

```text
Editorial
```

Application:

```text
load relation
verify anchor is one endpoint
graph advisory lock
lock endpoints UUID order
verify anchor version
delete relation
bump both endpoint Resources once
commit
```

Do not bump only the currently viewed Resource.

---

## 41. Publication Endpoint

Endpoint:

```text
PUT /resources/{resource_id}/publication?expected_version=N
```

Body:

```json
{
  "state": "published"
}
```

Do not mutate publication state through ordinary Resource PATCH.

---

## 42. Publication Capability Rules

Transitions among:

```text
draft
pending
published
```

require:

```text
Editorial
```

Any transition to or from:

```text
restricted
removed
```

requires:

```text
Administration
```

This prevents an Editor from bypassing an Admin restriction/removal by republishing.

---

## 43. published_at Semantics

First successful transition into `published` when:

```text
published_at IS NULL
```

sets:

```text
published_at = transaction timestamp
```

It is never cleared.

Example:

```text
draft → published
→ first timestamp

published → restricted → published
→ original timestamp retained

published → draft → published
→ original timestamp retained
```

---

## 44. Resource Soft Delete

Endpoint:

```text
DELETE /resources/{resource_id}?expected_version=N
```

Capability:

```text
Administration
```

Behavior:

```text
deleted_at = transaction timestamp
Resource version +1
```

No hard delete.

After delete:

```text
normal Admin Resource GET → 404 CURATION_NOT_FOUND
Public Resource GET       → 404 RESOURCE_NOT_FOUND
```

P0-2C adds no restore endpoint or restore UI.

---

## 45. Category API

Implement:

```text
GET    /categories
GET    /categories/{category_id}
POST   /categories
PATCH  /categories/{category_id}
PUT    /categories/{category_id}/localizations/{locale}
DELETE /categories/{category_id}/localizations/{locale}
DELETE /categories/{category_id}
```

---

## 46. Category Create

Capability:

```text
Editorial
```

Body:

```json
{
  "slug": "game",
  "default_locale": "en",
  "localization": {
    "name": "Game",
    "description": null
  }
}
```

Create:

```text
state=active
deleted_at=NULL
parent + default localization atomically
```

Slug becomes immutable after creation.

---

## 47. Category PATCH

Allowed:

```text
default_locale
state
```

Changing default locale:

```text
Editorial
target localization must exist
```

Changing state:

```text
active ↔ retired
→ Administration
```

No Category version field.

No-op must not update `updated_at`.

---

## 48. Category Localization

Endpoints:

```text
PUT    /categories/{id}/localizations/{locale}
DELETE /categories/{id}/localizations/{locale}
```

Capability:

```text
Editorial
```

Cannot delete current default locale.

Category/Tag localization changes do not bump Resource versions.

---

## 49. Category Soft Delete

Endpoint:

```text
DELETE /categories/{id}
```

Capability:

```text
Administration
```

If any non-soft-deleted Resource references the Category:

```text
409 CURATION_IN_USE
```

Do not allow one Category delete to silently hide many active Resources.

Correct workflow:

```text
migrate Resources to another active Category
↓
soft-delete old Category
```

---

## 50. Tag API

Implement analogous endpoints:

```text
GET    /tags
GET    /tags/{tag_id}
POST   /tags
PATCH  /tags/{tag_id}
PUT    /tags/{tag_id}/localizations/{locale}
DELETE /tags/{tag_id}/localizations/{locale}
DELETE /tags/{tag_id}
```

Same:

```text
slug immutable
state active/retired
default locale invariant
Editorial localization
Administration state/soft-delete
```

---

## 51. Tag Soft Delete

If Tag is bound to any Resource with:

```text
resources.deleted_at IS NULL
```

return:

```text
409 CURATION_IN_USE
```

Bindings attached only to soft-deleted Resources do not block taxonomy cleanup.

---

## 52. Taxonomy Concurrency

P0-2C intentionally does not add optimistic version fields to Category/Tag.

Use row locks for mutation.

This is a deliberate simplification.

Do not create taxonomy version columns.

---

## 53. Admin Transport Boundary

Extend Admin transport middleware coverage to:

```text
/resources
/categories
/tags
```

All curation routes:

```text
AdminSession required
Cache-Control: no-store
5-second request context deadline
```

Unsafe methods:

```text
POST
PUT
PATCH
DELETE
```

must additionally require:

```text
exact ADMIN_ORIGIN
valid X-CSRF-Token
```

Do not accept Public session cookies.

---

## 54. Admin Access Guard

Add or refactor a small Admin actor guard so GET curation routes require `AdminAccess`.

Resolve actor once per request and store it in Fiber Locals.

CSRF and handler logic should reuse the resolved actor.

Do not unnecessarily resolve Admin session repeatedly in the same request.

Mutation transaction must still revalidate actor/capability as described earlier.

---

## 55. Admin Body Limit

Current Admin server body limit is too small for Resource Markdown.

Raise Admin Fiber hard body limit to approximately:

```text
256 KiB
```

This is a transport ceiling, not content validation.

Retain strict existing Auth body decoding limits:

```text
login / reauth etc.
→ 8 KiB decoder contract unchanged
```

Do not loosen Auth request validation just because the Admin process accepts larger curation bodies.

---

## 56. Admin UI Information Architecture

Refactor Admin into:

```text
/login

/resources
/resources/new

/resources/$resourceId
/resources/$resourceId/localizations
/resources/$resourceId/tags
/resources/$resourceId/sources
/resources/$resourceId/relations
/resources/$resourceId/external-ids

/taxonomy/categories
/taxonomy/categories/$categoryId

/taxonomy/tags
/taxonomy/tags/$tagId

/account
```

Root:

```text
/
→ redirect /resources
```

Move existing Workspace account/session/reauthentication features to `/account`.

Preserve existing Admin authentication behavior.

---

## 57. Admin Frontend Structure

Refactor away from a growing `main.tsx` / `Auth.tsx` monolith.

Suggested:

```text
apps/admin/src/

main.tsx
router.tsx

layouts/
  AdminShell.tsx

pages/
  LoginPage.tsx
  AccountPage.tsx

  resources/
    ResourceListPage.tsx
    ResourceCreatePage.tsx
    ResourceLayout.tsx
    ResourceOverviewPage.tsx
    ResourceLocalizationsPage.tsx
    ResourceTagsPage.tsx
    ResourceSourcesPage.tsx
    ResourceRelationsPage.tsx
    ResourceExternalIDsPage.tsx

  taxonomy/
    CategoryListPage.tsx
    CategoryPage.tsx
    TagListPage.tsx
    TagPage.tsx

components/
  admin/
    Panel.tsx
    Field.tsx
    Button.tsx
    Badge.tsx
    ConfirmDialog.tsx
    EmptyState.tsx

lib/
  admin-api.ts
  capabilities.ts
  query-keys.ts
```

Exact filenames may adapt to actual repo conventions.

Do not create one giant generic dynamic form framework.

---

## 58. Admin UI Technology

Continue:

```text
React 19
TanStack Router
TanStack Query
Tailwind layout
SCSS Modules for visual treatment
@tap4furry/design tokens
```

Do not introduce a full UI framework such as:

```text
MUI
Ant Design
Mantine
large shadcn bundle
```

Use a small set of reusable Admin primitives.

If an accessible modal primitive is truly needed, prefer a minimal dependency or native `<dialog>`.

---

## 59. Resource List UI

`/resources`

Display:

```text
Name
Slug
Category
Publication
Lifecycle
Rating
Version
Updated
```

Controls:

```text
Publication State filter
Category filter
Exact slug lookup
New Resource
```

No:

```text
bulk edit
bulk publish
bulk delete
table customization
saved filters
fuzzy search
```

---

## 60. Create Resource UI

`/resources/new`

Fields:

```text
Slug
Default Locale
Category
Content Rating
Lifecycle

Default Localization:
Name
Summary
Description
```

Primary action text:

```text
Create Draft
```

Successful create:

```text
navigate /resources/{id}
```

Do not expose Publish during Resource creation.

---

## 61. Resource Editor Layout

Shared header:

```text
Resource Name
slug
Publication
Lifecycle
Content Rating
Version
View Public Page (only when published)
```

Tabs are real routes:

```text
Overview
Localizations
Tags
Sources
Relations
External IDs
```

All share canonical Resource detail query key.

Do not build one huge scrolling form.

---

## 62. Resource Overview

Edit only:

```text
slug
default locale
category
lifecycle
content rating
```

Publication is a distinct explicit action area.

If `published_at != NULL`:

```text
slug readonly
show "Frozen after first publication"
```

If current Category is retired:

```text
show current retired Category
allow it to remain
do not offer retired Categories as new selection targets
```

---

## 63. Publication UI

Do not render publication as a generic freeform dropdown.

Show explicit state actions.

Example Draft:

```text
Current: Draft

Move to Pending
Publish
```

Published:

```text
Current: Published

Move to Draft
```

Admin additionally sees:

```text
Restrict
Remove
```

Restricted/Removed recovery actions are Admin-only.

Frontend reflects capability; backend remains authoritative.

---

## 64. Localization UI

Use:

```text
locale list
+
editor panel
```

Example:

```text
en       Default
zh-Hans
ja

[Add localization]
```

Editor:

```text
Name
Summary
Description (Markdown textarea)
Save
```

For non-default localization:

```text
Make default
Delete
```

Default localization cannot be deleted.

---

## 65. Markdown Editing UI

Use a normal textarea.

Do not add:

```text
TipTap
Milkdown
MDX editor
rich-text state machine
client sanitizer
complex live preview
```

Explain:

```text
Markdown supported.
Raw HTML and embedded images are not supported.
```

Canonical renderer remains the Public Astro server-side Markdown pipeline.

---

## 66. Resource Tags UI

Show current selected Tags plus available active Tags.

Current retired Tag:

```text
visible
selected
marked Retired
may be preserved
may be removed
```

Once removed, retired Tag cannot be newly selected again.

UI may locally filter the already-loaded Tag list.

This is not backend Search.

Submit entire set through the set-replacement endpoint.

---

## 67. Source UI

Show each Source with:

```text
URL
Label
Type
Availability
Primary
Rights
```

Editor can edit ordinary Source fields.

Rights is visible to Editor but read-only.

Administrator gets explicit rights control.

No Source Delete action.

Removal uses:

```text
Availability → Removed
```

Setting another Source primary should require no manual un-primary step.

---

## 68. Relation UI

Display:

```text
→ outgoing
← incoming
↔ symmetric
```

Keep stored/API relation vocabulary unchanged.

No invented inverse types.

Target lookup uses exact slug:

```text
GET /resources?slug={canonical-slug}
```

Flow:

```text
Relation type
Target slug
Lookup
confirm Resource
Add relation
```

This exact lookup is curation utility, not P0-4 Search.

---

## 69. External IDs UI

Editable row set:

```text
namespace | external_id
```

Actions:

```text
Add row
Remove row
Save set
```

Perform basic client-side empty/duplicate validation.

Backend owns namespace syntax and global uniqueness.

Conflict with another Resource:

```text
409 CURATION_CONFLICT
```

Do not auto-transfer.

---

## 70. Taxonomy UI

Top-level:

```text
Taxonomy
├─ Categories
└─ Tags
```

List includes active and retired.

Detail includes:

```text
slug (immutable)
state
default locale
localizations
```

Editor:

```text
localization editing
default locale change
```

Administrator:

```text
retire/reactivate
soft delete
```

Show Admin-only controls as disabled or clearly marked when appropriate rather than pretending the capability does not exist.

---

## 71. Danger Zone

Use a shared Danger Zone pattern for:

```text
Resource soft delete
Category soft delete
Tag soft delete
```

Require typed slug confirmation.

Example:

```text
Type "untamed-kingdom" to confirm.
```

Do not use typed confirmation for ordinary updates.

Resource soft-delete warning should state:

```text
Restoration is not available from Admin yet.
```

Archive is a lifecycle state and must not be confused with deletion.

---

## 72. Moderator UI

Moderator can navigate and inspect the full canonical Resource Graph.

Show:

```text
Read-only
Your current role does not include Editorial capability.
```

Mutation controls are disabled/read-only.

Do not block Moderator from the entire Resource UI.

This preserves context for future Moderation work.

---

## 73. Frontend Capability Helper

Add small UX helper:

```text
apps/admin/src/lib/capabilities.ts
```

Mirror current static role mapping:

```text
canAdminAccess
canEditorial
canModerate
canAdministrate
```

Include clear comment:

```text
UX only.
Backend capability checks are authoritative.
```

No dynamic RBAC client system.

---

## 74. React Query Strategy

For canonical curation mutation:

```text
mutation retry = false
optimistic update = none
```

After success:

```text
invalidate canonical Resource query
refetch server state
```

Reason:

```text
URL normalization
Primary Source auto-switch
Relation endpoint version bump
published_at first-write
locale canonicalization
```

may produce canonical side effects.

Do not attempt optimistic reconstruction.

---

## 75. Resource Version Conflict UX

When mutation returns:

```text
409 RESOURCE_VERSION_CONFLICT
```

do not:

```text
retry automatically
discard current form
immediately overwrite with refetch
```

Instead:

```text
show conflict notice
preserve unsaved local form
disable further save
offer explicit "Reload latest version"
```

Only after user explicitly reloads should local draft be discarded.

---

## 76. Dirty Form Guard

For Overview and localization forms:

```text
dirty route navigation
page refresh
browser close
```

should prompt once.

Use:

```text
TanStack Router blocker
+
beforeunload
```

Do not implement:

```text
autosave
localStorage draft persistence
server draft buffer
```

---

## 77. Admin Source Audit

Add a lightweight source audit, suggested:

```text
scripts/admin-curation-audit.mjs
```

Protect long-lived Agent boundaries such as:

```text
Resource editor tabs remain real routes
capability helper remains UX-only
canonical mutations do not configure automatic retry
no Resource autosave
destructive actions use shared ConfirmDialog
main.tsx/router do not absorb all business UI
```

Avoid brittle visual snapshots.

---

## 78. Admin API Client

Continue contract-first:

```text
contracts/openapi/admin.yaml
↓
oapi-codegen
Orval
↓
@tap4furry/api-client/admin
```

Do not hand-edit generated code.

Update public Admin client exports intentionally.

Frontend `admin-api.ts` may wrap generated methods for:

```text
same-origin credentials
CSRF acquisition
stable AdminRequestError mapping
```

Do not duplicate DTOs manually.

---

## 79. Shared Dev Smoke

Do not create a separate `smoke:curation:dev`.

Extend existing:

```text
pnpm smoke:admin:dev
```

because it already owns the real Admin authentication fixture and role lifecycle.

Use:

```text
gfp_admin
gfp_api
gfp_migrator
real Admin session
real CSRF
RoleOperator
Public Handler
```

No private values printed.

---

## 80. Shared Smoke Flow

Extend smoke approximately:

```text
create temporary verified account

grant Moderator
→ Admin GET Resource Graph PASS
→ curation mutation 403

grant Editor
→ create Category
→ create Tag
→ create Draft Resource
→ add localization
→ set tags
→ create Source
→ create Relation
→ set External ID
→ publish
→ Public API confirms visible

Editor attempts:
→ rights change 403
→ restricted/removed 403
→ soft delete 403

grant Admin
→ rights change PASS
→ restrict/remove transitions PASS
→ retire taxonomy PASS
→ soft-delete governance PASS

cleanup only temporary account + temporary Resource graph fixture
```

Shared smoke must leave:

```text
Goose version 6
no fixture rows
no role/grant changes outside fixture
```

---

## 81. Disposable Curation Integration

Add explicit disposable curation integration guard, suggested:

```text
GFP_CURATION_INTEGRATION=1
```

Use actual prepared CI roles.

Test:

```text
curation.App
Admin transport
capability revalidation
Resource CAS
no-op
localization
tags
sources
relations
external IDs
publication
soft deletion
taxonomy
```

---

## 82. Capability Race Tests

Must test role mutation vs curation mutation.

At minimum:

```text
case A:
curation locks actor first
concurrent role revoke waits
curation completes

case B:
role revoke commits first
curation begins after
curation 403
```

Do not rely on timing sleeps alone; coordinate transactions deterministically where possible.

---

## 83. Admin → Public Lifecycle Closure

Disposable and/or shared smoke must prove:

```text
Admin creates Draft
→ Public GET 404

Admin publishes
→ Public GET 200

Admin restricts/removes
→ Public GET 404

Admin republishes
→ Public GET 200

Admin soft-deletes
→ Public GET 404
```

This is the core P0-2C end-to-end acceptance.

---

## 84. Database Privilege Preservation

P0-2C must reuse P0-2A grants.

Verify after implementation:

```text
gfp_api
→ Resource Core SELECT-only

gfp_admin
→ existing precise curation DML only

gfp_worker
→ no Resource Core access

gfp_readonly
→ SELECT-only
```

Stop if implementation appears to require:

```text
hard DELETE Resource
UPDATE Category/Tag slug
Worker Resource access
wider runtime grants
```

---

## 85. P0 Regression

All previous integration stays green:

```text
P0-1 Auth/OAuth/Recovery/Admin auth
P0-2A Resource Core / grants / CAS
P0-2B Public reads / Astro SSR
```

P0-2C may not regress Public privacy or cache/SEO behavior.

---

## 86. Frontend Acceptance

At minimum verify:

```text
/ redirects to /resources

Moderator:
read-only Resource UI

Editor:
ordinary curation controls

Admin:
governance/danger controls

Create Draft flow

Resource real subroutes:
Overview
Localizations
Tags
Sources
Relations
External IDs

published slug readonly

retired binding UX

Source no Delete

Rights Editor read-only

Relation exact-slug lookup

External ID whole-set edit

Danger Zone typed confirmation

version conflict preserves local form

dirty route/page guard

no optimistic canonical mutation

no autosave
```

---

## 87. Frontend Test Strategy

Do not introduce a major browser testing stack only for P0-2C.

Use:

```text
TypeScript typecheck
Vite production build
small pure helper tests
repository/source audit
shared-dev Admin smoke
manual product acceptance
```

Do not add Playwright/Cypress unless an implementation-specific blocker proves it is justified.

---

## 88. CI Integration

Keep existing outer CI:

```text
pnpm check
pnpm generate
generated drift
pnpm integration:ci
pnpm build:images
```

Extend `integration:ci` with P0-2C curation tests.

Logical chain:

```text
P0-1 auth
P0-2A Resource Core
P0-2B Public read
P0-2C curation
Astro SSR
```

CI must not access:

```text
shared gfp_dev
Tailscale
real OAuth providers
Resend
Cloudflare
production
```

---

## 89. Documentation Updates

Add this implementation spec:

```text
docs/implementation/p0-2c-admin-resource-curation.md
```

Update relevant:

```text
README.md
CHANGELOG.md

AGENTS.md / .agents/architecture.md if package routing changes

contracts/architecture.md
contracts/database.md
contracts/openapi/admin.yaml

docs/architecture/backend.md
docs/architecture/data.md
docs/development.md
docs/product/domain-model.md
```

Document:

```text
curation Application Layer
capability model
Resource CAS
publication governance
retired taxonomy semantics
relation cycle rules
Admin UI architecture
no schema migration
```

Do not rewrite unrelated P0 history.

---

## 90. Acceptance Gate

P0-2C is complete only when all are true.

### Database

```text
Goose remains 6                         PASS
no migration 00007                      PASS
00001..00006 unchanged                  PASS
Resource Core grants unchanged          PASS
```

### Application

```text
curation.App owns canonical mutations   PASS
actor lock + capability revalidation    PASS
explicit transactions                   PASS
PostgreSQL tx timestamp use             PASS
Resource CAS                            PASS
stale child writes rollback             PASS
no-op no version/update timestamp bump  PASS
```

### Taxonomy

```text
create + default localization            PASS
slug immutable                           PASS
default locale invariant                 PASS
retire/reactivate Admin-only             PASS
retired binding semantics                PASS
in-use Category delete protection        PASS
in-use Tag delete protection             PASS
soft delete only                         PASS
```

### Resource

```text
Create Draft only                        PASS
version starts 1                         PASS
slug freezes after first publish         PASS
active Category required on change       PASS
localization mutation                    PASS
Tag set replacement                      PASS
External ID set replacement              PASS
publication semantics                    PASS
soft delete                              PASS
```

### Source

```text
create/edit                              PASS
URL normalization                        PASS
primary auto-switch                      PASS
no hard delete endpoint                  PASS
removed availability preserved           PASS
rights Administration-only               PASS
```

### Relation

```text
self-edge rejected                       PASS
related_to canonicalization              PASS
directed relation direction preserved    PASS
per-type cycle prevention                PASS
concurrent cycle prevention              PASS
both endpoint revisions bump             PASS
```

### Security

```text
AdminSession required                    PASS
unsafe ADMIN_ORIGIN                      PASS
unsafe CSRF                              PASS
Moderator read-only                      PASS
Editor ordinary curation                 PASS
Admin governance                         PASS
role-change race correct                 PASS
```

### Admin UI

```text
Resources                                PASS
Create Draft                             PASS
Resource subroutes                       PASS
Taxonomy                                 PASS
Account                                  PASS
capability-aware controls                PASS
version conflict preserves form          PASS
dirty guard                              PASS
typed destructive confirmation           PASS
no optimistic canonical writes           PASS
no autosave                              PASS
```

### Integration

```text
Admin → Public lifecycle closure         PASS
disposable curation matrix               PASS
P0-1 regression                          PASS
P0-2A regression                         PASS
P0-2B regression                         PASS
pnpm smoke:admin:dev                     PASS
pnpm generate                            PASS
pnpm check                               PASS
pnpm integration:ci                      PASS
pnpm build:images                        PASS
generated drift                          PASS
```

---

## 91. Stop Conditions

Stop and report rather than broadening scope if:

1. migration 00007 appears necessary;
2. existing P0-2A DB grants appear insufficient and widening is proposed;
3. hard DELETE of Resource/Category/Tag/Source appears necessary;
4. Resource/Taxonomy domain needs pgx/Fiber/OpenAPI imports;
5. Worker/Redis/River becomes necessary for curation;
6. fuzzy Search/ranking is introduced for Admin pickers;
7. dynamic RBAC or new roles are proposed;
8. Source hard-delete is proposed instead of availability removal;
9. relation cycle correctness would rely only on UI checks;
10. stale CAS would allow partial child changes to commit;
11. React mutation code begins automatically retrying version conflicts;
12. autosave/local draft persistence is introduced;
13. Contribution/P0-3 or Search/P0-4 functionality begins leaking into this phase;
14. Restore semantics are being invented without a dedicated design.

---

## 92. Recommended Implementation Order

1. Confirm clean `dev` at `5874ffa4...` or clearly equivalent newer baseline.
2. Confirm CI Run #9 baseline green.
3. Hash/verify `00001..00006`.
4. Add this implementation spec.
5. Extend `contracts/openapi/admin.yaml`.
6. Add Admin read + curation sqlc queries.
7. Run generation.
8. Add transaction-safe Admin capability primitive.
9. Implement `internal/curation`.
10. Implement Application integration tests first:
   - CAS
   - no-op
   - capability
   - role race
   - publication
   - taxonomy
   - Source
   - Relation
   - cycle concurrency
11. Implement Admin curation transport.
12. Extend Admin middleware to Resource/Taxonomy routes.
13. Raise Admin process body limit while keeping Auth decoder limits.
14. Add Admin HTTP integration tests.
15. Refactor Admin router and Account page.
16. Build AdminShell and reusable Admin primitives.
17. Implement Resource List/Create.
18. Implement Resource Editor layout + Overview.
19. Implement Localizations.
20. Implement Tags.
21. Implement Sources.
22. Implement Relations.
23. Implement External IDs.
24. Implement Category/Tag pages.
25. Implement capabilities/query keys/Admin API wrapper.
26. Implement version conflict + dirty-form UX.
27. Add Admin curation source audit.
28. Extend existing `smoke:admin:dev`.
29. Update docs/contracts/CHANGELOG.
30. Run `pnpm generate`.
31. Run `pnpm check`.
32. Run `pnpm integration:ci`.
33. Run `pnpm build:images`.
34. Run `pnpm smoke:admin:dev`.
35. Re-check Goose version 6.
36. Re-check migration `00001..00006` hashes.
37. Re-check runtime DB privileges.
38. Inspect diff for P0-3/P0-4 scope creep.
39. Commit locally on `dev`.
40. Do not push unless explicitly authorized.

---

## 93. Commit Contract

Recommended commit:

```text
feat: add admin resource curation
```

Do not:

```text
merge main
tag
release
deploy production
```

Push `dev` only when explicitly authorized in the active workflow.

---

## 94. Final Codex Report

Report in Chinese.

### Implemented

Summarize:

```text
curation Application Layer
Admin Resource/Taxonomy API
Resource CAS
publication
localizations
tags
sources
relations
external IDs
Admin React curation UI
```

### Database Preservation

Confirm:

```text
Goose = 6
no 00007
00001..00006 unchanged
runtime grants unchanged
```

### Security / Capability

Report actual validation for:

```text
Moderator
Editor
Admin
role-change race
CSRF
ADMIN_ORIGIN
Admin session
```

### Concurrency

Report:

```text
stale expected_version
no-op
child rollback
Relation endpoint versions
Relation concurrent cycle test
```

### Admin → Public

Report actual:

```text
Draft → Public 404
Published → Public 200
Restricted/Removed → Public 404
Republished → Public 200
Soft deleted → Public 404
```

### UI

Report:

```text
Admin routes
Resource editor tabs
Taxonomy
capability UX
version conflict UX
dirty guard
Danger Zone
```

### Verification

List every command actually executed and PASS/FAIL, including:

```text
pnpm generate
pnpm check
pnpm integration:ci
pnpm build:images
pnpm smoke:admin:dev
```

Do not claim unexecuted commands passed.

### Regression

Confirm:

```text
P0-1
P0-2A
P0-2B
```

remain green.

### Git

Report:

```text
branch
commit SHA
git status
whether pushed
```

### Next

If all gates pass:

```text
P0-2C complete
P0-2 complete
Ready for P0-3 — Contribution & Review
```
