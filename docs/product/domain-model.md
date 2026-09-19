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
CreatorProvided
Archive
Mirror
Community
External
Unknown
```

Source states:

```text
Active
Unavailable
Broken
Removed
Restricted
Disputed
ReviewHold
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
