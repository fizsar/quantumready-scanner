package main

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

func TestDemosUsage(t *testing.T) {
	env := func(string) string { return "" }
	for _, args := range [][]string{{"demos"}, {"demos", "nope"}, {"demos", "list", "extra"}, {"demos", "list", "-n", "0"},
		{"demos", "show"}, {"demos", "show", "not-a-uuid"}, {"demos", "delete", "a", "b"}, {"demos", "list"}} {
		if err := run(context.Background(), args, env, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%q) accepted", args)
		}
	}
}

func TestDemos(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	q := store.NewStore(pool).Queries
	var out bytes.Buffer
	if err := demosOn(ctx, q, "list", uuid.Nil, 50, &out); err != nil || !strings.Contains(out.String(), "(no demo requests)") {
		t.Fatalf("empty list: %q %v", out.String(), err)
	}
	r, err := q.InsertDemoRequest(ctx, store.InsertDemoRequestParams{Name: "Ana", Email: "ana@example.org", Company: "Acme",
		Domain: "example.org", Message: "Hola,\nqueremos una demo.", Lang: "es", PrivacyVersion: "2026-10-20"})
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := demosOn(ctx, q, "list", uuid.Nil, 50, &out); err != nil || !strings.Contains(out.String(), r.ID.String()) ||
		!strings.Contains(out.String(), "ana@example.org") || strings.Contains(out.String(), "queremos") {
		t.Errorf("list (no message): %q %v", out.String(), err)
	}
	out.Reset()
	if err := demosOn(ctx, q, "show", r.ID, 0, &out); err != nil || !strings.Contains(out.String(), "queremos una demo") ||
		!strings.Contains(out.String(), "accepted (version 2026-10-20)") {
		t.Errorf("show: %q %v", out.String(), err)
	}
	out.Reset()
	if err := demosOn(ctx, q, "delete", r.ID, 0, &out); err != nil || !strings.Contains(out.String(), "deleted "+r.ID.String()) {
		t.Errorf("delete: %q %v", out.String(), err)
	}
	for _, cmd := range []string{"show", "delete"} {
		if err := demosOn(ctx, q, cmd, r.ID, 0, &out); err == nil || !strings.Contains(err.Error(), "no demo request") {
			t.Errorf("%s after delete: %v", cmd, err)
		}
	}
}
