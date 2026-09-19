-- name: GetResourceRevision :one
SELECT id, version, published_at FROM app.resources WHERE id = $1 AND deleted_at IS NULL;

-- name: LockResourceCore :one
SELECT id, slug, category_id, default_locale, publication_state, lifecycle,
       content_rating, version, published_at, created_at, updated_at
FROM app.resources WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: CompareAndBumpResourceVersion :one
UPDATE app.resources SET version = version + 1, updated_at = sqlc.arg(now)
WHERE id = sqlc.arg(id) AND version = sqlc.arg(expected_version) AND deleted_at IS NULL
RETURNING version;

-- name: ResourceLocalizationExists :one
SELECT EXISTS (SELECT 1 FROM app.resource_localizations WHERE resource_id = $1 AND locale = $2);

-- name: FindResourceRelation :one
SELECT id FROM app.resource_relations
WHERE source_resource_id = $1 AND target_resource_id = $2 AND relation_type = $3;
