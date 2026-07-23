package importer

import (
	"context"
	"database/sql"
	"fmt"
)

// checkIntegrity runs SQLite integrity_check on the database at path.
func checkIntegrity(ctx context.Context, path string) (string, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return "", err
	}
	defer db.Close()

	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return "", err
	}
	return result, nil
}
