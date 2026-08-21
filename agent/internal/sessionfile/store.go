// Package sessionfile implements the built-in transcript store.
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

	"github.com/alfredxw/denova/agent/internal/localfs"
	"github.com/alfredxw/denova/agent/session"
	"github.com/gofrs/flock"
)

const (
	formatVersion      = 2
	leaseRetryInterval = 10 * time.Millisecond
	replayBufferBytes  = 64 << 10
)

// Store keeps one manifest and one append-only transcript per Session.
// Checksummed transaction lines make a multi-record Append the physical unit;
// an incomplete final line is discarded on reopen.
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

type manifest struct {
	Version   int         `json:"version"`
	KeySHA256 string      `json:"key_sha256"`
	Key       session.Key `json:"key"`
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

func (store *Store) Open(ctx context.Context, key session.Key) (session.Log, error) {
	if store == nil || store.root == "" {
		return nil, errors.New("agent Session file Store is nil")
	}
	key, err := session.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	base, digest, err := store.baseForKey(key)
	if err != nil {
		return nil, err
	}
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

func (store *Store) List(ctx context.Context, selector session.Selector) ([]session.Key, error) {
	if store == nil || store.root == "" {
		return nil, errors.New("agent Session file Store is nil")
	}
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(store.root, "*.manifest.json"))
	if err != nil {
		return nil, err
	}
	result := make([]session.Key, 0, len(paths))
	for _, path := range paths {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Agent Session manifest: %w", err)
		}
		var value manifest
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("decode Agent Session manifest: %w", err)
		}
		key, err := session.NormalizeKey(value.Key)
		if err != nil || value.Version != formatVersion {
			return nil, errors.New("Agent Session manifest is invalid")
		}
		base, digest, _ := store.baseForKey(key)
		if value.KeySHA256 != hex.EncodeToString(digest[:]) || filepath.Base(path) != filepath.Base(base)+".manifest.json" {
			return nil, errors.New("Agent Session manifest identity is invalid")
		}
		if selector.Matches(key) {
			result = append(result, key)
		}
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
	base, _, err := store.baseForKey(key)
	if err != nil {
		return err
	}
	release, err := acquireLease(ctx, base+".lease")
	if err != nil {
		return err
	}
	defer release()
	for _, path := range []string{base + ".jsonl", base + ".manifest.json"} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete Agent Session file: %w", err)
		}
	}
	return localfs.SyncDirectory(store.root)
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
	path        string
	release     func() error
	mu          sync.Mutex
	initialized bool
	revision    session.Revision
	validBytes  int64
	closed      bool
	closeOnce   sync.Once
	closeErr    error
}

func (log *logFile) Replay(ctx context.Context, apply func(session.Record) error) (session.ReplayStats, error) {
	if apply == nil {
		return session.ReplayStats{}, errors.New("replay Agent Session: reducer is required")
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return session.ReplayStats{}, session.ErrLogClosed
	}
	return log.replayLocked(ctx, apply)
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
		if _, err := log.replayLocked(ctx, func(session.Record) error { return nil }); err != nil {
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
		record.Revision = expected + session.Revision(index) + 1
		record.Data = append(json.RawMessage(nil), record.Data...)
		committed[index] = record
	}
	body := transactionBody{Version: formatVersion, Start: committed[0].Revision, End: committed[len(committed)-1].Revision, Records: committed}
	canonical, err := json.Marshal(body)
	if err != nil {
		return log.revision, err
	}
	digest := sha256.Sum256(canonical)
	encoded, err := json.Marshal(transaction{
		Version: body.Version, Start: body.Start, End: body.End, Records: body.Records,
		Checksum: hex.EncodeToString(digest[:]),
	})
	if err != nil {
		return log.revision, err
	}
	encoded = append(encoded, '\n')
	file, err := os.OpenFile(log.path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return log.revision, err
	}
	if _, err := file.Seek(log.validBytes, io.SeekStart); err == nil {
		err = file.Truncate(log.validBytes)
	}
	if err == nil {
		err = writeAll(file, encoded)
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		log.initialized = false
		return log.revision, errors.Join(session.ErrCommitUnknown, err)
	}
	log.revision = body.End
	log.validBytes += int64(len(encoded))
	return log.revision, nil
}

func (log *logFile) replayLocked(ctx context.Context, apply func(session.Record) error) (session.ReplayStats, error) {
	file, err := os.Open(log.path)
	if errors.Is(err, os.ErrNotExist) {
		log.initialized, log.revision, log.validBytes = true, 0, 0
		return session.ReplayStats{}, nil
	}
	if err != nil {
		return session.ReplayStats{}, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, replayBufferBytes)
	var stats session.ReplayStats
	var revision session.Revision
	var validBytes int64
	for {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return stats, err
			}
		}
		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if len(line) == 0 || line[len(line)-1] != '\n' {
			// A crash can leave only the final transaction incomplete. Ignore it;
			// the next append truncates back to validBytes.
			break
		}
		records, err := decodeTransaction(line[:len(line)-1], revision)
		if err != nil {
			return stats, err
		}
		for _, record := range records {
			stats.RecordsRead++
			stats.BytesRead += int64(len(record.Kind) + len(record.Data))
			if err := apply(record); err != nil {
				return stats, err
			}
		}
		revision = records[len(records)-1].Revision
		validBytes += int64(len(line))
		if readErr != nil {
			break
		}
	}
	log.initialized, log.revision, log.validBytes = true, revision, validBytes
	return stats, nil
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
	if body.Version != formatVersion || value.Checksum != hex.EncodeToString(digest[:]) ||
		len(body.Records) == 0 || body.Start != current+1 || body.End != body.Start+session.Revision(len(body.Records))-1 {
		return nil, errors.New("Agent Session transaction is invalid")
	}
	for index, record := range body.Records {
		if record.Revision != body.Start+session.Revision(index) || session.ValidateRecord(record) != nil {
			return nil, errors.New("Agent Session record is invalid")
		}
	}
	return body.Records, nil
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

func ensureManifest(path string, key session.Key, digest [sha256.Size]byte) error {
	want := manifest{Version: formatVersion, KeySHA256: hex.EncodeToString(digest[:]), Key: key}
	if existing, err := os.ReadFile(path); err == nil {
		var got manifest
		if json.Unmarshal(existing, &got) != nil || got.Version != want.Version || got.KeySHA256 != want.KeySHA256 {
			return errors.New("Agent Session manifest does not match Session identity")
		}
		gotKey, err := session.CanonicalKey(got.Key)
		if err != nil {
			return errors.New("Agent Session manifest does not match Session identity")
		}
		wantKey, err := session.CanonicalKey(want.Key)
		if err != nil {
			return err
		}
		if gotKey != wantKey {
			return errors.New("Agent Session manifest does not match Session identity")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		return err
	}
	return commitAtomicFile(path, append(encoded, '\n'))
}

func acquireLease(ctx context.Context, path string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	lease := flock.New(path, flock.SetPermissions(0o600))
	locked, err := lease.TryLockContext(ctx, leaseRetryInterval)
	if err != nil || !locked {
		return nil, errors.Join(errors.New("acquire Agent Session lease"), err)
	}
	var once sync.Once
	var result error
	return func() error {
		once.Do(func() { result = lease.Unlock() })
		return result
	}, nil
}

func commitAtomicFile(path string, data []byte) error {
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
	err = writeAll(file, data)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err == nil {
		err = os.Rename(temporary, path)
	}
	if err != nil {
		return err
	}
	committed = true
	return localfs.SyncDirectory(filepath.Dir(path))
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
