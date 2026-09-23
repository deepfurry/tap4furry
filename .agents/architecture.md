# Architecture

Public/Admin are distinct transport and security boundaries, sharing one Go module.

```text
apps/web / apps/admin → generated API clients → Public/Admin transport
                                                ↓
                                   auth / identity / curation
                                                ↓
                                           sqlc / pgx
                                                ↓
                                           PostgreSQL
worker → jobs adapter → shared Application/Domain (when implemented)
```

P0-1A/B/C/D add local/OAuth auth, PostgreSQL sessions, recovery, basic profiles and isolated Admin auth to P0-0 infrastructure.
P0-2A adds pure `taxonomy` and `resource` domain packages alongside `auth` and `identity`.
P0-2B adds anonymous Public read HTTP/SSR; P0-2C implements canonical Admin curation. `cmd/*` composes dependencies, signals and
bounded cleanup; reusable behavior lives in `internal/`.

Auth owns the challenge-mail interface; `internal/mail` implements post-commit local
capture and Resend delivery with a five-second budget, no queue/retry and safe errors.
Only Public API composes mail; production explicitly requires Resend. Challenges
persist only easyhash token hashes; security events accept only
IDs, a closed event type and time. Public unsafe authenticated routes require exact
Origin and a session-bound HMAC CSRF value; rotation invalidates the previous value.

Auth owns the narrow OAuth provider and ephemeral-flow interfaces. `oauthprovider`
uses OAuth2/OIDC libraries and returns validated identity fields, never provider
tokens. `redisstore` implements one-use, ten-minute flows under `gfp:auth:oauth:flow:`.
Account resolution uses provider subjects; email collisions require explicit linking.
All existing-user auth mutations take the User lock before credential/identity/
session/challenge locks. Provider network I/O and KDFs stay outside transactions.

Admin uses password-only `kind=admin` sessions (8h absolute, 1h idle, 5m touch),
Strict cookies and independent Origin/CSRF. Current static roles are read on every
request. `adminctl` role writes use the owner, an operator advisory lock then User
lock, with last-active-admin protection. Reset/change revoke both session kinds.
Auth owns subject/global throttle policies; Redis stores HMAC-keyed counters/TTLs.
Public local auth fails open on Redis outage; Admin login/reauth fail closed.

Worker never calls Public/Admin HTTP. Application/domain code must not import Fiber,
transport DTOs, Redis or River. River types stay inside Jobs infrastructure and its
objects live in `river`; business data lives in `app`. PostgreSQL is canonical,
Redis holds disposable state. No ORM, AutoMigrate, vectors, MongoDB or message broker.

`curation.App` owns READ COMMITTED canonical mutations. Auth's transaction capability
primitive locks the actor User and rechecks the active Admin session and live roles.
Resource CAS is checked under the parent lock before child diffs; changes bump once,
no-ops preserve version and updated_at. Graph writes additionally use one fixed
advisory xact lock and UUID-ordered endpoint locks, with separate directed-type DAG
checks. Taxonomy row locks serialize binding/retirement/deletion decisions. All
canonical timestamps use transaction_timestamp(); Resource Core grants remain those of migration 6.
Admin read snapshots use purpose-built sqlc, without rebuilding domain objects.

Admin React routes separate Resources, six editor tabs, Taxonomy and Account.
Static capability helpers only guide UX; the backend is authoritative. Mutations
have no retries or optimistic updates, and refetch canonical snapshots after success.
Version conflicts preserve drafts until explicit reload; dirty forms block navigation
and unload. Shared native dialog primitives protect typed soft deletion.

Resource Core is ten relational tables in `app`, owned by Goose migration 6. Domain
primitives normalize locale/text/URLs and canonicalize symmetric relations without
I/O. sqlc provides parent locks and optimistic version bumps. A transaction creates
the parent plus its default localization and bumps each affected Resource once;
relation edits lock both endpoints in UUID order. No circular localization FK,
trigger, queue, seed taxonomy or Resource Worker access exists. Acceptance fixtures
under `database/resourcecheck` and `database/publicreadcheck` are developer-only,
never runtime dependencies.

Public Resource reads call purpose-built sqlc through the API pool, without Auth,
Redis or worker dependencies. Detail uses one read-only repeatable-read snapshot;
SQL filters hidden nodes/Source governance/Relation endpoints and signals missing
canonical localizations. Public DTOs omit internal governance fields. Anonymous Astro
SSR calls `API_INTERNAL_ORIGIN` with generated URL builders and no browser credentials.
Markdown is rendered/sanitized server-side into exactly one audited HTML sink; these
pages have no React islands. P0-6 sets all four reads and Resource SSR to no-store,
including successes, so new requests observe committed governance.

OpenAPI owns Go transport and TypeScript clients. Goose migrations plus SQL queries
own sqlc output. Generated files are committed, reviewed, and never manually edited.

Contribution application transactions add a separate typed proposal lifecycle at
migration 7. Public uses only its own pool; new-table DML does not expand canonical
Resource privileges. Author User locks serialize quota/idempotency and revocation.
Review locks reviewer User then proposal then canonical parents, rechecks Editorial
and rejects self-review. `curation.ApplyReviewedTx` independently revalidates capability
and joins without committing. Canonical mutations, accepted snapshot, terminal event
and review audit commit together. Fixed field-presence bits distinguish original
input from server-filled fields; author projections never expose the latter as history.
Private contribution shells use React and browser session/CSRF, independently of
anonymous Resource SSR. No additional HTML sink, queue, cache or remote URL fetch.

Migration 8 extends that lifecycle with Source addition/removal, additive Tag sets,
dual-endpoint Relations and raw Resource translations. Typed tables preserve each
base/proposed/accepted change; originals use fixed presence bits, never fallback
values. `ApplyReviewedOwnedTx` shares the existing graph lock and validates both
submitted endpoint versions before writing canonical state and per-resource audit.
Private multi-query projections use REPEATABLE READ followed by fresh READ COMMITTED
authorization; the transactions are sequential, without holding a second pool connection.
All mutations remain READ COMMITTED with User/session/capability revalidation.

P0-6 adds `governance` policy/audit primitives and the `moderation` application.
Reports and four-operation canonical resolutions commit atomically; private reads
use consistent projection plus fresh authorization. User restrictions/trust updates
lock both Users in UUID order. Public profile mutation now uses an authorized,
User-locked transaction. Reporting, security and privacy opt-out stay available.
Manual Source checks never fetch URLs; recommendation eligibility is a separate,
tested policy, with no ranking or Search domain. Migration 9 adds eight tables,
preserving all previous migrations/ACLs. `database/governancecheck` is developer-only.

Public web uses anonymous Astro Node SSR with isolated React interaction; Admin is
a React SPA. Apps use API-client facades, never deep generated imports or direct DB
access. Tailwind v4 owns layout; shared SCSS and SCSS Modules own visual appearance.
