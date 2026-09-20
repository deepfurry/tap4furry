# P0-2C verification — 2026-09-20

Baseline: `5874ffa4c374250909062f538ec6201fa7cef2a0` on clean `dev`.
GitHub Actions Run #9 was confirmed completed/success before implementation.
The attached P0-2C specification was copied verbatim; its Markdown hard-break
whitespace is intentional. No substantive repository/specification conflict was found.

## Implemented boundaries

- `curation.App` owns READ COMMITTED canonical mutation transactions. Auth locks the
  actor User and rechecks the active Admin session/current static capabilities.
  Authorization expiry uses current database clock after locking; canonical graph
  timestamps consistently use transaction_timestamp().
- Resource-owned mutations require expected_version under parent locks. Stale CAS
  and child identity conflicts roll back completely; true no-op leaves version and
  updated_at unchanged. Resource description accepts at most 50,000 Unicode code points.
- Relation mutations take fixed advisory xact lock `72463159018202`, then Resource
  locks in UUID order. Directed cycles are checked independently per relation_type;
  related_to is canonicalized. Every changed relation bumps both endpoints once.
- Moderator reads; Editor performs ordinary curation; Admin governs Source rights,
  transitions to/from restricted/removed, taxonomy retirement and soft deletion.
  Source rows are retained with removed availability. Parent restore is absent.
- Dedicated Admin sqlc reads and OpenAPI cover Resources, localizations, tag sets,
  Sources/rights, Relations, external IDs, publication, Categories and Tags. Exact
  slug lookup is equality only. All curation requests require AdminAccess, no-store
  and five-second deadlines; unsafe requests additionally require Origin and CSRF.
- Admin Fiber ceiling is 256 KiB. Auth's existing 8 KiB decoder remains unchanged.
  Resource/Taxonomy domains have no new infrastructure or transport dependencies.
- React Admin now has Resources/List/Create, six real editor subroutes, Taxonomy
  lists/details and Account. Mutations never automatically retry or optimistically
  rebuild canonical state. Success refetches; conflict preserves forms until explicit
  reload. Dirty route/beforeunload guards and typed soft-delete confirmation are present.

## Executed acceptance

| Command / check | Final result |
| --- | --- |
| `gh run list --branch dev --limit 3 --json databaseId,number,status,conclusion,headSha` | PASS — baseline Run #9 success |
| Baseline `pnpm check` | PASS |
| `pnpm generate` | PASS — repeated after implementation and final check |
| `pnpm check` | PASS — audit, generation, lint/vet, TypeScript, Node/Go tests, app/server builds |
| `pnpm check:generated` | PASS — byte-identical outputs; no added/removed generated files |
| `go -C server test -count=1 -timeout=3m -run TestIntegrationCuration -v ./internal/transport/public` with disposable guards | PASS |
| `pnpm integration:ci` with `CI=true GFP_DISPOSABLE_INFRA=1` | PASS — all previous and new integration gates |
| `pnpm build:images` | PASS — Server, Web, Admin, PostgreSQL images; no publish |
| `pnpm smoke:admin:dev` | PASS — repeated with real prepared development identities |
| `pnpm --filter @tap4furry/admin typecheck` | PASS |
| `pnpm exec eslint apps/admin scripts` | PASS |
| `node --test scripts/tests/admin-curation.test.mjs` | PASS — capability, sets, source boundaries, generated client conflict behavior |
| `pnpm audit:repository` | PASS — working/staged content, secrets and dependency boundaries |
| `node .cache/p02c-preservation-check.mjs` | PASS — six migrations, private-input hashes, frozen boundaries and branch protection |
| `node .cache/p02c-grants.mjs --capture`, then `node .cache/p02c-grants.mjs` | PASS — exact effective privileges and identical before/after catalog fingerprints |
| `node .cache/p02a-db-post-audit.mjs` | PASS — prepared identities, Goose 6, ten tables, no vector, zero shared Resource rows after cleanup |
| `git diff --cached --check` excluding the verbatim specification's Markdown hard breaks | PASS — source whitespace review |
| Post-stage `pnpm generate` + `git diff --exit-code` / untracked-file check on generated directories | PASS — staged generated outputs remain identical |

Disposable integration includes P0-1 Auth/OAuth/Recovery/Admin regressions; P0-2A
schema, exact grants, CAS and guarded 00006 Down/Up; P0-2B Public privacy/localization
and Astro SSR/Markdown/cache/status/SEO; and the P0-2C matrix. Curation tests exercise
both role-revoke orderings, actual database lock-wait observation, stale CAS, no-op,
child rollback, canonical/default localization, Unicode description limits, retired
bindings, Source primary/retention, publication timestamps/slug freeze, in-use taxonomy,
exact slug/filter/pagination and concurrent opposite-edge graph mutation.

Admin→Public HTTP lifecycle passed on disposable and shared development:

| Canonical state | Public GET |
| --- | --- |
| Draft | 404 |
| Published | 200 |
| Restricted / Removed | 404 |
| Republished | 200 |
| Soft-deleted | 404 |

Browser product acceptance used only disposable `gfp_ci` accounts/data. It verified
Editor Create Draft, six editor routes, publish/frozen slug, Source normalization and
Editor rights read-only, exact Relation lookup/add, external-ID set edit, Tag binding,
locale canonicalization, Moderator read-only controls, Admin restrict/republish,
retired Tag retention, Account/session placement and typed soft-delete. Two browser
views verified stale-version conflict keeps local text and disables save until explicit
reload. The visible dirty-route dialog kept input when canceled. beforeunload wiring
is verified by the source audit and TanStack blocker configuration; a native browser
close prompt was not independently exercised. No browser testing framework was added.

## Preservation and cleanup

- Live shared development Goose version remains **6**; all ten Resource Core tables
  remain present. No 00007 exists. Migrations 00001–00006 match pre-work SHA-256 hashes.
- Live effective table and column privileges match the P0-2A matrix: API/readonly
  SELECT-only, Admin precise DML, Worker no Resource access. Before/after catalog
  fingerprints are identical. No runtime grants, cluster roles or Redis ACLs changed.
- Shared Admin smoke cleaned only its generated account/roles and graph; the live
  Resource Core row count returned to its initial **0**. No shared migration/down ran.
- Hashes of pre-existing ignored private launch inputs remain identical. Secret and
  Tailnet scans passed. No private input, credential, token or session was staged.
- Go/pnpm dependency manifests and lockfiles, pure Resource/Taxonomy domain packages,
  Public OpenAPI and Public Astro implementation are unchanged from the baseline.
- No Contribution, Search, new Resource Redis keys, River jobs or Worker grants were
  introduced. No push, main merge, tag, release or production deployment occurred.

## Resolved verification failures

Early Orval validation rejected empty `required` arrays on PATCH schemas; the source
contract was corrected and regenerated. Integration exposed an unsupported Source
DELETE request being flattened to 500; the boundary now returns a safe 404 and the
test passes. New source-audit line limits were adjusted for formatted route declarations.
Direct/`pnpm exec` invocation of the generation-check script lacked pnpm's script
environment; the repository's intended `pnpm check:generated` command passed.

The local verification WSL also hosted a separate Docker daemon with `--bridge=none`,
which removed the default bridge and caused DB timeouts and one image-build failure.
The task-owned temporary daemon used a dedicated bridge for the successful image run.
This affected local disposable tooling only; no repository CI/network architecture or
shared infrastructure was changed. Final required gates passed after these corrections.

P0-2C and P0-2 implementation acceptance are complete. P0-3 Contribution & Review may
start as a separate scope. Production deployment remains a separate decision.
