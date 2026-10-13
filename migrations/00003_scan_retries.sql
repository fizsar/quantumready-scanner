-- Retries for transient scanner errors (timeout, connection reset…): the scan
-- goes back to the queue once, not before not_before. Definitive errors
-- (NXDOMAIN, connection refused, anti-SSRF block…) fail directly.

-- +goose Up
ALTER TABLE scans
    ADD COLUMN retries    integer NOT NULL DEFAULT 0,
    ADD COLUMN not_before timestamptz;

-- +goose Down
ALTER TABLE scans
    DROP COLUMN not_before,
    DROP COLUMN retries;
