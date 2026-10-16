package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// queuedOrgScan adds a queued organization scan (it goes before public ones).
func (a *authAPI) queuedOrgScan(t *testing.T, org uuid.UUID, host string) {
	t.Helper()
	if _, err := a.owner.Exec(context.Background(),
		"INSERT INTO scans (host, port, organization_id, intensity) VALUES ($1, 443, $2, 'full')", host, org); err != nil {
		t.Fatal(err)
	}
}

func TestPublicQueueCapAndPosition(t *testing.T) {
	a := newAuthAPI(t, AuthLimits{EmailIP: NewLimiter(100, 100)}, openPublic(PublicLimits{}),
		func(c *Config) { c.PublicQueueMax = 2; c.ScansPerMinute = 20 })
	_, me := a.signedIn(t, "ana@example.com", "203.0.113.1")
	org := *me.ActiveOrgID
	for i := range 3 {
		a.queuedOrgScan(t, org, "org"+string(rune('a'+i))+".example.com") // waiting before any public scan
	}
	c := a.client("198.51.100.20")
	pub := func(host string) *httptestRecorder {
		return c.do(t, "POST", "/api/scans", js(map[string]string{"host": host}))
	}
	first := viewOf(t, pub("one.example.org"))
	second := viewOf(t, pub("two.example.org"))
	// Positions count public scans only: organizations' scans change nothing.
	if first.QueuePosition == nil || *first.QueuePosition != 1 || second.QueuePosition == nil || *second.QueuePosition != 2 {
		t.Fatalf("positions: %v %v", first.QueuePosition, second.QueuePosition)
	}
	if second.QueueETASeconds == nil || *second.QueueETASeconds != 6 { // 2 scans at 20 per minute
		t.Errorf("eta: %v", second.QueueETASeconds)
	}
	a.queuedOrgScan(t, org, "orgz.example.com")
	w := c.do(t, "GET", "/api/scans/"+second.ID.String(), "")
	if v := viewOf(t, w); v.QueuePosition == nil || *v.QueuePosition != 2 {
		t.Errorf("position after another organization scan: %s", w.Body)
	}

	// The public queue is full: busy, without spending quota; reuse still works.
	w = pub("three.example.org")
	expect(t, w, http.StatusTooManyRequests, "busy")
	if w.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After %q", w.Header().Get("Retry-After"))
	}
	expect(t, pub("one.example.org"), http.StatusOK, "")

	// A scan that started is no longer counted.
	if _, err := a.owner.Exec(context.Background(), `UPDATE scans SET status = 'running', started_at = now(), attempts = 1,
		lease_token = gen_random_uuid(), lease_until = now() + interval '1 minute' WHERE id = $1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if v := viewOf(t, c.do(t, "GET", "/api/scans/"+second.ID.String(), "")); v.QueuePosition == nil || *v.QueuePosition != 1 {
		t.Errorf("position after the first started: %v", v.QueuePosition)
	}
	expect(t, pub("three.example.org"), http.StatusAccepted, "")
}
