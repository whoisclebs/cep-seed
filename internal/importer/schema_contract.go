package importer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/whoisclebs/cep-seed/internal/apperror"
)

// validateSchemaContract checks that the database at path has been initialized
// by the API: expected migration version 1, required tables exist, required
// columns exist, and the generated column formatted_postal_code is present.
func validateSchemaContract(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return apperror.Wrapf(apperror.KindSchema, err, "open database %s for validation", path)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return apperror.Newf(apperror.KindSchema, "cannot connect to database %s — has the API initialized it?", path)
	}

	var tblName string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='_migrations'").Scan(&tblName)
	if err != nil {
		return apperror.Newf(apperror.KindSchema, "database %s has no _migrations table — run the API to initialize the schema first", path)
	}

	var version int
	err = db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM _migrations").Scan(&version)
	if err != nil {
		return apperror.Wrapf(apperror.KindSchema, err, "read migration version from %s", path)
	}
	switch {
	case version == 0:
		return apperror.Newf(apperror.KindSchema, "database %s has no migration applied (version 0) — run the API to initialize the schema first", path)
	case version > 1:
		return apperror.Newf(apperror.KindSchema, "database %s has migration version %d, but only version 1 is supported", path, version)
	}

	requiredTables := []string{"countries", "states", "localities", "neighborhoods", "postal_codes", "dataset_metadata"}
	for _, table := range requiredTables {
		var name string
		err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			return apperror.Newf(apperror.KindSchema, "required table %q not found in %s — run the API to initialize the schema first", table, path)
		}
	}

	requiredCols := []string{"postal_code", "street", "state_code", "locality_id", "neighborhood_id", "address_type"}
	colMap := make(map[string]bool)
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(postal_codes)")
	if err != nil {
		return apperror.Wrapf(apperror.KindSchema, err, "read postal_codes columns from %s", path)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return apperror.Wrapf(apperror.KindSchema, err, "scan column info")
		}
		colMap[name] = true
	}
	for _, col := range requiredCols {
		if !colMap[col] {
			return apperror.Newf(apperror.KindSchema, "required column %q not found in postal_codes table — schema version mismatch", col)
		}
	}

	var genColCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='postal_codes'
		AND sql LIKE '%formatted_postal_code%'`).Scan(&genColCount)
	if err != nil {
		return apperror.Wrapf(apperror.KindSchema, err, "verify generated column")
	}
	if genColCount == 0 {
		return apperror.Newf(apperror.KindSchema, "generated column formatted_postal_code not found in postal_codes — run the API to initialize the schema first")
	}

	return nil
}
