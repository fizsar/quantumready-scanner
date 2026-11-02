-- +goose Up
-- Refused public scan requests, counted per day (UTC) and kind (Apertura,
-- decision of 03/11/2026): what qradmin public-report shows before and after
-- opening the scanner. Only the day, the kind and a number: by its columns
-- this table cannot hold an IP, a domain or anything else about who asked
-- (TestScanRefusals checks the columns). Kinds:
--   client_limit  the per-client limit (429 rate_limited)
--   domain_limit  the per-domain limit (429 domain_rate_limited)
--   global_limit  the global limit (429 busy)
--   queue_full    the public queue is full (429 busy)
--   excluded      an excluded or opted-out domain (403 scan_not_allowed)
--   proof_failed  a missing, wrong or reused proof of work (400 proof_failed)
-- The 403 host_not_allowed of the closed scanner is counted by M1 already
-- (usage_daily, scan_blocked_403).
CREATE TABLE scan_refusals_daily (
    day   date   NOT NULL,
    kind  text   NOT NULL CHECK (kind IN ('client_limit', 'domain_limit', 'global_limit',
                                          'queue_full', 'excluded', 'proof_failed')),
    count bigint NOT NULL CHECK (count > 0),
    PRIMARY KEY (day, kind)
);
REVOKE ALL ON scan_refusals_daily FROM PUBLIC;

-- add_scan_refusals adds n refusals of a kind to today's count (UTC, the
-- database's clock: the caller cannot choose the day). qr_app has no
-- privilege on the table itself: it can only add, never read, change or
-- delete. Hardened as purge_expired_public_scans (00019): explicit
-- search_path with the tables' schema written in at migration time.
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION add_scan_refusals(p_kind text, p_n bigint)
        RETURNS void
        LANGUAGE plpgsql VOLATILE SECURITY DEFINER
        SET search_path = pg_catalog, %I, pg_temp
        AS $body$
        BEGIN
            IF p_n IS NULL OR p_n <= 0 THEN
                RAISE EXCEPTION 'add_scan_refusals: the count must be positive, got %%', p_n;
            END IF;
            INSERT INTO scan_refusals_daily (day, kind, count)
            VALUES ((now() AT TIME ZONE 'UTC')::date, p_kind, p_n)
            ON CONFLICT (day, kind) DO UPDATE SET count = scan_refusals_daily.count + EXCLUDED.count;
        END
        $body$
    $f$, current_schema());
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION add_scan_refusals(text, bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION add_scan_refusals(text, bigint) TO qr_app;

-- +goose Down
DROP FUNCTION add_scan_refusals(text, bigint);
DROP TABLE scan_refusals_daily;
