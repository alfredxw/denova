// Package file provides the durable local-file implementation of session.Store.
package file

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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alfredxw/denova/agent/session"
	"github.com/gofrs/flock"
)

const (
	formatVersion      = 1
	leaseRetryInterval = 10 * time.Millisecond
	replayBufferBytes  = 64 << 10
)

// Store keeps one checksummed append-only transaction log per exact Session
// key. A transaction contains a complete Append batch, so replay sees all of a
// batch or none of it after a crash.
type Store struct{ root string }

func New(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("agent Session file Store root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve agent Session file Store root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create agent Session file Store root: %w", err)
	}
	return &Store{root: filepath.Clean(absolute)}, nil
}

func (store *Store) Open(ctx context.Context, key session.Key) (session.Log, error) {
	if store == nil || store.root == "" {
		return nil, errors.New("agent Session file Store is nil")
	}
	key, err := session.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	canonical, err := session.CanonicalKey(key)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(canonical))
	base := filepath.Join(store.root, hex.EncodeToString(digest[:]))
	release, err := acquireLease(ctx, base+".lease")
	if err != nil {
		return nil, err
	}
	if err := ensureManifest(base+".manifest.json", key, digest); err != nil {
		_ = release()
		return nil, err
	}
	return &logFile{path: base + ".jsonl", release: release}, nil
}

type logFile struct {
	path    string
	release func() error

	mu           sync.Mutex
	initialized  bool
	revision     session.Revision
	needsNewline bool
	closed       bool
	closeOnce    sync.Once
	closeErr     error
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
	if log.needsNewline {
		data = append(data, '\n')
	}
	data = append(data, encoded...)
	data = append(data, '\n')
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
			return log.revision, errors.Join(session.ErrCommitUnknown, fmt.Errorf("sync agent Session file Log directory: %w", err))
		}
	}
	log.revision = body.End
	log.needsNewline = false
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
	if err := ctx.Err(); err != nil {
		return session.ReplayStats{}, err
	}
	file, err := os.Open(log.path)
	if errors.Is(err, os.ErrNotExist) {
		log.initialized, log.revision, log.needsNewline = true, 0, false
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
		revision = records[len(records)-1].Revision
		validBytes += int64(len(line))
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
	temporary := path + ".tmp-" + newID()
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create agent Session file manifest: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if err := writeAll(file, append(encoded, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write agent Session file manifest: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync agent Session file manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close agent Session file manifest: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit agent Session file manifest: %w", err)
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
