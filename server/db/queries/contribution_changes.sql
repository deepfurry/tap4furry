-- name: ContributionSourceChanges :many
SELECT * FROM app.contribution_source_changes WHERE contribution_id=$1 ORDER BY snapshot_kind;
-- name: ContributionTagChanges :many
SELECT * FROM app.contribution_tag_changes WHERE contribution_id=$1 ORDER BY snapshot_kind,tag_id;
-- name: ContributionRelationChanges :many
SELECT * FROM app.contribution_relation_changes WHERE contribution_id=$1 ORDER BY snapshot_kind;
-- name: ContributionLocalizationChanges :many
SELECT * FROM app.contribution_localization_changes WHERE contribution_id=$1 ORDER BY snapshot_kind;
-- name: ContributionPutSourceChange :exec
INSERT INTO app.contribution_source_changes(contribution_id,snapshot_kind,source_id,url,label,source_type,availability_state) VALUES($1,$2,$3,$4,$5,$6,$7);
-- name: ContributionPutTagChange :exec
INSERT INTO app.contribution_tag_changes(contribution_id,snapshot_kind,tag_id,was_bound) VALUES($1,$2,$3,$4);
-- name: ContributionPutRelationChange :exec
INSERT INTO app.contribution_relation_changes(contribution_id,snapshot_kind,other_resource_id,relation_type,direction,other_base_version) VALUES($1,$2,$3,$4,$5,$6);
-- name: ContributionPutAcceptedRelation :exec
INSERT INTO app.contribution_relation_changes(contribution_id,snapshot_kind,other_resource_id,relation_type,direction,other_base_version,anchor_result_version,other_result_version)
VALUES($1,'accepted',$2,$3,$4,$5,$6,$7);
-- name: ContributionPutLocalizationChange :exec
INSERT INTO app.contribution_localization_changes(contribution_id,snapshot_kind,locale,row_exists,name,summary,description,supplied_fields) VALUES($1,$2,$3,$4,$5,$6,$7,$8);
-- name: ContributionAuditResource :exec
INSERT INTO app.contribution_review_resource_changes(contribution_id,resource_id,before_version,after_version) VALUES($1,$2,$3,$4);
-- name: ContributionResourceAudits :many
SELECT * FROM app.contribution_review_resource_changes WHERE contribution_id=$1 ORDER BY resource_id;
-- name: ContributionPublicSource :one
SELECT id,url,label,source_type,availability_state FROM app.resource_sources
WHERE id=$1 AND resource_id=$2 AND availability_state<>'removed' AND rights_status IN ('unknown','creator_provided','confirmed');
-- name: ContributionSourceURLExists :one
SELECT EXISTS(SELECT 1 FROM app.resource_sources WHERE resource_id=$1 AND url=$2);
-- name: ContributionTagState :one
SELECT t.state,l.name,(l.tag_id IS NOT NULL)::boolean AS canonical_ok,
 EXISTS(SELECT 1 FROM app.resource_tags rt WHERE rt.resource_id=$2 AND rt.tag_id=t.id)::boolean AS bound
FROM app.tags t LEFT JOIN app.tag_localizations l ON l.tag_id=t.id AND l.locale=t.default_locale WHERE t.id=$1 AND t.deleted_at IS NULL;
-- name: ContributionTranslation :one
SELECT * FROM app.resource_localizations WHERE resource_id=$1 AND locale=$2;
-- name: ContributionRelationExists :one
SELECT EXISTS(SELECT 1 FROM app.resource_relations WHERE source_resource_id=$1 AND target_resource_id=$2 AND relation_type=$3);
