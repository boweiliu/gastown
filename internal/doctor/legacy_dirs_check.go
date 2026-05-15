package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/migration"
)

// LegacyDirsCheck detects old-named runtime directories (polecats/, mayor/,
// refinery/, witness/) and migrates them to new names (workers/, coordinator/,
// merger/, watcher/) with backward-compatibility symlinks.
type LegacyDirsCheck struct {
	FixableCheck
	legacyDirs []string
}

// NewLegacyDirsCheck creates a new legacy dirs check.
func NewLegacyDirsCheck() *LegacyDirsCheck {
	return &LegacyDirsCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "legacy-dirs",
				CheckDescription: "Check for old-named runtime directories",
				CheckCategory:    CategoryCore,
			},
		},
	}
}

// Run checks for legacy directory names that need migration.
func (c *LegacyDirsCheck) Run(ctx *CheckContext) *CheckResult {
	c.legacyDirs = nil

	// Check town-level: mayor/ (should be coordinator/)
	c.checkLegacyDir(ctx.TownRoot, constants.LegacyDirMayor)

	// Check each rig
	entries, err := os.ReadDir(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusError,
			Message:  fmt.Sprintf("could not read town root: %v", err),
			Category: c.CheckCategory,
		}
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Name() == constants.DirCoordinator || entry.Name() == constants.LegacyDirMayor {
			continue
		}
		rigPath := filepath.Join(ctx.TownRoot, entry.Name())
		for oldName := range constants.DirRenames {
			c.checkLegacyDir(rigPath, oldName)
		}
	}

	if len(c.legacyDirs) == 0 {
		return &CheckResult{
			Name:     c.Name(),
			Status:   StatusOK,
			Message:  "No legacy directory names found",
			Category: c.CheckCategory,
		}
	}

	return &CheckResult{
		Name:     c.Name(),
		Status:   StatusWarning,
		Message:  fmt.Sprintf("Found %d legacy directory name(s): %s", len(c.legacyDirs), strings.Join(c.legacyDirs, ", ")),
		FixHint:  "Run 'gt upgrade' to migrate directory names",
		Category: c.CheckCategory,
	}
}

// Fix migrates legacy directories to new names with symlinks.
func (c *LegacyDirsCheck) Fix(ctx *CheckContext) error {
	result := migration.MigrateRuntimeDirs(ctx.TownRoot, false)

	if len(result.Errors) > 0 {
		return fmt.Errorf("migration errors: %s", strings.Join(result.Errors, "; "))
	}

	return nil
}

// checkLegacyDir checks if a legacy directory exists as a real directory (not symlink).
func (c *LegacyDirsCheck) checkLegacyDir(parentDir, oldName string) {
	path := filepath.Join(parentDir, oldName)
	fi, err := os.Lstat(path)
	if err != nil {
		return
	}
	// Skip if already a symlink (already migrated)
	if fi.Mode()&os.ModeSymlink != 0 {
		return
	}
	if fi.IsDir() {
		rel, _ := filepath.Rel(filepath.Dir(parentDir), path)
		if rel == "" {
			rel = path
		}
		c.legacyDirs = append(c.legacyDirs, oldName+" in "+filepath.Base(parentDir))
	}
}
