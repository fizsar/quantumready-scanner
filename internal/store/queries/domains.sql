-- Domains of the active organization (milestone 3.4). CreateDomain,
-- ListDomains and GetDomain are in tenancy.sql.

-- name: CountDomains :one
SELECT count(*) FROM domains WHERE organization_id = @organization_id;

-- name: DeleteDomain :execrows
DELETE FROM domains WHERE organization_id = @organization_id AND id = @id;

-- name: RequestDomainCheck :one
UPDATE domains SET check_requested_at = now()
WHERE organization_id = @organization_id AND id = @id
RETURNING *;
