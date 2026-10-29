package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Tenant is who a request acts for. The zero value is the public,
// anonymous tenant: it sees only public scans (no organization).
type Tenant struct {
	OrgID  *uuid.UUID // active organization, nil for public requests
	UserID *uuid.UUID // signed-in user, nil for anonymous requests
}

// Public is the anonymous tenant.
var Public = Tenant{}

// Intensities of a scan (scanner.Intensity values).
const (
	IntensityBasic = "basic"
	IntensityFull  = "full"
)

// How long scans are kept (privacy: data minimization): public ones 30
// days, an organization's history 13 months. The purge comes with milestone 3.6.
const (
	PublicScanTTL = 30 * 24 * time.Hour
	OrgScanTTL    = 395 * 24 * time.Hour
)

// OrgScans lists the active organization's scans, newest first.
func (s *Store) OrgScans(ctx context.Context, t Tenant, limit int32) (scs []Scan, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		scs, err = q.ListOrganizationScans(ctx, ListOrganizationScansParams{OrganizationID: t.OrgID, MaxRows: limit})
		return err
	})
	return scs, err
}

// ErrFullNeedsOrg is returned for a full scan without an organization: full
// scans exist only for an organization's verified domains.
var ErrFullNeedsOrg = errors.New("store: a full scan needs an organization")

// Tx runs fn in a transaction as the database role qr_app, with the tenant
// in app.org_id / app.user_id. Row-level security then limits every query
// to the tenant's rows, even one that forgets its organization_id filter.
// Both settings are transaction-local, so pooled connections never leak a
// tenant to the next request.
func (s *Store) Tx(ctx context.Context, t Tenant, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE qr_app"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.org_id', $1, true), set_config('app.user_id', $2, true)",
			idString(t.OrgID), idString(t.UserID)); err != nil {
			return err
		}
		return fn(s.WithTx(tx))
	})
}

func idString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// ScanRequest describes a scan to enqueue or reuse.
type ScanRequest struct {
	Host      string
	Port      int32
	Intensity string        // IntensityBasic or IntensityFull
	Cooldown  time.Duration // reuse a non-failed scan this recent
	// ScoringVersion: reuse only a scan judged (or to be judged) with this
	// model (scoring.Version); an older judgement is not handed out again.
	ScoringVersion string
}

// EnqueueOrReuse enqueues a scan for the tenant unless one of the same
// target, intensity and owner was created within the cooldown and has not
// failed; then it returns that one with reused=true. A basic scan never
// answers a full request, and a tenant never gets another tenant's scan.
// Public scans expire after PublicScanTTL. Concurrent calls for the same
// target and owner are serialized, so they cannot both enqueue.
func (s *Store) EnqueueOrReuse(ctx context.Context, t Tenant, r ScanRequest) (sc Scan, reused bool, err error) {
	if r.Intensity != IntensityBasic && r.Intensity != IntensityFull {
		return Scan{}, false, errors.New("store: unknown intensity " + r.Intensity)
	}
	if r.Intensity == IntensityFull && t.OrgID == nil {
		return Scan{}, false, ErrFullNeedsOrg
	}
	err = s.Tx(ctx, t, func(q *Queries) error {
		if err := q.LockTarget(ctx, fmt.Sprintf("%s:%d:%s:%s", r.Host, r.Port, r.Intensity, idString(t.OrgID))); err != nil {
			return err
		}
		recent, err := q.RecentScanForTarget(ctx, RecentScanForTargetParams{
			Host: r.Host, Port: r.Port, Intensity: r.Intensity, OrganizationID: t.OrgID, ScoringVersion: r.ScoringVersion,
			WithinSeconds: r.Cooldown.Seconds(),
		})
		if err == nil {
			sc, reused = recent, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		e := time.Now().Add(PublicScanTTL)
		if t.OrgID != nil {
			e = time.Now().Add(OrgScanTTL)
		}
		expires := &e
		sc, err = q.EnqueueScan(ctx, EnqueueScanParams{
			Host: r.Host, Port: r.Port, OrganizationID: t.OrgID, Intensity: r.Intensity, RequestedBy: t.UserID, ExpiresAt: expires,
		})
		return err
	})
	return sc, reused, err
}

// RecentPublicScan returns a public basic scan of host:port to reuse — in
// flight or done within cooldown, or done within doneWithin — or
// pgx.ErrNoRows; only one judged (or to be judged) with scoringVersion. It
// reads with the public tenant, so row-level security alone already keeps
// organizations' scans out.
func (s *Store) RecentPublicScan(ctx context.Context, host string, port int32, cooldown, doneWithin time.Duration, scoringVersion string) (sc Scan, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		sc, err = q.RecentPublicScan(ctx, RecentPublicScanParams{Host: host, Port: port, ScoringVersion: scoringVersion,
			CooldownSeconds: cooldown.Seconds(), DoneSeconds: doneWithin.Seconds()})
		return err
	})
	return sc, err
}

// PublicQueueLength is how many public scans wait in the queue (public
// tenant: organizations' scans are not visible, let alone counted).
func (s *Store) PublicQueueLength(ctx context.Context) (n int64, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		n, err = q.CountPublicQueued(ctx)
		return err
	})
	return n, err
}

// PublicQueuePosition is the place of a queued public scan among public
// scans (1: next). Organizations' scans, which go first, are not counted.
func (s *Store) PublicQueuePosition(ctx context.Context, createdAt time.Time) (n int64, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		n, err = q.PublicQueuePosition(ctx, createdAt)
		return err
	})
	return n, err
}

// ScanFor returns a scan the tenant may see: public, or of its organization.
// Another organization's scan is pgx.ErrNoRows, as if it did not exist.
func (s *Store) ScanFor(ctx context.Context, t Tenant, id uuid.UUID) (sc Scan, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		sc, err = q.GetScanForTenant(ctx, GetScanForTenantParams{ID: id, OrganizationID: t.OrgID})
		return err
	})
	return sc, err
}

// FindingsFor returns the findings of a scan the tenant may see (none otherwise).
func (s *Store) FindingsFor(ctx context.Context, t Tenant, scanID uuid.UUID) (fs []Finding, err error) {
	err = s.Tx(ctx, t, func(q *Queries) error {
		fs, err = q.ListFindings(ctx, scanID)
		return err
	})
	return fs, err
}
