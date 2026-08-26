package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
		current, err := store.current()
		if err != nil {
			return err
		}
		snapshot = current
		return nil
	})
	return snapshot, err
}

func (store *Store) Update(ctx context.Context, changes ChangeSet) (Result, error) {
	return store.update(ctx, changes, true)
}

// Write atomically applies a management change without invoking the State
// schema validator. Path and revision safety still apply. This supports live
// workspaces where consumers, rather than editors, accept or reject the whole
// snapshot.
func (store *Store) Write(ctx context.Context, changes ChangeSet) (Result, error) {
	return store.update(ctx, changes, false)
}

func (store *Store) update(ctx context.Context, changes ChangeSet, validate bool) (Result, error) {
	if diagnostics := ValidateChanges(changes.Changes); len(diagnostics) != 0 {
		return Result{}, &ValidationError{Diagnostics: diagnostics}
	}
	var result Result
	err := store.withLock(ctx, func() error {
		if err := store.recoverTransaction(); err != nil {
			return err
		}
		base, err := store.current()
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
		if validate {
			if err := store.validate(ctx, candidate); err != nil {
				return err
			}
		}
		result, err = store.apply(base, candidate)
		return err
	})
	return result, err
}

func (store *Store) validate(ctx context.Context, snapshot Snapshot) error {
	if store.validator == nil {
		return nil
	}
	diagnostics := store.validator.Validate(ctx, Snapshot{Revision: snapshot.Revision, files: cloneFiles(snapshot.files)})
	if len(diagnostics) == 0 {
		return nil
	}
	return &ValidationError{Diagnostics: append([]Diagnostic(nil), diagnostics...)}
}

func (store *Store) current() (Snapshot, error) {
	files, err := scanFiles(store.root)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFromFiles(files), nil
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

func validRevision(revision string) bool {
	revision = strings.TrimSpace(revision)
	if len(revision) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}
