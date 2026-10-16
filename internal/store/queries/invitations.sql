-- Invitations (milestone 3.3b), inside the active organization (RLS).
-- Finding one by its token goes through invitation_by_token (store code).

-- name: IsMemberByEmail :one
SELECT EXISTS (
    SELECT 1 FROM memberships m JOIN users u ON u.id = m.user_id
    WHERE m.organization_id = @organization_id AND u.email = @email
);

-- name: RevokePendingInvitationsFor :exec
-- A new invitation to the same address replaces the pending one.
UPDATE invitations SET revoked_at = now()
WHERE organization_id = @organization_id AND email = @email AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: CreateInvitation :one
INSERT INTO invitations (organization_id, email, role, token_hash, invited_by, expires_at)
VALUES (@organization_id, @email, @role, @token_hash, @invited_by, @expires_at)
RETURNING id, email, role, created_at, expires_at;

-- name: ListPendingInvitations :many
SELECT i.id, i.email, i.role, i.created_at, i.expires_at, u.email AS inviter_email
FROM invitations i LEFT JOIN users u ON u.id = i.invited_by
WHERE i.organization_id = @organization_id AND i.accepted_at IS NULL AND i.revoked_at IS NULL
ORDER BY i.created_at DESC;

-- name: RevokeInvitation :execrows
UPDATE invitations SET revoked_at = now()
WHERE organization_id = @organization_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: MarkInvitationAccepted :execrows
-- Single use: only a pending, unexpired invitation can be accepted.
UPDATE invitations SET accepted_at = now(), accepted_by = @accepted_by
WHERE id = @id AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now();
