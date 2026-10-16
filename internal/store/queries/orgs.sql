-- Organization management (milestone 3.3). Always within the active
-- organization (app.org_id): RLS refuses anything else.

-- name: GetOrganization :one
SELECT * FROM organizations WHERE id = @id;

-- name: RenameOrganization :execrows
UPDATE organizations SET name = @name WHERE id = @id;

-- name: ListMembers :many
SELECT m.user_id, u.email, u.name, m.role, m.created_at
FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.organization_id = @organization_id
ORDER BY m.created_at, u.email;

-- name: LockMemberships :many
-- The actor's and the target's memberships, locked until the end of the
-- transaction (in user_id order, so two requests never deadlock).
SELECT * FROM memberships
WHERE organization_id = @organization_id AND user_id = ANY(@user_ids::uuid[])
ORDER BY user_id FOR UPDATE;

-- name: SetMemberRole :execrows
UPDATE memberships SET role = @role WHERE organization_id = @organization_id AND user_id = @user_id;

-- name: RemoveMember :execrows
DELETE FROM memberships WHERE organization_id = @organization_id AND user_id = @user_id;

-- name: SetActiveOrganization :exec
UPDATE sessions SET active_org_id = sqlc.narg(organization_id) WHERE id = @id AND user_id = @user_id;
