-- name: GovernanceProfile :one
SELECT COALESCE(g.trust_level,'new')::text AS trust_level, COALESCE(g.revision,0)::bigint AS revision
FROM app.users u LEFT JOIN app.user_governance_profiles g ON g.user_id=u.id
WHERE u.id=$1 AND u.account_state='active' AND u.deleted_at IS NULL;

-- name: GovernanceRestrictions :many
SELECT id,user_id,scope,reason_code,user_message,starts_at,expires_at,revoked_at
FROM app.user_restrictions WHERE user_id=$1 ORDER BY starts_at DESC,id DESC;

-- name: GovernanceEffectiveRestrictions :many
SELECT id,scope,reason_code,user_message,starts_at,expires_at FROM app.user_restrictions
WHERE user_id=$1 AND revoked_at IS NULL AND starts_at<=sqlc.arg(now)::timestamptz
AND (expires_at IS NULL OR expires_at>sqlc.arg(now)::timestamptz) ORDER BY scope,id;

-- name: GovernanceSetProfile :exec
INSERT INTO app.user_governance_profiles(user_id,trust_level,revision) VALUES($1,$2,1)
ON CONFLICT(user_id) DO UPDATE SET trust_level=excluded.trust_level,revision=app.user_governance_profiles.revision+1,updated_at=transaction_timestamp();

-- name: GovernanceInsertRestriction :exec
INSERT INTO app.user_restrictions(id,user_id,scope,reason_code,user_message,internal_note,created_by,starts_at,expires_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9);

-- name: GovernanceRevokeRestriction :execrows
UPDATE app.user_restrictions SET revoked_at=$3,revoked_by=$4,revoke_reason=$5
WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>$3);

-- name: GovernanceDistribution :one
SELECT COALESCE((SELECT policy FROM app.resource_distribution_policies WHERE resource_id=$1),'normal')::text AS policy;

-- name: GovernanceSetDistribution :exec
INSERT INTO app.resource_distribution_policies(resource_id,policy) VALUES($1,$2)
ON CONFLICT(resource_id) DO UPDATE SET policy=excluded.policy;

-- name: GovernanceInsertAction :exec
INSERT INTO app.moderation_actions(id,actor_id,action,resource_id,source_id,user_id,restriction_id,report_id,reason,internal_note,request_id,request_fingerprint)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12);

-- name: GovernanceActionByRequest :one
SELECT * FROM app.moderation_actions WHERE actor_id=$1 AND request_id=$2;

-- name: GovernanceInsertAudit :exec
INSERT INTO app.audit_entries(id,actor_id,operation,resource_id,category_id,tag_id,user_id,source_id,contribution_id,moderation_action_id,
 fields,before_version,after_version,before_publication,after_publication,before_rights,after_rights,before_distribution,after_distribution,before_trust,after_trust,reason,before_availability,after_availability,before_taxonomy_state,after_taxonomy_state)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26);

-- name: GovernanceAudit :one
SELECT * FROM app.audit_entries WHERE id=$1;

-- name: GovernanceAuditList :many
SELECT * FROM app.audit_entries
WHERE (sqlc.narg(resource_id)::uuid IS NULL OR resource_id=sqlc.narg(resource_id))
AND (sqlc.narg(user_id)::uuid IS NULL OR user_id=sqlc.narg(user_id))
AND (sqlc.narg(operation)::text IS NULL OR operation=sqlc.narg(operation))
ORDER BY occurred_at DESC,id DESC LIMIT sqlc.arg(fetch_limit) OFFSET sqlc.arg(page_offset)::bigint;

-- name: GovernanceRecommendationEligible :one
SELECT EXISTS(SELECT 1 FROM app.resources r JOIN app.categories c ON c.id=r.category_id
WHERE r.id=$1 AND r.publication_state='published' AND r.deleted_at IS NULL AND c.deleted_at IS NULL
AND r.content_rating='general' AND r.lifecycle='active' AND c.state='active'
AND NOT EXISTS(SELECT 1 FROM app.resource_distribution_policies p WHERE p.resource_id=r.id AND p.policy='excluded')
AND EXISTS(SELECT 1 FROM app.resource_sources s WHERE s.resource_id=r.id AND s.availability_state='active' AND s.rights_status IN ('creator_provided','confirmed'))
AND EXISTS(SELECT 1 FROM app.resource_localizations l WHERE l.resource_id=r.id AND l.locale=r.default_locale)
AND EXISTS(SELECT 1 FROM app.category_localizations l WHERE l.category_id=c.id AND l.locale=c.default_locale)
AND NOT EXISTS(SELECT 1 FROM app.resource_tags rt JOIN app.tags t ON t.id=rt.tag_id
 LEFT JOIN app.tag_localizations tl ON tl.tag_id=t.id AND tl.locale=t.default_locale
 WHERE rt.resource_id=r.id AND t.deleted_at IS NULL AND tl.tag_id IS NULL))::boolean AS eligible;
