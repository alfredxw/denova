package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CompletedUpdate projects an existing successful transaction. It is not a
// second receipt and remains available while a subsequent update is staged.
type CompletedUpdate struct {
	ID              string    `json:"id"`
	PreviousVersion string    `json:"previous_version"`
	Version         string    `json:"version"`
	CompletedAt     time.Time `json:"completed_at"`
}

func (s *Service) CompletedUpdate() (*CompletedUpdate, error) {
	root := updateDataDir(filepath.Dir(s.executablePath))
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read completed updates: %w", err)
	}
	var completed *CompletedUpdate
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "pending-") {
			continue
		}
		path := filepath.Join(root, entry.Name(), manifestFileName)
		manifest, err := readManifest(path)
		if err != nil || manifest.State != PhaseSucceeded || manifest.PreviousVersion == "" || !sameVersion(manifest.Version, s.currentVersion) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat completed update: %w", err)
		}
		if completed == nil || info.ModTime().After(completed.CompletedAt) {
			completed = &CompletedUpdate{ID: manifest.ID, PreviousVersion: manifest.PreviousVersion, Version: manifest.Version, CompletedAt: info.ModTime().UTC()}
		}
	}
	return completed, nil
}
