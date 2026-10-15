package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Database roles (migrations 00004 and 00005, docs/security.md).
const (
	RoleApp    = "qr_app"    // API: row-level security per organization
	RoleWorker = "qr_worker" // worker: scan queue and findings only
	RoleMailer = "qr_mailer" // mailer: email outbox only
)

// UseRole makes every connection of a pool act as role from the start (SET
// ROLE), whatever user it logs in as. Until milestone 3.7 the services log in
// as the database owner; from then on as their own users, and this is a no-op
// safety net. Only the three known roles are accepted.
func UseRole(cfg *pgxpool.Config, role string) error {
	switch role {
	case RoleApp, RoleWorker, RoleMailer:
	default:
		return fmt.Errorf("store: unknown role %q", role)
	}
	prev := cfg.AfterConnect
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		if prev != nil {
			if err := prev(ctx, c); err != nil {
				return err
			}
		}
		_, err := c.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		return err
	}
	return nil
}

// Pool returns the underlying connection pool (tests build role-restricted
// pools on the same schema from it).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }
