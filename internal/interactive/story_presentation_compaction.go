package interactive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/localfs"
)

const (
	storyPresentationCompactionMinBytes      = 4 * 1024 * 1024
	storyPresentationCompactionMinEvents     = 128
	storyPresentationCompactionEventsPerTurn = 50
)

// OptimizeBloatedStoryStorage performs a recoverable one-time migration for
// journals produced by the former per-stream-delta display persistence. Normal
// append-only stories are skipped from catalog metadata without opening their
// canonical JSONL. Every changed file is copied to an adjacent .bak before an
// atomic rewrite, and the rebuilt sidecar remains disposable.
func (s *Store) OptimizeBloatedStoryStorage() (int, error) {
	if s == nil {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.readIndexLocked()
	if err != nil {
		return 0, err
	}
	optimized := 0
	for _, summary := range index.Stories {
		path := s.storyPath(summary.ID)
		info, statErr := os.Stat(path)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return optimized, statErr
		}
		turns := max(1, summary.TurnCount)
		if info.Size() < storyPresentationCompactionMinBytes ||
			summary.Events < storyPresentationCompactionMinEvents ||
			summary.Events <= turns*storyPresentationCompactionEventsPerTurn {
			continue
		}
		changed, compactErr := s.compactStoryPresentationHistoryLocked(summary.ID)
		if compactErr != nil {
			return optimized, compactErr
		}
		if changed {
			optimized++
		}
	}
	return optimized, nil
}

func (s *Store) compactStoryPresentationHistoryLocked(storyID string) (bool, error) {
	release, err := s.acquireStoryMutationLeaseLocked(storyID)
	if err != nil {
		return false, err
	}
	defer release()

	meta, events, err := s.readStoryConversationJournalLocked(storyID, true)
	if err != nil {
		return false, err
	}
	displayRevisions := 0
	for _, event := range events {
		if event.Envelope.Type == StoryEventTypeTurnDisplayAppended {
			displayRevisions++
		}
	}
	if displayRevisions < storyPresentationCompactionMinEvents {
		return false, nil
	}
	projected, err := projectStoryEventOverlays(events)
	if err != nil {
		return false, fmt.Errorf("project story display revisions before compaction: %w", err)
	}
	projectedTurns := make(map[string]StoryEventRecord)
	for _, event := range projected {
		if event.Envelope.Type == StoryEventTypeTurn {
			projectedTurns[event.Envelope.ID] = event
		}
	}
	compacted := make([]StoryEventRecord, 0, len(events)-displayRevisions)
	for _, event := range events {
		switch event.Envelope.Type {
		case StoryEventTypeTurnDisplayAppended:
			continue
		case StoryEventTypeTurn:
			compacted = append(compacted, projectedTurns[event.Envelope.ID])
		default:
			compacted = append(compacted, event)
		}
	}
	path := s.storyPath(storyID)
	backupPath, err := backupStoryForPresentationCompaction(path)
	if err != nil {
		return false, fmt.Errorf("backup story before presentation compaction: %w", err)
	}
	if err := s.rewriteStoryLocked(storyID, meta, compacted); err != nil {
		return false, fmt.Errorf("compact story presentation history (backup %s): %w", backupPath, err)
	}
	if _, err := s.publishStorySummaryLocked(storyID); err != nil {
		return false, fmt.Errorf("rebuild compacted story projection (backup %s): %w", backupPath, err)
	}
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[interactive-story] compacted superseded display revisions story_id=%s removed=%d backup=%s",
		storyID, displayRevisions, backupPath,
	))
	return true, nil
}

func backupStoryForPresentationCompaction(path string) (string, error) {
	backupPath := fmt.Sprintf("%s.presentation-v1.%s.%d.bak", path, time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	source, err := os.Open(path)
	if err != nil {
		return "", err
	}
	backup, err := os.OpenFile(backupPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = source.Close()
		return "", err
	}
	_, copyErr := io.Copy(backup, source)
	sourceCloseErr := source.Close()
	if copyErr == nil {
		copyErr = backup.Sync()
	}
	backupCloseErr := backup.Close()
	if copyErr != nil || sourceCloseErr != nil || backupCloseErr != nil {
		return "", errors.Join(copyErr, sourceCloseErr, backupCloseErr)
	}
	if err := localfs.SyncDirectory(filepath.Dir(path)); err != nil {
		return "", err
	}
	return strings.TrimSpace(backupPath), nil
}
