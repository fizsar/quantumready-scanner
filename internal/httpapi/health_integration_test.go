package httpapi

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestHealthPostgres runs against a real PostgreSQL when QR_TEST_DATABASE_URL is
// set (make test sets it from DATABASE_URL if the dev database is up).
func TestHealthPostgres(t *testing.T) {
	dsn := os.Getenv("QR_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("QR_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	rec := doRequest(t, newTestRouter(pool), http.MethodGet, "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
