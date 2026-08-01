package filejournal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"os"
	"path/filepath"
)

const journalCommandRecordVersion = 1

type journalCommandRecordBody struct {
	Version     int                  `json:"version"`
	CommandID   runstate.CommandID   `json:"command_id"`
	OperationID runstate.OperationID `json:"operation_id"`
	Cursor      runstate.Cursor      `json:"cursor"`
	Fingerprint string               `json:"fingerprint"`
}

type journalCommandRecord struct {
	journalCommandRecordBody
	Checksum string `json:"checksum"`
}

func (j *journal) commandRecordPath(commandID runstate.CommandID) string {
	digest := sha256.Sum256([]byte(commandID))
	hexDigest := hex.EncodeToString(digest[:])
	return filepath.Join(j.path+".command-records", hexDigest[:2], hexDigest[2:]+".json")
}

func encodeCommandRecord(record runstate.CommandRecord) ([]byte, error) {
	body := journalCommandRecordBody{
		Version: journalCommandRecordVersion, CommandID: record.Receipt.CommandID,
		OperationID: record.Receipt.OperationID, Cursor: record.Receipt.Cursor, Fingerprint: record.Fingerprint,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(journalCommandRecord{
		journalCommandRecordBody: body,
		Checksum:                 hex.EncodeToString(digest[:]),
	})
}

func decodeCommandRecord(encoded []byte, commandID runstate.CommandID) (runstate.CommandRecord, error) {
	var persisted journalCommandRecord
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		return runstate.CommandRecord{}, err
	}
	bodyJSON, err := json.Marshal(persisted.journalCommandRecordBody)
	if err != nil {
		return runstate.CommandRecord{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if persisted.Version != journalCommandRecordVersion || persisted.CommandID != commandID ||
		persisted.OperationID == "" || persisted.Cursor == 0 || persisted.Checksum != hex.EncodeToString(digest[:]) {
		return runstate.CommandRecord{}, fmt.Errorf("command record checksum, version, or identity mismatch")
	}
	return runstate.CommandRecord{
		Receipt:     runstate.Receipt{CommandID: persisted.CommandID, OperationID: persisted.OperationID, Cursor: persisted.Cursor},
		Fingerprint: persisted.Fingerprint,
	}, nil
}

func (j *journal) persistCommandRecordLocked(record runstate.CommandRecord) error {
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

func (j *journal) persistCommandRecordsLocked(commands map[runstate.CommandID]runstate.CommandRecord) error {
	if j.pendingCommandRecords == nil {
		j.pendingCommandRecords = make(map[runstate.CommandID]runstate.CommandRecord)
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

func (j *journal) readCommandRecordLocked(commandID runstate.CommandID) (runstate.CommandRecord, bool, error) {
	encoded, err := os.ReadFile(j.commandRecordPath(commandID))
	if errors.Is(err, os.ErrNotExist) {
		return runstate.CommandRecord{}, false, nil
	}
	if err != nil {
		return runstate.CommandRecord{}, false, err
	}
	record, err := decodeCommandRecord(encoded, commandID)
	if err != nil {
		return runstate.CommandRecord{}, false, err
	}
	return record, true, nil
}

func (j *journal) lookupTailCommandLocked(ctx context.Context, commandID runstate.CommandID) (runstate.CommandRecord, bool, error) {
	if !j.initialized {
		state := runstate.NewJournalProjectionState(j.binding)
		if _, err := j.replayCheckpointLocked(ctx, state); err != nil {
			return runstate.CommandRecord{}, false, err
		}
	}
	record, found := j.commandIndex[commandID]
	return record, found, nil
}
