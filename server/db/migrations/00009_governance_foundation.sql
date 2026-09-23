-- +goose Up
CREATE TABLE app.reports (
 id uuid PRIMARY KEY,
 reporter_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
 source_id uuid REFERENCES app.resource_sources(id) ON DELETE RESTRICT,
 target_kind text NOT NULL CHECK (target_kind IN ('resource','source')),
 reason text NOT NULL CHECK (reason IN ('broken_link','rights_concern','malicious_link','privacy','content_rating','spam','other')),
 body text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4000 AND body=btrim(body)),
 request_id uuid NOT NULL,
 request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint)=32),
 status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','in_review','resolved','dismissed','withdrawn')),
 version bigint NOT NULL DEFAULT 1 CHECK (version>0),
 queue text NOT NULL CHECK (queue IN ('moderation','administration')),
 priority smallint NOT NULL CHECK (priority IN (0,1)),
 duplicate_of uuid REFERENCES app.reports(id) ON DELETE RESTRICT,
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 decided_at timestamptz,
 UNIQUE(reporter_id,request_id),
 CHECK ((target_kind='source')=(source_id IS NOT NULL)),
 CHECK ((status IN ('resolved','dismissed','withdrawn'))=(decided_at IS NOT NULL)),
 CHECK (duplicate_of IS NULL OR duplicate_of<>id)
);
CREATE INDEX reports_owner ON app.reports(reporter_id,created_at DESC,id DESC);
CREATE INDEX reports_queue ON app.reports(status,priority DESC,created_at,id);
CREATE UNIQUE INDEX reports_active_target ON app.reports(reporter_id,resource_id,COALESCE(source_id,'00000000-0000-0000-0000-000000000000'::uuid),reason) WHERE status IN ('open','in_review');
CREATE TABLE app.report_events (
 id uuid PRIMARY KEY,
 report_id uuid NOT NULL REFERENCES app.reports(id) ON DELETE RESTRICT,
 event_type text NOT NULL CHECK (event_type IN ('submitted','triaged','escalated','noted','resolved','dismissed','withdrawn')),
 actor_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 request_id uuid NOT NULL,
 request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint)=32),
 safe_message text CHECK (char_length(safe_message) BETWEEN 1 AND 1000),
 internal_note text CHECK (char_length(internal_note) BETWEEN 1 AND 2000),
 audit_id uuid,
 resolution_type text CHECK (resolution_type IN ('no_change','link_audit','publication','source_availability','source_rights','distribution')),
 occurred_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE(actor_id,request_id),
 CHECK (event_type NOT IN ('resolved','dismissed') OR safe_message IS NOT NULL),
 CHECK ((event_type='resolved')=(resolution_type IS NOT NULL)),
 CHECK ((resolution_type IS NULL OR resolution_type='no_change')=(audit_id IS NULL))
);
CREATE INDEX report_events_history ON app.report_events(report_id,occurred_at,id);
CREATE UNIQUE INDEX report_events_terminal ON app.report_events(report_id) WHERE event_type IN ('resolved','dismissed','withdrawn');
CREATE TABLE app.user_governance_profiles (
 user_id uuid PRIMARY KEY REFERENCES app.users(id) ON DELETE RESTRICT,
 trust_level text NOT NULL CHECK (trust_level IN ('new','established','trusted')),
 revision bigint NOT NULL CHECK (revision>0),
 updated_at timestamptz NOT NULL DEFAULT transaction_timestamp()
);
CREATE TABLE app.user_restrictions (
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 scope text NOT NULL CHECK (scope IN ('contribution_submit','public_profile_write','all_write')),
 reason_code text NOT NULL CHECK (reason_code IN ('spam','abuse','repeated_policy_violation','other')),
 user_message text NOT NULL CHECK (char_length(user_message) BETWEEN 1 AND 1000),
 internal_note text CHECK (char_length(internal_note) BETWEEN 1 AND 2000),
 created_by uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 starts_at timestamptz NOT NULL,
 expires_at timestamptz,
 revoked_at timestamptz,
 revoked_by uuid REFERENCES app.users(id) ON DELETE RESTRICT,
 revoke_reason text CHECK (char_length(revoke_reason) BETWEEN 1 AND 1000),
 CHECK (expires_at IS NULL OR expires_at>starts_at),
 CHECK ((revoked_at IS NULL AND revoked_by IS NULL AND revoke_reason IS NULL) OR (revoked_at IS NOT NULL AND revoked_by IS NOT NULL AND revoke_reason IS NOT NULL)),
 CHECK (created_by<>user_id AND (revoked_by IS NULL OR revoked_by<>user_id))
);
CREATE INDEX user_restrictions_effective ON app.user_restrictions(user_id,scope,expires_at) WHERE revoked_at IS NULL;
CREATE TABLE app.resource_distribution_policies (
 resource_id uuid PRIMARY KEY REFERENCES app.resources(id) ON DELETE RESTRICT,
 policy text NOT NULL CHECK (policy IN ('normal','excluded'))
);
CREATE TABLE app.source_checks (
 id uuid PRIMARY KEY,
 resource_id uuid NOT NULL REFERENCES app.resources(id) ON DELETE RESTRICT,
 source_id uuid NOT NULL REFERENCES app.resource_sources(id) ON DELETE RESTRICT,
 url_fingerprint bytea NOT NULL CHECK (octet_length(url_fingerprint)=32),
 resource_version bigint NOT NULL CHECK (resource_version>0),
 outcome text NOT NULL CHECK (outcome IN ('reachable','unreachable','uncertain')),
 note text NOT NULL CHECK (char_length(note) BETWEEN 1 AND 2000),
 actor_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 request_id uuid NOT NULL,
 request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint)=32),
 observed_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE(actor_id,request_id)
);
CREATE INDEX source_checks_latest ON app.source_checks(source_id,observed_at DESC,id DESC);
CREATE TABLE app.moderation_actions (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 action text NOT NULL CHECK (action IN ('publication','source_rights','source_availability','distribution','trust','restrict','revoke_restriction')),
 resource_id uuid REFERENCES app.resources(id) ON DELETE RESTRICT,
 source_id uuid REFERENCES app.resource_sources(id) ON DELETE RESTRICT,
 user_id uuid REFERENCES app.users(id) ON DELETE RESTRICT,
 restriction_id uuid REFERENCES app.user_restrictions(id) ON DELETE RESTRICT,
 report_id uuid REFERENCES app.reports(id) ON DELETE RESTRICT,
 reason text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 1000),
 internal_note text CHECK (char_length(internal_note) BETWEEN 1 AND 2000),
 request_id uuid NOT NULL,
 request_fingerprint bytea NOT NULL CHECK (octet_length(request_fingerprint)=32),
 occurred_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 UNIQUE(actor_id,request_id),
 CHECK ((resource_id IS NOT NULL)::int+(user_id IS NOT NULL)::int=1),
 CHECK (source_id IS NULL OR resource_id IS NOT NULL),
 CHECK (restriction_id IS NULL OR user_id IS NOT NULL)
);
CREATE TABLE app.audit_entries (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
 operation text NOT NULL CHECK (operation IN ('create','core','localization','tags','source','source_rights','relation','external_ids','publication','soft_delete','taxonomy','distribution','trust','restrict','revoke_restriction')),
 resource_id uuid REFERENCES app.resources(id) ON DELETE RESTRICT,
 category_id uuid REFERENCES app.categories(id) ON DELETE RESTRICT,
 tag_id uuid REFERENCES app.tags(id) ON DELETE RESTRICT,
 user_id uuid REFERENCES app.users(id) ON DELETE RESTRICT,
 source_id uuid REFERENCES app.resource_sources(id) ON DELETE RESTRICT,
 contribution_id uuid REFERENCES app.contributions(id) ON DELETE RESTRICT,
 moderation_action_id uuid REFERENCES app.moderation_actions(id) ON DELETE RESTRICT,
 fields text[] NOT NULL CHECK (cardinality(fields)>0 AND fields <@ ARRAY['core','localization','tags','sources','relations','external_ids','publication_state','deleted_at','taxonomy','distribution','trust','restrictions']::text[]),
 before_version bigint,
 after_version bigint,
 before_availability text CHECK (before_availability IN ('active','unavailable','broken','restricted','removed')),
 after_availability text CHECK (after_availability IN ('active','unavailable','broken','restricted','removed')),
 before_taxonomy_state text CHECK (before_taxonomy_state IN ('active','retired')),
 after_taxonomy_state text CHECK (after_taxonomy_state IN ('active','retired')),
 before_publication text CHECK (before_publication IN ('draft','pending','published','restricted','removed')),
 after_publication text CHECK (after_publication IN ('draft','pending','published','restricted','removed')),
 before_rights text CHECK (before_rights IN ('unknown','creator_provided','confirmed','rights_review','disputed','removed_by_request')),
 after_rights text CHECK (after_rights IN ('unknown','creator_provided','confirmed','rights_review','disputed','removed_by_request')),
 before_distribution text CHECK (before_distribution IN ('normal','excluded')),
 after_distribution text CHECK (after_distribution IN ('normal','excluded')),
 before_trust text CHECK (before_trust IN ('new','established','trusted')),
 after_trust text CHECK (after_trust IN ('new','established','trusted')),
 reason text CHECK (char_length(reason) BETWEEN 1 AND 1000),
 occurred_at timestamptz NOT NULL DEFAULT transaction_timestamp(),
 CHECK ((resource_id IS NOT NULL)::int+(category_id IS NOT NULL)::int+(tag_id IS NOT NULL)::int+(user_id IS NOT NULL)::int=1),
 CHECK (source_id IS NULL OR resource_id IS NOT NULL),
 CHECK ((before_version IS NULL AND after_version IS NULL) OR (before_version>=0 AND after_version=before_version+1))
);
CREATE INDEX audit_entries_timeline ON app.audit_entries(occurred_at DESC,id DESC);
CREATE INDEX audit_entries_resource ON app.audit_entries(resource_id,occurred_at DESC,id DESC);
CREATE INDEX audit_entries_user ON app.audit_entries(user_id,occurred_at DESC,id DESC);
ALTER TABLE app.report_events ADD CONSTRAINT report_event_audit_fk FOREIGN KEY(audit_id) REFERENCES app.audit_entries(id) ON DELETE RESTRICT;

REVOKE ALL ON app.reports,app.report_events,app.user_governance_profiles,app.user_restrictions,
 app.resource_distribution_policies,app.source_checks,app.moderation_actions,app.audit_entries FROM PUBLIC,gfp_api,gfp_admin,gfp_worker,gfp_readonly;
GRANT SELECT ON app.reports,app.report_events,app.user_governance_profiles,app.user_restrictions,
 app.resource_distribution_policies,app.source_checks,app.moderation_actions,app.audit_entries TO gfp_admin,gfp_readonly;
GRANT SELECT(id,reporter_id,resource_id,source_id,target_kind,reason,body,request_id,request_fingerprint,status,version,created_at,decided_at) ON app.reports TO gfp_api;
GRANT INSERT(id,reporter_id,resource_id,source_id,target_kind,reason,body,request_id,request_fingerprint,queue,priority) ON app.reports TO gfp_api;
GRANT UPDATE(status,version,updated_at,decided_at) ON app.reports TO gfp_api;
GRANT UPDATE(status,version,updated_at,decided_at,queue,duplicate_of) ON app.reports TO gfp_admin;
GRANT SELECT(id,report_id,event_type,safe_message,occurred_at,actor_id,request_id,request_fingerprint) ON app.report_events TO gfp_api;
GRANT INSERT(id,report_id,event_type,actor_id,request_id,request_fingerprint,safe_message) ON app.report_events TO gfp_api;
GRANT INSERT(id,report_id,event_type,actor_id,request_id,request_fingerprint,safe_message,internal_note,audit_id,resolution_type) ON app.report_events TO gfp_admin;
GRANT INSERT(user_id,trust_level,revision) ON app.user_governance_profiles TO gfp_admin;
GRANT INSERT(id,user_id,scope,reason_code,user_message,internal_note,created_by,starts_at,expires_at) ON app.user_restrictions TO gfp_admin;
GRANT INSERT(resource_id,policy) ON app.resource_distribution_policies TO gfp_admin;
GRANT INSERT(id,resource_id,source_id,url_fingerprint,resource_version,outcome,note,actor_id,request_id,request_fingerprint,observed_at) ON app.source_checks TO gfp_admin;
GRANT INSERT(id,actor_id,action,resource_id,source_id,user_id,restriction_id,report_id,reason,internal_note,request_id,request_fingerprint) ON app.moderation_actions TO gfp_admin;
GRANT INSERT(id,actor_id,operation,resource_id,category_id,tag_id,user_id,source_id,contribution_id,moderation_action_id,fields,before_version,after_version,before_publication,after_publication,before_rights,after_rights,before_distribution,after_distribution,before_trust,after_trust,reason,before_availability,after_availability,before_taxonomy_state,after_taxonomy_state) ON app.audit_entries TO gfp_admin;
GRANT SELECT(user_id,trust_level,revision) ON app.user_governance_profiles TO gfp_api;
GRANT SELECT(id,user_id,scope,reason_code,user_message,starts_at,expires_at,revoked_at) ON app.user_restrictions TO gfp_api;
GRANT SELECT ON app.resource_distribution_policies TO gfp_api;
GRANT UPDATE(trust_level,revision,updated_at) ON app.user_governance_profiles TO gfp_admin;
GRANT UPDATE(revoked_at,revoked_by,revoke_reason) ON app.user_restrictions TO gfp_admin;
GRANT UPDATE(policy) ON app.resource_distribution_policies TO gfp_admin;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM app.reports) OR EXISTS(SELECT 1 FROM app.report_events)
 OR EXISTS(SELECT 1 FROM app.user_governance_profiles) OR EXISTS(SELECT 1 FROM app.user_restrictions)
 OR EXISTS(SELECT 1 FROM app.resource_distribution_policies) OR EXISTS(SELECT 1 FROM app.source_checks)
 OR EXISTS(SELECT 1 FROM app.moderation_actions) OR EXISTS(SELECT 1 FROM app.audit_entries) THEN
  RAISE EXCEPTION 'Cannot downgrade while governance data exists';
 END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE app.report_events DROP CONSTRAINT report_event_audit_fk;
DROP TABLE app.audit_entries;
DROP TABLE app.moderation_actions;
DROP TABLE app.source_checks;
DROP TABLE app.resource_distribution_policies;
DROP TABLE app.user_restrictions;
DROP TABLE app.user_governance_profiles;
DROP TABLE app.report_events;
DROP TABLE app.reports;
