-- name: ListAdminResources :many
SELECT r.*, l.name, c.slug AS category_slug, c.default_locale AS category_locale,
       c.state AS category_state, cl.name AS category_name
FROM app.resources r
LEFT JOIN app.resource_localizations l ON l.resource_id=r.id AND l.locale=r.default_locale
JOIN app.categories c ON c.id=r.category_id
LEFT JOIN app.category_localizations cl ON cl.category_id=c.id AND cl.locale=c.default_locale
WHERE r.deleted_at IS NULL
 AND (sqlc.narg(publication_state)::text IS NULL OR r.publication_state=sqlc.narg(publication_state))
 AND (sqlc.narg(category_id)::uuid IS NULL OR r.category_id=sqlc.narg(category_id))
 AND (sqlc.narg(slug)::text IS NULL OR r.slug=sqlc.narg(slug))
ORDER BY r.updated_at DESC, r.id DESC LIMIT sqlc.arg(row_limit)::bigint OFFSET sqlc.arg(row_offset)::bigint;

-- name: GetAdminResource :one
SELECT r.*, c.slug AS category_slug, c.default_locale AS category_locale,
       c.state AS category_state, cl.name AS category_name
FROM app.resources r JOIN app.categories c ON c.id=r.category_id
LEFT JOIN app.category_localizations cl ON cl.category_id=c.id AND cl.locale=c.default_locale
WHERE r.id=$1 AND r.deleted_at IS NULL;

-- name: AdminResourceLocalizations :many
SELECT * FROM app.resource_localizations WHERE resource_id=$1 ORDER BY locale;

-- name: AdminResourceTags :many
SELECT t.*, l.name FROM app.tags t JOIN app.resource_tags rt ON rt.tag_id=t.id
LEFT JOIN app.tag_localizations l ON l.tag_id=t.id AND l.locale=t.default_locale
WHERE rt.resource_id=$1 ORDER BY t.slug,t.id;

-- name: AdminResourceSources :many
SELECT * FROM app.resource_sources WHERE resource_id=$1 ORDER BY created_at,id;

-- name: AdminResourceRelations :many
SELECT rr.*, r.id AS other_id,r.slug AS other_slug,l.name AS other_name,r.publication_state AS other_publication_state,
 CASE WHEN rr.relation_type='related_to' THEN 'symmetric' WHEN rr.source_resource_id=sqlc.arg(anchor_id) THEN 'outgoing' ELSE 'incoming' END::text AS direction
FROM app.resource_relations rr
JOIN app.resources r ON r.id=CASE WHEN rr.source_resource_id=sqlc.arg(anchor_id) THEN rr.target_resource_id ELSE rr.source_resource_id END
LEFT JOIN app.resource_localizations l ON l.resource_id=r.id AND l.locale=r.default_locale
WHERE (rr.source_resource_id=sqlc.arg(anchor_id) OR rr.target_resource_id=sqlc.arg(anchor_id)) AND r.deleted_at IS NULL
ORDER BY rr.relation_type,rr.id;

-- name: AdminResourceExternalIDs :many
SELECT * FROM app.resource_external_ids WHERE resource_id=$1 ORDER BY namespace,external_id;

-- name: ListAdminCategories :many
SELECT t.*,l.name FROM app.categories t LEFT JOIN app.category_localizations l ON l.category_id=t.id AND l.locale=t.default_locale
WHERE t.deleted_at IS NULL ORDER BY t.slug,t.id;

-- name: GetAdminCategory :one
SELECT * FROM app.categories WHERE id=$1 AND deleted_at IS NULL;

-- name: AdminCategoryLocalizations :many
SELECT * FROM app.category_localizations WHERE category_id=$1 ORDER BY locale;

-- name: ListAdminTags :many
SELECT t.*,l.name FROM app.tags t LEFT JOIN app.tag_localizations l ON l.tag_id=t.id AND l.locale=t.default_locale
WHERE t.deleted_at IS NULL ORDER BY t.slug,t.id;

-- name: GetAdminTag :one
SELECT * FROM app.tags WHERE id=$1 AND deleted_at IS NULL;

-- name: AdminTagLocalizations :many
SELECT * FROM app.tag_localizations WHERE tag_id=$1 ORDER BY locale;
