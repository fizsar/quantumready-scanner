-- +goose Up
-- Milestone 3.5: public scanning of any domain.

-- Domains the operator excludes from public scans (on request, abuse…),
-- with their subdomains. Managed with qradmin as the owner; no service role
-- reads it directly.
CREATE TABLE scan_exclusions (
    name       text PRIMARY KEY CHECK (name = lower(name) AND length(name) BETWEEN 1 AND 253),
    reason     text CHECK (length(reason) <= 500),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX domains_block_public_idx ON domains (name) WHERE block_public;

-- public_scan_blocked: is any of names (a host and its parent domains) on
-- the exclusion list, or opted out by an organization that has it verified
-- (lost ones during the 14-day grace)? Only a boolean comes out: a public
-- request cannot learn which organization, nor whether the domain is
-- verified at all, and both reasons look the same. Same hardening as
-- invitation_by_token (migration 00008).
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format($f$
        CREATE FUNCTION public_scan_blocked(names text[])
        RETURNS boolean
        LANGUAGE sql STABLE SECURITY DEFINER
        SET search_path = pg_catalog, %I, pg_temp
        AS $body$
            SELECT cardinality(names) BETWEEN 1 AND 127 AND (
                EXISTS (SELECT 1 FROM scan_exclusions e WHERE e.name = ANY (names))
                OR EXISTS (SELECT 1 FROM domains d
                           WHERE d.block_public AND d.name = ANY (names)
                             AND (d.status = 'verified' OR (d.status = 'lost' AND d.lost_at > now() - interval '14 days'))))
        $body$
    $f$, current_schema());
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION public_scan_blocked(text[]) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION public_scan_blocked(text[]) TO qr_app;

-- +goose Down
DROP FUNCTION public_scan_blocked(text[]);
DROP INDEX domains_block_public_idx;
DROP TABLE scan_exclusions;
