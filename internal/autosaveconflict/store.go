package autosaveconflict

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store appends immutable autosave conflict records under one Denova data dir.
type Store struct {
	dataDir string
}

// NewStore creates an append-only conflict store. Filesystem state is created
// lazily only after an input has passed validation.
func NewStore(dataDir string) *Store {
	return &Store{dataDir: strings.TrimSpace(dataDir)}
}

// Append durably records both sides of a conflict without deriving filenames
// from user-controlled resource identifiers.
func (s *Store) Append(ctx context.Context, input Input) (AppendResult, error) {
	if err := ctx.Err(); err != nil {
		return AppendResult{}, err
	}
	input = normalizeInput(input)
	if err := validateInput(s.dataDir, input); err != nil {
		return AppendResult{}, err
	}

	recordID, err := newRecordID()
	if err != nil {
		return AppendResult{}, fmt.Errorf("generate autosave conflict id: %w", err)
	}
	record := Record{
		Version:       recordVersion,
		ID:            recordID,
		CreatedAt:     time.Now().UTC(),
		Resource:      input.Resource,
		Scope:         input.Scope,
		ResourceID:    input.ID,
		Base:          cloneSnapshot(input.Base),
		Local:         cloneSnapshot(input.Local),
		External:      cloneSnapshot(input.External),
		Merged:        cloneSnapshot(input.Merged),
		Strategy:      input.Strategy,
		ConflictPaths: cloneConflictPaths(input.ConflictPaths),
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return AppendResult{}, fmt.Errorf("encode autosave conflict: %w", err)
	}
	data = append(data, '\n')

	root, conflictRoot, err := openConflictRoot(s.dataDir)
	if err != nil {
		return AppendResult{}, err
	}
	defer root.Close()
	defer conflictRoot.Close()

	name := record.ID + ".json"
	if err := writeAtomicRecord(ctx, conflictRoot, name, data); err != nil {
		return AppendResult{}, err
	}
	return AppendResult{
		Record: record,
		Path:   filepath.Join(s.dataDir, DirectoryName, name),
	}, nil
}

func normalizeInput(input Input) Input {
	input.Resource = strings.TrimSpace(input.Resource)
	input.Scope = strings.TrimSpace(input.Scope)
	input.ID = strings.TrimSpace(input.ID)
	input.Strategy = strings.TrimSpace(input.Strategy)
	input.Base.Revision = strings.TrimSpace(input.Base.Revision)
	input.Local.Revision = strings.TrimSpace(input.Local.Revision)
	input.External.Revision = strings.TrimSpace(input.External.Revision)
	input.Merged.Revision = strings.TrimSpace(input.Merged.Revision)
	return input
}

func validateInput(dataDir string, input Input) error {
	if strings.TrimSpace(dataDir) == "" {
		return fmt.Errorf("%w: data directory is required", ErrInvalidInput)
	}
	for field, value := range map[string]string{
		"resource": input.Resource,
		"scope":    input.Scope,
		"id":       input.ID,
		"strategy": input.Strategy,
	} {
		if value == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidInput, field)
		}
	}
	for field, snapshot := range map[string]Snapshot{
		"base":     input.Base,
		"local":    input.Local,
		"external": input.External,
		"merged":   input.Merged,
	} {
		if len(snapshot.Value) == 0 || !json.Valid(snapshot.Value) {
			return fmt.Errorf("%w: %s.value must be valid JSON", ErrInvalidInput, field)
		}
	}
	if len(input.ConflictPaths) == 0 {
		return fmt.Errorf("%w: conflict_paths must contain at least one path", ErrInvalidInput)
	}
	return nil
}

func newRecordID() (string, error) {
	var random [16]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return "", err
	}
	return "autosave-conflict-" + hex.EncodeToString(random[:]), nil
}

func openConflictRoot(dataDir string) (*os.Root, *os.Root, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create Denova data directory: %w", err)
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open Denova data directory: %w", err)
	}
	closeRoot := true
	defer func() {
		if closeRoot {
			_ = root.Close()
		}
	}()

	info, statErr := root.Lstat(DirectoryName)
	if errors.Is(statErr, os.ErrNotExist) {
		if mkdirErr := root.Mkdir(DirectoryName, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
			return nil, nil, fmt.Errorf("create autosave conflict directory: %w", mkdirErr)
		}
		info, statErr = root.Lstat(DirectoryName)
	}
	if statErr != nil {
		return nil, nil, fmt.Errorf("inspect autosave conflict directory: %w", statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, nil, fmt.Errorf("autosave conflict path is not a real directory")
	}
	if err := root.Chmod(DirectoryName, 0o700); err != nil {
		return nil, nil, fmt.Errorf("secure autosave conflict directory: %w", err)
	}
	dataDirectory, err := root.Open(".")
	if err != nil {
		return nil, nil, fmt.Errorf("open Denova data directory for sync: %w", err)
	}
	if syncErr := syncDirectory(dataDirectory); syncErr != nil {
		_ = dataDirectory.Close()
		return nil, nil, fmt.Errorf("sync Denova data directory: %w", syncErr)
	}
	if err := dataDirectory.Close(); err != nil {
		return nil, nil, fmt.Errorf("close Denova data directory after sync: %w", err)
	}
	conflictRoot, err := root.OpenRoot(DirectoryName)
	if err != nil {
		return nil, nil, fmt.Errorf("open autosave conflict directory: %w", err)
	}
	closeRoot = false
	return root, conflictRoot, nil
}

func writeAtomicRecord(ctx context.Context, root *os.Root, name string, data []byte) (err error) {
	var random [12]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return fmt.Errorf("generate autosave conflict temp name: %w", err)
	}
	tempName := ".autosave-conflict-" + hex.EncodeToString(random[:]) + ".tmp"
	file, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create autosave conflict temp file: %w", err)
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = root.Remove(tempName)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("secure autosave conflict temp file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write autosave conflict temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync autosave conflict temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close autosave conflict temp file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := root.Lstat(name); err == nil {
		return fmt.Errorf("autosave conflict record already exists: %s", name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect autosave conflict target: %w", err)
	}
	if err := root.Rename(tempName, name); err != nil {
		return fmt.Errorf("commit autosave conflict record: %w", err)
	}
	removeTemp = false
	directory, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open autosave conflict directory for sync: %w", err)
	}
	defer directory.Close()
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync autosave conflict directory: %w", err)
	}
	return nil
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	return Snapshot{
		Revision: snapshot.Revision,
		Value:    append(json.RawMessage(nil), snapshot.Value...),
	}
}

func cloneConflictPaths(paths [][]string) [][]string {
	cloned := make([][]string, len(paths))
	for index, path := range paths {
		cloned[index] = append([]string(nil), path...)
	}
	return cloned
}
