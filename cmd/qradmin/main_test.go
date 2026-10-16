package main

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/fizsar/quantumready-saas/internal/auth"
	"github.com/fizsar/quantumready-saas/internal/store"
	"github.com/fizsar/quantumready-saas/internal/testdb"
	"github.com/fizsar/quantumready-saas/migrations"
)

func TestUsage(t *testing.T) {
	env := func(string) string { return "" }
	for _, args := range [][]string{nil, {"nope"}, {"signup-code", "extra"}, {"signup-code", "-days", "0"}, {"signup-code", "-days", "91"}, {"signup-code"}} {
		if err := run(context.Background(), args, env, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%q) accepted", args)
		}
	}
}

func TestExclusions(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	q := store.NewStore(pool).Queries
	var out bytes.Buffer
	if err := excludeOn(ctx, q, "add", "example.org", "asked by mail", &out); err != nil {
		t.Fatal(err)
	}
	if err := excludeOn(ctx, q, "list", "", "", &out); err != nil || !strings.Contains(out.String(), "example.org\t") || !strings.Contains(out.String(), "asked by mail") {
		t.Errorf("list: %q %v", out.String(), err)
	}
	if err := excludeOn(ctx, q, "remove", "example.org", "", &out); err != nil {
		t.Error(err)
	}
	if err := excludeOn(ctx, q, "remove", "example.org", "", &out); err == nil {
		t.Error("removing a missing name succeeded")
	}
	for _, args := range [][]string{{"exclude"}, {"exclude", "add"}, {"exclude", "add", "co.uk"}, {"exclude", "add", "192.0.2.1"}, {"exclude", "nope"}} {
		if err := run(ctx, args, func(string) string { return "" }, &out); err == nil {
			t.Errorf("run(%q) accepted", args)
		}
	}
}

func TestOptOuts(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ('00000000-0000-0000-0000-0000000000aa', 'Acme');
		INSERT INTO domains (organization_id, name, token, status, block_public)
		VALUES ('00000000-0000-0000-0000-0000000000aa', 'example.org', 't', 'pending', true)`); err != nil {
		t.Fatal(err)
	}
	q := store.NewStore(pool).Queries
	var out bytes.Buffer
	if err := optoutOn(ctx, q, "list", "", &out); err != nil || !strings.Contains(out.String(), "example.org\tpending") {
		t.Errorf("list: %q %v", out.String(), err)
	}
	if err := optoutOn(ctx, q, "clear", "example.org", &out); err != nil {
		t.Fatal(err)
	}
	var blocked bool
	_ = pool.QueryRow(ctx, "SELECT public_scan_blocked(ARRAY['example.org'])").Scan(&blocked)
	if blocked {
		t.Error("still blocked after clear")
	}
	if err := optoutOn(ctx, q, "clear", "example.org", &out); err == nil {
		t.Error("clearing twice succeeded")
	}
	for _, args := range [][]string{{"optout"}, {"optout", "clear"}, {"optout", "clear", "co.uk"}, {"optout", "nope"}} {
		if err := run(ctx, args, func(string) string { return "" }, &out); err == nil {
			t.Errorf("run(%q) accepted", args)
		}
	}
}

func TestSignupCode(t *testing.T) {
	pool := testdb.Schema(t, func(ctx context.Context, db *sql.DB) error {
		_, err := migrations.Up(ctx, db)
		return err
	})
	ctx := context.Background()
	var out bytes.Buffer
	if err := signupCode(ctx, store.NewStore(pool), "for Ana", 14*24*time.Hour, &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	code := lines[len(lines)-1]
	var note string
	var days float64
	err := pool.QueryRow(ctx, "SELECT note, extract(epoch FROM expires_at - now()) / 86400 FROM signup_codes WHERE code_hash = $1",
		auth.HashToken(code)).Scan(&note, &days)
	if err != nil || note != "for Ana" || days < 13.9 || days > 14 {
		t.Fatalf("stored code: %q %.2f %v (output %q)", note, days, err, out.String())
	}
	var raw int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM signup_codes WHERE position(convert_to($1, 'UTF8') IN code_hash) > 0 OR note LIKE '%' || $1 || '%'", code).Scan(&raw)
	if raw != 0 {
		t.Error("the code itself is stored")
	}
}
