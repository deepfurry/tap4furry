# Development contract

Use `dev` for integration; `main` is a stable release snapshot. Do not merge, push,
tag or release without explicit authorization. Preserve unrelated local changes.

Runtime processes read process environment only. The ignored developer launch inputs
`server/env/{api,admin,worker,migrator}.local` and `.local/readonly.env` contain real
credentials. Never print, commit, overwrite with placeholders or remove them or their
ignore rules. Public `.example` templates must contain only localhost placeholders.

Tap4Furry branding does not rename `gfp_dev`, `gfp_ci`, PostgreSQL `gfp_*` roles
or Redis `gfp:` keys. These are stable infrastructure identifiers. BRAND-0 runs
existing smokes only on shared development; it applies no shared migrations.

Shared development PostgreSQL/Redis are accessed using private local configuration.
Only repository-owned Goose/River migrations on `gfp_dev` using the prepared migrator
are allowed. No shared Infra SSH, container administration, cluster-role/Redis ACL
changes or Tailscale changes. Stop and report missing capabilities with sanitized
evidence. Do not expose real DSNs, passwords, tokens or Tailnet addresses.

CI never loads `.local` files or connects to shared Infra. It provisions disposable
PostgreSQL 18 and Redis 8 with disposable credentials and performs fresh migrations,
driver checks and container builds. CI does not deploy or publish images in P0-0.

`pnpm check` is the cross-platform verification entry point; explicit integration
and image commands are documented in `docs/development.md`. Record actual results.

P0-1D Admin dev origin is `http://localhost:5173`, proxying to port 8081. Use
`pnpm adminctl:dev grant-role|revoke-role|list-roles -email ... [-role ...]` with
prepared migrator input. No HTTP role-management surface exists. Grant only to an
active verified local password account. Do not remove the last active administrator.
`pnpm smoke:admin:dev` checks real runtime/owner identities and cleans only its own
temporary account and Resource graph. Its fixture roles progress Moderator→Editor→Admin
to validate read-only, ordinary curation and governance. Runtime never gets role
mutation rights; product role changes do not change PostgreSQL grants.

P0-3A extends the same Admin smoke with two temporary verified author accounts,
real Public submission, revised review, draft publication, correction CAS and private
history. New-table grants are checked column by column; fixture cleanup is owner-only
and limited to generated IDs. Use `pnpm migrate:dev` for migration 7 up only after
disposable 7→6→5→7 acceptance. Never run Down against shared development.

P0-3B extends this smoke to seven temporary verified authors: the two old flows plus
five new typed changes. Independent authors respect the real 60-second quota rather
than bypassing it. Migration 8 applies only after disposable downgrade protection,
old-proposal preservation and 8→7→8 acceptance. Shared checks assert Goose=8,
unchanged Resource Core grants and all ten Contribution table/column grant matrices;
cleanup remains restricted to owned fixture IDs. No new smoke command or Infra changes.

P0-6 extends the same Admin smoke with private report handling, live role boundaries,
restricted business writes with protected channels, fixed trust quotas, manual source
observations, recommendation exclusion, no-store Public visibility and business audit.
It reuses the temporary accounts, cleans only owned records and asserts Goose=9 and
the exact new column grants. Existing Resource/identity grants and private inputs
remain unchanged. Disposable tests also exercise authorization/restriction races,
audit failure rollback, report terminal competition and migration 9 downgrade refusal.

`ADMIN_CSRF_SECRET` is separate from Public `CSRF_SECRET`; API/Admin share
`AUTH_THROTTLE_SECRET`. Production requires explicit private values of at least
32 bytes; development/test have public defaults. Existing private files stay intact.
Production Admin requires operator-provided Cloudflare Access with MFA; do not
provision it during implementation. Human OAuth/Admin sign-off can remain explicitly
pending after all automated implementation gates pass.
