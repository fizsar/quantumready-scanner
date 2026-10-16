-- Accounts, sessions, email tokens, signup codes and the email outbox
-- (milestone 3.2). Run by the API as qr_app (store.Store.Tx), except the
-- outbox claims (qr_mailer) and signup code creation (owner, qradmin).

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = @email;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = @password_hash WHERE id = @id;

-- name: MarkEmailVerified :exec
UPDATE users SET email_verified_at = coalesce(email_verified_at, now()) WHERE id = @id;

-- name: TouchUserLogin :exec
UPDATE users SET last_login_at = now() WHERE id = @id;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, csrf_token, active_org_id, expires_at)
VALUES (@user_id, @token_hash, @csrf_token, sqlc.narg(active_org_id), @expires_at)
RETURNING *;

-- name: GetSessionByToken :one
-- A live session: not expired, and used within the idle timeout.
SELECT s.id, s.user_id, s.csrf_token, s.active_org_id, s.last_seen_at, s.expires_at,
       u.email, u.name, u.lang, u.email_verified_at, u.signup_via
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = @token_hash AND s.expires_at > now()
  AND s.last_seen_at > now() - make_interval(secs => @idle_seconds::float8);

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now() WHERE id = @id;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = @id AND user_id = @user_id;

-- name: DeleteUserSessions :exec
-- All sessions of a user except keep (pass NULL to end them all).
DELETE FROM sessions WHERE user_id = @user_id AND id IS DISTINCT FROM sqlc.narg(keep);

-- name: CreateAuthToken :exec
INSERT INTO auth_tokens (user_id, purpose, token_hash, expires_at)
VALUES (@user_id, @purpose, @token_hash, @expires_at);

-- name: InvalidateAuthTokens :exec
-- Earlier unused tokens of the same purpose stop working when a new one is sent.
UPDATE auth_tokens SET used_at = now() WHERE user_id = @user_id AND purpose = @purpose AND used_at IS NULL;

-- name: ConsumeAuthToken :one
-- Single use: the token is spent even if what follows fails.
UPDATE auth_tokens SET used_at = now()
WHERE token_hash = @token_hash AND purpose = @purpose AND used_at IS NULL AND expires_at > now()
RETURNING user_id;

-- name: CheckSignupCode :one
SELECT id FROM signup_codes WHERE code_hash = @code_hash AND used_at IS NULL AND expires_at > now();

-- name: ConsumeSignupCode :execrows
UPDATE signup_codes SET used_at = now(), used_by = @used_by
WHERE id = @id AND used_at IS NULL AND expires_at > now();

-- name: CreateSignupCode :one
INSERT INTO signup_codes (code_hash, note, expires_at) VALUES (@code_hash, sqlc.narg(note), @expires_at)
RETURNING id, expires_at;

-- name: EnqueueEmail :exec
INSERT INTO email_outbox (to_email, template, lang, subject, body_text)
VALUES (@to_email, @template, @lang, @subject, @body_text);

-- name: ClaimEmails :many
-- Leases up to max_rows due emails to one mailer for lease_seconds.
UPDATE email_outbox
SET attempts = attempts + 1, next_attempt_at = now() + make_interval(secs => @lease_seconds::float8)
WHERE id IN (
    SELECT e.id FROM email_outbox e
    WHERE e.status = 'queued' AND e.next_attempt_at <= now()
    ORDER BY e.id
    LIMIT @max_rows
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: MarkEmailSent :exec
UPDATE email_outbox SET status = 'sent', sent_at = now(), last_error = NULL WHERE id = @id;

-- name: MarkEmailRetry :exec
UPDATE email_outbox
SET next_attempt_at = now() + make_interval(secs => @delay_seconds::float8), last_error = @last_error
WHERE id = @id;

-- name: MarkEmailFailed :exec
UPDATE email_outbox SET status = 'failed', last_error = @last_error WHERE id = @id;

-- name: PurgeEmails :execrows
-- Sent and failed emails carry links with tokens: keep them only briefly.
DELETE FROM email_outbox
WHERE status IN ('sent', 'failed') AND created_at < now() - make_interval(secs => @older_than_seconds::float8);

-- name: MarkSignupViaInvitation :exec
UPDATE users SET signup_via = 'invitation' WHERE id = @id;
