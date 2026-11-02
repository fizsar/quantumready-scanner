package store

import "context"

// The kinds of refused public scan requests: the closed list of migration
// 00022, in its order. Nothing else is ever written.
const (
	RefusedClientLimit = "client_limit" // 429 rate_limited
	RefusedDomainLimit = "domain_limit" // 429 domain_rate_limited
	RefusedGlobalLimit = "global_limit" // 429 busy (global limit)
	RefusedQueueFull   = "queue_full"   // 429 busy (public queue full)
	RefusedExcluded    = "excluded"     // 403 scan_not_allowed (exclusion list or opt-out)
	RefusedProofFailed = "proof_failed" // 400 proof_failed
)

// RefusalKinds lists them in that order.
var RefusalKinds = []string{RefusedClientLimit, RefusedDomainLimit, RefusedGlobalLimit, RefusedQueueFull, RefusedExcluded, RefusedProofFailed}

// AddScanRefusals adds n refused public scan requests of a kind to today's
// count (UTC; migration 00022). It calls add_scan_refusals, which only qr_app
// may execute: the API flushes its counts every few seconds. The kind must be
// one of the closed list of the migration; nothing about who asked is stored.
func (q *Queries) AddScanRefusals(ctx context.Context, kind string, n int64) error {
	_, err := q.db.Exec(ctx, "SELECT add_scan_refusals($1, $2)", kind, n)
	return err
}
