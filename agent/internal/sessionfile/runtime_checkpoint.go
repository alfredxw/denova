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
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	"github.com/alfredxw/denova/agent/session"
)

const runtimeCheckpointSidecarVersion = 1

// canonicalAnchor identifies one checksummed transaction in the canonical
// public Log. Sidecars are trusted only after the referenced transaction is
// read back and its complete runtime event is revalidated.
type canonicalAnchor struct {
	Start         int64            `json:"start"`
	Bytes         int64            `json:"bytes"`
	SHA256        string           `json:"sha256"`
	FirstRevision session.Revision `json:"first_revision"`
	LastRevision  session.Revision `json:"last_revision"`
}

type runtimeCheckpointSidecarBody struct {
	Version         int                  `json:"version"`
	KeySHA256       string               `json:"key_sha256"`
	Cursor          runstate.Cursor      `json:"cursor"`
	CanonicalOffset int64                `json:"canonical_offset"`
	Anchor          canonicalAnchor      `json:"anchor"`
	Checkpoint      json.RawMessage      `json:"checkpoint"`
	Commands        []runstate.CommandID `json:"commands,omitempty"`
}

type runtimeCheckpointSidecar struct {
	runtimeCheckpointSidecarBody
	Checksum string `json:"checksum"`
}

func (log *logFile) runtimeCheckpointPath() string {
	return strings.TrimSuffix(log.path, ".jsonl") + ".runtime-checkpoint.json"
}

// ReplayRuntimeCheckpoint restores an opaque runtime reducer snapshot and
// streams only the canonical records appended after it. A missing, stale, or
// corrupt sidecar always falls back to a complete canonical replay.
func (log *logFile) ReplayRuntimeCheckpoint(
	ctx context.Context,
	target runstate.JournalCheckpointState,
) (runstate.JournalReplayStats, error) {
	if target == nil {
		return runstate.JournalReplayStats{}, errors.New("replay Agent Session checkpoint: reducer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return runstate.JournalReplayStats{}, session.ErrLogClosed
	}
	checkpoint, snapshotBytes, checkpointErr := log.loadRuntimeCheckpointLocked()
	if checkpointErr == nil {
		candidate := target.Fresh()
		commands, commandsErr := runtimeCommandSet(checkpoint.Commands)
		if restoreErr := candidate.RestoreCheckpoint(checkpoint.Checkpoint); restoreErr == nil && commandsErr == nil &&
			candidate.Cursor() == checkpoint.Cursor {
			stats, replayErr := log.replayRuntimeFromLocked(ctx, checkpoint.CanonicalOffset, checkpoint.Cursor, candidate, commands)
			stats.SnapshotBytesRead = snapshotBytes
			stats.BytesRead += snapshotBytes
			stats.SnapshotGeneration = uint64(checkpoint.Cursor)
			if replayErr == nil {
				if stats.RecordsRead == 0 {
					log.lastTransactionStart = checkpoint.Anchor.Start
					log.lastTransactionBytes = checkpoint.Anchor.Bytes
					log.lastTransactionHash = checkpoint.Anchor.SHA256
					log.lastTransactionFirst = checkpoint.Anchor.FirstRevision
				}
				log.publishRuntimeCommandIndexBestEffort(commands)
				if err := target.PublishFrom(candidate); err != nil {
					return stats, fmt.Errorf("publish Agent Session checkpoint: %w", err)
				}
				return stats, nil
			}
		}
	}

	// Never publish partially restored state. Rebuild into a fresh reducer and
	// replace the broken acceleration file only after canonical replay succeeds.
	candidate := target.Fresh()
	commands := make(map[runstate.CommandID]struct{})
	stats, err := log.replayRuntimeFromLocked(ctx, 0, 0, candidate, commands)
	if err != nil {
		return stats, err
	}
	log.publishRuntimeCommandIndexBestEffort(commands)
	if err := target.PublishFrom(candidate); err != nil {
		return stats, fmt.Errorf("publish full Agent Session replay: %w", err)
	}
	if candidate.Cursor() > 0 && candidate.CheckpointSafe() {
		_ = log.writeRuntimeCheckpointLocked(candidate)
	}
	return stats, nil
}

func (log *logFile) MaybeRuntimeCheckpoint(ctx context.Context, state runstate.JournalCheckpointState) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return session.ErrLogClosed
	}
	if state == nil || !state.CheckpointSafe() || !log.checkpointThresholdReached() {
		return nil
	}
	if runstate.Cursor(log.revision) != state.Cursor() {
		return fmt.Errorf("Agent Session checkpoint cursor %d does not match canonical revision %d", state.Cursor(), log.revision)
	}
	return log.writeRuntimeCheckpointLocked(state)
}

func (log *logFile) checkpointThresholdReached() bool {
	options := log.options.normalized()
	return log.tailBytes >= options.CheckpointTailBytes || log.tailRecords >= options.CheckpointTailRecords
}

func (log *logFile) writeRuntimeCheckpointLocked(state runstate.JournalCheckpointState) error {
	if state == nil || state.Cursor() == 0 || log.lastTransactionBytes <= 0 ||
		runstate.Cursor(log.revision) != state.Cursor() {
		return errors.New("Agent Session checkpoint has no exact canonical head")
	}
	if !log.runtimeIndexReady {
		return errors.New("Agent Session checkpoint has no complete command index")
	}
	checkpoint, err := state.MarshalCheckpoint()
	if err != nil {
		return err
	}
	body := runtimeCheckpointSidecarBody{
		Version: runtimeCheckpointSidecarVersion, KeySHA256: log.keySHA256,
		Cursor: state.Cursor(), CanonicalOffset: log.canonicalBytes,
		Anchor: canonicalAnchor{
			Start: log.lastTransactionStart, Bytes: log.lastTransactionBytes,
			SHA256: log.lastTransactionHash, FirstRevision: log.lastTransactionFirst, LastRevision: log.revision,
		},
		Checkpoint: checkpoint, Commands: sortedRuntimeCommands(log.runtimeCommands),
	}
	if _, err := log.readAnchoredTransactionLocked(body.Anchor); err != nil {
		return err
	}
	encoded, err := encodeRuntimeCheckpointSidecar(body)
	if err != nil {
		return err
	}
	if err := commitAtomicFile(log.runtimeCheckpointPath(), append(encoded, '\n')); err != nil {
		return fmt.Errorf("commit Agent Session runtime checkpoint: %w", err)
	}
	log.tailBytes, log.tailRecords = 0, 0
	return nil
}

func (log *logFile) loadRuntimeCheckpointLocked() (runtimeCheckpointSidecarBody, int64, error) {
	encoded, err := os.ReadFile(log.runtimeCheckpointPath())
	if err != nil {
		return runtimeCheckpointSidecarBody{}, 0, err
	}
	var sidecar runtimeCheckpointSidecar
	if err := json.Unmarshal(encoded, &sidecar); err != nil {
		return runtimeCheckpointSidecarBody{}, int64(len(encoded)), err
	}
	bodyJSON, err := json.Marshal(sidecar.runtimeCheckpointSidecarBody)
	if err != nil {
		return runtimeCheckpointSidecarBody{}, int64(len(encoded)), err
	}
	digest := sha256.Sum256(bodyJSON)
	body := sidecar.runtimeCheckpointSidecarBody
	if body.Version != runtimeCheckpointSidecarVersion || body.KeySHA256 != log.keySHA256 ||
		body.Cursor == 0 || body.CanonicalOffset <= 0 || len(body.Checkpoint) == 0 ||
		sidecar.Checksum != hex.EncodeToString(digest[:]) {
		return runtimeCheckpointSidecarBody{}, int64(len(encoded)), errors.New("Agent Session runtime checkpoint identity or checksum is invalid")
	}
	if _, err := runtimeCommandSet(body.Commands); err != nil {
		return runtimeCheckpointSidecarBody{}, int64(len(encoded)), err
	}
	records, err := log.readAnchoredTransactionLocked(body.Anchor)
	if err != nil {
		return runtimeCheckpointSidecarBody{}, int64(len(encoded)), err
	}
	if body.Anchor.Start+body.Anchor.Bytes != body.CanonicalOffset ||
		body.Anchor.FirstRevision != records[0].Revision ||
		body.Anchor.LastRevision != records[len(records)-1].Revision ||
		runstate.Cursor(body.Anchor.LastRevision) != body.Cursor {
		return runtimeCheckpointSidecarBody{}, int64(len(encoded)), errors.New("Agent Session runtime checkpoint canonical anchor is stale")
	}
	return body, int64(len(encoded)), nil
}

func encodeRuntimeCheckpointSidecar(body runtimeCheckpointSidecarBody) ([]byte, error) {
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bodyJSON)
	return json.Marshal(runtimeCheckpointSidecar{
		runtimeCheckpointSidecarBody: body, Checksum: hex.EncodeToString(digest[:]),
	})
}

func (log *logFile) replayRuntimeFromLocked(
	ctx context.Context,
	offset int64,
	cursor runstate.Cursor,
	target runstate.JournalCheckpointState,
	commands map[runstate.CommandID]struct{},
) (runstate.JournalReplayStats, error) {
	file, err := os.Open(log.path)
	if errors.Is(err, os.ErrNotExist) && offset == 0 && cursor == 0 {
		log.initialized, log.revision, log.needsNewline = true, 0, false
		log.canonicalBytes, log.tailBytes, log.tailRecords = 0, 0, 0
		return runstate.JournalReplayStats{}, nil
	}
	if err != nil {
		return runstate.JournalReplayStats{}, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return runstate.JournalReplayStats{}, err
	}
	if offset < 0 || offset > info.Size() {
		_ = file.Close()
		return runstate.JournalReplayStats{}, errors.New("Agent Session checkpoint offset is outside canonical Log")
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		return runstate.JournalReplayStats{}, err
	}
	reader := bufio.NewReaderSize(file, replayBufferBytes)
	stats := runstate.JournalReplayStats{}
	revision := session.Revision(cursor)
	validBytes := offset
	lineCount := int64(0)
	var lastStart, lastBytes int64
	var lastHash string
	var lastFirst session.Revision
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return stats, err
		}
		line, readErr := reader.ReadBytes('\n')
		stats.BytesRead += int64(len(line))
		stats.TailBytesRead += int64(len(line))
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		hasNewline := len(line) > 0 && line[len(line)-1] == '\n'
		payload := bytes.TrimSuffix(line, []byte{'\n'})
		records, decodeErr := decodeTransaction(payload, revision)
		if decodeErr != nil {
			finalTorn := errors.Is(readErr, io.EOF) && !hasNewline && syntacticallyTornJSON(payload, decodeErr)
			if finalTorn {
				if closeErr := file.Close(); closeErr != nil {
					return stats, closeErr
				}
				if err := backupAndTruncate(log.path, validBytes); err != nil {
					return stats, err
				}
				file = nil
				stats.BytesRead -= int64(len(line))
				stats.TailBytesRead -= int64(len(line))
				break
			}
			_ = file.Close()
			return stats, fmt.Errorf("decode Agent Session runtime tail at revision %d: %w", revision+1, decodeErr)
		}
		anchor := canonicalAnchor{
			Start: validBytes, Bytes: int64(len(line)),
			FirstRevision: records[0].Revision, LastRevision: records[len(records)-1].Revision,
		}
		digest := sha256.Sum256(line)
		anchor.SHA256 = hex.EncodeToString(digest[:])
		for _, record := range records {
			event, err := decodeRuntimeRecord(record)
			if err != nil {
				_ = file.Close()
				return stats, err
			}
			if err := target.Reduce(event); err != nil {
				_ = file.Close()
				return stats, fmt.Errorf("reduce Agent Session runtime cursor %d: %w", event.Cursor, err)
			}
			stats.EventsRead++
		}
		log.persistRuntimeCommandsBestEffort(records, anchor)
		indexRuntimeCommands(commands, records)
		stats.RecordsRead++
		revision = records[len(records)-1].Revision
		lastStart, lastBytes, lastHash, lastFirst = anchor.Start, anchor.Bytes, anchor.SHA256, anchor.FirstRevision
		validBytes += int64(len(line))
		lineCount++
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return stats, readErr
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if file != nil {
		if err := file.Close(); err != nil {
			return stats, err
		}
	}
	log.initialized, log.revision, log.needsNewline = true, revision, false
	log.canonicalBytes = validBytes
	if lineCount > 0 {
		log.lastTransactionStart, log.lastTransactionBytes, log.lastTransactionHash = lastStart, lastBytes, lastHash
		log.lastTransactionFirst = lastFirst
	} else if offset == 0 {
		log.lastTransactionStart, log.lastTransactionBytes, log.lastTransactionHash = 0, 0, ""
		log.lastTransactionFirst = 0
	}
	log.tailBytes, log.tailRecords = validBytes-offset, lineCount
	return stats, nil
}

func decodeRuntimeRecord(record session.Record) (runstate.Event, error) {
	if record.Kind != runtimeRecordKind || record.Version != runtimeRecordV1 {
		return runstate.Event{}, fmt.Errorf("unsupported Agent Session runtime record %q version %d", record.Kind, record.Version)
	}
	event, err := runstate.UnmarshalJournalEvent(record.Data)
	if err != nil {
		return runstate.Event{}, err
	}
	if runstate.Cursor(record.Revision) != event.Cursor {
		return runstate.Event{}, fmt.Errorf("Agent Session runtime record revision %d does not match cursor %d", record.Revision, event.Cursor)
	}
	return event, nil
}

func (log *logFile) readAnchoredTransactionLocked(anchor canonicalAnchor) ([]session.Record, error) {
	if anchor.Start < 0 || anchor.Bytes <= 0 || len(anchor.SHA256) != sha256.Size*2 ||
		anchor.FirstRevision == 0 || anchor.LastRevision < anchor.FirstRevision {
		return nil, errors.New("Agent Session sidecar canonical anchor is invalid")
	}
	file, err := os.Open(log.path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if anchor.Start > info.Size() || anchor.Bytes > info.Size()-anchor.Start {
		_ = file.Close()
		return nil, errors.New("Agent Session sidecar canonical anchor is outside the Log")
	}
	encoded := make([]byte, anchor.Bytes)
	_, readErr := file.ReadAt(encoded, anchor.Start)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	digest := sha256.Sum256(encoded)
	if hex.EncodeToString(digest[:]) != anchor.SHA256 || encoded[len(encoded)-1] != '\n' {
		return nil, errors.New("Agent Session sidecar canonical anchor hash is stale")
	}
	records, err := decodeTransaction(encoded[:len(encoded)-1], anchor.FirstRevision-1)
	if err != nil || records[len(records)-1].Revision != anchor.LastRevision {
		return nil, errors.Join(errors.New("Agent Session sidecar canonical transaction is invalid"), err)
	}
	return records, nil
}
