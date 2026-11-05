package config

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"denova/internal/revisionfile"
)

func preserveSettingsMigrationBackup(path string, snapshot revisionfile.Snapshot) (string, error) {
	if !snapshot.Exists {
		return "", errors.New("cannot back up a missing settings file")
	}
	sum := sha256.Sum256(snapshot.Content)
	backupPath := fmt.Sprintf("%s.pre-settings-migration-%x.bak", path, sum[:6])
	_, err := revisionfile.ReplaceIfRevision(
		context.Background(),
		backupPath,
		revisionfile.MissingRevision,
		snapshot.Content,
		revisionfile.Options{FileMode: snapshot.Mode.Perm(), DirectoryMode: 0o755},
	)
	if err == nil {
		return backupPath, nil
	}
	if !errors.Is(err, revisionfile.ErrRevisionConflict) {
		return "", fmt.Errorf("back up legacy settings to %s: %w", backupPath, err)
	}
	existing, readErr := revisionfile.Read(context.Background(), backupPath)
	if readErr != nil {
		return "", fmt.Errorf("verify legacy settings backup %s: %w", backupPath, readErr)
	}
	if !existing.Exists || !bytes.Equal(existing.Content, snapshot.Content) {
		return "", fmt.Errorf("legacy settings backup collision at %s", backupPath)
	}
	return backupPath, nil
}
