-- name: InsertDemoRequest :one
INSERT INTO demo_requests (name, email, company, domain, message, lang, privacy_version)
VALUES (@name, @email, @company, @domain, @message, @lang, @privacy_version)
RETURNING id, created_at;

-- name: ListDemoRequests :many
-- Newest first, without the message (qradmin demos list).
SELECT id, created_at, name, email, company, domain, lang FROM demo_requests
ORDER BY created_at DESC LIMIT @max_rows;

-- name: GetDemoRequest :one
SELECT * FROM demo_requests WHERE id = @id;

-- name: DeleteDemoRequest :execrows
DELETE FROM demo_requests WHERE id = @id;

-- name: PurgeDemoRequests :execrows
-- Demo requests are kept @keep_months months.
DELETE FROM demo_requests WHERE created_at < now() - make_interval(months => @keep_months::int);
