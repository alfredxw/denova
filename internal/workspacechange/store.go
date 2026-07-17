package workspacechange

import (
	"bufio"
	"bytes"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/workspacepath"
)

const (
	eventChangePrepared         = "change_prepared"
	eventChangeApplied          = "change_applied"
	eventChangeRecoveredApplied = "change_recovered_applied"
	eventChangeAborted          = "change_aborted"
	eventChangeConflicted       = "change_conflicted"
	eventReviewUpdated          = "review_updated"
	eventChangeState            = "change_state"
	eventCommentUpserted        = "comment_upserted"
	eventCommentsUpserted       = "comments_upserted"
	eventHistoryState           = "history_state"
	eventOperationPrepared      = "operation_prepared"
	eventOperationPathApplied   = "operation_path_applied"
	eventOperationCommitted     = "operation_committed"
	eventOperationConflicted    = "operation_conflicted"

	historyStateUndone          = "undone"
	historyStateRedone          = "redone"
	historyStateRedoInvalidated = "redo_invalidated"
	maxLedgerEventBytes         = 2 * 1024 * 1024
)

type ledgerEvent struct {
	Type          string            `json:"type"`
	CreatedAt     time.Time         `json:"created_at"`
	Metadata      *ChangeMetadata   `json:"metadata,omitempty"`
	ChangeSet     *ChangeSet        `json:"change_set,omitempty"`
	ChangeSetID   string            `json:"change_set_id,omitempty"`
	GroupID       string            `json:"group_id,omitempty"`
	EditStatuses  map[string]string `json:"edit_statuses,omitempty"`
	ApplyState    string            `json:"apply_state,omitempty"`
	Comment       *Comment          `json:"comment,omitempty"`
	Comments      []Comment         `json:"comments,omitempty"`
	HistoryState  string            `json:"history_state,omitempty"`
	Operation     *durableOperation `json:"operation,omitempty"`
	OperationID   string            `json:"operation_id,omitempty"`
	OperationPath string            `json:"operation_path,omitempty"`
	ConflictPaths []string          `json:"conflict_paths,omitempty"`
}

type eventStore struct {
	dir        string
	ledgerPath string
	blobDir    string
	durability *durabilityOps
	root       *os.Root
}

func newEventStore(workspace string, durability *durabilityOps) (*eventStore, error) {
	dir := workspacepath.Path(workspace, "changes")
	blobDir := filepath.Join(dir, "blobs")
	blobRel, err := filepath.Rel(workspace, blobDir)
	if err != nil || blobRel == "." || blobRel == ".." || strings.HasPrefix(blobRel, ".."+string(filepath.Separator)) {
		return nil, newError(ErrorCodeConflict, "workspace change storage path escapes the workspace", map[string]any{"path": blobDir})
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := mkdirAllRootDurable(root, blobRel, 0o700, durability); err != nil {
		return nil, err
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	canonicalBlobDir, err := filepath.EvalSymlinks(blobDir)
	if err != nil {
		return nil, err
	}
	canonicalRel, err := filepath.Rel(canonicalWorkspace, canonicalBlobDir)
	if err != nil || canonicalRel == ".." || strings.HasPrefix(canonicalRel, ".."+string(filepath.Separator)) {
		return nil, newError(ErrorCodeConflict, "workspace change storage resolved outside the workspace", map[string]any{"path": blobDir})
	}
	store := &eventStore{
		dir:        dir,
		ledgerPath: filepath.Join(dir, "ledger.jsonl"),
		blobDir:    blobDir,
		durability: durability,
	}
	// Make the ledger inode and its directory entry durable before append ever
	// relies on it. Later appends only extend this existing file.
	dirRel, err := filepath.Rel(workspace, dir)
	if err != nil || dirRel == "." || dirRel == ".." || strings.HasPrefix(dirRel, ".."+string(filepath.Separator)) {
		return nil, newError(ErrorCodeConflict, "workspace change ledger path escapes the workspace", map[string]any{"path": dir})
	}
	changesRoot, err := root.OpenRoot(filepath.FromSlash(dirRel))
	if err != nil {
		return nil, err
	}
	if info, statErr := changesRoot.Lstat("ledger.jsonl"); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			_ = changesRoot.Close()
			return nil, newError(ErrorCodeConflict, "workspace change ledger path is not a regular file", map[string]any{"path": store.ledgerPath})
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		_ = changesRoot.Close()
		return nil, statErr
	}
	ledger, err := changesRoot.OpenFile("ledger.jsonl", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		_ = changesRoot.Close()
		return nil, err
	}
	info, statErr := ledger.Stat()
	if statErr == nil && !info.Mode().IsRegular() {
		statErr = fmt.Errorf("workspace change ledger is not a regular file")
	}
	if statErr == nil {
		statErr = ledger.Sync()
	}
	closeErr := ledger.Close()
	if statErr != nil {
		_ = changesRoot.Close()
		return nil, statErr
	}
	if closeErr != nil {
		_ = changesRoot.Close()
		return nil, closeErr
	}
	if err := durability.syncRootDir(changesRoot, "."); err != nil {
		_ = changesRoot.Close()
		return nil, err
	}
	store.root = changesRoot
	return store, nil
}

func (s *eventStore) close() {
	if s != nil && s.root != nil {
		_ = s.root.Close()
	}
}

func (s *eventStore) openBlobRoot() (*os.Root, error) {
	info, err := s.root.Lstat("blobs")
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, newError(ErrorCodeConflict, "workspace change blob path is not a directory", map[string]any{"path": s.blobDir})
	}
	return s.root.OpenRoot("blobs")
}

func (s *eventStore) append(event ledgerEvent) error {
	encodedEvent := event
	if encodedEvent.ChangeSet != nil {
		change := ledgerChangeSet(*encodedEvent.ChangeSet)
		encodedEvent.ChangeSet = &change
	}
	if encodedEvent.Operation != nil {
		operation := cloneDurableOperation(*encodedEvent.Operation)
		for index := range operation.Changes {
			operation.Changes[index].ChangeSet = ledgerChangeSet(operation.Changes[index].ChangeSet)
		}
		encodedEvent.Operation = &operation
	}
	encoded, err := json.Marshal(encodedEvent)
	if err != nil {
		return err
	}
	if len(encoded) > maxLedgerEventBytes {
		return fmt.Errorf("workspace change ledger event exceeds %d bytes", maxLedgerEventBytes)
	}
	info, err := s.root.Lstat("ledger.jsonl")
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return newError(ErrorCodeConflict, "workspace change ledger path is not a regular file", map[string]any{"path": s.ledgerPath})
	}
	file, err := s.root.OpenFile("ledger.jsonl", os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return nil
}

func ledgerChangeSet(input ChangeSet) ChangeSet {
	change := cloneChangeSet(input)
	change.BeforeContent = ""
	change.AfterContent = ""
	for index := range change.Edits {
		change.Edits[index].OldString = ""
		change.Edits[index].NewString = ""
	}
	return change
}

func (s *eventStore) readAll() ([]ledgerEvent, error) {
	info, err := s.root.Lstat("ledger.jsonl")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, newError(ErrorCodeConflict, "workspace change ledger path is not a regular file", map[string]any{"path": s.ledgerPath})
	}
	file, err := s.root.OpenFile("ledger.jsonl", os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), maxLedgerEventBytes+1)
	scanner.Split(splitLedgerRecords)
	var events []ledgerEvent
	line := 0
	completeOffset := int64(0)
	tornTail := false
	for scanner.Scan() {
		line++
		record := scanner.Bytes()
		complete := len(record) > 0 && record[len(record)-1] == '\n'
		if !complete {
			tornTail = len(record) > 0
			break
		}
		completeOffset += int64(len(record))
		data := strings.TrimSpace(string(record[:len(record)-1]))
		if data == "" {
			continue
		}
		var event ledgerEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, fmt.Errorf("decode change ledger line %d: %w", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if tornTail {
		if err := file.Truncate(completeOffset); err != nil {
			return nil, fmt.Errorf("truncate torn workspace change ledger tail: %w", err)
		}
		if err := file.Sync(); err != nil {
			return nil, fmt.Errorf("sync repaired workspace change ledger: %w", err)
		}
		if err := s.durability.syncRootDir(s.root, "."); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func splitLedgerRecords(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		return index + 1, data[:index+1], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func (s *eventStore) writeBlob(content []byte) (string, error) {
	revision := Revision(content)
	name := strings.TrimPrefix(revision, "sha256:")
	blobRoot, err := s.openBlobRoot()
	if err != nil {
		return "", err
	}
	defer blobRoot.Close()
	if info, statErr := blobRoot.Lstat(name); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", newError(ErrorCodeConflict, "workspace change blob path is not a regular file", map[string]any{"path": name})
		}
		existing, err := blobRoot.ReadFile(name)
		if err != nil {
			return "", err
		}
		if Revision(existing) != revision {
			return "", fmt.Errorf("workspace change blob checksum mismatch for %q", revision)
		}
		if err := s.durability.syncRootDir(blobRoot, "."); err != nil {
			return "", err
		}
		return revision, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}
	var random [12]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return "", err
	}
	tempName := fmt.Sprintf(".blob-%x", random[:])
	temp, err := blobRoot.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = blobRoot.Remove(tempName)
		}
	}()
	if _, err := temp.Write(content); err != nil {
		return "", err
	}
	if err := temp.Sync(); err != nil {
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := blobRoot.Rename(tempName, name); err != nil {
		info, statErr := blobRoot.Lstat(name)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", err
		}
		existing, readErr := blobRoot.ReadFile(name)
		if readErr != nil || Revision(existing) != revision {
			return "", errors.Join(err, readErr)
		}
	}
	removeTemp = false
	if err := s.durability.syncRootDir(blobRoot, "."); err != nil {
		return "", err
	}
	return revision, nil
}

func (s *eventStore) readBlob(reference string) ([]byte, error) {
	name := strings.TrimPrefix(strings.TrimSpace(reference), "sha256:")
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return nil, fmt.Errorf("invalid blob reference %q", reference)
	}
	blobRoot, err := s.openBlobRoot()
	if err != nil {
		return nil, err
	}
	defer blobRoot.Close()
	info, err := blobRoot.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, newError(ErrorCodeConflict, "workspace change blob path is not a regular file", map[string]any{"path": name})
	}
	content, err := blobRoot.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if actual := Revision(content); actual != "sha256:"+name {
		return nil, fmt.Errorf("workspace change blob checksum mismatch: reference=%q actual=%q", reference, actual)
	}
	return content, nil
}
