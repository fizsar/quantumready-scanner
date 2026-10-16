-- +goose Up
-- How each account was created. During the closed beta, accounts that came
-- in through an invitation cannot create organizations of their own
-- (BETA_INVITED_CAN_CREATE_ORGS lifts it).
ALTER TABLE users ADD COLUMN signup_via text NOT NULL DEFAULT 'code' CHECK (signup_via IN ('code', 'invitation'));

-- invitation_by_token, hardened (milestone 3.3 review):
--  * explicit search_path: pg_catalog, the schema of the tables (public in
--    production; written in at migration time so per-test schemas work),
--    pg_temp last (unlisted, it would be searched first);
--  * only pending, unexpired invitations, and only the columns the preview
--    and the acceptance use;
--  * lookup only by a 32-byte token hash (one signature, no overloads);
--  * EXECUTE for qr_app only.
DROP FUNCTION invitation_by_token(bytea);
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION invitation_by_token(hash bytea)
        RETURNS TABLE (id uuid, organization_id uuid, organization_name text, email text, role text,
                       inviter_email text, expires_at timestamptz)
        LANGUAGE sql STABLE SECURITY DEFINER
        SET search_path = pg_catalog, %I, pg_temp
        AS $body$
            SELECT i.id, i.organization_id, o.name, i.email, i.role, u.email, i.expires_at
            FROM invitations i
            JOIN organizations o ON o.id = i.organization_id
            LEFT JOIN users u ON u.id = i.invited_by
            WHERE length(hash) = 32 AND i.token_hash = hash
              AND i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > now()
        $body$
    $f$, current_schema());
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION invitation_by_token(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION invitation_by_token(bytea) TO qr_app;

-- +goose Down
DROP FUNCTION invitation_by_token(bytea);
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
ALTER TABLE users DROP COLUMN signup_via;
