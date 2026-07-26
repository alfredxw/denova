package runtime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type generationTailReplay struct {
	stats        JournalReplayStats
	cursor       Cursor
	bytes        int64
	records      int64
	needsNewline bool
	tailHash     string
	commands     map[CommandID]CommandRecord
}

// ReplayHarnessState restores a checksummed reducer checkpoint and streams
// only its bounded canonical tail. A previous generation may reconstruct a
// corrupt active snapshot, but it can never replace the active generation:
// publishing recovered state always requires a complete active-tail replay.
func (j *fileJournal) ReplayHarnessState(ctx context.Context, target *harnessState) (JournalReplayStats, error) {
	if err := ctx.Err(); err != nil {
		return JournalReplayStats{}, err
	}
	if target == nil {
		return JournalReplayStats{}, fmt.Errorf("file journal checkpoint target is nil")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return JournalReplayStats{}, fmt.Errorf("file journal is closed")
	}
	return j.replayHarnessStateLocked(ctx, target)
}

func (j *fileJournal) replayHarnessStateLocked(ctx context.Context, target *harnessState) (JournalReplayStats, error) {
	if len(j.generationCandidates) == 0 {
		return JournalReplayStats{}, fmt.Errorf("file journal has no active generation")
	}
	// A failed validation must invalidate any earlier in-memory replay. Cursor is
	// deliberately left untouched for diagnostics, but Append will be forced
	// through this canonical replay again before it can write.
	j.initialized = false
	j.indexReady = false
	active := j.generationCandidates[0]
	activeState, activeCheckpoint, stats, snapshotErr := j.restoreGenerationSnapshotLocked(target, active)
	if snapshotErr == nil {
		activeTailPath, err := j.resolveGenerationFile(active.TailFile)
		if err != nil {
			return stats, err
		}
		activeTail, err := j.scanGenerationTailLocked(ctx, activeTailPath, activeState.cursor, active.SnapshotFile != "", true, func(event Event) error {
			return activeState.reduce(event)
		})
		addGenerationTailStats(&stats, activeTail)
		if err != nil {
			return stats, fmt.Errorf("replay canonical active generation %d tail: %w", active.Generation, err)
		}
		j.publishGenerationReplayLocked(target, activeState, active, activeTailPath, activeTail, stats, activeCheckpoint)
		return stats, nil
	}

	// A previous generation is allowed to replace only the unreadable snapshot
	// bytes. Its fully replayed state must land exactly on the active checkpoint,
	// after which the canonical active tail is mandatory.
	if active.SnapshotFile == "" || len(j.generationCandidates) < 2 {
		return stats, fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr)
	}
	previous := j.generationCandidates[1]
	previousState, _, previousStats, previousSnapshotErr := j.restoreGenerationSnapshotLocked(target, previous)
	addJournalReplayStats(&stats, previousStats)
	if previousSnapshotErr != nil {
		return stats, errors.Join(
			fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr),
			fmt.Errorf("restore previous generation %d snapshot: %w", previous.Generation, previousSnapshotErr),
		)
	}
	previousTailPath, err := j.resolveGenerationFile(previous.TailFile)
	if err != nil {
		return stats, errors.Join(snapshotErr, err)
	}
	previousTail, err := j.scanGenerationTailLocked(ctx, previousTailPath, previousState.cursor, true, false, func(event Event) error {
		return previousState.reduce(event)
	})
	addGenerationTailStats(&stats, previousTail)
	if err != nil {
		return stats, errors.Join(
			fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr),
			fmt.Errorf("rebuild active checkpoint from previous generation %d tail: %w", previous.Generation, err),
		)
	}
	if previousTail.cursor != active.CheckpointCursor {
		return stats, errors.Join(
			fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr),
			fmt.Errorf("previous generation %d rebuilt cursor %d, want active checkpoint cursor %d", previous.Generation, previousTail.cursor, active.CheckpointCursor),
		)
	}
	reconstructed, err := previousState.checkpoint()
	if err != nil {
		return stats, errors.Join(snapshotErr, fmt.Errorf("checkpoint reconstructed generation %d state: %w", previous.Generation, err))
	}
	continued := freshCheckpointTarget(target)
	if err := restoreHarnessCheckpoint(&continued, reconstructed); err != nil {
		return stats, errors.Join(snapshotErr, fmt.Errorf("restore reconstructed active checkpoint: %w", err))
	}
	activeTailPath, err := j.resolveGenerationFile(active.TailFile)
	if err != nil {
		return stats, errors.Join(snapshotErr, err)
	}
	activeTail, err := j.scanGenerationTailLocked(ctx, activeTailPath, active.CheckpointCursor, true, true, func(event Event) error {
		return continued.reduce(event)
	})
	addGenerationTailStats(&stats, activeTail)
	if err != nil {
		return stats, errors.Join(
			fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr),
			fmt.Errorf("replay canonical active generation %d tail after snapshot reconstruction: %w", active.Generation, err),
		)
	}
	j.publishGenerationReplayLocked(target, continued, active, activeTailPath, activeTail, stats, &reconstructed)
	return stats, nil
}

func (j *fileJournal) restoreGenerationSnapshotLocked(
	template *harnessState,
	generation fileJournalGeneration,
) (harnessState, *harnessCheckpoint, JournalReplayStats, error) {
	candidate := freshCheckpointTarget(template)
	var stats JournalReplayStats
	if generation.SnapshotFile == "" {
		return candidate, nil, stats, nil
	}
	snapshotPath, err := j.resolveGenerationFile(generation.SnapshotFile)
	if err != nil {
		return candidate, nil, stats, err
	}
	encoded, err := os.ReadFile(snapshotPath)
	stats.BytesRead = int64(len(encoded))
	stats.SnapshotBytesRead = int64(len(encoded))
	if err != nil {
		return candidate, nil, stats, fmt.Errorf("read generation %d snapshot: %w", generation.Generation, err)
	}
	loaded, err := decodeFileJournalSnapshot(bytes.TrimSpace(encoded), generation)
	if err != nil {
		return candidate, nil, stats, fmt.Errorf("decode generation %d snapshot: %w", generation.Generation, err)
	}
	if err := restoreHarnessCheckpoint(&candidate, loaded); err != nil {
		return candidate, nil, stats, fmt.Errorf("restore generation %d snapshot: %w", generation.Generation, err)
	}
	stats.SnapshotGeneration = generation.Generation
	return candidate, &loaded, stats, nil
}

func addGenerationTailStats(stats *JournalReplayStats, replay generationTailReplay) {
	if stats == nil {
		return
	}
	stats.BytesRead += replay.stats.BytesRead
	stats.TailBytesRead += replay.stats.BytesRead
	stats.RecordsRead += replay.stats.RecordsRead
	stats.EventsRead += replay.stats.EventsRead
}

func addJournalReplayStats(target *JournalReplayStats, addition JournalReplayStats) {
	if target == nil {
		return
	}
	target.BytesRead += addition.BytesRead
	target.SnapshotBytesRead += addition.SnapshotBytesRead
	target.TailBytesRead += addition.TailBytesRead
	target.RecordsRead += addition.RecordsRead
	target.EventsRead += addition.EventsRead
	if addition.SnapshotGeneration != 0 {
		target.SnapshotGeneration = addition.SnapshotGeneration
	}
}

func (j *fileJournal) publishGenerationReplayLocked(
	target *harnessState,
	state harnessState,
	active fileJournalGeneration,
	tailPath string,
	replay generationTailReplay,
	stats JournalReplayStats,
	checkpoint *harnessCheckpoint,
) {
	*target = state
	j.initialized = true
	j.cursor = replay.cursor
	j.needsNewline = replay.needsNewline
	j.lastTailHash = replay.tailHash
	j.tailBytes, j.tailRecords = replay.bytes, replay.records
	j.lastReplay = stats
	j.tailPath = tailPath
	j.activeGeneration = active
	j.checkpoint = checkpoint
	j.commandIndex = replay.commands
	j.indexReady = active.SnapshotFile == ""
}

func freshCheckpointTarget(template *harnessState) harnessState {
	state := newHarnessState(template.binding)
	state.retainTimeline = template.retainTimeline
	state.retainCommandIndex = template.retainCommandIndex
	state.maxRetainedEvents = template.maxRetainedEvents
	state.maxRetainedMessages = template.maxRetainedMessages
	state.maxRetainedCommands = template.maxRetainedCommands
	state.memoryLimits = template.memoryLimits.normalized()
	return state
}

func (j *fileJournal) scanGenerationTailLocked(
	ctx context.Context,
	path string,
	start Cursor,
	required bool,
	repairTornTail bool,
	reduce func(Event) error,
) (generationTailReplay, error) {
	result := generationTailReplay{cursor: start, commands: make(map[CommandID]CommandRecord)}
	file, err := j.openReplayFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	reader := bufio.NewReaderSize(file, fileJournalReadBufferBytes)
	var validBytes int64
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return result, err
		}
		record, readErr := reader.ReadBytes('\n')
		result.stats.BytesRead += int64(len(record))
		if len(record) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		lineNumber++
		hasNewline := len(record) > 0 && record[len(record)-1] == '\n'
		line := record
		if hasNewline {
			line = line[:len(line)-1]
		}
		if len(line) == 0 {
			_ = file.Close()
			return result, fmt.Errorf("line %d is empty", lineNumber)
		}
		events, decodeErr := decodeTransaction(line, result.cursor)
		if decodeErr != nil {
			isFinalPartial := errors.Is(readErr, io.EOF) && !hasNewline
			if isFinalPartial && repairTornTail && isSyntacticallyTornJSON(line, decodeErr) {
				if closeErr := file.Close(); closeErr != nil {
					return result, closeErr
				}
				if repairErr := backupAndRepairTornTailPath(path, validBytes); repairErr != nil {
					return result, repairErr
				}
				result.stats.BytesRead = validBytes
				break
			}
			_ = file.Close()
			return result, fmt.Errorf("line %d: %w", lineNumber, decodeErr)
		}
		result.stats.RecordsRead++
		result.stats.EventsRead += int64(len(events))
		for _, event := range events {
			if accepted, ok := event.Payload.(CommandAcceptedEvent); ok {
				result.commands[accepted.CommandID] = CommandRecord{
					Receipt:     Receipt{CommandID: accepted.CommandID, OperationID: accepted.OperationID, Cursor: event.Cursor},
					Fingerprint: accepted.Fingerprint,
				}
			}
			if reduce != nil {
				if err := reduce(event); err != nil {
					_ = file.Close()
					return result, fmt.Errorf("reduce cursor %d: %w", event.Cursor, err)
				}
			}
		}
		result.cursor = events[len(events)-1].Cursor
		validBytes += int64(len(record))
		result.bytes = validBytes
		result.records++
		result.tailHash = journalRecordHash(line)
		result.needsNewline = !hasNewline
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return result, readErr
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if err := file.Close(); err != nil {
		return result, err
	}
	return result, nil
}

func backupAndRepairTornTailPath(path string, validBytes int64) error {
	backupPath, err := copyRecoveryBackup(path)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open torn generation tail for repair (backup %s): %w", backupPath, err)
	}
	repairErr := file.Truncate(validBytes)
	if repairErr == nil {
		repairErr = file.Sync()
	}
	closeErr := file.Close()
	if repairErr != nil || closeErr != nil {
		return errors.Join(repairErr, closeErr)
	}
	return syncDirectory(filepath.Dir(path))
}
