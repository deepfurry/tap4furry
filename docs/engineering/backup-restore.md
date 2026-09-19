# Tap4Furry — Backup & Restore

## Principle

> A backup is not trusted until restore has been tested.

## P0 Recovery Targets

Engineering targets, not external SLA:

```text
RPO ≈ 6 hours
RTO ≈ 2 hours
```

Increase protection when user-generated data becomes valuable enough that this loss window is unacceptable.

## PostgreSQL P0 Backup

Start simple:

```text
Every ~6 hours
→ pg_dump -Fc
→ remote backup storage
```

Also periodically save required cluster/global objects such as roles.

Use PostgreSQL tooling compatible with the production runtime.

Prefer using the Tap4Furry PostgreSQL runtime image as the backup-tool environment.

## Remote Storage

Backups must not exist only on the production VPS.

Local backup directory is staging only.

Preferred long-term:

```text
Production R2
≠
Database backup provider/account
```

At minimum use separate bucket/credentials and strong retention controls.

## Retention Example

```text
6-hour backups → 7 days
daily          → 30 days
weekly         → 3 months
monthly        → 12 months
```

Exact retention may change with cost and data value.

## Restore Verification

Automate a weekly restore test:

```text
latest remote dump
    ↓
download
    ↓
disposable PostgreSQL container
    ↓
pg_restore
    ↓
smoke validation
    ↓
destroy
```

Smoke checks may include:

- DB starts
- app schema exists
- migration metadata is valid
- important table counts are plausible
- critical FK/query paths work

Restore failure is a critical operational defect.

## PITR

Do not add PITR in P0 solely for architecture completeness.

Introduce WAL archiving/PITR when:

- RPO ≈ 6h is no longer acceptable
- public contributions become difficult to reconstruct
- operational maturity justifies more complex recovery

At that point evaluate dedicated tools such as pgBackRest or WAL-G.

## Redis

No backup.

If Redis contains data that cannot be lost, that data belongs in the wrong store.

## River

River tables are included in PostgreSQL backup, but exact in-flight job recovery is not a core recovery objective.

After restore:

```text
canonical app state
→ reconciliation
→ recreate necessary work
```

## Media

R2/S3 is the canonical media store.

Protect against accidental deletion/credential compromise with retention and, when justified,
a secondary off-account/off-provider copy.

## Secrets Recovery

Maintain a secure off-VPS recovery copy of:

- OAuth secrets
- R2 credentials
- email credentials
- Turnstile secret
- DB credentials
- other production secrets

Git alone is not sufficient for disaster recovery.

## PostgreSQL Runtime Upgrade

Minor runtime updates replace the image while preserving compatible PGDATA.

Major upgrades are separate planned maintenance events.

External extensions such as pgvector must be available as required by the chosen upgrade procedure.
