# Tap4Furry — Repository & Code Generation Layout

## Monorepo

```text
tap4furry/
│
├── apps/
│   ├── web/
│   └── admin/
│
├── packages/
│   ├── api-client/
│   ├── react-ui/
│   ├── design/
│   └── i18n/
│
├── server/
│   ├── cmd/
│   │   ├── api/
│   │   ├── admin/
│   │   └── worker/
│   ├── internal/
│   │   ├── auth/
│   │   ├── identity/
│   │   ├── resource/
│   │   ├── taxonomy/
│   │   ├── collection/
│   │   ├── interaction/
│   │   ├── contribution/
│   │   ├── discovery/
│   │   ├── exchange/
│   │   ├── discussion/
│   │   ├── poll/
│   │   ├── trust/
│   │   ├── moderation/
│   │   ├── notification/
│   │   ├── analytics/
│   │   ├── transport/
│   │   ├── jobs/
│   │   ├── database/
│   │   ├── storage/
│   │   ├── mail/
│   │   └── platform/
│   ├── db/
│   │   ├── migrations/
│   │   ├── queries/
│   │   └── sqlc.yaml
│   ├── codegen/
│   │   └── oapi/
│   ├── go.mod
│   └── go.sum
│
├── contracts/
│   └── openapi/
│       ├── public.yaml
│       └── admin.yaml
│
├── deploy/
├── docs/
├── package.json
├── pnpm-workspace.yaml
├── pnpm-lock.yaml
└── README.md
```

## Workspace

```text
pnpm workspace
```

No Turbo/Nx initially.

Backend uses one `server/go.mod`.

## Contract Ownership

Top-level:

```text
contracts/openapi/public.yaml
contracts/openapi/admin.yaml
```

Public/Admin specs remain separate.

## Codegen

```text
OpenAPI
├── oapi-codegen → Go Transport
└── Orval        → TypeScript Client

Goose migrations
└── sqlc schema source

SQL queries
└── sqlc → Go DB package
```

Do not maintain duplicate `schema.sql`.

## Generated Code

Generated outputs are committed to Git but never manually edited.

CI regenerates and checks:

```text
git diff --exit-code
```

## Go Tools

Pin generator tooling with the Go module/tool mechanism where practical.

Use `go generate` for Go-side generation.

Expose one root orchestration command:

```text
pnpm generate
```

## SQL Location

Centralize SQL source:

```text
server/db/queries/
```

with domain-oriented files.

Generated sqlc output may be centralized under:

```text
server/internal/database/sqlc/
```

## UI Sharing

`packages/react-ui` contains reusable primitives only.

Public Astro business components remain in `apps/web`.

Admin business components remain in `apps/admin`.

## Avoid Garbage Drawers

Do not create catch-all `utils`, `common`, `misc`, `helpers`, or `shared` packages without a real stable abstraction.

Prefer a little duplication over premature abstraction.
