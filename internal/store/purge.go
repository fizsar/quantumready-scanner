package store

import "context"

// PurgeExpiredPublicScans deletes the finished public scans past their expiry
// (30 days; migration 00019) with their findings, and returns how many. It
// calls purge_expired_public_scans, which only qr_worker may execute: the
// worker runs it once a day.
func (q *Queries) PurgeExpiredPublicScans(ctx context.Context) (int64, error) {
	var n int64
	err := q.db.QueryRow(ctx, "SELECT purge_expired_public_scans()").Scan(&n)
	return n, err
}
