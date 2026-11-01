-- +goose Up
-- Public scans are kept 30 days (/privacy, /about-scans; Apertura, decision
-- of 01/11/2026). Those stored before milestone 3.1 got an expiry only if
-- basic (00004): the full ones of the showcase era (production since
-- 15/10/2026) had none. They get one now, 30 days after they were created,
-- so the first purge deletes every public scan older than that.
UPDATE scans SET expires_at = created_at + interval '30 days'
WHERE organization_id IS NULL AND expires_at IS NULL;

-- purge_expired_public_scans deletes the public scans past their expiry and
-- returns how many (their findings go with them: ON DELETE CASCADE). Only
-- finished ones: a scan in the queue is never pulled from under a worker.
-- A row without an expiry (written by an older release during a rollback)
-- counts 30 days from its creation. Hardened as public_scan_blocked: explicit
-- search_path (pg_catalog, the tables' schema written in at migration time,
-- pg_temp last), no arguments, EXECUTE for qr_worker only (the worker runs it
-- once a day). It runs as the migrations' owner, which is the superuser in
-- production; a separate, non-superuser owner (later) needs a policy on scans
-- for itself, or this deletes nothing (TestPurgeExpiredPublicScans).
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION purge_expired_public_scans()
        RETURNS bigint
        LANGUAGE sql VOLATILE SECURITY DEFINER
        SET search_path = pg_catalog, %I, pg_temp
        AS $body$
            WITH gone AS (
                DELETE FROM scans
                WHERE organization_id IS NULL AND status IN ('done', 'failed')
                  AND coalesce(expires_at, created_at + interval '30 days') <= now()
                RETURNING 1)
            SELECT count(*) FROM gone
        $body$
    $f$, current_schema());
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION purge_expired_public_scans() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION purge_expired_public_scans() TO qr_worker;

-- +goose Down
DROP FUNCTION purge_expired_public_scans();
