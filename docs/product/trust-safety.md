# Tap4Furry — Trust, Safety, Privacy, and Moderation

## Core Concepts

### Trust
Historical reliability.

### Risk
Risk of the current action.

### Budget
How much high-impact public activity may be created.

### Distribution
How much exposure valid content receives.

## Trust States

```text
New
Established
Trusted
```

Do not publish opaque trust scores.

## Contribution Cost

Low:

```text
Save
Want
Have
Vote
```

Medium:

```text
Reply
Comment
Suggest Edit
```

High:

```text
Create Discussion
Create Listing
Create Collection
Create Poll
Submit Resource
```

## Publish ≠ Distribution

Valid content may be public but receive limited exposure.

## Anti-flooding

Do not order community surfaces purely by creation time.

Use author diversity and meaningful-activity rules.

## Edge vs Application

Cloudflare handles network/bot/credential flooding.

Go handles account age, trust, contribution budget, resource context, action velocity, and domain-level abuse.

## Restrictions

Examples:

```text
discussion_create
exchange_create
poll_create
contact_request
public_profile
all_write
```

## Moderation States

Prefer reversible states:

```text
Limit Distribution
Lock
Hide
Restrict
ReviewHold
Expire
Archive
Remove
```

## Privacy

Never public:

```text
email
OAuth identities
sessions
security records
reports
moderation records
recovery data
```

Default private:

```text
Saved
Want
Have
Following
Voting history
Adult-content preference
Search/View history
```

## Reputation

Show evidence:

```text
Verified Creator
Maintains Resources
Accepted Contributions
```

Avoid karma/XP/global numeric trust.

## Voting

```text
Public aggregate
Private individual vote
```

## Account Deletion

Do not cascade-delete public knowledge.

Revoke/delete private auth/account data while preserving knowledge integrity through de-identification/tombstones where appropriate.

## Audit vs Security Events

Audit = governance/business state changes.

Security Events = login/session/recovery/authentication events.
