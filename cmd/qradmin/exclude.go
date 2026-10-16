package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// exclude manages the exclusion list of public scans (scan_exclusions):
// domains whose owners asked us not to scan them, without an account.
func exclude(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cmd, args := args[0], args[1:]
	var name, reason string
	switch cmd {
	case "add", "remove":
		if len(args) == 0 {
			return errors.New(usage)
		}
		n, err := domains.Normalize(args[0])
		if err != nil {
			return fmt.Errorf("%s: %w", args[0], err)
		}
		name = n
		fs := flag.NewFlagSet("exclude", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		r := fs.String("reason", "", "why (who asked, ticket…)")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 0 || (cmd == "remove" && *r != "") {
			return errors.New(usage)
		}
		reason = strings.TrimSpace(*r)
	case "list":
		if len(args) != 0 {
			return errors.New(usage)
		}
	default:
		return errors.New(usage)
	}
	pool, err := open(ctx, getenv)
	if err != nil {
		return err
	}
	defer pool.Close()
	return excludeOn(ctx, store.NewStore(pool).Queries, cmd, name, reason, out)
}

func excludeOn(ctx context.Context, q *store.Queries, cmd, name, reason string, out io.Writer) error {
	switch cmd {
	case "add":
		var r *string
		if reason != "" {
			r = &reason
		}
		if err := q.AddScanExclusion(ctx, store.AddScanExclusionParams{Name: name, Reason: r}); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "excluded from public scans: %s (and its subdomains)\n", name)
		return err
	case "remove":
		n, err := q.RemoveScanExclusion(ctx, name)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%s is not on the list", name)
		}
		_, err = fmt.Fprintf(out, "removed from the exclusion list: %s\n", name)
		return err
	default:
		rows, err := q.ListScanExclusions(ctx)
		if err != nil {
			return err
		}
		for _, e := range rows {
			r := ""
			if e.Reason != nil {
				r = *e.Reason
			}
			fmt.Fprintf(out, "%s\t%s\t%s\n", e.Name, e.CreatedAt.UTC().Format("2006-01-02"), r)
		}
		return nil
	}
}
