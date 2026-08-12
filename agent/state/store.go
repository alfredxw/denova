package state

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

type Store struct {
	root      string
	private   string
	validator Validator
	lock      *flock.Flock
	mu        sync.Mutex
}

func Open(options Options) (*Store, error) {
	root := strings.TrimSpace(options.Root)
	if root == "" {
		return nil, errors.New("Agent state root is required")
	}
	runtimeRoot := strings.TrimSpace(options.RuntimeRoot)
	if runtimeRoot == "" {
		return nil, errors.New("Agent state runtime root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Agent state root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create Agent state root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("canonicalize Agent state root: %w", err)
	}
	runtimeAbsolute, err := filepath.Abs(runtimeRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve Agent state runtime root: %w", err)
	}
	if err := os.MkdirAll(runtimeAbsolute, 0o700); err != nil {
		return nil, fmt.Errorf("create private Agent state directory: %w", err)
	}
	private, err := filepath.EvalSymlinks(runtimeAbsolute)
	if err != nil {
		return nil, fmt.Errorf("canonicalize Agent state runtime root: %w", err)
	}
	if private == canonical || strings.HasPrefix(private, canonical+string(filepath.Separator)) {
		return nil, errors.New("Agent state runtime root must be outside the visible State root")
	}
	store := &Store{
		root: canonical, private: private, validator: options.Validator,
		lock: flock.New(filepath.Join(private, "store.lock")),
	}
	if err := store.withLock(context.Background(), store.recoverTransaction); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *Store) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

func (store *Store) Current(ctx context.Context) (Snapshot, error) {
	var snapshot Snapshot
	err := store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		current, err := store.currentValidated(ctx)
		if err != nil {
			return err
		}
		snapshot = current
		return store.cacheSnapshot(current)
	})
	return snapshot, err
}

// ForRun returns one immutable snapshot per stable Run ID. The private pin is
// durable, so every cycle and cold recovery of that Run sees identical files.
func (store *Store) ForRun(ctx context.Context, runID string) (Snapshot, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Snapshot{}, errors.New("Agent state Run ID is required")
	}
	var result Snapshot
	err := store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		token := stateToken(runID)
		pinPath := filepath.Join(store.private, "runs", token+".json")
		var pin runPin
		if err := readJSON(pinPath, &pin); err == nil {
			snapshot, loadErr := store.loadCachedSnapshot(pin.Revision)
			if loadErr != nil {
				return fmt.Errorf("restore Agent state Run snapshot: %w", loadErr)
			}
			snapshot.Token = token
			result = snapshot
			return store.validate(ctx, snapshot)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		current, err := store.currentValidated(ctx)
		if err != nil {
			return err
		}
		if err := store.cacheSnapshot(current); err != nil {
			return err
		}
		if err := atomicJSON(pinPath, runPin{Revision: current.Revision}); err != nil {
			return fmt.Errorf("pin Agent state Run snapshot: %w", err)
		}
		current.Token = token
		result = current
		return nil
	})
	return result, err
}

// BindRun makes targetRunID restore the snapshot already pinned for
// sourceRunID. Hosts use this when the durable public Run ID becomes available
// only after an accepted command has assembled its State snapshot.
func (store *Store) BindRun(ctx context.Context, targetRunID, sourceRunID string) (found bool, err error) {
	targetRunID = strings.TrimSpace(targetRunID)
	sourceRunID = strings.TrimSpace(sourceRunID)
	if targetRunID == "" || sourceRunID == "" {
		return false, errors.New("Agent state source and target Run IDs are required")
	}
	err = store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		var source runPin
		sourcePath := filepath.Join(store.private, "runs", stateToken(sourceRunID)+".json")
		if readErr := readJSON(sourcePath, &source); errors.Is(readErr, fs.ErrNotExist) {
			return nil
		} else if readErr != nil {
			return readErr
		}
		if _, loadErr := store.loadCachedSnapshot(source.Revision); loadErr != nil {
			return fmt.Errorf("bind Agent state Run snapshot: %w", loadErr)
		}
		targetPath := filepath.Join(store.private, "runs", stateToken(targetRunID)+".json")
		var target runPin
		if readErr := readJSON(targetPath, &target); readErr == nil {
			if target.Revision != source.Revision {
				return fmt.Errorf("%w: target Run already uses a different State revision", ErrConflict)
			}
			found = true
			return nil
		} else if !errors.Is(readErr, fs.ErrNotExist) {
			return readErr
		}
		if writeErr := atomicJSON(targetPath, source); writeErr != nil {
			return fmt.Errorf("bind Agent state Run snapshot: %w", writeErr)
		}
		found = true
		return nil
	})
	return found, err
}

func (store *Store) Update(ctx context.Context, changes ChangeSet) (Result, error) {
	var result Result
	err := store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		base, err := store.currentValidated(ctx)
		if err != nil {
			return err
		}
		if expected := strings.TrimSpace(changes.BaseRevision); expected != "" && expected != base.Revision {
			return fmt.Errorf("%w: expected=%s current=%s", ErrConflict, expected, base.Revision)
		}
		candidate, err := applyChanges(base, changes.Changes)
		if err != nil {
			return err
		}
		if err := store.validate(ctx, candidate); err != nil {
			return err
		}
		result, err = store.publish(base, candidate)
		return err
	})
	return result, err
}

func (store *Store) BeginDraft(ctx context.Context, baseRevision string) (*Draft, error) {
	var draft *Draft
	err := store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		base, err := store.currentValidated(ctx)
		if err != nil {
			return err
		}
		if expected := strings.TrimSpace(baseRevision); expected != "" && expected != base.Revision {
			return fmt.Errorf("%w: expected=%s current=%s", ErrConflict, expected, base.Revision)
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		root := filepath.Join(store.private, "drafts", id, "files")
		if err := copySnapshot(root, base); err != nil {
			return fmt.Errorf("create Agent state draft: %w", err)
		}
		draft = &Draft{store: store, id: id, root: root, baseRevision: base.Revision}
		return nil
	})
	return draft, err
}

// ResumeDraft reopens a durable unpublished draft after process recovery.
func (store *Store) ResumeDraft(ctx context.Context, id, baseRevision string) (*Draft, error) {
	id = strings.TrimSpace(id)
	baseRevision = strings.TrimSpace(baseRevision)
	if len(id) != 32 {
		return nil, ErrInvalidPath
	}
	if _, err := hex.DecodeString(id); err != nil {
		return nil, ErrInvalidPath
	}
	var draft *Draft
	err := store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		root := filepath.Join(store.private, "drafts", id, "files")
		if _, err := scanFiles(root); err != nil {
			return fmt.Errorf("resume Agent state draft: %w", err)
		}
		if !validRevision(baseRevision) {
			return errors.New("Agent state draft base revision is invalid")
		}
		draft = &Draft{store: store, id: id, root: root, baseRevision: baseRevision}
		return nil
	})
	return draft, err
}

func (store *Store) validate(ctx context.Context, snapshot Snapshot) error {
	if store.validator == nil {
		return nil
	}
	diagnostics := store.validator.Validate(ctx, Snapshot{Revision: snapshot.Revision, Token: snapshot.Token, files: cloneFiles(snapshot.files)})
	if len(diagnostics) == 0 {
		return nil
	}
	return &ValidationError{Diagnostics: append([]Diagnostic(nil), diagnostics...)}
}

func (store *Store) currentValidated(ctx context.Context) (Snapshot, error) {
	files, err := scanFiles(store.root)
	if err != nil {
		return Snapshot{}, err
	}
	snapshot := snapshotFromFiles(files)
	if err := store.validate(ctx, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (store *Store) withLock(ctx context.Context, operation func() error) error {
	if store == nil || store.lock == nil {
		return errors.New("Agent state Store is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	locked, err := store.lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire Agent state lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("acquire Agent state lock: %w", context.Canceled)
	}
	defer store.lock.Unlock()
	return operation()
}

func stateToken(runID string) string {
	digest := sha256.Sum256([]byte(runID))
	return hex.EncodeToString(digest[:])
}

func validRevision(revision string) bool {
	revision = strings.TrimSpace(revision)
	if len(revision) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

type runPin struct {
	Revision string `json:"revision"`
}

func randomID() (string, error) {
	content := make([]byte, 16)
	if _, err := rand.Read(content); err != nil {
		return "", fmt.Errorf("create Agent state identity: %w", err)
	}
	return hex.EncodeToString(content), nil
}
