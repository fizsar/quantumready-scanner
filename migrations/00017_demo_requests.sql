-- "Request a free demo" (sprint 1): what visitors send from /demo, kept 12
-- months (cmd/api purges daily; /privacy says so) and readable with
-- `qradmin demos`. Unlike the M1 counters, these rows are in the backups.
-- The M1 event list gains demo_requested (the form's metric: where the
-- button was, never what was written).
--
-- Comes from the showcase branch, like 00015: no qr_* roles here. The grants
-- for qr_app go in 00018, only on main.

-- +goose Up
CREATE TABLE demo_requests (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    name            text        NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    email           text        NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
    company         text        NOT NULL CHECK (length(company) BETWEEN 1 AND 100),
    domain          text        NOT NULL CHECK (length(domain) BETWEEN 1 AND 253),
    message         text        NOT NULL DEFAULT '' CHECK (length(message) <= 2000),
    lang            text        NOT NULL CHECK (lang IN ('es', 'en')),
    -- Which version of /privacy the sender accepted (the box is mandatory).
    privacy_version text        NOT NULL CHECK (length(privacy_version) BETWEEN 1 AND 20)
);
CREATE INDEX demo_requests_created_at ON demo_requests (created_at);

ALTER TABLE usage_daily DROP CONSTRAINT usage_daily_event_check;
ALTER TABLE usage_daily ADD CONSTRAINT usage_daily_event_check CHECK (event IN ('page_view', 'visit_source',
    'scan_started', 'scan_blocked_403', 'under_the_hood_open', 'json_download', 'lang_switch', 'demo_requested'));

-- +goose Down
DELETE FROM usage_daily WHERE event = 'demo_requested';
ALTER TABLE usage_daily DROP CONSTRAINT usage_daily_event_check;
ALTER TABLE usage_daily ADD CONSTRAINT usage_daily_event_check CHECK (event IN ('page_view', 'visit_source',
    'scan_started', 'scan_blocked_403', 'under_the_hood_open', 'json_download', 'lang_switch'));
DROP TABLE demo_requests;
