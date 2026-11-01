package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Login users of the services (Apertura step 4, decision of 01/11/2026). The
// API and the worker log in as qr_app and qr_worker themselves, never as the
// database owner, which is a superuser in production: a superuser skips
// row-level security, and a SET ROLE on its session can be undone with RESET
// ROLE. The migrations' owner gives the two roles LOGIN and their passwords
// on every deploy, from the .env (generated on the VPS, openssl rand -hex 32).

// LoginRoles maps each service role to the variable with its password.
var LoginRoles = []struct{ Role, Env string }{
	{RoleApp, "QR_APP_DB_PASSWORD"},
	{RoleWorker, "QR_WORKER_DB_PASSWORD"},
}

// passwordRe: at least 32 hex digits. Hex also keeps the literal safe to
// write into ALTER ROLE, which takes no bind parameters.
var passwordRe = regexp.MustCompile(`^[0-9a-f]{32,128}$`)

// SyncLoginRoles gives each role of LoginRoles whose variable is set LOGIN
// and that password (stored as SCRAM by PostgreSQL), and returns the roles
// it set. A role whose variable is empty is left as it is (NOLOGIN after the
// migrations): development and tests. Passwords never reach the log.
func SyncLoginRoles(ctx context.Context, db *sql.DB, getenv func(string) string) ([]string, error) {
	var done []string
	for _, r := range LoginRoles {
		pw := getenv(r.Env)
		if pw == "" {
			continue
		}
		if !passwordRe.MatchString(pw) {
			return done, fmt.Errorf("%s must be 32 to 128 lower-case hex digits (openssl rand -hex 32)", r.Env)
		}
		stmt := fmt.Sprintf("ALTER ROLE %s WITH LOGIN PASSWORD '%s'", pgx.Identifier{r.Role}.Sanitize(), pw)
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return done, fmt.Errorf("enable login of %s: %w", r.Role, err)
		}
		done = append(done, r.Role)
	}
	return done, nil
}

// ErrPrivilegedRole means a service is connected with a role that skips
// row-level security: a superuser or one with BYPASSRLS.
var ErrPrivilegedRole = errors.New("store: connected as a superuser or a role with BYPASSRLS")

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CheckUnprivileged returns ErrPrivilegedRole if the login role of the
// session (session_user), the role it acts as (current_user) or any role the
// login is a member of is a superuser or has BYPASSRLS. After SET ROLE,
// current_user is the service role, but a superuser login could still RESET
// ROLE, and a member of a privileged role could SET ROLE to it at any time
// (pg_has_role MEMBER: direct or indirect, whatever INHERIT or SET say).
func CheckUnprivileged(ctx context.Context, q rowQuerier) error {
	var sessionUser, currentUser string
	var privileged bool
	err := q.QueryRow(ctx, `
		SELECT session_user::text, current_user::text, coalesce(bool_or(r.rolsuper OR r.rolbypassrls), false)
		FROM pg_roles r
		WHERE r.rolname = current_user OR pg_has_role(session_user, r.oid, 'MEMBER')`).Scan(&sessionUser, &currentUser, &privileged)
	if err != nil {
		return fmt.Errorf("check database role: %w", err)
	}
	if privileged {
		return fmt.Errorf("%w (session_user %s, current_user %s): log in as the service's own user", ErrPrivilegedRole, sessionUser, currentUser)
	}
	return nil
}

// RequireUnprivileged makes every new connection of the pool check
// CheckUnprivileged (after SET ROLE, if UseRole came first): a privileged one
// is closed before any query runs on it.
func RequireUnprivileged(cfg *pgxpool.Config) {
	prev := cfg.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if prev != nil {
			if err := prev(ctx, c); err != nil {
				return err
			}
		}
		return CheckUnprivileged(ctx, c)
	}
}

// CheckPoolAtStart opens one connection of pool, which runs the checks of
// RequireUnprivileged, within wait. It returns ErrPrivilegedRole (wrapped)
// if the role is privileged: the service must not start. If the database is
// unreachable it returns nil and reports reachable=false: the service starts
// and every later connection is still checked.
func CheckPoolAtStart(ctx context.Context, pool *pgxpool.Pool, wait time.Duration) (reachable bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	for {
		c, err := pool.Acquire(ctx)
		if err == nil {
			c.Release()
			return true, nil
		}
		if errors.Is(err, ErrPrivilegedRole) {
			return true, err
		}
		select {
		case <-ctx.Done():
			return false, nil
		case <-time.After(time.Second):
		}
	}
}
