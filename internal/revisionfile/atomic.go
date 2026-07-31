package revisionfile

import (
	"fmt"
	"os"
	"path/filepath"

	"denova/internal/localfs"
)

func atomicReplace(path string, content []byte, fileMode, directoryMode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, directoryMode); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".denova-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err = temp.Chmod(fileMode); err != nil {
		return err
	}
	if _, err = temp.Write(content); err != nil {
		return err
	}
	if err = temp.Sync(); err != nil {
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tempPath, path); err != nil {
		return err
	}
	if err := localfs.SyncDirectory(dir); err != nil {
		return fmt.Errorf("sync revision file directory: %w", err)
	}
	return nil
}
