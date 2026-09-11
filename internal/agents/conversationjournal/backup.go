package conversationjournal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"denova/internal/localfs"
)

func preserveFormatBackup(path, upgrade string) error {
	if strings.ContainsAny(upgrade, `/\:`) || upgrade == "." || upgrade == ".." {
		return errors.New("invalid journal format upgrade name")
	}
	destination := path + ".pre-" + upgrade + ".bak"
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("journal format backup is not a regular file: %s", destination)
		}
		return localfs.SyncDirectory(filepath.Dir(path))
	} else if !os.IsNotExist(err) {
		return err
	}
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	defer source.Close()
	temporary, err := os.CreateTemp(filepath.Dir(path), ".journal-backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	_, copyErr := io.Copy(temporary, source)
	err = errors.Join(copyErr, temporary.Sync(), temporary.Close())
	if err != nil {
		return fmt.Errorf("preserve original journal format: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return localfs.SyncDirectory(filepath.Dir(path))
}
