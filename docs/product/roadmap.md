# Tap4Furry — Product Roadmap

## Current Position

Reviewed on 2026-09-23 against `dev` at `96ef1d5`.

| Stage | Status | Delivered outcome |
| --- | --- | --- |
| P0-0 | Completed | Engineering foundation, generated contracts, disposable CI and runtime images |
| P0-1A/B/C/D | Completed | Identity, local/OAuth auth, recovery, sessions and isolated Admin auth |
| BRAND-0 / MAIL-0 | Completed | Tap4Furry branding and Resend transactional mail |
| P0-2A | Completed | Resource/Taxonomy domain and ten-table Resource Core |
| P0-2B | Completed | Anonymous Resource/taxonomy APIs and Resource list/detail Astro SSR |
| P0-2C | Completed | Admin curation, publication, live capabilities and transactional Resource CAS |
| P0-3–P0-7 | Planned | Contributions, discovery, organization, governance and full P0 integration |

The implemented loop is **curate → publish → browse → visit a Source**.
[P0-2 acceptance](../implementation/p0-2c-verification.md) records the implementation
checks; it does not establish production readiness. The homepage still emphasizes
accounts and engineering health. Launch content, production operation and real user
feedback remain to be accepted separately.

## Delivery Strategy

Ship a **curated Resource directory Beta before completing all of P0**. Anonymous
visitors should find and understand useful resources without registering. Curators
use the existing Admin application; community contribution remains the longer-term
direction, with manual feedback handling as the initial bridge.

This roadmap owns delivery order. Product/architecture designs describe the target
system; their full menus and domains are not first-launch requirements. Existing
implementation specifications retain their historical scope and acceptance evidence.
P0 phase numbers remain capability identifiers, not a mandatory serial release order.
The older external implementation outline is background, not the current progress list.

```text
Completed P0-2
  → BETA-1: usable public journey
  → BETA-2 + BETA-3: launch content and production readiness
  → BETA-4: small-cohort launch and feedback
  → one evidence-selected P0 capability slice at a time
```

Content preparation and operator readiness can proceed alongside BETA-1; public
launch waits for all three gates. These are work milestones, not release versions
or promised dates. Planning does not authorize deployment, image publishing, push,
main merges, tags or releases; those remain explicit operator decisions.

## First Launch — Curated Resource Beta

### BETA-1 — Public Journey

**Status:** Next. **Depends on:** completed P0-2.

**Outcome:** A new visitor can reach a useful Resource and its Source from the homepage.

- [ ] Replace the engineering welcome/health surface with a resource-first homepage
  and clear Resources navigation; keep account access secondary.
- [ ] Reuse the existing anonymous list/detail APIs and SSR pages. Make titles,
  summaries, category/tag labels, availability and Source actions understandable.
- [ ] Finish mobile layout, keyboard navigation, empty/error states and basic visual
  consistency for this journey. Show only implemented navigation/actions.
- [ ] Add `/privacy`, `/terms` and visible feedback/content-concern contact information,
  describing actual behavior and the Beta's limits.

**Acceptance:** On desktop and mobile, an anonymous visitor can follow homepage →
Resource list → detail → Source without a dead end or login requirement. Existing
visibility, locale, Markdown safety, canonical/status/cache and DTO privacy tests pass.
No new domain, schema, search engine, media upload or Resource-page hydration is needed.

### BETA-2 — Useful Launch Content

**Status:** Planned; preparation can start now. **Depends on:** existing Admin curation;
final content acceptance uses BETA-1 and the BETA-3 target environment.

**Outcome:** The directory offers a coherent, useful collection rather than test fixtures.

- [ ] Choose one initial resource niche and launch language; define its content/rating
  boundary using the [content policy](content-policy.md).
- [ ] Prepare roughly 30–50 useful resources as a planning target, with consistent
  taxonomy, informative summaries, canonical localization and reviewed Source links.
- [ ] Enter and publish through authorized Admin workflows in the approved launch
  environment. Keep smoke fixtures and shared development data separate from launch data.
- [ ] Name the curator/operator responsible for corrections and rights/content concerns;
  exercise the contact → review → correct/restrict/remove process with a test report.

**Acceptance:** A curator has checked the launch set through the Public surface,
including duplicate/rights/availability review. Visitors have meaningful choices;
content quality takes precedence over the target count. The manual concern-handling
path works before public exposure. Automated ingestion and Report/Trust domains wait.

### BETA-3 — Minimum Production Readiness

**Status:** Planned. **Depends on:** an operator-approved production environment and
the BETA-1 release candidate. Pull forward the necessary launch gates from P0-7.

**Outcome:** The selected revision can be operated, recovered and rolled back safely.

- [ ] Implement the missing release/deployment artifacts using the existing
  [deployment design](../architecture/deployment.md) and [CI/CD design](../engineering/ci-cd.md):
  immutable SHA images, explicit migrations, health checks and current/previous SHA.
  Production pulls images; it never uses shared development databases or credentials.
- [ ] Verify HTTPS, same-origin API routing, internal SSR origin, private DB/Redis
  access and Cloudflare Access + MFA for Admin. Validate public abuse protections
  appropriate to the exposed auth surfaces; preserve application throttling.
- [ ] Inject production secrets privately. Exercise deployed registration, verification,
  recovery, Google/GitHub flows, Admin login/role revocation and curator publication.
  Keep provider-specific setup and any required provider approval explicitly tracked.
- [ ] Verify SEO/cache behavior and secret-safe logs on the real proxy path; establish
  health inspection, log rotation, maintenance handling and an incident contact.
- [ ] Set up off-host PostgreSQL backups and restore verification according to the
  [backup contract](../engineering/backup-restore.md). Record a disposable restore
  rehearsal, application rollback and the measured recovery result before opening.
- [ ] Run the candidate's required repository/integration/image gates and deployed
  smoke/human acceptance; record exact SHA, results and remaining blockers.

**Acceptance:** An operator can deploy, diagnose, restore data and roll back application
images using the recorded procedure. The Beta journey and all exposed auth methods
work on the real domains. No unresolved access/privacy/data-loss blocker remains.
Actual publishing and deployment need separate authorization. No observability stack,
new queue architecture or shared-development infrastructure changes are prerequisites.

### BETA-4 — Small-Cohort Launch and Feedback

**Status:** Planned. **Depends on:** BETA-1/2/3 accepted and explicit launch authorization.

**Outcome:** Real usage determines the next development investment.

- [ ] Invite an initial 10–20 target users as a planning target; clearly label the
  Beta and its available features. Anonymous browsing remains open to those visitors.
- [ ] Observe whether users can find a relevant resource and understand/visit its
  Source. Ask what is missing and whether they would return; use voluntary feedback
  and manual notes, without introducing behavioral tracking or an analytics stack.
- [ ] Hold the first review after roughly one to two weeks of use, then review after
  each small delivery. Record concrete examples, severity, frequency and the chosen
  next slice; insufficient feedback calls for more outreach/content, not more domains.

**Acceptance:** Initial feedback is reviewed and one next outcome is selected. Fix
blocking defects first; expand the audience only when operation and concern handling
are manageable. Beta launch is a milestone, not a claim that all of P0 is complete.

## P0 — Feedback-Driven Resource Platform

The full goal remains **Find → Understand → Organize → Contribute**. Complete it in
small vertical slices after Beta, with the following dependency and priority rules.

| Capability | First useful slice and acceptance | When to prioritize / dependency |
| --- | --- | --- |
| P0-4 Search & Discovery | Basic text query and a small set of category/tag filters with bounded pagination; relevant results preserve Public visibility, locale and Source/Relation privacy | First if visitors repeatedly cannot find existing resources; PostgreSQL/pg_trgm only, no new engine |
| P0-3 Contribution & Review | Resource suggestion or correction → bounded proposal → Admin review → accepted canonical update with traceable outcome | First if missing/corrected content and curator workload dominate; reuse curation invariants, add minimum anti-abuse and reviewer history before enabling submissions |
| P0-5 Collections + Save / Want / Have | Start with private Save and a saved list; add Collections and Want/Have only when their distinct use is demonstrated | After repeat visitors ask to keep/organize resources; ownership and default privacy tested before exposure |
| P0-6 Trust / Reports / Moderation | Structured report → operator decision → recorded action; then restrictions, contribution budgets and trust evidence as needed | Manual handling starts in Beta; required abuse controls accompany each new public-write capability, never wait for the entire phase |
| P0-7 Integration & Hardening | Validate the implemented cross-domain journeys, constraints, privacy, performance, recovery and operational readiness | Launch-critical work is already required in BETA-3; broaden regression and readiness as capabilities expand |

Prefer P0-4's basic findability slice when discovery friction is the first observed
problem; choose P0-3 first when contribution demand is stronger. Record that choice
before implementation instead of starting both full phases. No new architecture is
needed to make this priority decision.

The remaining P0 backlog also includes public profile presentation, richer Discover
surfaces, content eligibility, audit and Source-health basics. Account identity,
manual content review and Resource eligibility already have foundations; distinguish
those from the unfinished public UI and automated workflows. None of this backlog
silently becomes a Beta launch gate. Preserve the separation between contribution
history, governance audit and authentication security events.

## Working Rhythm and Verification

- Select one user-visible outcome per implementation slice, with a short scope,
  non-goals, dependencies and observable acceptance criteria. Write only the next
  slice's detailed specification, not every future phase in advance.
- Reuse Auth, curation, generated clients and existing UI/runtime boundaries. Add
  abstractions, dependencies, jobs or schemas only when the selected slice uses them.
- During implementation run focused checks for changed behavior. At completion follow
  the [engineering playbook](../../.agents/playbook.md): `pnpm check`, regeneration
  and drift validation, plus integration, migration, smoke and image gates required
  by the active specification. CI retains disposable infrastructure and full gates.
  Repeat expensive checks when changes or failures invalidate prior evidence.
- Keep security, authorization, CAS, migration history, grants, privacy and recovery
  requirements intact. Simplify initial UX and manual operations rather than weakening
  these boundaries. Migrations 1–6 and stable `gfp_*` / `gfp:` identifiers remain protected.
- Record actual completion and verification evidence here/by link after each milestone;
  separate code acceptance, operational readiness and user feedback. Do not claim a
  deployment or product capability from a design document alone.

## Deferred / Long-term

These preserve product direction; they are not commitments for the next release.

| Stage | Scope | Entry condition |
| --- | --- | --- |
| P0.5 — Exchange & Claims | Want/Trade/Offer, expiration, contact requests, blocking, creator/maintainer claims and basic organizations | Resource usage is established and demand plus operator capacity justify exchange |
| P1 — Community & Public Signals | Discussions, polls, following, notifications, aggregate Insights/Most Wanted/Hidden Gems/landscape/availability/growing tags | Recurring usage and useful public data justify each surface |
| P2 — Ecosystem Intelligence | Semantic/hybrid search, recommendations, graph visualization, advanced personalization, surveys, trends, Open Data and a public API | Measured needs exceed the simpler resource/discovery model |

Payments, escrow, transaction guarantees, generic file hosting and generic social
feeds remain [product non-goals](PRODUCT.md#non-goals). No real-time DM or advanced
organization management in P0.

### Evolution Gates

- **pgvector:** only after semantic search/recommendation demonstrates value and a
  separate architecture decision changes the current no-vector boundary.
- **NATS:** only for a genuinely distributed system requiring durable independent
  consumers; reassess transactional outbox needs then, not for Beta.
- **MongoDB:** no planned role; reconsider only for a justified independent domain.
- **River:** existing infrastructure boundary remains; add durable business jobs only
  when a delivered capability needs them. No speculative workers or mail queues.
