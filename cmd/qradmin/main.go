// Command qradmin runs administrative tasks against the database as its
// owner (DATABASE_URL of the migrations, not a qr_* role).
//
//	qradmin signup-code [-note "for Ana"] [-days 14]
//
// signup-code creates a closed-beta registration code and prints it once:
// only its SHA-256 is stored, so it cannot be shown again.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/auth"
	"github.com/fizsar/quantumready-saas/internal/store"
)

const usage = `usage: qradmin signup-code [-note TEXT] [-days N]`

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "qradmin:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) == 0 || args[0] != "signup-code" {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet("signup-code", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	note := fs.String("note", "", "who the code is for (stored with it)")
	days := fs.Int("days", 14, "days until the code expires (1-90)")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return errors.New(usage)
	}
	if *days < 1 || *days > 90 {
		return errors.New("-days must be between 1 and 90")
	}
	dsn := getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	return signupCode(ctx, store.NewStore(pool), strings.TrimSpace(*note), time.Duration(*days)*24*time.Hour, out)
}

func signupCode(ctx context.Context, st *store.Store, note string, ttl time.Duration, out io.Writer) error {
	code, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	var n *string
	if note != "" {
		n = &note
	}
	row, err := st.CreateSignupCode(ctx, hash, n, ttl)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Signup code (shown only once; expires %s):\n%s\n",
		row.ExpiresAt.UTC().Format("2006-01-02 15:04 MST"), code)
	return err
}
