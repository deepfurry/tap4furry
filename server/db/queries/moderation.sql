-- name: ReportPublicTarget :one
SELECT r.id,r.slug,l.name,s.id AS source_id,
 (l.resource_id IS NOT NULL AND cl.category_id IS NOT NULL AND NOT EXISTS (
 SELECT 1 FROM app.resource_tags rt JOIN app.tags t ON t.id=rt.tag_id
 LEFT JOIN app.tag_localizations tl ON tl.tag_id=t.id AND tl.locale=t.default_locale
 WHERE rt.resource_id=r.id AND t.deleted_at IS NULL AND tl.tag_id IS NULL
 ))::boolean AS canonical_ok
FROM app.resources r JOIN app.categories c ON c.id=r.category_id
LEFT JOIN app.resource_localizations l ON l.resource_id=r.id AND l.locale=r.default_locale
LEFT JOIN app.category_localizations cl ON cl.category_id=c.id AND cl.locale=c.default_locale
LEFT JOIN app.resource_sources s ON s.resource_id=r.id AND s.id=sqlc.narg(source_id)::uuid
 AND s.availability_state<>'removed' AND s.rights_status IN ('unknown','creator_provided','confirmed')
WHERE r.id=sqlc.arg(resource_id)::uuid AND r.publication_state='published' AND r.deleted_at IS NULL AND c.deleted_at IS NULL
AND (sqlc.narg(source_id)::uuid IS NULL OR s.id IS NOT NULL);

-- name: ReportInsert :exec
INSERT INTO app.reports(id,reporter_id,resource_id,source_id,target_kind,reason,body,request_id,request_fingerprint,queue,priority)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: ReportByRequest :one
SELECT id,request_fingerprint FROM app.reports WHERE reporter_id=$1 AND request_id=$2;

-- name: ReportQuota :one
SELECT count(*) FILTER(WHERE created_at>sqlc.arg(now)::timestamptz-interval '24 hours')::bigint AS recent,
 count(*) FILTER(WHERE status IN ('open','in_review'))::bigint AS pending,
 (min(created_at) FILTER(WHERE created_at>sqlc.arg(now)::timestamptz-interval '24 hours'))::timestamptz AS first_at,
 max(created_at)::timestamptz AS last_at FROM app.reports WHERE reporter_id=$1;

-- name: ReportOwned :one
SELECT id,reporter_id,resource_id,source_id,target_kind,reason,body,status,created_at,decided_at FROM app.reports WHERE id=$1 AND reporter_id=$2;

-- name: ReportLockOwned :one
SELECT id,reporter_id,resource_id,source_id,target_kind,reason,body,status,created_at,decided_at FROM app.reports WHERE id=$1 AND reporter_id=$2 FOR UPDATE;

-- name: ReportListOwned :many
SELECT id,reporter_id,resource_id,source_id,target_kind,reason,body,status,created_at,decided_at FROM app.reports WHERE reporter_id=$1 AND (sqlc.narg(status)::text IS NULL OR status=sqlc.narg(status))
ORDER BY created_at DESC,id DESC LIMIT sqlc.arg(fetch_limit) OFFSET sqlc.arg(page_offset)::bigint;

-- name: ReportWithdraw :exec
UPDATE app.reports SET status='withdrawn',version=version+1,decided_at=transaction_timestamp(),updated_at=transaction_timestamp()
WHERE id=$1 AND reporter_id=$2 AND status IN ('open','in_review');

-- name: ReportPublicEventInsert :exec
INSERT INTO app.report_events(id,report_id,event_type,actor_id,request_id,request_fingerprint,safe_message) VALUES($1,$2,$3,$4,$5,$6,$7);

-- name: ReportStaffEventInsert :exec
INSERT INTO app.report_events(id,report_id,event_type,actor_id,request_id,request_fingerprint,safe_message,internal_note,audit_id,resolution_type) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10);

-- name: ReportEventReplay :one
SELECT id,report_id,request_fingerprint FROM app.report_events WHERE actor_id=$1 AND request_id=$2;

-- name: ReportPublicEvents :many
SELECT event_type,safe_message,occurred_at FROM app.report_events WHERE report_id=$1 AND event_type<>'noted' ORDER BY occurred_at,id;

-- name: ReportStaffEvents :many
SELECT * FROM app.report_events WHERE report_id=$1 ORDER BY occurred_at,id;

-- name: ReportLock :one
SELECT * FROM app.reports WHERE id=$1 FOR UPDATE;

-- name: ReportAdmin :one
SELECT * FROM app.reports WHERE id=$1;

-- name: ReportListAdmin :many
SELECT * FROM app.reports WHERE (sqlc.narg(status)::text IS NULL OR status=sqlc.narg(status))
AND (sqlc.narg(reason)::text IS NULL OR reason=sqlc.narg(reason))
AND (sqlc.narg(queue)::text IS NULL OR queue=sqlc.narg(queue))
ORDER BY priority DESC,created_at,id LIMIT sqlc.arg(fetch_limit) OFFSET sqlc.arg(page_offset)::bigint;

-- name: ReportDecide :execrows
UPDATE app.reports SET status=sqlc.arg(status),queue=sqlc.arg(queue),duplicate_of=sqlc.narg(duplicate_of),version=version+1,
 updated_at=transaction_timestamp(),decided_at=CASE WHEN sqlc.arg(status)::text IN ('resolved','dismissed') THEN transaction_timestamp() ELSE NULL END
WHERE id=$1 AND version=sqlc.arg(expected_version) AND status IN ('open','in_review');

-- name: SourceCheckInsert :exec
INSERT INTO app.source_checks(id,resource_id,source_id,url_fingerprint,resource_version,outcome,note,actor_id,request_id,request_fingerprint,observed_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11);

-- name: SourceCheckByRequest :one
SELECT * FROM app.source_checks WHERE actor_id=$1 AND request_id=$2;

-- name: SourceCheckLatest :one
SELECT * FROM app.source_checks WHERE source_id=$1 ORDER BY observed_at DESC,id DESC LIMIT 1;

-- name: SourceHealthList :many
SELECT s.id AS source_id,r.id AS resource_id,r.version AS resource_version,r.slug AS resource_slug,s.url,s.availability_state,s.rights_status,
 EXISTS(SELECT 1 FROM app.reports p WHERE p.source_id=s.id AND p.reason='broken_link' AND p.status IN ('open','in_review'))::boolean AS open_broken_report
FROM app.resource_sources s JOIN app.resources r ON r.id=s.resource_id
WHERE r.deleted_at IS NULL AND (sqlc.narg(source_id)::uuid IS NULL OR s.id=sqlc.narg(source_id)) AND (sqlc.narg(resource_id)::uuid IS NULL OR r.id=sqlc.narg(resource_id))
AND (sqlc.narg(availability_state)::text IS NULL OR s.availability_state=sqlc.narg(availability_state))
AND (sqlc.narg(open_broken_report)::boolean IS NULL OR sqlc.narg(open_broken_report)::boolean = EXISTS(SELECT 1 FROM app.reports p WHERE p.source_id=s.id AND p.reason='broken_link' AND p.status IN ('open','in_review')))
ORDER BY (SELECT max(sc.observed_at) FROM app.source_checks sc WHERE sc.source_id=s.id) ASC NULLS FIRST,s.id
LIMIT sqlc.arg(fetch_limit) OFFSET sqlc.arg(page_offset)::bigint;

-- name: GovernanceAdminRestrictions :many
SELECT * FROM app.user_restrictions WHERE user_id=$1 ORDER BY starts_at DESC,id DESC;
