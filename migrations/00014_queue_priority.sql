-- +goose Up
-- Milestone 3.5e: organizations' scans go first, then public ones, oldest
-- first within each group (ClaimScan orders by organization_id IS NULL,
-- created_at); public queue length and positions count only public rows.
DROP INDEX scans_queued_idx;
CREATE INDEX scans_queued_idx ON scans ((organization_id IS NULL), created_at) WHERE status = 'queued';

-- +goose Down
DROP INDEX scans_queued_idx;
CREATE INDEX scans_queued_idx ON scans (created_at) WHERE status = 'queued';
