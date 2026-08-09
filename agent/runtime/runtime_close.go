package runtime

import (
	"context"
	"errors"
	"fmt"
)

// CloseBinding evicts one command lane after durably aborting any active run.
func (r *Runtime) CloseBinding(ctx context.Context, binding BindingRef) error {
	if r == nil {
		return ErrInvalidBinding
	}
	ref := binding.Clone()
	if err := ValidateBindingRef(ref); err != nil {
		return err
	}
	key := bindingJournalKey(ref)
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if scope := r.matchingScopeCloseLocked(ref); scope != nil {
		r.mu.Unlock()
		return waitScopeClose(ctx, nil, scope)
	}
	if pending := r.closing[key]; pending != nil {
		r.mu.Unlock()
		return waitCloseCall(ctx, nil, pending)
	}
	pending := &closeCall{ready: make(chan struct{}), ref: ref}
	r.closing[key] = pending
	r.mu.Unlock()
	go r.finishCloseBinding(key, pending)
	return waitCloseCall(ctx, nil, pending)
}

func (r *Runtime) finishCloseBinding(key string, pending *closeCall) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil && !completed {
			r.mu.Lock()
			if r.closing[key] == pending {
				delete(r.closing, key)
				pending.err = ownerPanicError("close binding", recovered)
				close(pending.ready)
			}
			r.mu.Unlock()
		}
	}()
	var closeErr error
	for {
		r.mu.Lock()
		opening := r.opening[key]
		projection := r.projecting[key]
		if opening == nil && projection == nil {
			h := r.harness[key]
			delete(r.harness, key)
			delete(r.access, key)
			r.mu.Unlock()
			if h != nil {
				closeErr = safeHarnessClose(h)
			}
			break
		}
		r.mu.Unlock()
		if opening != nil {
			<-opening.ready
		}
		if projection != nil {
			<-projection.ready
		}
	}
	r.mu.Lock()
	pending.err = closeErr
	delete(r.closing, key)
	close(pending.ready)
	completed = true
	r.mu.Unlock()
}

// CloseBindings atomically fences and evicts every binding matching selector.
// The fence is held until pre-existing Open calls finish and every matching
// actor has durably terminated and released its journal lease. ctx only controls
// how long this caller waits; cancellation never cancels the owner close.
func (r *Runtime) CloseBindings(ctx context.Context, selector BindingSelector) error {
	if r == nil {
		return ErrInvalidBinding
	}
	selector = selector.clone()
	if err := validateBindingSelector(selector); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		r.mu.Lock()
		// Scope closes are deliberately serialized. This makes every caller a
		// real barrier even when two selectors overlap, without cyclic ownership
		// between independently running close goroutines.
		var prior *scopeCloseCall
		for _, scope := range r.scopes {
			prior = scope
			break
		}
		if prior != nil {
			r.mu.Unlock()
			if err := waitScopeClose(ctx, nil, prior); err != nil {
				return err
			}
			continue
		}
		r.nextScope++
		id := r.nextScope
		pending := &scopeCloseCall{ready: make(chan struct{}), selector: selector}
		r.scopes[id] = pending
		r.mu.Unlock()
		go r.finishCloseBindings(id, pending)
		return waitScopeClose(ctx, nil, pending)
	}
}

func (r *Runtime) finishCloseBindings(id uint64, pending *scopeCloseCall) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil && !completed {
			r.mu.Lock()
			if r.scopes[id] == pending {
				delete(r.scopes, id)
				pending.err = ownerPanicError("close binding scope", recovered)
				close(pending.ready)
			}
			r.mu.Unlock()
		}
	}()
	var firstErr error
	for {
		r.mu.Lock()
		openings := make([]*openCall, 0)
		for _, opening := range r.opening {
			if pending.selector.matches(opening.ref) {
				openings = append(openings, opening)
			}
		}
		projections := make([]*projectCall, 0)
		for _, projection := range r.projecting {
			if pending.selector.matches(projection.ref) {
				projections = append(projections, projection)
			}
		}
		if len(openings) > 0 || len(projections) > 0 {
			r.mu.Unlock()
			for _, opening := range openings {
				<-opening.ready
			}
			for _, projection := range projections {
				<-projection.ready
			}
			continue
		}
		harnesses := make([]*Harness, 0)
		for key, h := range r.harness {
			if pending.selector.matches(h.binding) {
				harnesses = append(harnesses, h)
				delete(r.harness, key)
				delete(r.access, key)
			}
		}
		closings := make([]*closeCall, 0)
		for _, closing := range r.closing {
			if pending.selector.matches(closing.ref) {
				closings = append(closings, closing)
			}
		}
		r.mu.Unlock()
		for _, h := range harnesses {
			if err := safeHarnessClose(h); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		for _, closing := range closings {
			<-closing.ready
			if closing.err != nil && firstErr == nil {
				firstErr = closing.err
			}
		}
		break
	}
	r.mu.Lock()
	pending.err = firstErr
	delete(r.scopes, id)
	close(pending.ready)
	completed = true
	r.mu.Unlock()
}

func (r *Runtime) matchingScopeCloseLocked(ref BindingRef) *scopeCloseCall {
	for _, pending := range r.scopes {
		if pending.selector.matches(ref) {
			return pending
		}
	}
	return nil
}

func waitCloseCall(ctx, lifecycle context.Context, pending *closeCall) error {
	select {
	case <-pending.ready:
		return pending.err
	case <-ctx.Done():
		return ctx.Err()
	case <-contextDone(lifecycle):
		return ErrRuntimeClosed
	}
}

func waitScopeClose(ctx, lifecycle context.Context, pending *scopeCloseCall) error {
	select {
	case <-pending.ready:
		return pending.err
	case <-ctx.Done():
		return ctx.Err()
	case <-contextDone(lifecycle):
		return ErrRuntimeClosed
	}
}

func contextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

// Close terminates every binding owned by the Runtime. It has no implicit
// deadline; the caller controls cancellation through ctx.
func (r *Runtime) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		harnesses := make([]*Harness, 0, len(r.harness))
		for _, h := range r.harness {
			harnesses = append(harnesses, h)
		}
		openings := make([]*openCall, 0, len(r.opening))
		for _, pending := range r.opening {
			openings = append(openings, pending)
		}
		projections := make([]*projectCall, 0, len(r.projecting))
		for _, pending := range r.projecting {
			projections = append(projections, pending)
		}
		closings := make([]*closeCall, 0, len(r.closing))
		for _, pending := range r.closing {
			closings = append(closings, pending)
		}
		scopes := make([]*scopeCloseCall, 0, len(r.scopes))
		for _, pending := range r.scopes {
			scopes = append(scopes, pending)
		}
		go r.finishClose(harnesses, openings, projections, closings, scopes)
	}
	done := r.done
	r.mu.Unlock()
	select {
	case <-done:
		r.mu.Lock()
		err := r.closeErr
		r.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) finishClose(harnesses []*Harness, openings []*openCall, projections []*projectCall, closings []*closeCall, scopes []*scopeCloseCall) {
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil && !completed {
			r.cancel()
			r.mu.Lock()
			r.closeErr = ownerPanicError("close runtime", recovered)
			close(r.done)
			r.mu.Unlock()
		}
	}()
	var firstErr error
	closeWaiters := make([]<-chan error, 0, len(harnesses))
	for _, h := range harnesses {
		waiter, err := h.admitCloseFence()
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if waiter != nil {
			closeWaiters = append(closeWaiters, waiter)
		}
	}
	// Every actor that existed at the Runtime.Close linearization point now has
	// state.closing installed. Lifecycle cancellation may wake engine goroutines,
	// but their completion can no longer start a queued NextTurn successor.
	r.cancel()
	for _, waiter := range closeWaiters {
		if err := <-waiter; err != nil && !errors.Is(err, ErrHarnessClosed) && firstErr == nil {
			firstErr = err
		}
	}
	for _, pending := range openings {
		<-pending.ready
		if pending.err != nil && !errors.Is(pending.err, ErrRuntimeClosed) && !errors.Is(pending.err, context.Canceled) && firstErr == nil {
			firstErr = pending.err
		}
	}
	for _, pending := range projections {
		<-pending.ready
		if pending.err != nil && !errors.Is(pending.err, ErrRuntimeClosed) && !errors.Is(pending.err, context.Canceled) && firstErr == nil {
			firstErr = pending.err
		}
	}
	for _, pending := range closings {
		<-pending.ready
		if pending.err != nil && firstErr == nil {
			firstErr = pending.err
		}
	}
	for _, pending := range scopes {
		<-pending.ready
		if pending.err != nil && firstErr == nil {
			firstErr = pending.err
		}
	}
	r.mu.Lock()
	r.closeErr = firstErr
	close(r.done)
	completed = true
	r.mu.Unlock()
}

func safeHarnessClose(h *Harness) (err error) {
	if h == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = ownerPanicError("close harness", recovered)
		}
	}()
	return h.Close(context.Background())
}

func ownerPanicError(scope string, recovered any) error {
	return fmt.Errorf("%w: runtime owner panic while attempting to %s: %v", ErrHarnessFailed, scope, recovered)
}
