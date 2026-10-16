-- +goose Up
-- The owner's opt-out from public scans stays in force even when the domain
-- is no longer verified (grace over, record removed, anything): when in
-- doubt, an opt-out stops scanning. Setting or lifting it still needs the
-- domain verified (API), so nobody can block a domain they do not control;
-- the operator can lift it with qradmin optout clear.
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format($f$
        CREATE OR REPLACE FUNCTION public_scan_blocked(names text[])
        RETURNS boolean
        LANGUAGE sql STABLE SECURITY DEFINER
        SET search_path = pg_catalog, %I, pg_temp
        AS $body$
            SELECT cardinality(names) BETWEEN 1 AND 127 AND (
                EXISTS (SELECT 1 FROM scan_exclusions e WHERE e.name = ANY (names))
                OR EXISTS (SELECT 1 FROM domains d WHERE d.block_public AND d.name = ANY (names)))
        $body$
    $f$, current_schema());
END
$$;
-- +goose StatementEnd

-- Removing a blocked domain from an organization keeps the block, as an
-- operator exclusion: qr_app may add to the list (never read it).
GRANT INSERT ON scan_exclusions TO qr_app;

-- +goose Down
REVOKE INSERT ON scan_exclusions FROM qr_app;
-- +goose StatementBegin
DO $$
BEGIN
    EXECUTE format($f$
        CREATE OR REPLACE FUNCTION public_scan_blocked(names text[])
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
