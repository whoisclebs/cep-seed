package importer

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB creates an empty SQLite database at the given path and
// returns an open connection.
func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// execTest runs a SQL statement on the given DB, failing the test on error.
func execTest(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err != nil {
		t.Fatalf("exec %q: %v", stmt, err)
	}
}

// createMinimalSchemaV1 creates tables that match the structure expected by
// validateSchemaContract, including the GENERATED formatted_postal_code column
// and the address_type CHECK constraint.
func createMinimalSchemaV1(t *testing.T, db *sql.DB, version int) {
	t.Helper()
	execTest(t, db, `CREATE TABLE _migrations (version INTEGER PRIMARY KEY, applied_at TEXT)`)
	execTest(t, db, `INSERT INTO _migrations (version, applied_at) VALUES (?, 'now')`, version)
	execTest(t, db, `CREATE TABLE countries (code TEXT PRIMARY KEY, name TEXT NOT NULL)`)
	execTest(t, db, `CREATE TABLE states (code TEXT PRIMARY KEY, name TEXT NOT NULL, region TEXT NOT NULL, country_code TEXT NOT NULL REFERENCES countries(code))`)
	execTest(t, db, `CREATE TABLE localities (id TEXT PRIMARY KEY, name TEXT NOT NULL, state_code TEXT NOT NULL REFERENCES states(code), ibge_code TEXT NULL)`)
	execTest(t, db, `CREATE TABLE neighborhoods (id TEXT PRIMARY KEY, name TEXT NOT NULL, locality_id TEXT NOT NULL REFERENCES localities(id))`)
	execTest(t, db, `CREATE TABLE postal_codes (
		postal_code TEXT PRIMARY KEY NOT NULL,
		formatted_postal_code TEXT GENERATED ALWAYS AS (substr(postal_code,1,5)||'-'||substr(postal_code,6,3)) STORED,
		street TEXT NULL,
		additional_information TEXT NULL,
		state_code TEXT NOT NULL REFERENCES states(code),
		locality_id TEXT NULL REFERENCES localities(id),
		neighborhood_id TEXT NULL REFERENCES neighborhoods(id),
		address_type TEXT NOT NULL CHECK (address_type IN ('STREET','LOCALITY','LARGE_USER','POSTAL_UNIT','COMMUNITY_POSTAL_BOX')),
		postal_unit TEXT NULL,
		CHECK (length(postal_code) = 8)
	)`)
	execTest(t, db, `CREATE TABLE dataset_metadata (id INTEGER PRIMARY KEY, source_release TEXT NOT NULL, built_at TEXT NOT NULL)`)
}

// TestValidateSchemaContract_Missing tests that a database with no
// _migrations table at all is rejected. The database file must be valid
// and openable but missing the _migrations table.
func TestValidateSchemaContract_Missing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db := openTestDB(t, path)
	// Create a table so the database file exists and is openable,
	// but omit _migrations.
	execTest(t, db, `CREATE TABLE some_table (id INTEGER PRIMARY KEY)`)
	db.Close()

	err := validateSchemaContract(ctx, path)
	if err == nil {
		t.Fatal("expected error for database with no _migrations table")
	}
	if !strings.Contains(err.Error(), "no _migrations table") {
		t.Errorf("error should mention missing _migrations table, got: %v", err)
	}
}

// TestValidateSchemaContract_VersionZero tests that a database with
// _migrations version 0 is rejected with a clear error.
func TestValidateSchemaContract_VersionZero(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db := openTestDB(t, path)
	createMinimalSchemaV1(t, db, 0)
	db.Close()

	err := validateSchemaContract(ctx, path)
	if err == nil {
		t.Fatal("expected error for version 0")
	}
	if !strings.Contains(err.Error(), "version 0") {
		t.Errorf("error should mention version 0, got: %v", err)
	}
}

// TestValidateSchemaContract_FutureVersion tests that a database with
// migration version > 1 (e.g., version 2) is rejected with a clear error.
func TestValidateSchemaContract_FutureVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db := openTestDB(t, path)
	createMinimalSchemaV1(t, db, 2)
	db.Close()

	err := validateSchemaContract(ctx, path)
	if err == nil {
		t.Fatal("expected error for future schema version")
	}
	if !strings.Contains(err.Error(), "version 2") || !strings.Contains(err.Error(), "only version 1") {
		t.Errorf("error should mention version 2 and 'only version 1', got: %v", err)
	}
}

// TestValidateSchemaContract_ValidVersion1 verifies that a database with
// the correct migration version 1 and all required schema elements passes.
func TestValidateSchemaContract_ValidVersion1(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db := openTestDB(t, path)
	createMinimalSchemaV1(t, db, 1)
	db.Close()

	err := validateSchemaContract(ctx, path)
	if err != nil {
		t.Errorf("unexpected error for valid v1 schema: %v", err)
	}
}
