# Tap4Furry — CI/CD

## CI

GitHub Actions is the CI system.

Pull Request / push checks should cover:

```text
Contract / Codegen
Go
Frontend
Database / Migration
Integration
Docker image build
```

## Codegen Drift

CI regenerates:

- OpenAPI Go server code
- Orval TypeScript clients
- sqlc output

Then:

```text
git diff --exit-code
```

Generated-code drift fails CI.

## Go

Typical checks:

```text
gofmt check
go vet
go test
```

Database integration tests use real PostgreSQL, not SQLite emulation.

## Frontend

Typical checks:

```text
pnpm lint
pnpm typecheck
pnpm test
web build
admin build
```

## Database

CI validates:

```text
fresh empty DB
→ goose up
→ River migrate
→ integration smoke/tests
```

## Images

After merge to main:

```text
tap4furry-server:<git-sha>
tap4furry-web:<git-sha>
tap4furry-admin:<git-sha>
```

are built and pushed to GHCR.

The Tap4Furry PostgreSQL runtime has a separate release lifecycle.

## Production Deployment

Do **not** auto-deploy every main merge in P0.

CI produces trusted artifacts.

Production deployment is an explicit controlled action from a trusted management path.

Typical VPS command:

```text
deploy <git-sha>
```

The deployment script handles:

- image pull
- preflight
- migrations
- Worker/API/Web update
- health checks
- release record

## Production Host

Production does not build source.

It only pulls immutable images.

## Release Metadata

Track:

```text
CURRENT_SHA
PREVIOUS_SHA
DEPLOYED_AT
```

Rollback deploys the previous application image SHA.

Database schema is forward-fixed by default.

## Dependency Maintenance

Use dependency automation for visibility, but do not auto-merge architecture-critical upgrades.

Especially review:

- River
- PostgreSQL runtime
- Fiber
- Astro
- sqlc
- oapi-codegen
- major frontend framework/runtime changes

Pin third-party GitHub Actions to immutable commit SHAs where practical.
