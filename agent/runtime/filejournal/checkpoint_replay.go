package filejournal

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"io"
	"os"
	"path/filepath"
)

type generationTailReplay struct {
	stats        runstate.JournalReplayStats
	cursor       runstate.Cursor
	bytes        int64
	records      int64
	needsNewline bool
	tailHash     string
	commands     map[runstate.CommandID]runstate.CommandRecord
}

// ReplayCheckpoint restores a checksummed reducer checkpoint and streams
// only its bounded canonical tail. A previous generation may reconstruct a
// corrupt active snapshot, but it can never replace the active generation:
// publishing recovered state always requires a complete active-tail replay.
func (j *journal) ReplayCheckpoint(ctx context.Context, target runstate.JournalCheckpointState) (runstate.JournalReplayStats, error) {
	if err := ctx.Err(); err != nil {
		return runstate.JournalReplayStats{}, err
	}
	if target == nil {
		return runstate.JournalReplayStats{}, fmt.Errorf("file journal checkpoint target is nil")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return runstate.JournalReplayStats{}, fmt.Errorf("file journal is closed")
	}
	return j.replayCheckpointLocked(ctx, target)
}

func (j *journal) replayCheckpointLocked(ctx context.Context, target runstate.JournalCheckpointState) (runstate.JournalReplayStats, error) {
	if len(j.generationCandidates) == 0 {
		return runstate.JournalReplayStats{}, fmt.Errorf("file journal has no active generation")
	}
	// A failed validation must invalidate any earlier in-memory replay. runstate.Cursor is
	// deliberately left untouched for diagnostics, but Append will be forced
	// through this canonical replay again before it can write.
	j.initialized = false
	j.indexReady = false
	active := j.generationCandidates[0]
	activeState, stats, snapshotErr := j.restoreGenerationSnapshotLocked(target, active)
	if snapshotErr == nil {
		activeTailPath, err := j.resolveGenerationFile(active.TailFile)
		if err != nil {
			return stats, err
		}
		activeTail, err := j.scanGenerationTailLocked(ctx, activeTailPath, activeState.Cursor(), active.SnapshotFile != "", true, func(event runstate.Event) error {
			return activeState.Reduce(event)
		})
		addGenerationTailStats(&stats, activeTail)
		if err != nil {
			return stats, fmt.Errorf("replay canonical active generation %d tail: %w", active.Generation, err)
		}
		if err := j.publishGenerationReplayLocked(target, activeState, active, activeTailPath, activeTail, stats); err != nil {
			return stats, err
		}
		return stats, nil
	}

	// A previous generation is allowed to replace only the unreadable snapshot
	// bytes. Its fully replayed state must land exactly on the active checkpoint,
	// after which the canonical active tail is mandatory.
	if active.SnapshotFile == "" || len(j.generationCandidates) < 2 {
		return stats, fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr)
	}
	previous := j.generationCandidates[1]
	previousState, previousStats, previousSnapshotErr := j.restoreGenerationSnapshotLocked(target, previous)
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
	previousTail, err := j.scanGenerationTailLocked(ctx, previousTailPath, previousState.Cursor(), true, false, func(event runstate.Event) error {
		return previousState.Reduce(event)
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
	reconstructed, err := previousState.MarshalCheckpoint()
	if err != nil {
		return stats, errors.Join(snapshotErr, fmt.Errorf("checkpoint reconstructed generation %d state: %w", previous.Generation, err))
	}
	continued := target.Fresh()
	if err := continued.RestoreCheckpoint(reconstructed); err != nil {
		return stats, errors.Join(snapshotErr, fmt.Errorf("restore reconstructed active checkpoint: %w", err))
	}
	activeTailPath, err := j.resolveGenerationFile(active.TailFile)
	if err != nil {
		return stats, errors.Join(snapshotErr, err)
	}
	activeTail, err := j.scanGenerationTailLocked(ctx, activeTailPath, active.CheckpointCursor, true, true, func(event runstate.Event) error {
		return continued.Reduce(event)
	})
	addGenerationTailStats(&stats, activeTail)
	if err != nil {
		return stats, errors.Join(
			fmt.Errorf("restore canonical active generation %d snapshot: %w", active.Generation, snapshotErr),
			fmt.Errorf("replay canonical active generation %d tail after snapshot reconstruction: %w", active.Generation, err),
		)
	}
	if err := j.publishGenerationReplayLocked(target, continued, active, activeTailPath, activeTail, stats); err != nil {
		return stats, err
	}
	return stats, nil
}

func (j *journal) restoreGenerationSnapshotLocked(
	template runstate.JournalCheckpointState,
	generation journalGeneration,
) (runstate.JournalCheckpointState, runstate.JournalReplayStats, error) {
	candidate := template.Fresh()
	var stats runstate.JournalReplayStats
	if generation.SnapshotFile == "" {
		return candidate, stats, nil
	}
	snapshotPath, err := j.resolveGenerationFile(generation.SnapshotFile)
	if err != nil {
		return candidate, stats, err
	}
	encoded, err := os.ReadFile(snapshotPath)
	stats.BytesRead = int64(len(encoded))
	stats.SnapshotBytesRead = int64(len(encoded))
	if err != nil {
		return candidate, stats, fmt.Errorf("read generation %d snapshot: %w", generation.Generation, err)
	}
	loaded, err := decodeFileJournalSnapshot(bytes.TrimSpace(encoded), generation)
	if err != nil {
		return candidate, stats, fmt.Errorf("decode generation %d snapshot: %w", generation.Generation, err)
	}
	if err := candidate.RestoreCheckpoint(loaded); err != nil {
		return candidate, stats, fmt.Errorf("restore generation %d snapshot: %w", generation.Generation, err)
	}
	if candidate.Cursor() != generation.CheckpointCursor {
		return candidate, stats, fmt.Errorf("restore generation %d cursor %d, want %d", generation.Generation, candidate.Cursor(), generation.CheckpointCursor)
	}
	stats.SnapshotGeneration = generation.Generation
	return candidate, stats, nil
}

func addGenerationTailStats(stats *runstate.JournalReplayStats, replay generationTailReplay) {
	if stats == nil {
		return
	}
	stats.BytesRead += replay.stats.BytesRead
	stats.TailBytesRead += replay.stats.BytesRead
	stats.RecordsRead += replay.stats.RecordsRead
	stats.EventsRead += replay.stats.EventsRead
}

func addJournalReplayStats(target *runstate.JournalReplayStats, addition runstate.JournalReplayStats) {
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

func (j *journal) publishGenerationReplayLocked(
	target runstate.JournalCheckpointState,
	state runstate.JournalCheckpointState,
	active journalGeneration,
	tailPath string,
	replay generationTailReplay,
	stats runstate.JournalReplayStats,
) error {
	if err := target.PublishFrom(state); err != nil {
		return fmt.Errorf("publish file journal checkpoint state: %w", err)
	}
	j.initialized = true
	j.cursor = replay.cursor
	j.needsNewline = replay.needsNewline
	j.lastTailHash = replay.tailHash
	j.tailBytes, j.tailRecords = replay.bytes, replay.records
	j.lastReplay = stats
	j.tailPath = tailPath
	j.activeGeneration = active
	j.commandIndex = replay.commands
	j.indexReady = active.SnapshotFile == ""
	return nil
}

func (j *journal) scanGenerationTailLocked(
	ctx context.Context,
	path string,
	start runstate.Cursor,
	required bool,
	repairTornTail bool,
	reduce func(runstate.Event) error,
) (generationTailReplay, error) {
	result := generationTailReplay{cursor: start, commands: make(map[runstate.CommandID]runstate.CommandRecord)}
	file, err := j.openReplayFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	reader := bufio.NewReaderSize(file, journalReadBufferBytes)
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
			if accepted, ok := event.Payload.(runstate.CommandAcceptedEvent); ok {
				result.commands[accepted.CommandID] = runstate.CommandRecord{
					Receipt:     runstate.Receipt{CommandID: accepted.CommandID, OperationID: accepted.OperationID, Cursor: event.Cursor},
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
