-- Least privilege for the background roles (milestone 3.2).
--
-- qr_worker scans: it reads and updates the scan queue and stores findings,
-- for every organization, and nothing else. No BYPASSRLS: explicit policies
-- on the two tables it needs. No access to users, sessions, auth tokens,
-- organizations, domains (granted in 3.4 for re-verification), the audit log
-- or the email outbox.
--
-- qr_mailer sends email: it reads, updates and purges email_outbox rows,
-- which carry the recipient, subject and body already rendered, and nothing
-- else. Sending runs in its own process (cmd/mailer) because the worker
-- relaxes crypto/tls for scanning (GODEBUG tlssha1=1, rsa1024min=0), which
-- must not weaken the SMTP connection.

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'qr_mailer') THEN
        CREATE ROLE qr_mailer NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
    END IF;
    ALTER ROLE qr_worker NOBYPASSRLS;
    EXECUTE format('GRANT qr_mailer TO %I', current_user);
    EXECUTE format('GRANT USAGE ON SCHEMA %I TO qr_mailer', current_schema());
END
$$;
-- +goose StatementEnd

REVOKE ALL ON users, sessions, auth_tokens, signup_codes, organizations, memberships, invitations,
    domains, audit_log, email_outbox, deletion_tombstones, scans, findings FROM qr_worker;
GRANT SELECT, UPDATE ON scans TO qr_worker;
GRANT INSERT ON findings TO qr_worker;
CREATE POLICY scans_worker ON scans FOR SELECT TO qr_worker USING (true);
CREATE POLICY scans_worker_update ON scans FOR UPDATE TO qr_worker USING (true) WITH CHECK (true);
CREATE POLICY findings_worker ON findings FOR INSERT TO qr_worker WITH CHECK (true);

-- The outbox holds each email fully rendered (the mailer reads nothing else).
ALTER TABLE email_outbox
    DROP COLUMN params,
    ADD COLUMN subject    text NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
    ADD COLUMN body_text  text NOT NULL CHECK (length(body_text) BETWEEN 1 AND 20000),
    ADD COLUMN last_error text;
GRANT SELECT, UPDATE, DELETE ON email_outbox TO qr_mailer;
GRANT SELECT ON email_outbox TO qr_app; -- RETURNING id when enqueuing

-- +goose Down
REVOKE ALL ON email_outbox FROM qr_mailer;
REVOKE SELECT ON email_outbox FROM qr_app;
ALTER TABLE email_outbox
    DROP COLUMN last_error,
    DROP COLUMN body_text,
    DROP COLUMN subject,
    ADD COLUMN params jsonb NOT NULL DEFAULT '{}';
DROP POLICY findings_worker ON findings;
DROP POLICY scans_worker_update ON scans;
DROP POLICY scans_worker ON scans;
-- Back to the 00004 privileges.
GRANT SELECT, INSERT, UPDATE, DELETE ON scans, findings, email_outbox, domains, sessions, auth_tokens,
    invitations, users, audit_log, deletion_tombstones TO qr_worker;
GRANT SELECT ON organizations, memberships, signup_codes TO qr_worker;
-- The role keeps NOBYPASSRLS: role attributes are cluster-wide and other
-- schemas (tests run in parallel) rely on it.
