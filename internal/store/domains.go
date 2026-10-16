package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Domain statuses.
const (
	DomainPending  = "pending"  // never verified, or verification lost after the grace period
	DomainVerified = "verified" // the TXT record was there at the last check
	DomainLost     = "lost"     // the record is gone; full scans continue during DomainGrace
)

// Domain verification timing.
const (
	DomainRecheck     = 7 * 24 * time.Hour  // a verified domain is checked again weekly
	DomainLostRecheck = 24 * time.Hour      // a lost one daily
	DomainGrace       = 14 * 24 * time.Hour // then it is no longer verified
)

// ErrDomainExists means the organization already has that domain.
var ErrDomainExists = errors.New("store: domain already added")

// Verified reports whether d allows full scans now: verified, or lost less
// than DomainGrace ago.
func (d Domain) Verified(now time.Time) bool {
	return d.Status == DomainVerified || (d.Status == DomainLost && d.LostAt != nil && now.Before(d.LostAt.Add(DomainGrace)))
}

// AddDomain adds a domain (already normalized) to the active organization,
// with its verification token. ErrDomainExists, or ErrLimit beyond maxDomains.
func (s *Store) AddDomain(ctx context.Context, t Tenant, name, token string, maxDomains int) (d Domain, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		n, err := q.CountDomains(ctx, *t.OrgID)
		if err != nil {
			return err
		}
		if maxDomains > 0 && n >= int64(maxDomains) {
			return ErrLimit
		}
		d, err = q.CreateDomain(ctx, CreateDomainParams{OrganizationID: *t.OrgID, Name: name, Token: token})
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return ErrDomainExists
		}
		return err
	})
	return d, err
}

// Domains lists the active organization's domains.
func (s *Store) Domains(ctx context.Context, t Tenant) (ds []Domain, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		ds, err = q.ListDomains(ctx, *t.OrgID)
		return err
	})
	return ds, err
}

// DomainFor returns a domain of the active organization, or ErrNotFound.
func (s *Store) DomainFor(ctx context.Context, t Tenant, id uuid.UUID) (d Domain, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		d, err = q.GetDomain(ctx, GetDomainParams{OrganizationID: *t.OrgID, ID: id})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}

// DeleteDomain removes a domain of the active organization (its scans stay).
func (s *Store) DeleteDomain(ctx context.Context, t Tenant, id uuid.UUID) error {
	return s.Tx(ctx, t, func(q *Queries) error {
		n, err := q.DeleteDomain(ctx, DeleteDomainParams{OrganizationID: *t.OrgID, ID: id})
		if err == nil && n != 1 {
			err = ErrNotFound
		}
		return err
	})
}

// RequestDomainCheck asks the worker to check a domain's TXT record now.
func (s *Store) RequestDomainCheck(ctx context.Context, t Tenant, id uuid.UUID) (d Domain, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		d, err = q.RequestDomainCheck(ctx, RequestDomainCheckParams{OrganizationID: *t.OrgID, ID: id})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}
