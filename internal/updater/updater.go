// Package updater downloads, extracts, and imports the eDNE Básico
// dataset from the Correios outer ZIP archive. It handles nested
// archive extraction, release derivation, skip logic, and atomic
// import through the existing importer pipeline.
//
// The updater is one-shot: each process execution performs at most one
// update attempt. A new update is a new execution.
package updater

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/whoisclebs/cep-seed/internal/apperror"
	"github.com/whoisclebs/cep-seed/internal/importer"
)

// Defaults for the updater.
const (
	DefaultTimeout              = 5 * time.Minute
	DefaultRetryDelay           = 5 * time.Second
	DefaultMaxRetries           = 3
	DefaultMaxDownloadBytes     = 500 << 20 // 500 MB
	DefaultMaxUncompressedBytes = 1 << 30   // 1 GB
	DefaultMaxEntryBytes        = 100 << 20 // 100 MB
)

// Config controls updater behaviour.
type Config struct {
	SourceURL            string        // URL of the Correios outer ZIP archive
	DBPath               string        // path to the current cep.db (may not exist)
	DataDir              string        // directory for temporary extraction
	Timeout              time.Duration // HTTP download timeout
	MaxDownloadBytes     int64
	MaxUncompressedBytes int64
	MaxEntryBytes        int64
}

// Result describes the outcome of an update run.
type Result int

const (
	ResultSuccess Result = iota
	ResultSkip
	ResultFailure
)

// DatasetUpdateResult is an explicit shape for the outcome of an update run.
type DatasetUpdateResult struct {
	Result Result
	Report *importer.Report
	Error  error // populated when Result is ResultFailure
}

// Updater handles one update cycle.
type Updater struct {
	config Config
	client *http.Client
	logger *slog.Logger
}

// New creates an Updater with the given config.
func New(cfg Config) *Updater {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if cfg.MaxDownloadBytes <= 0 {
		cfg.MaxDownloadBytes = DefaultMaxDownloadBytes
	}
	if cfg.MaxUncompressedBytes <= 0 {
		cfg.MaxUncompressedBytes = DefaultMaxUncompressedBytes
	}
	if cfg.MaxEntryBytes <= 0 {
		cfg.MaxEntryBytes = DefaultMaxEntryBytes
	}
	return &Updater{
		config: cfg,
		client: &http.Client{Timeout: timeout},
		logger: slog.Default(),
	}
}

// WithHTTPClient sets the HTTP client on the updater and returns the
// updater for chaining. Used in tests to inject httptest servers.
func (updater *Updater) WithHTTPClient(client *http.Client) *Updater {
	updater.client = client
	return updater
}

// Run performs one update cycle. It returns a DatasetUpdateResult with
// ResultSuccess on a successful import, ResultSkip when the current
// release is already applied, or ResultFailure on any error. Temporary
// files are cleaned up in all cases. Errors at stage boundaries are
// classified with apperror kinds via Wrap.
func (updater *Updater) Run(ctx context.Context) DatasetUpdateResult {
	logger := updater.logger.With("source", updater.config.SourceURL)

	if err := os.MkdirAll(updater.config.DataDir, 0700); err != nil {
		logger.Error("create data directory", "path", updater.config.DataDir, "error", err)
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.Wrap(apperror.KindInternal, err, "create data directory")}
	}

	workDir, err := os.MkdirTemp(updater.config.DataDir, "cep-update-")
	if err != nil {
		logger.Error("create work dir", "error", err)
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.Wrap(apperror.KindInternal, err, "create work directory")}
	}
	defer os.RemoveAll(workDir)

	// 1. Download outer ZIP with size limit.
	outerPath := filepath.Join(workDir, "outer.zip")
	if err := updater.download(ctx, updater.config.SourceURL, outerPath); err != nil {
		logger.Error("download", "error", err)
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.Wrap(apperror.KindDownload, err, "download failed")}
	}

	// 2. Extract and select inner base ZIP.
	innerPath, release, err := updater.selectInnerBase(ctx, outerPath)
	if err != nil {
		logger.Error("select inner base", "error", err)
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.Wrap(apperror.KindArchive, err, "select inner archive")}
	}

	// 3. Extract inner ZIP to work directory (zip-slip safe, size limited).
	extractDir := filepath.Join(workDir, "extracted")
	if err := updater.extractZIPSafe(ctx, innerPath, extractDir); err != nil {
		logger.Error("extract inner zip", "error", err)
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.Wrap(apperror.KindArchive, err, "extract inner archive")}
	}

	// 4. Detect Delimitado/ subdirectory.
	srcDir := extractDir
	if info, err := os.Stat(filepath.Join(extractDir, "Delimitado")); err == nil && info.IsDir() {
		srcDir = filepath.Join(extractDir, "Delimitado")
	}

	// 5. Verify extracted structure.
	if !updater.hasLocalidadeFile(srcDir) {
		logger.Error("LOG_LOCALIDADE.TXT not found in extracted archive")
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.New(apperror.KindArchive, "LOG_LOCALIDADE.TXT not found in extracted archive")}
	}

	// 6. Check if already on this release.
	skip, err := updater.shouldSkip(ctx, release)
	if err != nil {
		logger.Error("check current release", "error", err)
		return DatasetUpdateResult{Result: ResultFailure, Error: apperror.Wrap(apperror.KindSchema, err, "check current release")}
	}
	if skip {
		logger.Info("current release already applied, skipping", "release", release)
		return DatasetUpdateResult{Result: ResultSkip}
	}

	// 7. Run importer.
	logger.Info("importing new release", "release", release)
	importerInstance := importer.New(importer.ImportConfig{
		SrcDir:  srcDir,
		OutPath: updater.config.DBPath,
		Release: release,
	})
	report, err := importerInstance.Run(ctx)
	if err != nil {
		logger.Error("import failed", "release", release, "error", err)
		// Classify importer errors. The importer already returns typed errors
		// at its boundaries; we wrap here for the outer orchestration layer.
		return DatasetUpdateResult{Result: ResultFailure, Report: report, Error: err}
	}
	logger.Info("import complete",
		"release", release,
		"integrity", report.Integrity,
		"accepted", importer.TotalAccepted(report.Accepted),
		"warnings", len(report.Warnings),
		"collisions", len(report.Collisions),
		"db_size", report.DBSize,
	)
	for _, warning := range report.Warnings {
		logger.Warn("unresolved reference",
			"postal_code", warning.PostalCode,
			"address_type", warning.AddressType,
			"record_class", warning.RecordClass,
			"reference_type", warning.ReferenceType,
			"reference_id", warning.ReferenceID,
			"source_file", warning.SourceFile,
		)
	}

	return DatasetUpdateResult{Result: ResultSuccess, Report: report}
}
