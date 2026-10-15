package store

import (
	"context"

	"github.com/google/uuid"
)

// InsertFindingsParams is one finding to store with a completed scan.
type InsertFindingsParams struct {
	ScanID   uuid.UUID
	Position int32
	Code     string
	Axis     string
	Severity string
	Category string
	Params   []byte
}

// InsertFindings stores findings in a single statement (InsertFindingsBatch).
func (q *Queries) InsertFindings(ctx context.Context, rows []InsertFindingsParams) error {
	if len(rows) == 0 {
		return nil
	}
	var b InsertFindingsBatchParams
	for _, r := range rows {
		b.ScanIds = append(b.ScanIds, r.ScanID)
		b.Positions = append(b.Positions, r.Position)
		b.Codes = append(b.Codes, r.Code)
		b.Axes = append(b.Axes, r.Axis)
		b.Severities = append(b.Severities, r.Severity)
		b.Categories = append(b.Categories, r.Category)
		b.Params = append(b.Params, r.Params)
	}
	return q.InsertFindingsBatch(ctx, b)
}
