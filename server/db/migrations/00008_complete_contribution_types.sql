-- +goose Up
ALTER TABLE app.contributions DROP CONSTRAINT contributions_kind_check;
ALTER TABLE app.contributions DROP CONSTRAINT contributions_check;
ALTER TABLE app.contributions ADD CONSTRAINT contributions_kind_check CHECK (kind IN
 ('create_resource','update_resource','add_source','remove_broken_source','add_tag','add_relation','add_translation'));
ALTER TABLE app.contributions ADD CONSTRAINT contributions_check CHECK (
 (kind='create_resource' AND target_resource_id IS NULL AND base_version IS NULL) OR
 (kind<>'create_resource' AND target_resource_id IS NOT NULL AND base_version IS NOT NULL AND base_version>0));

CREATE TABLE app.contribution_source_changes (
 contribution_id uuid NOT NULL REFERENCES app.contributions(id) ON DELETE RESTRICT,
 snapshot_kind text NOT NULL CHECK (snapshot_kind IN ('base','proposed','accepted')),
 source_id uuid REFERENCES app.resource_sources(id) ON DELETE RESTRICT,
 url text CHECK (char_length(url) BETWEEN 1 AND 2048 AND url=btrim(url) AND url ~ '^https?://[^/?#[:space:]]+' AND position('#' IN url)=0),
 label text CHECK (char_length(label) BETWEEN 1 AND 80 AND label=btrim(label)),
 source_type text CHECK (source_type IN ('official','store','archive','mirror','community','external','unknown')),
 availability_state text CHECK (availability_state IN ('active','unavailable','broken','restricted','removed')),
 CHECK ((url IS NULL AND label IS NULL AND source_type IS NULL AND availability_state IS NULL)
     OR (url IS NOT NULL AND source_type IS NOT NULL)),
 PRIMARY KEY(contribution_id,snapshot_kind)
);
CREATE TABLE app.contribution_tag_changes (
 contribution_id uuid NOT NULL REFERENCES app.contributions(id) ON DELETE RESTRICT,
 snapshot_kind text NOT NULL CHECK (snapshot_kind IN ('base','proposed','accepted')),
 tag_id uuid NOT NULL REFERENCES app.tags(id) ON DELETE RESTRICT,
 was_bound boolean NOT NULL,
 PRIMARY KEY(contribution_id,snapshot_kind,tag_id)
);
CREATE TABLE app.contribution_relation_changes (
 contribution_id uuid NOT NULL REFERENCES app.contributions(id) ON DELETE RESTRICT,
 snapshot_kind text NOT NULL CHECK (snapshot_kind IN ('base','proposed','accepted')),
 other_resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
 relation_type text NOT NULL CHECK (relation_type IN ('part_of','successor_of','derived_from','related_to')),
 direction text NOT NULL CHECK (direction IN ('outgoing','incoming','symmetric')),
 other_base_version bigint NOT NULL CHECK (other_base_version>0),
 anchor_result_version bigint CHECK (anchor_result_version>0),
 other_result_version bigint CHECK (other_result_version>0),
 CHECK ((relation_type='related_to') = (direction='symmetric')),
 CHECK ((snapshot_kind='accepted' AND anchor_result_version IS NOT NULL AND other_result_version IS NOT NULL)
     OR (snapshot_kind<>'accepted' AND anchor_result_version IS NULL AND other_result_version IS NULL)),
 PRIMARY KEY(contribution_id,snapshot_kind)
);
CREATE TABLE app.contribution_localization_changes (
 contribution_id uuid NOT NULL REFERENCES app.contributions(id) ON DELETE RESTRICT,
 snapshot_kind text NOT NULL CHECK (snapshot_kind IN ('base','proposed','accepted')),
 locale text NOT NULL CHECK (char_length(locale) BETWEEN 2 AND 64),
 row_exists boolean NOT NULL,
 name text CHECK (char_length(name) BETWEEN 1 AND 160 AND name=btrim(name)),
 summary text CHECK (char_length(summary) BETWEEN 1 AND 500 AND summary=btrim(summary)),
 description text CHECK (char_length(description) BETWEEN 1 AND 50000 AND description=btrim(description)),
 -- Fixed presence bits: name=1, summary=2, description=4.
 supplied_fields smallint NOT NULL CHECK (supplied_fields BETWEEN 0 AND 7),
 CHECK ((row_exists AND name IS NOT NULL) OR (NOT row_exists AND snapshot_kind='base' AND name IS NULL AND summary IS NULL AND description IS NULL)),
 PRIMARY KEY(contribution_id,snapshot_kind)
);
CREATE TABLE app.contribution_review_resource_changes (
 contribution_id uuid NOT NULL REFERENCES app.contribution_review_audits(contribution_id) ON DELETE RESTRICT,
 resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
 before_version bigint NOT NULL CHECK (before_version>0),
 after_version bigint NOT NULL CHECK (after_version=before_version+1),
 PRIMARY KEY(contribution_id,resource_id)
);
REVOKE ALL ON app.contribution_source_changes,app.contribution_tag_changes,app.contribution_relation_changes,
 app.contribution_localization_changes,app.contribution_review_resource_changes FROM PUBLIC,gfp_api,gfp_admin,gfp_worker,gfp_readonly;
GRANT SELECT ON app.contribution_source_changes,app.contribution_tag_changes,app.contribution_relation_changes,
 app.contribution_localization_changes TO gfp_api,gfp_admin,gfp_readonly;
GRANT INSERT(contribution_id,snapshot_kind,source_id,url,label,source_type,availability_state) ON app.contribution_source_changes TO gfp_api,gfp_admin;
GRANT INSERT(contribution_id,snapshot_kind,tag_id,was_bound) ON app.contribution_tag_changes TO gfp_api,gfp_admin;
GRANT INSERT(contribution_id,snapshot_kind,other_resource_id,relation_type,direction,other_base_version) ON app.contribution_relation_changes TO gfp_api;
GRANT INSERT(contribution_id,snapshot_kind,other_resource_id,relation_type,direction,other_base_version,anchor_result_version,other_result_version) ON app.contribution_relation_changes TO gfp_admin;
GRANT INSERT(contribution_id,snapshot_kind,locale,row_exists,name,summary,description,supplied_fields) ON app.contribution_localization_changes TO gfp_api,gfp_admin;
GRANT SELECT ON app.contribution_review_resource_changes TO gfp_admin,gfp_readonly;
GRANT INSERT(contribution_id,resource_id,before_version,after_version) ON app.contribution_review_resource_changes TO gfp_admin;

-- +goose Down
-- Never discard contributions to make a downgrade possible. Shared dev is up-only.
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM app.contributions WHERE kind NOT IN ('create_resource','update_resource')) THEN
  RAISE EXCEPTION 'Cannot downgrade while complete contribution types exist';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE app.contribution_review_resource_changes;
DROP TABLE app.contribution_localization_changes;
DROP TABLE app.contribution_relation_changes;
DROP TABLE app.contribution_tag_changes;
DROP TABLE app.contribution_source_changes;
ALTER TABLE app.contributions DROP CONSTRAINT contributions_kind_check;
ALTER TABLE app.contributions DROP CONSTRAINT contributions_check;
ALTER TABLE app.contributions ADD CONSTRAINT contributions_kind_check CHECK (kind IN ('create_resource','update_resource'));
ALTER TABLE app.contributions ADD CONSTRAINT contributions_check CHECK (
 (kind='create_resource' AND target_resource_id IS NULL AND base_version IS NULL) OR
 (kind='update_resource' AND target_resource_id IS NOT NULL AND base_version IS NOT NULL AND base_version>0));
