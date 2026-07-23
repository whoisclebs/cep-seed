package importer

import (
	"context"
	"database/sql"
	"fmt"
)

// clearDataTables removes data from all import-populated tables in FK-safe
// order (children first, then parents), preserving countries, states,
// _migrations, and schema structure. Runs inside a single transaction.
func clearDataTables(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open db for clear: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign_keys: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	clearOrder := []string{
		"DELETE FROM postal_codes",
		"DELETE FROM neighborhoods",
		"DELETE FROM localities",
		"DELETE FROM dataset_metadata",
	}
	for _, stmt := range clearOrder {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("clear table (%s): %w", stmt, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit clear: %w", err)
	}
	return nil
}
