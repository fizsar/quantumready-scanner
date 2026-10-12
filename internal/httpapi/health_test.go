package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakePinger struct {
	err   error
	block bool // wait until the context is done, then return its error
}

func (f fakePinger) Ping(ctx context.Context) error {
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func newTestRouter(db Pinger) http.Handler {
	return NewRouter(Config{
		Version:     "1.2.3-test",
		DB:          db,
		PingTimeout: 50 * time.Millisecond,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func doRequest(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		db         Pinger
		wantStatus int
		wantBody   healthResponse
	}{
		{
			name:       "database reachable",
			db:         fakePinger{},
			wantStatus: http.StatusOK,
			wantBody:   healthResponse{Status: "ok", Version: "1.2.3-test"},
		},
		{
			name:       "database error",
			db:         fakePinger{err: errors.New("dial tcp 127.0.0.1:5432: connection refused")},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   healthResponse{Status: "unavailable", Version: "1.2.3-test"},
		},
		{
			name:       "database ping times out",
			db:         fakePinger{block: true},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   healthResponse{Status: "unavailable", Version: "1.2.3-test"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			rec := doRequest(t, newTestRouter(tt.db), http.MethodGet, "/api/health")
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Errorf("request took %v, want it bounded by the ping timeout", elapsed)
			}

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type = %q", ct)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", cc)
			}
			if xcto := rec.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want nosniff", xcto)
			}

			// Decode strictly so that leaking extra fields (e.g. error details) fails the test.
			dec := json.NewDecoder(rec.Body)
			dec.DisallowUnknownFields()
			var got healthResponse
			if err := dec.Decode(&got); err != nil {
				t.Fatalf("decode body %q: %v", rec.Body.String(), err)
			}
			if got != tt.wantBody {
				t.Errorf("body = %+v, want %+v", got, tt.wantBody)
			}
		})
	}
}

func TestHealthRouting(t *testing.T) {
	h := newTestRouter(fakePinger{})

	if rec := doRequest(t, h, http.MethodPost, "/api/health"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/health status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if rec := doRequest(t, h, http.MethodGet, "/api/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /api/nope status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
