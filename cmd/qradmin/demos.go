package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/store"
)

const demosUsage = `usage: qradmin demos list [-n N]   (newest first; default 50)
       qradmin demos show ID
       qradmin demos delete ID          (e.g. when the sender asks; they are purged after 12 months anyway)`

// demos reads and deletes the "Request a free demo" submissions.
func demos(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(demosUsage)
	}
	cmd, args := args[0], args[1:]
	var id uuid.UUID
	var limit int32 = 50
	switch cmd {
	case "list":
		fs := flag.NewFlagSet("demos list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		n := fs.Int("n", 50, "how many")
		if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *n < 1 || *n > 1000 {
			return errors.New(demosUsage)
		}
		limit = int32(*n)
	case "show", "delete":
		if len(args) != 1 {
			return errors.New(demosUsage)
		}
		var err error
		if id, err = uuid.Parse(args[0]); err != nil {
			return fmt.Errorf("%q is not a demo request id", args[0])
		}
	default:
		return errors.New(demosUsage)
	}
	pool, err := connect(ctx, getenv)
	if err != nil {
		return err
	}
	defer pool.Close()
	return demosOn(ctx, store.NewStore(pool).Queries, cmd, id, limit, out)
}

func demosOn(ctx context.Context, q *store.Queries, cmd string, id uuid.UUID, limit int32, out io.Writer) error {
	switch cmd {
	case "list":
		rows, err := q.ListDemoRequests(ctx, limit)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Fprintln(out, "(no demo requests)")
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "created (UTC)\tid\tname\temail\tcompany\tdomain\tlang")
		for _, r := range rows {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.CreatedAt.UTC().Format("2006-01-02 15:04"), r.ID, r.Name, r.Email, r.Company, r.Domain, r.Lang)
		}
		return w.Flush()
	case "show":
		r, err := q.GetDemoRequest(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("no demo request %s", id)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "id:       %s\ncreated:  %s UTC (purged after 12 months)\nname:     %s\nemail:    %s\ncompany:  %s\ndomain:   %s\nlang:     %s\nprivacy:  accepted (version %s)\nmessage:\n%s\n",
			r.ID, r.CreatedAt.UTC().Format(time.DateTime), r.Name, r.Email, r.Company, r.Domain, r.Lang, r.PrivacyVersion, r.Message)
		return nil
	default: // delete
		n, err := q.DeleteDemoRequest(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no demo request %s", id)
		}
		fmt.Fprintf(out, "deleted %s (copies in backups disappear as those age out, within 6 months)\n", id)
		return nil
	}
}

// connect opens the database of DATABASE_URL (the owner's, as for the
// migrations).
func connect(ctx context.Context, getenv func(string) string) (*pgxpool.Pool, error) {
	dsn := getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	return pgxpool.New(ctx, dsn)
}
