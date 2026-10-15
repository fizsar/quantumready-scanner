package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/testdb"
)

// TestHealthPostgres runs against a real PostgreSQL (see testdb.DSN; make test
// provides it when the dev database is up).
func TestHealthPostgres(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), testdb.DSN(t))
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	// Open the connection first: the test router allows the ping only 50 ms,
	// and dialing a new connection can take longer under a loaded test run
	// (the timeout itself is tested with a fake in health_test.go).
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("warm-up ping: %v", err)
	}

	rec := doRequest(t, newTestRouter(pool), http.MethodGet, "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
