-- Canonical localizations are LEFT JOINed deliberately. Missing canonical rows
-- must reach the adapter as an invariant failure, never disappear as a 404.
-- Empty optional text is forbidden by migration 6. A final empty sentinel makes
-- sqlc's COALESCE result non-null; the adapter restores an explicit JSON null.
-- name: ListPublicResources :many
SELECT r.id, r.slug, COALESCE(w.name, d.name, '')::text AS name,
       COALESCE(w.summary, d.summary, '')::text AS summary,
       c.id AS category_id, c.slug AS category_slug,
       COALESCE(cw.name, cd.name, '')::text AS category_name,
       r.lifecycle, r.content_rating, r.published_at, r.updated_at,
       COALESCE((d.resource_id IS NOT NULL AND cd.category_id IS NOT NULL), false)::boolean AS canonical_ok
FROM app.resources r
JOIN app.categories c ON c.id = r.category_id AND c.deleted_at IS NULL
LEFT JOIN app.resource_localizations d ON d.resource_id = r.id AND d.locale = r.default_locale
LEFT JOIN app.resource_localizations w ON w.resource_id = r.id AND w.locale = sqlc.arg(locale)::text
LEFT JOIN app.category_localizations cd ON cd.category_id = c.id AND cd.locale = c.default_locale
LEFT JOIN app.category_localizations cw ON cw.category_id = c.id AND cw.locale = sqlc.arg(locale)::text
WHERE r.deleted_at IS NULL AND r.publication_state = 'published'
ORDER BY r.published_at DESC, r.id DESC
LIMIT sqlc.arg(fetch_limit)::bigint OFFSET sqlc.arg(page_offset)::bigint;

-- name: GetPublicResourceBySlug :one
SELECT r.id, r.slug, r.default_locale,
       COALESCE(w.name, d.name, '')::text AS name,
       COALESCE(w.summary, d.summary, '')::text AS summary,
       COALESCE(w.description, d.description, '')::text AS description,
       c.id AS category_id, c.slug AS category_slug,
       COALESCE(cw.name, cd.name, '')::text AS category_name,
       r.lifecycle, r.content_rating, r.published_at, r.updated_at,
       COALESCE((d.resource_id IS NOT NULL AND cd.category_id IS NOT NULL), false)::boolean AS canonical_ok
FROM app.resources r
JOIN app.categories c ON c.id = r.category_id AND c.deleted_at IS NULL
LEFT JOIN app.resource_localizations d ON d.resource_id = r.id AND d.locale = r.default_locale
LEFT JOIN app.resource_localizations w ON w.resource_id = r.id AND w.locale = sqlc.arg(locale)::text
LEFT JOIN app.category_localizations cd ON cd.category_id = c.id AND cd.locale = c.default_locale
LEFT JOIN app.category_localizations cw ON cw.category_id = c.id AND cw.locale = sqlc.arg(locale)::text
WHERE r.slug = sqlc.arg(slug) AND r.deleted_at IS NULL AND r.publication_state = 'published';

-- Child reads run in the same read-only snapshot as the public parent lookup.
-- name: ListPublicResourceLocales :many
SELECT locale FROM app.resource_localizations WHERE resource_id = $1 ORDER BY locale;

-- name: ListPublicResourceTags :many
SELECT t.id, t.slug, COALESCE(w.name, d.name, '')::text AS name,
       COALESCE((d.tag_id IS NOT NULL), false)::boolean AS canonical_ok
FROM app.resource_tags rt
JOIN app.tags t ON t.id = rt.tag_id AND t.deleted_at IS NULL
LEFT JOIN app.tag_localizations d ON d.tag_id = t.id AND d.locale = t.default_locale
LEFT JOIN app.tag_localizations w ON w.tag_id = t.id AND w.locale = sqlc.arg(locale)::text
WHERE rt.resource_id = sqlc.arg(resource_id)
ORDER BY t.slug;

-- name: ListPublicResourceSources :many
SELECT id, url, label, source_type, availability_state, is_primary
FROM app.resource_sources
WHERE resource_id = $1
  AND availability_state IN ('active', 'unavailable', 'broken', 'restricted')
  AND rights_status IN ('unknown', 'creator_provided', 'confirmed')
ORDER BY is_primary DESC, source_type, url, id;

-- name: ListPublicResourceRelations :many
WITH edges AS (
    SELECT outgoing.target_resource_id AS other_id, outgoing.relation_type,
           CASE WHEN outgoing.relation_type = 'related_to' THEN 'symmetric' ELSE 'outgoing' END::text AS direction
    FROM app.resource_relations outgoing WHERE outgoing.source_resource_id = sqlc.arg(resource_id)
    UNION ALL
    SELECT incoming.source_resource_id AS other_id, incoming.relation_type,
           CASE WHEN incoming.relation_type = 'related_to' THEN 'symmetric' ELSE 'incoming' END::text AS direction
    FROM app.resource_relations incoming WHERE incoming.target_resource_id = sqlc.arg(resource_id)
)
SELECT e.relation_type, e.direction, r.id, r.slug,
       COALESCE(w.name, d.name, '')::text AS name,
       COALESCE((d.resource_id IS NOT NULL AND cd.category_id IS NOT NULL), false)::boolean AS canonical_ok
FROM edges e
JOIN app.resources r ON r.id = e.other_id AND r.deleted_at IS NULL AND r.publication_state = 'published'
JOIN app.categories c ON c.id = r.category_id AND c.deleted_at IS NULL
LEFT JOIN app.resource_localizations d ON d.resource_id = r.id AND d.locale = r.default_locale
LEFT JOIN app.resource_localizations w ON w.resource_id = r.id AND w.locale = sqlc.arg(locale)::text
LEFT JOIN app.category_localizations cd ON cd.category_id = c.id AND cd.locale = c.default_locale
ORDER BY e.relation_type, e.direction, r.slug;

-- name: ListPublicResourceExternalIDs :many
SELECT namespace, external_id FROM app.resource_external_ids
WHERE resource_id = $1 ORDER BY namespace, external_id;

-- name: ListPublicCategories :many
SELECT c.id, c.slug, COALESCE(w.name, d.name, '')::text AS name,
       COALESCE(w.description, d.description, '')::text AS description,
       COALESCE((d.category_id IS NOT NULL), false)::boolean AS canonical_ok
FROM app.categories c
LEFT JOIN app.category_localizations d ON d.category_id = c.id AND d.locale = c.default_locale
LEFT JOIN app.category_localizations w ON w.category_id = c.id AND w.locale = sqlc.arg(locale)::text
WHERE c.state = 'active' AND c.deleted_at IS NULL ORDER BY c.slug;

-- name: ListPublicTags :many
SELECT t.id, t.slug, COALESCE(w.name, d.name, '')::text AS name,
       COALESCE(w.description, d.description, '')::text AS description,
       COALESCE((d.tag_id IS NOT NULL), false)::boolean AS canonical_ok
FROM app.tags t
LEFT JOIN app.tag_localizations d ON d.tag_id = t.id AND d.locale = t.default_locale
LEFT JOIN app.tag_localizations w ON w.tag_id = t.id AND w.locale = sqlc.arg(locale)::text
WHERE t.state = 'active' AND t.deleted_at IS NULL ORDER BY t.slug;
