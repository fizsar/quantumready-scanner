-- +goose Up
-- ADR-0007 (decided 24/10/2026, Apertura): a finished scan is history and
-- stays as it was. The worker may update a scan only while it is queued or
-- running (claim, heartbeat, complete, fail, requeue, queue timeout); a done
-- or failed one is out of its reach. WITH CHECK stays true: completing moves
-- a running scan to done.
ALTER POLICY scans_worker_update ON scans USING (status IN ('queued', 'running'));

-- +goose Down
ALTER POLICY scans_worker_update ON scans USING (true);
