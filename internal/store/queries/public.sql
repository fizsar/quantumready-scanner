-- Public scanning (milestone 3.5).

-- name: SetDomainBlockPublic :one
-- The owner's opt-out from public scans. Only a verified domain can change
-- it (the caller checks Verified); once set it stays in force regardless.
UPDATE domains SET block_public = @block_public
WHERE organization_id = @organization_id AND id = @id
RETURNING *;

-- name: KeepOptOutAsExclusion :exec
-- A blocked domain removed from an organization keeps its block (run after
-- the delete). qr_app cannot read scan_exclusions, so instead of ON
-- CONFLICT it asks public_scan_blocked whether the name is still covered.
INSERT INTO scan_exclusions (name, reason)
SELECT @name::text, @reason::text WHERE NOT public_scan_blocked(ARRAY[@name::text]);

-- name: ListOptOuts :many
-- Owner opt-outs (operator view, qradmin): every organization.
SELECT name, organization_id, status FROM domains WHERE block_public ORDER BY name;

-- name: ClearOptOut :execrows
UPDATE domains SET block_public = false WHERE name = @name AND block_public;

-- name: AddScanExclusion :exec
INSERT INTO scan_exclusions (name, reason) VALUES (@name, sqlc.narg(reason))
ON CONFLICT (name) DO UPDATE SET reason = excluded.reason;

-- name: RemoveScanExclusion :execrows
DELETE FROM scan_exclusions WHERE name = @name;

-- name: ListScanExclusions :many
SELECT * FROM scan_exclusions ORDER BY name;
