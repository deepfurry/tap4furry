-- name: GetCategoryState :one
SELECT id, slug, default_locale, state FROM app.categories WHERE id = $1 AND deleted_at IS NULL;

-- name: GetTagState :one
SELECT id, slug, default_locale, state FROM app.tags WHERE id = $1 AND deleted_at IS NULL;

-- name: LockCategoryLocalizationParent :one
SELECT id, default_locale FROM app.categories WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: LockTagLocalizationParent :one
SELECT id, default_locale FROM app.tags WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: CategoryLocalizationExists :one
SELECT EXISTS (SELECT 1 FROM app.category_localizations WHERE category_id = $1 AND locale = $2);

-- name: TagLocalizationExists :one
SELECT EXISTS (SELECT 1 FROM app.tag_localizations WHERE tag_id = $1 AND locale = $2);
