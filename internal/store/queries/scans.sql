-- name: EnqueueScan :one
INSERT INTO scans (host, port, organization_id, intensity, requested_by, expires_at)
VALUES (@host, @port, sqlc.narg(organization_id), @intensity, sqlc.narg(requested_by), sqlc.narg(expires_at))
RETURNING *;

-- name: ClaimScan :one
-- Leases the oldest queued scan that is due (not_before passed), or a running
-- one whose lease expired (its worker died), to the caller. SKIP LOCKED lets
-- workers claim concurrently without waiting on each other or taking the same row.
UPDATE scans
SET status      = 'running',
    attempts    = attempts + 1,
    lease_token = @lease_token,
    lease_until = now() + make_interval(secs => @lease_seconds::float8),
    started_at  = now()
WHERE id = (
    SELECT s.id FROM scans s
    WHERE ((s.status = 'queued' AND (s.not_before IS NULL OR s.not_before <= now()))
           OR (s.status = 'running' AND s.lease_until < now()))
      AND s.attempts < s.max_attempts
      -- Good Internet citizen: at most @max_per_minute scans start per minute
      -- across all workers (0: no cap), so the server never looks like a sweep.
      AND (@max_per_minute::int = 0
           OR (SELECT count(*) FROM scans x WHERE x.started_at > now() - interval '1 minute') < @max_per_minute::int)
    ORDER BY s.created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: FailExpiredScans :many
-- Running scans whose lease expired with no attempts left: give up.
UPDATE scans
SET status = 'failed', error = 'lease_expired', finished_at = now(),
    lease_token = NULL, lease_until = NULL
WHERE status = 'running' AND lease_until < now() AND attempts >= max_attempts
RETURNING id;

-- name: CompleteScan :execrows
-- Only the current lease holder can complete a scan.
UPDATE scans
SET status = 'done', finished_at = now(), error = NULL,
    lease_token = NULL, lease_until = NULL,
    result = @result, scanner_schema = @scanner_schema,
    scoring = @scoring, scoring_version = @scoring_version,
    score = @score, grade = @grade, pqc = @pqc
WHERE id = @id AND status = 'running' AND lease_token = @lease_token;

-- name: FailScan :execrows
-- Only the current lease holder can fail a scan.
UPDATE scans
SET status = 'failed', finished_at = now(), error = @error,
    lease_token = NULL, lease_until = NULL
WHERE id = @id AND status = 'running' AND lease_token = @lease_token;

-- name: RetryScan :execrows
-- Back to the queue after a transient error, not before the delay. Only the
-- current lease holder can do it. error keeps the transient reason meanwhile.
UPDATE scans
SET status = 'queued', retries = retries + 1, error = @error,
    not_before = now() + make_interval(secs => @delay_seconds::float8),
    lease_token = NULL, lease_until = NULL
WHERE id = @id AND status = 'running' AND lease_token = @lease_token;

-- name: InsertFindingsBatch :exec
-- One statement for all the findings of a scan. Not COPY: PostgreSQL refuses
-- COPY FROM for roles under row-level security (the worker, qr_worker).
INSERT INTO findings (scan_id, position, code, axis, severity, category, params)
SELECT unnest(@scan_ids::uuid[]), unnest(@positions::int4[]), unnest(@codes::text[]), unnest(@axes::text[]),
       unnest(@severities::text[]), unnest(@categories::text[]), unnest(@params::jsonb[]);

-- name: LockTarget :exec
-- Serializes enqueues of the same target until the transaction ends.
SELECT pg_advisory_xact_lock(hashtextextended(@target::text, 0));

-- name: RecentScanForTarget :one
-- The latest scan of a target created within the cooldown that has not
-- failed, with the same intensity and the same owner: a basic scan never
-- answers a full request, and an organization never gets another one's scan
-- (public scans have no organization).
SELECT * FROM scans
WHERE host = @host AND port = @port AND status <> 'failed'
  AND intensity = @intensity AND organization_id IS NOT DISTINCT FROM sqlc.narg(organization_id)
  AND created_at > now() - make_interval(secs => @within_seconds::float8)
ORDER BY created_at DESC
LIMIT 1;

-- name: GetScan :one
SELECT * FROM scans WHERE id = @id;

-- name: ListFindings :many
SELECT * FROM findings WHERE scan_id = @scan_id ORDER BY position;
