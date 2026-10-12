package httpapi

import (
	"context"
	"net/http"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// healthHandler serves GET /api/health. It answers 200 {"status":"ok"} when the
// database responds within cfg.PingTimeout and 503 {"status":"unavailable"}
// otherwise. Error details are logged, never returned to the client.
func healthHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), cfg.PingTimeout)
		defer cancel()

		resp := healthResponse{Status: "ok", Version: cfg.Version}
		status := http.StatusOK
		if err := cfg.DB.Ping(ctx); err != nil {
			cfg.Logger.WarnContext(r.Context(), "health: database ping failed", "err", err)
			resp.Status = "unavailable"
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, resp)
	}
}
