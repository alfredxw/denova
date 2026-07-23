// Package lifecycle provides hierarchical admission scopes for long-lived
// application work. Closing a scope first fences new work, then cancels and
// waits for every admitted lease and child scope.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrClosing reports that a scope has fenced new work and is draining.
	ErrClosing = errors.New("lifecycle scope is closing")
	// ErrClosed reports that a scope has finished draining.
	ErrClosed = errors.New("lifecycle scope is closed")
)

type scopeState uint8

const (
	scopeOpen scopeState = iota
	scopeClosing
	scopeClosed
)

// Scope is a hierarchical admission and cancellation boundary. A Lease must
// be released exactly once when its operation has stopped touching resources
// owned by the scope.
type Scope struct {
	name   string
	parent *Scope

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	state    scopeState
	leases   int
	children map[*Scope]struct{}
	done     chan struct{}
	closeOne sync.Once
}

// Lease represents one operation admitted before the scope was fenced.
type Lease struct {
	scope  *Scope
	once   sync.Once
	cancel context.CancelFunc
	stop   func() bool
}

// NewRoot creates an open root scope.
func NewRoot(name string) *Scope {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scope{
		name:     name,
		ctx:      ctx,
		cancel:   cancel,
		children: make(map[*Scope]struct{}),
		done:     make(chan struct{}),
	}
}

// Child creates an open child. Closing the parent closes and waits for it.
func (s *Scope) Child(name string) (*Scope, error) {
	if s == nil {
		return nil, ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.admissionErrorLocked(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(s.ctx)
	child := &Scope{
		name:     name,
		parent:   s,
		ctx:      ctx,
		cancel:   cancel,
		children: make(map[*Scope]struct{}),
		done:     make(chan struct{}),
	}
	s.children[child] = struct{}{}
	return child, nil
}

// Acquire atomically admits one operation.
func (s *Scope) Acquire() (*Lease, error) {
	if s == nil {
		return nil, ErrClosed
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.admissionErrorLocked(); err != nil {
		return nil, err
	}
	s.leases++
	return &Lease{scope: s}, nil
}

// AcquireContext admits one operation and returns a context canceled by
// either the caller or the scope. Releasing the lease also releases the
// derived context resources.
func (s *Scope) AcquireContext(parent context.Context) (context.Context, *Lease, error) {
	if parent == nil {
		parent = context.Background()
	}
	lease, err := s.Acquire()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	lease.cancel = cancel
	lease.stop = context.AfterFunc(s.Context(), cancel)
	return ctx, lease, nil
}

// Context is canceled as soon as scope closing begins. It must not be used as
// evidence that all owners have exited; Wait provides that barrier.
func (s *Scope) Context() context.Context {
	if s == nil {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}
	return s.ctx
}

// Name returns the diagnostic name of the scope.
func (s *Scope) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Release ends an admitted operation. It is idempotent.
func (l *Lease) Release() {
	if l == nil || l.scope == nil {
		return
	}
	l.once.Do(func() {
		if l.stop != nil {
			l.stop()
		}
		if l.cancel != nil {
			l.cancel()
		}
		s := l.scope
		s.mu.Lock()
		if s.leases > 0 {
			s.leases--
		}
		s.tryFinishLocked()
		s.mu.Unlock()
	})
}

// BeginClose synchronously fences new work and cancels the scope. Draining
// proceeds independently, so a caller that stops waiting cannot strand a
// half-closed scope.
func (s *Scope) BeginClose() {
	if s == nil {
		return
	}
	s.closeOne.Do(func() {
		s.mu.Lock()
		s.state = scopeClosing
		s.cancel()
		children := make([]*Scope, 0, len(s.children))
		for child := range s.children {
			children = append(children, child)
		}
		s.tryFinishLocked()
		s.mu.Unlock()

		for _, child := range children {
			child.BeginClose()
		}
	})
}

// Wait waits until every admitted operation and child has exited. The caller
// context only controls waiting; it never reopens or abandons the close.
func (s *Scope) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close fences, cancels, and waits for the full subtree.
func (s *Scope) Close(ctx context.Context) error {
	s.BeginClose()
	return s.Wait(ctx)
}

func (s *Scope) admissionErrorLocked() error {
	switch s.state {
	case scopeClosing:
		return fmt.Errorf("%w: %s", ErrClosing, s.name)
	case scopeClosed:
		return fmt.Errorf("%w: %s", ErrClosed, s.name)
	default:
		return nil
	}
}

func (s *Scope) tryFinishLocked() {
	if s.state != scopeClosing || s.leases != 0 || len(s.children) != 0 {
		return
	}
	s.state = scopeClosed
	close(s.done)
	parent := s.parent
	if parent == nil {
		return
	}
	parent.mu.Lock()
	delete(parent.children, s)
	parent.tryFinishLocked()
	parent.mu.Unlock()
}
