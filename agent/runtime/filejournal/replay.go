package filejournal

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const journalReadBufferBytes = 64 * 1024

// replayLocked validates the complete canonical stream while retaining at
// most one transaction of event payloads. The compact command receipt map is
// rebuilt separately because exact historical command idempotency must outlive
// the actor's bounded hot cache. reduce may be nil when a caller only needs
// cursor and command-index reconstruction (for example before the first
// append).
func (j *journal) replayLocked(
	ctx context.Context,
	repairTornTail bool,
	reduce func(runstate.Event) error,
) (runstate.JournalReplayStats, error) {
	if err := ctx.Err(); err != nil {
		return runstate.JournalReplayStats{}, err
	}
	if j.activeGeneration.SnapshotFile != "" {
		state := runstate.NewJournalCheckpointState(j.binding)
		stats, err := j.replayCheckpointLocked(ctx, state)
		if err != nil {
			return stats, err
		}
		if reduce != nil {
			for _, event := range state.RetainedEvents() {
				if err := reduce(event); err != nil {
					return stats, err
				}
			}
		}
		return stats, nil
	}
	file, err := j.openReplayFile(j.path)
	if errors.Is(err, os.ErrNotExist) {
		j.rememberReplay(0, false, "", map[runstate.CommandID]runstate.CommandRecord{}, runstate.JournalReplayStats{})
		return runstate.JournalReplayStats{}, nil
	}
	if err != nil {
		return runstate.JournalReplayStats{}, fmt.Errorf("open file journal for replay: %w", err)
	}
	reader := bufio.NewReaderSize(file, journalReadBufferBytes)
	commands := make(map[runstate.CommandID]runstate.CommandRecord)
	var stats runstate.JournalReplayStats
	var cursor runstate.Cursor
	var validBytes int64
	var tailHash string
	var needsNewline bool
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return stats, err
		}
		record, readErr := reader.ReadBytes('\n')
		stats.BytesRead += int64(len(record))
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
			return stats, fmt.Errorf("decode file journal line %d: empty record", lineNumber)
		}
		transactionEvents, decodeErr := decodeTransaction(line, cursor)
		if decodeErr != nil {
			isFinalPartial := errors.Is(readErr, io.EOF) && !hasNewline
			if isFinalPartial && repairTornTail && isSyntacticallyTornJSON(line, decodeErr) {
				if closeErr := file.Close(); closeErr != nil {
					return stats, fmt.Errorf("close torn file journal before repair: %w", closeErr)
				}
				file = nil
				if repairErr := j.backupAndRepairTornTail(validBytes); repairErr != nil {
					return stats, repairErr
				}
				stats.BytesRead = validBytes
				needsNewline = false
				break
			}
			_ = file.Close()
			return stats, fmt.Errorf("decode file journal line %d: %w", lineNumber, decodeErr)
		}
		stats.RecordsRead++
		stats.EventsRead += int64(len(transactionEvents))
		for _, event := range transactionEvents {
			if accepted, ok := event.Payload.(runstate.CommandAcceptedEvent); ok {
				commands[accepted.CommandID] = runstate.CommandRecord{
					Receipt: runstate.Receipt{
						CommandID: accepted.CommandID, OperationID: accepted.OperationID, Cursor: event.Cursor,
					},
					Fingerprint: accepted.Fingerprint,
				}
			}
			if reduce != nil {
				if err := reduce(event); err != nil {
					_ = file.Close()
					return stats, fmt.Errorf("reduce file journal at cursor %d: %w", event.Cursor, err)
				}
			}
		}
		cursor = transactionEvents[len(transactionEvents)-1].Cursor
		validBytes += int64(len(record))
		tailHash = journalRecordHash(line)
		needsNewline = !hasNewline
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return stats, fmt.Errorf("read file journal line %d: %w", lineNumber, readErr)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if file != nil {
		if closeErr := file.Close(); closeErr != nil {
			return stats, fmt.Errorf("close file journal after replay: %w", closeErr)
		}
	}
	j.rememberReplay(cursor, needsNewline, tailHash, commands, stats)
	if err := j.rewriteCommandIndexLocked(); err != nil {
		// Replay success is canonical; the acceleration index is rebuildable.
		j.indexReady = false
	}
	return stats, nil
}

func (j *journal) rememberReplay(
	cursor runstate.Cursor,
	needsNewline bool,
	tailHash string,
	commands map[runstate.CommandID]runstate.CommandRecord,
	stats runstate.JournalReplayStats,
) {
	j.initialized = true
	j.cursor = cursor
	j.needsNewline = needsNewline
	j.lastTailHash = tailHash
	j.commandIndex = commands
	j.indexReady = true
	j.lastReplay = stats
}

func journalRecordHash(line []byte) string {
	digest := sha256.Sum256(line)
	return hex.EncodeToString(digest[:])
}

func isSyntacticallyTornJSON(line []byte, decodeErr error) bool {
	var syntaxErr *json.SyntaxError
	if !errors.As(decodeErr, &syntaxErr) {
		return false
	}
	return syntaxErr.Offset >= int64(len(line)) && strings.Contains(syntaxErr.Error(), "unexpected end of JSON input")
}

func (j *journal) backupAndRepairTornTail(validBytes int64) error {
	backupPath, err := copyRecoveryBackup(j.path)
	if err != nil {
		return fmt.Errorf("back up torn file journal before repair: %w", err)
	}
	file, err := os.OpenFile(j.path, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open torn file journal for repair (backup %s): %w", backupPath, err)
	}
	repairErr := file.Truncate(validBytes)
	if repairErr == nil {
		repairErr = file.Sync()
	}
	closeErr := file.Close()
	if repairErr != nil {
		return fmt.Errorf("repair torn file journal tail (backup %s): %w", backupPath, repairErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close repaired file journal (backup %s): %w", backupPath, closeErr)
	}
	if err := syncDirectory(filepath.Dir(j.path)); err != nil {
		return fmt.Errorf("sync repaired file journal directory (backup %s): %w", backupPath, err)
	}
	return nil
}

// copyRecoveryBackup uses io.Copy so recovery does not briefly retain another
// full copy of a large journal merely to preserve the original bytes.
func copyRecoveryBackup(journalPath string) (string, error) {
	base := fmt.Sprintf("%s.recovery.%d.%d", journalPath, time.Now().UTC().UnixNano(), os.Getpid())
	for suffix := 0; ; suffix++ {
		backupPath := fmt.Sprintf("%s.%d.bak", base, suffix)
		source, err := os.Open(journalPath)
		if err != nil {
			return "", err
		}
		target, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			_ = source.Close()
			continue
		}
		if err != nil {
			_ = source.Close()
			return "", err
		}
		_, copyErr := io.Copy(target, source)
		sourceCloseErr := source.Close()
		if copyErr == nil {
			copyErr = target.Sync()
		}
		targetCloseErr := target.Close()
		if copyErr != nil || sourceCloseErr != nil || targetCloseErr != nil {
			return "", errors.Join(copyErr, sourceCloseErr, targetCloseErr)
		}
		if err := syncDirectory(filepath.Dir(journalPath)); err != nil {
			return "", err
		}
		return backupPath, nil
	}
}
