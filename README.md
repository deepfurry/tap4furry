# Tap4Furry

A discovery and exchange platform for the furry ecosystem, centered on resource
knowledge. P0-1 Identity/Auth and human acceptance are complete. MAIL-0 connects
verification and recovery mail to Resend. P0-2A adds the ten-table Resource Core,
pure Taxonomy/Resource primitives, localization and Resource revision/CAS groundwork.
P0-2B exposes anonymous Resource/taxonomy read APIs and `/resources` Astro SSR pages,
with field-level locale fallback and sanitized Markdown. P0-2C Admin Resource Curation
is next; canonical writes, search and contributions remain future phases.
Production deployment sign-off remains a separate gate.

Canonical repository: [deepfurry/tap4furry](https://github.com/deepfurry/tap4furry).
Future production origins are `https://tap4furry.com` and
`https://admin.tap4furry.com`, each with same-origin `/api/*` routing.

## Development

Requires Go 1.27.1+, Node 24 LTS and pnpm 10.11.0. Docker is required for disposable
infrastructure and image acceptance. The repository uses one Go module and a pnpm
workspace, with no global Go generator installation required.

```text
pnpm install --frozen-lockfile
pnpm generate
pnpm check
```

Prepare private `server/env/*.local` inputs as described in
[development.md](docs/development.md). Existing real files must be preserved. In
separate terminals, `pnpm dev:api`, `pnpm dev:admin-api`, `pnpm dev:worker`,
`pnpm dev:web` and `pnpm dev:admin` start the applications.

Mail defaults to private local capture in development. Production Public API
requires explicit `MAIL_MODE=resend`, a private `RESEND_API_KEY`,
`MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>` and
`MAIL_REPLY_TO=support@tap4furry.com`. Verification/reset emails use plain text and
minimal HTML; tracking stays disabled at the provider. Delivery is one bounded
post-commit attempt, with no durable raw-token queue. The opt-in
`pnpm smoke:mail:resend:dev` reads only ignored `.local/resend-smoke.env` and sends
one synthetic email; see the [private smoke procedure](docs/development.md#transactional-mail-mail-0).

## Repository map

| Area | Responsibility |
| --- | --- |
| `apps/web` | Public Astro Node SSR with React islands |
| `apps/admin` | React/Vite SPA with TanStack Router/Query |
| `packages/api-client`, `packages/design` | Generated API facades and shared SCSS |
| `server` | API/Admin/Worker, drivers, jobs, migrations and generators |
| `contracts` | OpenAPI and durable engineering constraints |
| `deploy` | Server/Web/Admin/PostgreSQL image definitions |
| `docs` | Product, architecture, implementation and development guidance |

`dev` is the current integration branch; `main` is a stable release snapshot.
Publishing, merging to `main`, tagging and releasing require explicit instruction.

`gfp_*` and `gfp:` are intentionally retained stable infrastructure identifiers and
are not product-brand surfaces. BRAND-0 changes no database schema or infrastructure.

Start with [AGENTS.md](AGENTS.md), the [P0-2B specification](docs/implementation/p0-2b-public-resource-read-surface.md),
[product overview](docs/product/PRODUCT.md) and [architecture](docs/architecture/ARCHITECTURE.md).
See [CHANGELOG.md](CHANGELOG.md) and [deployment artifacts](deploy/README.md).

License: **AGPL-3.0-only**. See [LICENSE](LICENSE).
