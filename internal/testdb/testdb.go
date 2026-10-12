// Package testdb gives integration tests their PostgreSQL connection string.
package testdb

import (
	"os"
	"testing"
)

// DSN returns QR_TEST_DATABASE_URL. When it is unset the test is skipped, or
// fails if REQUIRE_DB=1 (set in CI so integration tests can't be skipped silently).
func DSN(t testing.TB) string {
	t.Helper()
	if dsn := os.Getenv("QR_TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	if os.Getenv("REQUIRE_DB") == "1" {
		t.Fatal("REQUIRE_DB=1 but QR_TEST_DATABASE_URL is not set")
		return ""
	}
	t.Skip("QR_TEST_DATABASE_URL not set")
	return ""
}
