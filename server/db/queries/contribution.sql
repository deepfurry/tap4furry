-- name: ContributionVerified :one
SELECT EXISTS(SELECT 1 FROM app.auth_identities WHERE user_id=$1 AND provider='email' AND verified_at IS NOT NULL);

-- name: ContributionResource :one
SELECT r.id,r.slug,r.default_locale,r.category_id,r.lifecycle,r.content_rating,r.version,
    l.name,l.summary,l.description,(l.resource_id IS NOT NULL AND cl.category_id IS NOT NULL)::boolean AS canonical_ok
FROM app.resources r JOIN app.categories c ON c.id=r.category_id
LEFT JOIN app.resource_localizations l ON l.resource_id=r.id AND l.locale=r.default_locale
LEFT JOIN app.category_localizations cl ON cl.category_id=c.id AND cl.locale=c.default_locale
WHERE r.publication_state='published' AND r.deleted_at IS NULL AND c.deleted_at IS NULL
AND ((sqlc.narg(id)::uuid IS NOT NULL AND r.id=sqlc.narg(id)) OR (sqlc.narg(slug)::text IS NOT NULL AND r.slug=sqlc.narg(slug)));

-- name: ContributionCategory :one
SELECT state FROM app.categories WHERE id=$1 AND deleted_at IS NULL;

-- name: ContributionQuota :one
SELECT count(*) FILTER (WHERE created_at > sqlc.arg(now)::timestamptz - interval '24 hours')::bigint AS recent,
    count(*) FILTER (WHERE status='pending')::bigint AS pending,
    coalesce(max(created_at),'epoch'::timestamptz)::timestamptz AS last_at,
    coalesce(min(created_at) FILTER (WHERE created_at > sqlc.arg(now)::timestamptz - interval '24 hours'),sqlc.arg(now)::timestamptz)::timestamptz AS first_at
FROM app.contributions WHERE author_id=sqlc.arg(author_id);

-- name: ContributionByRequest :one
SELECT * FROM app.contributions WHERE author_id=$1 AND request_id=$2;

-- name: ContributionOwned :one
SELECT * FROM app.contributions WHERE id=$1 AND author_id=$2;

-- name: ContributionLockOwned :one
SELECT * FROM app.contributions WHERE id=$1 AND author_id=$2 FOR UPDATE;

-- name: ContributionLock :one
SELECT * FROM app.contributions WHERE id=$1 FOR UPDATE;

-- name: ContributionGet :one
SELECT * FROM app.contributions WHERE id=$1;

-- name: ContributionListOwned :many
SELECT c.*,CASE WHEN c.kind='create_resource' OR (c.submitted_fields & 1)<>0 THEN p.name ELSE 'Resource correction' END::text AS name FROM app.contributions c JOIN app.contribution_contents p ON p.contribution_id=c.id AND p.content_kind='proposed'
WHERE c.author_id=sqlc.arg(author_id) AND (sqlc.narg(status)::text IS NULL OR c.status=sqlc.narg(status))
ORDER BY c.created_at DESC,c.id DESC LIMIT sqlc.arg(fetch_limit)::int OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContributionListAdmin :many
SELECT c.*,p.name FROM app.contributions c JOIN app.contribution_contents p ON p.contribution_id=c.id AND p.content_kind='proposed'
WHERE (sqlc.narg(status)::text IS NULL OR c.status=sqlc.narg(status)) AND (sqlc.narg(kind)::text IS NULL OR c.kind=sqlc.narg(kind))
ORDER BY c.created_at,c.id LIMIT sqlc.arg(fetch_limit)::int OFFSET sqlc.arg(page_offset)::bigint;

-- name: ContributionCreate :one
INSERT INTO app.contributions(id,author_id,kind,target_resource_id,base_version,reason,previous_id,request_id,request_fingerprint,submitted_fields)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING *;

-- name: ContributionPutContent :exec
INSERT INTO app.contribution_contents(contribution_id,content_kind,default_locale,category_id,name,summary,description,lifecycle,content_rating,slug)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: ContributionPutSource :exec
INSERT INTO app.contribution_initial_sources(contribution_id,content_kind,url,label,source_type,availability_state)
VALUES($1,$2,$3,$4,$5,$6);

-- name: ContributionContents :many
SELECT c.*,s.url,s.label,s.source_type,s.availability_state
FROM app.contribution_contents c LEFT JOIN app.contribution_initial_sources s USING(contribution_id,content_kind)
WHERE c.contribution_id=$1 ORDER BY c.content_kind;

-- name: ContributionWithdraw :execrows
UPDATE app.contributions SET status='withdrawn',decided_at=clock_timestamp()
WHERE id=$1 AND author_id=$2 AND status='pending';

-- name: ContributionDecide :one
UPDATE app.contributions SET status=$2,result_resource_id=$3,result_version=$4,decided_at=clock_timestamp()
WHERE id=$1 AND status='pending' RETURNING *;

-- name: ContributionPublicEvent :exec
INSERT INTO app.contribution_events(contribution_id,event_type,actor_id,message,occurred_at)
SELECT id,$2,$3,$4,coalesce(decided_at,created_at) FROM app.contributions WHERE id=$1;

-- name: ContributionReviewEvent :exec
INSERT INTO app.contribution_events(contribution_id,event_type,actor_id,message,internal_note,occurred_at)
SELECT id,$2,$3,$4,$5,decided_at FROM app.contributions WHERE id=$1;

-- name: ContributionPublicHistory :many
SELECT event_type,message,occurred_at FROM app.contribution_events WHERE contribution_id=$1 ORDER BY occurred_at,event_type;

-- name: ContributionAdminHistory :many
SELECT * FROM app.contribution_events WHERE contribution_id=$1 ORDER BY occurred_at,event_type;

-- name: ContributionAudit :exec
INSERT INTO app.contribution_review_audits(contribution_id,actor_id,action,resource_id,before_version,after_version,occurred_at)
SELECT id,$2,status,$3,$4,$5,decided_at FROM app.contributions WHERE id=$1;
