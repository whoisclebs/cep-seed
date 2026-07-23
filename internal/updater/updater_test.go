package updater_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whoisclebs/cep-seed/internal/apperror"
	"github.com/whoisclebs/cep-seed/internal/updater"
	_ "modernc.org/sqlite"
)

// fixturePath points to the synthetic schema-v1 fixture. See testdata/.fixture-provenance.md.
var fixturePath = filepath.Join("..", "..", "testdata", "empty-schema-v1.db")

// ── Helpers ──────────────────────────────────────────────────────

// writeFile is a helper to write content to a file.
func writeFile(test *testing.T, path, content string) {
	test.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		test.Fatalf("write %s: %v", path, err)
	}
}

// copyFixture copies the schema-v1 fixture to dstPath.
func copyFixture(t *testing.T, dstPath string) {
	t.Helper()
	src, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v — has the fixture been generated?", fixturePath, err)
	}
	if err := os.WriteFile(dstPath, src, 0644); err != nil {
		t.Fatalf("write fixture to %s: %v", dstPath, err)
	}
}

// createFixtureDB creates a minimal valid CEP database at path with the given release.
// Uses the fixture plus data inserts.
func createFixtureDB(test *testing.T, path, release string) {
	test.Helper()
	copyFixture(test, path)

	database, err := sql.Open("sqlite", path)
	if err != nil {
		test.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec("PRAGMA foreign_keys = ON"); err != nil {
		test.Fatalf("enable foreign_keys: %v", err)
	}

	tx, err := database.Begin()
	if err != nil {
		test.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO localities (id, name, state_code, ibge_code) VALUES ('3550308', 'São Paulo', 'SP', '3550308')`); err != nil {
		test.Fatalf("insert locality: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO dataset_metadata (id, source_release, built_at) VALUES (1, ?, '2026-07-17T00:00:00Z')`, release); err != nil {
		test.Fatalf("insert metadata: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO postal_codes (postal_code, street, state_code, locality_id, address_type) VALUES ('01001000', 'Praça da Sé', 'SP', '3550308', 'STREET')`); err != nil {
		test.Fatalf("insert cep: %v", err)
	}

	if err := tx.Commit(); err != nil {
		test.Fatalf("commit: %v", err)
	}
}

// createZIP creates a ZIP archive in memory from a map of filename→content.
func createZIP(files map[string]string) []byte {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for name, content := range files {
		fileWriter, err := zipWriter.Create(name)
		if err != nil {
			panic(fmt.Sprintf("create zip entry %s: %v", name, err))
		}
		if _, err := io.WriteString(fileWriter, content); err != nil {
			panic(fmt.Sprintf("write zip entry %s: %v", name, err))
		}
	}
	zipWriter.Close()
	return buf.Bytes()
}

// createNestedZIP creates an outer ZIP containing one or more inner ZIPs
// as binary entries. innerZIPs maps entry name → inner zip bytes.
func createNestedZIP(innerZIPs map[string][]byte) []byte {
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	for name, data := range innerZIPs {
		fileWriter, err := zipWriter.Create(name)
		if err != nil {
			panic(fmt.Sprintf("create zip entry %s: %v", name, err))
		}
		if _, err := fileWriter.Write(data); err != nil {
			panic(fmt.Sprintf("write zip entry %s: %v", name, err))
		}
	}
	zipWriter.Close()
	return buf.Bytes()
}

// updaterFixture creates a nested ZIP pair for testing:
// outer: contains eDNE_Basico_26071.zip + eDNE_Delta_26071_D.zip (delta)
// inner: contains LOG_LOCALIDADE.TXT
func updaterFixture(test *testing.T) (outerData []byte, release string) {
	test.Helper()

	// Inner ZIP with eDNE delimited files.
	innerZIP := createZIP(map[string]string{
		"LOG_LOCALIDADE.TXT": "3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n",
	})

	// Outer ZIP containing both base inner ZIP and a delta.
	outerData = createNestedZIP(map[string][]byte{
		"eDNE_Basico_26071.zip":  innerZIP,
		"eDNE_Delta_26071_D.zip": innerZIP, // delta should be ignored
	})
	return outerData, "26071"
}

// ── Tests ────────────────────────────────────────────────────────

// TestUpdater_BaseVsDeltaSelection verifies that only the base inner
// ZIP is selected and delta archives are ignored.
func TestUpdater_BaseVsDeltaSelection(test *testing.T) {
	outerData, release := updaterFixture(test)

	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")
	createFixtureDB(test, dbPath, "init")
	outerPath := filepath.Join(dir, "outer.zip")
	if err := os.WriteFile(outerPath, outerData, 0644); err != nil {
		test.Fatalf("write outer zip: %v", err)
	}

	// Use a custom download that copies from outerPath.
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		data, _ := os.ReadFile(outerPath)
		responseWriter.Write(data)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	updaterInstance := updater.New(cfg)

	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultSuccess {
		test.Fatalf("expected success, got %v", result.Result)
	}

	// Verify the DB was created with the correct release.
	database, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", dbPath))
	if err != nil {
		test.Fatalf("open db: %v", err)
	}
	defer database.Close()

	var gotRelease string
	if err := database.QueryRow("SELECT source_release FROM dataset_metadata LIMIT 1").Scan(&gotRelease); err != nil {
		test.Fatalf("read metadata: %v", err)
	}
	if gotRelease != release {
		test.Errorf("release = %q, want %q", gotRelease, release)
	}
}

func TestUpdater_CreatesMissingDataDirectory(test *testing.T) {
	rootDirectory := test.TempDir()
	dataDirectory := filepath.Join(rootDirectory, "missing", "work")
	databasePath := filepath.Join(rootDirectory, "cep.db")
	createFixtureDB(test, databasePath, "init")
	outerArchive, _ := updaterFixture(test)

	archiveServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		_, _ = responseWriter.Write(outerArchive)
	}))
	defer archiveServer.Close()

	updaterInstance := updater.New(updater.Config{
		SourceURL: archiveServer.URL,
		DBPath:    databasePath,
		DataDir:   dataDirectory,
		Timeout:   5 * time.Second,
	})

	if result := updaterInstance.Run(context.Background()); result.Result != updater.ResultSuccess {
		test.Fatalf("expected updater success, got %v", result.Result)
	}
	if directoryInfo, err := os.Stat(dataDirectory); err != nil || !directoryInfo.IsDir() {
		test.Fatalf("expected updater to create data directory %q: %v", dataDirectory, err)
	}
}

// TestUpdater_ReleaseDerivation tests that the release is correctly
// extracted from the inner ZIP filename.
func TestUpdater_ReleaseDerivation(test *testing.T) {
	test.Run("standard filename", func(subtest *testing.T) {
		dir := subtest.TempDir()
		dbPath := filepath.Join(dir, "cep.db")

		// Create a DB with release "26071".
		createFixtureDB(subtest, dbPath, "26071")

		// Create fixture with same release.
		outerData, _ := updaterFixture(subtest)

		testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			responseWriter.Write(outerData)
		}))
		defer testServer.Close()

		cfg := updater.Config{
			SourceURL: testServer.URL,
			DBPath:    dbPath,
			DataDir:   dir,
			Timeout:   5 * time.Second,
		}
		updaterInstance := updater.New(cfg)
		result := updaterInstance.Run(context.Background())
		if result.Result != updater.ResultSkip {
			subtest.Fatalf("expected skip when release matches, got %v", result.Result)
		}
	})
}

// TestUpdater_Skip verifies that the updater skips when the current
// database already has the same release.
func TestUpdater_Skip(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")
	createFixtureDB(test, dbPath, "26071")

	outerData, _ := updaterFixture(test)
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultSkip {
		test.Fatalf("expected ResultSkip, got %v", result.Result)
	}
}

// TestUpdater_CleanupOnFailure verifies that temporary files are
// removed when a download fails.
func TestUpdater_CleanupOnFailure(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")

	// Server that returns an error.
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   2 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure, got %v", result.Result)
	}

	// Verify no temp files remain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		test.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "cep-update-") {
			test.Errorf("temp dir %s should have been cleaned up", entry.Name())
		}
	}
}

// TestUpdater_ZipSlipRejection verifies that a malicious archive with
// path traversal is rejected.
func TestUpdater_ZipSlipRejection(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")

	// Create a malicious inner ZIP with a path traversal entry.
	maliciousInner := createZIP(map[string]string{
		"../../etc/passwd": "root:x:0:0:root:/root:/bin/bash\n",
	})

	// Wrap in an outer ZIP with a proper base name.
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_99999.zip": maliciousInner,
	})

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure for zip-slip, got %v", result.Result)
	}
}

// TestUpdater_Cancellation verifies that a cancelled context stops
// the updater cleanly.
func TestUpdater_Cancellation(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")

	// Server that delays to trigger cancellation.
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		time.Sleep(5 * time.Second)
		responseWriter.Write([]byte("data"))
	}))
	defer testServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   10 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(ctx)
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure on cancellation, got %v", result.Result)
	}
}

// TestUpdater_HTTPRetry verifies that a transient HTTP failure is
// retried.
func TestUpdater_HTTPRetry(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")
	createFixtureDB(test, dbPath, "init")

	attempt := 0
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		attempt++
		if attempt < 2 {
			responseWriter.WriteHeader(http.StatusInternalServerError)
			return
		}
		// On third attempt, do the actual download.
		outerData, _ := updaterFixture(test)
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultSuccess {
		test.Fatalf("expected success after retry, got %v", result.Result)
	}
	if attempt != 2 {
		test.Errorf("expected 2 attempts (fail then success), got %d", attempt)
	}
}

// TestUpdater_CleanupOnSuccess verifies temp files are removed after
// a successful update.
func TestUpdater_CleanupOnSuccess(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")
	createFixtureDB(test, dbPath, "init")

	outerData, _ := updaterFixture(test)
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultSuccess {
		test.Fatalf("expected success, got %v", result.Result)
	}

	// Check that the database exists and no temp update dirs remain.
	if _, err := os.Stat(dbPath); err != nil {
		test.Errorf("database should exist: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		test.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "cep-update-") {
			test.Errorf("temp dir %s should have been cleaned up after success", entry.Name())
		}
	}
}

// TestUpdater_DelimitadoDirectory verifies that when the inner ZIP
// contains a Delimitado/ subdirectory, the updater detects it and
// passes the Delimitado/ path to the importer.
func TestUpdater_DelimitadoDirectory(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")
	createFixtureDB(test, dbPath, "init")

	// Inner ZIP with Delimitado/ prefix — like the real eDNE archive.
	innerZIP := createZIP(map[string]string{
		"Delimitado/LOG_LOCALIDADE.TXT": "3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n",
	})
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_26071.zip": innerZIP,
	})

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    dbPath,
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultSuccess {
		test.Fatalf("expected success with Delimitado/ directory, got %v", result.Result)
	}

	// Verify the DB was created.
	if _, err := os.Stat(dbPath); err != nil {
		test.Errorf("database should exist: %v", err)
	}
}

func TestUpdater_SkipsUnusedFixoDirectory(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")
	createFixtureDB(test, dbPath, "init")

	innerZIP := createZIP(map[string]string{
		"Delimitado/LOG_LOCALIDADE.TXT": "3550308@SP@Sao Paulo@01001000@1@M@@Sao Paulo@3550308\n",
		"Fixo/UNUSED.TXT":               strings.Repeat("x", 200),
	})
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_26071.zip": innerZIP,
	})

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	updaterInstance := updater.New(updater.Config{
		SourceURL:            testServer.URL,
		DBPath:               dbPath,
		DataDir:              dir,
		Timeout:              5 * time.Second,
		MaxDownloadBytes:     1 << 20,
		MaxUncompressedBytes: 1 << 20,
		MaxEntryBytes:        100,
	})
	if result := updaterInstance.Run(context.Background()); result.Result != updater.ResultSuccess {
		test.Fatalf("expected success while skipping Fixo, got %v", result.Result)
	}
}

// TestUpdater_DownloadSizeLimit verifies that a download exceeding
// the configured limit is rejected.
func TestUpdater_DownloadSizeLimit(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		// Return more data than the limit.
		responseWriter.Write([]byte(strings.Repeat("x", 200)))
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL:            testServer.URL,
		DBPath:               dbPath,
		DataDir:              dir,
		Timeout:              5 * time.Second,
		MaxDownloadBytes:     100,
		MaxUncompressedBytes: 1 << 30,
		MaxEntryBytes:        100 << 20,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure for oversized download, got %v", result.Result)
	}
}

// TestUpdater_EntrySizeLimit verifies that an entry exceeding the
// per-entry uncompressed limit is rejected.
func TestUpdater_EntrySizeLimit(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")

	// Inner ZIP with a large entry.
	innerZIP := createZIP(map[string]string{
		"LOG_LOCALIDADE.TXT": strings.Repeat("x", 150),
	})
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_26071.zip": innerZIP,
	})

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL:            testServer.URL,
		DBPath:               dbPath,
		DataDir:              dir,
		Timeout:              5 * time.Second,
		MaxDownloadBytes:     1 << 20,
		MaxUncompressedBytes: 1 << 30,
		MaxEntryBytes:        100,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure for oversized entry, got %v", result.Result)
	}
}

// TestUpdater_TotalSizeLimit verifies that total uncompressed data
// exceeding the limit is rejected.
func TestUpdater_TotalSizeLimit(test *testing.T) {
	dir := test.TempDir()
	dbPath := filepath.Join(dir, "cep.db")

	// Inner ZIP with entries that exceed the total limit.
	innerZIP := createZIP(map[string]string{
		"LOG_LOCALIDADE.TXT": strings.Repeat("x", 500),
	})
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_26071.zip": innerZIP,
	})

	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL:            testServer.URL,
		DBPath:               dbPath,
		DataDir:              dir,
		Timeout:              5 * time.Second,
		MaxDownloadBytes:     1 << 20,
		MaxUncompressedBytes: 400,
		MaxEntryBytes:        1 << 20,
	}
	updaterInstance := updater.New(cfg)
	result := updaterInstance.Run(context.Background())
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure for oversized total, got %v", result.Result)
	}
}

func TestUpdater_DownloadError_IsKindDownload(test *testing.T) {
	dir := test.TempDir()
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    filepath.Join(dir, "cep.db"),
		DataDir:   dir,
		Timeout:   2 * time.Second,
	}
	result := updater.New(cfg).Run(context.Background())
	if result.Result != updater.ResultFailure {
		test.Fatalf("expected ResultFailure, got %v", result.Result)
	}
	if result.Error == nil {
		test.Fatal("expected non-nil error on ResultFailure")
	}
	if !apperror.IsKind(result.Error, apperror.KindDownload) {
		test.Errorf("error kind = %v, want KindDownload", apperror.KindFrom(result.Error))
	}
}

func TestUpdater_ZipSlip_IsKindArchive(test *testing.T) {
	dir := test.TempDir()
	maliciousInner := createZIP(map[string]string{
		"../../etc/passwd": "root:x:0:0:root:/root:/bin/bash\n",
	})
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_99999.zip": maliciousInner,
	})
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    filepath.Join(dir, "cep.db"),
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	result := updater.New(cfg).Run(context.Background())
	if result.Error == nil {
		test.Fatal("expected non-nil error")
	}
	if !apperror.IsKind(result.Error, apperror.KindArchive) {
		test.Errorf("error kind = %v, want KindArchive", apperror.KindFrom(result.Error))
	}
}

func TestUpdater_MissingLocalidade_IsKindArchive(test *testing.T) {
	dir := test.TempDir()
	// Inner ZIP does NOT contain LOG_LOCALIDADE.TXT.
	innerZIP := createZIP(map[string]string{
		"LOG_LOGRADOURO_SP.TXT": "some data\n",
	})
	outerData := createNestedZIP(map[string][]byte{
		"eDNE_Basico_26071.zip": innerZIP,
	})
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(outerData)
	}))
	defer testServer.Close()

	cfg := updater.Config{
		SourceURL: testServer.URL,
		DBPath:    filepath.Join(dir, "cep.db"),
		DataDir:   dir,
		Timeout:   5 * time.Second,
	}
	result := updater.New(cfg).Run(context.Background())
	if result.Error == nil {
		test.Fatal("expected non-nil error")
	}
	if !apperror.IsKind(result.Error, apperror.KindArchive) {
		test.Errorf("error kind = %v, want KindArchive", apperror.KindFrom(result.Error))
	}
}
