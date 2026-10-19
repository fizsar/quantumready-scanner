-- name: CountUsage :exec
-- One more of event/dim on day. A day keeps at most @max_dims different dims
-- per event; beyond that they add up under 'other', so the table cannot be
-- flooded with junk values.
INSERT INTO usage_daily (day, event, dim, count)
VALUES (@day, @event,
    CASE WHEN EXISTS (SELECT 1 FROM usage_daily u WHERE u.day = @day AND u.event = @event AND u.dim = @dim)
           OR (SELECT count(*) FROM usage_daily u WHERE u.day = @day AND u.event = @event) < @max_dims::bigint
         THEN @dim::text ELSE 'other' END,
    1)
ON CONFLICT (day, event, dim) DO UPDATE SET count = usage_daily.count + 1;

-- name: CountBlockedTarget :exec
-- One more 403 towards a registrable domain on day, with the same cap.
INSERT INTO blocked_targets_daily (day, domain, count)
VALUES (@day,
    CASE WHEN EXISTS (SELECT 1 FROM blocked_targets_daily b WHERE b.day = @day AND b.domain = @domain)
           OR (SELECT count(*) FROM blocked_targets_daily b WHERE b.day = @day) < @max_domains::bigint
         THEN @domain::text ELSE 'other' END,
    1)
ON CONFLICT (day, domain) DO UPDATE SET count = blocked_targets_daily.count + 1;

-- name: PurgeBlockedTargets :execrows
-- Domains of blocked scan requests are kept @keep_days days.
DELETE FROM blocked_targets_daily WHERE day < current_date - @keep_days::int;

-- name: UsageSince :many
SELECT day, event, dim, count FROM usage_daily WHERE day >= @since ORDER BY day, event, dim;

-- name: BlockedTargetsSince :many
SELECT domain, sum(count)::bigint AS total FROM blocked_targets_daily
WHERE day >= @since GROUP BY domain ORDER BY total DESC, domain;
