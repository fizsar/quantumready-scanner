package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/store"
)

const refusalsNote = `client_limit, domain_limit, global_limit: 429 by the per-client, per-domain
and global limits; queue_full: 429, the public queue was full; excluded: 403
scan_not_allowed (exclusion list or opt-out); proof_failed: 400, no valid proof
of work. Counted by the API per day and kind only (migration 00022), written
every 30 s. closed_403: 403 host_not_allowed while the scanner is closed, from
the M1 counter, which leaves bots out.`

// publicReportCmd prints what public scanning did, day by day (UTC): scans,
// outcomes, distinct domains, queue timeouts, the longest queue, the refused
// requests by kind, the most scanned domains (only those at -min or more in
// the period) and the exclusions. Public scans are kept 30 days, so -days goes
// up to 31. It only reads, and nothing it reads says who asked.
func publicReportCmd(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	fs := flag.NewFlagSet("public-report", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	days := fs.Int("days", 7, "days to show, today included")
	minScans := fs.Int("min", 3, "list a domain among the most scanned from this many scans")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		return errors.New(usage)
	}
	if *days < 1 || *days > 31 {
		return errors.New("-days must be between 1 and 31 (public scans are kept 30 days)")
	}
	if *minScans < 2 {
		return errors.New("-min must be 2 or more (a single scan says nothing about a domain)")
	}
	pool, err := open(ctx, getenv)
	if err != nil {
		return err
	}
	defer pool.Close()
	now := time.Now().UTC()
	since := now.Truncate(24*time.Hour).AddDate(0, 0, 1-*days)
	return publicReport(ctx, store.NewStore(pool).Queries, since, now, *minScans, out)
}

type dayReport struct {
	scans, done, failed, full, timeouts, domains, maxQueue, exclusions int64
}

func publicReport(ctx context.Context, q *store.Queries, since, now time.Time, minScans int, out io.Writer) error {
	byDay, err := q.PublicScansByDay(ctx, since)
	if err != nil {
		return err
	}
	hosts, err := q.PublicScanHostsByDay(ctx, since)
	if err != nil {
		return err
	}
	spans, err := q.PublicQueueSpans(ctx, since)
	if err != nil {
		return err
	}
	added, err := q.ExclusionsAddedByDay(ctx, since)
	if err != nil {
		return err
	}
	excluded, err := q.CountExclusions(ctx)
	if err != nil {
		return err
	}
	sinceDay := pgtype.Date{Time: since, Valid: true}
	refusals, err := q.RefusalsByDay(ctx, sinceDay)
	if err != nil {
		return err
	}
	closed, err := q.ClosedRefusalsByDay(ctx, sinceDay)
	if err != nil {
		return err
	}

	days := map[string]*dayReport{}
	var keys []string
	for d := since; !d.After(now); d = d.AddDate(0, 0, 1) {
		k := d.Format(time.DateOnly)
		days[k] = &dayReport{}
		keys = append(keys, k)
	}
	at := func(t time.Time) *dayReport { return days[t.UTC().Format(time.DateOnly)] }
	for _, r := range byDay {
		if d := at(r.Day.Time); d != nil {
			d.scans, d.done, d.failed, d.timeouts, d.full = r.Scans, r.Done, r.Failed, r.QueueTimeouts, r.FullScans
		}
	}
	perDomain := map[string]int64{}
	seen := map[string]map[string]bool{}
	for _, r := range hosts {
		reg := domains.Registrable(r.Host)
		perDomain[reg] += r.Scans
		k := r.Day.Time.Format(time.DateOnly)
		if seen[k] == nil {
			seen[k] = map[string]bool{}
		}
		seen[k][reg] = true
	}
	for k, regs := range seen {
		if d := days[k]; d != nil {
			d.domains = int64(len(regs))
		}
	}
	for _, r := range added {
		if d := at(r.Day.Time); d != nil {
			d.exclusions = r.Added
		}
	}
	for k, n := range maxQueueByDay(spans, since, now) {
		if d := days[k]; d != nil {
			d.maxQueue = n
		}
	}

	fmt.Fprintf(out, "Public scans since %s (UTC). Nothing here identifies who asked for a scan.\n\n", since.Format(time.DateOnly))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "day\tscans\tdone\tfailed\tfull\tdomains\tqueue_timeouts\tmax_queue\texclusions_added\t")
	var total dayReport
	for _, k := range keys {
		d := days[k]
		fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t\n", k, d.scans, d.done, d.failed, d.full, d.domains, d.timeouts, d.maxQueue, d.exclusions)
		total.scans += d.scans
		total.done += d.done
		total.failed += d.failed
		total.full += d.full
		total.timeouts += d.timeouts
		total.exclusions += d.exclusions
		total.maxQueue = max(total.maxQueue, d.maxQueue)
	}
	fmt.Fprintf(w, "total\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t\n", total.scans, total.done, total.failed, total.full, len(perDomain), total.timeouts, total.maxQueue, total.exclusions)
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out, "\ndomains: distinct registrable domains (www.shop.example.co.uk counts as example.co.uk).")
	fmt.Fprintln(out, "max_queue: the most public scans waiting at once that day, rebuilt from when each was")
	fmt.Fprintln(out, "created and last claimed (a retried scan counts as waiting until its last claim).")

	if err := refusalTable(out, keys, refusals, closed); err != nil {
		return err
	}

	fmt.Fprintf(out, "\nMost scanned domains (registrable; %d scans or more in the period)\n", minScans)
	var often []dimCount
	for d, n := range perDomain {
		if n >= int64(minScans) {
			often = append(often, dimCount{d, n})
		}
	}
	if len(often) == 0 {
		fmt.Fprintln(out, "  (none)")
	} else {
		w = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for _, kv := range top(toMap(often), 20) {
			fmt.Fprintf(w, "  %s\t%d\n", kv.dim, kv.n)
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "\nExclusions in force: %d (qradmin exclude list)\n", excluded)
	return nil
}

// refusalTable prints the refused requests per day and kind.
func refusalTable(out io.Writer, keys []string, refusals []store.ScanRefusalsDaily, closed []store.ClosedRefusalsByDayRow) error {
	per := map[string]map[string]int64{}
	for _, k := range keys {
		per[k] = map[string]int64{}
	}
	for _, r := range refusals {
		if d := per[r.Day.Time.Format(time.DateOnly)]; d != nil {
			d[r.Kind] += r.Count
		}
	}
	for _, r := range closed {
		if d := per[r.Day.Time.Format(time.DateOnly)]; d != nil {
			d["closed_403"] += r.Refused
		}
	}
	cols := append(append([]string{}, store.RefusalKinds...), "closed_403")
	fmt.Fprintln(out, "\nRefused requests")
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprint(w, "day\t")
	for _, c := range cols {
		fmt.Fprintf(w, "%s\t", c)
	}
	fmt.Fprintln(w)
	total := map[string]int64{}
	row := func(label string, m map[string]int64) {
		fmt.Fprintf(w, "%s\t", label)
		for _, c := range cols {
			fmt.Fprintf(w, "%d\t", m[c])
		}
		fmt.Fprintln(w)
	}
	for _, k := range keys {
		row(k, per[k])
		for c, n := range per[k] {
			total[c] += n
		}
	}
	row("total", total)
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out, "\n"+refusalsNote)
	return nil
}

func toMap(s []dimCount) map[string]int64 {
	m := make(map[string]int64, len(s))
	for _, kv := range s {
		m[kv.dim] = kv.n
	}
	return m
}

// maxQueueByDay sweeps the queue spans: the largest number of public scans
// waiting at the same time on each day (UTC) from since to now. A queue that
// carries over midnight counts for the new day as well.
func maxQueueByDay(spans []store.PublicQueueSpansRow, since, now time.Time) map[string]int64 {
	type event struct {
		t     time.Time
		delta int64
	}
	var events []event
	for _, s := range spans {
		if s.LeftQueue.After(s.CreatedAt) {
			events = append(events, event{s.CreatedAt, 1}, event{s.LeftQueue, -1})
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if !events[i].t.Equal(events[j].t) {
			return events[i].t.Before(events[j].t)
		}
		return events[i].delta < events[j].delta // a scan leaving before another enters at the same instant
	})
	out := map[string]int64{}
	var cur int64
	day := since.AddDate(0, 0, -2) // before every span (created from a day before since)
	note := func(d time.Time, n int64) {
		if !d.Before(since) && !d.After(now) {
			k := d.Format(time.DateOnly)
			out[k] = max(out[k], n)
		}
	}
	for _, e := range events {
		for next := day.AddDate(0, 0, 1); !e.t.Before(next); next = next.AddDate(0, 0, 1) {
			day = next
			note(day, cur) // still waiting at midnight
		}
		cur += e.delta
		note(e.t.UTC().Truncate(24*time.Hour), cur)
	}
	for next := day.AddDate(0, 0, 1); !next.After(now); next = next.AddDate(0, 0, 1) {
		note(next, cur)
	}
	return out
}
