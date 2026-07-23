package importer

import (
	"fmt"
	"os"
)

// atomicReplace atomically promotes a .new database to the final path.
// It renames existing → .previous, then .new → existing.
// If the second rename fails, it restores .previous → existing.
func atomicReplace(newPath, outPath string) error {
	hasExisting := false
	if _, statErr := os.Stat(outPath); statErr == nil {
		hasExisting = true
		prevPath := outPath + ".previous"
		if err := os.Rename(outPath, prevPath); err != nil {
			os.Remove(newPath)
			return fmt.Errorf("backup %s → %s: %w", outPath, prevPath, err)
		}
	}

	if err := os.Rename(newPath, outPath); err != nil {
		if hasExisting {
			prevPath := outPath + ".previous"
			os.Rename(prevPath, outPath) // best-effort restore
		}
		return fmt.Errorf("rename %s → %s: %w", newPath, outPath, err)
	}
	return nil
}
