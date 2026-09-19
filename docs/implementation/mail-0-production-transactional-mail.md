# MAIL-0 — Production Transactional Mail

**Repository:** `deepfurry/tap4furry`
**Target branch:** `dev`
**Baseline:** `a13175bab218115f27c33dcb547f9589b38ec718`
**Prerequisite:** P0-1 + Human Acceptance + BRAND-0 complete
**Status:** Codex implementation specification

---

## 1. Goal

MAIL-0 closes the remaining production mail-delivery gap before P0-2.

The external provider and DNS are already prepared:

```text
Provider: Resend
Domain: tap4furry.com
Region: Tokyo
DKIM: verified
Return-Path: configured
DMARC: p=none
Open tracking: off
Click tracking: off

From:
Tap4Furry <no-reply@tap4furry.com>

Reply-To:
support@tap4furry.com
```

Cloudflare Email Routing is already configured for:

```text
support@tap4furry.com
security@tap4furry.com
copyright@tap4furry.com
```

MAIL-0 only connects the existing application-owned `ChallengeMailer` boundary to Resend.

It must not become a general mail subsystem.

---

## 2. Current Repository Boundary

Current auth owns:

```go
type ChallengeMailer interface {
    SendEmailVerification(context.Context, string, string) error
    SendPasswordReset(context.Context, string, string) error
}
```

Current implementations:

```text
Local
Disabled
```

Current delivery behavior is intentionally:

```text
issue challenge in PostgreSQL
→ commit transaction
→ deliver raw token
```

Raw challenge tokens are not persisted in:

```text
River
Redis
mail queue tables
logs
```

This invariant must remain.

Current API runtime supports:

```text
MAIL_MODE=local
MAIL_MODE=disabled
```

Production currently defaults to disabled delivery.

MAIL-0 adds `resend` and makes production mail configuration explicit.

---

## 3. Scope

Implement:

```text
Resend ChallengeMailer adapter
production mail configuration
verification email
password-reset email
plain-text + HTML bodies
bounded provider call
safe error flattening
unit/config tests
real opt-in Resend smoke
documentation
```

No database migration is required.

Do not edit migrations `00001` through `00005`.

---

## 4. Explicit Non-goals

Do not add:

```text
SMTP
River mail jobs
Redis mail queue
database outbox
retry queue
provider webhooks
delivery-status persistence
bounce processing
complaint processing
marketing email
newsletter
campaigns
audiences
email automation
template CMS
React Email
MJML
Resend hosted templates
open tracking
click tracking
attachments
localization framework
new Public/Admin endpoints
frontend changes
```

Do not modify auth token/challenge semantics.

---

## 5. Provider SDK

Use the official Resend Go SDK:

```text
github.com/resend/resend-go/v3
```

Use the context-aware send API:

```text
Emails.SendWithContext(...)
```

Do not use a hand-written Resend HTTP client unless the official SDK proves unsuitable during implementation.

Do not use SMTP.

Pin the resolved module version through normal `go.mod` / `go.sum`.

Do not upgrade unrelated dependencies.

---

## 6. Package Layout

Keep mail provider code inside:

```text
server/internal/mail/
```

Expected shape:

```text
local.go
resend.go
```

A small shared pure message/link renderer may be introduced if it avoids duplicating subject/path/body logic, but do not create a template framework.

Auth continues to depend only on:

```text
ChallengeMailer
```

and must not import Resend.

---

## 7. Configuration Contract

Extend API config with:

```text
MAIL_MODE
RESEND_API_KEY
MAIL_FROM
MAIL_REPLY_TO
```

Canonical production values:

```text
MAIL_MODE=resend
MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>
MAIL_REPLY_TO=support@tap4furry.com
```

### Development

Default remains:

```text
MAIL_MODE=local
```

Allowed:

```text
local
disabled
resend
```

### Test

Allow:

```text
local
disabled
resend
```

Tests must not make real network calls.

### Production

Require:

```text
MAIL_MODE=resend
```

Production must fail configuration validation rather than boot with disabled recovery mail.

Do not silently default production to `disabled`.

---

## 8. Config Validation

When:

```text
MAIL_MODE=resend
```

require:

```text
RESEND_API_KEY non-empty
MAIL_FROM valid RFC mailbox/address
MAIL_REPLY_TO valid RFC mailbox/address
```

Use standard-library address parsing where practical.

Do not print secret values in errors.

Do not require `MAIL_LOCAL_DIR` in Resend mode.

When:

```text
MAIL_MODE=local
```

preserve the existing private local-capture validation.

When:

```text
MAIL_MODE=disabled
```

no provider configuration is required.

Production must reject:

```text
local
disabled
missing/invalid Resend configuration
```

---

## 9. API Runtime Composition

Current runtime composition:

```text
Disabled
or
Local
```

becomes:

```text
switch MAIL_MODE:
  local    → Local
  resend   → Resend
  disabled → Disabled
```

Only Public API owns challenge delivery.

Do not add Resend configuration to:

```text
Admin API
Worker
Migrator
```

unless there is an actual runtime requirement.

---

## 10. Resend Adapter

Provide a constructor conceptually similar to:

```go
NewResend(apiKey, from, replyTo, publicOrigin string)
```

The adapter owns only provider delivery details.

It must:

```text
build verification/reset message
send through Resend
map every provider/network failure to package-level delivery failure
never expose raw provider error outside the mail package
```

Prefer a narrow injectable email-service interface internally so unit tests do not call Resend.

---

## 11. Link Contract

Preserve the existing fragment-token URLs:

```text
<PUBLIC_ORIGIN>/verify-email#token=<encoded-token>
<PUBLIC_ORIGIN>/reset-password#token=<encoded-token>
```

Examples:

```text
http://localhost:4321/verify-email#token=...
https://tap4furry.com/verify-email#token=...
```

Do not change to:

```text
?token=
```

Do not add token query parameters.

Continue escaping/encoding the token safely.

---

## 12. Email Verification Message

Subject:

```text
Verify your Tap4Furry email
```

Plain-text body should communicate:

```text
Verify your Tap4Furry email.
The link expires in 24 hours.
Verification URL.
Ignore the message if the recipient did not request it.
```

HTML body should be minimal:

```text
Tap4Furry heading
short explanation
Verify email CTA/link
24-hour expiration
ignore-if-not-requested note
```

No remote images.

No tracking pixels.

No JavaScript.

No unnecessary external links.

---

## 13. Password Reset Message

Subject:

```text
Reset your Tap4Furry password
```

Plain-text body:

```text
Password reset requested.
The link expires in 30 minutes.
Reset URL.
Ignore the message if the recipient did not request it.
```

HTML body:

```text
Tap4Furry heading
short explanation
Reset password CTA/link
30-minute expiration
ignore-if-not-requested note
```

No remote images/tracking.

---

## 14. Sender Contract

Every Resend challenge email uses:

```text
From:
MAIL_FROM

Reply-To:
MAIL_REPLY_TO
```

Expected production identity:

```text
Tap4Furry <no-reply@tap4furry.com>
support@tap4furry.com
```

Do not set CC/BCC.

Do not add custom tracking.

Do not attach provider metadata to the message.

---

## 15. Delivery Timeout

Current post-commit auth delivery timeout is 2 seconds.

Change it to:

```text
5 seconds
```

Reason:

```text
Local capture is filesystem-only.
Resend is a bounded Internet request.
Delivery occurs after DB commit and does not hold the auth transaction.
```

Do not increase unrelated HTTP/database timeouts.

---

## 16. Failure Semantics

Preserve current auth behavior.

### Registration

If mail delivery fails:

```text
registration remains committed
session remains valid
safe warning only
```

Do not roll back the account.

### Authenticated verification resend

Provider failure:

```text
MAIL_UNAVAILABLE
```

through the existing safe auth error behavior.

### Password reset request

Preserve account-enumeration resistance:

```text
known / unknown / ineligible / provider-failure
→ same public accepted response
```

Never reveal whether Resend accepted a particular recipient.

---

## 17. No Retry / No Queue

MAIL-0 sends each post-commit challenge once.

Do not add automatic retry.

Do not persist the raw token for later retry.

Do not add a River job containing:

```text
recipient
raw token
verification URL
reset URL
```

If durable retry is needed later, design it as a separate security-sensitive phase.

---

## 18. Logging / Secret Safety

Never log:

```text
RESEND_API_KEY
recipient email
raw challenge token
verification/reset URL
provider request body
provider response body
Authorization header
```

Safe logs may contain only static operational labels such as:

```text
component=mail
provider=resend
operation=verification
```

Do not pass raw Resend error strings into HTTP responses.

---

## 19. Repository Secret Audit

Extend repository/private-value auditing so API keys are treated as secrets.

At minimum:

```text
RESEND_API_KEY
generic *_API_KEY where appropriate
```

must be considered private when reading ignored local env inputs.

Do not emit matched secret values.

Continue rejecting tracked:

```text
.local/*
server/env/*.local
```

private configuration.

---

## 20. `server/env/api.example`

Update the public example without real secrets.

Suggested shape:

```dotenv
APP_ENV=development
HTTP_ADDR=127.0.0.1:8080
PUBLIC_ORIGIN=http://localhost:4321

MAIL_MODE=local
MAIL_LOCAL_DIR=../.local/mail

# Production example:
# MAIL_MODE=resend
# RESEND_API_KEY=replace-me
# MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>
# MAIL_REPLY_TO=support@tap4furry.com
```

Retain existing DB/Redis/OAuth placeholders.

---

## 21. Real Resend Smoke

Add an explicit opt-in command:

```text
pnpm smoke:mail:resend:dev
```

It must not require creating a User or DB challenge.

Use an ignored input:

```text
.local/resend-smoke.env
```

Expected private fields:

```text
RESEND_API_KEY=<real restricted production key>
RESEND_TEST_RECIPIENT=<an address controlled by the operator>
```

Do not require the user to paste these into source code.

The smoke may use the canonical public non-secret values:

```text
MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>
MAIL_REPLY_TO=support@tap4furry.com>
PUBLIC_ORIGIN=http://localhost:4321
```

or allow explicit private overrides where useful.

Smoke behavior:

1. validate private input exists;
2. generate a synthetic random non-production challenge token;
3. call the real Resend adapter once;
4. send a verification-style smoke email to `RESEND_TEST_RECIPIENT`;
5. print only a safe PASS/failure summary;
6. do not print recipient, token, URL, API key, or provider response body.

No database mutation.

No cleanup is required beyond process exit.

If the environment cannot reach Resend, report it honestly.

---

## 22. CI

CI must never use the real Resend key.

Unit tests use a fake injected email service.

Existing disposable integration remains offline from external mail providers.

Required:

```text
pnpm check
pnpm generate
generated drift
pnpm integration:ci
pnpm build:images
```

plus MAIL-0 unit/config tests.

Do not add network-dependent CI.

---

## 23. Required Tests

### Config

Test:

```text
development default local
development resend valid
development resend missing key rejected
invalid MAIL_FROM rejected
invalid MAIL_REPLY_TO rejected

production resend valid
production local rejected
production disabled rejected
production missing key rejected
```

### Adapter

Verify exact provider request fields:

```text
From
ReplyTo
To
Subject
Text
Html
```

for:

```text
verification
password reset
```

### Content

Verify:

```text
verification path
reset path
fragment token
24-hour text
30-minute text
Tap4Furry branding
no tracking/remote-image content
```

### Error handling

Test:

```text
canceled context
provider failure
safe flattened error
```

### Regression

Keep existing:

```text
Local capture tests
Disabled behavior
Auth verification
Auth password reset
enumeration resistance
P0-1 integration
```

green.

---

## 24. Documentation

Update:

```text
README.md
CHANGELOG.md
docs/development.md
docs/architecture/security.md
docs/engineering/tech-stack.md
server/env/api.example
```

Document:

```text
Resend is production transactional mail provider
local capture remains development default
production requires resend
From / Reply-To contract
tracking remains disabled externally
raw challenge tokens remain non-durable
real smoke procedure
```

Update stale text claiming no production mail provider exists.

Do not rewrite unrelated P0-1 documentation.

---

## 25. No Migration / Infrastructure Mutation

MAIL-0 requires:

```text
NO Goose migration
NO PostgreSQL role changes
NO Redis ACL changes
NO River migration
NO DNS modification
NO Cloudflare modification
NO Resend domain modification
```

The provider/domain/DNS configuration already exists externally.

Do not touch `gfp_*` or `gfp:` identifiers.

---

## 26. Implementation Order

1. Audit current `dev` and confirm baseline.
2. Run baseline `pnpm check`.
3. Add official Resend Go SDK.
4. Extend API mail config.
5. Add Resend adapter and narrow test injection boundary.
6. Add minimal text/HTML challenge rendering.
7. Compose `MAIL_MODE=resend` in Public API.
8. Change post-commit delivery timeout 2s → 5s.
9. Extend secret audit for API key material.
10. Add unit/config tests.
11. Add opt-in `smoke:mail:resend:dev`.
12. Update docs/env/changelog.
13. Run full generate/check/integration/image acceptance.
14. If private smoke input exists, run real Resend smoke.
15. Inspect diff and secret audit.
16. Commit locally on `dev`.
17. Do not push.

---

## 27. Acceptance Criteria

### Architecture

```text
ChallengeMailer boundary unchanged             PASS
Auth has no Resend dependency                  PASS
no queue/outbox/retry                          PASS
no DB migration                               PASS
```

### Configuration

```text
MAIL_MODE=resend                              PASS
RESEND_API_KEY                                PASS
MAIL_FROM                                     PASS
MAIL_REPLY_TO                                 PASS
production requires resend                    PASS
```

### Delivery

```text
verification email                            PASS
password-reset email                          PASS
plain text                                    PASS
HTML                                          PASS
From/Reply-To                                 PASS
5s bounded context                            PASS
```

### Security

```text
token remains fragment-only                   PASS
raw token not persisted                       PASS
API key not logged                            PASS
recipient/token/URL not logged                PASS
provider error flattened                      PASS
secret audit recognizes API key               PASS
```

### Regression

```text
local capture                                 PASS
disabled test path                            PASS
auth verification/reset semantics             PASS
enumeration resistance                        PASS
P0-1 tests                                    PASS
```

### Repository

```text
pnpm check                                    PASS
pnpm generate                                 PASS
generated drift                               PASS
pnpm integration:ci                           PASS
pnpm build:images                             PASS
```

### Real provider

If `.local/resend-smoke.env` is supplied:

```text
pnpm smoke:mail:resend:dev                    PASS
```

Do not claim PASS if not actually run.

---

## 28. Stop Conditions

Stop and report instead of broadening scope if:

1. current mail boundary materially differs from this specification;
2. production mail would require persisting raw challenge tokens;
3. Resend integration requires SMTP or a durable queue;
4. existing auth semantics must change;
5. a DB migration appears necessary;
6. shared Infra/DNS/Cloudflare changes appear necessary;
7. real Resend API key would need to be committed or printed;
8. CI would require external network mail delivery.

---

## 29. Commit Contract

After verification:

```text
branch = dev
```

Recommended commit:

```text
feat: add production transactional email
```

Do not push.

Do not merge `main`.

Do not tag/release.

---

## 30. Final Codex Report

Return the report in Chinese.

Include:

### Implemented

```text
Resend adapter
config contract
mail rendering
runtime composition
5s timeout
secret audit
real smoke command
```

### Security

Confirm:

```text
raw challenge tokens remain non-durable
no secret/provider payload logging
no retry/outbox
```

### Verification

List all commands actually run and PASS/FAIL.

Separate:

```text
offline/unit/CI verification
real Resend smoke
```

### Production Contract

Report:

```text
MAIL_MODE=resend
MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>
MAIL_REPLY_TO=support@tap4furry.com
```

Never print `RESEND_API_KEY`.

### Next Phase

If all gates pass:

```text
MAIL-0 complete
Ready for P0-2 — Taxonomy & Resource Core
```

### Git

Report:

```text
branch
commit SHA
git status
```

Do not push.
