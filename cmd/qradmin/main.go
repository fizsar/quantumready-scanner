// Command qradmin runs administrative tasks against the database as its
// owner (DATABASE_URL of the migrations, not a qr_* role).
//
//	qradmin stats [-days 30]
//
// stats prints the M1 usage counters: events per day, the most common values
// of each event and the domains most often refused with a 403 (only those
// asked for 3 times or more; they are kept 30 days).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fizsar/quantumready-saas/internal/store"
)

const usage = `usage: qradmin stats [-days N]   (usage counters of the last N days, UTC; 1-366, default 30)`

// events in the order of the table columns (the closed list of migration 00015).
var events = []string{"page_view", "visit_source", "scan_started", "scan_blocked_403", "under_the_hood_open", "json_download", "lang_switch"}

// topPerEvent is how many values of each event the summary lists.
const topPerEvent = 15

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "qradmin:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) == 0 || args[0] != "stats" {
		return errors.New(usage)
	}
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	days := fs.Int("days", 30, "days to show, today included")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 {
		return errors.New(usage)
	}
	if *days < 1 || *days > 366 {
		return errors.New("-days must be between 1 and 366")
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
	since := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1-*days)
	return stats(ctx, store.NewStore(pool).Queries, since, out)
}

func stats(ctx context.Context, q *store.Queries, since time.Time, out io.Writer) error {
	day := pgtype.Date{Time: since, Valid: true}
	rows, err := q.UsageSince(ctx, day)
	if err != nil {
		return err
	}
	blocked, err := q.BlockedTargetsSince(ctx, day)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Usage since %s (UTC)\n\n", since.Format(time.DateOnly))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprint(w, "day\t")
	for _, e := range events {
		fmt.Fprintf(w, "%s\t", e)
	}
	fmt.Fprintln(w)
	perDay := map[string]map[string]int64{}
	var dayKeys []string
	total := map[string]int64{}
	byDim := map[string]map[string]int64{}
	for _, r := range rows {
		d := r.Day.Time.Format(time.DateOnly)
		if perDay[d] == nil {
			perDay[d] = map[string]int64{}
			dayKeys = append(dayKeys, d)
		}
		perDay[d][r.Event] += r.Count
		total[r.Event] += r.Count
		if byDim[r.Event] == nil {
			byDim[r.Event] = map[string]int64{}
		}
		byDim[r.Event][r.Dim] += r.Count
	}
	line := func(label string, counts map[string]int64) {
		fmt.Fprintf(w, "%s\t", label)
		for _, e := range events {
			fmt.Fprintf(w, "%d\t", counts[e])
		}
		fmt.Fprintln(w)
	}
	for _, d := range dayKeys {
		line(d, perDay[d])
	}
	line("total", total)
	if err := w.Flush(); err != nil {
		return err
	}

	for _, e := range events {
		if _, plain := byDim[e][""]; len(byDim[e]) == 0 || plain && len(byDim[e]) == 1 {
			continue // nothing counted, or an event without values (scan_blocked_403)
		}
		fmt.Fprintf(out, "\n%s (top %d)\n", e, topPerEvent)
		w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, kv := range top(byDim[e], topPerEvent) {
			fmt.Fprintf(w, "  %s\t%d\n", kv.dim, kv.n)
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}

	fmt.Fprintln(out, "\nRefused targets (403), asked for 3 times or more; kept 30 days")
	if len(blocked) == 0 {
		fmt.Fprintln(out, "  (none)")
		return nil
	}
	w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, b := range blocked {
		fmt.Fprintf(w, "  %s\t%d\n", b.Domain, b.Total)
	}
	return w.Flush()
}

type dimCount struct {
	dim string
	n   int64
}

// top returns the n largest counts, ties by name.
func top(m map[string]int64, n int) []dimCount {
	s := make([]dimCount, 0, len(m))
	for d, c := range m {
		s = append(s, dimCount{d, c})
	}
	sort.Slice(s, func(i, j int) bool {
		if s[i].n != s[j].n {
			return s[i].n > s[j].n
		}
		return s[i].dim < s[j].dim
	})
	if len(s) > n {
		s = s[:n]
	}
	return s
}
