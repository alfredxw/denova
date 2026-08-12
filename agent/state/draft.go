package state

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Draft is an isolated filesystem workspace. Callers may bind ordinary
// read/write/edit/search/shell tools to Root and publish exactly once.
type Draft struct {
	store        *Store
	id           string
	root         string
	baseRevision string
	mu           sync.Mutex
	closed       bool
}

func (draft *Draft) ID() string {
	if draft == nil {
		return ""
	}
	return draft.id
}

func (draft *Draft) Root() string {
	if draft == nil {
		return ""
	}
	return draft.root
}

func (draft *Draft) BaseRevision() string {
	if draft == nil {
		return ""
	}
	return draft.baseRevision
}

// Validate checks the complete draft without publishing or closing it. Hosts
// can use this as a completion guard while ordinary file tools are still able
// to repair every reported diagnostic in the same Agent run.
func (draft *Draft) Validate(ctx context.Context) error {
	if draft == nil || draft.store == nil {
		return ErrDraftClosed
	}
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.closed {
		return ErrDraftClosed
	}
	return draft.store.withLock(ctx, func() error {
		if err := draft.store.recoverTransaction(); err != nil {
			return err
		}
		files, err := scanFiles(draft.root)
		if err != nil {
			return err
		}
		return draft.store.validate(ctx, snapshotFromFiles(files))
	})
}

func (draft *Draft) Publish(ctx context.Context) (Result, error) {
	if draft == nil || draft.store == nil {
		return Result{}, ErrDraftClosed
	}
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.closed {
		return Result{}, ErrDraftClosed
	}
	var result Result
	err := draft.store.withLock(ctx, func() error {
		if err := draft.store.recoverTransaction(); err != nil {
			return err
		}
		base, err := draft.store.currentValidated(ctx)
		if err != nil {
			return err
		}
		if base.Revision != draft.baseRevision {
			return fmt.Errorf("%w: expected=%s current=%s", ErrConflict, draft.baseRevision, base.Revision)
		}
		files, err := scanFiles(draft.root)
		if err != nil {
			return err
		}
		candidate := snapshotFromFiles(files)
		if err := draft.store.validate(ctx, candidate); err != nil {
			return err
		}
		result, err = draft.store.publish(base, candidate)
		return err
	})
	if err != nil {
		return Result{}, err
	}
	draft.closed = true
	if err := os.RemoveAll(filepath.Dir(draft.root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Publication is already durable at this point. Returning an operation
		// error would invite a retry and misreport a committed State mutation.
		result.CleanupError = errors.Join(result.CleanupError, fmt.Errorf("remove published Agent state draft: %w", err))
	}
	return result, nil
}

func (draft *Draft) Discard() error {
	if draft == nil {
		return nil
	}
	draft.mu.Lock()
	defer draft.mu.Unlock()
	if draft.closed {
		return nil
	}
	if draft.root == "" {
		draft.closed = true
		return nil
	}
	if err := os.RemoveAll(filepath.Dir(draft.root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("discard Agent state draft: %w", err)
	}
	draft.closed = true
	return nil
}
