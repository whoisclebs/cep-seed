package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// writeDatabase copies an existing API-initialized database, validates the
// schema contract, clears data tables, inserts new reference data and postal
// codes, updates dataset_metadata, validates integrity, and then atomically
// promotes the copy to the configured output path with rollback preservation.
func writeDatabase(ctx context.Context, config ImportConfig, results []fileResult,
	canonical map[string]canonicalRow, accepted map[string]int, warnings []UnresolvedReference,
	builtAt string, start time.Time) (*Report, error) {

	if err := validateSchemaContract(ctx, config.OutPath); err != nil {
		return nil, fmt.Errorf("schema validation failed: %w", err)
	}

	newPath := config.OutPath + ".new"

	if err := copyFile(config.OutPath, newPath); err != nil {
		return nil, fmt.Errorf("copy database: %w", err)
	}

	if err := clearDataTables(ctx, newPath); err != nil {
		os.Remove(newPath)
		return nil, fmt.Errorf("clear data tables: %w", err)
	}

	if err := insertReferenceData(ctx, newPath, results); err != nil {
		os.Remove(newPath)
		return nil, fmt.Errorf("insert reference data: %w", err)
	}

	if err := insertData(ctx, newPath, canonical, config.Release, builtAt); err != nil {
		os.Remove(newPath)
		return nil, fmt.Errorf("insert data: %w", err)
	}

	integrity, err := checkIntegrity(ctx, newPath)
	if err != nil {
		os.Remove(newPath)
		return nil, fmt.Errorf("integrity check: %w", err)
	}

	fi, err := os.Stat(newPath)
	if err != nil {
		return nil, fmt.Errorf("stat database: %w", err)
	}

	if err := atomicReplace(newPath, config.OutPath); err != nil {
		return nil, fmt.Errorf("atomic replace: %w", err)
	}

	return &Report{
		ConsumedFiles: consumedFiles(results),
		Accepted:      accepted,
		Rejected:      0,
		Collisions:    nil,
		Warnings:      warnings,
		DBSize:        fi.Size(),
		Integrity:     integrity,
	}, nil
}

// consumedFiles extracts sorted basenames from file results.
func consumedFiles(results []fileResult) []string {
	var names []string
	for _, result := range results {
		names = append(names, filepath.Base(result.srcPath))
	}
	sort.Strings(names)
	return names
}

// TotalAccepted sums counts across a type→count map.
func TotalAccepted(accepted map[string]int) int {
	total := 0
	for _, count := range accepted {
		total += count
	}
	return total
}

// formatElapsed returns a human-readable duration string.
func formatElapsed(d time.Duration) string {
	return d.Truncate(time.Millisecond).String()
}
