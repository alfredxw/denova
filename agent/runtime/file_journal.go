package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const fileJournalVersion = 1
const fileJournalManifestVersion = 1

// FileJournalStore keeps one JSON transaction per line. Each line is
// checksummed and contains a contiguous cursor range, so a command batch is
// either fully replayed or ignored as a torn final append.
type FileJournalStore struct {
	root    string
	options FileJournalOptions
}

// FileJournalOptions controls bounded tail generations. A checkpoint is
// attempted at the next reducer-safe transaction boundary after either tail
// threshold is crossed. Zero values select production defaults.
type FileJournalOptions struct {
	CheckpointTailBytes   int64
	CheckpointTailRecords int64
}

func DefaultFileJournalOptions() FileJournalOptions {
	return FileJournalOptions{}.normalized()
}

func (options FileJournalOptions) normalized() FileJournalOptions {
	if options.CheckpointTailBytes <= 0 {
		options.CheckpointTailBytes = 64 << 20
	}
	if options.CheckpointTailRecords <= 0 {
		options.CheckpointTailRecords = 4096
	}
	return options
}

func NewFileJournalStore(root string) (*FileJournalStore, error) {
	return NewFileJournalStoreWithOptions(root, FileJournalOptions{})
}

func NewFileJournalStoreWithOptions(root string, options FileJournalOptions) (*FileJournalStore, error) {
	if root == "" {
		return nil, fmt.Errorf("file journal root is required")
	}
	cleaned := filepath.Clean(root)
	if err := os.MkdirAll(cleaned, 0o700); err != nil {
		return nil, fmt.Errorf("create file journal root: %w", err)
	}
	return &FileJournalStore{root: cleaned, options: options.normalized()}, nil
}

func (s *FileJournalStore) OpenJournal(ctx context.Context, key string) (Journal, error) {
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
	journal := &fileJournal{
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

func manifestBinding(key string) BindingRef {
	var binding BindingRef
	if err := json.Unmarshal([]byte(key), &binding); err == nil && ValidateBindingRef(binding) == nil {
		return binding
	}
	return BindingRef{}
}

type fileJournalManifest struct {
	Version   int         `json:"version"`
	KeySHA256 string      `json:"key_sha256"`
	Binding   *BindingRef `json:"binding,omitempty"`
}

func ensureFileJournalManifest(path, key string, digest [sha256.Size]byte) error {
	want := fileJournalManifest{Version: fileJournalManifestVersion, KeySHA256: hex.EncodeToString(digest[:])}
	var binding BindingRef
	if err := json.Unmarshal([]byte(key), &binding); err == nil && ValidateBindingRef(binding) == nil {
		want.Binding = &binding
	}
	existing, err := os.ReadFile(path)
	if err == nil {
		var got fileJournalManifest
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

func bindingRefsEqual(left, right *BindingRef) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Equal(*right)
}

type fileJournal struct {
	path                  string
	tailPath              string
	binding               BindingRef
	keySHA256             string
	options               FileJournalOptions
	activeGeneration      fileJournalGeneration
	generationCandidates  []fileJournalGeneration
	manifestSequence      uint64
	checkpoint            *harnessCheckpoint
	tailBytes             int64
	tailRecords           int64
	checkpointHook        func(fileJournalCheckpointStage) error
	mu                    sync.Mutex
	initialized           bool
	cursor                Cursor
	needsNewline          bool
	commandIndex          map[CommandID]CommandRecord
	indexReady            bool
	pendingCommandRecords map[CommandID]CommandRecord
	lastTailHash          string
	lastReplay            JournalReplayStats
	closed                bool
	openReplayFile        func(string) (*os.File, error)
	syncFile              func(*os.File) error
	closeFile             func(*os.File) error
	syncDirectory         func(string) error
	release               func() error
	closeOnce             sync.Once
	closeErr              error
}

func (j *fileJournal) Close() error {
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
	Version  int            `json:"version"`
	Start    Cursor         `json:"start"`
	End      Cursor         `json:"end"`
	Events   []encodedEvent `json:"events"`
	Checksum string         `json:"checksum"`
}

type journalTransactionBody struct {
	Version int            `json:"version"`
	Start   Cursor         `json:"start"`
	End     Cursor         `json:"end"`
	Events  []encodedEvent `json:"events"`
}

func (j *fileJournal) Load(ctx context.Context) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if j.closed {
		return nil, fmt.Errorf("file journal is closed")
	}
	if j.activeGeneration.SnapshotFile != "" {
		state := newHarnessState(j.binding)
		if _, err := j.replayHarnessStateLocked(ctx, &state); err != nil {
			j.initialized = false
			return nil, err
		}
		return cloneEvents(state.events), nil
	}
	events := make([]Event, 0)
	_, err := j.replayLocked(ctx, true, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		j.initialized = false
		return nil, err
	}
	return events, nil
}

// Replay decodes one checksummed transaction at a time and sends its events
// directly to reduce. Unlike Load it never retains the complete event history.
func (j *fileJournal) Replay(ctx context.Context, reduce func(Event) error) (JournalReplayStats, error) {
	if err := ctx.Err(); err != nil {
		return JournalReplayStats{}, err
	}
	if reduce == nil {
		return JournalReplayStats{}, fmt.Errorf("file journal replay reducer is required")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return JournalReplayStats{}, fmt.Errorf("file journal is closed")
	}
	if j.activeGeneration.SnapshotFile != "" {
		state := newHarnessState(j.binding)
		stats, err := j.replayHarnessStateLocked(ctx, &state)
		if err != nil {
			j.initialized = false
			return stats, err
		}
		for _, event := range state.events {
			if err := reduce(event); err != nil {
				return stats, err
			}
		}
		return stats, nil
	}
	stats, err := j.replayLocked(ctx, true, reduce)
	if err != nil {
		j.initialized = false
		return stats, err
	}
	return stats, nil
}

func (j *fileJournal) Append(ctx context.Context, expected Cursor, payloads []EventPayload) ([]Event, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(payloads) == 0 {
		return nil, nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if j.closed {
		return nil, fmt.Errorf("file journal is closed")
	}
	if !j.initialized {
		state := newProjectionState(j.binding)
		if _, err := j.replayHarnessStateLocked(ctx, &state); err != nil {
			return nil, err
		}
	}
	current := j.cursor
	if current != expected {
		return nil, fmt.Errorf("journal cursor conflict: have %d, expected %d", current, expected)
	}
	committed := make([]Event, 0, len(payloads))
	encoded := make([]encodedEvent, 0, len(payloads))
	for _, payload := range payloads {
		current++
		event := Event{Cursor: current, Durability: EventDurable, Payload: payload}
		diskEvent, encodeErr := encodeDurableEvent(event)
		if encodeErr != nil {
			return nil, encodeErr
		}
		committed = append(committed, event)
		encoded = append(encoded, diskEvent)
	}
	body := journalTransactionBody{
		Version: fileJournalVersion,
		Start:   committed[0].Cursor,
		End:     committed[len(committed)-1].Cursor,
		Events:  encoded,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode file journal checksum body: %w", err)
	}
	digest := sha256.Sum256(bodyJSON)
	transaction := journalTransaction{
		Version: body.Version, Start: body.Start, End: body.End, Events: body.Events,
		Checksum: hex.EncodeToString(digest[:]),
	}
	line, err := json.Marshal(transaction)
	if err != nil {
		return nil, fmt.Errorf("encode file journal transaction: %w", err)
	}
	prefix := []byte(nil)
	if j.needsNewline {
		prefix = []byte{'\n'}
	}
	record := append(prefix, line...)
	record = append(record, '\n')
	file, err := os.OpenFile(j.tailPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open file journal for append: %w", err)
	}
	writeErr := writeAll(file, record)
	// Even after Write reports an error, Sync can establish that the complete
	// record which read-back observes reached the durability boundary.
	syncErr := j.syncFile(file)
	closeErr := j.closeFile(file)
	if writeErr != nil || syncErr != nil || closeErr != nil {
		operationErr := errors.Join(
			wrapOptionalError("append file journal", writeErr),
			wrapOptionalError("sync file journal", syncErr),
			wrapOptionalError("close file journal", closeErr),
		)
		confirmed, reconcileErr := j.reconcileAppend(expected, committed, encoded)
		if reconcileErr != nil {
			return nil, errors.Join(operationErr, fmt.Errorf("reconcile ambiguous file journal append: %w", reconcileErr))
		}
		// Read-back resolves a lost Write/Close result only after Sync itself
		// succeeded. Seeing page-cache bytes cannot mask a failed fsync.
		if !confirmed || syncErr != nil {
			return nil, operationErr
		}
		if err := j.syncDirectory(filepath.Dir(j.tailPath)); err != nil {
			return nil, fmt.Errorf("sync file journal directory: %w", err)
		}
		return cloneEvents(committed), nil
	}
	if err := j.syncDirectory(filepath.Dir(j.tailPath)); err != nil {
		_, reconcileErr := j.reconcileAppend(expected, committed, encoded)
		if reconcileErr != nil {
			return nil, errors.Join(
				fmt.Errorf("sync file journal directory: %w", err),
				fmt.Errorf("reconcile directory-sync-uncertain append: %w", reconcileErr),
			)
		}
		return nil, fmt.Errorf("sync file journal directory: %w", err)
	}
	j.cursor = committed[len(committed)-1].Cursor
	j.needsNewline = false
	j.lastTailHash = journalRecordHash(line)
	j.tailBytes += int64(len(record))
	j.tailRecords++
	indexChainReady := j.indexReady
	j.indexCommittedCommands(committed)
	var indexErr error
	if j.activeGeneration.SnapshotFile != "" {
		indexErr = nil
	} else if indexChainReady {
		indexErr = j.appendCommandIndexLocked(expected, committed)
	} else {
		indexErr = j.rewriteCommandIndexLocked()
	}
	if indexErr != nil {
		// The index is a rebuildable acceleration structure. The canonical
		// transaction has crossed its durability boundary, so an index write
		// failure must not turn a confirmed append into a false command error.
		j.indexReady = false
		slog.ErrorContext(ctx, fmt.Sprintf("[agent-runtime] command index update deferred journal=%s cursor=%d error=%v", filepath.Base(j.path), j.cursor, indexErr))
	}
	return cloneEvents(committed), nil
}

func (j *fileJournal) LookupCommand(ctx context.Context, commandID CommandID) (CommandRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return CommandRecord{}, false, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return CommandRecord{}, false, fmt.Errorf("file journal is closed")
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
				return CommandRecord{}, false, replayErr
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

func (j *fileJournal) reconcileAppend(expected Cursor, committed []Event, encoded []encodedEvent) (bool, error) {
	suffix := make([]Event, 0, len(committed))
	_, err := j.replayLocked(context.Background(), true, func(event Event) error {
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
	if j.cursor != expected+Cursor(len(committed)) || len(suffix) < len(committed) {
		return false, nil
	}
	for index, event := range suffix {
		reloaded, err := encodeDurableEvent(event)
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

func decodeTransaction(line []byte, current Cursor) ([]Event, error) {
	var transaction journalTransaction
	if err := json.Unmarshal(line, &transaction); err != nil {
		return nil, err
	}
	if transaction.Version != fileJournalVersion {
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
	if transaction.End != transaction.Start+Cursor(len(transaction.Events))-1 {
		return nil, fmt.Errorf("transaction end %d does not match %d events from %d", transaction.End, len(transaction.Events), transaction.Start)
	}
	events := make([]Event, 0, len(transaction.Events))
	for index, encoded := range transaction.Events {
		expected := transaction.Start + Cursor(index)
		if encoded.Cursor != expected {
			return nil, fmt.Errorf("event cursor %d, want %d", encoded.Cursor, expected)
		}
		event, decodeErr := decodeDurableEvent(encoded)
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
