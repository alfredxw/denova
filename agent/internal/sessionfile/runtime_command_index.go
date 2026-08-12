package sessionfile

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	"github.com/alfredxw/denova/agent/session"
)

const (
	runtimeCommandSidecarVersion  = 1
	runtimeCommandIndexVersion    = 1
	maxRuntimeCommandSidecarBytes = 1 << 20
)

type runtimeCommandSidecarBody struct {
	Version     int                  `json:"version"`
	KeySHA256   string               `json:"key_sha256"`
	CommandID   runstate.CommandID   `json:"command_id"`
	OperationID runstate.OperationID `json:"operation_id"`
	Cursor      runstate.Cursor      `json:"cursor"`
	Fingerprint string               `json:"fingerprint"`
	Anchor      canonicalAnchor      `json:"anchor"`
}

type runtimeCommandSidecar struct {
	runtimeCommandSidecarBody
	Checksum string `json:"checksum"`
}

// runtimeCommandIndexBody is a compact snapshot followed by small deltas. It
// proves that the receipt directory covers the exact canonical head, so a
// missing receipt can mean "not accepted" without scanning Session history.
// Every receipt that is found is still validated against its canonical
// transaction; neither sidecar is a durable authority.
type runtimeCommandIndexBody struct {
	Version           int                  `json:"version"`
	KeySHA256         string               `json:"key_sha256"`
	Snapshot          bool                 `json:"snapshot,omitempty"`
	PreviousOffset    int64                `json:"previous_offset"`
	PreviousRevision  session.Revision     `json:"previous_revision"`
	CanonicalOffset   int64                `json:"canonical_offset"`
	CanonicalRevision session.Revision     `json:"canonical_revision"`
	Anchor            canonicalAnchor      `json:"anchor"`
	Commands          []runstate.CommandID `json:"commands,omitempty"`
}

type runtimeCommandIndexRecord struct {
	runtimeCommandIndexBody
	Checksum string `json:"checksum"`
}

func (log *logFile) runtimeCommandPath(commandID runstate.CommandID) string {
	digest := sha256.Sum256([]byte(commandID))
	encoded := hex.EncodeToString(digest[:])
	return filepath.Join(strings.TrimSuffix(log.path, ".jsonl")+".runtime-commands", encoded[:2], encoded[2:]+".json")
}

func (log *logFile) runtimeCommandIndexPath() string {
	return strings.TrimSuffix(log.path, ".jsonl") + ".runtime-command-index.jsonl"
}

func (log *logFile) LookupRuntimeCommand(
	ctx context.Context,
	commandID runstate.CommandID,
) (runstate.CommandRecord, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return runstate.CommandRecord{}, false, session.ErrLogClosed
	}
	path := log.runtimeCommandPath(commandID)
	record, found, receiptErr := log.readRuntimeCommandSidecarLocked(path, commandID)
	if receiptErr == nil && found {
		return record, true, nil
	}
	if err := log.ensureRuntimeCommandIndexLocked(ctx); err != nil {
		return runstate.CommandRecord{}, false, err
	}
	if _, accepted := log.runtimeCommands[commandID]; !accepted {
		// A valid coverage index makes absence authoritative without making the
		// index itself authoritative for a positive receipt.
		if receiptErr != nil && !errors.Is(receiptErr, os.ErrNotExist) {
			_ = os.Remove(path)
		}
		return runstate.CommandRecord{}, false, nil
	}

	// The coverage index says this command exists, but its independently
	// anchored receipt is missing or invalid. Rebuild all acceleration from the
	// canonical Log once, then validate the exact receipt again.
	_ = os.Remove(path)
	if _, err := log.replayLocked(ctx, true, nil); err != nil {
		return runstate.CommandRecord{}, false, err
	}
	record, found, err := log.readRuntimeCommandSidecarLocked(path, commandID)
	if errors.Is(err, os.ErrNotExist) {
		return runstate.CommandRecord{}, false, errors.New("Agent Session command index conflicts with canonical Log")
	}
	return record, found, err
}

func (log *logFile) ensureRuntimeCommandIndexLocked(ctx context.Context) error {
	// The Session lease excludes external appends. Once loaded, this complete
	// process view remains current until this log appends (which advances it) or
	// an ambiguous canonical commit explicitly invalidates it.
	if log.runtimeIndexReady {
		return nil
	}
	commands, offset, revision, err := log.loadRuntimeCommandIndexLocked(ctx)
	if err == nil {
		log.setRuntimeCommandIndex(commands, offset, revision, true)
		return nil
	}
	// Missing, torn, corrupt, or stale acceleration is a cache miss. A complete
	// canonical replay repairs both the receipt directory and coverage index.
	_, replayErr := log.replayLocked(ctx, true, nil)
	return replayErr
}

func (log *logFile) loadRuntimeCommandIndexLocked(ctx context.Context) (map[runstate.CommandID]struct{}, int64, session.Revision, error) {
	log.runtimeIndexLoadCount++
	canonicalInfo, err := os.Stat(log.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[runstate.CommandID]struct{}{}, 0, 0, nil
	}
	if err != nil {
		return nil, 0, 0, err
	}
	if canonicalInfo.Size() == 0 {
		return map[runstate.CommandID]struct{}{}, 0, 0, nil
	}
	file, err := os.Open(log.runtimeCommandIndexPath())
	if err != nil {
		return nil, 0, 0, err
	}
	reader := bufio.NewReaderSize(file, replayBufferBytes)
	commands := make(map[runstate.CommandID]struct{})
	var previous runtimeCommandIndexBody
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, 0, 0, err
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		lineNumber++
		if len(line) == 0 || line[len(line)-1] != '\n' {
			_ = file.Close()
			return nil, 0, 0, fmt.Errorf("Agent Session command index line %d is torn", lineNumber)
		}
		record, decodeErr := decodeRuntimeCommandIndexRecord(bytes.TrimSuffix(line, []byte{'\n'}))
		if decodeErr != nil {
			_ = file.Close()
			return nil, 0, 0, fmt.Errorf("decode Agent Session command index line %d: %w", lineNumber, decodeErr)
		}
		body := record.runtimeCommandIndexBody
		if body.KeySHA256 != log.keySHA256 || body.CanonicalOffset <= 0 || body.CanonicalRevision == 0 ||
			body.Anchor.Start+body.Anchor.Bytes != body.CanonicalOffset || body.Anchor.LastRevision != body.CanonicalRevision {
			_ = file.Close()
			return nil, 0, 0, fmt.Errorf("Agent Session command index line %d has invalid canonical identity", lineNumber)
		}
		if lineNumber == 1 {
			if !body.Snapshot || body.PreviousOffset != 0 || body.PreviousRevision != 0 {
				_ = file.Close()
				return nil, 0, 0, errors.New("Agent Session command index does not start with a snapshot")
			}
		} else if body.Snapshot || body.PreviousOffset != previous.CanonicalOffset ||
			body.PreviousRevision != previous.CanonicalRevision || body.CanonicalOffset <= previous.CanonicalOffset ||
			body.CanonicalRevision <= previous.CanonicalRevision {
			_ = file.Close()
			return nil, 0, 0, fmt.Errorf("Agent Session command index chain breaks at line %d", lineNumber)
		}
		for _, commandID := range body.Commands {
			if commandID == "" {
				_ = file.Close()
				return nil, 0, 0, fmt.Errorf("Agent Session command index line %d contains an empty command", lineNumber)
			}
			commands[commandID] = struct{}{}
		}
		previous = body
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return nil, 0, 0, readErr
		}
	}
	if closeErr := file.Close(); closeErr != nil {
		return nil, 0, 0, closeErr
	}
	if lineNumber == 0 || previous.CanonicalOffset != canonicalInfo.Size() {
		return nil, 0, 0, errors.New("Agent Session command index does not cover the canonical head")
	}
	records, err := log.readAnchoredTransactionLocked(previous.Anchor)
	if err != nil || records[len(records)-1].Revision != previous.CanonicalRevision {
		return nil, 0, 0, errors.Join(errors.New("Agent Session command index head is stale"), err)
	}
	return commands, previous.CanonicalOffset, previous.CanonicalRevision, nil
}

func (log *logFile) setRuntimeCommandIndex(
	commands map[runstate.CommandID]struct{},
	offset int64,
	revision session.Revision,
	persisted bool,
) {
	if commands == nil {
		commands = make(map[runstate.CommandID]struct{})
	}
	log.runtimeCommands = commands
	log.runtimeIndexOffset = offset
	log.runtimeIndexRevision = revision
	log.runtimeIndexReady = true
	log.runtimeIndexPersisted = persisted
}

func (log *logFile) publishRuntimeCommandIndexBestEffort(commands map[runstate.CommandID]struct{}) {
	log.setRuntimeCommandIndex(commands, log.canonicalBytes, log.revision, false)
	if log.revision == 0 {
		return
	}
	if err := log.rewriteRuntimeCommandIndexLocked(); err == nil {
		log.runtimeIndexPersisted = true
	}
}

func sortedRuntimeCommands(commands map[runstate.CommandID]struct{}) []runstate.CommandID {
	result := make([]runstate.CommandID, 0, len(commands))
	for commandID := range commands {
		result = append(result, commandID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func runtimeCommandSet(commandIDs []runstate.CommandID) (map[runstate.CommandID]struct{}, error) {
	commands := make(map[runstate.CommandID]struct{}, len(commandIDs))
	for _, commandID := range commandIDs {
		if commandID == "" {
			return nil, errors.New("Agent Session command index contains an empty command")
		}
		if _, duplicate := commands[commandID]; duplicate {
			return nil, errors.New("Agent Session command index contains a duplicate command")
		}
		commands[commandID] = struct{}{}
	}
	return commands, nil
}

func indexRuntimeCommands(commands map[runstate.CommandID]struct{}, records []session.Record) []runstate.CommandID {
	acceptedIDs := make([]runstate.CommandID, 0, 1)
	for _, record := range records {
		event, err := decodeRuntimeRecord(record)
		if err != nil {
			continue
		}
		accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
		if !ok {
			continue
		}
		commands[accepted.CommandID] = struct{}{}
		acceptedIDs = append(acceptedIDs, accepted.CommandID)
	}
	return acceptedIDs
}

func (log *logFile) advanceRuntimeCommandIndexBestEffort(
	records []session.Record,
	previousOffset int64,
	anchor canonicalAnchor,
) {
	if !log.runtimeIndexReady {
		return
	}
	commands := indexRuntimeCommands(log.runtimeCommands, records)
	previousRevision := anchor.FirstRevision - 1
	body := runtimeCommandIndexBody{
		Version: runtimeCommandIndexVersion, KeySHA256: log.keySHA256,
		PreviousOffset: previousOffset, PreviousRevision: previousRevision,
		CanonicalOffset: log.canonicalBytes, CanonicalRevision: log.revision,
		Anchor: anchor, Commands: commands,
	}
	var err error
	if log.runtimeIndexPersisted && log.runtimeIndexOffset == previousOffset &&
		log.runtimeIndexRevision == previousRevision {
		err = log.appendRuntimeCommandIndexLocked(body)
	} else {
		log.runtimeIndexOffset = log.canonicalBytes
		log.runtimeIndexRevision = log.revision
		err = log.rewriteRuntimeCommandIndexLocked()
	}
	log.runtimeIndexOffset = log.canonicalBytes
	log.runtimeIndexRevision = log.revision
	log.runtimeIndexPersisted = err == nil
}

func (log *logFile) rewriteRuntimeCommandIndexLocked() error {
	if log.revision == 0 || log.canonicalBytes == 0 {
		if err := os.Remove(log.runtimeCommandIndexPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	body := runtimeCommandIndexBody{
		Version: runtimeCommandIndexVersion, KeySHA256: log.keySHA256, Snapshot: true,
		CanonicalOffset: log.canonicalBytes, CanonicalRevision: log.revision,
		Anchor: canonicalAnchor{
			Start: log.lastTransactionStart, Bytes: log.lastTransactionBytes,
			SHA256: log.lastTransactionHash, FirstRevision: log.lastTransactionFirst, LastRevision: log.revision,
		},
		Commands: sortedRuntimeCommands(log.runtimeCommands),
	}
	if _, err := log.readAnchoredTransactionLocked(body.Anchor); err != nil {
		return err
	}
	encoded, err := encodeRuntimeCommandIndexRecord(body)
	if err != nil {
		return err
	}
	return commitAtomicFile(log.runtimeCommandIndexPath(), append(encoded, '\n'))
}

func (log *logFile) appendRuntimeCommandIndexLocked(body runtimeCommandIndexBody) error {
	encoded, err := encodeRuntimeCommandIndexRecord(body)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(log.runtimeCommandIndexPath(), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	return errors.Join(writeAll(file, append(encoded, '\n')), file.Close())
}

func encodeRuntimeCommandIndexRecord(body runtimeCommandIndexBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(runtimeCommandIndexRecord{
		runtimeCommandIndexBody: body, Checksum: hex.EncodeToString(digest[:]),
	})
}

func decodeRuntimeCommandIndexRecord(encoded []byte) (runtimeCommandIndexRecord, error) {
	var record runtimeCommandIndexRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		return runtimeCommandIndexRecord{}, err
	}
	bodyJSON, err := json.Marshal(record.runtimeCommandIndexBody)
	if err != nil {
		return runtimeCommandIndexRecord{}, err
	}
	digest := sha256.Sum256(bodyJSON)
	if record.Version != runtimeCommandIndexVersion || record.Checksum != hex.EncodeToString(digest[:]) {
		return runtimeCommandIndexRecord{}, errors.New("Agent Session command index checksum or version is invalid")
	}
	return record, nil
}

func (log *logFile) persistRuntimeCommandsBestEffort(records []session.Record, anchor canonicalAnchor) {
	for _, record := range records {
		event, err := decodeRuntimeRecord(record)
		if err != nil {
			continue
		}
		accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
		if !ok {
			continue
		}
		path := log.runtimeCommandPath(accepted.CommandID)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			continue
		}
		body := runtimeCommandSidecarBody{
			Version: runtimeCommandSidecarVersion, KeySHA256: log.keySHA256,
			CommandID: accepted.CommandID, OperationID: accepted.OperationID,
			Cursor: event.Cursor, Fingerprint: accepted.Fingerprint, Anchor: anchor,
		}
		encoded, err := encodeRuntimeCommandSidecar(body)
		if err != nil || os.MkdirAll(filepath.Dir(path), 0o700) != nil {
			continue
		}
		_ = commitAtomicFile(path, append(encoded, '\n'))
	}
}

func encodeRuntimeCommandSidecar(body runtimeCommandSidecarBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(runtimeCommandSidecar{
		runtimeCommandSidecarBody: body, Checksum: hex.EncodeToString(digest[:]),
	})
}

func (log *logFile) readRuntimeCommandSidecarLocked(
	path string,
	commandID runstate.CommandID,
) (runstate.CommandRecord, bool, error) {
	encoded, err := readSmallSidecar(path, maxRuntimeCommandSidecarBytes)
	if err != nil {
		return runstate.CommandRecord{}, false, err
	}
	var sidecar runtimeCommandSidecar
	if err := json.Unmarshal(encoded, &sidecar); err != nil {
		return runstate.CommandRecord{}, false, err
	}
	bodyJSON, err := json.Marshal(sidecar.runtimeCommandSidecarBody)
	if err != nil {
		return runstate.CommandRecord{}, false, err
	}
	digest := sha256.Sum256(bodyJSON)
	body := sidecar.runtimeCommandSidecarBody
	if body.Version != runtimeCommandSidecarVersion || body.KeySHA256 != log.keySHA256 ||
		body.CommandID != commandID || body.OperationID == "" || body.Cursor == 0 ||
		sidecar.Checksum != hex.EncodeToString(digest[:]) {
		return runstate.CommandRecord{}, false, errors.New("Agent Session command sidecar identity or checksum is invalid")
	}
	records, err := log.readAnchoredTransactionLocked(body.Anchor)
	if err != nil {
		return runstate.CommandRecord{}, false, err
	}
	for _, record := range records {
		event, err := decodeRuntimeRecord(record)
		if err != nil {
			return runstate.CommandRecord{}, false, err
		}
		accepted, ok := event.Payload.(runstate.CommandAcceptedEvent)
		if !ok || accepted.CommandID != commandID {
			continue
		}
		if event.Cursor != body.Cursor || accepted.OperationID != body.OperationID || accepted.Fingerprint != body.Fingerprint {
			return runstate.CommandRecord{}, false, errors.New("Agent Session command sidecar conflicts with canonical acceptance")
		}
		return runstate.CommandRecord{
			Receipt:     runstate.Receipt{CommandID: commandID, OperationID: body.OperationID, Cursor: body.Cursor},
			Fingerprint: body.Fingerprint,
		}, true, nil
	}
	return runstate.CommandRecord{}, false, errors.New("Agent Session command sidecar has no canonical acceptance")
}

func readSmallSidecar(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 || info.Size() > limit {
		return nil, errors.New("Agent Session sidecar size is invalid")
	}
	return os.ReadFile(path)
}
