// Package sessionfile is Agent's private durable file Store implementation.
package sessionfile

import (
	"bufio"
	"context"
	"crypto/rand"
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
	"sync"
	"sync/atomic"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	"github.com/alfredxw/denova/agent/session"
	"github.com/gofrs/flock"
)

const (
	formatVersion      = 1
	leaseRetryInterval = 10 * time.Millisecond
	replayBufferBytes  = 64 << 10
	runtimeRecordKind  = "agent.runtime.event"
	runtimeRecordV1    = 1
)

// Store keeps one checksummed append-only transaction log per exact Session
// key. A transaction contains a complete Append batch, so replay sees all of a
// batch or none of it after a crash. Runtime checkpoints and indexes are
// rebuildable sidecars of this canonical Log, never a second authority.
type Store struct {
	root    string
	options Options
}

func (store *Store) Open(ctx context.Context, key session.Key) (session.Log, error) {
	if store == nil || store.root == "" {
		return nil, errors.New("agent Session file Store is nil")
	}
	key, err := session.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	base, digest, err := store.baseForKey(key)
	if err != nil {
		return nil, err
	}
	release, err := acquireLease(ctx, base+".lease")
	if err != nil {
		return nil, err
	}
	if err := store.finishPendingDeletion(ctx, key, base); err != nil {
		_ = release()
		return nil, err
	}
	if err := ensureManifest(base+".manifest.json", key, digest); err != nil {
		_ = release()
		return nil, err
	}
	return &logFile{
		path: base + ".jsonl", keySHA256: hex.EncodeToString(digest[:]),
		options: store.options.normalized(), release: release,
	}, nil
}

func (store *Store) List(ctx context.Context, selector session.Selector) ([]session.Key, error) {
	if store == nil || store.root == "" {
		return nil, errors.New("agent Session file Store is nil")
	}
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	paths, err := filepath.Glob(filepath.Join(store.root, "*.manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("list agent Session file manifests: %w", err)
	}
	byCanonical := make(map[string]session.Key, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read agent Session file manifest %s: %w", filepath.Base(path), err)
		}
		var item manifest
		if err := json.Unmarshal(encoded, &item); err != nil {
			return nil, fmt.Errorf("decode agent Session file manifest %s: %w", filepath.Base(path), err)
		}
		key, err := session.NormalizeKey(item.Key)
		if err != nil {
			return nil, fmt.Errorf("validate agent Session file manifest %s: %w", filepath.Base(path), err)
		}
		base, digest, err := store.baseForKey(key)
		if err != nil || item.Version != formatVersion || item.KeySHA256 != hex.EncodeToString(digest[:]) ||
			filepath.Base(path) != filepath.Base(base)+".manifest.json" {
			return nil, fmt.Errorf("agent Session file manifest %s has invalid identity", filepath.Base(path))
		}
		if pending, err := deletionPending(base + ".deleting.json"); err != nil {
			return nil, err
		} else if pending {
			continue
		}
		if selector.Matches(key) {
			canonical, _ := session.CanonicalKey(key)
			byCanonical[canonical] = key
		}
	}
	result := make([]session.Key, 0, len(byCanonical))
	for _, key := range byCanonical {
		result = append(result, key)
	}
	sort.Slice(result, func(left, right int) bool {
		leftKey, _ := session.CanonicalKey(result[left])
		rightKey, _ := session.CanonicalKey(result[right])
		return leftKey < rightKey
	})
	return result, nil
}

func (store *Store) Delete(ctx context.Context, key session.Key) error {
	if store == nil || store.root == "" {
		return errors.New("agent Session file Store is nil")
	}
	key, err := session.NormalizeKey(key)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	base, _, err := store.baseForKey(key)
	if err != nil {
		return err
	}
	release, err := acquireLease(ctx, base+".lease")
	if err != nil {
		return err
	}
	defer release()
	if err := ensureDeletionMarker(base+".deleting.json", key); err != nil {
		return err
	}
	return store.finishPendingDeletion(ctx, key, base)
}

// finishPendingDeletion runs while the Session-wide lease is held. The
// durable marker makes List hide the Session and makes every generic/runtime
// Open finish the same deletion before creating a fresh generation.
func (store *Store) finishPendingDeletion(ctx context.Context, key session.Key, base string) error {
	markerPath := base + ".deleting.json"
	if _, err := os.Stat(markerPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect agent Session deletion marker: %w", err)
	}
	if err := validateDeletionMarker(markerPath, key); err != nil {
		return err
	}
	manifestPath := base + ".manifest.json"
	if encoded, readErr := os.ReadFile(manifestPath); readErr == nil {
		var got manifest
		if decodeErr := json.Unmarshal(encoded, &got); decodeErr != nil {
			return fmt.Errorf("decode deleted agent Session manifest: %w", decodeErr)
		}
		gotKey, gotErr := session.CanonicalKey(got.Key)
		wantKey, wantErr := session.CanonicalKey(key)
		if gotErr != nil || wantErr != nil || gotKey != wantKey {
			return errors.New("refuse to delete agent Session whose manifest identity does not match")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read deleted agent Session manifest: %w", readErr)
	}
	patterns := []string{
		base + ".jsonl.recovery.*.bak", manifestPath + ".tmp-*",
		base + ".runtime-checkpoint.json.tmp-*",
		base + ".runtime-command-index.jsonl.tmp-*",
	}
	paths := make([]string, 0, len(patterns)+2)
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("list agent Session deletion files: %w", err)
		}
		paths = append(paths, matches...)
	}
	paths = append(paths, base+".jsonl", base+".runtime-checkpoint.json", base+".runtime-command-index.jsonl", manifestPath)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete agent Session file %s: %w", filepath.Base(path), err)
		}
	}
	if err := os.RemoveAll(base + ".runtime-commands"); err != nil {
		return fmt.Errorf("delete agent Session command sidecars: %w", err)
	}
	if err := syncDirectory(store.root); err != nil {
		return err
	}
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove agent Session deletion marker: %w", err)
	}
	return syncDirectory(store.root)
}

func (store *Store) baseForKey(key session.Key) (string, [sha256.Size]byte, error) {
	canonical, err := session.CanonicalKey(key)
	if err != nil {
		return "", [sha256.Size]byte{}, err
	}
	digest := sha256.Sum256([]byte(canonical))
	return filepath.Join(store.root, hex.EncodeToString(digest[:])), digest, nil
}

type logFile struct {
	path      string
	keySHA256 string
	options   Options
	release   func() error

	mu                    sync.Mutex
	initialized           bool
	revision              session.Revision
	needsNewline          bool
	canonicalBytes        int64
	lastTransactionStart  int64
	lastTransactionBytes  int64
	lastTransactionHash   string
	lastTransactionFirst  session.Revision
	tailBytes             int64
	tailRecords           int64
	runtimeCommands       map[runstate.CommandID]struct{}
	runtimeIndexOffset    int64
	runtimeIndexRevision  session.Revision
	runtimeIndexReady     bool
	runtimeIndexPersisted bool
	runtimeIndexLoadCount int64
	fullReplayCount       int64
	closed                bool
	closeOnce             sync.Once
	closeErr              error
}

func (log *logFile) Replay(ctx context.Context, apply func(session.Record) error) (session.ReplayStats, error) {
	if apply == nil {
		return session.ReplayStats{}, errors.New("replay agent Session file Log: reducer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return session.ReplayStats{}, session.ErrLogClosed
	}
	return log.replayLocked(ctx, true, apply)
}

func (log *logFile) Append(ctx context.Context, expected session.Revision, records ...session.Record) (session.Revision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	for _, record := range records {
		if err := session.ValidateRecord(record); err != nil {
			return 0, err
		}
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return 0, session.ErrLogClosed
	}
	if !log.initialized {
		if _, err := log.replayLocked(ctx, true, nil); err != nil {
			return 0, err
		}
	}
	if log.revision != expected {
		return log.revision, &session.RevisionConflictError{Expected: expected, Actual: log.revision}
	}
	if len(records) == 0 {
		return log.revision, nil
	}
	if err := ctx.Err(); err != nil {
		return log.revision, err
	}

	committed := make([]session.Record, len(records))
	for index, record := range records {
		record.Data = append(json.RawMessage(nil), record.Data...)
		record.Revision = expected + session.Revision(index) + 1
		committed[index] = record
	}
	body := transactionBody{
		Version: formatVersion, Start: committed[0].Revision,
		End: committed[len(committed)-1].Revision, Records: committed,
	}
	encodedBody, err := json.Marshal(body)
	if err != nil {
		return log.revision, fmt.Errorf("encode agent Session file transaction: %w", err)
	}
	digest := sha256.Sum256(encodedBody)
	encoded, err := json.Marshal(transaction{
		Version: body.Version, Start: body.Start, End: body.End, Records: body.Records,
		Checksum: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return log.revision, fmt.Errorf("encode agent Session file transaction: %w", err)
	}
	data := make([]byte, 0, len(encoded)+2)
	previousCanonicalBytes := log.canonicalBytes
	transactionStart := log.canonicalBytes
	if log.needsNewline {
		data = append(data, '\n')
		transactionStart++
	}
	data = append(data, encoded...)
	data = append(data, '\n')
	transactionBytes := int64(len(encoded) + 1)
	transactionDigest := sha256.Sum256(data[len(data)-int(transactionBytes):])
	_, statErr := os.Stat(log.path)
	created := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !created {
		return log.revision, fmt.Errorf("stat agent Session file Log: %w", statErr)
	}
	file, err := os.OpenFile(log.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return log.revision, fmt.Errorf("open agent Session file Log: %w", err)
	}
	writeErr := writeAll(file, data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		log.initialized = false
		log.runtimeIndexReady = false
		log.runtimeIndexPersisted = false
		return log.revision, errors.Join(
			session.ErrCommitUnknown,
			wrapIO("write agent Session file Log", writeErr),
			wrapIO("sync agent Session file Log", syncErr),
			wrapIO("close agent Session file Log", closeErr),
		)
	}
	if created {
		if err := syncDirectory(filepath.Dir(log.path)); err != nil {
			log.initialized = false
			log.runtimeIndexReady = false
			log.runtimeIndexPersisted = false
			return log.revision, errors.Join(session.ErrCommitUnknown, fmt.Errorf("sync agent Session file Log directory: %w", err))
		}
	}
	log.revision = body.End
	log.needsNewline = false
	log.canonicalBytes += int64(len(data))
	log.lastTransactionStart = transactionStart
	log.lastTransactionBytes = transactionBytes
	log.lastTransactionHash = hex.EncodeToString(transactionDigest[:])
	log.lastTransactionFirst = body.Start
	log.tailBytes += int64(len(data))
	log.tailRecords++
	anchor := canonicalAnchor{
		Start: transactionStart, Bytes: transactionBytes, SHA256: log.lastTransactionHash,
		FirstRevision: body.Start, LastRevision: body.End,
	}
	log.persistRuntimeCommandsBestEffort(committed, anchor)
	log.advanceRuntimeCommandIndexBestEffort(committed, previousCanonicalBytes, anchor)
	return log.revision, nil
}

func (log *logFile) Close() error {
	if log == nil {
		return nil
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.closeOnce.Do(func() {
		log.closed = true
		if log.release != nil {
			log.closeErr = log.release()
		}
	})
	return log.closeErr
}

func (log *logFile) replayLocked(
	ctx context.Context,
	repairTornTail bool,
	apply func(session.Record) error,
) (session.ReplayStats, error) {
	log.fullReplayCount++
	if err := ctx.Err(); err != nil {
		return session.ReplayStats{}, err
	}
	file, err := os.Open(log.path)
	if errors.Is(err, os.ErrNotExist) {
		log.initialized, log.revision, log.needsNewline = true, 0, false
		log.canonicalBytes, log.tailBytes, log.tailRecords = 0, 0, 0
		log.setRuntimeCommandIndex(nil, 0, 0, false)
		_ = os.Remove(log.runtimeCommandIndexPath())
		return session.ReplayStats{}, nil
	}
	if err != nil {
		return session.ReplayStats{}, fmt.Errorf("open agent Session file Log for replay: %w", err)
	}
	reader := bufio.NewReaderSize(file, replayBufferBytes)
	var stats session.ReplayStats
	var revision session.Revision
	var validBytes int64
	var needsNewline bool
	var lastStart, lastBytes int64
	var lastHash string
	var lastFirst session.Revision
	commands := make(map[runstate.CommandID]struct{})
	lineNumber := 0
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return stats, err
		}
		line, readErr := reader.ReadBytes('\n')
		stats.BytesRead += int64(len(line))
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		lineNumber++
		hasNewline := len(line) > 0 && line[len(line)-1] == '\n'
		payload := line
		if hasNewline {
			payload = payload[:len(payload)-1]
		}
		if len(payload) == 0 {
			_ = file.Close()
			return stats, fmt.Errorf("decode agent Session file Log line %d: empty transaction", lineNumber)
		}
		records, decodeErr := decodeTransaction(payload, revision)
		if decodeErr != nil {
			finalTorn := errors.Is(readErr, io.EOF) && !hasNewline && syntacticallyTornJSON(payload, decodeErr)
			if finalTorn && repairTornTail {
				if err := file.Close(); err != nil {
					return stats, fmt.Errorf("close torn agent Session file Log: %w", err)
				}
				if err := backupAndTruncate(log.path, validBytes); err != nil {
					return stats, err
				}
				file = nil
				stats.BytesRead = validBytes
				needsNewline = false
				break
			}
			_ = file.Close()
			return stats, fmt.Errorf("decode agent Session file Log line %d: %w", lineNumber, decodeErr)
		}
		lineStart := validBytes
		anchor := canonicalAnchor{
			Start: lineStart, Bytes: int64(len(line)),
			FirstRevision: records[0].Revision, LastRevision: records[len(records)-1].Revision,
		}
		digest := sha256.Sum256(line)
		anchor.SHA256 = hex.EncodeToString(digest[:])
		for _, record := range records {
			stats.RecordsRead++
			stats.BytesRead += int64(len(record.Kind) + len(record.Data))
			if apply != nil {
				record.Data = append(json.RawMessage(nil), record.Data...)
				if err := apply(record); err != nil {
					_ = file.Close()
					return stats, fmt.Errorf("reduce agent Session file Log at revision %d: %w", record.Revision, err)
				}
			}
		}
		log.persistRuntimeCommandsBestEffort(records, anchor)
		indexRuntimeCommands(commands, records)
		revision = records[len(records)-1].Revision
		validBytes += int64(len(line))
		lastStart, lastBytes, lastHash, lastFirst = lineStart, int64(len(line)), anchor.SHA256, records[0].Revision
		needsNewline = !hasNewline
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return stats, fmt.Errorf("read agent Session file Log line %d: %w", lineNumber, readErr)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
	}
	if file != nil {
		if err := file.Close(); err != nil {
			return stats, fmt.Errorf("close agent Session file Log after replay: %w", err)
		}
	}
	log.initialized, log.revision, log.needsNewline = true, revision, needsNewline
	log.canonicalBytes = validBytes
	log.lastTransactionStart, log.lastTransactionBytes, log.lastTransactionHash = lastStart, lastBytes, lastHash
	log.lastTransactionFirst = lastFirst
	log.tailBytes, log.tailRecords = validBytes, int64(lineNumber)
	log.setRuntimeCommandIndex(commands, validBytes, revision, false)
	if revision > 0 {
		if err := log.rewriteRuntimeCommandIndexLocked(); err == nil {
			log.runtimeIndexPersisted = true
		}
	}
	return stats, nil
}

type transactionBody struct {
	Version int              `json:"version"`
	Start   session.Revision `json:"start"`
	End     session.Revision `json:"end"`
	Records []session.Record `json:"records"`
}

type transaction struct {
	Version  int              `json:"version"`
	Start    session.Revision `json:"start"`
	End      session.Revision `json:"end"`
	Records  []session.Record `json:"records"`
	Checksum string           `json:"checksum"`
}

func decodeTransaction(encoded []byte, current session.Revision) ([]session.Record, error) {
	var value transaction
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	body := transactionBody{Version: value.Version, Start: value.Start, End: value.End, Records: value.Records}
	canonical, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(canonical)
	if value.Version != formatVersion || value.Checksum != hex.EncodeToString(digest[:]) {
		return nil, errors.New("unsupported version or checksum mismatch")
	}
	if len(value.Records) == 0 || value.Start != current+1 || value.End != value.Start+session.Revision(len(value.Records))-1 {
		return nil, errors.New("non-contiguous transaction revision range")
	}
	for index, record := range value.Records {
		want := value.Start + session.Revision(index)
		if record.Revision != want {
			return nil, fmt.Errorf("record revision %d, want %d", record.Revision, want)
		}
		if err := session.ValidateRecord(record); err != nil {
			return nil, err
		}
	}
	return value.Records, nil
}

type manifest struct {
	Version   int         `json:"version"`
	KeySHA256 string      `json:"key_sha256"`
	Key       session.Key `json:"key"`
}

type deletionMarker struct {
	Version   int         `json:"version"`
	KeySHA256 string      `json:"key_sha256"`
	Key       session.Key `json:"key"`
}

func ensureDeletionMarker(path string, key session.Key) error {
	canonical, err := session.CanonicalKey(key)
	if err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(canonical))
	marker := deletionMarker{Version: formatVersion, KeySHA256: hex.EncodeToString(digest[:]), Key: key}
	if err := validateDeletionMarkerValue(marker, key); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return validateDeletionMarker(path, key)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect agent Session deletion marker: %w", err)
	}
	encoded, err := json.Marshal(marker)
	if err != nil {
		return fmt.Errorf("encode agent Session deletion marker: %w", err)
	}
	if err := commitAtomicFile(path, append(encoded, '\n')); err != nil {
		return fmt.Errorf("commit agent Session deletion marker: %w", err)
	}
	return nil
}

func validateDeletionMarker(path string, key session.Key) error {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read agent Session deletion marker: %w", err)
	}
	var marker deletionMarker
	if err := json.Unmarshal(encoded, &marker); err != nil {
		return fmt.Errorf("decode agent Session deletion marker: %w", err)
	}
	return validateDeletionMarkerValue(marker, key)
}

func validateDeletionMarkerValue(marker deletionMarker, key session.Key) error {
	got, gotErr := session.CanonicalKey(marker.Key)
	want, wantErr := session.CanonicalKey(key)
	if gotErr != nil || wantErr != nil {
		return errors.New("agent Session deletion marker has invalid identity")
	}
	digest := sha256.Sum256([]byte(want))
	if marker.Version != formatVersion || marker.KeySHA256 != hex.EncodeToString(digest[:]) || got != want {
		return errors.New("agent Session deletion marker does not match Session identity")
	}
	return nil
}

func deletionPending(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("inspect agent Session deletion marker: %w", err)
	}
}

func ensureManifest(path string, key session.Key, digest [sha256.Size]byte) error {
	want := manifest{Version: formatVersion, KeySHA256: hex.EncodeToString(digest[:]), Key: key}
	existing, err := os.ReadFile(path)
	if err == nil {
		var got manifest
		if err := json.Unmarshal(existing, &got); err != nil {
			return fmt.Errorf("decode agent Session file manifest: %w", err)
		}
		gotKey, gotErr := session.CanonicalKey(got.Key)
		wantKey, wantErr := session.CanonicalKey(want.Key)
		if gotErr != nil || wantErr != nil || got.Version != want.Version || got.KeySHA256 != want.KeySHA256 || gotKey != wantKey {
			return errors.New("agent Session file manifest does not match Session identity")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read agent Session file manifest: %w", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		return fmt.Errorf("encode agent Session file manifest: %w", err)
	}
	return commitAtomicFile(path, append(encoded, '\n'))
}

func commitAtomicFile(path string, encoded []byte) error {
	temporary := path + ".tmp-" + newID()
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if err := writeAll(file, encoded); err != nil {
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
	committed = true
	return syncDirectory(filepath.Dir(path))
}

func acquireLease(ctx context.Context, path string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lease := flock.New(path, flock.SetPermissions(0o600))
	locked, err := lease.TryLockContext(ctx, leaseRetryInterval)
	if err != nil {
		return nil, fmt.Errorf("acquire agent Session file lease: %w", err)
	}
	if !locked {
		return nil, errors.New("acquire agent Session file lease: lock was not acquired")
	}
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() { result = lease.Unlock() })
		return result
	}, nil
}

func syntacticallyTornJSON(encoded []byte, decodeErr error) bool {
	var syntax *json.SyntaxError
	return errors.As(decodeErr, &syntax) && syntax.Offset >= int64(len(encoded)) &&
		strings.Contains(syntax.Error(), "unexpected end of JSON input")
}

func backupAndTruncate(path string, validBytes int64) error {
	backup := fmt.Sprintf("%s.recovery.%d.%d.bak", path, time.Now().UTC().UnixNano(), os.Getpid())
	source, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open torn agent Session file Log for backup: %w", err)
	}
	target, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = source.Close()
		return fmt.Errorf("create agent Session file Log recovery backup: %w", err)
	}
	_, copyErr := io.Copy(target, source)
	copyErr = errors.Join(copyErr, source.Close())
	if copyErr == nil {
		copyErr = target.Sync()
	}
	copyErr = errors.Join(copyErr, target.Close())
	if copyErr != nil {
		return fmt.Errorf("backup torn agent Session file Log: %w", copyErr)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open torn agent Session file Log for repair: %w", err)
	}
	repairErr := file.Truncate(validBytes)
	if repairErr == nil {
		repairErr = file.Sync()
	}
	repairErr = errors.Join(repairErr, file.Close())
	if repairErr != nil {
		return fmt.Errorf("repair torn agent Session file Log (backup %s): %w", backup, repairErr)
	}
	return syncDirectory(filepath.Dir(path))
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func wrapIO(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

var fallbackSequence atomic.Uint64

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("fallback-%x-%x-%x", time.Now().UTC().UnixNano(), os.Getpid(), fallbackSequence.Add(1))
}

var _ session.Store = (*Store)(nil)
var _ session.Log = (*logFile)(nil)
