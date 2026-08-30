package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const portableDataMigrationVersion = 4

type portableDataMigrationReceipt struct {
	Version      int       `json:"version"`
	CompletedAt  time.Time `json:"completed_at"`
	ProjectCount int       `json:"project_count"`
	Backups      []string  `json:"backups,omitempty"`
}

// preparePortableDataMigration preserves the small v0.3.3 indexes and receipts
// before any final-schema write. Backups are content-addressed so a crash can
// safely retry even if an earlier migration step already rewrote a file.
// Project content and workspace-private state remain copy-only sources.
func preparePortableDataMigration(dataRoot string) ([]string, error) {
	receipt, complete, err := readPortableDataMigrationReceipt(dataRoot)
	if err != nil {
		return nil, err
	}
	if complete {
		return append([]string(nil), receipt.Backups...), nil
	}

	backups := make([]string, 0, 4)
	for _, name := range []string{"books.json", "projects.json"} {
		source := filepath.Join(dataRoot, name)
		backup, found, backupErr := preservePortableMigrationFile(dataRoot, source, strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name))
		if backupErr != nil {
			return nil, backupErr
		}
		if found {
			backups = append(backups, backup)
		}
	}

	legacyMetaRoot := filepath.Join(dataRoot, "book_meta")
	walkErr := filepath.WalkDir(legacyMetaRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		backup, found, backupErr := preservePortableMigrationFile(dataRoot, path, "book-meta", ".json")
		if backupErr != nil {
			return backupErr
		}
		if found {
			backups = append(backups, backup)
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, os.ErrNotExist) {
		return nil, fmt.Errorf("back up v0.3.3 Book metadata index: %w", walkErr)
	}
	stateEntries, readErr := os.ReadDir(filepath.Join(dataRoot, "project-state"))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return nil, fmt.Errorf("list v0.3.3 Project state roots: %w", readErr)
	}
	for _, entry := range stateEntries {
		if !entry.IsDir() {
			continue
		}
		source := filepath.Join(dataRoot, "project-state", entry.Name(), "migration.json")
		backup, found, backupErr := preservePortableMigrationFile(dataRoot, source, "project-state-receipt", ".json")
		if backupErr != nil {
			return nil, backupErr
		}
		if found {
			backups = append(backups, backup)
		}
	}
	sort.Strings(backups)
	return compactStrings(backups), nil
}

func preservePortableMigrationFile(dataRoot, sourcePath, prefix, extension string) (string, bool, error) {
	content, err := os.ReadFile(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read portable migration backup source %s: %w", sourcePath, err)
	}
	digest := sha256.Sum256(content)
	name := prefix + "-" + hex.EncodeToString(digest[:8]) + extension
	backupDir := filepath.Join(dataRoot, "backups", portableDataBackupDirectory)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", false, fmt.Errorf("create portable migration backup directory: %w", err)
	}
	backupPath := filepath.Join(backupDir, name)
	file, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if _, writeErr := file.Write(content); writeErr != nil {
			_ = file.Close()
			return "", false, fmt.Errorf("write portable migration backup %s: %w", backupPath, writeErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return "", false, fmt.Errorf("close portable migration backup %s: %w", backupPath, closeErr)
		}
	} else if !errors.Is(err, fs.ErrExist) {
		return "", false, fmt.Errorf("create portable migration backup %s: %w", backupPath, err)
	} else {
		existing, readErr := os.ReadFile(backupPath)
		if readErr != nil {
			return "", false, fmt.Errorf("verify portable migration backup %s: %w", backupPath, readErr)
		}
		if !bytes.Equal(existing, content) {
			return "", false, fmt.Errorf("portable migration backup collision at %s", backupPath)
		}
	}
	relative, err := filepath.Rel(dataRoot, backupPath)
	if err != nil {
		return "", false, err
	}
	return filepath.ToSlash(relative), true, nil
}

func completePortableDataMigration(dataRoot string, backups []string, projectCount int) error {
	if _, complete, err := readPortableDataMigrationReceipt(dataRoot); err != nil || complete {
		return err
	}
	backups = append([]string(nil), backups...)
	sort.Strings(backups)
	receipt := portableDataMigrationReceipt{
		Version: portableDataMigrationVersion, CompletedAt: time.Now().UTC(),
		ProjectCount: projectCount, Backups: compactStrings(backups),
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encode portable data migration receipt: %w", err)
	}
	if err := writePortableMigrationFileAtomic(portableDataMigrationReceiptPath(dataRoot), append(raw, '\n')); err != nil {
		return fmt.Errorf("write portable data migration receipt: %w", err)
	}
	return nil
}

func readPortableDataMigrationReceipt(dataRoot string) (portableDataMigrationReceipt, bool, error) {
	path := portableDataMigrationReceiptPath(dataRoot)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return portableDataMigrationReceipt{}, false, nil
	}
	if err != nil {
		return portableDataMigrationReceipt{}, false, fmt.Errorf("read portable data migration receipt: %w", err)
	}
	var receipt portableDataMigrationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return portableDataMigrationReceipt{}, false, fmt.Errorf("decode portable data migration receipt: %w", err)
	}
	if receipt.Version != portableDataMigrationVersion {
		return portableDataMigrationReceipt{}, false, fmt.Errorf(
			"portable data migration receipt version %d does not match supported version %d",
			receipt.Version, portableDataMigrationVersion,
		)
	}
	return receipt, true, nil
}

func portableDataMigrationReceiptPath(dataRoot string) string {
	return filepath.Join(dataRoot, "backups", portableDataBackupDirectory, "migration.json")
}

func writePortableMigrationFileAtomic(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".portable-migration-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := values[:0]
	for _, value := range values {
		if value == "" || (len(result) > 0 && result[len(result)-1] == value) {
			continue
		}
		result = append(result, value)
	}
	return result
}
