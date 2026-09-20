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

`ADMIN_CSRF_SECRET` is separate from Public `CSRF_SECRET`; API/Admin share
`AUTH_THROTTLE_SECRET`. Production requires explicit private values of at least
32 bytes; development/test have public defaults. Existing private files stay intact.
Production Admin requires operator-provided Cloudflare Access with MFA; do not
provision it during implementation. Human OAuth/Admin sign-off can remain explicitly
pending after all automated implementation gates pass.
