package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/fizsar/quantumready-saas/internal/domains"
	"github.com/fizsar/quantumready-saas/internal/store"
)

// optout lists or lifts owners' opt-outs from public scans. An opt-out stays
// in force even when the domain is no longer verified; only its owner
// (verified again) or the operator, here, can lift it.
func optout(ctx context.Context, args []string, getenv func(string) string, out io.Writer) error {
	var name string
	switch {
	case len(args) == 1 && args[0] == "list":
	case len(args) == 2 && args[0] == "clear":
		n, err := domains.Normalize(args[1])
		if err != nil {
			return fmt.Errorf("%s: %w", args[1], err)
		}
		name = n
	default:
		return errors.New(usage)
	}
	pool, err := open(ctx, getenv)
	if err != nil {
		return err
	}
	defer pool.Close()
	return optoutOn(ctx, store.NewStore(pool).Queries, args[0], name, out)
}

func optoutOn(ctx context.Context, q *store.Queries, cmd, name string, out io.Writer) error {
	if cmd == "clear" {
		n, err := q.ClearOptOut(ctx, name)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%s has no owner opt-out (see qradmin exclude list for operator exclusions)", name)
		}
		_, err = fmt.Fprintf(out, "owner opt-out lifted for %s (%d organization(s))\n", name, n)
		return err
	}
	rows, err := q.ListOptOuts(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		fmt.Fprintf(out, "%s\t%s\t%s\n", r.Name, r.Status, r.OrganizationID)
	}
	return nil
}
