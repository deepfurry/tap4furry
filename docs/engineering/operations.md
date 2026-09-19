# Tap4Furry — Operations Baseline

## Scope

This document intentionally does **not** define a full observability or alerting architecture.

Only the minimum operational requirements necessary for deployment and recovery are included.

## Health Endpoints

Backend processes should expose:

```text
/health/live
/health/ready
```

### Live
Answers whether the process is alive.

Do not fan out to every dependency.

### Ready
Answers whether the process can currently provide business service.

PostgreSQL may be a readiness dependency.

Redis should not necessarily be a hard readiness dependency because the architecture is designed
to degrade when Redis is unavailable.

## Logs

Go applications use:

```text
log/slog
structured JSON
stdout/stderr
```

Minimum context may include:

```text
service
environment
release_sha
request_id
error_code
```

Do not log credentials, raw sessions, OAuth tokens/codes, cookies, reset tokens, or secrets.

Docker log rotation must be enabled so logs cannot fill the VPS disk.

## Host Nginx

Use normal host logging and log rotation.

Do not log cookies/authorization secrets.

## Basic Production Discipline

- maintain current/previous release SHA
- preserve maintenance/fallback page
- never expose PostgreSQL/Redis publicly
- avoid building on production
- avoid destructive automatic DB rollback
- avoid `docker system prune --volumes`
- keep app images immutable and SHA-tagged
- treat PostgreSQL backup/restore as first-class

## Maintenance

Database runtime upgrades are explicit maintenance events.

Application releases and database-runtime releases have separate lifecycles.

## Explicitly Out of Scope

Not defined here:

- Prometheus
- Grafana
- Loki
- Tempo
- OpenTelemetry
- distributed tracing
- external alert routing
- metrics architecture
- alert thresholds
- SLO/SLA framework
