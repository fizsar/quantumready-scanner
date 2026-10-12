package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// healthcheck probes GET /api/health on the local API. It backs the container
// HEALTHCHECK (`api healthcheck`), since the distroless image has no shell or curl.
func healthcheck(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse API_ADDR %q: %w", addr, err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", port) + "/api/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status %d", resp.StatusCode)
	}
	return nil
}
