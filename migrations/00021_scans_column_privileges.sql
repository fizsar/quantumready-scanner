-- +goose Up
-- Column privileges on scans (security review of 01/11/2026; only in main,
-- where the roles exist). Defence in depth against a compromised API or
-- worker, on top of the application and row-level security:
--
-- - qr_app (API) inserts only the request: target, owner, intensity and who
--   asked. status ('queued'), created_at, id and the results take their
--   defaults; it can no longer write a finished scan with a made-up result,
--   nor an expiry in the far future that the purge would never reach.
-- - expires_at is set here, from created_at: 30 days for a public scan, 13
--   months (395 days) for an organization's (store.PublicScanTTL and
--   store.OrgScanTTL). A role allowed to write it (the owner: tests, qradmin)
--   may still give one; qr_app cannot. A release without this migration
--   (showcase, during a rollback) inserts as the owner without an expiry and
--   gets one too.
-- - qr_worker updates only the columns of its work (claim, heartbeat,
--   complete, fail, retry, queue timeout), never the target, the owner, the
--   intensity, who asked, the attempts allowed or the expiry.
REVOKE INSERT ON scans FROM qr_app;
GRANT INSERT (host, port, organization_id, intensity, requested_by) ON scans TO qr_app;

REVOKE UPDATE ON scans FROM qr_worker;
GRANT UPDATE (status, attempts, lease_token, lease_until, started_at, finished_at, error,
              result, scanner_schema, scoring, scoring_version, score, grade, pqc,
              retries, not_before) ON scans TO qr_worker;

-- +goose StatementBegin
CREATE FUNCTION scans_set_expiry() RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, pg_temp
AS $$
BEGIN
    IF NEW.expires_at IS NULL THEN
        NEW.expires_at := NEW.created_at + CASE WHEN NEW.organization_id IS NULL
                                                THEN interval '30 days' ELSE interval '395 days' END;
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION scans_set_expiry() FROM PUBLIC;
CREATE TRIGGER scans_set_expiry BEFORE INSERT ON scans
    FOR EACH ROW EXECUTE FUNCTION scans_set_expiry();

-- +goose Down
DROP TRIGGER scans_set_expiry ON scans;
DROP FUNCTION scans_set_expiry();
REVOKE UPDATE ON scans FROM qr_worker;
GRANT UPDATE ON scans TO qr_worker;
REVOKE INSERT ON scans FROM qr_app;
GRANT INSERT ON scans TO qr_app;
