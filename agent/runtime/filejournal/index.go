package filejournal

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	runstate "github.com/alfredxw/denova/agent/runtime"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const journalCommandIndexVersion = 1

type journalCommandIndexEntry struct {
	CommandID   runstate.CommandID   `json:"command_id"`
	OperationID runstate.OperationID `json:"operation_id"`
	Cursor      runstate.Cursor      `json:"cursor"`
	Fingerprint string               `json:"fingerprint"`
}

// journalCommandIndexBody is an append-only index checkpoint. A compact
// snapshot is written after canonical replay; subsequent journal appends add
// only one small checksummed delta instead of rewriting every historical
// command receipt (which would make the index itself O(N²)).
type journalCommandIndexBody struct {
	Version               int                        `json:"version"`
	Snapshot              bool                       `json:"snapshot,omitempty"`
	PreviousJournalCursor runstate.Cursor            `json:"previous_journal_cursor"`
	JournalBytes          int64                      `json:"journal_bytes"`
	JournalModifiedNS     int64                      `json:"journal_modified_ns"`
	JournalCursor         runstate.Cursor            `json:"journal_cursor"`
	TailRecordSHA256      string                     `json:"tail_record_sha256,omitempty"`
	Commands              []journalCommandIndexEntry `json:"commands,omitempty"`
}

type journalCommandIndex struct {
	journalCommandIndexBody
	Checksum string `json:"checksum"`
}

func (j *journal) commandIndexPath() string {
	return j.path + ".commands.jsonl"
}

func (j *journal) indexCommittedCommands(events []runstate.Event) {
	if j.commandIndex == nil {
		j.commandIndex = make(map[runstate.CommandID]runstate.CommandRecord)
	}
	for _, event := range events {
		accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
		if !ok {
			continue
		}
		j.commandIndex[accepted.CommandID] = runstate.CommandRecord{
			Receipt: runstate.Receipt{
				CommandID: accepted.CommandID, OperationID: accepted.OperationID, Cursor: event.Cursor,
			},
			Fingerprint: accepted.Fingerprint,
		}
	}
	j.indexReady = true
}

func (j *journal) rewriteCommandIndexLocked() error {
	info, err := os.Stat(j.tailPath)
	if errors.Is(err, os.ErrNotExist) {
		if j.cursor != 0 {
			return fmt.Errorf("journal disappeared at cursor %d", j.cursor)
		}
		_ = os.Remove(j.commandIndexPath())
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat command index journal: %w", err)
	}
	body := journalCommandIndexBody{
		Version: journalCommandIndexVersion, Snapshot: true,
		JournalBytes: info.Size(), JournalModifiedNS: info.ModTime().UnixNano(),
		JournalCursor: j.cursor, TailRecordSHA256: j.lastTailHash,
		Commands: commandIndexEntries(j.commandIndex),
	}
	line, err := encodeCommandIndexRecord(body)
	if err != nil {
		return err
	}
	return writeRebuildableFile(j.commandIndexPath(), append(line, '\n'), 0o600)
}

func (j *journal) appendCommandIndexLocked(previous runstate.Cursor, events []runstate.Event) error {
	info, err := os.Stat(j.tailPath)
	if err != nil {
		return fmt.Errorf("stat command index journal: %w", err)
	}
	entries := commandIndexEntriesFromEvents(events)
	body := journalCommandIndexBody{
		Version:               journalCommandIndexVersion,
		PreviousJournalCursor: previous,
		JournalBytes:          info.Size(), JournalModifiedNS: info.ModTime().UnixNano(),
		JournalCursor: j.cursor, TailRecordSHA256: j.lastTailHash, Commands: entries,
	}
	line, err := encodeCommandIndexRecord(body)
	if err != nil {
		return err
	}
	// A missing snapshot means an earlier best-effort index update failed. One
	// compact rebuild restores the chain; the canonical journal is untouched.
	if _, err := os.Stat(j.commandIndexPath()); errors.Is(err, os.ErrNotExist) {
		return j.rewriteCommandIndexLocked()
	} else if err != nil {
		return err
	}
	file, err := os.OpenFile(j.commandIndexPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	writeErr := writeAll(file, append(line, '\n'))
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	// This index is checksummed and rebuilt from the canonical fsynced journal
	// after any missing or torn write. Do not add a second durability barrier to
	// every user-visible runtime event.
	return nil
}

func commandIndexEntries(commands map[runstate.CommandID]runstate.CommandRecord) []journalCommandIndexEntry {
	entries := make([]journalCommandIndexEntry, 0, len(commands))
	for commandID, record := range commands {
		entries = append(entries, journalCommandIndexEntry{
			CommandID: commandID, OperationID: record.Receipt.OperationID,
			Cursor: record.Receipt.Cursor, Fingerprint: record.Fingerprint,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].CommandID < entries[right].CommandID
	})
	return entries
}

func commandIndexEntriesFromEvents(events []runstate.Event) []journalCommandIndexEntry {
	entries := make([]journalCommandIndexEntry, 0, 1)
	for _, event := range events {
		accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
		if !ok {
			continue
		}
		entries = append(entries, journalCommandIndexEntry{
			CommandID: accepted.CommandID, OperationID: accepted.OperationID,
			Cursor: event.Cursor, Fingerprint: accepted.Fingerprint,
		})
	}
	return entries
}

func encodeCommandIndexRecord(body journalCommandIndexBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode command index checksum body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	encoded, err := json.Marshal(journalCommandIndex{
		journalCommandIndexBody: body,
		Checksum:                hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return nil, fmt.Errorf("encode command index: %w", err)
	}
	return encoded, nil
}

// loadPersistedCommandIndexLocked validates every sidecar delta, its cursor
// chain, final journal stat identity, and the raw hash of the canonical tail.
// A stale or corrupt index is never partially trusted.
func (j *journal) loadPersistedCommandIndexLocked() (bool, error) {
	file, err := os.Open(j.commandIndexPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open command index: %w", err)
	}
	reader := bufio.NewReaderSize(file, journalReadBufferBytes)
	commands := make(map[runstate.CommandID]runstate.CommandRecord)
	var last journalCommandIndexBody
	lineNumber := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		lineNumber++
		line = bytes.TrimSuffix(line, []byte{'\n'})
		if len(line) == 0 {
			_ = file.Close()
			return false, fmt.Errorf("command index line %d is empty", lineNumber)
		}
		var record journalCommandIndex
		if err := json.Unmarshal(line, &record); err != nil {
			_ = file.Close()
			return false, fmt.Errorf("decode command index line %d: %w", lineNumber, err)
		}
		bodyJSON, err := json.Marshal(record.journalCommandIndexBody)
		if err != nil {
			_ = file.Close()
			return false, err
		}
		digest := sha256.Sum256(bodyJSON)
		if record.Version != journalCommandIndexVersion || record.Checksum != hex.EncodeToString(digest[:]) {
			_ = file.Close()
			return false, fmt.Errorf("command index line %d checksum or version mismatch", lineNumber)
		}
		if lineNumber == 1 {
			if !record.Snapshot || record.PreviousJournalCursor != 0 {
				_ = file.Close()
				return false, fmt.Errorf("command index does not start with a snapshot")
			}
		} else if record.Snapshot || record.PreviousJournalCursor != last.JournalCursor || record.JournalCursor <= last.JournalCursor {
			_ = file.Close()
			return false, fmt.Errorf("command index cursor chain breaks at line %d", lineNumber)
		}
		for _, entry := range record.Commands {
			if entry.CommandID == "" || entry.OperationID == "" || entry.Cursor <= 0 || entry.Cursor > record.JournalCursor {
				_ = file.Close()
				return false, fmt.Errorf("command index line %d contains an invalid receipt", lineNumber)
			}
			if existing, duplicate := commands[entry.CommandID]; duplicate &&
				(existing.Receipt.OperationID != entry.OperationID || existing.Receipt.Cursor != entry.Cursor || existing.Fingerprint != entry.Fingerprint) {
				_ = file.Close()
				return false, fmt.Errorf("command index contains conflicting command %q", entry.CommandID)
			}
			commands[entry.CommandID] = runstate.CommandRecord{
				Receipt:     runstate.Receipt{CommandID: entry.CommandID, OperationID: entry.OperationID, Cursor: entry.Cursor},
				Fingerprint: entry.Fingerprint,
			}
		}
		last = record.journalCommandIndexBody
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return false, readErr
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	if lineNumber == 0 {
		return false, fmt.Errorf("command index is empty")
	}
	info, err := os.Stat(j.tailPath)
	if err != nil {
		return false, fmt.Errorf("stat indexed journal: %w", err)
	}
	if info.Size() != last.JournalBytes || info.ModTime().UnixNano() != last.JournalModifiedNS {
		return false, fmt.Errorf("command index journal identity is stale")
	}
	if info.Size() == 0 {
		if last.JournalCursor != 0 || last.TailRecordSHA256 != "" || len(commands) != 0 {
			return false, fmt.Errorf("empty journal has a non-empty command index")
		}
	} else {
		tail, err := readLastJournalRecord(j.tailPath)
		if err != nil {
			return false, err
		}
		if journalRecordHash(tail) != last.TailRecordSHA256 {
			return false, fmt.Errorf("command index tail identity mismatch")
		}
	}
	j.commandIndex = commands
	j.indexReady = true
	return true, nil
}

// readLastJournalRecord seeks backwards in bounded blocks. It allocates only
// the final transaction rather than scanning or retaining the full journal.
func readLastJournalRecord(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open indexed journal tail: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat indexed journal tail: %w", err)
	}
	end := info.Size()
	if end == 0 {
		return nil, nil
	}
	last := make([]byte, 1)
	if _, err := file.ReadAt(last, end-1); err != nil {
		return nil, fmt.Errorf("read indexed journal tail: %w", err)
	}
	if last[0] == '\n' {
		end--
	}
	if end == 0 {
		return nil, fmt.Errorf("indexed journal contains no record")
	}
	const blockBytes int64 = 64 * 1024
	blocks := make([][]byte, 0, 1)
	position := end
	start := int64(0)
	for position > 0 {
		readStart := position - blockBytes
		if readStart < 0 {
			readStart = 0
		}
		block := make([]byte, position-readStart)
		if _, err := file.ReadAt(block, readStart); err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("read indexed journal tail block: %w", err)
		}
		if newline := bytes.LastIndexByte(block, '\n'); newline >= 0 {
			start = readStart + int64(newline) + 1
			block = block[newline+1:]
			blocks = append(blocks, block)
			break
		}
		blocks = append(blocks, block)
		position = readStart
	}
	record := make([]byte, 0, end-start)
	for index := len(blocks) - 1; index >= 0; index-- {
		record = append(record, blocks[index]...)
	}
	return record, nil
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if err := writeAll(file, data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	cleanup = false
	return syncDirectory(directory)
}

func writeRebuildableFile(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if err := writeAll(file, data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
