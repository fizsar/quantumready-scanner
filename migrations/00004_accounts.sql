-- Phase 3 (milestone 3.1): accounts, organizations, verified domains, audit
-- log, email outbox, and tenant isolation with row-level security (RLS).
--
-- Isolation has two layers (docs/security.md):
-- 1. The application scopes every query to the organization of the session.
-- 2. PostgreSQL RLS: the API runs its queries as role qr_app inside a
--    transaction that sets app.org_id / app.user_id (store.Store.Tx). With
--    FORCE ROW LEVEL SECURITY, a missing WHERE clause still cannot read or
--    write another organization's rows. The worker runs as qr_worker, which
--    serves every organization but only on the tables it needs (00005).
--
-- Roles are cluster-wide, so they are created only if missing (tests run each
-- package in its own schema of the same database) and never dropped here.

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'qr_app') THEN
        CREATE ROLE qr_app NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'qr_worker') THEN
        CREATE ROLE qr_worker NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
    -- The connecting user (owner, used by migrate and, until milestone 3.7,
    -- by the API and the worker) can switch to them with SET ROLE.
    EXECUTE format('GRANT qr_app, qr_worker TO %I', current_user);
    EXECUTE format('GRANT USAGE ON SCHEMA %I TO qr_app, qr_worker', current_schema());
END
$$;
-- +goose StatementEnd

-- Tenant context of the current transaction (set by store.Store.Tx).
CREATE FUNCTION app_org_id() RETURNS uuid LANGUAGE sql STABLE AS
$$ SELECT nullif(current_setting('app.org_id', true), '')::uuid $$;
CREATE FUNCTION app_user_id() RETURNS uuid LANGUAGE sql STABLE AS
$$ SELECT nullif(current_setting('app.user_id', true), '')::uuid $$;

CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Lowercased by the application; unique among live accounts.
    email             text NOT NULL UNIQUE CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
    email_verified_at timestamptz,
    password_hash     text NOT NULL, -- argon2id, PHC string format
    name              text CHECK (length(name) <= 100),
    lang              text NOT NULL DEFAULT 'es' CHECK (lang IN ('es', 'en')),
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_login_at     timestamptz
);

-- Opaque session tokens: only their SHA-256 is stored.
CREATE TABLE sessions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash    bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    csrf_token    text NOT NULL,
    -- Organization the user is working in (switchable).
    active_org_id uuid,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

-- Single-use email tokens (verification, password reset): hash only.
CREATE TABLE auth_tokens (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose    text NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    token_hash bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);
CREATE INDEX auth_tokens_user_idx ON auth_tokens (user_id);

-- Closed beta: registration needs one of these codes (hash only).
CREATE TABLE signup_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash  bytea NOT NULL UNIQUE CHECK (length(code_hash) = 32),
    note       text,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_by    uuid REFERENCES users (id) ON DELETE SET NULL,
    used_at    timestamptz
);

CREATE TABLE organizations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE sessions ADD CONSTRAINT sessions_active_org_fk
    FOREIGN KEY (active_org_id) REFERENCES organizations (id) ON DELETE SET NULL;

CREATE TABLE memberships (
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('owner', 'admin', 'viewer')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE invitations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    email           text NOT NULL CHECK (email = lower(email)),
    role            text NOT NULL CHECK (role IN ('admin', 'viewer')),
    token_hash      bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    invited_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL,
    accepted_at     timestamptz
);
CREATE INDEX invitations_org_idx ON invitations (organization_id);

-- Domains an organization verifies with a TXT record at
-- _quantumready.<name> (value qr-verify=<token>). Verifying a domain covers
-- its subdomains. Several organizations may verify the same domain, each with
-- its own token and its own scans.
CREATE TABLE domains (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    name            text NOT NULL CHECK (name = lower(name) AND length(name) BETWEEN 1 AND 253),
    token           text NOT NULL,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'lost')),
    verified_at     timestamptz,
    last_checked_at timestamptz,
    lost_at         timestamptz, -- record gone: full scans stop after the grace period
    -- The owner can block public (anonymous) scans of this domain.
    block_public    boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);
CREATE INDEX domains_name_idx ON domains (name);

-- Sensitive actions. IP truncated (/24, /48) and erased after 90 days.
CREATE TABLE audit_log (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id uuid REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid REFERENCES users (id) ON DELETE SET NULL,
    action          text NOT NULL,
    target          text,
    ip_prefix       text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_org_idx ON audit_log (organization_id, created_at DESC);
CREATE INDEX audit_log_user_idx ON audit_log (user_id, created_at DESC);

-- Emails are queued here and sent by the worker (retries, no sending in
-- the request path).
CREATE TABLE email_outbox (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    to_email        text NOT NULL,
    template        text NOT NULL,
    lang            text NOT NULL CHECK (lang IN ('es', 'en')),
    params          jsonb NOT NULL DEFAULT '{}',
    status          text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'failed')),
    attempts        integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz
);
CREATE INDEX email_outbox_due_idx ON email_outbox (next_attempt_at) WHERE status = 'queued';

-- Hashes of deleted account and organization IDs, re-applied after restoring
-- a backup so that deleted data does not come back (docs/privacy).
CREATE TABLE deletion_tombstones (
    id_hash    bytea PRIMARY KEY CHECK (length(id_hash) = 32),
    kind       text NOT NULL CHECK (kind IN ('user', 'organization')),
    deleted_at timestamptz NOT NULL DEFAULT now()
);

-- Scans: owner (NULL = public, anonymous), intensity, requester, expiry.
ALTER TABLE scans
    ADD CONSTRAINT scans_organization_fk FOREIGN KEY (organization_id) REFERENCES organizations (id) ON DELETE CASCADE,
    ADD COLUMN intensity    text NOT NULL DEFAULT 'basic' CHECK (intensity IN ('basic', 'full')),
    ADD COLUMN requested_by uuid REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN expires_at   timestamptz;
CREATE INDEX scans_org_idx ON scans (organization_id, created_at DESC);
CREATE INDEX scans_expires_idx ON scans (expires_at) WHERE expires_at IS NOT NULL;
DROP INDEX scans_target_idx;
CREATE INDEX scans_target_idx ON scans (host, port, intensity, created_at DESC);

-- Existing scans (phase 2: allowlisted own domains, no accounts) become
-- public. Their intensity comes from the stored result (absent before
-- milestone 2.7.3: basic). Basic ones expire 30 days from now, like any
-- public scan; full ones stay until they are assigned to an organization.
UPDATE scans SET intensity = coalesce(result ->> 'intensity', 'basic') WHERE result IS NOT NULL;
UPDATE scans SET expires_at = now() + interval '30 days' WHERE intensity = 'basic';

-- Row-level security.
ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
-- A user sees the organizations they belong to (switcher); writes only to
-- the active one. Creating one (signup) happens before any membership.
CREATE POLICY organizations_read ON organizations FOR SELECT TO qr_app
    USING (id = app_org_id() OR id IN (SELECT m.organization_id FROM memberships m WHERE m.user_id = app_user_id()));
CREATE POLICY organizations_create ON organizations FOR INSERT TO qr_app WITH CHECK (true);
CREATE POLICY organizations_update ON organizations FOR UPDATE TO qr_app
    USING (id = app_org_id()) WITH CHECK (id = app_org_id());
CREATE POLICY organizations_delete ON organizations FOR DELETE TO qr_app USING (id = app_org_id());

-- Reading: the active organization's members, and the user's own
-- memberships everywhere. Writing: only in the active organization (seeing
-- your membership in another one does not let you change it).
ALTER TABLE memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY memberships_read ON memberships FOR SELECT TO qr_app
    USING (organization_id = app_org_id() OR user_id = app_user_id());
CREATE POLICY memberships_create ON memberships FOR INSERT TO qr_app WITH CHECK (organization_id = app_org_id());
CREATE POLICY memberships_update ON memberships FOR UPDATE TO qr_app
    USING (organization_id = app_org_id()) WITH CHECK (organization_id = app_org_id());
CREATE POLICY memberships_delete ON memberships FOR DELETE TO qr_app USING (organization_id = app_org_id());

ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY invitations_tenant ON invitations TO qr_app
    USING (organization_id = app_org_id()) WITH CHECK (organization_id = app_org_id());

ALTER TABLE domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE domains FORCE ROW LEVEL SECURITY;
CREATE POLICY domains_tenant ON domains TO qr_app
    USING (organization_id = app_org_id()) WITH CHECK (organization_id = app_org_id());

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_log_tenant ON audit_log TO qr_app
    USING (organization_id = app_org_id())
    -- Account events (login, password…) have no organization.
    WITH CHECK (organization_id IS NULL OR organization_id = app_org_id());

-- Public scans (no organization) are readable by anyone who has the ID;
-- organization scans only inside that organization.
ALTER TABLE scans ENABLE ROW LEVEL SECURITY;
ALTER TABLE scans FORCE ROW LEVEL SECURITY;
CREATE POLICY scans_tenant ON scans TO qr_app
    USING (organization_id IS NULL OR organization_id = app_org_id())
    WITH CHECK (organization_id IS NULL OR organization_id = app_org_id());

ALTER TABLE findings ENABLE ROW LEVEL SECURITY;
ALTER TABLE findings FORCE ROW LEVEL SECURITY;
CREATE POLICY findings_tenant ON findings TO qr_app
    USING (EXISTS (SELECT 1 FROM scans s WHERE s.id = scan_id)); -- inherits the scans policy

-- Privileges. qr_app: what the API needs. qr_worker: queue, emails,
-- verification and retention jobs.
GRANT SELECT, INSERT, UPDATE, DELETE ON users, sessions, auth_tokens, organizations, memberships,
    invitations, domains TO qr_app;
GRANT SELECT, UPDATE ON signup_codes TO qr_app;
GRANT SELECT, INSERT ON audit_log, scans TO qr_app;
GRANT SELECT ON findings TO qr_app;
GRANT INSERT ON email_outbox, deletion_tombstones TO qr_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON scans, findings, email_outbox, domains, sessions, auth_tokens,
    invitations, users, audit_log, deletion_tombstones TO qr_worker;
GRANT SELECT ON organizations, memberships, signup_codes TO qr_worker;
GRANT EXECUTE ON FUNCTION app_org_id(), app_user_id() TO qr_app, qr_worker;

-- +goose Down
DROP POLICY findings_tenant ON findings;
ALTER TABLE findings NO FORCE ROW LEVEL SECURITY;
ALTER TABLE findings DISABLE ROW LEVEL SECURITY;
DROP POLICY scans_tenant ON scans;
ALTER TABLE scans NO FORCE ROW LEVEL SECURITY;
ALTER TABLE scans DISABLE ROW LEVEL SECURITY;
REVOKE ALL ON scans, findings FROM qr_app, qr_worker;
DROP INDEX scans_target_idx;
CREATE INDEX scans_target_idx ON scans (host, port, created_at DESC);
DROP INDEX scans_expires_idx;
DROP INDEX scans_org_idx;
ALTER TABLE scans
    DROP CONSTRAINT scans_organization_fk,
    DROP COLUMN expires_at,
    DROP COLUMN requested_by,
    DROP COLUMN intensity;
-- One statement: the organizations policy depends on memberships.
ALTER TABLE sessions DROP CONSTRAINT sessions_active_org_fk;
DROP TABLE deletion_tombstones, email_outbox, audit_log, domains, invitations, memberships, organizations,
    signup_codes, auth_tokens, sessions, users;
DROP FUNCTION app_user_id();
DROP FUNCTION app_org_id();
