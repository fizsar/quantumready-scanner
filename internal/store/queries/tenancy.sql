-- Accounts, organizations and domains (phase 3). Every query on organization
-- data takes organization_id explicitly (first layer of isolation); RLS is
-- the second (migrations/00004_accounts.sql, store.Store.Tx).

-- name: CreateUser :one
INSERT INTO users (email, password_hash, name, lang)
VALUES (@email, @password_hash, sqlc.narg(name), @lang)
RETURNING *;

-- name: CreateOrganization :one
INSERT INTO organizations (name) VALUES (@name) RETURNING *;

-- name: AddMembership :exec
INSERT INTO memberships (organization_id, user_id, role) VALUES (@organization_id, @user_id, @role);

-- name: GetMembership :one
SELECT * FROM memberships WHERE organization_id = @organization_id AND user_id = @user_id;

-- name: ListUserOrganizations :many
-- The organizations a user belongs to, with their role (organization switcher).
SELECT o.id, o.name, o.created_at, m.role
FROM organizations o JOIN memberships m ON m.organization_id = o.id
WHERE m.user_id = @user_id
ORDER BY o.created_at;

-- name: CreateDomain :one
INSERT INTO domains (organization_id, name, token) VALUES (@organization_id, @name, @token) RETURNING *;

-- name: ListDomains :many
SELECT * FROM domains WHERE organization_id = @organization_id ORDER BY name;

-- name: GetDomain :one
SELECT * FROM domains WHERE organization_id = @organization_id AND id = @id;

-- name: ListOrganizationScans :many
SELECT * FROM scans WHERE organization_id = @organization_id ORDER BY created_at DESC LIMIT @max_rows;

-- name: GetScanForTenant :one
-- A public scan (no organization), or one of the given organization.
SELECT * FROM scans
WHERE id = @id AND (organization_id IS NULL OR organization_id = sqlc.narg(organization_id));

-- name: InsertAudit :exec
INSERT INTO audit_log (organization_id, user_id, action, target, ip_prefix)
VALUES (sqlc.narg(organization_id), sqlc.narg(user_id), @action, sqlc.narg(target), sqlc.narg(ip_prefix));
