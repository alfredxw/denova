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
	"sync"
)

const journalVersion = 1
const journalManifestVersion = 1

// Store keeps one JSON transaction per line. Each line is
// checksummed and contains a contiguous cursor range, so a command batch is
// either fully replayed or ignored as a torn final append.
type Store struct {
	root    string
	options Options
}

// Options controls bounded tail generations. A checkpoint is
// attempted at the next reducer-safe transaction boundary after either tail
// threshold is crossed. Zero values select production defaults.
type Options struct {
	CheckpointTailBytes   int64
	CheckpointTailRecords int64
}

func DefaultOptions() Options {
	return Options{}.normalized()
}

func (options Options) normalized() Options {
	if options.CheckpointTailBytes <= 0 {
		options.CheckpointTailBytes = 64 << 20
	}
	if options.CheckpointTailRecords <= 0 {
		options.CheckpointTailRecords = 4096
	}
	return options
}

func NewStore(root string) (*Store, error) {
	return NewStoreWithOptions(root, Options{})
}

func NewStoreWithOptions(root string, options Options) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("file journal root is required")
	}
	cleaned := filepath.Clean(root)
	if err := os.MkdirAll(cleaned, 0o700); err != nil {
		return nil, fmt.Errorf("create file journal root: %w", err)
	}
	return &Store{root: cleaned, options: options.normalized()}, nil
}

func (s *Store) OpenJournal(ctx context.Context, key string) (runstate.Journal, error) {
	if s == nil {
		return nil, fmt.Errorf("file journal store is nil")
	}
	digest := sha256.Sum256([]byte(key))
	path := filepath.Join(s.root, hex.EncodeToString(digest[:])+".jsonl")
	releaseLease, err := acquireFileJournalLease(ctx, path+".lease")
	if err != nil {
		return nil, fmt.Errorf("lock file journal binding lease: %w", err)
	}
	if err := ensureFileJournalManifest(path+".manifest.json", key, digest); err != nil {
		_ = releaseLease()
		return nil, err
	}
	journal := &journal{
		path:           path,
		tailPath:       path,
		binding:        manifestBinding(key),
		keySHA256:      hex.EncodeToString(digest[:]),
		options:        s.options.normalized(),
		openReplayFile: func(path string) (*os.File, error) { return os.Open(path) },
		syncFile:       func(file *os.File) error { return file.Sync() },
		closeFile:      func(file *os.File) error { return file.Close() },
		syncDirectory:  syncDirectory,
		release:        releaseLease,
	}
	if err := journal.loadGenerationLayoutLocked(); err != nil {
		_ = releaseLease()
		return nil, err
	}
	return journal, nil
}

func manifestBinding(key string) runstate.BindingRef {
	var binding runstate.BindingRef
	if err := json.Unmarshal([]byte(key), &binding); err == nil && runstate.ValidateBindingRef(binding) == nil {
		return binding
	}
	return runstate.BindingRef{}
}

type journalManifest struct {
	Version   int                  `json:"version"`
	KeySHA256 string               `json:"key_sha256"`
	Binding   *runstate.BindingRef `json:"binding,omitempty"`
}

func ensureFileJournalManifest(path, key string, digest [sha256.Size]byte) error {
	want := journalManifest{Version: journalManifestVersion, KeySHA256: hex.EncodeToString(digest[:])}
	var binding runstate.BindingRef
	if err := json.Unmarshal([]byte(key), &binding); err == nil && runstate.ValidateBindingRef(binding) == nil {
		want.Binding = &binding
	}
	existing, err := os.ReadFile(path)
	if err == nil {
		var got journalManifest
		if err := json.Unmarshal(existing, &got); err != nil {
			return fmt.Errorf("decode file journal manifest: %w", err)
		}
		if got.Version != want.Version || got.KeySHA256 != want.KeySHA256 || !bindingRefsEqual(got.Binding, want.Binding) {
			return fmt.Errorf("file journal manifest does not match binding identity")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read file journal manifest: %w", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		return fmt.Errorf("encode file journal manifest: %w", err)
	}
	temporary := path + ".tmp-" + newID("manifest")
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create file journal manifest: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporary)
		}
	}()
	if err := writeAll(file, append(encoded, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write file journal manifest: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync file journal manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close file journal manifest: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit file journal manifest: %w", err)
	}
	cleanup = false
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync file journal manifest directory: %w", err)
	}
	return nil
}

func bindingRefsEqual(left, right *runstate.BindingRef) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

type journal struct {
	path                  string
	tailPath              string
	binding               runstate.BindingRef
	keySHA256             string
	options               Options
	activeGeneration      journalGeneration
	generationCandidates  []journalGeneration
	manifestSequence      uint64
	tailBytes             int64
	tailRecords           int64
	checkpointHook        func(journalCheckpointStage) error
	mu                    sync.Mutex
	initialized           bool
	cursor                runstate.Cursor
	needsNewline          bool
	commandIndex          map[runstate.CommandID]runstate.CommandRecord
	indexReady            bool
	pendingCommandRecords map[runstate.CommandID]runstate.CommandRecord
	lastTailHash          string
	lastReplay            runstate.JournalReplayStats
	closed                bool
	openReplayFile        func(string) (*os.File, error)
	syncFile              func(*os.File) error
	closeFile             func(*os.File) error
	syncDirectory         func(string) error
	release               func() error
	closeOnce             sync.Once
	closeErr              error
}

func (j *journal) Close() error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.closeOnce.Do(func() {
		j.closed = true
		if j.release != nil {
			j.closeErr = j.release()
		}
	})
	return j.closeErr
}

type journalTransaction struct {
	Version  int                     `json:"version"`
	Start    runstate.Cursor         `json:"start"`
	End      runstate.Cursor         `json:"end"`
	Events   []runstate.JournalEvent `json:"events"`
	Checksum string                  `json:"checksum"`
}

type journalTransactionBody struct {
	Version int                     `json:"version"`
	Start   runstate.Cursor         `json:"start"`
	End     runstate.Cursor         `json:"end"`
	Events  []runstate.JournalEvent `json:"events"`
}
