# Tap4Furry — Architecture Overview

## Architecture Style

> **Multi-process Modular Monolith**

One backend codebase, one primary PostgreSQL database, multiple runtime processes for security and operational separation.

```text
                              Internet
                                 │
                          Cloudflare Edge
                    DNS / CDN / WAF / Turnstile
                                 │
                           Host Nginx
                                 │
                  ┌──────────────┴──────────────┐
                  │                             │
             Public Web                    Admin Web
          Astro + React                 React + Vite
                  │                             │
                  ▼                             ▼
             Public API                    Admin API
                  │                             │
                  └──────────────┬──────────────┘
                                 │
                        Shared Go Domain
                                 │
                  ┌──────────────┼──────────────┐
                  │              │              │
             PostgreSQL        Redis          Worker
            Source of Truth   Ephemeral         │
                                              River
                                               │
                                      R2 / Email / Internet
```

## Runtime Units

Backend builds:

```text
tap4furry-api
tap4furry-admin
tap4furry-worker
```

They share Application/Domain code and do not call one another over HTTP.

## P0/P1 Infrastructure

```text
PostgreSQL
Redis
River
R2 / S3-compatible storage
Transactional Email
Cloudflare
Host Nginx
Docker Compose
```

Not P0/P1:

```text
MongoDB
NATS
Kubernetes
gRPC
Microservices
Service Mesh
Distributed Tracing
Elasticsearch/OpenSearch
Read Replica
Multi-region Active-Active
```

`pgvector` is a future PostgreSQL-extension path, not a current application dependency.

## Key Boundaries

- PostgreSQL is canonical.
- Redis is ephemeral.
- Public and Admin are separate transport/security boundaries.
- Cloudflare is acceleration/security infrastructure, not application runtime.
- Astro never connects directly to PostgreSQL.
- Public Web reads business data through Go Public API.
- Admin reads through Go Admin API.
- Worker reuses Application/Domain code.
- River is infrastructure, not business truth.

## Deployment

```text
Hong Kong VPS preferred
Tokyo fallback

Host:
Nginx

Docker:
Public Web
Admin Web
Public API
Admin API
Worker
PostgreSQL
Redis
River UI (optional)
```

Architecture complexity must be earned.
