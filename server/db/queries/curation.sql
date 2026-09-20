-- name: CurationBumpRevision :one
UPDATE app.resources SET version=version+1,updated_at=transaction_timestamp()
WHERE id=$1 AND version=$2 RETURNING version;

-- name: CurationCreateResource :exec
INSERT INTO app.resources(id,slug,default_locale,category_id,publication_state,lifecycle,content_rating,version,created_at,updated_at)
VALUES($1,$2,$3,$4,'draft',$5,$6,1,transaction_timestamp(),transaction_timestamp());

-- name: CurationUpdateCore :exec
UPDATE app.resources SET slug=$2,default_locale=$3,category_id=$4,lifecycle=$5,content_rating=$6 WHERE id=$1;

-- name: CurationPublish :exec
UPDATE app.resources SET publication_state=sqlc.arg(state),
 published_at=CASE WHEN sqlc.arg(state)::text='published' AND published_at IS NULL THEN transaction_timestamp() ELSE published_at END
WHERE id=sqlc.arg(id);

-- name: CurationSoftDeleteResource :exec
UPDATE app.resources SET deleted_at=transaction_timestamp() WHERE id=$1;

-- name: CurationPutLocalization :execrows
INSERT INTO app.resource_localizations(resource_id,locale,name,summary,description,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,transaction_timestamp(),transaction_timestamp())
ON CONFLICT(resource_id,locale) DO UPDATE SET name=EXCLUDED.name,summary=EXCLUDED.summary,description=EXCLUDED.description,updated_at=transaction_timestamp()
WHERE (resource_localizations.name,resource_localizations.summary,resource_localizations.description) IS DISTINCT FROM (EXCLUDED.name,EXCLUDED.summary,EXCLUDED.description);

-- name: CurationDeleteLocalization :execrows
DELETE FROM app.resource_localizations WHERE resource_id=$1 AND locale=$2;

-- name: CurationCurrentTags :many
SELECT tag_id FROM app.resource_tags WHERE resource_id=$1 ORDER BY tag_id;

-- name: CurationAddTag :exec
INSERT INTO app.resource_tags(resource_id,tag_id) VALUES($1,$2);

-- name: CurationRemoveTag :exec
DELETE FROM app.resource_tags WHERE resource_id=$1 AND tag_id=$2;

-- name: CurationAddExternalID :exec
INSERT INTO app.resource_external_ids(resource_id,namespace,external_id,created_at) VALUES($1,$2,$3,transaction_timestamp());

-- name: CurationRemoveExternalID :exec
DELETE FROM app.resource_external_ids WHERE resource_id=$1 AND namespace=$2 AND external_id=$3;

-- name: CurationGetSource :one
SELECT * FROM app.resource_sources WHERE id=$1 AND resource_id=$2;

-- name: CurationCreateSource :exec
INSERT INTO app.resource_sources(id,resource_id,url,label,source_type,availability_state,rights_status,is_primary,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,'unknown',$7,transaction_timestamp(),transaction_timestamp());

-- name: CurationClearPrimary :exec
UPDATE app.resource_sources SET is_primary=false,updated_at=transaction_timestamp() WHERE resource_id=$1 AND is_primary AND id<>$2;

-- name: CurationUpdateSource :execrows
UPDATE app.resource_sources SET url=$2,label=$3,source_type=$4,availability_state=$5,is_primary=$6,updated_at=transaction_timestamp()
WHERE id=$1 AND (url,label,source_type,availability_state,is_primary) IS DISTINCT FROM ($2,$3,$4,$5,$6);

-- name: CurationSourceRights :execrows
UPDATE app.resource_sources SET rights_status=$2,updated_at=transaction_timestamp() WHERE id=$1 AND rights_status<>$2;

-- name: CurationLockRelationGraph :exec
SELECT pg_advisory_xact_lock(72463159018202);

-- name: CurationGetRelation :one
SELECT * FROM app.resource_relations WHERE id=$1;

-- name: CurationRelationCycle :one
WITH RECURSIVE reachable(id) AS (
 SELECT sqlc.arg(target_id)::uuid
 UNION
 SELECT rr.target_resource_id FROM app.resource_relations rr JOIN reachable r ON rr.source_resource_id=r.id
 WHERE rr.relation_type=sqlc.arg(relation_type)
)
SELECT EXISTS(SELECT 1 FROM reachable WHERE id=sqlc.arg(source_id)::uuid);

-- name: CurationAddRelation :execrows
INSERT INTO app.resource_relations(id,source_resource_id,target_resource_id,relation_type,created_at)
VALUES($1,$2,$3,$4,transaction_timestamp()) ON CONFLICT(source_resource_id,target_resource_id,relation_type) DO NOTHING;

-- name: CurationDeleteRelation :execrows
DELETE FROM app.resource_relations WHERE id=$1;

-- name: CurationLockCategory :one
SELECT * FROM app.categories WHERE id=$1 AND deleted_at IS NULL FOR UPDATE;

-- name: CurationCreateCategory :exec
INSERT INTO app.categories(id,slug,default_locale,state,created_at,updated_at)
VALUES($1,$2,$3,'active',transaction_timestamp(),transaction_timestamp());

-- name: CurationUpdateCategory :exec
UPDATE app.categories SET default_locale=$2,state=$3,updated_at=transaction_timestamp() WHERE id=$1;

-- name: CurationTouchCategory :exec
UPDATE app.categories SET updated_at=transaction_timestamp() WHERE id=$1;

-- name: CurationPutCategoryLocalization :execrows
INSERT INTO app.category_localizations(category_id,locale,name,description,created_at,updated_at)
VALUES($1,$2,$3,$4,transaction_timestamp(),transaction_timestamp())
ON CONFLICT(category_id,locale) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,updated_at=transaction_timestamp()
WHERE (category_localizations.name,category_localizations.description) IS DISTINCT FROM (EXCLUDED.name,EXCLUDED.description);

-- name: CurationDeleteCategoryLocalization :execrows
DELETE FROM app.category_localizations WHERE category_id=$1 AND locale=$2;

-- name: CurationCategoryInUse :one
SELECT EXISTS(SELECT 1 FROM app.resources r
 WHERE r.category_id=$1 AND r.deleted_at IS NULL);

-- name: CurationSoftDeleteCategory :exec
UPDATE app.categories SET deleted_at=transaction_timestamp(),updated_at=transaction_timestamp() WHERE id=$1;

-- name: CurationLockTag :one
SELECT * FROM app.tags WHERE id=$1 AND deleted_at IS NULL FOR UPDATE;

-- name: CurationCreateTag :exec
INSERT INTO app.tags(id,slug,default_locale,state,created_at,updated_at)
VALUES($1,$2,$3,'active',transaction_timestamp(),transaction_timestamp());

-- name: CurationUpdateTag :exec
UPDATE app.tags SET default_locale=$2,state=$3,updated_at=transaction_timestamp() WHERE id=$1;

-- name: CurationTouchTag :exec
UPDATE app.tags SET updated_at=transaction_timestamp() WHERE id=$1;

-- name: CurationPutTagLocalization :execrows
INSERT INTO app.tag_localizations(tag_id,locale,name,description,created_at,updated_at)
VALUES($1,$2,$3,$4,transaction_timestamp(),transaction_timestamp())
ON CONFLICT(tag_id,locale) DO UPDATE SET name=EXCLUDED.name,description=EXCLUDED.description,updated_at=transaction_timestamp()
WHERE (tag_localizations.name,tag_localizations.description) IS DISTINCT FROM (EXCLUDED.name,EXCLUDED.description);

-- name: CurationDeleteTagLocalization :execrows
DELETE FROM app.tag_localizations WHERE tag_id=$1 AND locale=$2;

-- name: CurationTagInUse :one
SELECT EXISTS(SELECT 1 FROM app.resources r JOIN app.resource_tags rt ON rt.resource_id=r.id
 WHERE rt.tag_id=$1 AND r.deleted_at IS NULL);

-- name: CurationSoftDeleteTag :exec
UPDATE app.tags SET deleted_at=transaction_timestamp(),updated_at=transaction_timestamp() WHERE id=$1;
