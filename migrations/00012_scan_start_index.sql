-- +goose Up
-- The worker counts the scans started in the last minute before claiming
-- one (global cap, milestone 3.5).
CREATE INDEX scans_started_idx ON scans (started_at) WHERE started_at IS NOT NULL;

-- +goose Down
DROP INDEX scans_started_idx;
