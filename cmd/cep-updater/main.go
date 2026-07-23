// Command cep-updater downloads, extracts, and imports the current
// eDNE Básico release atomically. One-shot, one update per execution.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/whoisclebs/cep-seed/internal/config"
	"github.com/whoisclebs/cep-seed/internal/updater"
)

const defaultEDNEURL = "https://www2.correios.com.br/sistemas/edne/download/eDNE_Basico.zip"

func main() {
	src := flag.String("source", config.EnvDefault("CEP_EDNE_URL", defaultEDNEURL), "URL of the outer eDNE ZIP archive")
	db := flag.String("db", config.EnvDefault("CEP_DB_PATH", "/data/cep.db"), "path to the CEP database")
	dd := flag.String("data-dir", "/tmp/cep-updater", "temporary directory for extraction")
	to := flag.Duration("timeout", 5*time.Minute, "HTTP download timeout")
	flag.Parse()

	// Validate env-var duration override (flag default is plain; env var must be valid).
	envTimeout, err := config.ParseEnvDuration("CEP_DOWNLOAD_TIMEOUT")
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if envTimeout > 0 {
		*to = envTimeout
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	r := updater.New(updater.Config{SourceURL: *src, DBPath: *db, DataDir: *dd, Timeout: *to}).Run(ctx)

	// Usage signal on stdout (preserving JSON stdout for Report).
	switch r.Result {
	case updater.ResultSuccess:
		os.Exit(0)
	case updater.ResultSkip:
		os.Exit(0)
	default:
		slog.Error("update failed", "error", r.Error)
		os.Exit(1)
	}
}
