package runtime

import (
	"context"
	"errors"
	"fmt"
)

// Project returns a bounded, read-only status projection. If no actor is open,
// it reads and releases the journal lease directly; it never creates an Engine,
// actor, lifecycle watcher, or durable recovery event.
func (r *Runtime) Project(ctx context.Context, binding BindingRef) (StatusSnapshot, error) {
	if r == nil {
		return StatusSnapshot{}, ErrInvalidBinding
	}
	ref := binding.Clone()
	if err := ValidateBindingRef(ref); err != nil {
		return StatusSnapshot{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	key := bindingJournalKey(ref)
	for {
		r.mu.Lock()
		if r.closed {
			r.mu.Unlock()
			return StatusSnapshot{}, ErrRuntimeClosed
		}
		if pending := r.matchingScopeCloseLocked(ref); pending != nil {
			r.mu.Unlock()
			if err := waitScopeClose(ctx, r.ctx, pending); err != nil {
				return StatusSnapshot{}, err
			}
			continue
		}
		if pending := r.closing[key]; pending != nil {
			r.mu.Unlock()
			if err := waitCloseCall(ctx, r.ctx, pending); err != nil {
				return StatusSnapshot{}, err
			}
			continue
		}
		if h := r.harness[key]; h != nil && h.terminalError() == nil {
			r.touchBindingLocked(key)
			r.mu.Unlock()
			return h.Status(ctx)
		}
		if pending := r.opening[key]; pending != nil {
			r.mu.Unlock()
			if _, err := waitOpenCall(ctx, r.ctx, pending); err != nil {
				return StatusSnapshot{}, err
			}
			continue
		}
		if pending := r.projecting[key]; pending != nil {
			r.mu.Unlock()
			return waitProjectCall(ctx, r.ctx, pending)
		}
		pending := &projectCall{ready: make(chan struct{}), ref: ref}
		r.projecting[key] = pending
		r.mu.Unlock()
		go r.finishProject(ref, key, pending)
		return waitProjectCall(ctx, r.ctx, pending)
	}
}

func (r *Runtime) finishProject(ref BindingRef, key string, pending *projectCall) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil && !completed {
			r.mu.Lock()
			if r.projecting[key] == pending {
				delete(r.projecting, key)
				pending.err = ownerPanicError("project binding", recovered)
				close(pending.ready)
			}
			r.mu.Unlock()
		}
	}()
	snapshot, err := r.projectStoredStatus(ref, key)
	r.mu.Lock()
	if r.closed && err == nil {
		err = ErrRuntimeClosed
	}
	delete(r.projecting, key)
	pending.snapshot = snapshot
	pending.err = err
	close(pending.ready)
	completed = true
	r.mu.Unlock()
}

func (r *Runtime) projectStoredStatus(ref BindingRef, key string) (snapshot StatusSnapshot, resultErr error) {
	j, err := r.journals.OpenJournal(r.ctx, key)
	if err != nil {
		return StatusSnapshot{}, fmt.Errorf("open agent journal projection: %w", err)
	}
	journalClosed := false
	defer func() {
		if journalClosed {
			return
		}
		if closeErr := safeJournalClose(j); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("release agent journal projection lease: %w", closeErr)
		}
	}()
	state := newProjectionState(ref)
	state.maxRetainedCommands = r.config.RetainedCommandLimit
	_, replayErr := replayHarnessJournalState(r.ctx, j, &state)
	closeErr := safeJournalClose(j)
	journalClosed = true
	if replayErr != nil || closeErr != nil {
		return StatusSnapshot{}, errors.Join(
			wrapRuntimeError("replay agent journal projection", replayErr),
			wrapRuntimeError("release agent journal projection lease", closeErr),
		)
	}
	return state.conservativeStoredStatus(r.config.ProjectionTextMaxBytes), nil
}

func safeJournalClose(j Journal) (err error) {
	if j == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = ownerPanicError("close journal", recovered)
		}
	}()
	return j.Close()
}

func waitProjectCall(ctx, lifecycle context.Context, pending *projectCall) (StatusSnapshot, error) {
	select {
	case <-pending.ready:
		return cloneStatusSnapshot(pending.snapshot), pending.err
	case <-ctx.Done():
		return StatusSnapshot{}, ctx.Err()
	case <-contextDone(lifecycle):
		return StatusSnapshot{}, ErrRuntimeClosed
	}
}

func wrapRuntimeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}
