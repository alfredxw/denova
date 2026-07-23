package revisionfile

import (
	"fmt"
	"os"
	"path/filepath"
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
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open revision file directory for sync: %w", err)
	}
	defer directory.Close()
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync revision file directory: %w", err)
	}
	return nil
}
