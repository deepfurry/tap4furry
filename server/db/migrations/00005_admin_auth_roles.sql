-- +goose Up
CREATE TABLE app.user_roles (
    user_id uuid NOT NULL REFERENCES app.users(id) ON DELETE RESTRICT,
    role text NOT NULL CHECK (role IN ('moderator', 'editor', 'admin')),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, role)
);

-- Prepared shared defaults may grant future tables broadly. This new owned
-- table is operator-write-only, regardless of those defaults.
REVOKE ALL ON app.user_roles FROM PUBLIC, gfp_api, gfp_admin, gfp_worker, gfp_readonly;

ALTER TABLE app.security_events DROP CONSTRAINT security_events_event_type_check;
ALTER TABLE app.security_events ADD CONSTRAINT security_events_event_type_check CHECK (event_type IN (
    'account_registered', 'login_succeeded', 'logout',
    'email_verification_requested', 'email_verified',
    'password_reset_requested', 'password_reset_completed', 'password_changed',
    'reauthenticated', 'session_revoked', 'other_sessions_revoked',
    'oauth_login_succeeded', 'oauth_identity_linked', 'oauth_identity_unlinked', 'oauth_reauthenticated',
    'login_failed', 'admin_login_succeeded', 'admin_login_failed', 'admin_logout',
    'admin_reauthenticated', 'admin_session_revoked', 'admin_other_sessions_revoked',
    'role_moderator_granted', 'role_moderator_revoked', 'role_editor_granted',
    'role_editor_revoked', 'role_admin_granted', 'role_admin_revoked'
));

GRANT USAGE ON SCHEMA app TO gfp_admin;
GRANT SELECT ON app.users, app.auth_identities, app.password_credentials, app.user_roles, app.sessions TO gfp_admin;
GRANT INSERT ON app.sessions, app.security_events TO gfp_admin;
GRANT UPDATE (last_seen_at, idle_expires_at, revoked_at) ON app.sessions TO gfp_admin;
-- User/credential row locks and compare-and-swap KDF upgrades only.
GRANT UPDATE (updated_at) ON app.users TO gfp_admin;
GRANT UPDATE (password_hash, updated_at) ON app.password_credentials TO gfp_admin;
GRANT SELECT ON app.user_roles TO gfp_readonly;

-- +goose Down
REVOKE SELECT ON app.user_roles FROM gfp_readonly;
REVOKE UPDATE (password_hash, updated_at) ON app.password_credentials FROM gfp_admin;
REVOKE UPDATE (updated_at) ON app.users FROM gfp_admin;
REVOKE UPDATE (last_seen_at, idle_expires_at, revoked_at) ON app.sessions FROM gfp_admin;
REVOKE INSERT ON app.sessions, app.security_events FROM gfp_admin;
REVOKE SELECT ON app.users, app.auth_identities, app.password_credentials, app.user_roles, app.sessions FROM gfp_admin;
-- Refuse rollback over retained Admin security history; never erase audit rows.
ALTER TABLE app.security_events DROP CONSTRAINT security_events_event_type_check;
ALTER TABLE app.security_events ADD CONSTRAINT security_events_event_type_check CHECK (event_type IN (
    'account_registered', 'login_succeeded', 'logout',
    'email_verification_requested', 'email_verified',
    'password_reset_requested', 'password_reset_completed', 'password_changed',
    'reauthenticated', 'session_revoked', 'other_sessions_revoked',
    'oauth_login_succeeded', 'oauth_identity_linked', 'oauth_identity_unlinked', 'oauth_reauthenticated'
));
DROP TABLE app.user_roles;
