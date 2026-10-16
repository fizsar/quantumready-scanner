package worker

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/dnscheck"
	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

type fakeTXT map[string][]string // name → records; "error" → unavailable

func (f fakeTXT) TXT(_ context.Context, name string) ([]string, error) {
	if r := f[name]; len(r) == 1 && r[0] == "error" {
		return nil, dnscheck.ErrUnavailable
	}
	return f[name], nil
}

// The domain checker, run with the worker's production role, moves each
// domain through pending → verified → lost → pending as the record comes
// and goes, and leaves it alone when DNS does not answer.
func TestDomainChecks(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	org := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO organizations (id, name) VALUES ($1, 'Acme')", org); err != nil {
		t.Fatal(err)
	}
	type row struct {
		name, status, sets string // sets: extra SQL for the row's state
	}
	ids := map[string]uuid.UUID{}
	for _, r := range []row{
		{"new-ok.example", "pending", "check_requested_at = now()"},
		{"new-missing.example", "pending", "check_requested_at = now()"},
		{"due-gone.example", "verified", "verified_at = now() - interval '30 days', last_checked_at = now() - interval '8 days'"},
		{"lost-long.example", "lost", "verified_at = now() - interval '60 days', lost_at = now() - interval '15 days', last_checked_at = now() - interval '2 days'"},
		{"lost-back.example", "lost", "verified_at = now() - interval '60 days', lost_at = now() - interval '3 days', last_checked_at = now() - interval '2 days'"},
		{"fresh.example", "verified", "verified_at = now() - interval '9 days', last_checked_at = now() - interval '1 day'"},
		{"dns-down.example", "verified", "verified_at = now() - interval '9 days', last_checked_at = now() - interval '8 days'"},
		{"never-asked.example", "pending", "created_at = now()"},
	} {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, "INSERT INTO domains (organization_id, name, token, status) VALUES ($1, $2, 'tok-'||$2, $3) RETURNING id",
			org, r.name, r.status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE domains SET "+r.sets+" WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
		ids[r.name] = id
	}
	dns := fakeTXT{
		"_quantumready.new-ok.example":    {"v=spf1 -all", " qr-verify=tok-new-ok.example "},
		"_quantumready.lost-back.example": {"qr-verify=tok-lost-back.example"},
		"_quantumready.lost-long.example": {"qr-verify=someone-else"},
		"_quantumready.dns-down.example":  {"error"},
	}
	ws := store.NewStore(testdb.AsRole(t, pool, store.RoleWorker))
	dc := &DomainChecker{Queue: ws.Queries, DNS: dns, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Batch: 3}
	total := 0
	for {
		n, err := dc.Once(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		total += n
	}
	if total != 6 {
		t.Errorf("checked %d domains, want 6 (not the fresh one nor the never-asked one)", total)
	}

	type state struct {
		status                            string
		verified, lost, requested, leased bool
		lastErr                           string
	}
	get := func(name string) state {
		var s state
		var e *string
		if err := pool.QueryRow(ctx, `SELECT status, verified_at IS NOT NULL, lost_at IS NOT NULL, check_requested_at IS NOT NULL,
			coalesce(check_lease_until > now(), false), last_check_error FROM domains WHERE id = $1`, ids[name]).
			Scan(&s.status, &s.verified, &s.lost, &s.requested, &s.leased, &e); err != nil {
			t.Fatal(err)
		}
		if e != nil {
			s.lastErr = *e
		}
		return s
	}
	for name, want := range map[string]state{
		"new-ok.example":      {status: "verified", verified: true},
		"new-missing.example": {status: "pending", lastErr: "dns_record_missing"},
		"due-gone.example":    {status: "lost", verified: true, lost: true, lastErr: "dns_record_missing"},
		"lost-long.example":   {status: "pending", lastErr: "dns_record_missing"}, // grace over: no longer verified
		"lost-back.example":   {status: "verified", verified: true},
		"fresh.example":       {status: "verified", verified: true},
		"dns-down.example":    {status: "verified", verified: true, leased: true, lastErr: "dns_unavailable"}, // unchanged, backing off
		"never-asked.example": {status: "pending"},
	} {
		if got := get(name); got != want {
			t.Errorf("%s: %+v, want %+v", name, got, want)
		}
	}
	if n, _ := dc.Once(ctx); n != 0 {
		t.Errorf("%d domains checked again right away (the DNS failure should back off)", n)
	}
}
