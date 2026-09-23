# Architecture contract

## P0-3A basic contribution review

- `internal/contribution` owns typed new-Resource/default-language correction proposals.
  Author transactions lock User, revalidate Public session and verified email for
  submission, then serialize request replay and rolling quotas. Reads/withdrawals
  remain available to valid unverified accounts; ownership is in SQL predicates.
- Editorial review locks reviewer User then proposal, rejects self/terminal decisions,
  and joins `curation.ApplyReviewedTx` in the same READ COMMITTED transaction.
  Existing Resources require the exact submitted version and default language and
  must still be public. New Resources start as drafts at version 1. No-op is rejected.
- Original/base/accepted snapshots, decision events and business audits are typed,
  immutable history. No JSONB/EAV, new Redis keys, business jobs, automatic merging,
  notifications or later contribution types are introduced.
- Author DTOs project only explicitly supplied original fields, safe messages and
  current public links. Accepted content is visible only at its exact current public
  revision; subsequent changes suppress historical content. Baselines, internal notes,
  actor IDs and governance fields stay in Editorial DTOs.
- Public submission and private history use session-bound CSRF and no-store.
  Resource pages remain anonymous Astro SSR. Private React forms use generated
  clients, no HTML sink/persisted drafts, and explicit submission/preview/decision.
  Reviewer edits require a reason; conflicts preserve input and never retry automatically.

## P0-3B complete contribution types

- The same lifecycle supports add_source, remove_broken_source, add_tag,
  add_relation and add_translation. Closed typed payloads reject unknown/mixed
  variants; old create/update contracts remain compatible. No generic command bus.
- `curation.ApplyReviewedOwnedTx` joins the review transaction without committing.
  Sources start rights=unknown/non-primary; removal only changes availability.
  Tags add 1–10 active bindings without replacing unrelated/retired bindings.
- Relation acceptance takes the same graph advisory lock before UUID-ordered
  endpoints, compares both submitted versions, checks cycles per type and writes
  both version audits. Either endpoint stale or any audit failure rolls back everything.
- Translation context uses the raw non-default locale, separately from the default
  reference. Fixed presence bits preserve omitted versus explicitly cleared fields.
  Authors never receive server-filled original fields or hidden accepted snapshots.
- Multi-query private projections share one REPEATABLE READ snapshot. A subsequent
  READ COMMITTED authorization check prevents an older snapshot from surviving a
  role/session revocation that held User first. No concurrent nested connection.
  Mutations continue to revalidate live authorization in their own READ COMMITTED transaction.
- A single private form/review surface covers all seven kinds. Explicit previews,
  revision explanations, dirty guards and conflict preservation remain mandatory;
  anonymous Resource SSR and its sole sanitized HTML sink are unchanged.

## Core boundaries

- One Go module, three runtime processes: Public API, Admin API, Worker.
- Public/Admin remain separate transport/security boundaries; no inter-process HTTP
  calls to share business behavior. Composition belongs in `cmd/*`.
- Application owns transactions; Domain stays ordinary Go without transport
  DTOs, Fiber, Redis or River. No ceremonial service/repository wrappers around sqlc.
- River imports and types stay under `server/internal/jobs`.
- PostgreSQL is canonical; Redis is ephemeral and all created keys use `gfp:`.
- OpenAPI is spec-first. Public adds local authentication, `/me` and profiles in
  P0-1A and session/verification/recovery APIs in P0-1B. P0-1C adds explicit OAuth;
  P0-1D adds independent password-only Admin authentication and static roles.
  Both expose `/health/live` and `/health/ready`.
  Liveness never probes dependencies. Readiness requires PostgreSQL; Redis failure
  reports degraded state while PostgreSQL remains ready.
- Go 1.27+, Node 24, pnpm workspace; Astro/React 19 public SSR, React 19/Vite admin.
  Tailwind v4 handles layout; SCSS handles appearance.
- `auth` and `identity` serve P0-1; P0-2A opens only `taxonomy` and `resource` as pure
  domain packages with no transport, database, Redis or River dependency. No future scaffolds, unused
  dependencies, ORM/AutoMigrate, MongoDB, NATS, vectors or observability stack.
- Business IDs use Go standard-library `uuid.NewV7`. easyhash must explicitly use
  Argon2id and an Argon2id upgrade policy; password hashes never enter transport.
- Public cookies carry opaque random tokens; only SHA-256 lookup hashes persist.
  Exact Origin protects unsafe auth/profile requests; authenticated unsafe requests
  also require a session-bound HMAC CSRF value. Never expose raw session tokens to JS.
- Auth owns the challenge-mail consumer interface and database transactions. Mail
  delivery happens after commit, with no raw challenge token in Redis/River/logs.
  MAIL-0 adds Resend via its official SDK inside `internal/mail`, with a five-second
  bound and flattened errors. Production requires explicit `MAIL_MODE=resend`;
  private local capture remains the development default. No mail queue/outbox/retry.
- Auth owns static role/capability policy and the narrow throttle interface;
  `redisstore` implements HMAC subject/global counters with bounded TTLs.
  Public local auth fails open on Redis outage; Admin login/reauth fail closed.
- Admin uses only `kind=admin` password sessions and re-reads roles on each request.
  `adminctl` alone changes roles via the owner connection; runtime cannot write roles.
  Normal revocations stay within one session kind. Password reset/change are the
  explicit exception: all kinds revoked atomically, one Public replacement created.
- P0-2A implements Resource Domain/Schema only: no Resource HTTP contract, frontend,
  full CRUD, Contribution, Search, Redis data or River business jobs. Taxonomy is
  flat and unseeded; slugs are ASCII and no automatic transliteration is performed.
- Parent/default localization creation is atomic. Default changes require an existing
  translation; deleting the current default is rejected while holding the parent lock.
  Resource-owned knowledge edits bump `version` once per logical transaction and
  endpoint; stale CAS aborts all child writes. Category/Tag edits do not bump Resources.
- P0-2C curation uses Editorial for ordinary canonical edits; Administration is
  required for retiring taxonomy, restricted/removed Resources, soft deletion and
  rights-status mutations. Moderator has no canonical write capability. `gfp_admin`
  is a database identity, distinct from the product's `admin` role. No new roles.

## P0-2C canonical curation

- `internal/curation` owns canonical mutation transactions, using existing Auth's
  User-first lock/session/live-role checks. Transport actor caching never substitutes
  for transaction-time capability revalidation. Reads use `admin_resource.sql`.
- Mutations use READ COMMITTED, PostgreSQL transaction timestamps and Resource CAS.
  True no-op does not bump version/time. Stale/conflicting child writes fully roll back.
- Relations use a fixed graph advisory xact lock before UUID-ordered Resource locks;
  directed cycles are checked per relation_type, and changes bump both endpoints once.
- Editor handles ordinary edits and draft/pending/published transitions; Admin is
  required on either side of restricted/removed transitions, rights, retirement and
  soft deletion. No Source hard-delete, restore, bulk actions or fuzzy search.
- Admin routes require AdminAccess, no-store and five-second deadlines. Unsafe
  requests require exact ADMIN_ORIGIN plus session-bound CSRF. Fiber ceiling is
  256 KiB; Auth's strict 8 KiB decoder is unchanged. Description limit is 50,000
  Unicode characters in Application/OpenAPI without a database schema change.
- React uses real editor subroutes, no automatic mutation retries/optimistic writes,
  explicit conflict reload preserving local form, dirty guards and typed soft deletion.

## P0-2B anonymous reads

- Only GET `/resources`, `/resources/{slug}`, `/categories`, `/tags` are added.
  Public handlers use dedicated sqlc reads, with a five-second request deadline and
  no session/Origin/CSRF/OAuth/Redis dependency. Auth behavior stays unchanged.
- Resource/taxonomy remain pure domains. Detail reads one read-only PostgreSQL
  snapshot for parent and children. No repository/service wrapper, mutation or cache.
- Astro `/resources` and `/resources/[slug]` use server-only `API_INTERNAL_ORIGIN` and
  generated URL builders (strip exactly `/api`). Runtime production configuration is
  required; only development defaults to loopback. No Cookie/Auth/CSRF forwarding,
  external-domain round trip, React island, retry or process-local cache.
- Markdown-it disables raw HTML/linkify/typographer; sanitize-html allowlists content,
  protocols and attributes. No images/MDX. Only Markdown.astro has `set:html` and server
  helpers cannot be imported by browser modules/scripts.
- API/page success uses `public, max-age=0, s-maxage=60, stale-while-revalidate=30`;
  errors use no-store, SSR errors also noindex. SEO uses the fixed production site,
  excludes locale from canonical, and includes page only above 1. No Accept-Language Vary.
