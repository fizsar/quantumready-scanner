package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func demoParams(name string) InsertDemoRequestParams {
	return InsertDemoRequestParams{Name: name, Email: "ana@example.org", Company: "Acme", Domain: "example.org",
		Message: "", Lang: "es", PrivacyVersion: "2026-10-20"}
}

// Demo requests: stored, listed newest first, read, deleted on request and
// purged after keep_months.
func TestDemoRequests(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	var ids []string
	for _, n := range []string{"Old", "Almost", "New"} {
		r, err := st.InsertDemoRequest(ctx, demoParams(n))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID.String())
	}
	// Age them: 13 months, 11 months and now.
	for i, age := range []string{"13 months", "11 months"} {
		if _, err := st.pool.Exec(ctx, `UPDATE demo_requests SET created_at = now() - $1::interval WHERE id = $2`, age, ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	list, err := st.ListDemoRequests(ctx, 10)
	if err != nil || len(list) != 3 || list[0].Name != "New" || list[2].Name != "Old" {
		t.Fatalf("list: %v %v", list, err)
	}
	if n, err := st.PurgeDemoRequests(ctx, 12); err != nil || n != 1 {
		t.Errorf("purge: %d %v, want 1 (13 months old)", n, err)
	}
	got, err := st.GetDemoRequest(ctx, list[1].ID)
	if err != nil || got.Name != "Almost" || got.PrivacyVersion != "2026-10-20" {
		t.Errorf("get: %+v %v", got, err)
	}
	if n, err := st.DeleteDemoRequest(ctx, list[1].ID); err != nil || n != 1 {
		t.Errorf("delete: %d %v", n, err)
	}
	if _, err := st.GetDemoRequest(ctx, list[1].ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("after delete: %v", err)
	}
	if n, _ := st.DeleteDemoRequest(ctx, list[1].ID); n != 0 {
		t.Error("deleting twice deleted something")
	}
	// The database refuses what the API would never send.
	for name, p := range map[string]InsertDemoRequestParams{
		"empty name":   demoParams(""),
		"long message": func() InsertDemoRequestParams { p := demoParams("x"); p.Message = string(make([]byte, 2001)); return p }(),
		"other lang":   func() InsertDemoRequestParams { p := demoParams("x"); p.Lang = "fr"; return p }(),
		"no privacy":   func() InsertDemoRequestParams { p := demoParams("x"); p.PrivacyVersion = ""; return p }(),
	} {
		if _, err := st.InsertDemoRequest(ctx, p); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
