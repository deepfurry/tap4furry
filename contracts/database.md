# Database contract

PostgreSQL 18.x is the canonical source of truth. `app` is the application schema;
`river` is isolated job infrastructure. Redis is ephemeral, never canonical storage.

Goose SQL migrations under `server/db/migrations` are the sole application schema
source. No duplicate `schema.sql`, ORM, GORM or AutoMigrate. Never modify a released
or shared-environment-applied migration in place: add a new migration. Runtime
startup must not migrate. Explicit migration order is Goose then official River
`rivermigrate` using its explicit `Schema` option; never copy/rewrite River SQL.

The explicit migrator first ensures `app` exists so Goose can create
`app.goose_db_version` before applying migration 1. This bookkeeping bootstrap is
necessary on a fresh database and respects the prepared migrator's lack of CREATE
on `public`. All subsequent application schema evolution remains Goose-owned.

`pg_trgm` is required in `public`; `vector` must not be enabled. Migration 1 remains
immutable. Migration 2 owns identity, profile, credential and session tables in
`app`. Goose may create namespaces, but River owns objects inside `river`.

sqlc consumes Goose migrations and `server/db/queries`, generating committed pgx/v5
code under `server/internal/database/sqlc`. Never hand-edit generated code.

Development roles:

| Role | Purpose |
| --- | --- |
| `gfp_migrator` | Explicit Goose/River migrations and owned-object grants |
| `gfp_api` | Public API runtime |
| `gfp_admin` | Admin API runtime |
| `gfp_worker` | Worker and River runtime |
| `gfp_readonly` | Operator inspection only |

Only the migrator accesses migration machinery. Worker gets River object runtime
privileges, not ownership/DDL. Public/Admin enqueue grants wait until actually used.
Cluster roles and shared server configuration are operator-owned; missing privileges
are a stop condition, never grounds for SSH, superuser use or widening Redis ACLs.

P0-1A grants API only the SELECT/INSERT/UPDATE rights its use cases need; Admin and
Worker receive no identity DML. Readonly may SELECT. IDs are generated in Go with
standard-library UUIDv7. Restrictive FKs, CHECKs and UNIQUE constraints enforce the
account/identity/profile/session invariants; public views never SELECT credentials.

Migration 3 adds `auth_challenges` and `security_events`. Challenges have one
unconsumed/non-invalidated row per identity/purpose, enforced by a partial unique
index. Indexed token hashes identify challenges; consumption is revalidated inside
the transaction. Auth mutations lock the User, then the credential row when needed,
before identity/session/challenge writes, and run password KDFs outside transactions.
Reset/change revoke all Public/Admin sessions and create one Public replacement atomically with the event.

API may SELECT/INSERT/UPDATE challenges, INSERT events and update identity
verification timestamps. It cannot read/update events or access their identity
sequence directly. Admin/Worker gain no rights; readonly may SELECT. Event session
IDs are historical references without a session FK; event users retain restrictive
FKs. Events have no arbitrary metadata/email/credential fields.

Migration 4 extends the event CHECK and constrains session auth methods to password,
password_reset, google and github. One partial unique index enforces one linked
identity per User/provider. OAuth-only Users still have an email identity and profile,
but no password credential. API receives only owned-object UPDATE(updated_at) on
users for row locking, UPDATE(email) on identities for private metadata, and DELETE
on identities for unlinking. Runtime SQL restricts deletion to Google/GitHub rows.
These object grants use the prepared migrator; cluster roles remain unchanged.
OAuth state/PKCE/nonce and provider tokens never enter application tables.

Migration 5 adds only `app.user_roles` (User FK, fixed role CHECK, composite primary
key) and extends the closed event CHECK. It clears inherited default table grants
before granting Admin/readonly SELECT. API and Worker receive no role privileges.
Admin may read users/email identities/credentials/roles/sessions, insert sessions
and events, update session activity/revocation, lock users via UPDATE(updated_at),
and CAS-upgrade credential password_hash/updated_at. Admin cannot change password
epochs, identities, roles, profiles, challenges or account state. Identity sequence
access is unnecessary. Migrations 1–5 are now immutable on shared development.

Operator role changes serialize with a fixed transaction advisory lock before the
target User lock. Last-active-admin checks, role writes, final-role Admin session
revocation and role events commit together. Admin login/reauth lock User then
credential and revalidate password, account, role and initiating session before
writing; KDFs remain outside transactions. No role or capability cache exists.

## P0-2A Resource Core

Migration `00006_resource_core.sql` owns exactly ten new tables:
`categories`, `category_localizations`, `tags`, `tag_localizations`, `resources`,
`resource_localizations`, `resource_tags`, `resource_sources`, `resource_relations`
and `resource_external_ids`, all in `app`. Migrations 1–5 remain byte-for-byte intact.
IDs come from Go UUIDv7; pure mappings use composite keys. FKs use RESTRICT, states
use TEXT/CHECK, times use explicit timestamptz. No ENUM/JSONB/EAV/RLS/triggers or seeds.

Category/Tag slugs (1–64) cannot be changed by Admin. Resource slugs (1–80) freeze
after first publication through domain/application policy. Parents soft-delete;
Sources use availability=removed; memberships/relations/external IDs delete physically.
Public visibility and lifecycle/rating remain independent. Rating and Resource
version require explicit values; new Resources start at version 1. Published requires
published_at. No search indexes are introduced.

Locale columns have defensive shape/length checks and per-entity lower(locale)
uniqueness. Go `x/text/language` canonicalizes input. The parent and default locale
row commit together; parent locking protects default switches and localization
deletion. No circular FK exists. P0-2B will use field-level requested→default fallback.

Resource version anchors Resource-owned canonical knowledge, including children,
state and soft delete. A logical mutation increments once per affected Resource;
relations affect both endpoints, locked in UUID order. sqlc compare-and-bump rejects
stale expected versions and deleted rows. Full mutation orchestration is P0-2C.

Source URL normalization never fetches; `(resource_id,url)` is unique, with at most
one primary per Resource, independently of availability/rights. `related_to` orders
UUID endpoints; the three directed relation types keep direction. Cycle checks are
future application work. External IDs are globally unique by `(namespace,external_id)`.

Migration 6 first clears inherited default grants on its own tables. API and readonly
receive SELECT on all ten; Worker receives nothing. Admin gets SELECT/INSERT, exact
mutable-column UPDATE grants, and DELETE only on localization/mapping/relation tables.
No Admin hard DELETE exists for Category/Tag/Resource/Source, and IDs/created_at and
relationship identities cannot be updated. No cluster-role/default-privilege changes.

Only a `_test.go` migration helper can perform the migration round-trip. It requires
`CI=true`, `GFP_DISPOSABLE_INFRA=1`, `GFP_RESOURCE_INTEGRATION=1`, fixed loopback
`gfp_ci`/migrator identity and exactly current/target version 7 (7→6→5→7). Shared `migrate:dev`
remains up-only. Resource smoke cleanup uses migrator only and randomly owned fixture
IDs in FK-safe order; it never rolls back shared schema or touches existing Resources.

## P0-2B read model (schema frozen at 6)

Migrations 00001–00006 are immutable; P0-2B creates no migration 00007 or schema,
index, grant or role changes. `public_resource.sql` is the sole Public Resource
query source. API uses the existing SELECT grants; Worker still has no Resource access.

Public Resources require published + not deleted + Category not deleted; lifecycle
and content rating do not hide published knowledge. Browse taxonomy requires active,
while linked retired taxonomy stays visible. Deleted tags are omitted. Sources omit
removed availability and allow only unknown/creator_provided/confirmed rights.
Relations require a public other endpoint and preserve stored vocabulary with
outgoing/incoming/symmetric direction; no hidden endpoint or relation-row ID leaks.

Requested locale matches exactly after canonicalization, with independent nullable
field fallback to each entity's default. No language-family matching. LEFT JOIN plus
explicit canonical-row checks distinguish corrupt defaults (500 INTERNAL_ERROR) from
hidden/missing Resources (404 RESOURCE_NOT_FOUND), including Category/Tag defaults.
Read DTOs never expose publication_state/version/deleted_at/rights_status/created_at.
Resource lists sort published_at DESC then id DESC, fetch page_size+1 with overflow-safe
offset and return has_next without counts. Detail child reads share a read-only
repeatable-read snapshot so concurrent visibility changes cannot split the graph.

## P0-2C writes (schema and grants frozen at 6)

No migration, new index or grant accompanies curation. `curation.sql` contains only
purpose-built queries using the existing precise Admin DML. Canonical timestamps
come from transaction_timestamp(). The actor User is locked before all target locks;
Auth rechecks active Admin session and current static capabilities in READ COMMITTED.
Resource parent locks and expected_version comparisons precede owned child diffs.
The final compare-and-bump remains inside the transaction, including soft deletion;
no-op leaves all revision/timestamp values unchanged. A changed relation increments
both endpoints exactly once, after fixed advisory graph lock and UUID row lock order.
Recursive cycle checks traverse only the same directed relation_type.

Category/Tag mutations and new bindings lock taxonomy parents. Existing retired
bindings may remain; new retired/deleted bindings fail. Default translations must
exist before switching, and the current default cannot be deleted. In-use checks
prevent taxonomy soft deletion while non-deleted Resources reference it. Sources
are retained via availability=removed; setting primary clears the previous primary
in the same transaction. External IDs remain globally unique and never auto-transfer.

## P0-3A proposal schema (migration 7)

`contributions`, `contribution_contents`, `contribution_initial_sources`,
`contribution_events` and `contribution_review_audits` are five additional tables.
Migrations 1–6 and all existing object grants remain unchanged. The migration clears
inherited privileges on its five new tables before explicit grants; no cluster changes.

| Role | New-table grants |
| --- | --- |
| API | SELECT proposal/content/source; SELECT only contribution_id/event_type/message/occurred_at on events; INSERT proposal input columns/content/source and public event columns; UPDATE only proposal status/decided_at |
| Admin | SELECT all five; INSERT content/source/event/audit; UPDATE only proposal status/decided_at/result_resource_id/result_version |
| Worker | None |
| Readonly | SELECT all five; no writes |

No runtime has UPDATE on snapshots/history/audits or DELETE on proposal objects.
The unique author/request_id and stored normalized SHA-256 digest support replay.
Closed submitted_fields bits (name=1, summary=2, description=4, category=8,
lifecycle=16, rating=32, locale=64) preserve omission versus explicit null for author
projection; complete proposed snapshots serve review only. Source belongs to new
Resource proposals only. IDs/defaults/terminal-result checks and one terminal event
prevent duplicate decisions; application locks enforce cross-table state transitions.

Proposal quota is 10 per rolling 24 hours, 5 pending and a 60-second interval, under
the author User lock. Same-key same-content replay precedes quotas; changed content
with the same key conflicts. An opaque HMAC binds edit context to User/Resource/
version/default locale; no raw context identifier is stored. Public needs no Resource
UPDATE/locking grant. Review reads the recorded version under canonical locks and
commits canonical write, accepted content, decision event and audit atomically.
