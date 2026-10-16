-- +goose Up
-- Milestone 3.3: organization members and invitations.

-- Every organization keeps at least one owner, enforced here for every
-- path (the store turns the error into 409 last_owner). It locks the
-- organization row, so two owners demoting each other at the same time are
-- serialized and the second one fails instead of leaving no owner.
-- +goose StatementBegin
CREATE FUNCTION memberships_keep_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.role <> 'owner' OR (TG_OP = 'UPDATE' AND NEW.role = 'owner') THEN
        RETURN NULL;
    END IF;
    PERFORM 1 FROM organizations WHERE id = OLD.organization_id FOR UPDATE;
    IF NOT FOUND THEN
        RETURN NULL; -- the organization itself is being deleted
    END IF;
    IF NOT EXISTS (SELECT 1 FROM memberships WHERE organization_id = OLD.organization_id AND role = 'owner') THEN
        RAISE EXCEPTION 'an organization needs at least one owner'
            USING ERRCODE = 'check_violation', CONSTRAINT = 'memberships_last_owner';
    END IF;
    RETURN NULL;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER memberships_keep_owner AFTER UPDATE OF role OR DELETE ON memberships
    FOR EACH ROW EXECUTE FUNCTION memberships_keep_owner();

-- Invitations: revocation, who accepted, one pending invitation per address.
ALTER TABLE invitations
    ADD COLUMN revoked_at  timestamptz,
    ADD COLUMN accepted_by uuid REFERENCES users (id) ON DELETE SET NULL;
CREATE UNIQUE INDEX invitations_pending_idx ON invitations (organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

-- +goose Down
DROP INDEX invitations_pending_idx;
ALTER TABLE invitations DROP COLUMN accepted_by, DROP COLUMN revoked_at;
DROP TRIGGER memberships_keep_owner ON memberships;
DROP FUNCTION memberships_keep_owner();
