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

	rec := doRequest(t, newTestRouter(pool), http.MethodGet, "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
