# Tap4Furry — Discovery

## Model

```text
Search          → relevance
Discover        → diversity
Recommendation  → relationship
Ranking         → order
Distribution    → actual exposure
```

Popularity is a signal, not a right to visibility.

## Pipeline

```text
Candidate Generation
        ↓
Eligibility Filter
        ↓
Ranking
        ↓
Diversification
        ↓
Distribution
```

## Signals

Keep independent:

```text
Quality
Trust
Freshness
Popularity
Demand
Engagement
Editorial
Risk
```

Do not collapse the product into one universal Resource score.

## P0 Search

Use PostgreSQL:

```text
FTS
+
pg_trgm
```

Prioritize roughly:

```text
Exact Name
> Name Similarity
> Canonical Tags
> Summary
> Description
```

## Search Miss → Contribution

```text
No resources found.
Couldn't find it?
[ Submit this resource ]
```

## Discover Surfaces

- New
- Trending
- Updated
- Most Wanted
- Hidden Gems
- Curated
- Explore
- For You (later)

## Cold Start

New Resources receive controlled exploration exposure based on quality/trust/risk signals.

Bulk submissions do not receive proportional homepage exposure.

## Publish ≠ Distribution

Content may be valid and public while receiving limited exposure.

## Anti-manipulation

Public count ≠ ranking weight.

Ranking may consider account trust, signal freshness, behavior diversity, and anomaly signals.

## P0 Recommendation

Use explainable content-based similarity:

```text
Tag overlap
Category overlap
Type
Creator / Organization
Resource Relations
Shared Collections
```

## Later

P2 may evaluate semantic search, embeddings, hybrid search, collaborative filtering, and graph recommendation.

`pgvector` is a future PostgreSQL extension path, not a P0 dependency.
