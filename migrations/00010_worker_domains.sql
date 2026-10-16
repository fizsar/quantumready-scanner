-- +goose Up
-- Milestone 3.4: the worker checks domain TXT records (first verification
-- and the weekly re-verification). It gets exactly the columns that needs,
-- column by column: not organization_id, block_public or created_at, and it
-- can neither add nor delete domains.
GRANT SELECT (id, name, token, status, verified_at, last_checked_at, lost_at, check_requested_at, check_lease_until)
    ON domains TO qr_worker;
GRANT UPDATE (status, verified_at, last_checked_at, lost_at, check_requested_at, check_lease_until, last_check_error)
    ON domains TO qr_worker;
CREATE POLICY domains_worker ON domains FOR SELECT TO qr_worker USING (true);
CREATE POLICY domains_worker_update ON domains FOR UPDATE TO qr_worker USING (true) WITH CHECK (true);

-- +goose Down
DROP POLICY domains_worker_update ON domains;
DROP POLICY domains_worker ON domains;
REVOKE UPDATE (status, verified_at, last_checked_at, lost_at, check_requested_at, check_lease_until, last_check_error)
    ON domains FROM qr_worker;
REVOKE SELECT (id, name, token, status, verified_at, last_checked_at, lost_at, check_requested_at, check_lease_until)
    ON domains FROM qr_worker;
