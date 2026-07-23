// Package importer builds the canonical postal code lookup database from
// eDNE Básico delimited source files. The pipeline is decomposed into
// cohesive units: discovery, canonicalization, and writing.
package importer

import (
	"context"
	"fmt"
	"time"

	"github.com/whoisclebs/cep-seed/internal/apperror"
)

// Report summarises an import run.
//
// Warnings carry UnresolvedReference entries for rows that could not be
// joined to localities or neighborhoods. These rows are still written to
// the database with SQL NULL for the unresolved reference and the
// state-level fields populated from the source record's UF.
//
// Rejected is always zero because the import is fail-fast for genuine
// errors (malformed data, CEP collisions, FK violations on state_code).
// Unresolved references are not errors — they are recorded as warnings.
type Report struct {
	SourceRelease string                `json:"source_release"`
	BuiltAt       string                `json:"built_at"`
	Elapsed       string                `json:"elapsed"`
	ConsumedFiles []string              `json:"consumed_files"`
	Accepted      map[string]int        `json:"accepted_by_type"`
	Rejected      int                   `json:"rejected"`
	Collisions    []CollisionRecord     `json:"collisions,omitempty"`
	Warnings      []UnresolvedReference `json:"warnings,omitempty"`
	DBSize        int64                 `json:"db_size_bytes"`
	Integrity     string                `json:"integrity"`
}

// CollisionRecord describes a CEP collision between two source records.
type CollisionRecord struct {
	CEP         string `json:"cep"`
	Existing    string `json:"existing_type"`
	Conflicting string `json:"conflicting_type"`
	Source      string `json:"source_file"`
}

// UnresolvedReference describes a record whose locality or neighborhood
// reference could not be resolved. The record is still imported with
// SQL NULL for the unresolved reference.
type UnresolvedReference struct {
	PostalCode    string `json:"postal_code"`
	AddressType   string `json:"address_type"`
	RecordClass   string `json:"record_class"`
	ReferenceType string `json:"reference_type"` // "locality" or "neighborhood"
	ReferenceID   string `json:"reference_id"`
	SourceFile    string `json:"source_file"`
}

// ImportConfig controls the importer behaviour.
type ImportConfig struct {
	SrcDir  string // path to extracted eDNE delimited files
	OutPath string // output database path (final destination after atomic rename)
	Release string // eDNE source release identifier
}

// canonicalRow is a denormalized database row ready for insertion into the
// postal_codes table. StateCode is always set from the source record's UF.
// LocalityID and NeighborhoodID may be empty (meaning SQL NULL) when the
// referenced locality or neighborhood is not found in the reference data.
type canonicalRow struct {
	postalCode, street, additionalInformation string
	stateCode                                 string
	localityID                                string
	neighborhoodID                            string
	addressType, postalUnit                   string
}

// Importer orchestrates the full import pipeline.
type Importer struct {
	config ImportConfig
}

// New creates a new Importer.
func New(cfg ImportConfig) *Importer {
	return &Importer{config: cfg}
}

// Run executes the full import pipeline and returns a report.
// Errors at stage boundaries are classified with apperror kinds:
//   - discoverAndParse errors → KindParse
//   - no supported files → KindSchema
//   - buildCanonical errors → KindParse
//   - collision errors → KindCollision
//   - writeDatabase errors → KindSchema / KindInternal (from writeDatabase)
func (importer *Importer) Run(ctx context.Context) (*Report, error) {
	start := time.Now()

	// 1. Discover and parse all supported source files.
	results, err := importer.discoverAndParse(ctx)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindParse, err, "discover and parse eDNE files")
	}
	if len(results) == 0 {
		return nil, apperror.New(apperror.KindSchema, fmt.Sprintf("no supported eDNE files found in %s", importer.config.SrcDir))
	}

	// 2. Build canonical rows from parsed results, detecting collisions
	//    and collecting unresolved reference warnings.
	canonical, collisions, accepted, warnings, err := buildCanonical(results)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindParse, err, "build canonical rows")
	}

	// 3. Fail on collisions — operator must resolve or set precedence.
	if len(collisions) > 0 {
		report := &Report{
			SourceRelease: importer.config.Release,
			BuiltAt:       time.Now().UTC().Format(time.RFC3339),
			Elapsed:       time.Since(start).String(),
			Collisions:    collisions,
			Accepted:      accepted,
		}
		return report, apperror.Newf(apperror.KindCollision, "collision error: %d CEP collisions found; importer requires explicit precedence", len(collisions))
	}

	// 4. Write database with migrations, validate integrity, publish atomically.
	builtAt := time.Now().UTC().Format(time.RFC3339)
	report, err := writeDatabase(ctx, importer.config, results, canonical, accepted, warnings, builtAt, start)
	if err != nil {
		return report, apperror.Wrap(apperror.KindInternal, err, "write database")
	}

	report.SourceRelease = importer.config.Release
	report.BuiltAt = builtAt
	report.Elapsed = time.Since(start).String()
	return report, nil
}
