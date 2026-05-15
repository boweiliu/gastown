// Package migration provides runtime directory structure migration.
package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/constants"
)

// DirRenameResult tracks what happened during a directory rename migration.
type DirRenameResult struct {
	Renamed  []string // Directories that were renamed
	Linked   []string // Symlinks created for backward compatibility
	Skipped  []string // Directories skipped (new name already exists)
	Errors   []string // Errors encountered
}

// MigrateRuntimeDirs renames old Gas Town runtime directories to new names
// and creates backward-compatibility symlinks from old names to new names.
//
// Town-level renames:
//   - mayor/ → coordinator/
//
// Rig-level renames (for each rig under townRoot):
//   - polecats/ → workers/
//   - crew/ → team/
//   - refinery/ → merger/
//   - witness/ → watcher/
//   - mayor/ → coordinator/ (the rig-level coordinator clone)
func MigrateRuntimeDirs(townRoot string, dryRun bool) *DirRenameResult {
	result := &DirRenameResult{}

	// Town-level: mayor/ → coordinator/
	migrateDir(townRoot, constants.LegacyDirMayor, constants.DirCoordinator, dryRun, result)

	// Find all rigs by looking for directories that contain a coordinator/ (or mayor/) subdir
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("read town root: %v", err))
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden dirs, known non-rig dirs
		if strings.HasPrefix(name, ".") || name == constants.DirCoordinator || name == constants.LegacyDirMayor {
			continue
		}

		rigPath := filepath.Join(townRoot, name)

		// Check if this looks like a rig (has coordinator/ or mayor/ subdir)
		isRig := false
		for _, marker := range []string{constants.DirCoordinator, constants.LegacyDirMayor, constants.DirWorkers, constants.LegacyDirPolecats} {
			if fi, err := os.Stat(filepath.Join(rigPath, marker)); err == nil && fi.IsDir() {
				isRig = true
				break
			}
		}
		if !isRig {
			continue
		}

		// Rig-level renames
		for oldName, newName := range constants.DirRenames {
			migrateDir(rigPath, oldName, newName, dryRun, result)
		}
	}

	return result
}

// migrateDir renames oldName to newName within parentDir and creates a
// backward-compatibility symlink from oldName → newName.
func migrateDir(parentDir, oldName, newName string, dryRun bool, result *DirRenameResult) {
	oldPath := filepath.Join(parentDir, oldName)
	newPath := filepath.Join(parentDir, newName)

	// Check if old directory is a symlink (already migrated)
	if fi, err := os.Lstat(oldPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		result.Skipped = append(result.Skipped, oldPath+" (already symlinked)")
		return
	}

	// Check if old directory exists as a real directory
	oldInfo, err := os.Stat(oldPath)
	if err != nil || !oldInfo.IsDir() {
		// Old dir doesn't exist, nothing to migrate
		return
	}

	// Check if new directory already exists
	if _, err := os.Stat(newPath); err == nil {
		result.Skipped = append(result.Skipped, oldPath+" (new name already exists)")
		return
	}

	relPath, _ := filepath.Rel(parentDir, oldPath)

	if dryRun {
		result.Renamed = append(result.Renamed, fmt.Sprintf("%s → %s (dry run)", relPath, newName))
		result.Linked = append(result.Linked, fmt.Sprintf("%s → %s (dry run)", relPath, newName))
		return
	}

	// Rename old → new
	if err := os.Rename(oldPath, newPath); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("rename %s → %s: %v", relPath, newName, err))
		return
	}
	result.Renamed = append(result.Renamed, fmt.Sprintf("%s → %s", relPath, newName))

	// Create symlink old → new for backward compatibility
	if err := os.Symlink(newName, oldPath); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("symlink %s → %s: %v", relPath, newName, err))
		return
	}
	result.Linked = append(result.Linked, fmt.Sprintf("%s → %s", relPath, newName))
}
