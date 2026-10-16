-- +goose Up
-- Milestone 3.3b: accepting an invitation.
--
-- Whoever holds an invitation link is not a member yet, so row-level
-- security hides the invitation from them. This function finds it by the
-- token's hash only (256-bit, single use) and returns what the invitation
-- page shows. SECURITY DEFINER runs it as the migrations owner; the search
-- path is the one of the migration, so nothing on the caller's path can
-- stand in for these tables.
-- +goose StatementBegin
CREATE FUNCTION invitation_by_token(hash bytea)
RETURNS TABLE (id uuid, organization_id uuid, organization_name text, email text, role text,
               inviter_email text, expires_at timestamptz, accepted_at timestamptz, revoked_at timestamptz)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path FROM CURRENT AS $$
    SELECT i.id, i.organization_id, o.name, i.email, i.role, u.email, i.expires_at, i.accepted_at, i.revoked_at
    FROM invitations i
    JOIN organizations o ON o.id = i.organization_id
    LEFT JOIN users u ON u.id = i.invited_by
    WHERE i.token_hash = hash
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION invitation_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION invitation_by_token(bytea) TO qr_app;

-- +goose Down
DROP FUNCTION invitation_by_token(bytea);
