package updater

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

// releaseFromFilename extracts the release number from a base ZIP
// filename. Returns the release string or empty on mismatch.
func releaseFromFilename(name string) string {
	base := filepath.Base(name)
	matches := baseRE.FindStringSubmatch(base)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// shouldSkip checks whether the current database already has the
// given release. Returns true if the release matches.
func (updater *Updater) shouldSkip(ctx context.Context, release string) (bool, error) {
	database, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", updater.config.DBPath))
	if err != nil {
		// Cannot open the database — cannot skip, surface the error.
		return false, fmt.Errorf("open existing db: %w", err)
	}
	defer database.Close()

	if err := database.PingContext(ctx); err != nil {
		return false, fmt.Errorf("ping existing db: %w", err)
	}

	var currentRelease string
	err = database.QueryRowContext(ctx, `SELECT source_release FROM dataset_metadata LIMIT 1`).Scan(&currentRelease)
	if err != nil {
		return false, fmt.Errorf("read current release: %w", err)
	}
	return currentRelease == release, nil
}
