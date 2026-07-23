package importer

import (
	"context"
	"database/sql"
	"fmt"
)

// insertReferenceData seeds the localities and neighborhoods tables from
// the parsed eDNE results. Countries and states are already seeded by the
// API migration DDL and are preserved across updates.
func insertReferenceData(ctx context.Context, path string, results []fileResult) error {
	localities, neighborhoods := buildReferenceRecords(results)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open db for references: %w", err)
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

	localityStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO localities (id, name, state_code, ibge_code, gia_code, siafi_code, area_code)
		VALUES (?, ?, ?, ?, NULL, NULL, NULL)`)
	if err != nil {
		return fmt.Errorf("prepare locality insert: %w", err)
	}
	defer localityStmt.Close()

	for _, locality := range localities {
		var ibge any
		if locality.IBGE != "" {
			ibge = locality.IBGE
		}
		if _, err := localityStmt.ExecContext(ctx, locality.LocalidadeID, locality.Localidade, locality.UF, ibge); err != nil {
			return fmt.Errorf("insert locality %s: %w", locality.LocalidadeID, err)
		}
	}

	neighborhoodStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO neighborhoods (id, name, locality_id) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare neighborhood insert: %w", err)
	}
	defer neighborhoodStmt.Close()

	for _, neighborhood := range neighborhoods {
		if _, err := neighborhoodStmt.ExecContext(ctx, neighborhood.BairroID, neighborhood.Bairro, neighborhood.LocalidadeID); err != nil {
			return fmt.Errorf("insert neighborhood %s: %w", neighborhood.BairroID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reference inserts: %w", err)
	}
	return nil
}
