# Tap4Furry — Product Domain Model

## Core Domains

```text
Resource
Discovery
Identity
Collection
Exchange
Discussion
Poll
Trust & Safety
```

Supporting:

```text
Contribution
Moderation
Notification
Analytics
Admin
```

## Core Entities

P0-2A implements Resource/Taxonomy data foundations; P0-2B adds anonymous public
reads and SSR Resource pages. P0-2C adds Admin curation, and P0-3A implements
new-Resource/default-language correction proposals and atomic review. The remaining
P0 contribution types are in the Stage 1.2 design; Search and Exchange remain future
work. The broader entity map below is product direction, not an implementation inventory.

```text
User
Organization
Resource
ResourceSource
ResourceRelation
Category
Tag
Collection
CollectionItem
Listing
Discussion
Reply
Poll
PollOption
PollVote
Contribution
Report
Notification
```

Cross-cutting:

```text
ResourceActorRelation
TrustProfile
Restriction
ModerationAction
AuditLog
SecurityEvent
```

## Resource

A Resource is a long-lived knowledge entity, not a download link.

It belongs to exactly one Category and has flat many-to-many Tags. Category/Tag
states are `active` and `retired`; retired taxonomy remains historically valid but
cannot receive new bindings. Category/Tag/Resource use independent soft deletion.
Category/Tag slugs are immutable ASCII kebab-case (1–64); Resource slugs (1–80)
become immutable through normal application behavior after first publication.

The intended initial categories are `game`, `creative-work`, `tool`, `platform`,
`community`, `event`, `knowledge`, `marketplace`, `service` and `other`. Migration 6
seeds none; P0-2C/operator curation creates them without magic UUIDs. Tags have no
hierarchy, aliases, implications, weights or primary-tag concept.

Each entity has a canonical `default_locale` and a separate localization table.
Creation atomically includes its default translation. Switching requires an existing
target; the current default cannot be deleted. Resource translations contain required
name and optional summary/description; blank optional fields become NULL. Future
reads fall back per field from requested locale to default locale, not per whole row.

`Resource.version` starts explicitly at 1 and represents a logical revision of the
node plus Resource-owned localizations, tags, sources, relations and external IDs.
Each transaction bumps each affected Resource exactly once; relation edits affect
both endpoints. Category/Tag entity edits do not cascade revision changes.

Independent state dimensions:

### Publication State

```text
Draft
Pending
Published
Restricted
Removed
```

### Lifecycle

```text
Active
Inactive
Discontinued
Delisted
Archived
Unknown
```

### Content Rating

```text
General
Mature
Explicit
```

## ResourceSource

A Source is an external place where a Resource can be learned about or accessed.

Source types:

```text
Official
Store
Archive
Mirror
Community
External
Unknown
```

Availability states (independent of Source type and rights status):

```text
Active
Unavailable
Broken
Removed
Restricted
```

Authorization-related platform states should be factual:

```text
Confirmed
CreatorProvided
Unknown
Disputed
RightsReview
RemovedByRequest
```

P0-2A stores lowercase state values. `creator_provided` belongs to rights status,
not Source type; `disputed`/`rights_review` also describe rights rather than availability.
A primary Source may be unavailable; a mirror may have unknown rights. Removing or
restricting a Source does not remove the Resource itself. Source URLs are normalized
conservatively without fetching, with zero or one primary Source per Resource.

## Resource Relations and External IDs

Public reads hide all unpublished/deleted endpoints and Sources under rights review,
dispute or removal. Published explicit/discontinued Resources remain readable with
clear content rating/lifecycle labels. Related historical retired taxonomy remains
visible; deleted taxonomy is hidden. Public pages expose knowledge, not internal
publication state, version, deletion timestamps or Source rights status. Markdown
descriptions render without raw HTML or images; Resource images remain Media work.

Relations store only `part_of`, `successor_of`, `derived_from` and `related_to`.
Directed edges retain direction and reads derive inverse semantics. `related_to`
stores smaller UUID→larger UUID; self-edges are forbidden. P0-2C owns directed
cycle prevention per relation type under one graph lock. No actor ownership,
duplicate merge or source-as-relation model.

External IDs use a lowercase stable namespace and an opaque trimmed identifier;
`(namespace, external_id)` is globally unique. Several distinct IDs may share a
namespace on one Resource. No provider snapshots or synchronization state is stored.

Canonical editing uses Editorial (`editor`/`admin`); `moderator` can inspect the graph.
Retiring taxonomy, restricted/removed Resource states, soft deletion and rights
status changes require Administration. P0-2C uses the existing static roles and grants.
All Resources start Draft; first publication permanently freezes slug and preserves
published_at through later state changes. Restricted/Removed recovery remains
Admin-only. Sources use availability removal, while Resources/Category/Tag soft-delete
without a restore surface. Taxonomy cannot be deleted while used by live Resources.

## Identity

```text
Auth Identity
    ↓
User Account
    ↓
Public Profile
```

Verification proves a relationship:

```text
User / Organization ↔ Resource
```

not a government identity.

## Resource Interactions

```text
Save
Want
Have
```

Default user visibility: private. Aggregate counts may be public.

## Listing

Unified Exchange model:

```text
Want
Trade
Offer
```

Listings are not order/transaction records.

## Discussion

```text
Discussion
Reply
```

Prefer flat presentation with optional reply references.

## Poll

P0/P1:

```text
one eligible user
one equal-weight vote
```

Closed Polls are immutable; invalid historical polls become `Void`.

## Contribution

Represents a proposed public-knowledge change:

```text
Create Resource
Update Resource
Add Source
Remove Broken Source
Add Tag
Add Relation
Add Translation
Claim
```

## Trust

Internal states initially:

```text
New
Established
Trusted
```

Public reputation shows evidence, not a numeric trust score.

## Audit

Audit is for high-impact governance state changes and is distinct from Contribution history and Security Events.
