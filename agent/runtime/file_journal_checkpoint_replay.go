package runtime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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
// only its bounded generation tail. If the active generation is corrupt, each
// previous durable candidate is tried from a fresh reducer state.
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
	var failures error
	for index, generation := range j.generationCandidates {
		candidate := freshCheckpointTarget(target)
		var checkpoint *harnessCheckpoint
		var stats JournalReplayStats
		if generation.SnapshotFile != "" {
			snapshotPath, err := j.resolveGenerationFile(generation.SnapshotFile)
			if err != nil {
				failures = errors.Join(failures, err)
				continue
			}
			encoded, err := os.ReadFile(snapshotPath)
			stats.BytesRead += int64(len(encoded))
			stats.SnapshotBytesRead += int64(len(encoded))
			if err != nil {
				failures = errors.Join(failures, fmt.Errorf("read generation %d snapshot: %w", generation.Generation, err))
				continue
			}
			loaded, err := decodeFileJournalSnapshot(bytes.TrimSpace(encoded), generation)
			if err != nil {
				failures = errors.Join(failures, fmt.Errorf("decode generation %d snapshot: %w", generation.Generation, err))
				continue
			}
			if err := restoreHarnessCheckpoint(&candidate, loaded); err != nil {
				failures = errors.Join(failures, fmt.Errorf("restore generation %d snapshot: %w", generation.Generation, err))
				continue
			}
			checkpoint = &loaded
			stats.SnapshotGeneration = generation.Generation
		}
		tailPath, err := j.resolveGenerationFile(generation.TailFile)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		replay, err := j.scanGenerationTailLocked(ctx, tailPath, candidate.cursor, generation.SnapshotFile != "", index == 0, func(event Event) error {
			return candidate.reduce(event)
		})
		stats.BytesRead += replay.stats.BytesRead
		stats.TailBytesRead += replay.stats.BytesRead
		stats.RecordsRead += replay.stats.RecordsRead
		stats.EventsRead += replay.stats.EventsRead
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("replay generation %d tail: %w", generation.Generation, err))
			continue
		}
		selectedGeneration := generation
		selectedTailPath := tailPath
		selectedReplay := replay
		continuedIntoActiveTail := false
		// If the active snapshot is corrupt, its previous generation is still a
		// complete base for the active checkpoint: the frozen previous tail ends
		// exactly at Active.CheckpointCursor. Continue with the current active
		// tail so transactions committed after that checkpoint are not lost.
		if index > 0 && len(j.generationCandidates) > 0 {
			active := j.generationCandidates[0]
			if active.Generation > generation.Generation && replay.cursor == active.CheckpointCursor {
				activeTailPath, resolveErr := j.resolveGenerationFile(active.TailFile)
				if resolveErr == nil {
					continued := freshCheckpointTarget(target)
					base, checkpointErr := candidate.checkpoint()
					if checkpointErr == nil {
						checkpointErr = restoreHarnessCheckpoint(&continued, base)
					}
					if checkpointErr == nil {
						continuation, continuationErr := j.scanGenerationTailLocked(ctx, activeTailPath, active.CheckpointCursor, true, true, func(event Event) error {
							return continued.reduce(event)
						})
						stats.BytesRead += continuation.stats.BytesRead
						stats.TailBytesRead += continuation.stats.BytesRead
						stats.RecordsRead += continuation.stats.RecordsRead
						stats.EventsRead += continuation.stats.EventsRead
						if continuationErr == nil {
							candidate = continued
							selectedGeneration = active
							selectedTailPath = activeTailPath
							selectedReplay = continuation
							continuedIntoActiveTail = true
						} else {
							failures = errors.Join(failures, fmt.Errorf("continue generation %d tail from previous snapshot: %w", active.Generation, continuationErr))
						}
					} else {
						failures = errors.Join(failures, fmt.Errorf("clone previous generation recovery state: %w", checkpointErr))
					}
				} else {
					failures = errors.Join(failures, resolveErr)
				}
			}
		}
		*target = candidate
		j.initialized = true
		j.cursor = selectedReplay.cursor
		j.needsNewline = selectedReplay.needsNewline
		j.lastTailHash = selectedReplay.tailHash
		j.tailBytes, j.tailRecords = selectedReplay.bytes, selectedReplay.records
		j.lastReplay = stats
		j.tailPath = selectedTailPath
		j.activeGeneration = selectedGeneration
		j.checkpoint = checkpoint
		j.commandIndex = selectedReplay.commands
		j.indexReady = selectedGeneration.SnapshotFile == ""
		if index > 0 {
			log.Printf("[agent-runtime] fell back to previous journal generation=%d binding=%+v", generation.Generation, j.binding)
			if !continuedIntoActiveTail {
				j.generationCandidates = append([]fileJournalGeneration{generation}, j.generationCandidates[index+1:]...)
			}
		}
		return stats, nil
	}
	if failures == nil {
		failures = fmt.Errorf("file journal has no recoverable generation")
	}
	return JournalReplayStats{}, failures
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
