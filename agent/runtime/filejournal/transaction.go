package filejournal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"log/slog"
	"os"
	"path/filepath"
)

func (j *journal) LookupCommand(ctx context.Context, commandID runstate.CommandID) (runstate.CommandRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return runstate.CommandRecord{}, false, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return runstate.CommandRecord{}, false, fmt.Errorf("file journal is closed")
	}
	if j.activeGeneration.SnapshotFile != "" {
		if record, found, err := j.readCommandRecordLocked(commandID); err != nil || found {
			return record, found, err
		}
		return j.lookupTailCommandLocked(ctx, commandID)
	}
	if !j.indexReady {
		loaded, err := j.loadPersistedCommandIndexLocked()
		if err != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[agent-runtime] rebuilding invalid command index journal=%s error=%v", filepath.Base(j.path), err))
		}
		if !loaded {
			if _, replayErr := j.replayLocked(ctx, true, nil); replayErr != nil {
				return runstate.CommandRecord{}, false, replayErr
			}
		}
	}
	record, ok := j.commandIndex[commandID]
	if !ok {
		if persisted, found, err := j.readCommandRecordLocked(commandID); err != nil || found {
			return persisted, found, err
		}
	}
	return record, ok, nil
}

func wrapOptionalError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func (j *journal) reconcileAppend(expected runstate.Cursor, committed []runstate.Event, encoded []runstate.JournalEvent) (bool, error) {
	suffix := make([]runstate.Event, 0, len(committed))
	_, err := j.replayLocked(context.Background(), true, func(event runstate.Event) error {
		if len(suffix) == len(committed) {
			copy(suffix, suffix[1:])
			suffix[len(suffix)-1] = event
		} else {
			suffix = append(suffix, event)
		}
		return nil
	})
	if err != nil {
		j.initialized = false
		return false, err
	}
	if j.cursor != expected+runstate.Cursor(len(committed)) || len(suffix) < len(committed) {
		return false, nil
	}
	for index, event := range suffix {
		reloaded, err := runstate.EncodeJournalEvent(event)
		if err != nil {
			return false, err
		}
		if reloaded.Cursor != encoded[index].Cursor || reloaded.Type != encoded[index].Type || !bytes.Equal(reloaded.Data, encoded[index].Data) {
			return false, nil
		}
	}
	return true, nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func decodeTransaction(line []byte, current runstate.Cursor) ([]runstate.Event, error) {
	var transaction journalTransaction
	if err := json.Unmarshal(line, &transaction); err != nil {
		return nil, err
	}
	if transaction.Version != journalVersion {
		return nil, fmt.Errorf("unsupported version %d", transaction.Version)
	}
	body := journalTransactionBody{
		Version: transaction.Version,
		Start:   transaction.Start,
		End:     transaction.End,
		Events:  transaction.Events,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	if transaction.Checksum != hex.EncodeToString(digest[:]) {
		return nil, fmt.Errorf("checksum mismatch")
	}
	if len(transaction.Events) == 0 || transaction.Start != current+1 {
		return nil, fmt.Errorf("non-contiguous transaction start %d after %d", transaction.Start, current)
	}
	if transaction.End != transaction.Start+runstate.Cursor(len(transaction.Events))-1 {
		return nil, fmt.Errorf("transaction end %d does not match %d events from %d", transaction.End, len(transaction.Events), transaction.Start)
	}
	events := make([]runstate.Event, 0, len(transaction.Events))
	for index, encoded := range transaction.Events {
		expected := transaction.Start + runstate.Cursor(index)
		if encoded.Cursor != expected {
			return nil, fmt.Errorf("event cursor %d, want %d", encoded.Cursor, expected)
		}
		event, decodeErr := runstate.DecodeJournalEvent(encoded)
		if decodeErr != nil {
			return nil, decodeErr
		}
		events = append(events, event)
	}
	return events, nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return fmt.Errorf("short write")
		}
		data = data[written:]
	}
	return nil
}
