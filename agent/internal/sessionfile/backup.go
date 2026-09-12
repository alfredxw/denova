package sessionfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/alfredxw/denova/agent/session"
	"io"
	"os"
	"path/filepath"

	"github.com/alfredxw/denova/agent/internal/localfs"
)

func isResilienceRecord(kind string) bool {
	switch kind {
	case "session.input", "session.input_update", "session.control", "turn.checkpoint", "turn.tool", "turn.interaction", "turn.interaction_response":
		return true
	default:
		return false
	}
}

// The exact allocated Session path stays unchanged. A completed backup is
// installed before the first record that the latest release cannot understand.
func backupReleasedTranscript(path, upgrade string) error {
	destination := path + ".pre-" + upgrade + ".bak"
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Agent transcript backup is not a regular file: %s", destination)
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
	temporary, err := os.CreateTemp(filepath.Dir(path), ".agent-backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	_, copyErr := io.Copy(temporary, source)
	err = errors.Join(copyErr, temporary.Sync(), temporary.Close())
	if err != nil {
		return fmt.Errorf("preserve released Agent transcript: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return localfs.SyncDirectory(filepath.Dir(path))
}

func usesIncrementalCompaction(record session.Record) bool {
	if record.Kind != "session.capability_set" {
		return false
	}
	var payload struct {
		Capability string `json:"capability"`
		State      struct {
			Version uint16 `json:"version"`
		} `json:"state"`
	}
	return json.Unmarshal(record.Data, &payload) == nil && payload.Capability == "agent.compaction" && payload.State.Version == 2
}
