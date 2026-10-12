package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveStatus(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	return ":" + port // the API usually listens on ":8080" inside the container
}

func TestHealthcheck(t *testing.T) {
	if err := healthcheck(serveStatus(t, http.StatusOK)); err != nil {
		t.Errorf("healthy API: got %v, want nil", err)
	}
	if err := healthcheck(serveStatus(t, http.StatusServiceUnavailable)); err == nil {
		t.Error("503 from API: got nil, want error")
	}
	if err := healthcheck("not-an-addr"); err == nil {
		t.Error("bad API_ADDR: got nil, want error")
	}

	// Nothing listening on the port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	if err := healthcheck(addr); err == nil {
		t.Error("API down: got nil, want error")
	}
}
