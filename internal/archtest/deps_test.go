// Package archtest checks the shape of the code base (docs/adr): which
// packages may depend on which. It holds tests only.
package archtest

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/fizsar/quantumready-saas/"

// public are the packages that must stay importable from another module one
// day (ADR-0008: the engine, the evaluators and the catalog of texts): they
// must not reach the API, the database, the queue or any server-side concern.
var public = []string{
	"internal/scanner",
	"internal/scanner/rawtls",
	"internal/scoring",
	"internal/catalog",
	"internal/domains",
}

// forbidden are what a public package may never import, directly or not.
var forbidden = []string{
	module + "internal/store",
	module + "internal/httpapi",
	module + "internal/worker",
	module + "internal/auth",
	module + "internal/org",
	module + "internal/mail",
	module + "internal/altcha",
	module + "internal/testdb",
	module + "migrations",
	"github.com/jackc/pgx/v5",
	"github.com/go-chi/chi/v5",
	"database/sql",
	"net/http",
}

func deps(t *testing.T, pkg string) []string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./"+pkg)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

// TestPublicPackagesStayIndependent: ADR-0008. The engine, the evaluators and
// the catalog never import the API, the database or a server.
func TestPublicPackagesStayIndependent(t *testing.T) {
	for _, pkg := range public {
		got := deps(t, pkg)
		if !slices.Contains(got, module+pkg) {
			t.Fatalf("%s: go list did not list the package itself: %v", pkg, got)
		}
		for _, d := range got {
			for _, f := range forbidden {
				if d == f || strings.HasPrefix(d, f+"/") {
					t.Errorf("%s depends on %s (ADR-0008: public packages stay free of the API and the database)", pkg, d)
				}
			}
		}
	}
}

// The check works: the API does depend on what the public packages may not.
func TestForbiddenListBites(t *testing.T) {
	got := deps(t, "internal/httpapi")
	for _, f := range []string{module + "internal/store", "net/http", "github.com/go-chi/chi/v5"} {
		if !slices.Contains(got, f) {
			t.Errorf("internal/httpapi no longer depends on %s: is the forbidden list still meaningful?", f)
		}
	}
}
