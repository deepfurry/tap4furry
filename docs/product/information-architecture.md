# Tap4Furry — Information Architecture

This is the target product structure. Follow the [Chinese roadmap](roadmap.md) for
the agreed launch scope and implementation order. Deliver complete usable journeys
for the included areas; placeholder navigation does not count as implementation.

## Primary Navigation

```text
Discover
Resources
Exchange
Community
```

Global actions:

```text
Search
Submit
User
```

## Public IA

```text
Tap4Furry
│
├── Discover
│   ├── Trending
│   ├── New
│   ├── Updated
│   ├── Hidden Gems
│   ├── Most Wanted
│   ├── Curated
│   └── Insights
│
├── Resources
│   ├── Categories
│   ├── Tags
│   ├── Types
│   ├── Creators / Organizations
│   ├── Collections
│   └── Resource Detail
│
├── Exchange
│   ├── Want
│   ├── Trade
│   ├── Offer
│   └── My Listings
│
├── Community
│   ├── Discussions
│   └── Polls
│
├── Search
├── Submit
│   ├── Resource
│   ├── Suggest Edit
│   ├── Add Source
│   ├── Add Translation
│   ├── Claim Resource
│   └── Report
└── User
    ├── Profile
    ├── Saved
    ├── Want
    ├── Have
    ├── Collections
    ├── Contributions
    ├── Exchange
    ├── Notifications
    ├── Privacy
    └── Security
```

## Recommended Routes

```text
/
/discover
/resources
/resources/[slug]
/categories/[slug]
/tags/[slug]
/collections
/collections/[slug]
/search
/users/[handle]
/organizations/[slug]
/submit
/contributions/[id]
/exchange
/exchange/[id]
/discussions
/discussions/[slug]
/polls
/polls/[slug]
/insights
```

Authenticated:

```text
/me
/me/saved
/me/want
/me/have
/me/collections
/me/contributions
/me/listings
/me/notifications
/me/privacy
/me/security
```

Admin:

```text
/admin
/admin/review
/admin/content-health
/admin/resources
/admin/community
/admin/users
/admin/trust-safety
/admin/taxonomy
/admin/insights
/admin/jobs
/admin/audit
/admin/settings
```

## Homepage

Recommended:

```text
Hero Search
Featured Collection
Trending
Recently Added
Hidden Gems
Explore by Category
```

Authenticated users may later receive `For You`, but the homepage remains resource-first.

## Resource Detail

Recommended:

```text
Cover / media
Name
Summary
Type / Category / Tags
Lifecycle state
Content rating
Primary Source action
Save / Want / Have
Overview
Creator / Publisher / Maintainer
Availability
Sources
Tags
Relations
Collections
Related Resources
History / Updates
Discussion
```

Primary CTA: visit/inspect the appropriate source.

Secondary: Save / Want / Have.

Tertiary: Suggest Edit / Claim / Report.

## Search

Keep the UI visually shallow while retaining strong filtering.

```text
Search furry resources...

All   Games   Tools   Assets   Learning

Active filter chips

Filters +
```

Arbitrary search/filter URLs should generally be `noindex`.

## Collections

- Personal
- Curated
- Official

Public does not automatically mean featured.

## Exchange

One Listing model:

```text
Want
Trade
Offer
```

Listings should reference Resources where possible and expire automatically.

No checkout/payment/escrow/order state.

## Community

Discussions remain lightweight:

```text
Discussion
Reply
```

Polls collect lightweight community signals. Individual votes are private; aggregate results may be public.

## Identity

Profiles are identity cards, not social homepages.

Do not emphasize online status, profile visitors, follower counts, karma, or full activity timelines.

## SEO Indexing Boundaries

Index:

- Resources
- Categories
- canonical Tags
- valuable public Collections
- indexable public Profiles / Organizations
- official Poll results
- Insights

Conditional:

- Discussions
- user Polls
- Exchange Listings

Noindex:

- Search results
- arbitrary filters
- account pages
- private collections
- contribution review
- notifications
- admin pages
