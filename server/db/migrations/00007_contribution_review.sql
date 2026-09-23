-- +goose Up
CREATE TABLE app.contributions (
    id uuid PRIMARY KEY,
    author_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('create_resource','update_resource')),
    target_resource_id uuid REFERENCES app.resources(id) ON DELETE RESTRICT,
    base_version bigint,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','rejected','withdrawn')),
    reason text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 2000 AND reason=btrim(reason)),
    previous_id uuid REFERENCES app.contributions(id) ON DELETE RESTRICT,
    request_id uuid NOT NULL,
    request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint)=32),
    -- Closed presence bits: name,summary,description,category,lifecycle,rating,locale.
    submitted_fields smallint NOT NULL CHECK (submitted_fields BETWEEN 0 AND 127),
    result_resource_id uuid REFERENCES app.resources(id) ON DELETE RESTRICT,
    result_version bigint,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    decided_at timestamptz,
    UNIQUE (author_id,request_id),
    CHECK ((kind='create_resource' AND target_resource_id IS NULL AND base_version IS NULL)
        OR (kind='update_resource' AND target_resource_id IS NOT NULL AND base_version IS NOT NULL AND base_version>0)),
    CHECK ((status='pending' AND decided_at IS NULL) OR (status<>'pending' AND decided_at IS NOT NULL AND decided_at>=created_at)),
    CHECK ((status='accepted' AND result_resource_id IS NOT NULL AND result_version IS NOT NULL AND result_version>0)
        OR (status<>'accepted' AND result_resource_id IS NULL AND result_version IS NULL))
);
CREATE INDEX contributions_author_created_idx ON app.contributions(author_id,created_at DESC,id DESC);
CREATE INDEX contributions_queue_idx ON app.contributions(status,created_at,id);

-- Closed, typed snapshots, not arbitrary properties or JSON payloads.
CREATE TABLE app.contribution_contents (
    contribution_id uuid NOT NULL REFERENCES app.contributions(id) ON DELETE RESTRICT,
    content_kind text NOT NULL CHECK (content_kind IN ('base','proposed','accepted')),
    default_locale text NOT NULL CHECK (char_length(default_locale) BETWEEN 2 AND 64),
    category_id uuid NOT NULL REFERENCES app.categories(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160 AND name=btrim(name)),
    summary text CHECK (char_length(summary) BETWEEN 1 AND 500 AND summary=btrim(summary)),
    description text CHECK (char_length(description) BETWEEN 1 AND 50000 AND description=btrim(description)),
    lifecycle text NOT NULL CHECK (lifecycle IN ('active','inactive','discontinued','delisted','archived','unknown')),
    content_rating text NOT NULL CHECK (content_rating IN ('general','mature','explicit')),
    slug text CHECK (char_length(slug) BETWEEN 1 AND 80 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    PRIMARY KEY (contribution_id,content_kind)
);
CREATE TABLE app.contribution_initial_sources (
    contribution_id uuid NOT NULL,
    content_kind text NOT NULL,
    url text NOT NULL CHECK (char_length(url) BETWEEN 1 AND 2048 AND url=btrim(url) AND url ~ '^https?://[^/?#[:space:]]+' AND position('#' IN url)=0),
    label text CHECK (char_length(label) BETWEEN 1 AND 80 AND label=btrim(label)),
    source_type text NOT NULL CHECK (source_type IN ('official','store','archive','mirror','community','external','unknown')),
    availability_state text NOT NULL CHECK (availability_state IN ('active','unavailable','broken','removed','restricted')),
    PRIMARY KEY (contribution_id,content_kind),
    FOREIGN KEY (contribution_id,content_kind) REFERENCES app.contribution_contents(contribution_id,content_kind) ON DELETE RESTRICT
);
CREATE TABLE app.contribution_events (
    contribution_id uuid NOT NULL REFERENCES app.contributions(id) ON DELETE RESTRICT,
    event_type text NOT NULL CHECK (event_type IN ('submitted','accepted','rejected','withdrawn')),
    actor_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    message text CHECK (char_length(message) BETWEEN 1 AND 2000),
    internal_note text CHECK (char_length(internal_note) BETWEEN 1 AND 2000),
    occurred_at timestamptz NOT NULL,
    PRIMARY KEY (contribution_id,event_type)
);
CREATE UNIQUE INDEX contribution_terminal_event_idx ON app.contribution_events(contribution_id) WHERE event_type<>'submitted';
CREATE TABLE app.contribution_review_audits (
    contribution_id uuid PRIMARY KEY REFERENCES app.contributions(id) ON DELETE RESTRICT,
    actor_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    action text NOT NULL CHECK (action IN ('accepted','rejected')),
    resource_id uuid REFERENCES app.resources(id) ON DELETE RESTRICT,
    before_version bigint CHECK (before_version>0),
    after_version bigint CHECK (after_version>0),
    occurred_at timestamptz NOT NULL,
    CHECK ((action='accepted' AND resource_id IS NOT NULL AND after_version IS NOT NULL)
        OR (action='rejected' AND after_version IS NULL))
);

REVOKE ALL ON app.contributions,app.contribution_contents,app.contribution_initial_sources,
    app.contribution_events,app.contribution_review_audits FROM PUBLIC,gfp_api,gfp_admin,gfp_worker,gfp_readonly;
GRANT SELECT ON app.contributions,app.contribution_contents,app.contribution_initial_sources TO gfp_api,gfp_admin,gfp_readonly;
GRANT SELECT ON app.contribution_events,app.contribution_review_audits TO gfp_admin,gfp_readonly;
GRANT SELECT(contribution_id,event_type,message,occurred_at) ON app.contribution_events TO gfp_api;
GRANT INSERT(id,author_id,kind,target_resource_id,base_version,reason,previous_id,request_id,request_fingerprint,submitted_fields) ON app.contributions TO gfp_api;
GRANT INSERT ON app.contribution_contents,app.contribution_initial_sources TO gfp_api;
GRANT INSERT(contribution_id,event_type,actor_id,message,occurred_at) ON app.contribution_events TO gfp_api;
GRANT INSERT ON app.contribution_contents,app.contribution_initial_sources,app.contribution_events,app.contribution_review_audits TO gfp_admin;
GRANT UPDATE(status,decided_at) ON app.contributions TO gfp_api;
GRANT UPDATE(status,decided_at,result_resource_id,result_version) ON app.contributions TO gfp_admin;

-- +goose Down
-- Disposable integration only; shared development remains up-only.
DROP TABLE app.contribution_review_audits;
DROP TABLE app.contribution_events;
DROP TABLE app.contribution_initial_sources;
DROP TABLE app.contribution_contents;
DROP TABLE app.contributions;
