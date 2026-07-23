// Command edne-import builds the CEP lookup database from eDNE Básico
// delimited source files.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/whoisclebs/cep-seed/internal/importer"
)

func main() {
	src := flag.String("src", "", "source directory with eDNE delimited files")
	out := flag.String("out", "", "output database path")
	rel := flag.String("release", "", "eDNE source release identifier")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if *src == "" {
		*src = os.Getenv("CEP_EDNE_DIR")
	}
	if *src == "" || *out == "" || *rel == "" {
		fmt.Fprintln(os.Stderr, "usage: edne-import --src=<edne-dir> --out=<output-path> --release=<release-id>")
		os.Exit(2)
	}

	imp := importer.New(importer.ImportConfig{SrcDir: *src, OutPath: *out, Release: *rel})
	report, err := imp.Run(context.Background())
	if err != nil {
		if report != nil {
			json.NewEncoder(os.Stdout).Encode(report) //nolint:errcheck
		}
		slog.Error("import failed", "error", err)
		os.Exit(1)
	}
	json.NewEncoder(os.Stdout).Encode(report) //nolint:errcheck
	slog.Info("import completed",
		"records", importer.TotalAccepted(report.Accepted),
		"warnings", len(report.Warnings), "db_size", report.DBSize, "elapsed", report.Elapsed)
}
