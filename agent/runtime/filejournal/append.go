package filejournal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"log/slog"
	"os"
	"path/filepath"
)

func (j *journal) Append(ctx context.Context, expected runstate.Cursor, payloads []runstate.EventPayload) ([]runstate.Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(payloads) == 0 {
		return nil, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if j.closed {
		return nil, fmt.Errorf("file journal is closed")
	}
	if !j.initialized {
		state := runstate.NewJournalProjectionState(j.binding)
		if _, err := j.replayCheckpointLocked(ctx, state); err != nil {
			return nil, err
		}
	}
	current := j.cursor
	if current != expected {
		return nil, fmt.Errorf("journal cursor conflict: have %d, expected %d", current, expected)
	}
	committed := make([]runstate.Event, 0, len(payloads))
	encoded := make([]runstate.JournalEvent, 0, len(payloads))
	for _, payload := range payloads {
		current++
		event := runstate.Event{Cursor: current, Durability: runstate.EventDurable, Payload: payload}
		diskEvent, encodeErr := runstate.EncodeJournalEvent(event)
		if encodeErr != nil {
			return nil, encodeErr
		}
		committed = append(committed, event)
		encoded = append(encoded, diskEvent)
	}
	body := journalTransactionBody{
		Version: journalVersion,
		Start:   committed[0].Cursor,
		End:     committed[len(committed)-1].Cursor,
		Events:  encoded,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode file journal checksum body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	transaction := journalTransaction{
		Version: body.Version, Start: body.Start, End: body.End, Events: body.Events,
		Checksum: hex.EncodeToString(digest[:]),
	}
	line, err := json.Marshal(transaction)
	if err != nil {
		return nil, fmt.Errorf("encode file journal transaction: %w", err)
	}
	prefix := []byte(nil)
	if j.needsNewline {
		prefix = []byte{'\n'}
	}
	record := append(prefix, line...)
	record = append(record, '\n')
	_, statErr := os.Stat(j.tailPath)
	tailCreated := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !tailCreated {
		return nil, fmt.Errorf("stat file journal before append: %w", statErr)
	}
	file, err := os.OpenFile(j.tailPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open file journal for append: %w", err)
	}
	writeErr := writeAll(file, record)
	// Even after Write reports an error, Sync can establish that the complete
	// record which read-back observes reached the durability boundary.
	syncErr := j.syncFile(file)
	closeErr := j.closeFile(file)
	if writeErr != nil || syncErr != nil || closeErr != nil {
		operationErr := errors.Join(
			wrapOptionalError("append file journal", writeErr),
			wrapOptionalError("sync file journal", syncErr),
			wrapOptionalError("close file journal", closeErr),
		)
		confirmed, reconcileErr := j.reconcileAppend(expected, committed, encoded)
		if reconcileErr != nil {
			return nil, errors.Join(operationErr, fmt.Errorf("reconcile ambiguous file journal append: %w", reconcileErr))
		}
		// Read-back resolves a lost Write/Close result only after Sync itself
		// succeeded. Seeing page-cache bytes cannot mask a failed fsync.
		if !confirmed || syncErr != nil {
			return nil, operationErr
		}
		if tailCreated {
			if err := j.syncDirectory(filepath.Dir(j.tailPath)); err != nil {
				return nil, fmt.Errorf("sync file journal directory: %w", err)
			}
		}
		return cloneJournalEvents(committed), nil
	}
	// File.Sync is the durability boundary for an existing append. Directory
	// sync is needed only when O_CREATE publishes the tail's namespace entry.
	if tailCreated {
		if err := j.syncDirectory(filepath.Dir(j.tailPath)); err != nil {
			_, reconcileErr := j.reconcileAppend(expected, committed, encoded)
			if reconcileErr != nil {
				return nil, errors.Join(
					fmt.Errorf("sync file journal directory: %w", err),
					fmt.Errorf("reconcile directory-sync-uncertain append: %w", reconcileErr),
				)
			}
			return nil, fmt.Errorf("sync file journal directory: %w", err)
		}
	}
	j.cursor = committed[len(committed)-1].Cursor
	j.needsNewline = false
	j.lastTailHash = journalRecordHash(line)
	j.tailBytes += int64(len(record))
	j.tailRecords++
	indexChainReady := j.indexReady
	j.indexCommittedCommands(committed)
	var indexErr error
	if j.activeGeneration.SnapshotFile != "" {
		indexErr = nil
	} else if indexChainReady {
		indexErr = j.appendCommandIndexLocked(expected, committed)
	} else {
		indexErr = j.rewriteCommandIndexLocked()
	}
	if indexErr != nil {
		// The index is a rebuildable acceleration structure. The canonical
		// transaction has crossed its durability boundary, so an index write
		// failure must not turn a confirmed append into a false command error.
		j.indexReady = false
		slog.ErrorContext(ctx, fmt.Sprintf("[agent-runtime] command index update deferred journal=%s cursor=%d error=%v", filepath.Base(j.path), j.cursor, indexErr))
	}
	return cloneJournalEvents(committed), nil
}
