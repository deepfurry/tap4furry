-- +goose Up
CREATE TABLE app.categories (
    id uuid PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (char_length(slug) BETWEEN 1 AND 64 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    default_locale text NOT NULL CHECK (char_length(default_locale) BETWEEN 2 AND 64 AND default_locale ~ '^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$'),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'retired')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    deleted_at timestamptz CHECK (deleted_at IS NULL OR deleted_at >= created_at)
);

CREATE TABLE app.category_localizations (
    category_id uuid NOT NULL REFERENCES app.categories(id) ON DELETE RESTRICT,
    locale text NOT NULL CHECK (char_length(locale) BETWEEN 2 AND 64 AND locale ~ '^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80 AND name !~ '^[[:space:]]|[[:space:]]$'),
    description text CHECK (description IS NULL OR (description ~ '[^[:space:]]' AND char_length(description) <= 500)),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    PRIMARY KEY (category_id, locale)
);
CREATE UNIQUE INDEX category_localizations_locale_ci_idx ON app.category_localizations (category_id, lower(locale));

CREATE TABLE app.tags (
    id uuid PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (char_length(slug) BETWEEN 1 AND 64 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    default_locale text NOT NULL CHECK (char_length(default_locale) BETWEEN 2 AND 64 AND default_locale ~ '^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$'),
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'retired')),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    deleted_at timestamptz CHECK (deleted_at IS NULL OR deleted_at >= created_at)
);

CREATE TABLE app.tag_localizations (
    tag_id uuid NOT NULL REFERENCES app.tags(id) ON DELETE RESTRICT,
    locale text NOT NULL CHECK (char_length(locale) BETWEEN 2 AND 64 AND locale ~ '^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80 AND name !~ '^[[:space:]]|[[:space:]]$'),
    description text CHECK (description IS NULL OR (description ~ '[^[:space:]]' AND char_length(description) <= 500)),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    PRIMARY KEY (tag_id, locale)
);
CREATE UNIQUE INDEX tag_localizations_locale_ci_idx ON app.tag_localizations (tag_id, lower(locale));

CREATE TABLE app.resources (
    id uuid PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (char_length(slug) BETWEEN 1 AND 80 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    default_locale text NOT NULL CHECK (char_length(default_locale) BETWEEN 2 AND 64 AND default_locale ~ '^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$'),
    category_id uuid NOT NULL REFERENCES app.categories(id) ON DELETE RESTRICT,
    publication_state text NOT NULL DEFAULT 'draft' CHECK (publication_state IN ('draft', 'pending', 'published', 'restricted', 'removed')),
    lifecycle text NOT NULL DEFAULT 'unknown' CHECK (lifecycle IN ('active', 'inactive', 'discontinued', 'delisted', 'archived', 'unknown')),
    content_rating text NOT NULL CHECK (content_rating IN ('general', 'mature', 'explicit')),
    version bigint NOT NULL CHECK (version >= 1),
    published_at timestamptz CHECK (published_at IS NULL OR published_at >= created_at),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    deleted_at timestamptz CHECK (deleted_at IS NULL OR deleted_at >= created_at),
    CHECK (publication_state <> 'published' OR published_at IS NOT NULL)
);
CREATE INDEX resources_category_id_idx ON app.resources (category_id);

CREATE TABLE app.resource_localizations (
    resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
    locale text NOT NULL CHECK (char_length(locale) BETWEEN 2 AND 64 AND locale ~ '^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$'),
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160 AND name !~ '^[[:space:]]|[[:space:]]$'),
    summary text CHECK (summary IS NULL OR (char_length(summary) BETWEEN 1 AND 500 AND summary !~ '^[[:space:]]|[[:space:]]$')),
    description text CHECK (description IS NULL OR description ~ '[^[:space:]]'),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    PRIMARY KEY (resource_id, locale)
);
CREATE UNIQUE INDEX resource_localizations_locale_ci_idx ON app.resource_localizations (resource_id, lower(locale));

CREATE TABLE app.resource_tags (
    resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
    tag_id uuid NOT NULL REFERENCES app.tags(id) ON DELETE RESTRICT,
    PRIMARY KEY (resource_id, tag_id)
);
CREATE INDEX resource_tags_tag_id_idx ON app.resource_tags (tag_id);

CREATE TABLE app.resource_sources (
    id uuid PRIMARY KEY,
    resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
    url text NOT NULL CHECK (url = btrim(url) AND char_length(url) BETWEEN 1 AND 2048 AND url ~ '^https?://[^/?#[:space:]]+' AND position('#' IN url) = 0),
    label text CHECK (label IS NULL OR (char_length(label) BETWEEN 1 AND 80 AND label !~ '^[[:space:]]|[[:space:]]$')),
    source_type text NOT NULL CHECK (source_type IN ('official', 'store', 'archive', 'mirror', 'community', 'external', 'unknown')),
    availability_state text NOT NULL DEFAULT 'active' CHECK (availability_state IN ('active', 'unavailable', 'broken', 'removed', 'restricted')),
    rights_status text NOT NULL DEFAULT 'unknown' CHECK (rights_status IN ('unknown', 'creator_provided', 'confirmed', 'disputed', 'rights_review', 'removed_by_request')),
    is_primary boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL CHECK (updated_at >= created_at),
    UNIQUE (resource_id, url)
);
CREATE UNIQUE INDEX resource_sources_primary_idx ON app.resource_sources (resource_id) WHERE is_primary;

CREATE TABLE app.resource_relations (
    id uuid PRIMARY KEY,
    source_resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
    target_resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
    relation_type text NOT NULL CHECK (relation_type IN ('part_of', 'successor_of', 'derived_from', 'related_to')),
    created_at timestamptz NOT NULL,
    UNIQUE (source_resource_id, target_resource_id, relation_type),
    CHECK (source_resource_id <> target_resource_id),
    CHECK (relation_type <> 'related_to' OR source_resource_id < target_resource_id)
);
CREATE INDEX resource_relations_target_idx ON app.resource_relations (target_resource_id, relation_type);

CREATE TABLE app.resource_external_ids (
    resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
    namespace text NOT NULL CHECK (char_length(namespace) BETWEEN 1 AND 64 AND namespace ~ '^[a-z0-9]+([._-][a-z0-9]+)*$'),
    external_id text NOT NULL CHECK (char_length(external_id) BETWEEN 1 AND 512 AND external_id !~ '^[[:space:]]|[[:space:]]$'),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (resource_id, namespace, external_id),
    UNIQUE (namespace, external_id)
);

-- Clear shared default grants within this migration transaction before granting
-- the exact new-object privileges. Do not alter cluster roles/default privileges.
REVOKE ALL ON app.categories, app.category_localizations, app.tags, app.tag_localizations,
    app.resources, app.resource_localizations, app.resource_tags, app.resource_sources,
    app.resource_relations, app.resource_external_ids
FROM PUBLIC, gfp_api, gfp_admin, gfp_worker, gfp_readonly;

GRANT SELECT ON app.categories, app.category_localizations, app.tags, app.tag_localizations,
    app.resources, app.resource_localizations, app.resource_tags, app.resource_sources,
    app.resource_relations, app.resource_external_ids TO gfp_api, gfp_admin, gfp_readonly;

GRANT INSERT ON app.categories, app.category_localizations, app.tags, app.tag_localizations,
    app.resources, app.resource_localizations, app.resource_tags, app.resource_sources,
    app.resource_relations, app.resource_external_ids TO gfp_admin;
GRANT UPDATE (default_locale, state, updated_at, deleted_at) ON app.categories, app.tags TO gfp_admin;
GRANT UPDATE (name, description, updated_at) ON app.category_localizations, app.tag_localizations TO gfp_admin;
GRANT UPDATE (slug, default_locale, category_id, publication_state, lifecycle, content_rating,
    version, published_at, updated_at, deleted_at) ON app.resources TO gfp_admin;
GRANT UPDATE (name, summary, description, updated_at) ON app.resource_localizations TO gfp_admin;
GRANT UPDATE (url, label, source_type, availability_state, rights_status, is_primary, updated_at) ON app.resource_sources TO gfp_admin;
GRANT DELETE ON app.category_localizations, app.tag_localizations, app.resource_localizations,
    app.resource_tags, app.resource_relations, app.resource_external_ids TO gfp_admin;

-- +goose Down
-- Only the guarded disposable integration test runs this rollback. Shared dev
-- uses the existing up-only migrator; no CASCADE or seed rows are introduced.
DROP TABLE app.resource_external_ids;
DROP TABLE app.resource_relations;
DROP TABLE app.resource_sources;
DROP TABLE app.resource_tags;
DROP TABLE app.resource_localizations;
DROP TABLE app.resources;
DROP TABLE app.tag_localizations;
DROP TABLE app.tags;
DROP TABLE app.category_localizations;
DROP TABLE app.categories;
