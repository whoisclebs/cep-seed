package importer

import (
	"fmt"
	"io"
	"os"
)

// copyFile copies src to dst using a safe read-then-write approach.
// It creates the destination file with 0644 permissions, syncs the
// destination to disk before closing, and returns contextual errors.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer source.Close()

	destination, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("create destination %s: %w", dst, err)
	}

	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}

	if err := destination.Sync(); err != nil {
		destination.Close()
		return fmt.Errorf("sync destination %s: %w", dst, err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close destination %s: %w", dst, err)
	}
	return nil
}
