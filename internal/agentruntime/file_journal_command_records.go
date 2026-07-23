package agentruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const fileJournalCommandRecordVersion = 1

type fileJournalCommandRecordBody struct {
	Version     int         `json:"version"`
	CommandID   CommandID   `json:"command_id"`
	OperationID OperationID `json:"operation_id"`
	Cursor      Cursor      `json:"cursor"`
	Fingerprint string      `json:"fingerprint"`
}

type fileJournalCommandRecord struct {
	fileJournalCommandRecordBody
	Checksum string `json:"checksum"`
}

func (j *fileJournal) commandRecordPath(commandID CommandID) string {
	digest := sha256.Sum256([]byte(commandID))
	hexDigest := hex.EncodeToString(digest[:])
	return filepath.Join(j.path+".command-records", hexDigest[:2], hexDigest[2:]+".json")
}

func encodeCommandRecord(record CommandRecord) ([]byte, error) {
	body := fileJournalCommandRecordBody{
		Version: fileJournalCommandRecordVersion, CommandID: record.Receipt.CommandID,
		OperationID: record.Receipt.OperationID, Cursor: record.Receipt.Cursor, Fingerprint: record.Fingerprint,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(fileJournalCommandRecord{
		fileJournalCommandRecordBody: body,
		Checksum:                     hex.EncodeToString(digest[:]),
	})
}

func decodeCommandRecord(encoded []byte, commandID CommandID) (CommandRecord, error) {
	var persisted fileJournalCommandRecord
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		return CommandRecord{}, err
	}
	bodyJSON, err := json.Marshal(persisted.fileJournalCommandRecordBody)
	if err != nil {
		return CommandRecord{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if persisted.Version != fileJournalCommandRecordVersion || persisted.CommandID != commandID ||
		persisted.OperationID == "" || persisted.Cursor == 0 || persisted.Checksum != hex.EncodeToString(digest[:]) {
		return CommandRecord{}, fmt.Errorf("command record checksum, version, or identity mismatch")
	}
	return CommandRecord{
		Receipt:     Receipt{CommandID: persisted.CommandID, OperationID: persisted.OperationID, Cursor: persisted.Cursor},
		Fingerprint: persisted.Fingerprint,
	}, nil
}

func (j *fileJournal) persistCommandRecordLocked(record CommandRecord) error {
	path := j.commandRecordPath(record.Receipt.CommandID)
	existing, err := os.ReadFile(path)
	if err == nil {
		loaded, decodeErr := decodeCommandRecord(existing, record.Receipt.CommandID)
		if decodeErr != nil {
			return fmt.Errorf("read existing command record %q: %w", record.Receipt.CommandID, decodeErr)
		}
		if loaded.Receipt.OperationID != record.Receipt.OperationID || loaded.Receipt.Cursor != record.Receipt.Cursor || loaded.Fingerprint != record.Fingerprint {
			return fmt.Errorf("command record %q conflicts with durable acceptance", record.Receipt.CommandID)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	// Persist both newly created directory entries before the record itself can
	// become the command-idempotency fence used by segment GC.
	if err := syncDirectory(filepath.Dir(j.path)); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(filepath.Dir(path))); err != nil {
		return err
	}
	encoded, err := encodeCommandRecord(record)
	if err != nil {
		return err
	}
	return writeAtomicFile(path, append(encoded, '\n'), 0o600)
}

func (j *fileJournal) persistCommandRecordsLocked(commands map[CommandID]CommandRecord) error {
	if j.pendingCommandRecords == nil {
		j.pendingCommandRecords = make(map[CommandID]CommandRecord)
	}
	for commandID, record := range commands {
		j.pendingCommandRecords[commandID] = record
	}
	for commandID, record := range j.pendingCommandRecords {
		if err := j.persistCommandRecordLocked(record); err != nil {
			return err
		}
		delete(j.pendingCommandRecords, commandID)
	}
	return nil
}

func (j *fileJournal) readCommandRecordLocked(commandID CommandID) (CommandRecord, bool, error) {
	encoded, err := os.ReadFile(j.commandRecordPath(commandID))
	if errors.Is(err, os.ErrNotExist) {
		return CommandRecord{}, false, nil
	}
	if err != nil {
		return CommandRecord{}, false, err
	}
	record, err := decodeCommandRecord(encoded, commandID)
	if err != nil {
		return CommandRecord{}, false, err
	}
	return record, true, nil
}

func (j *fileJournal) lookupTailCommandLocked(ctx context.Context, commandID CommandID) (CommandRecord, bool, error) {
	if !j.initialized {
		state := newProjectionState(j.binding)
		if _, err := j.replayHarnessStateLocked(ctx, &state); err != nil {
			return CommandRecord{}, false, err
		}
	}
	record, found := j.commandIndex[commandID]
	return record, found, nil
}
