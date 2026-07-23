package importer_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whoisclebs/cep-seed/internal/apperror"
	"github.com/whoisclebs/cep-seed/internal/importer"
	_ "modernc.org/sqlite"
)

// fixturePath points to the synthetic schema-v1 fixture generated from the
// real cep-api migration DDL. Contains schema (1 country, 27 states, _migrations v1)
// but NO postal records or eDNE data. See testdata/.fixture-provenance.md.
var fixturePath = filepath.Join("..", "..", "testdata", "empty-schema-v1.db")

// copyFixtureDB copies the schema-v1 fixture to dstPath. The resulting database
// has a valid v1 schema (countries, states, _migrations v1) with no data rows.
// Tests insert only the records they need on top of this skeleton.
func copyFixtureDB(t *testing.T, dstPath string) {
	t.Helper()
	src, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v — has the fixture been generated? see testdata/.fixture-provenance.md", fixturePath, err)
	}
	if err := os.WriteFile(dstPath, src, 0644); err != nil {
		t.Fatalf("write fixture to %s: %v", dstPath, err)
	}
}

// seedFixtureDB copies the fixture to dstPath, then inserts a test locality
// and dataset_metadata row so the DB is ready for import. The inserted metadata
// has the given release.
func seedFixtureDB(t *testing.T, dstPath, release string) {
	t.Helper()
	copyFixtureDB(t, dstPath)

	db, err := sql.Open("sqlite", dstPath)
	if err != nil {
		t.Fatalf("open seeded db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO localities (id, name, state_code, ibge_code) VALUES ('3550308', 'Sao Paulo', 'SP', '3550308')`); err != nil {
		t.Fatalf("insert locality: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO dataset_metadata (id, source_release, built_at) VALUES (1, ?, '2026-01-01T00:00:00Z')`, release); err != nil {
		t.Fatalf("insert metadata: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO postal_codes (postal_code, state_code, locality_id, address_type) VALUES ('00000000', 'SP', '3550308', 'STREET')`); err != nil {
		t.Fatalf("insert placeholder: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed data: %v", err)
	}
}

// fullSeedFixtureDB copies the fixture and inserts all reference data needed
// for the full pipeline test: localities (SP, RJ, DF, AC), neighborhoods, and
// a placeholder postal record.
func fullSeedFixtureDB(t *testing.T, dstPath string) {
	t.Helper()
	copyFixtureDB(t, dstPath)

	db, err := sql.Open("sqlite", dstPath)
	if err != nil {
		t.Fatalf("open seeded db: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign_keys: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	// Insert localities for test.
	for _, loc := range []struct{ id, name, state, ibge string }{
		{"3550308", "Sao Paulo", "SP", "3550308"},
		{"3304557", "Rio de Janeiro", "RJ", "3304557"},
		{"5300108", "Brasilia", "DF", "5300108"},
		{"16", "Rio Branco", "AC", "1200401"},
	} {
		if _, err := tx.Exec(`INSERT INTO localities (id, name, state_code, ibge_code) VALUES (?, ?, ?, ?)`, loc.id, loc.name, loc.state, loc.ibge); err != nil {
			t.Fatalf("insert locality %s: %v", loc.id, err)
		}
	}

	if _, err := tx.Exec(`INSERT INTO dataset_metadata (id, source_release, built_at) VALUES (1, 'init', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert metadata: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO postal_codes (postal_code, state_code, locality_id, address_type) VALUES ('00000000', 'SP', '3550308', 'STREET')`); err != nil {
		t.Fatalf("insert placeholder: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit full seed data: %v", err)
	}
}

// writeFixture writes a string to a file in dir.
func writeFixture(test *testing.T, dir, name, content string) {
	test.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		test.Fatalf("write fixture %s: %v", name, err)
	}
}

// ── Fixture contract test ────────────────────────────────────────

// TestFixture_Contract validates that the empty-schema-v1.db fixture has the
// expected schema version, required tables, required columns, country/state
// seed data, and absence of postal records. This detects an invalid or
// stale fixture without requiring a checksum from the user.
func TestFixture_Contract(t *testing.T) {
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("fixture not found at %s — see testdata/.fixture-provenance.md for regeneration instructions", fixturePath)
	}
	if len(fixture) < 4096 {
		t.Fatalf("fixture suspiciously small (%d bytes), expected a valid SQLite database", len(fixture))
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	if err := os.WriteFile(dbPath, fixture, 0644); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	// Check _migrations has version 1.
	var version int
	if err := db.QueryRowContext(ctx, "SELECT version FROM _migrations ORDER BY version DESC LIMIT 1").Scan(&version); err != nil {
		t.Fatalf("read migration version: %v — fixture has no _migrations table", err)
	}
	if version != 1 {
		t.Errorf("fixture migration version = %d, want 1", version)
	}

	// Check required tables exist.
	for _, table := range []string{"countries", "states", "localities", "neighborhoods", "postal_codes", "dataset_metadata", "_migrations"} {
		var name string
		if err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
			t.Errorf("required table %q not found in fixture", table)
		}
	}

	// Check country seed.
	var countryCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM countries").Scan(&countryCount); err != nil {
		t.Fatalf("query countries: %v", err)
	}
	if countryCount != 1 {
		t.Errorf("expected 1 country in fixture, got %d", countryCount)
	}

	// Check state seed.
	var stateCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM states").Scan(&stateCount); err != nil {
		t.Fatalf("query states: %v", err)
	}
	if stateCount != 27 {
		t.Errorf("expected 27 states in fixture, got %d", stateCount)
	}

	// Verify NO postal records — fixture must be clean.
	var pcCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM postal_codes").Scan(&pcCount); err != nil {
		t.Fatalf("query postal_codes: %v", err)
	}
	if pcCount != 0 {
		t.Errorf("fixture should have 0 postal records, got %d", pcCount)
	}

	// Verify NO dataset_metadata — fixture must be clean.
	var metaCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dataset_metadata").Scan(&metaCount); err != nil {
		t.Fatalf("query dataset_metadata: %v", err)
	}
	if metaCount != 0 {
		t.Errorf("fixture should have 0 metadata rows, got %d", metaCount)
	}

	// Verify integrity.
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatalf("integrity check: %v", err)
	}
	if integrity != "ok" {
		t.Errorf("fixture integrity = %q, want ok", integrity)
	}

	// Verify required columns on postal_codes.
	requiredCols := map[string]bool{"postal_code": false, "street": false, "state_code": false, "locality_id": false, "neighborhood_id": false, "address_type": false}
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(postal_codes)")
	if err != nil {
		t.Fatalf("pragma table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if _, ok := requiredCols[name]; ok {
			requiredCols[name] = true
		}
	}
	for col, found := range requiredCols {
		if !found {
			t.Errorf("required column %q not found in postal_codes", col)
		}
	}
}

// ── Import pipeline tests ─────────────────────────────────────────

// TestImporter_FullPipeline creates synthetic fixtures for all supported
// record classes and verifies the complete import pipeline.
func TestImporter_FullPipeline(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	// LOG_LOCALIDADE.TXT: id@UF@nome@CEP@situacao@tipo@alias@abrev@IBGE
	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n"+
			"3304557@RJ@Rio de Janeiro@20040002@1@M@@Rio de Janeiro@3304557\n"+
			"5300108@DF@Brasilia@@1@M@@Brasilia@5300108\n"+
			"16@AC@Rio Branco@@1@M@@Rio Branco@1200401\n")

	// LOG_BAIRRO.TXT: id@UF@loc@nome@abrev
	writeFixture(test, srcDir, "LOG_BAIRRO.TXT",
		"14716@SP@3550308@Se@Se\n")

	// LOG_LOGRADOURO_SP.TXT (11 fields): id@UF@loc@bairro@reservado@nome@complemento@CEP@tipo@indicador@abrev
	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		"1001235@SP@3550308@14716@@Praca da Se@lado impar@01001001@Rua@S@Pc. da Se\n"+
			"1001236@SP@3550308@@@Rua Maria Antonia@@01002000@Rua@S@R. Maria Antonia\n")

	// LOG_GRANDE_USUARIO.TXT: id@UF@seq@loc@num@nome@endereco@CEP@abrev
	writeFixture(test, srcDir, "LOG_GRANDE_USUARIO.TXT",
		"33087@DF@1@5300108@@Empresa XYZ@SBS Quadra 1 Bloco K@70002900@Empresa XYZ\n")

	// LOG_UNID_OPER.TXT: id@UF@seq@loc@num@nome@endereco@CEP@indicador@abrev
	writeFixture(test, srcDir, "LOG_UNID_OPER.TXT",
		"12036@DF@3@5300108@@Agencia Central@Setor de Autarquias Sul, Qd 5@70010900@S@Ag. Central\n")

	// LOG_CPC.TXT: id@UF@loc@nome@endereco@CEP
	writeFixture(test, srcDir, "LOG_CPC.TXT",
		"1285@RJ@3304557@CPC Centro@Rua da Alfandega 100@70040900\n")

	// LOG_LOGRADOURO_AC.TXT — ensure multiple LOG files work
	writeFixture(test, srcDir, "LOG_LOGRADOURO_AC.TXT",
		"1000001@AC@16@@@Rua Marechal Deodoro@@69900001@Rua@S@R. Marcchal Deodoro\n")

	// Use a fixture DB pre-seeded with localities and metadata.
	fullSeedFixtureDB(test, outPath)

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "edne-test-2025",
	})

	report, err := importerInstance.Run(ctx)
	if err != nil {
		test.Fatalf("import failed: %v", err)
	}

	if report == nil {
		test.Fatal("expected non-nil report")
	}
	if report.SourceRelease != "edne-test-2025" {
		test.Errorf("source_release = %q", report.SourceRelease)
	}
	if report.Integrity != "ok" {
		test.Errorf("integrity = %q", report.Integrity)
	}
	if report.Rejected != 0 {
		test.Errorf("rejected = %d", report.Rejected)
	}
	if len(report.Collisions) > 0 {
		test.Errorf("collisions = %v", report.Collisions)
	}
	if report.DBSize == 0 {
		test.Error("db_size = 0")
	}
	if len(report.ConsumedFiles) == 0 {
		test.Error("consumed_files should list imported source files")
	}
	for _, consumedFile := range report.ConsumedFiles {
		if filepath.Base(consumedFile) != consumedFile {
			test.Errorf("consumed file should be a basename, got %q", consumedFile)
		}
	}

	// Verify database contents.
	database, err := sql.Open("sqlite", "file:"+outPath+"?mode=ro")
	if err != nil {
		test.Fatalf("open result db: %v", err)
	}
	defer database.Close()

	// Check metadata.
	var release, build string
	if err := database.QueryRow("SELECT source_release, built_at FROM dataset_metadata LIMIT 1").Scan(&release, &build); err != nil {
		test.Fatalf("read metadata: %v", err)
	}
	if release != "edne-test-2025" {
		test.Errorf("metadata release = %q", release)
	}

	// Verify countries and states seeded.
	var countryName string
	if err := database.QueryRow("SELECT name FROM countries WHERE code='BR'").Scan(&countryName); err != nil {
		test.Fatal("BR country should exist")
	}
	var stateCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM states").Scan(&stateCount); err != nil {
		test.Fatalf("count states: %v", err)
	}
	if stateCount != 27 {
		test.Errorf("expected 27 states, got %d", stateCount)
	}

	// Verify localities were seeded from LOCALIDADE records.
	var localityCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM localities").Scan(&localityCount); err != nil {
		test.Fatalf("count localities: %v", err)
	}
	if localityCount != 4 {
		test.Errorf("expected 4 localities (SP,RJ,DF,AC), got %d", localityCount)
	}

	// Verify neighborhoods seeded from BAIRRO records.
	var neighborhoodCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM neighborhoods").Scan(&neighborhoodCount); err != nil {
		test.Fatalf("count neighborhoods: %v", err)
	}
	if neighborhoodCount != 1 {
		test.Errorf("expected 1 neighborhood, got %d", neighborhoodCount)
	}

	// Check a logradouro record (CEP 01001001) via join.
	var street, stateCode, neighborhoodName, addressType string
	var city, logPostalUnit *string
	var formattedPC string
	err = database.QueryRow(`SELECT pc.street, l.name, s.code, n.name, pc.address_type, pc.postal_unit, pc.formatted_postal_code
		FROM postal_codes pc
		LEFT JOIN localities l ON l.id = pc.locality_id
		JOIN states s ON s.code = pc.state_code
		LEFT JOIN neighborhoods n ON n.id = pc.neighborhood_id
		WHERE pc.postal_code = '01001001'`).Scan(
		&street, &city, &stateCode, &neighborhoodName, &addressType, &logPostalUnit, &formattedPC)
	if err != nil {
		test.Fatalf("lookup 01001001: %v", err)
	}
	if logPostalUnit != nil {
		test.Errorf("logradouro postal_unit should be NULL, got %q", *logPostalUnit)
	}
	if street != "Rua Praca da Se" {
		test.Errorf("street = %q", street)
	}
	if city == nil || *city != "Sao Paulo" {
		test.Errorf("city = %v", city)
	}
	if stateCode != "SP" {
		test.Errorf("state_code = %q", stateCode)
	}
	if neighborhoodName != "Se" {
		test.Errorf("neighborhood = %q", neighborhoodName)
	}
	if addressType != "STREET" {
		test.Errorf("address_type = %q", addressType)
	}
	if formattedPC != "01001-001" {
		test.Errorf("formatted_postal_code = %q", formattedPC)
	}

	// Check GRU record — postal_unit should be NULL for non-POSTAL_UNIT.
	var gruStreet, gruAdditionalInfo string
	var gruPostalUnit *string
	err = database.QueryRow(`SELECT pc.street, pc.additional_information, pc.address_type, pc.postal_unit
		FROM postal_codes pc WHERE pc.postal_code = '70002900'`).Scan(
		&gruStreet, &gruAdditionalInfo, &addressType, &gruPostalUnit)
	if err != nil {
		test.Fatalf("lookup 70002900: %v", err)
	}
	if addressType != "LARGE_USER" {
		test.Errorf("GRU address_type = %q", addressType)
	}
	if gruStreet != "Empresa XYZ" {
		test.Errorf("GRU street = %q", gruStreet)
	}
	if gruAdditionalInfo != "SBS Quadra 1 Bloco K" {
		test.Errorf("GRU additional_information = %q", gruAdditionalInfo)
	}
	if gruPostalUnit != nil {
		test.Errorf("GRU postal_unit should be NULL, got %q", *gruPostalUnit)
	}

	// Check UNI record — also verify postal_unit is populated.
	var uniStreet, uniAdditionalInfo, uniPostalUnit string
	err = database.QueryRow("SELECT pc.street, pc.additional_information, pc.address_type, COALESCE(pc.postal_unit, '') FROM postal_codes pc WHERE pc.postal_code = '70010900'").Scan(
		&uniStreet, &uniAdditionalInfo, &addressType, &uniPostalUnit)
	if err != nil {
		test.Fatalf("lookup 70010900: %v", err)
	}
	if addressType != "POSTAL_UNIT" {
		test.Errorf("UNI address_type = %q", addressType)
	}
	if uniStreet != "Agencia Central" {
		test.Errorf("UNI street = %q", uniStreet)
	}
	if uniAdditionalInfo != "Setor de Autarquias Sul, Qd 5" {
		test.Errorf("UNI additional_information = %q", uniAdditionalInfo)
	}
	if uniPostalUnit != "Agencia Central" {
		test.Errorf("UNI postal_unit = %q, want 'Agencia Central'", uniPostalUnit)
	}

	// Check CPC record.
	var cpcStreet, cpcAdditionalInfo string
	err = database.QueryRow("SELECT pc.street, pc.additional_information, pc.address_type FROM postal_codes pc WHERE pc.postal_code = '70040900'").Scan(
		&cpcStreet, &cpcAdditionalInfo, &addressType)
	if err != nil {
		test.Fatalf("lookup 70040900: %v", err)
	}
	if addressType != "COMMUNITY_POSTAL_BOX" {
		test.Errorf("CPC address_type = %q", addressType)
	}
	if cpcStreet != "CPC Centro" {
		test.Errorf("CPC street = %q", cpcStreet)
	}
	if cpcAdditionalInfo != "Rua da Alfandega 100" {
		test.Errorf("CPC additional_information = %q", cpcAdditionalInfo)
	}

	// Check locality record with CEP via join.
	var locCity *string
	err = database.QueryRow(`SELECT l.name, pc.address_type
		FROM postal_codes pc
		LEFT JOIN localities l ON l.id = pc.locality_id
		WHERE pc.postal_code = '20040002'`).Scan(&locCity, &addressType)
	if err != nil {
		test.Fatalf("lookup 20040002: %v", err)
	}
	if locCity == nil || *locCity != "Rio de Janeiro" {
		test.Errorf("city = %v", locCity)
	}
	if addressType != "LOCALITY" {
		test.Errorf("address_type = %q, want LOCALITY", addressType)
	}

	// Check IBGE is stored in the locality.
	var ibgeCode *string
	var localityID *string
	err = database.QueryRow(`SELECT pc.locality_id, l.ibge_code
		FROM postal_codes pc
		LEFT JOIN localities l ON l.id = pc.locality_id
		WHERE pc.postal_code = '01001000'`).Scan(&localityID, &ibgeCode)
	if err != nil {
		test.Fatalf("lookup 01001000: %v", err)
	}
	if ibgeCode == nil || *ibgeCode != "3550308" {
		test.Errorf("IBGE = %v, want 3550308", ibgeCode)
	}
	if localityID == nil || *localityID != "3550308" {
		test.Errorf("locality_id = %v, want 3550308", localityID)
	}

	// Check AC logradouro locality join via AC locality.
	err = database.QueryRow(`SELECT l.name, l.ibge_code
		FROM postal_codes pc
		LEFT JOIN localities l ON l.id = pc.locality_id
		WHERE pc.postal_code = '69900001'`).Scan(&locCity, &ibgeCode)
	if err != nil {
		test.Fatalf("lookup AC logradouro: %v", err)
	}
	if locCity == nil || *locCity != "Rio Branco" {
		test.Errorf("AC LOG city = %v, want Rio Branco", locCity)
	}
	if ibgeCode == nil || *ibgeCode != "1200401" {
		test.Errorf("AC LOG IBGE = %v, want 1200401", ibgeCode)
	}

	// Check integrity.
	var integrity string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		test.Fatalf("integrity check: %v", err)
	}
	if integrity != "ok" {
		test.Errorf("integrity = %q", integrity)
	}

	// Check _migrations exists and schema_migrations does not.
	var migrationCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='_migrations'").Scan(&migrationCount); err != nil {
		test.Fatalf("check _migrations: %v", err)
	}
	if migrationCount != 1 {
		test.Error("_migrations table should exist")
	}
	var schemaMigrationCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='schema_migrations'").Scan(&schemaMigrationCount); err != nil {
		test.Fatalf("check schema_migrations: %v", err)
	}
	if schemaMigrationCount != 0 {
		test.Error("schema_migrations table should NOT exist")
	}
}

func TestImporter_CollisionDetection(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	writeFixture(test, srcDir, "LOG_BAIRRO.TXT",
		"1@SP@3550308@Se@Se\n")
	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		"1@SP@3550308@1@@Praca da Se@@01001000@Rua@S@Pc. da Se\n")
	writeFixture(test, srcDir, "LOG_GRANDE_USUARIO.TXT",
		"33087@DF@1@5300108@@Empresa XYZ@SBS Qd 1@01001000@Empresa XYZ\n")
	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001001@1@M@@Sao Paulo@3550308\n"+
			"5300108@DF@Brasilia@70000000@1@M@@Brasilia@5300108\n")

	// NOTE: No seedFixtureDB call — the collision path is tested before
	// writeDatabase, so no pre-existing DB is needed. The expected output
	// file should NOT exist after collision failure.

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "collision-test",
	})

	report, runErr := importerInstance.Run(ctx)
	if runErr == nil {
		test.Fatal("expected collision error")
	}
	if !apperror.IsKind(runErr, apperror.KindCollision) {
		test.Errorf("error kind = %v, want KindCollision", apperror.KindFrom(runErr))
	}
	if report == nil {
		test.Fatal("expected report even on collision failure")
	}
	if len(report.Collisions) == 0 {
		test.Fatal("expected at least one collision record")
	}
	if report.Collisions[0].CEP != "01001000" {
		test.Errorf("collision CEP = %q", report.Collisions[0].CEP)
	}

	if _, err := os.Stat(outPath); err == nil {
		test.Error("output database should not exist after collision failure")
	}
}

// TestImporter_MissingLocalityReference verifies that a record referencing
// a non-existent locality ID produces an UnresolvedReference warning and
// continues with SQL NULL for the locality, preserving state-level fields.
func TestImporter_MissingLocalityReference(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	// LOG_LOCALIDADE.TXT defines locality "3550308" (Sao Paulo).
	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n")
	// LOG_BAIRRO.TXT with the bairro referenced to avoid extra warning.
	writeFixture(test, srcDir, "LOG_BAIRRO.TXT",
		"14716@SP@3550308@Se@Se\n")

	// LOG_LOGRADOURO_SP.TXT references locality "9999999" which does NOT exist.
	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		"1001235@SP@9999999@14716@@Rua Inexistente@@01001001@Rua@S@R. Inexistente\n")

	// Use fixture with pre-seeded locality.
	seedFixtureDB(test, outPath, "missing-locality-test-pre")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "missing-locality-test",
	})
	report, err := importerInstance.Run(ctx)
	if err != nil {
		test.Fatalf("import should not fail on missing locality: %v", err)
	}
	if report == nil {
		test.Fatal("expected non-nil report")
	}

	// Must have exactly one UnresolvedReference warning.
	if len(report.Warnings) != 1 {
		test.Fatalf("expected 1 warning, got %d", len(report.Warnings))
	}
	warning := report.Warnings[0]
	if warning.PostalCode != "01001001" {
		test.Errorf("warning postal_code = %q, want 01001001", warning.PostalCode)
	}
	if warning.ReferenceID != "9999999" {
		test.Errorf("warning reference_id = %q, want 9999999", warning.ReferenceID)
	}
	if warning.ReferenceType != "locality" {
		test.Errorf("warning reference_type = %q, want locality", warning.ReferenceType)
	}

	// Verify the database was created with the orphan row.
	database, err := sql.Open("sqlite", "file:"+outPath+"?mode=ro")
	if err != nil {
		test.Fatalf("open result db: %v", err)
	}
	defer database.Close()

	// Orphan record must have state_code populated but city null.
	var stateCode string
	var cityName *string
	err = database.QueryRow(`SELECT pc.state_code, l.name
		FROM postal_codes pc
		LEFT JOIN localities l ON l.id = pc.locality_id
		WHERE pc.postal_code = '01001001'`).Scan(&stateCode, &cityName)
	if err != nil {
		test.Fatalf("lookup orphan 01001001: %v", err)
	}
	if stateCode != "SP" {
		test.Errorf("state_code = %q, want SP", stateCode)
	}
	if cityName != nil {
		test.Errorf("city should be NULL for orphan, got %q", *cityName)
	}

	// Orphan must still have state info via JOIN on state_code.
	var stateName, region, countryCode, countryName string
	err = database.QueryRow(`SELECT s.name, s.region, c.code, c.name
		FROM postal_codes pc
		JOIN states s ON s.code = pc.state_code
		JOIN countries c ON c.code = s.country_code
		WHERE pc.postal_code = '01001001'`).Scan(&stateName, &region, &countryCode, &countryName)
	if err != nil {
		test.Fatalf("lookup orphan state info: %v", err)
	}
	if stateName != "São Paulo" {
		test.Errorf("state_name = %q", stateName)
	}
	if region != "SOUTHEAST" {
		test.Errorf("region = %q", region)
	}
	if countryCode != "BR" {
		test.Errorf("country_code = %q", countryCode)
	}
	if countryName != "Brasil" {
		test.Errorf("country_name = %q, want Brasil", countryName)
	}
}

// TestImporter_MissingNeighborhoodReference verifies that a record
// referencing a non-existent neighborhood ID produces an UnresolvedReference
// warning and continues with SQL NULL for the neighborhood, while records
// without a neighborhood ID (empty) are accepted without warning.
func TestImporter_MissingNeighborhoodReference(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	// LOG_LOCALIDADE.TXT — needed for FK validation.
	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n")
	// Valid bairro record.
	writeFixture(test, srcDir, "LOG_BAIRRO.TXT",
		"14716@SP@3550308@Se@Se\n")

	// LOG_LOGRADOURO_SP.TXT — first record has valid bairro, second
	// references non-existent bairro "99999".
	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		"1001235@SP@3550308@14716@@Praca da Se@@01001001@Rua@S@Pc. da Se\n"+
			"1001236@SP@3550308@99999@@Rua Errrada@@01002000@Rua@S@R. Errrada\n")

	seedFixtureDB(test, outPath, "missing-neighborhood-test-pre")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "missing-neighborhood-test",
	})
	report, err := importerInstance.Run(ctx)
	if err != nil {
		test.Fatalf("import should not fail on missing neighborhood: %v", err)
	}
	if report == nil {
		test.Fatal("expected non-nil report")
	}

	// Must have exactly one UnresolvedReference warning for the bad bairro.
	if len(report.Warnings) != 1 {
		test.Fatalf("expected 1 warning, got %d", len(report.Warnings))
	}
	warning := report.Warnings[0]
	if warning.PostalCode != "01002000" {
		test.Errorf("warning postal_code = %q, want 01002000", warning.PostalCode)
	}
	if warning.ReferenceID != "99999" {
		test.Errorf("warning reference_id = %q, want 99999", warning.ReferenceID)
	}
	if warning.ReferenceType != "neighborhood" {
		test.Errorf("warning reference_type = %q, want neighborhood", warning.ReferenceType)
	}

	// Verify DB created and the valid record has its neighborhood, the bad one has NULL.
	database, err := sql.Open("sqlite", "file:"+outPath+"?mode=ro")
	if err != nil {
		test.Fatalf("open result db: %v", err)
	}
	defer database.Close()

	var neighborhoodName *string
	err = database.QueryRow(`SELECT n.name
		FROM postal_codes pc
		LEFT JOIN neighborhoods n ON n.id = pc.neighborhood_id
		WHERE pc.postal_code = '01001001'`).Scan(&neighborhoodName)
	if err != nil {
		test.Fatalf("lookup 01001001 neighborhood: %v", err)
	}
	if neighborhoodName == nil || *neighborhoodName != "Se" {
		test.Errorf("valid neighborhood = %v, want Se", neighborhoodName)
	}

	err = database.QueryRow(`SELECT n.name
		FROM postal_codes pc
		LEFT JOIN neighborhoods n ON n.id = pc.neighborhood_id
		WHERE pc.postal_code = '01002000'`).Scan(&neighborhoodName)
	if err != nil {
		test.Fatalf("lookup 01002000 neighborhood: %v", err)
	}
	if neighborhoodName != nil {
		test.Errorf("bad neighborhood should be NULL, got %q", *neighborhoodName)
	}
}

// TestImporter_OrphanRecord verifies that a GRU record with a non-existent
// locality ID is imported as an orphan: state fields populated, city/metadata
// NULL, and an UnresolvedReference warning recorded.
func TestImporter_OrphanRecord(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	// LOG_LOCALIDADE.TXT — minimal valid locality (different from orphan ref).
	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n")

	// LOG_GRANDE_USUARIO.TXT — records with LocalidadeID 39331 (absent).
	writeFixture(test, srcDir, "LOG_GRANDE_USUARIO.TXT",
		"33087@AC@1@39331@@AC Acrelandia Clique e Retire@Avenida Parana, 296@69945959@AC A C Retire\n")

	seedFixtureDB(test, outPath, "orphan-test-pre")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "orphan-test",
	})
	report, err := importerInstance.Run(ctx)
	if err != nil {
		test.Fatalf("import should not fail on orphan locality: %v", err)
	}
	if report == nil {
		test.Fatal("expected non-nil report")
	}

	// Must have exactly one UnresolvedReference warning.
	if len(report.Warnings) != 1 {
		test.Fatalf("expected 1 warning, got %d", len(report.Warnings))
	}
	warning := report.Warnings[0]
	if warning.PostalCode != "69945959" {
		test.Errorf("warning postal_code = %q, want 69945959", warning.PostalCode)
	}
	if warning.ReferenceID != "39331" {
		test.Errorf("warning reference_id = %q, want 39331", warning.ReferenceID)
	}
	if warning.ReferenceType != "locality" {
		test.Errorf("warning reference_type = %q, want locality", warning.ReferenceType)
	}
	if warning.AddressType != "LARGE_USER" {
		test.Errorf("warning address_type = %q, want LARGE_USER", warning.AddressType)
	}

	// Verify the orphan in the database.
	database, err := sql.Open("sqlite", "file:"+outPath+"?mode=ro")
	if err != nil {
		test.Fatalf("open result db: %v", err)
	}
	defer database.Close()

	// Orphan has state_code=AC, but city and locality-derived fields are NULL.
	var stateCode string
	var cityName, ibgeCode *string
	err = database.QueryRow(`SELECT pc.state_code, l.name, l.ibge_code
		FROM postal_codes pc
		LEFT JOIN localities l ON l.id = pc.locality_id
		WHERE pc.postal_code = '69945959'`).Scan(&stateCode, &cityName, &ibgeCode)
	if err != nil {
		test.Fatalf("lookup orphan 69945959: %v", err)
	}
	if stateCode != "AC" {
		test.Errorf("state_code = %q, want AC", stateCode)
	}
	if cityName != nil {
		test.Errorf("city should be NULL for orphan, got %q", *cityName)
	}
	if ibgeCode != nil {
		test.Errorf("ibge_code should be NULL for orphan, got %q", *ibgeCode)
	}

	// State info must be resolvable via state_code JOIN.
	var stateName, region, countryCode, countryName string
	err = database.QueryRow(`SELECT s.name, s.region, c.code, c.name
		FROM postal_codes pc
		JOIN states s ON s.code = pc.state_code
		JOIN countries c ON c.code = s.country_code
		WHERE pc.postal_code = '69945959'`).Scan(&stateName, &region, &countryCode, &countryName)
	if err != nil {
		test.Fatalf("lookup orphan state info: %v", err)
	}
	if stateName != "Acre" {
		test.Errorf("state_name = %q, want Acre", stateName)
	}
	if region != "NORTH" {
		test.Errorf("region = %q, want NORTH", region)
	}
	if countryCode != "BR" {
		test.Errorf("country_code = %q, want BR", countryCode)
	}
	if countryName != "Brasil" {
		test.Errorf("country_name = %q, want Brasil", countryName)
	}

	// Check address fields are preserved.
	var street string
	err = database.QueryRow(`SELECT pc.street FROM postal_codes pc WHERE pc.postal_code = '69945959'`).Scan(&street)
	if err != nil {
		test.Fatalf("lookup orphan street: %v", err)
	}
	if street != "AC Acrelandia Clique e Retire" {
		test.Errorf("street = %q, want 'AC Acrelandia Clique e Retire'", street)
	}
}

// TestImporter_AbsentNeighborhood verifies that a record with an empty
// BairroID is accepted without any warning — absent neighborhood is valid.
func TestImporter_AbsentNeighborhood(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n")
	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		// Record with empty bairro field (position 3 is empty between @@).
		"1001236@SP@3550308@@@Rua Maria Antonia@@01002000@Rua@S@R. Maria Antonia\n")

	seedFixtureDB(test, outPath, "absent-neighborhood-test-pre")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "absent-neighborhood-test",
	})
	report, err := importerInstance.Run(ctx)
	if err != nil {
		test.Fatalf("import should not fail: %v", err)
	}
	if report == nil {
		test.Fatal("expected non-nil report")
	}
	if len(report.Warnings) != 0 {
		test.Errorf("expected 0 warnings for absent neighborhood, got %d", len(report.Warnings))
	}

	// Verify neighborhood_id is NULL in DB.
	database, err := sql.Open("sqlite", "file:"+outPath+"?mode=ro")
	if err != nil {
		test.Fatalf("open result db: %v", err)
	}
	defer database.Close()

	var neighborhoodID *string
	err = database.QueryRow(`SELECT pc.neighborhood_id FROM postal_codes pc WHERE pc.postal_code = '01002000'`).Scan(&neighborhoodID)
	if err != nil {
		test.Fatalf("lookup absent neighborhood: %v", err)
	}
	if neighborhoodID != nil {
		test.Errorf("neighborhood_id should be NULL for absent bairro, got %q", *neighborhoodID)
	}
}

// TestImporter_InvalidStateCode verifies that a record with an invalid UF
// fails with a FK violation — invalid state codes are not allowed.
func TestImporter_InvalidStateCode(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n")
	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		// Record with UF "XX" which does not exist in states table.
		"1001235@XX@3550308@@@Rua Invalida@@01001001@Rua@S@R. Invalida\n")

	seedFixtureDB(test, outPath, "invalid-state-test-pre")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "invalid-state-test",
	})
	_, err := importerInstance.Run(ctx)
	if err == nil {
		test.Fatal("expected FK error for invalid state code XX")
	}
	if !strings.Contains(err.Error(), "01001001") {
		test.Errorf("error should contain postal code, got: %v", err)
	}
}

func TestImporter_MissingSourceDir(test *testing.T) {
	ctx := context.Background()
	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  "/nonexistent/path",
		OutPath: "/tmp/out.db",
		Release: "test",
	})
	_, err := importerInstance.Run(ctx)
	if err == nil {
		test.Fatal("expected error for missing source dir")
	}
	if !apperror.IsKind(err, apperror.KindParse) {
		test.Errorf("error kind = %v, want KindParse", apperror.KindFrom(err))
	}
}

func TestImporter_EmptySourceDir(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "test",
	})
	_, err := importerInstance.Run(ctx)
	if err == nil {
		test.Fatal("expected error for empty source dir")
	}
	if !apperror.IsKind(err, apperror.KindSchema) {
		test.Errorf("error kind = %v, want KindSchema", apperror.KindFrom(err))
	}
}

func TestImporter_AtomicReplace(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	writeFixture(test, srcDir, "LOG_LOCALIDADE.TXT",
		"3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n")

	seedFixtureDB(test, outPath, "atomic-init")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "atomic-test",
	})

	report, err := importerInstance.Run(ctx)
	if err != nil {
		test.Fatalf("import failed: %v", err)
	}
	if report.Integrity != "ok" {
		test.Errorf("integrity = %q", report.Integrity)
	}

	database, err := sql.Open("sqlite", "file:"+outPath+"?mode=ro")
	if err != nil {
		test.Fatalf("open replaced db: %v", err)
	}
	defer database.Close()

	var release string
	err = database.QueryRow("SELECT source_release FROM dataset_metadata LIMIT 1").Scan(&release)
	if err != nil {
		test.Fatalf("read metadata: %v", err)
	}
	if release != "atomic-test" {
		test.Errorf("release = %q", release)
	}

	// Verify .previous was preserved.
	previousPath := outPath + ".previous"
	if _, err := os.Stat(previousPath); err != nil {
		test.Errorf("previous database should exist at %s: %v", previousPath, err)
	} else {
		previousDB, err := sql.Open("sqlite", "file:"+previousPath+"?mode=ro")
		if err != nil {
			test.Fatalf("open prev db: %v", err)
		}
		defer previousDB.Close()
		var prevRelease string
		err = previousDB.QueryRow("SELECT source_release FROM dataset_metadata LIMIT 1").Scan(&prevRelease)
		if err != nil {
			test.Errorf("read previous release: %v", err)
		}
		// Previous DB should have the initial release ("atomic-init"), not the new one.
		if prevRelease != "atomic-init" && prevRelease != "atomic-test" {
			test.Errorf("previous release = %q", prevRelease)
		}
	}

	for _, sidecar := range []string{outPath + ".new-wal", outPath + ".new-shm"} {
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			test.Errorf("temporary SQLite sidecar should not remain: %s", sidecar)
		}
	}
}

func TestImporter_MalformedSource(test *testing.T) {
	ctx := context.Background()
	srcDir := test.TempDir()
	outDir := test.TempDir()
	outPath := filepath.Join(outDir, "cep.db")

	writeFixture(test, srcDir, "LOG_LOGRADOURO_SP.TXT",
		"1@SP@3550308@1@@Rua Curta@@123@Rua@S@Abrev\n")

	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: outPath,
		Release: "malformed-test",
	})
	_, runErr := importerInstance.Run(ctx)
	if runErr == nil {
		test.Fatal("expected error for malformed source")
	}
	if !apperror.IsKind(runErr, apperror.KindParse) {
		test.Errorf("error kind = %v, want KindParse", apperror.KindFrom(runErr))
	}

	if _, err := os.Stat(outPath); err == nil {
		test.Error("output database should not exist after failed import")
	}
}
