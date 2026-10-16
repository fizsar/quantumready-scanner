-- Public scanning (milestone 3.5).

-- name: SetDomainBlockPublic :one
-- The verified owner's opt-out from public scans (it only takes effect while
-- the domain is verified: public_scan_blocked checks).
UPDATE domains SET block_public = @block_public
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: AddScanExclusion :exec
INSERT INTO scan_exclusions (name, reason) VALUES (@name, sqlc.narg(reason))
ON CONFLICT (name) DO UPDATE SET reason = excluded.reason;

-- name: RemoveScanExclusion :execrows
DELETE FROM scan_exclusions WHERE name = @name;

-- name: ListScanExclusions :many
SELECT * FROM scan_exclusions ORDER BY name;
