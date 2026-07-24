package documentreview

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/workspacepath"
)

const (
	eventThreadCreated    = "thread_created"
	eventCommentsUpserted = "comments_upserted"
	maxLedgerEventBytes   = 2 * 1024 * 1024
)

type ledgerEvent struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Thread    *Thread   `json:"thread,omitempty"`
	Comments  []Comment `json:"comments,omitempty"`
}

type eventStore struct {
	ledgerPath string
	root       *os.Root
}

func newEventStore(workspace string) (*eventStore, error) {
	workspaceRoot, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, err
	}
	defer workspaceRoot.Close()

	rel := workspacepath.Rel(workspace, "reviews")
	if err := ensurePrivateDirectory(workspaceRoot, rel); err != nil {
		return nil, err
	}
	reviewRoot, err := workspaceRoot.OpenRoot(filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	store := &eventStore{
		ledgerPath: filepath.Join(workspace, filepath.FromSlash(rel), "ledger.jsonl"),
		root:       reviewRoot,
	}
	if err := store.ensureLedger(); err != nil {
		reviewRoot.Close()
		return nil, err
	}
	return store, nil
}

func ensurePrivateDirectory(root *os.Root, rel string) error {
	current := "."
	for _, component := range strings.Split(filepath.ToSlash(rel), "/") {
		if component == "" || component == "." {
			continue
		}
		next := path.Join(current, component)
		info, err := root.Lstat(filepath.FromSlash(next))
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(filepath.FromSlash(next), 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = root.Lstat(filepath.FromSlash(next))
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return newError(ErrorCodeConflict, "document review storage path is not a private directory", map[string]any{"path": next})
		}
		if err := syncRootDirectory(root, next); err != nil {
			return err
		}
		if err := syncRootDirectory(root, current); err != nil {
			return err
		}
		current = next
	}
	return nil
}

func (s *eventStore) ensureLedger() error {
	if info, err := s.root.Lstat("ledger.jsonl"); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return newError(ErrorCodeConflict, "document review ledger is not a regular file", map[string]any{"path": s.ledgerPath})
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := s.root.OpenFile("ledger.jsonl", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if statErr == nil && !info.Mode().IsRegular() {
		statErr = newError(ErrorCodeConflict, "document review ledger is not a regular file", nil)
	}
	if statErr == nil {
		statErr = file.Sync()
	}
	closeErr := file.Close()
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	return syncRootDirectory(s.root, ".")
}

func (s *eventStore) append(event ledgerEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if len(encoded)+1 > maxLedgerEventBytes {
		return newError(ErrorCodeInvalid, "document review ledger event is too large", map[string]any{"max_bytes": maxLedgerEventBytes})
	}
	info, err := s.root.Lstat("ledger.jsonl")
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return newError(ErrorCodeConflict, "document review ledger is not a regular file", map[string]any{"path": s.ledgerPath})
	}
	originalSize := info.Size()
	file, err := s.root.OpenFile("ledger.jsonl", os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	payload := append(encoded, '\n')
	_, writeErr := file.Seek(originalSize, io.SeekStart)
	if writeErr == nil {
		var written int
		written, writeErr = file.Write(payload)
		if writeErr == nil && written != len(payload) {
			writeErr = io.ErrShortWrite
		}
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if writeErr != nil {
		rollbackErr := file.Truncate(originalSize)
		if rollbackErr == nil {
			rollbackErr = file.Sync()
		}
		closeErr := file.Close()
		return errors.Join(writeErr, rollbackErr, closeErr)
	}
	closeErr := file.Close()
	return closeErr
}

func (s *eventStore) readAll() ([]ledgerEvent, error) {
	info, err := s.root.Lstat("ledger.jsonl")
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, newError(ErrorCodeConflict, "document review ledger is not a regular file", map[string]any{"path": s.ledgerPath})
	}
	file, err := s.root.OpenFile("ledger.jsonl", os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := bufio.NewReaderSize(file, 64*1024)
	events := make([]ledgerEvent, 0)
	completeBytes := int64(0)
	lineNumber := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > maxLedgerEventBytes {
			return nil, fmt.Errorf("document review ledger line exceeds %d bytes", maxLedgerEventBytes)
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, readErr
		}
		if errors.Is(readErr, io.EOF) {
			if len(line) > 0 {
				if err := file.Truncate(completeBytes); err != nil {
					return nil, fmt.Errorf("truncate torn document review ledger tail: %w", err)
				}
				if err := file.Sync(); err != nil {
					return nil, fmt.Errorf("sync repaired document review ledger: %w", err)
				}
			}
			break
		}
		completeBytes += int64(len(line))
		lineNumber++
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event ledgerEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("decode document review ledger line %d: %w", lineNumber, err)
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *eventStore) close() {
	if s != nil && s.root != nil {
		_ = s.root.Close()
	}
}

func syncRootDirectory(root *os.Root, rel string) error {
	if rel == "" {
		rel = "."
	}
	dir, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return err
	}
	defer dir.Close()
	return syncDirectory(dir)
}
