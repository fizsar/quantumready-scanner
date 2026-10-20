-- +goose Up
-- M1 usage metrics: daily aggregate counters only. No IPs, user agents,
-- times of day or identifiers; nothing ties one row to another or to a person.
--
-- Number 00015 is reserved for M1 and identical in showcase and main. In
-- production (deployed from showcase) it runs after 00003; when phase 3
-- (00004–00014) is deployed later, goose applies those after this one
-- (migrations.Up allows exactly that gap). So this file must not depend on
-- anything from phase 3: no foreign keys, no roles, no RLS. The phase 3
-- roles get their privileges in 00016 (main only).

CREATE TABLE usage_daily (
    day   date   NOT NULL,
    event text   NOT NULL CHECK (event IN ('page_view', 'visit_source', 'scan_started', 'scan_blocked_403',
                                           'under_the_hood_open', 'json_download', 'lang_switch')),
    dim   text   NOT NULL DEFAULT '' CHECK (length(dim) <= 64),
    count bigint NOT NULL DEFAULT 0 CHECK (count >= 0),
    PRIMARY KEY (day, event, dim)
);

-- Registrable domains that scan requests outside the allowlist pointed at
-- (403 host_not_allowed), per day. Kept 30 days (the API deletes older rows)
-- and excluded from backups (deploy/backup).
CREATE TABLE blocked_targets_daily (
    day    date   NOT NULL,
    domain text   NOT NULL CHECK (length(domain) BETWEEN 1 AND 253),
    count  bigint NOT NULL DEFAULT 0 CHECK (count >= 0),
    PRIMARY KEY (day, domain)
);

-- +goose Down
DROP TABLE blocked_targets_daily;
DROP TABLE usage_daily;
