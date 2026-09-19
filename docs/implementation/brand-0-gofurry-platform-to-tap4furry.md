# BRAND-0 — GoFurry Platform to Tap4Furry Migration

**Repository:** `deepfurry/tap4furry`\
**Target branch:** `dev`\
**Release snapshot branch:** `main`\
**Baseline commit:** `6fa46f693b8232b0bb4fc497328c33f98c44e729`\
**Baseline phase:** P0-1D complete\
**Status:** Codex implementation specification

---

## 1. Goal

The GitHub repository has already been renamed:

```text
deepfurry/gofurry-platform
→
deepfurry/tap4furry
```

The canonical product brand is now:

```text
Tap4Furry
```

with primary product domain:

```text
tap4furry.com
```

BRAND-0 completes the repository-side rename before P0-2 begins.

This phase migrates all **product-facing and code-namespace identity** from the old GoFurry Platform / GoFurry International brand to Tap4Furry, while intentionally preserving existing development-infrastructure identifiers that are not user-facing brand surfaces.

BRAND-0 must not redesign architecture, authentication, database schema, or development infrastructure.

---

## 2. Verified Baseline

Current `dev` HEAD:

```text
6fa46f6 feat: add admin authentication and auth hardening
```

P0-1D CI Run #5 is green.

Current completed chain:

```text
P0-0  Engineering Foundation                    complete
P0-1A Identity + Password + Public Session      complete
P0-1B Verification / Recovery / CSRF            complete
P0-1C Google/GitHub OAuth + Account Linking     complete
P0-1D Admin Auth + Roles + Auth Hardening       complete
```

Therefore:

```text
P0-1 Identity/Auth implementation = complete
```

Human Google/GitHub browser acceptance remains a later P0-1 sign-off item and is not a BRAND-0 blocker.

---

## 3. Branch / Execution Contract

Work only on:

```text
dev
```

Before editing:

```bash
git branch --show-current
git status --short
git log -5 --oneline --decorate
git remote -v
```

Verify canonical remote points to:

```text
https://github.com/deepfurry/tap4furry.git
```

or the equivalent SSH URL.

Rules:

- do not rename the GitHub repository again;
- do not modify or merge `main`;
- do not tag or release;
- do not push unless explicitly instructed;
- preserve unrelated user changes;
- do not rewrite Git history;
- do not rewrite existing commits;
- after verification, commit locally on `dev`.

Recommended commit:

```text
chore: migrate platform branding to Tap4Furry
```

---

## 4. Product Identity After Migration

Canonical identity:

```text
Product:
Tap4Furry

Repository:
deepfurry/tap4furry

Primary domain:
tap4furry.com

Admin domain:
admin.tap4furry.com

Go module:
github.com/deepfurry/tap4furry/server

pnpm scope:
@tap4furry/*
```

Use:

```text
Tap4Furry
```

for normal product prose.

Do not use these as the canonical current name:

```text
GoFurry Platform
GoFurry International
Tap4furry
Tap 4 Furry
```

The existing generic description remains valid:

> A web-first resource discovery, contribution, and exchange platform for the furry ecosystem.

---

## 5. Core Migration Principle

Classify every old-name occurrence before changing it.

### Must migrate

Anything representing:

```text
product brand
repository identity
module/package namespace
user-facing copy
future production domain
cookie name
runtime artifact name
generated API branding
documentation self-reference
```

must become Tap4Furry.

### Must remain

Anything representing stable existing development infrastructure or a separate external dependency/project must remain unchanged.

Do **not** perform a blind global replacement.

---

## 6. Explicitly Preserved Infrastructure Identifiers

The following must remain:

```text
gfp_dev

gfp_api
gfp_admin
gfp_worker
gfp_migrator
gfp_readonly

gfp_runtime

Redis prefix:
gfp:
```

These are internal infrastructure identifiers, not product branding.

They are already embedded in:

```text
PostgreSQL roles
database grants
Redis ACL
private local env files
shared development Infra
CI disposable role conventions
applied migrations
```

Do not rename them.

Future migrations may continue using these role names.

Document explicitly:

> `gfp_*` and `gfp:` are intentionally retained stable infrastructure identifiers and are not product-brand surfaces.

---

## 7. Applied Migrations Are Immutable

Do not edit:

```text
server/db/migrations/00001_foundation.sql
server/db/migrations/00002_identity_local_auth.sql
server/db/migrations/00003_auth_security_recovery.sql
server/db/migrations/00004_oauth_identity.sql
server/db/migrations/00005_admin_auth_roles.sql
```

BRAND-0 requires:

```text
NO Goose migration
```

Do not create a brand-rename migration.

---

## 8. External Dependency Names Must Not Be Rebranded

Preserve:

```text
github.com/gofurry/easyhash
```

It is a separate external Go module and not this repository's product namespace.

Do not invent:

```text
github.com/tap4furry/easyhash
github.com/deepfurry/easyhash
```

Likewise preserve any other proper noun that genuinely refers to a separate GoFurry project rather than this product.

Every remaining `GoFurry`/`gofurry` occurrence after migration must be explicitly reviewed and justified.

---

## 9. Repository-wide Brand Inventory

Before editing, run a full local inventory:

```bash
rg -n -i "gofurry|go-furry|gofurry-platform|gofurry\.com|@gofurry|__Host-gofurry"
```

Also inspect:

```bash
rg -n "github\.com/deepfurry/gofurry-platform"
rg -n "@gofurry/"
rg -n "__Host-gofurry"
rg -n "gofurry_(session|admin_session)"
rg -n "gofurry-"
```

Classify every result:

```text
MIGRATE
PRESERVE
EXTERNAL/HISTORICAL — justify explicitly
```

Do not rely on blind replacement.

---

## 10. Go Module Migration

Change:

```go
module github.com/deepfurry/gofurry-platform/server
```

to:

```go
module github.com/deepfurry/tap4furry/server
```

Update all internal imports:

```text
github.com/deepfurry/gofurry-platform/server/...
→
github.com/deepfurry/tap4furry/server/...
```

This includes:

```text
cmd/*
internal/*
tests
smoke commands
operator commands
generated source when generator-owned config requires it
```

Do not change:

```text
github.com/gofurry/easyhash
```

Run `go mod tidy` only if required by the import migration.

Do not introduce unrelated dependency upgrades.

---

## 11. Generated Go Code

If generated Go source contains the old module path or old brand metadata:

- update generator source/config;
- regenerate;
- never hand-edit generated output.

Preserve ownership:

```text
OpenAPI → oapi-codegen
SQL → sqlc
```

`pnpm generate` remains the root generation command.

---

## 12. pnpm Workspace Identity

Change root package:

```json
"name": "gofurry-platform"
```

to:

```json
"name": "tap4furry"
```

Change self-owned workspace packages:

```text
@gofurry/web
@gofurry/admin
@gofurry/api-client
@gofurry/design
```

to:

```text
@tap4furry/web
@tap4furry/admin
@tap4furry/api-client
@tap4furry/design
```

Migrate any additional `@gofurry/*` workspace package if present.

Update all imports:

```text
@gofurry/...
→
@tap4furry/...
```

Update `pnpm-lock.yaml` normally.

Do not change unrelated dependency versions.

---

## 13. Package Export Contracts

Preserve package responsibilities and exports:

```text
@tap4furry/api-client/public
@tap4furry/api-client/admin
@tap4furry/design/*
```

Do not redesign package layout in BRAND-0.

---

## 14. Runtime Binary Names

Migrate built project artifact names:

```text
gofurry-api
gofurry-admin
gofurry-worker
```

to:

```text
tap4furry-api
tap4furry-admin
tap4furry-worker
```

If the operator binary is product-prefixed, use:

```text
tap4furry-adminctl
```

Source directories may remain:

```text
server/cmd/api
server/cmd/admin
server/cmd/worker
server/cmd/adminctl
```

Update Docker/build scripts/docs consistently.

---

## 15. Docker Image Names

Migrate:

```text
gofurry-server
gofurry-web
gofurry-admin
gofurry-postgres
```

to:

```text
tap4furry-server
tap4furry-web
tap4furry-admin
tap4furry-postgres
```

Prefer current local tags:

```text
tap4furry-server:local
tap4furry-web:local
tap4furry-admin:local
tap4furry-postgres:local
```

instead of stale phase-specific tags such as `*:p0-0-local`.

Do not redesign image contents or deployment topology.

---

## 16. UI Branding

Replace user-visible:

```text
GoFurry
GoFurry Platform
GoFurry International
GoFurry Admin
```

with appropriate:

```text
Tap4Furry
Tap4Furry Admin
```

Apply to:

```text
Public Web
Admin Web
page titles
headings
metadata
development mail content
brand-specific helper/error text
```

Do not rewrite unrelated prose.

---

## 17. Public Production Domain

Future Public origin:

```text
https://tap4furry.com
```

Replace current repository contracts/docs using:

```text
gofurry.com
```

with:

```text
tap4furry.com
```

Canonical same-origin API:

```text
https://tap4furry.com/api/*
```

Do not change development:

```text
http://localhost:4321
```

---

## 18. Admin Production Domain

Future Admin origin:

```text
https://admin.tap4furry.com
```

Replace:

```text
admin.gofurry.com
```

with:

```text
admin.tap4furry.com
```

Do not change development:

```text
http://localhost:5173
```

Admin API local runtime remains on its existing loopback port.

---

## 19. Public Cookie Rename

Production:

```text
__Host-gofurry_session
```

becomes:

```text
__Host-tap4furry_session
```

Development/test:

```text
gofurry_session
```

becomes:

```text
tap4furry_session
```

Update:

```text
constants
transport
OpenAPI security schemes
tests
docs
smoke tooling
```

Preserve:

```text
Secure in production
HttpOnly
SameSite=Lax
Path=/
No Domain
```

---

## 20. Admin Cookie Rename

Production:

```text
__Host-gofurry_admin_session
```

becomes:

```text
__Host-tap4furry_admin_session
```

Development/test:

```text
gofurry_admin_session
```

becomes:

```text
tap4furry_admin_session
```

Preserve:

```text
Secure in production
HttpOnly
SameSite=Strict
Path=/
No Domain
```

Public/Admin separation must remain unchanged.

---

## 21. OAuth Browser-binding Cookies

Audit P0-1C browser-binding/OAuth temporary cookie names.

If any product-scoped cookie contains `gofurry`, rename it to `tap4furry`.

Preserve all existing:

```text
HttpOnly
SameSite
Secure
TTL
state digest binding
```

Do not change:

```text
state generation
PKCE
nonce
Redis OAuth flow
callback paths
```

---

## 22. Development Secret Default Strings

Development-only default values that embed the old brand may be updated.

For example:

```text
gofurry-development-only-csrf-secret
gofurry-development-only-admin-csrf-secret
gofurry-development-only-auth-throttle-secret
```

may become:

```text
tap4furry-development-only-csrf-secret
tap4furry-development-only-admin-csrf-secret
tap4furry-development-only-auth-throttle-secret
```

Preserve env variable names:

```text
CSRF_SECRET
ADMIN_CSRF_SECRET
AUTH_THROTTLE_SECRET
```

No new secret is required.

---

## 23. OAuth Callback Contract

Development callbacks remain unchanged:

```text
http://localhost:4321/api/auth/oauth/google/callback
http://localhost:4321/api/auth/oauth/github/callback
```

Future production callbacks become:

```text
https://tap4furry.com/api/auth/oauth/google/callback
https://tap4furry.com/api/auth/oauth/github/callback
```

Continue deriving callbacks from `PUBLIC_ORIGIN`.

Do not add redirect-URI env vars.

---

## 24. OAuth Dev Credentials

Existing Google/GitHub Dev credentials remain valid.

Do not:

```text
regenerate Client Secrets
delete credentials
change localhost callbacks
commit private credentials
```

External display names may later be manually changed:

```text
GoFurry Platform Dev
→
Tap4Furry Dev
```

Google Cloud Project ID may remain historical.

---

## 25. Documentation Migration

Migrate living project docs from:

```text
GoFurry Platform
GoFurry International
```

to:

```text
Tap4Furry
```

Audit/update:

```text
README.md
AGENTS.md
.agents/*
contracts/*
docs/product/*
docs/architecture/*
docs/engineering/*
docs/development.md
docs/implementation/*
deploy/*
CHANGELOG.md
```

Completed implementation specs are living Agent context, so update their project/repository/module/package/domain self-references where appropriate.

Do not alter their technical phase intent.

Do not change:

```text
migration numbers
commit SHAs
retained gfp_* names
historical technical facts
```

---

## 26. Changelog

Do not rewrite Git history.

Add a BRAND-0 `Unreleased` entry covering:

```text
product/repository rename to Tap4Furry
Go module migration
pnpm scope migration
cookie/runtime artifact rename
production domain contract rename
intentional gfp_* Infra retention
```

Historical changelog prose may be adjusted only where current brand naming would otherwise be confusing.

---

## 27. README

Root README should become:

```text
# Tap4Furry
```

Update current status:

```text
P0-1 Identity/Auth implementation complete
P0-2 Taxonomy & Resource Core next
```

Update repository self-links to:

```text
deepfurry/tap4furry
```

Do not present `GoFurry International` as the current product name.

---

## 28. Product Docs

Headings such as:

```text
# GoFurry International — ...
```

become:

```text
# Tap4Furry — ...
```

Product prose should use `Tap4Furry`.

Do not change product strategy/scope.

---

## 29. Architecture Docs

Update runtime names and production-domain examples.

Preserve architecture:

```text
Multi-process Modular Monolith
PostgreSQL canonical
Redis ephemeral
River
Public/Admin separation
Astro + React
React/Vite Admin
```

BRAND-0 is not an architecture redesign.

---

## 30. Security Docs

Update current product references:

```text
__Host-gofurry_session
__Host-gofurry_admin_session
gofurry.com
admin.gofurry.com
GoFurry Admin
GoFurry authorization
```

to Tap4Furry equivalents.

Preserve:

```text
gfp:auth:...
```

and document why the `gfp:` namespace remains.

---

## 31. OpenAPI Branding

Update Public/Admin OpenAPI:

```text
title
description
cookie names
brand-specific examples
```

Canonical titles:

```text
Tap4Furry Public API
Tap4Furry Admin API
```

Do not rename routes merely for branding.

Regenerate Go + TypeScript outputs.

---

## 32. Local Development Environment Must Stay Stable

Do not change shared development Infra.

Do not SSH.

Do not rename:

```text
gfp_dev
gfp_api
gfp_admin
gfp_worker
gfp_migrator
gfp_readonly
gfp_runtime
gfp:
```

Do not require recreation of:

```text
server/env/api.local
server/env/admin.local
server/env/worker.local
server/env/migrator.local
.local/readonly.env
```

Real OAuth credentials stay untouched.

---

## 33. CI Infrastructure Stays Stable

CI may continue using:

```text
gfp_ci
gfp_api
gfp_admin
gfp_worker
gfp_migrator
gfp_runtime
gfp:
```

Do not rename disposable DB roles.

Only migrate product artifact/package/module naming.

---

## 34. No Compatibility Alias Required

This is pre-release.

Do not create:

```text
@gofurry/* compatibility packages
old Go-module shims
old cookie fallback
old binary aliases
```

Perform a clean rename.

---

## 35. No Session/Data Migration Required

Session rows do not depend on cookie names.

Old local browser cookies becoming unused is acceptable.

No production users exist.

Do not purge the DB.

Developers may need to log in again locally.

No database brand-data migration is needed.

---

## 36. GitHub Repository Metadata — Manual Follow-up

Repository is already:

```text
deepfurry/tap4furry
```

The repository description is already brand-neutral.

The GitHub Homepage should be manually changed from:

```text
https://gofurry.com
```

to:

```text
https://tap4furry.com
```

Codex should report this as manual follow-up unless it has explicitly authorized repository-admin capability.

Do not block code completion on this.

---

## 37. Google/GitHub OAuth — Manual Follow-up

Optional external branding cleanup:

```text
Google OAuth display name:
GoFurry Platform Dev
→ Tap4Furry Dev

GitHub OAuth App display name:
GoFurry Platform Dev
→ Tap4Furry Dev
```

Do not change:

```text
Client ID
Client Secret
localhost callbacks
Google Cloud Project ID
```

---

## 38. Production Deployment Is Out of Scope

Do not:

```text
configure DNS
provision VPS
configure Cloudflare
configure Nginx
issue certificates
create production OAuth clients
deploy containers
create production DBs
```

Only update future production contracts in the repository.

---

## 39. Test Updates

Update affected tests for:

```text
new Go module
new pnpm scopes
new cookie names
new runtime/image names
new product/UI strings
new production-domain examples
```

Do not weaken P0-1 tests.

All existing auth/OAuth/Admin behavior must remain green.

---

## 40. Real Development Smoke

Run safe smokes where the local environment is available:

```text
pnpm smoke:dev
pnpm smoke:auth:dev
pnpm smoke:oauth:dev
pnpm smoke:admin:dev
```

Do not expose secrets.

Interactive OAuth may still require human consent; report honestly.

No Infra changes are permitted.

---

## 41. CI Acceptance

Existing CI must remain green:

```text
pnpm install --frozen-lockfile
pnpm check
pnpm generate
generated drift check
pnpm integration:ci
pnpm build:images
```

CI should now validate Tap4Furry product/module/package/cookie/artifact names while retaining `gfp_*` infrastructure.

---

## 42. Applied Migration Integrity

Before completion, verify no applied migration changed.

Recommended:

```bash
git diff 6fa46f693b8232b0bb4fc497328c33f98c44e729 -- server/db/migrations/00001_foundation.sql
git diff 6fa46f693b8232b0bb4fc497328c33f98c44e729 -- server/db/migrations/00002_identity_local_auth.sql
git diff 6fa46f693b8232b0bb4fc497328c33f98c44e729 -- server/db/migrations/00003_auth_security_recovery.sql
git diff 6fa46f693b8232b0bb4fc497328c33f98c44e729 -- server/db/migrations/00004_oauth_identity.sql
git diff 6fa46f693b8232b0bb4fc497328c33f98c44e729 -- server/db/migrations/00005_admin_auth_roles.sql
```

Expected:

```text
no diff
```

---

## 43. Residual Old-brand Scan

After migration, run:

```bash
rg -n -i "gofurry|go-furry|gofurry-platform|gofurry\.com|@gofurry|__Host-gofurry"
```

Every result must be classified.

Legitimate expected example:

```text
github.com/gofurry/easyhash
```

Not allowed as unexplained current self-brand:

```text
github.com/deepfurry/gofurry-platform
@gofurry/*
GoFurry Platform
GoFurry International
gofurry.com
admin.gofurry.com
__Host-gofurry*
gofurry_session
gofurry_admin_session
gofurry-* product artifacts
```

`gfp_*` / `gfp:` remain intentionally.

---

## 44. Implementation Order

1. Confirm `dev`, clean tree, new remote, green P0-1D baseline.
2. Run baseline `pnpm check`.
3. Inventory old-brand strings with `rg`.
4. Classify each occurrence.
5. Update Go module path and internal imports.
6. Update pnpm root/workspace package names and imports.
7. Refresh lockfile as needed.
8. Rename runtime binary/image artifact names.
9. Rename Public/Admin/OAuth-related product cookie names.
10. Update branded development-only secret defaults.
11. Update UI branding and mail copy.
12. Update future production domains.
13. Update Public/Admin OpenAPI metadata/cookies.
14. Regenerate generated code.
15. Update README/AGENTS/contracts/docs/deploy/implementation specs.
16. Add BRAND-0 CHANGELOG entry.
17. Run residual old-brand scan.
18. Run `pnpm generate`.
19. Run generation again and verify zero drift.
20. Run `pnpm check`.
21. Run `pnpm integration:ci`.
22. Run `pnpm build:images`.
23. Run safe real-dev smokes where available.
24. Verify migrations 00001–00005 unchanged.
25. Inspect full diff.
26. Secret audit.
27. Verify retained `gfp_*`/`gfp:` contract.
28. Commit locally on `dev`.
29. Do not push.

---

## 45. Acceptance Criteria

### Repository / module

```text
repository = deepfurry/tap4furry                       PASS
module = github.com/deepfurry/tap4furry/server         PASS
root package = tap4furry                               PASS
```

### pnpm scope

```text
@tap4furry/web                                         PASS
@tap4furry/admin                                       PASS
@tap4furry/api-client                                  PASS
@tap4furry/design                                      PASS
no self-owned @gofurry/* remains                       PASS
```

### Product brand

```text
Tap4Furry canonical                                   PASS
Tap4Furry Admin                                        PASS
README/docs/UI migrated                                PASS
old current product names removed                      PASS
```

### Domains

```text
tap4furry.com                                          PASS
admin.tap4furry.com                                    PASS
old current-domain refs removed                        PASS
localhost dev origins unchanged                        PASS
```

### Cookies

```text
__Host-tap4furry_session                               PASS
tap4furry_session                                      PASS
__Host-tap4furry_admin_session                         PASS
tap4furry_admin_session                                PASS
old product cookie names removed                       PASS
security semantics unchanged                           PASS
```

### Runtime artifacts

```text
tap4furry-api                                          PASS
tap4furry-admin                                        PASS
tap4furry-worker                                       PASS
tap4furry-server image                                 PASS
tap4furry-web image                                    PASS
tap4furry-admin image                                  PASS
tap4furry-postgres image                               PASS
```

### Preserved Infra

```text
gfp_dev unchanged                                      PASS
gfp_api/admin/worker/migrator/readonly unchanged       PASS
gfp_runtime unchanged                                  PASS
gfp: unchanged                                         PASS
migrations 00001–00005 unchanged                       PASS
private local envs unchanged                           PASS
```

### External dependency

```text
github.com/gofurry/easyhash preserved                  PASS
```

### Verification

```text
pnpm generate                                          PASS
second generation no drift                             PASS
pnpm check                                             PASS
pnpm integration:ci                                    PASS
pnpm build:images                                      PASS
```

All P0-1 auth/OAuth/Admin behavior remains green.

---

## 46. Stop Conditions

Stop and report if:

1. current `dev` is not the expected P0-1D baseline;
2. baseline CI is not green;
3. repository remote unexpectedly still targets the old repo;
4. Go-module rename requires a compatibility publication;
5. pnpm-scope rename requires a public package migration;
6. cookie rename would affect production users;
7. the rename appears to require a DB migration;
8. an applied migration would need editing;
9. `gfp_*` Infra would need renaming;
10. OAuth credentials would need regeneration;
11. shared Infra would require SSH/admin changes;
12. a secret would need to be committed.

---

## 47. Final Verification

Before completion:

1. run `pnpm check`;
2. run `pnpm generate`;
3. run it again and verify zero drift;
4. run disposable integration;
5. build images;
6. run safe development smokes where available;
7. run residual old-brand scan;
8. verify migration integrity;
9. inspect `git status --short`;
10. inspect `git diff --stat`;
11. inspect full diff;
12. verify no secret/local files tracked;
13. verify retained Infra IDs unchanged;
14. verify `github.com/gofurry/easyhash` still resolves;
15. update `CHANGELOG.md`.

Do not claim checks passed unless actually run.

---

## 48. Commit Contract

After verification:

```text
branch = dev
```

Recommended commit:

```text
chore: migrate platform branding to Tap4Furry
```

Do not push.

Do not merge `main`.

Do not tag/release.

---

## 49. Manual Operator Follow-up

Report but do not block repository completion on:

```text
GitHub Homepage:
https://gofurry.com
→ https://tap4furry.com

Google OAuth display name:
GoFurry Platform Dev
→ Tap4Furry Dev

GitHub OAuth App display name:
GoFurry Platform Dev
→ Tap4Furry Dev
```

Do not regenerate OAuth secrets.

Do not change localhost callbacks.

---

## 50. Final Codex Report

Return the report in Chinese.

Include:

### Migrated

```text
product branding
Go module
pnpm scopes
cookies
runtime/image names
production-domain contracts
OpenAPI
UI/docs
```

### Intentionally Preserved

```text
gfp_dev
gfp_* DB roles
gfp_runtime
gfp:
github.com/gofurry/easyhash
migrations 00001–00005
localhost development topology
real local OAuth credentials
```

### Residual Old-brand Scan

List each remaining `GoFurry/gofurry` match and explain why it is legitimate.

### Verification

List each command actually executed and pass/fail.

### Manual Follow-up

List GitHub Homepage and OAuth display-name work if still pending.

### Next Phase

Report:

```text
BRAND-0 complete
P0-1 implementation remains complete
Ready for P0-1 final human acceptance and P0-2 Taxonomy & Resource Core
```

### Git

```text
branch
final commit SHA
git status
```

Do not push.
