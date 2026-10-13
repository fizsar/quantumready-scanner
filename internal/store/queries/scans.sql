-- name: EnqueueScan :one
INSERT INTO scans (host, port, organization_id)
VALUES (@host, @port, @organization_id)
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

-- name: InsertFindings :copyfrom
INSERT INTO findings (scan_id, position, code, axis, severity, category, params)
VALUES (@scan_id, @position, @code, @axis, @severity, @category, @params);

-- name: LockTarget :exec
-- Serializes enqueues of the same target until the transaction ends.
SELECT pg_advisory_xact_lock(hashtextextended(@target::text, 0));

-- name: RecentScanForTarget :one
-- The latest scan of a target created within the cooldown that has not failed.
SELECT * FROM scans
WHERE host = @host AND port = @port AND status <> 'failed'
  AND created_at > now() - make_interval(secs => @within_seconds::float8)
ORDER BY created_at DESC
LIMIT 1;

-- name: GetScan :one
SELECT * FROM scans WHERE id = @id;

-- name: ListFindings :many
SELECT * FROM findings WHERE scan_id = @scan_id ORDER BY position;
