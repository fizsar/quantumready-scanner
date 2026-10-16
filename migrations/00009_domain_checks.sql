-- +goose Up
-- Milestone 3.4: domain verification. The DNS checks run in the worker
-- (it has the network way out); the API only asks for one.
ALTER TABLE domains
    ADD COLUMN check_requested_at timestamptz, -- a member pressed "verify"
    ADD COLUMN check_lease_until  timestamptz, -- a worker is checking it (or backing off after a DNS error)
    ADD COLUMN last_check_error   text;        -- stable code of the last failed check
CREATE INDEX domains_due_idx ON domains (last_checked_at) WHERE status <> 'pending' OR check_requested_at IS NOT NULL;

-- +goose Down
DROP INDEX domains_due_idx;
ALTER TABLE domains DROP COLUMN check_requested_at, DROP COLUMN check_lease_until, DROP COLUMN last_check_error;
