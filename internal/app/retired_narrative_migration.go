package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"denova/internal/presetlayout"
	"denova/internal/revisionfile"
)

// migrateRetiredNarrativeStyle runs under the startup lease, after preset layout
// migration and before catalogs become accessible. Only the exact released
// default is retired; edited and unrecognized documents remain user-owned.
func migrateRetiredNarrativeStyle(dataRoot string) error {
	path := filepath.Join(presetlayout.NarrativeStyles(dataRoot), "direct-erotica.json")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read retired narrative style: %w", err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		slog.Warn("Preserved unreadable retired narrative style for manual review", "path", path, "error", err)
		return nil
	}
	// Canonical JSON accepts formatting and key-order differences but retains
	// every field, including override markers and unknown custom extensions.
	canonical, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("compare released narrative style: %w", err)
	}
	if revisionfile.Revision(canonical) != "sha256:5922f37af35c07cd96e05b34eab797dacd94e123ee33d749dbe8e981519b93b3" {
		slog.Warn("Preserved customized retired narrative style; review its prompts manually", "path", path)
		return nil
	}
	backupPath := filepath.Join(dataRoot, "backups", "default-prompts-v0.4.4", "direct-erotica.json")
	backup, err := os.ReadFile(backupPath)
	if err == nil {
		if !bytes.Equal(backup, raw) {
			return fmt.Errorf("retired narrative style backup differs from the original: %s", backupPath)
		}
		// A previous attempt may have preserved the source before cleanup.
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove backed-up retired narrative style: %w", err)
		}
	} else {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read retired narrative style backup: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
			return fmt.Errorf("create retired narrative style backup directory: %w", err)
		}
		if err := os.Rename(path, backupPath); err != nil {
			return fmt.Errorf("preserve retired narrative style: %w", err)
		}
	}
	slog.Info("Removed released narrative style from the active catalog after preserving the original", "path", path, "backup", backupPath)
	return nil
}
