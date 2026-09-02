package app

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"denova/internal/localfs"
)

const unreleasedAgentTranscriptDirectory = "agent-transcripts"

// backupUnreleasedAgentTranscripts removes the unreleased second transcript
// root from the active layout without treating it as migration input. The
// startup lease serializes this same-volume rename; absence is the durable,
// retry-safe completion marker.
func backupUnreleasedAgentTranscripts(dataDir string) (string, error) {
	source := filepath.Join(dataDir, unreleasedAgentTranscriptDirectory)
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect unreleased Agent transcript directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("unreleased Agent transcript path is not a regular directory: %s", source)
	}
	backupRoot := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return "", fmt.Errorf("create unreleased Agent transcript backup directory: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	destination := filepath.Join(backupRoot, unreleasedAgentTranscriptDirectory+"-unreleased-"+stamp)
	for suffix := 2; ; suffix++ {
		if _, statErr := os.Lstat(destination); os.IsNotExist(statErr) {
			break
		} else if statErr != nil {
			return "", fmt.Errorf("inspect unreleased Agent transcript backup destination: %w", statErr)
		}
		destination = filepath.Join(backupRoot, fmt.Sprintf("%s-unreleased-%s-%d", unreleasedAgentTranscriptDirectory, stamp, suffix))
	}
	if err := os.Rename(source, destination); err != nil {
		return "", fmt.Errorf("backup unreleased Agent transcript directory: %w", err)
	}
	if err := localfs.SyncDirectory(backupRoot); err != nil {
		return destination, fmt.Errorf("sync unreleased Agent transcript backup directory: %w", err)
	}
	if err := localfs.SyncDirectory(dataDir); err != nil {
		return destination, fmt.Errorf("sync Denova data directory after Agent transcript backup: %w", err)
	}
	slog.Info("[internal/app/unreleased_agent_transcripts.go] backed up unreleased Agent transcript directory",
		"source", source, "destination", destination)
	return destination, nil
}
