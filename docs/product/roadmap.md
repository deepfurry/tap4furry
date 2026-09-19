# Tap4Furry — Product Roadmap

## P0 — Resource Platform

Goal:

```text
Find
Understand
Organize
Contribute
```

Includes:

- Homepage / Discover
- Resources / Resource Detail
- Search
- Category / Tag
- Collections
- Public Profile
- Email / Google / GitHub auth
- Save / Want / Have
- Submit / Edit / Add Source / Report
- basic trust and contribution budget
- review queue
- content eligibility
- audit
- source health basics

Not P0:

- public Discussions
- Poll product
- Exchange UI
- public Insights
- semantic/vector search
- real-time DM
- advanced Organization management
- payments
- general file hosting

## P0.5 — Exchange & Claims

- Want / Trade / Offer
- automatic expiration
- Contact Request
- Block
- Claim Resource
- verified creator/developer/maintainer/publisher relation
- basic Organization profile

## P1 — Community & Public Signals

- Discussions
- Polls
- Follow Resource / Creator / Collection
- Notifications
- Most Wanted
- Hidden Gems
- public Insights
- Resource Landscape
- Availability
- Growing Tags
- official Poll results

## P2 — Ecosystem Intelligence

Evaluate only when usage justifies:

- semantic search;
- pgvector embeddings;
- hybrid search;
- graph recommendation;
- advanced personalization;
- Surveys;
- historical trend reports;
- Open Data;
- public API;
- Resource Graph visualization.

## Evolution Gates

### pgvector
Add only when semantic search/recommendation has demonstrated product value.

### NATS
Add only when the modular monolith becomes genuinely distributed and multiple independent consumers need durable event fan-out.

Likely future path:

```text
PostgreSQL transaction
+
Transactional Outbox
    ↓
NATS JetStream
    ↓
independent services
```

### MongoDB
No current planned role. Re-evaluate only if a genuinely independent document-oriented domain appears.

### River
Use in P0/P1 for durable jobs, but isolate it behind the Jobs infrastructure boundary.
