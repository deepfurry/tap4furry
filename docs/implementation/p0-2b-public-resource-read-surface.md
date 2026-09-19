# P0-2B — Public Resource Read Surface

**Repository:** `deepfurry/tap4furry`
**Target branch:** `dev`
**Baseline:** `9c2995ce86ac0872d598da885925454b18607f79`
**Baseline commit:** `feat: add resource domain foundation`
**Baseline CI:** GitHub Actions Run #8 — success
**Prerequisite:** P0-2A Resource Domain & Schema complete
**Status:** Codex implementation specification
**Date:** 2026-09-19

---

## 1. Goal

P0-2B makes the canonical **Resource Graph publicly readable** through:

```text
anonymous Public Go API
+
Astro SSR Resource pages
```

The phase answers:

> How can anonymous users safely, consistently and locally read published Resource knowledge without exposing internal governance state or coupling the read path to authentication, Redis, Worker or Admin?

P0-2B is a **public read-surface phase**. It does not introduce canonical mutation behavior.

---

## 2. Scope

P0-2B implements:

```text
Public API
├─ GET /resources
├─ GET /resources/{slug}
├─ GET /categories
└─ GET /tags

Public read SQL
└─ server/db/queries/public_resource.sql

Public transport
└─ server/internal/transport/public/resource.go

Astro SSR
├─ /resources
└─ /resources/[slug]

Public layout
Markdown rendering/sanitization
SEO basics
HTTP cache contract
Public read acceptance
shared-dev smoke
```

No new Resource schema is introduced.

---

## 3. Explicit Non-goals

Do not implement:

```text
Admin Resource CRUD
Contribution / Review
Search
Discovery ranking
Category detail pages
Tag detail pages
Filters
Custom sorting
Recommendation
Collections
Save / Want / Have
Exchange
Claims
Organizations
Comments / Discussions
Polls
Moderation workflow
Trust score
Notifications
Media/R2
Image gallery
Source availability probing
Analytics
Semantic/vector search
Sitemap
hreflang
localized route prefixes
JSON-LD
```

Do not introduce:

```text
new Redis keys
Resource cache in Redis
process-local Resource cache
River jobs
Worker Resource access
new PostgreSQL tables
new PostgreSQL columns
new PostgreSQL indexes via migration
```

---

## 4. Database Freeze

P0-2B must keep:

```text
Goose schema version = 6
```

The following migrations are immutable:

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

If P0-2B appears to require a schema change:

> Stop and report the conflict instead of silently changing the Resource Core.

P0-2B should prove that P0-2A is sufficient for a real public read surface.

---

## 5. Architecture Contract

Read path:

```text
Browser
   │
   ├── /resources
   └── /resources/{slug}
          │
          ▼
      Astro SSR
          │
          │ server-only HTTP
          ▼
   API_INTERNAL_ORIGIN
          │
          ▼
    Public Go API
          │
          ▼
   purpose-built sqlc
          │
          ▼
     PostgreSQL
```

Resource reads do not require:

```text
Public session
Auth
CSRF
Origin guard
OAuth
Redis
River
Worker
Admin API
```

The Public API process may still initialize its existing shared dependencies, but the Resource endpoint code path itself must not depend on them.

---

## 6. Domain Boundary

Keep:

```text
server/internal/resource
server/internal/taxonomy
```

as ordinary domain packages without persistence/transport dependencies.

Do not add imports from those packages to:

```text
pgx
sqlc
database
Fiber
OpenAPI generated DTOs
Redis
River
```

P0-2B query code may directly use purpose-built sqlc read queries.

Do not add ceremonial:

```text
ResourceRepository
ResourceReadRepository
ResourceServiceInterface
ResourceQueryServiceInterface
```

unless a real abstraction is required.

---

## 7. Public Resource Visibility

A Resource is publicly readable only when:

```sql
resources.deleted_at IS NULL
AND resources.publication_state = 'published'
```

Its Category must also satisfy:

```sql
categories.deleted_at IS NULL
```

Lifecycle does not control visibility.

All of these remain public if `publication_state=published`:

```text
active
inactive
discontinued
delisted
archived
unknown
```

Public detail must not distinguish:

```text
nonexistent
draft
pending
restricted
removed
soft-deleted
category soft-deleted
```

All return:

```text
404 RESOURCE_NOT_FOUND
```

Do not leak the existence of unpublished canonical entities.

---

## 8. Taxonomy Public Visibility

### Browse endpoints

`GET /categories` and `GET /tags` return only:

```text
state = active
AND deleted_at IS NULL
```

### Resource-linked taxonomy

When attached to an already-public Resource:

```text
active     → visible
retired    → visible
deleted    → hidden / invalid for read
```

`retired` means historical taxonomy, not erased knowledge.

---

## 9. Public Source Visibility

A Source may be returned when:

```text
availability_state IN (
  active,
  unavailable,
  broken,
  restricted
)
```

Hide:

```text
availability_state = removed
```

Public query allows only Sources whose `rights_status` is:

```text
unknown
creator_provided
confirmed
```

Hide:

```text
disputed
rights_review
removed_by_request
```

Public DTO must never expose `rights_status`.

Public Source DTO may expose:

```text
id
url
label
source_type
availability_state
is_primary
```

Do not expose:

```text
resource_id
rights_status
created_at
updated_at
```

---

## 10. Public Relation Visibility

A relation is visible only when both endpoints are public Resources.

A public Resource must not reveal a relation to:

```text
draft
pending
restricted
removed
soft-deleted
```

Resources.

Canonical stored relation vocabulary remains:

```text
part_of
successor_of
derived_from
related_to
```

Do not invent inverse stored/API types such as `predecessor_of` or `contains`.

Public relation DTO:

```text
type
direction
resource
```

Direction:

```text
outgoing
incoming
symmetric
```

For `related_to`, return `direction=symmetric`.

Do not expose relation row UUID.

---

## 11. External IDs

Resource Detail may return:

```text
namespace
external_id
```

from `resource_external_ids`.

Do not return them in Resource List.

Do not expose `created_at`.

---

## 12. Localization Contract

Public API accepts optional:

```text
?locale=
```

The input is parsed/canonicalized using existing:

```text
taxonomy.ParseLocale
```

Rules:

```text
locale omitted
→ entity default locale semantics

locale supplied
→ exact canonical locale match
→ per-field fallback to entity default locale
```

No language-family inference in P0-2B.

---

## 13. Field-level Fallback

For localized nullable fields:

```text
requested value
?? default value
```

independently.

For Resource:

```text
name
summary
description
```

For Category/Tag:

```text
name
description
```

A requested localization row existing does not disable fallback for its NULL fields.

---

## 14. Canonical Localization Corruption

P0-2A intentionally did not add circular FKs.

Therefore a manual/operator error can theoretically create:

```text
default_locale = en
but default localization row missing
```

Public Read must treat this as an internal invariant failure.

Do not return:

```text
404
empty localized name
partial invalid DTO
panic
```

Return:

```text
500 INTERNAL_ERROR
```

Log only a safe invariant message.

At least one disposable integration test must intentionally create this broken state.

Apply the same principle to missing canonical Category/Tag default localization.

---

## 15. Public API Endpoints

Modify:

```text
contracts/openapi/public.yaml
```

Add exactly:

```text
GET /resources
GET /resources/{slug}
GET /categories
GET /tags
```

Operation IDs:

```text
listResources
getResource
listCategories
listTags
```

All are anonymous.

Do not add session/CSRF requirements.

---

## 16. Query Parameters

Add reusable OpenAPI parameters.

### Locale

```text
name: locale
in: query
required: false
type: string
minLength: 2
maxLength: 64
```

True locale validity remains Go `taxonomy.ParseLocale`.

### Page

```text
name: page
in: query
required: false
integer
minimum: 1
default: 1
```

### Page Size

```text
name: page_size
in: query
required: false
integer
minimum: 1
maximum: 100
default: 24
```

Only `GET /resources` accepts Page/PageSize.

---

## 17. Error Contract

Reuse:

```text
VALIDATION_ERROR
INTERNAL_ERROR
```

Add exactly one public Resource error code:

```text
RESOURCE_NOT_FOUND
```

Behavior:

```text
invalid locale
invalid page
invalid page_size
→ 400 VALIDATION_ERROR

invalid/noncanonical slug syntax
nonexistent Resource
not-public Resource
→ 404 RESOURCE_NOT_FOUND

canonical data invariant broken
unexpected database/internal error
→ 500 INTERNAL_ERROR
```

Do not create `INVALID_LOCALE`, `INVALID_PAGINATION`, `RESOURCE_RESTRICTED` or `RESOURCE_REMOVED`.

---

## 18. Public DTOs

Add the following OpenAPI schemas:

```text
ResourceList
ResourceListItem
ResourceDetail

CategoryRef
TagRef
ResourceRef

ResourceSource
ResourceRelation
ResourceExternalID

CategoryList
CategoryItem

TagList
TagItem
```

Public enums:

```text
ResourceLifecycle
ResourceContentRating

ResourceSourceType
ResourceSourceAvailability

ResourceRelationType
ResourceRelationDirection
```

Do not expose these as Public DTO fields/types:

```text
PublicationState
RightsStatus
ResourceVersion
DeletedAt
```

---

## 19. ResourceList

Shape:

```text
items
page
page_size
has_next
```

No `total` or `total_pages`.

`ResourceListItem`:

```text
id
slug
name
summary
category
lifecycle
content_rating
published_at
updated_at
```

`summary` is required in JSON shape but nullable.

Do not include description/tags/sources/relations/external_ids/available_locales.

---

## 20. ResourceDetail

Shape:

```text
id
slug

requested_locale
default_locale
available_locales

name
summary
description

category
tags

lifecycle
content_rating

sources
relations
external_ids

published_at
updated_at
```

`summary` and `description` are required fields with nullable values.

Do not include:

```text
publication_state
version
deleted_at
rights_status
created_at
```

---

## 21. Reference DTOs

### CategoryRef

```text
id
slug
name
```

### TagRef

```text
id
slug
name
```

### ResourceRef

```text
id
slug
name
```

Do not merge these into one generic `TaxonomyRef`.

---

## 22. ResourceSource DTO

Fields:

```text
id
url
label
source_type
availability_state
is_primary
```

`label` is nullable.

Public Source availability enum contains only:

```text
active
unavailable
broken
restricted
```

Do not include `removed` in the Public enum.

---

## 23. ResourceRelation DTO

Fields:

```text
type
direction
resource
```

Types:

```text
part_of
successor_of
derived_from
related_to
```

Directions:

```text
outgoing
incoming
symmetric
```

No relation UUID. No inverse type field.

---

## 24. Category / Tag Browse DTO

### CategoryItem

```text
id
slug
name
description
```

### TagItem

```text
id
slug
name
description
```

`description` is required but nullable.

Responses are wrappers:

```text
CategoryList { items: [...] }
TagList      { items: [...] }
```

Do not return top-level arrays.

---

## 25. Public Read SQL

Create:

```text
server/db/queries/public_resource.sql
```

Expected named queries:

```text
ListPublicResources
GetPublicResourceBySlug
ListPublicResourceLocales
ListPublicResourceTags
ListPublicResourceSources
ListPublicResourceRelations
ListPublicResourceExternalIDs
ListPublicCategories
ListPublicTags
```

Do not add search/filter/ranking queries.

---

## 26. ListPublicResources

Public predicate:

```text
r.deleted_at IS NULL
r.publication_state = published
category.deleted_at IS NULL
```

Localization:

```text
Resource requested + default
Category requested + default
```

Use field-level fallback.

Prefer SQL rows instead of JSON aggregates.

Sort exactly:

```text
published_at DESC
id DESC
```

Pagination:

```text
LIMIT page_size + 1
OFFSET (page - 1) * page_size
```

Go returns first `page_size` rows and calculates `has_next`.

Use overflow-safe pagination math.

---

## 27. GetPublicResourceBySlug

Read:

```text
Resource core
localized Resource text
localized Category ref
```

Filter using the same Public predicate.

Invalid slug and missing/non-public Resource both map to `RESOURCE_NOT_FOUND`.

Do not expose publication state.

---

## 28. ListPublicResourceTags

Read Tag membership for Resource.

Allow:

```text
active
retired
```

Hide deleted Tags.

Localization:

```text
requested → default fallback
```

Sort:

```text
tag.slug ASC
```

Do not sort by localized name.

---

## 29. ListPublicResourceSources

Filter:

```text
availability_state <> removed

rights_status IN (
  unknown,
  creator_provided,
  confirmed
)
```

Do not SELECT rights status into public DTO.

Sort:

```text
is_primary DESC
source_type ASC
url ASC
id ASC
```

---

## 30. ListPublicResourceRelations

Prefer clear directional SQL, e.g. outgoing/incoming `UNION ALL`, over one hard-to-read giant CASE graph query.

For each branch, the other endpoint must satisfy the same Public Resource visibility predicate.

Return:

```text
relation_type
direction
other Resource id/slug/localized name
```

For `related_to` use `direction=symmetric`.

Sort deterministically, e.g. relation_type/direction/other.slug.

---

## 31. ListPublicResourceExternalIDs

Return:

```text
namespace
external_id
```

Sort:

```text
namespace ASC
external_id ASC
```

---

## 32. ListPublicResourceLocales

Return locales from `resource_localizations` for the Resource.

Sort `locale ASC`.

This represents Resource localization availability only.

Do not imply Category/Tag translations exist for the same locale.

---

## 33. ListPublicCategories / Tags

Filter:

```text
state = active
deleted_at IS NULL
```

Locale fallback:

```text
requested → default
```

Sort `slug ASC`.

Do not add resource_count/popularity/usage_count.

---

## 34. Public Transport

Add:

```text
server/internal/transport/public/resource.go
```

Keep the adapter thin.

It may:

```text
parse/validate query parameters
canonicalize locale
call sqlc queries
assemble OpenAPI DTOs
map known errors to stable Public API errors
set cache headers
```

Do not put Resource mutation/business workflows here.

Do not make `server/internal/resource` depend on sqlc.

---

## 35. Handler Composition

Extend existing Public `Handler` with a read-only resource reader backed by the existing Public API pgx pool.

Keep Auth and Identity behavior unchanged.

The Resource routes must receive a 5-second request context deadline.

Add:

```text
/resources
/categories
/tags
```

to ordinary Public request deadline coverage.

Do not put these routes behind originGuard/csrfGuard/oauthBoundary/session resolution.

---

## 36. Public API Cache Headers

Successful anonymous Resource endpoints:

```http
Cache-Control: public, max-age=0, s-maxage=60, stale-while-revalidate=30
```

Errors (`400`, `404`, `5xx`) must return:

```http
Cache-Control: no-store
```

Do not introduce application cache state.

---

## 37. API Client Generation

Continue spec-first generation.

Run:

```text
pnpm generate
```

Generated artifacts remain committed.

Do not hand-edit:

```text
server/internal/transport/public/generated/*
packages/api-client/src/generated/public/client.ts
```

Update:

```text
packages/api-client/src/public.ts
```

to re-export needed Resource functions/types and URL builders.

SSR should reuse generated URL builders rather than maintaining a second endpoint string contract.

---

## 38. Astro SSR Pages

Add only:

```text
apps/web/src/pages/resources/index.astro
apps/web/src/pages/resources/[slug].astro
```

Do not add Category/Tag detail pages in P0-2B.

Both pages are SSR.

Resource pages must contain:

```text
zero React islands
zero hydration directives
```

No Resource page component should use `client:*` hydration.

---

## 39. PublicLayout

Create:

```text
apps/web/src/layouts/PublicLayout.astro
```

Do not reuse `AccountLayout.astro`.

`PublicLayout` owns:

```text
doctype/html shell
html lang
title
meta description
canonical
Open Graph basics
robots
public navigation shell
```

Set Astro site origin:

```text
https://tap4furry.com
```

in Astro config.

Do not add fake `og:image`.

---

## 40. Web Locale Behavior

The Public API contract remains:

```text
locale omitted
→ entity.default_locale
```

The Web UX default is:

```text
en
```

Therefore Astro pages without `?locale=` should explicitly request `locale=en` from the Public API.

If English localization is unavailable, API field-level fallback naturally returns canonical default locale content.

When `?locale=` is present, canonicalize and use it.

Carry locale into Resource detail links.

Do not carry pagination into detail links.

---

## 41. Server-only API Adapter

Create:

```text
apps/web/src/lib/public-api.server.ts
```

Read:

```text
API_INTERNAL_ORIGIN
```

Production must configure it explicitly.

Development may default to:

```text
http://127.0.0.1:8080
```

Do not name this with Astro `PUBLIC_` prefix.

Never expose internal origin to browser code.

---

## 42. Browser API vs Internal SSR API

Generated browser-facing URLs remain:

```text
/api/*
```

SSR adapter must:

```text
reuse generated URL builder
strip exactly leading /api
join against API_INTERNAL_ORIGIN
```

Example:

```text
/api/resources?page=1
↓
http://127.0.0.1:8080/resources?page=1
```

Do not make Astro SSR call the public external domain through Cloudflare/Nginx.

Do not duplicate endpoint path construction manually.

---

## 43. SSR Request Privacy

Astro → Go Resource requests must not forward:

```text
Cookie
Authorization
CSRF
Public session
browser-specific auth state
```

Resource SSR is user-independent anonymous content.

Send only required HTTP request information such as `Accept: application/json` and query/path values.

---

## 44. SSR Timeout / Upstream Errors

Use a 5-second upstream timeout.

No retries.

Map:

```text
API 200
→ page 200

Resource 404
→ Astro 404

validation 400
→ Astro 400

timeout
API 5xx
invalid upstream response
→ Astro 503
```

Error pages:

```text
noindex
Cache-Control: no-store
```

Do not display upstream response bodies or internal origin values.

---

## 45. Markdown

Resource `description` remains canonical Markdown source from Public API.

Go API must not render HTML.

Astro SSR renders Markdown server-side.

Add:

```text
markdown-it
sanitize-html
```

No React Markdown library.

---

## 46. Markdown Server Boundary

Create:

```text
apps/web/src/lib/markdown.server.ts
apps/web/src/components/Markdown.astro
```

Pipeline:

```text
Markdown source
↓
markdown-it
↓
sanitize-html allowlist
↓
Markdown.astro
↓
single audited set:html sink
```

Configure Markdown parser conservatively:

```text
html: false
linkify: false
typographer: false
```

---

## 47. Markdown Allowed Content

Allow:

```text
paragraph
h2-h6
bold
italic
strikethrough
ordered/unordered list
blockquote
inline code
code block
link
horizontal rule
table
```

Do not allow/output:

```text
raw HTML
h1
img
iframe
svg
style
script
video
audio
object
embed
MDX/custom components
```

Resource images belong to future Media/R2 work.

Markdown image syntax must not produce `<img>`.

---

## 48. Markdown Link Safety

Allow link protocols:

```text
http
https
relative URL
```

Reject:

```text
javascript
data
file
vbscript
```

Markdown external links should receive:

```text
rel="ugc nofollow noreferrer"
```

Do not force `target="_blank"`.

Canonical Resource Source links are not treated as Markdown UGC links.

---

## 49. set:html Audit

P0-2B should strengthen repository audit:

`apps/web` `set:html` usage is allowed only in the dedicated Markdown component.

Do not permit arbitrary pages/components to bypass sanitizer.

Also prevent client/browser component code from importing:

```text
public-api.server.ts
markdown.server.ts
```

or other future server-only Resource modules.

Do not weaken existing audits.

---

## 50. Resource List Page

`/resources` renders:

```text
public header/navigation
page title/intro
Resource cards
Previous / Next
```

Resource card displays:

```text
name
summary
category
lifecycle
content rating
```

Do not include tags/sources/description/relations/external IDs/actions/view counts.

Resource link:

```text
/resources/{slug}
```

Preserve locale query if one is active.

---

## 51. Resource Detail Page

`/resources/[slug]` renders:

```text
breadcrumb/back to Resources

name
summary

category
tags

lifecycle
content rating

description Markdown

sources
relations
external IDs
```

Do not add Save/Want/Have/Claim/Report/Comments/Recommendations/Share count/View count.

Explicit Resources remain public when published and are marked clearly with the content rating.

Do not build an age-verification state machine in P0-2B.

---

## 52. SEO

### Detail

Title:

```text
{Name} · Tap4Furry
```

Meta description:

```text
summary
```

or if null:

```text
Discover {Name} on Tap4Furry.
```

Canonical:

```text
https://tap4furry.com/resources/{slug}
```

`?locale=` never enters canonical URL.

### List

Page 1 canonical:

```text
https://tap4furry.com/resources
```

Page N (`N > 1`):

```text
https://tap4furry.com/resources?page=N
```

Locale query is excluded from canonical.

Do not implement hreflang/localized canonical paths/sitemap/JSON-LD in P0-2B.

---

## 53. HTML lang

Resource pages with explicit locale:

```text
<html lang="{canonical requested locale}">
```

Without explicit locale, Web default is `en`.

Do not track per-field fallback language in HTML in P0-2B.

---

## 54. SSR Cache Contract

Successful anonymous Resource pages:

```http
Cache-Control: public, max-age=0, s-maxage=60, stale-while-revalidate=30
```

Errors (`400`, `404`, `5xx`) return:

```http
Cache-Control: no-store
```

Do not use Redis Resource cache, Astro process LRU, Go memory cache, ETag persistence or cache invalidation tables.

No `Vary: Accept-Language`.

Locale is explicit URL state.

---

## 55. Public API Integration Tests

Use disposable PostgreSQL fixture data and the real Public Handler.

At minimum cover:

```text
published                    → 200
draft                        → 404
pending                      → 404
restricted                   → 404
removed                      → 404
soft-deleted                 → 404

published + discontinued     → 200
published + explicit         → 200

retired Category             → Resource visible
deleted Category             → Resource hidden

active Tag                   → visible
retired Tag                  → visible
deleted Tag                  → hidden

Source active                → visible
Source unavailable           → visible
Source broken                → visible
Source restricted            → visible
Source removed               → hidden

rights unknown               → Source visible
rights creator_provided      → visible
rights confirmed             → visible
rights disputed              → hidden
rights_review                → hidden
removed_by_request           → hidden

relation public→public       → visible
relation public→draft        → hidden
relation public→restricted   → hidden

requested locale exists      → requested content
partial translation          → field fallback
missing requested locale     → default fallback
invalid locale               → 400

invalid slug                 → 404

page_size+1                  → has_next=true
final page                   → has_next=false
empty list                   → 200 with []
```

---

## 56. Public DTO Privacy Tests

Assert actual serialized Public responses do not contain:

```text
publication_state
version
deleted_at
rights_status
created_at
```

where the contract excludes them.

Do not rely only on generated schema.

---

## 57. Canonical Corruption Tests

Disposable-only test:

Create a Published Resource with:

```text
default_locale=en
```

then remove/break its `en` canonical localization using migrator fixture authority.

Public Detail must return:

```text
500 INTERNAL_ERROR
```

not 404/panic/empty name.

Cover at least one broken canonical Category/Tag localization case too.

---

## 58. Shared Development Smoke

Add:

```text
pnpm smoke:public-read:dev
```

Follow the existing smoke style.

Do not require manually starting API/Web servers.

Suggested shape:

```text
scripts/smoke-public-read-dev.mjs
server/cmd/public-read-smoke
```

The smoke may compose a local Fiber app in-process, using prepared `gfp_api`.

Use migrator only for this run's random fixture creation/cleanup when needed.

Never print private DSNs.

---

## 59. Shared Smoke Fixture

Create random UUIDv7 fixture data including:

```text
active Category + localization
active/retired Tags
published Resource
hidden draft/restricted Resource
requested/default localizations
visible/hidden Sources
visible/hidden Relations
External ID
enough rows to exercise pagination
```

Test through real Public HTTP endpoints:

```text
GET /resources
GET /resources/{slug}
GET /categories
GET /tags
```

Verify:

```text
status
visibility
locale fallback
pagination
Source filtering
Relation privacy
DTO privacy
cache headers
```

Cleanup only this random fixture using migrator in FK-safe order.

Do not modify shared roles, schema or existing data.

---

## 60. Astro SSR Acceptance

Add CI-level SSR acceptance independent of shared development infrastructure.

Use a local fake Public API implementing the expected OpenAPI response contract.

This test targets:

```text
Astro server rendering
server-only API adapter
status mapping
Markdown security
SEO
cache headers
pagination links
no hydration
```

It does not replace Go API integration tests.

---

## 61. Astro Acceptance Matrix

Verify:

```text
/resources
→ Resource cards appear in SSR HTML

/resources/test-resource
→ detail appears in SSR HTML

API Resource 404
→ Astro 404

API validation 400
→ Astro 400

API 5xx
→ Astro 503

upstream timeout
→ Astro 503

Markdown raw HTML/script
→ not rendered

javascript: link
→ removed/rejected

Markdown image
→ no <img>

Resource pages
→ no astro-island generated for Resource UI

canonical
→ correct

locale
→ forwarded/canonicalized as expected
→ html lang correct
→ canonical excludes locale

page=2
→ canonical correct
→ previous/next correct

success
→ shared-cache header

errors
→ no-store
```

No CI call may reach gfp_dev/Tailscale/tap4furry.com/Cloudflare/real OAuth providers/Resend.

---

## 62. P0-1 Regression

P0-2B changes the Public OpenAPI and Public Handler.

All existing P0-1 regression tests must remain green.

Do not break:

```text
registration
login/logout
verification
recovery
sessions
CSRF
OAuth
/me
/users/{handle}
Admin isolation
```

New Resource routes must remain outside auth/CSRF/origin enforcement where safe GET semantics require anonymity.

---

## 63. CI Integration

Keep outer CI workflow:

```text
pnpm check
pnpm generate
generated drift
pnpm integration:ci
pnpm build:images
```

Extend internal test coverage for P0-2B.

Expected:

```text
existing P0-1 integration
existing P0-2A resource core integration
new P0-2B Public API integration
new Astro SSR acceptance
```

No migration version change.

---

## 64. Dependencies

P0-2B may add to `apps/web`:

```text
markdown-it
sanitize-html
```

and required TypeScript typings if needed.

Do not add a React Markdown stack.

Do not add large UI frameworks.

Keep dependency additions minimal and auditable.

---

## 65. Documentation

Add this implementation document:

```text
docs/implementation/p0-2b-public-resource-read-surface.md
```

Update only relevant docs/contracts, likely including:

```text
README.md
CHANGELOG.md
contracts/architecture.md
contracts/database.md
contracts/openapi/public.yaml
docs/architecture/backend.md
docs/architecture/data.md
docs/development.md
docs/product/domain-model.md
```

Record:

```text
P0-2B Public read endpoints
public visibility rules
public DTO privacy
locale fallback
SSR architecture
Markdown security boundary
cache/SEO contract
P0-2C remains next
```

Do not rewrite unrelated P0-1/P0-2A history.

---

## 66. Repository Audit

Preserve existing P0-2A domain audits.

Add/strengthen:

```text
single set:html sink
server-only Resource helper boundaries
no transport/database dependency added into resource/taxonomy domain
```

Do not loosen secret/Tailnet/dependency/generated-file audits.

---

## 67. Acceptance Gate

P0-2B is complete only when all are true.

### Database

```text
Goose version remains 6                    PASS
no migration 00007                         PASS
00001..00006 unchanged                     PASS
```

### SQL

```text
public_resource.sql exists                 PASS
visibility predicate correct               PASS
field-level locale fallback                PASS
Source governance filter                   PASS
Relation endpoint privacy                  PASS
no search/ranking scope creep              PASS
```

### OpenAPI

```text
4 anonymous GET endpoints                  PASS
DTO shape frozen                           PASS
RESOURCE_NOT_FOUND                         PASS
internal governance fields absent          PASS
generated code in sync                     PASS
```

### Go Public API

```text
GET /resources                             PASS
GET /resources/{slug}                      PASS
GET /categories                            PASS
GET /tags                                  PASS
5s Resource request bound                  PASS
success cache headers                      PASS
error no-store                             PASS
canonical corruption → 500                 PASS
P0-1 regression                            PASS
```

### Astro

```text
/resources                                 PASS
/resources/[slug]                          PASS
PublicLayout                               PASS
server-only internal API adapter           PASS
no session/cookie forwarding               PASS
zero Resource React hydration              PASS
status mapping                             PASS
```

### Markdown

```text
server-only renderer                       PASS
raw HTML disabled                          PASS
sanitize allowlist                         PASS
unsafe link protocols removed              PASS
no Markdown image output                   PASS
single audited set:html sink               PASS
```

### SEO

```text
title/description                          PASS
Resource canonical                        PASS
pagination canonical                      PASS
locale excluded from canonical             PASS
errors noindex                             PASS
```

### Cache

```text
200 short shared cache                     PASS
400/404/5xx no-store                       PASS
no Redis/application Resource cache        PASS
```

### Acceptance

```text
disposable API matrix                      PASS
DTO privacy                                PASS
canonical corruption case                  PASS
Astro SSR acceptance                       PASS
pnpm smoke:public-read:dev                 PASS
pnpm generate                              PASS
pnpm check                                 PASS
pnpm integration:ci                       PASS
pnpm build:images                          PASS
generated drift                           PASS
```

---

## 68. Stop Conditions

Stop and report rather than broadening scope if:

1. a schema change or migration 00007 appears necessary;
2. implementing reads appears to require Redis/Worker/River;
3. Resource/Taxonomy domain would need database/transport imports;
4. Public API needs to expose `rights_status`, `version`, `publication_state` or `deleted_at`;
5. relation reads cannot avoid leaking unpublished endpoints;
6. Astro SSR appears to require forwarding user sessions;
7. Markdown rendering would require raw HTML/MDX;
8. Resource pages begin adding React hydration without real interaction;
9. P0-4 search/filter/ranking behavior begins entering P0-2B;
10. Admin CRUD or Contribution work begins entering this phase;
11. shared development requires any DB down/schema mutation;
12. application-level cache invalidation machinery becomes necessary.

---

## 69. Recommended Implementation Order

1. Confirm clean `dev` at `9c2995ce...` or a clearly equivalent newer baseline.
2. Run baseline `pnpm check`.
3. Verify migration `00001..00006` hashes.
4. Add this implementation document.
5. Extend `contracts/openapi/public.yaml`.
6. Add `server/db/queries/public_resource.sql`.
7. Run `pnpm generate`.
8. Update `@tap4furry/api-client/public` exports.
9. Implement Public Resource read adapter/handlers.
10. Extend Public route timeout handling.
11. Add Public API integration matrix.
12. Add canonical-corruption tests.
13. Add `PublicLayout.astro`.
14. Add `public-api.server.ts`.
15. Add Markdown dependencies and server renderer.
16. Add `Markdown.astro`.
17. Add `/resources`.
18. Add `/resources/[slug]`.
19. Add SEO/cache/status mapping.
20. Strengthen repository audit.
21. Add Astro SSR acceptance.
22. Add `pnpm smoke:public-read:dev`.
23. Update docs/contracts/CHANGELOG.
24. Run `pnpm generate`.
25. Run `pnpm check`.
26. Run `pnpm integration:ci`.
27. Run `pnpm build:images`.
28. Run `pnpm smoke:public-read:dev`.
29. Re-check migration `00001..00006` hashes and Goose version 6.
30. Inspect diff for P0-2C/P0-4 scope creep.
31. Commit locally on `dev`.
32. Do not push unless explicitly asked.

---

## 70. Commit Contract

Recommended commit:

```text
feat: add public resource read surface
```

Do not:

```text
merge main
tag
release
deploy production
```

If the user explicitly asks Codex to push the finished `dev` commit, follow that instruction; otherwise keep the implementation local.

---

## 71. Final Codex Report

Report in Chinese.

### Implemented

Summarize:

```text
Public Resource SQL read model
4 Public API endpoints
OpenAPI DTOs
locale fallback
Source/Relation privacy
Astro SSR Resource pages
Markdown sanitizer
SEO/cache
shared-dev smoke
```

### Database Preservation

Confirm:

```text
Goose remains version 6
no migration 00007
00001..00006 unchanged
```

### Public Privacy

Confirm actual responses omit:

```text
publication_state
version
deleted_at
rights_status
```

### Verification

List every command actually run and result, including:

```text
pnpm generate
pnpm check
pnpm integration:ci
pnpm build:images
pnpm smoke:public-read:dev
```

Do not claim unexecuted commands passed.

### Astro

Report:

```text
/resources
/resources/[slug]
React island count
Markdown security tests
SEO canonical tests
cache/status tests
```

### Regression

Confirm P0-1 and P0-2A integration remain green.

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
P0-2B complete
Ready for P0-2C — Admin Resource Curation
```
