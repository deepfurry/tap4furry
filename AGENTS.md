# Tap4Furry

A multi-process modular monolith for furry resource discovery and exchange. P0-0
established infrastructure; P0-1A/B/C/D add identity, local authentication, session
security, account recovery, explicit Google/GitHub linking and isolated Admin auth.
MAIL-0 adds Resend; P0-2A adds Resource/Taxonomy schema and domain primitives.
P0-2B adds anonymous Resource reads and Astro SSR; P0-2C adds Admin curation.

## Start here

1. Inspect branch, status, history and actual implementation before editing.
2. Read the active `docs/implementation/` specification first. Follow its Required
   Reading in order; load only related detailed docs. Never preload `docs/`.
3. Read `.agents/architecture.md`, `.agents/playbook.md` and affected `contracts/`.
4. Read related ADRs only for consequential architectural changes.

`dev` is the development integration branch; `main` is a stable release snapshot.
Work on `dev` for P0 phases. Never merge, push, tag or release without explicit user
instruction. Preserve unrelated changes and all ignored local credentials.

Canonical identity is Tap4Furry, `deepfurry/tap4furry`, Go module
`github.com/deepfurry/tap4furry/server` and pnpm scope `@tap4furry/*`.
`gfp_*` and `gfp:` are intentionally retained stable infrastructure identifiers,
not product-brand surfaces. Historical verification commands and external dependency
names retain their original identity.

## Map and authority

- `apps/web`, `apps/admin`: Astro public web and React admin SPA.
- `packages/api-client`, `packages/design`: generated clients and shared styling.
- `server/cmd`: process composition and developer commands; one `server/go.mod`.
- `server/internal`: transport, config, database, Redis, Jobs and runtime boundaries.
- `server/internal/curation`: canonical Resource/Taxonomy mutation transactions.
- `contracts/openapi`: authoritative Public/Admin APIs.
- `server/db/migrations`, `server/db/queries`: application schema and SQL sources.
- `deploy`: build artifacts; `.github/workflows`: disposable-infrastructure CI.

The active task/specification and repository contracts define intended changes.
Machine-readable OpenAPI/migrations/queries own their generated outputs. Source,
tests and configuration establish actual behavior; design docs explain intent.
Report material conflicts instead of silently overriding either. External templates
are pattern references only and never override this repository.

## Rules and verification

- Never hand-edit generated Go, TypeScript or sqlc output. Run `pnpm generate`.
- Goose owns application migrations; official River migrations own River objects.
  Startup never migrates. Never rewrite released/applied migration history.
- PostgreSQL is canonical; Redis is ephemeral with `gfp:` keys. River imports stay
  under `server/internal/jobs`. Do not create speculative domain packages.
- `taxonomy` and `resource` remain pure domain packages. P0-2B reads use dedicated
  sqlc queries and the Public pool, independently of Auth/Redis. Version/CAS spans
  owned knowledge changes;
  relation edits affect both endpoints. Default localization is transaction-owned.
- Migrations 1–6 are immutable; P0-2B stays at version 6. Migration 6 down is tested against
  guarded disposable `gfp_ci`; shared `gfp_dev` remains up-only. Worker has no
  Resource grants; Public API is SELECT-only, Admin has explicit column-level DML.
- Public reads filter visibility/Source rights/Relation endpoints in SQL and fail
  closed on missing canonical translations. Internal governance fields stay private.
- P0-2C also stays at version 6 with unchanged grants. Curation locks actor User,
  revalidates Admin session/current capabilities in the same READ COMMITTED transaction,
  then locks canonical parents and checks Resource CAS. No-op preserves revision/time.
  All relation writes take fixed graph advisory lock then UUID-ordered endpoints,
  check directed cycles per type and bump both endpoints. Use DB transaction timestamps.
- Moderator inspects; Editor curates; Admin governs rights, restricted/removed states,
  retirement and soft deletion. Source removal changes availability; no parent hard-delete
  or restore. Admin React has real editor routes, explicit conflict reload and dirty guards;
  successful mutations refetch, never retry or optimistically reconstruct canonical state.
- Resource pages are Astro SSR without islands. Only server helpers read
  `API_INTERNAL_ORIGIN`; no credentials are forwarded. Markdown.astro is the sole
  audited sanitized HTML sink. Errors are no-store/noindex; success uses short shared cache.
- Auth owns transactions and the challenge-mail interface. Raw challenge tokens
  never enter jobs; mail delivers once after commit to Resend or private local capture.
  Production requires explicit Resend config; provider payloads/errors stay private.
- Auth owns OAuth provider/flow interfaces. Provider tokens stay inside the adapter;
  Redis holds only ten-minute one-use flows. Never auto-link accounts by email.
- Admin uses separate password sessions and live static-role checks. Only owner
  `adminctl` mutates roles. Password reset/change revoke all session kinds. Auth
  owns the throttle boundary; Redis keeps HMAC fingerprints, counters and TTLs only.
- Never print, commit, replace or remove `server/env/*.local` or `.local/` secrets.
  Never include real DSNs, credentials or Tailnet addresses in tracked output.
- Do not SSH/administer shared Infra or change cluster roles/Redis ACLs. Prepared
  migrator credentials may apply repository migrations to `gfp_dev`; stop for
  missing capabilities. CI uses disposable PostgreSQL/Redis only.
- Final verification: `pnpm check`, then `pnpm generate` and generated drift check.
  Execute applicable migration, smoke and container acceptance gates too.
- Review the entire final diff and staged diff, audit secrets/dependencies, update
  `CHANGELOG.md`, and report actual commands/results, blockers and commit SHA.
  Commit only verified work; do not claim unexecuted checks passed.
