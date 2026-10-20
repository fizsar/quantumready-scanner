-- M1 usage counters (00015, which comes from the showcase branch, where the
-- qr_* roles do not exist): what the API needs as qr_app, and nothing more.
--
-- The API counts events with INSERT … ON CONFLICT DO UPDATE, whose cap on
-- different values per day reads the table (SELECT), and purges the domains
-- of refused scans after 30 days (DELETE, only on blocked_targets_daily).
-- qradmin stats reads them as the database owner, so it needs no grant.
-- qr_worker and qr_mailer get nothing.
--
-- No row-level security: the counters belong to no organization or user (no
-- organization_id, nothing per person), so there is no tenant to isolate.
-- TestRLSCoversEveryOrganizationTable only covers tables with an
-- organization_id; migrations/outoforder_test.go checks these privileges.

-- +goose Up
GRANT SELECT, INSERT, UPDATE ON usage_daily TO qr_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON blocked_targets_daily TO qr_app;

-- +goose Down
REVOKE ALL ON usage_daily, blocked_targets_daily FROM qr_app;
