-- Read by qradmin public-report as the database owner: what public scanning
-- did, day by day (UTC). Nothing here identifies who asked for a scan: the
-- scans table keeps no IP, account or client of public scans.

-- name: PublicScansByDay :many
SELECT (created_at AT TIME ZONE 'UTC')::date AS day,
       count(*)::bigint AS scans,
       count(*) FILTER (WHERE status = 'done')::bigint AS done,
       count(*) FILTER (WHERE status = 'failed')::bigint AS failed,
       count(*) FILTER (WHERE error = 'queue_timeout')::bigint AS queue_timeouts,
       count(*) FILTER (WHERE intensity = 'full')::bigint AS full_scans
FROM scans
WHERE organization_id IS NULL AND created_at >= sqlc.arg(since)::timestamptz
GROUP BY 1 ORDER BY 1;

-- name: PublicScanHostsByDay :many
SELECT (created_at AT TIME ZONE 'UTC')::date AS day, host, count(*)::bigint AS scans
FROM scans
WHERE organization_id IS NULL AND created_at >= sqlc.arg(since)::timestamptz
GROUP BY 1, 2 ORDER BY 1, 2;

-- name: PublicQueueSpans :many
-- When each public scan entered the queue and when it left it: its last
-- claim (started_at; a retried scan counts from its first enqueue to its
-- last claim, so the queue is over- rather than under-estimated), its end
-- if it never started (queue_timeout), or now if it still waits. Scans
-- created up to a day before @since count, since they may still wait then.
SELECT created_at,
       coalesce(started_at, finished_at, now())::timestamptz AS left_queue
FROM scans
WHERE organization_id IS NULL AND created_at >= sqlc.arg(since)::timestamptz - interval '1 day'
ORDER BY created_at;

-- name: ExclusionsAddedByDay :many
SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*)::bigint AS added
FROM scan_exclusions
WHERE created_at >= sqlc.arg(since)::timestamptz
GROUP BY 1 ORDER BY 1;

-- name: CountExclusions :one
SELECT count(*)::bigint FROM scan_exclusions;

-- name: RefusalsByDay :many
-- Refused public scan requests by kind (migration 00022).
SELECT day, kind, count FROM scan_refusals_daily
WHERE day >= sqlc.arg(since)::date ORDER BY day, kind;

-- name: ClosedRefusalsByDay :many
-- The closed scanner's 403 host_not_allowed: the M1 counter (bots excluded).
SELECT day, sum(count)::bigint AS refused FROM usage_daily
WHERE event = 'scan_blocked_403' AND day >= sqlc.arg(since)::date
GROUP BY day ORDER BY day;
