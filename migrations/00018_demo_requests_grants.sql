-- "Request a free demo" (00017, which comes from the showcase branch, where
-- the qr_* roles do not exist): what the API needs as qr_app, and nothing
-- more. It stores requests (INSERT … RETURNING id, created_at) and purges
-- them after 12 months (DELETE … WHERE created_at < …): SELECT only on those
-- two columns, so qr_app can never read a name, an email or a message.
-- qradmin demos reads them as the database owner. qr_worker and qr_mailer
-- get nothing. No row-level security: the requests belong to no
-- organization; migrations/outoforder_test.go checks these privileges.
--
-- The form is off by default (DEMO_ENABLED=false), but the purge runs anyway.

-- +goose Up
GRANT INSERT, DELETE ON demo_requests TO qr_app;
GRANT SELECT (id, created_at) ON demo_requests TO qr_app;

-- +goose Down
REVOKE ALL ON demo_requests FROM qr_app;
