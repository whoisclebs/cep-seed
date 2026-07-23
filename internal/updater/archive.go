package updater

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// baseRE matches the inner base ZIP filename eDNE_Basico_XXXXX.zip.
var baseRE = regexp.MustCompile(`^eDNE_Basico_(\d+)\.zip$`)

// selectInnerBase opens the outer ZIP, finds the first inner base
// archive matching eDNE_Basico_*.zip (skipping Delta archives),
// extracts it to a temporary file, and returns its path and release.
func (updater *Updater) selectInnerBase(ctx context.Context, outerPath string) (innerPath, release string, _ error) {
	zipReader, err := zip.OpenReader(outerPath)
	if err != nil {
		return "", "", fmt.Errorf("open outer zip: %w", err)
	}
	defer zipReader.Close()

	var baseFile *zip.File
	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(file.Name)
		if baseRE.MatchString(name) {
			baseFile = file
			release = releaseFromFilename(name)
			break
		}
	}
	if baseFile == nil {
		return "", "", fmt.Errorf("no eDNE_Basico_*.zip found in outer archive")
	}
	if release == "" {
		return "", "", fmt.Errorf("could not derive release from %s", baseFile.Name)
	}

	innerPath = filepath.Join(filepath.Dir(outerPath), "inner.zip")
	if err := extractZipFile(ctx, baseFile, innerPath); err != nil {
		return "", "", fmt.Errorf("extract inner zip: %w", err)
	}
	return innerPath, release, nil
}

// extractZIPSafe extracts a ZIP archive to dstDir while preventing
// zip-slip attacks and enforcing per-entry and total uncompressed
// size limits.
func (updater *Updater) extractZIPSafe(ctx context.Context, zipPath, dstDir string) error {
	zipReader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zipReader.Close()

	var totalUncompressed int64
	maxTotal := updater.config.MaxUncompressedBytes
	maxEntry := updater.config.MaxEntryBytes

	for _, file := range zipReader.File {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		entryName := filepath.ToSlash(filepath.Clean(file.Name))
		if strings.HasPrefix(strings.ToLower(entryName), "fixo/") {
			continue
		}

		dstPath := filepath.Join(dstDir, file.Name)
		if !strings.HasPrefix(filepath.Clean(dstPath), filepath.Clean(dstDir)+string(os.PathSeparator)) {
			return fmt.Errorf("zip-slip: %q escapes destination %q", file.Name, dstDir)
		}

		if file.UncompressedSize64 > uint64(maxEntry) {
			return fmt.Errorf("entry %q uncompressed size %d exceeds limit %d", file.Name, file.UncompressedSize64, maxEntry)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(dstPath, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(dstPath), 0755)

		if err := extractZipFile(ctx, file, dstPath); err != nil {
			return fmt.Errorf("extract %s: %w", file.Name, err)
		}

		totalUncompressed += int64(file.UncompressedSize64)
		if totalUncompressed > maxTotal {
			return fmt.Errorf("total uncompressed size %d exceeds limit %d", totalUncompressed, maxTotal)
		}
	}
	return nil
}

// extractZipFile extracts a single zip.File to dstPath.
func extractZipFile(ctx context.Context, zipFile *zip.File, dstPath string) error {
	readCloser, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("open zip entry: %w", err)
	}
	defer readCloser.Close()

	out, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", dstPath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, readCloser)
	return err
}

// hasLocalidadeFile checks whether the extracted directory contains
// LOG_LOCALIDADE.TXT (case-insensitive).
func (updater *Updater) hasLocalidadeFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "LOG_LOCALIDADE.TXT") {
			return true
		}
	}
	return false
}
