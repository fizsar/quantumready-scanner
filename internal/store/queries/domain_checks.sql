-- Domain checks, run by the worker as qr_worker (column grants in
-- migrations/00010_worker_domains.sql): only verification columns.

-- name: ClaimDomainChecks :many
-- Domains to check now: asked for by a member, verified ones weekly, lost
-- ones daily. The lease keeps two workers off the same domain and, after a
-- DNS error, delays the next try.
UPDATE domains SET check_lease_until = now() + make_interval(secs => @lease_seconds::float8)
WHERE id IN (
    SELECT d.id FROM domains d
    WHERE (d.check_lease_until IS NULL OR d.check_lease_until < now())
      AND (d.check_requested_at IS NOT NULL
           OR (d.status = 'verified' AND (d.last_checked_at IS NULL OR d.last_checked_at < now() - make_interval(secs => @recheck_seconds::float8)))
           OR (d.status = 'lost' AND (d.last_checked_at IS NULL OR d.last_checked_at < now() - make_interval(secs => @lost_recheck_seconds::float8))))
    ORDER BY d.check_requested_at NULLS LAST, d.last_checked_at NULLS FIRST
    LIMIT @max_rows
    FOR UPDATE SKIP LOCKED)
RETURNING id, name, token, status;

-- name: MarkDomainFound :exec
UPDATE domains SET
    status = 'verified',
    verified_at = CASE WHEN status = 'pending' THEN now() ELSE coalesce(verified_at, now()) END,
    lost_at = NULL, last_checked_at = now(), last_check_error = NULL, check_requested_at = NULL, check_lease_until = NULL
WHERE id = @id;

-- name: MarkDomainAbsent :exec
-- The record is gone: verified → lost (grace starts); lost past the grace
-- period → pending (no longer verified; the history stays).
UPDATE domains SET
    status = CASE WHEN status = 'verified' THEN 'lost'
                  WHEN status = 'lost' AND lost_at < now() - make_interval(secs => @grace_seconds::float8) THEN 'pending'
                  ELSE status END,
    lost_at = CASE WHEN status = 'verified' THEN now()
                   WHEN status = 'lost' AND lost_at < now() - make_interval(secs => @grace_seconds::float8) THEN NULL
                   ELSE lost_at END,
    verified_at = CASE WHEN status = 'lost' AND lost_at < now() - make_interval(secs => @grace_seconds::float8) THEN NULL
                       ELSE verified_at END,
    last_checked_at = now(), last_check_error = 'dns_record_missing', check_requested_at = NULL, check_lease_until = NULL
WHERE id = @id;

-- name: MarkDomainCheckError :exec
-- DNS did not answer: nothing changes but the error and a back-off.
UPDATE domains SET last_check_error = @error, check_requested_at = NULL,
    check_lease_until = now() + make_interval(secs => @backoff_seconds::float8)
WHERE id = @id;
