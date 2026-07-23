package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/whoisclebs/cep-seed/internal/edne"
)

// fileResult holds parsed output from a single source file.
type fileResult struct {
	layout  edne.Layout
	records []edne.SourceRecord
	srcPath string
}

// discoverAndParse finds all supported files in srcDir, parses them,
// and returns the results.
func (importer *Importer) discoverAndParse(ctx context.Context) ([]fileResult, error) {
	srcDir := importer.config.SrcDir
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("read source dir: %w", err)
	}

	parser := edne.NewParser()
	var results []fileResult

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		layout, ok := edne.LayoutForFile(entry.Name())
		if !ok {
			continue
		}

		fullPath := filepath.Join(srcDir, entry.Name())
		file, err := os.Open(fullPath)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", entry.Name(), err)
		}

		records, err := parser.ParseFile(ctx, file, layout, entry.Name())
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", entry.Name(), err)
		}

		results = append(results, fileResult{layout: layout, records: records, srcPath: fullPath})
	}

	return results, nil
}
