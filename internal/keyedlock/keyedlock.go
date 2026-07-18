// Package keyedlock provides a reference-counted lock keyed by a caller-supplied
// string. It is meant for protecting shared resources (files, workspaces) that
// are accessed through short-lived helpers where a mutex on the helper itself
// would not serialize concurrent read-modify-write cycles.
//
// Locks are removed from the internal map once their reference count drops to
// zero, so long-running processes do not accumulate entries for keys that are
// no longer in use.
package keyedlock

import "sync"

// KeyFunc canonicalizes a key before locking. It lets callers collapse
// equivalent inputs (for example symlinked paths) onto a single lock.
type KeyFunc func(string) string

// Lock is a reference-counted, key-addressable mutex. The zero value is not
// usable; use New to construct one.
type Lock struct {
	mu      sync.Mutex
	entries map[string]*entry
	keyFunc KeyFunc
}

type entry struct {
	mu   sync.Mutex
	refs int
}

// New creates a Lock that uses keyFunc to canonicalize keys. If keyFunc is nil,
// keys are used verbatim.
func New(keyFunc KeyFunc) *Lock {
	if keyFunc == nil {
		keyFunc = func(key string) string { return key }
	}
	return &Lock{entries: make(map[string]*entry), keyFunc: keyFunc}
}

// Lock acquires the lock for key and returns an unlock function that must be
// called exactly once. The returned unlock function is nil-safe on a nil
// receiver, but Lock itself is not.
func (l *Lock) Lock(key string) func() {
	key = l.keyFunc(key)
	l.mu.Lock()
	e := l.entries[key]
	if e == nil {
		e = &entry{}
		l.entries[key] = e
	}
	e.refs++
	l.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		l.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}
