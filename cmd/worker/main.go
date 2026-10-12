// Command worker processes scan jobs from the PostgreSQL queue.
//
// Placeholder: the job loop arrives with the scanning engine (phase 2).
package main

import (
	"log/slog"
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	logger.Info("worker not implemented yet", "version", version)
}
