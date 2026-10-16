package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Suffixes returns host and its parent domains with at least two labels
// (a.b.example.com → a.b.example.com, b.example.com, example.com).
func Suffixes(host string) []string {
	labels := strings.Split(host, ".")
	var out []string
	for i := 0; i+2 <= len(labels); i++ {
		out = append(out, strings.Join(labels[i:], "."))
	}
	return out
}

// PublicScanBlocked reports whether a public scan of host is refused: the
// host or a parent domain is on the exclusion list or opted out by its
// verified owner. Nothing else about the domain is revealed.
func (s *Store) PublicScanBlocked(ctx context.Context, host string) (blocked bool, err error) {
	names := Suffixes(host)
	if len(names) == 0 || len(names) > 127 {
		return true, nil
	}
	err = s.Tx(ctx, Public, func(q *Queries) error {
		return q.db.QueryRow(ctx, "SELECT public_scan_blocked($1)", names).Scan(&blocked)
	})
	return blocked, err
}

// SetDomainBlockPublic sets the opt-out of a domain of the active organization.
func (s *Store) SetDomainBlockPublic(ctx context.Context, t Tenant, id uuid.UUID, block bool) (d Domain, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		d, err = q.SetDomainBlockPublic(ctx, SetDomainBlockPublicParams{OrganizationID: *t.OrgID, ID: id, BlockPublic: block})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}
