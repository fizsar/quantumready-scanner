-- Scans and their findings. The scans table is also the job queue: workers
-- claim rows with FOR UPDATE SKIP LOCKED and hold them with a lease
-- (lease_token + lease_until); a scan whose lease expires is claimed again.

-- +goose Up
CREATE TABLE scans (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Owner, from phase 3 (foreign key to organizations added then).
    organization_id uuid,
    host            text NOT NULL CHECK (length(host) BETWEEN 1 AND 253),
    port            integer NOT NULL DEFAULT 443 CHECK (port BETWEEN 1 AND 65535),

    status          text NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts        integer NOT NULL DEFAULT 0,
    max_attempts    integer NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    lease_token     uuid,        -- set by the claim; changes on every claim
    lease_until     timestamptz, -- the claim expires here unless completed

    created_at      timestamptz NOT NULL DEFAULT now(),
    started_at      timestamptz, -- last claim
    finished_at     timestamptz,
    -- Failure reason as a stable code (blocked_address, unreachable,
    -- lease_expired…), never a raw error message.
    error           text,

    -- Results (status = 'done').
    result          jsonb,       -- scanner.Result
    scanner_schema  integer,     -- scanner.SchemaVersion of result
    scoring         jsonb,       -- scoring.Result without findings (blocks, cap, risks…)
    scoring_version text,        -- scoring.Version used
    score           integer CHECK (score BETWEEN 0 AND 100),
    grade           text CHECK (grade IN ('A', 'B', 'C', 'D', 'F')),
    pqc             boolean,

    CONSTRAINT scans_lease_consistent
        CHECK ((status = 'running') = (lease_token IS NOT NULL AND lease_until IS NOT NULL)),
    CONSTRAINT scans_done_has_result
        CHECK (status <> 'done' OR (result IS NOT NULL AND scoring_version IS NOT NULL AND grade IS NOT NULL))
);

-- Queue: oldest queued first; expired leases.
CREATE INDEX scans_queued_idx ON scans (created_at) WHERE status = 'queued';
CREATE INDEX scans_running_lease_idx ON scans (lease_until) WHERE status = 'running';
-- Latest scan of a target (API cooldown, history).
CREATE INDEX scans_target_idx ON scans (host, port, created_at DESC);

CREATE TABLE findings (
    id       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    scan_id  uuid NOT NULL REFERENCES scans (id) ON DELETE CASCADE,
    position integer NOT NULL, -- display order (severity first)
    code     text NOT NULL,    -- texts come from the scoring catalog
    axis     text NOT NULL CHECK (axis IN ('quantum', 'current')),
    severity text NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low', 'info', 'pass')),
    category text NOT NULL,
    params   jsonb NOT NULL DEFAULT '{}',
    UNIQUE (scan_id, position)
);

-- +goose Down
DROP TABLE findings;
DROP TABLE scans;
