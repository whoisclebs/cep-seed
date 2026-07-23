package importer

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// insertData inserts the metadata row and all canonical postal code records
// into the database at path.
func insertData(ctx context.Context, path string, canonical map[string]canonicalRow,
	release, builtAt string) error {

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open db for insert: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("enable foreign_keys: %w", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO dataset_metadata (id, source_release, built_at) VALUES (1, ?, ?)`,
		release, builtAt); err != nil {
		return fmt.Errorf("insert metadata: %w", err)
	}

	codes := make([]string, 0, len(canonical))
	for postalCode := range canonical {
		codes = append(codes, postalCode)
	}
	sort.Strings(codes)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	insertStmt, err := tx.PrepareContext(ctx, `INSERT INTO postal_codes (postal_code, street, additional_information, state_code, locality_id, neighborhood_id, address_type, postal_unit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer insertStmt.Close()

	for _, code := range codes {
		row := canonical[code]
		var street, additionalInformation, localityID, neighborhoodID, postalUnit any
		if row.street != "" {
			street = row.street
		}
		if row.additionalInformation != "" {
			additionalInformation = row.additionalInformation
		}
		if row.localityID != "" {
			localityID = row.localityID
		}
		if row.neighborhoodID != "" {
			neighborhoodID = row.neighborhoodID
		}
		if row.postalUnit != "" {
			postalUnit = row.postalUnit
		}
		if _, err := insertStmt.ExecContext(ctx,
			row.postalCode, street, additionalInformation,
			row.stateCode, localityID, neighborhoodID, row.addressType, postalUnit,
		); err != nil {
			return fmt.Errorf("insert %s: %w", code, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("checkpoint wal: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = DELETE"); err != nil {
		return fmt.Errorf("finalize journal mode: %w", err)
	}
	return nil
}
