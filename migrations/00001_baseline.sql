-- Baseline: starts schema versioning. The data model (organizations, users,
-- domains, scans, ...) arrives with phases 2 and 3.

-- +goose Up
SELECT 1;

-- +goose Down
SELECT 1;
