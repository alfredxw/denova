package harnessstate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"denova/config"
)

const publishedReadyFileName = "ready"

// PublishedReady is the only runtime selector between the released v0.3.3
// live-State behavior and the Draft/Published boundary. The marker is kept
// outside both visible State trees so it can never become model context.
func PublishedReady(cfg *config.Config) (bool, error) {
	path, err := publishedReadyPath(cfg)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect published Harness State marker: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("published Harness State marker is not a regular file")
	}
	return true, nil
}

// MarkPublishedReady commits the runtime selector only after the initial
// Published snapshot is durable. A crash before this point safely retries the
// one-time migration from the still-authoritative v0.3.3 State.
func MarkPublishedReady(cfg *config.Config) error {
	path, err := publishedReadyPath(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create published Harness State runtime directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".published-ready-*.tmp")
	if err != nil {
		return fmt.Errorf("create published Harness State marker: %w", err)
	}
	temporaryPath := temporary.Name()
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.WriteString("v1\n")
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temporaryPath, path)
	}
	if err != nil {
		_ = os.Remove(temporaryPath)
		return fmt.Errorf("commit published Harness State marker: %w", err)
	}
	return nil
}

func publishedReadyPath(cfg *config.Config) (string, error) {
	if cfg == nil {
		return "", errors.New("published Harness State config is nil")
	}
	dataDir := strings.TrimSpace(cfg.DataDir())
	if dataDir == "" {
		return "", errors.New("published Harness State data directory is required")
	}
	return filepath.Join(dataDir, "runtime", "harness-state-published", publishedReadyFileName), nil
}
