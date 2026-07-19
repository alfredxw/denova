// Package revisionfile serializes revisioned file mutations by canonical path
// and commits their bytes with an atomic, durable replacement.
package revisionfile

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"

	"denova/internal/keyedlock"
)

const MissingRevision = "missing"

// ErrRevisionConflict identifies a compare-and-swap write whose base revision
// no longer matches the exact bytes on disk.
var ErrRevisionConflict = errors.New("file revision conflict")

var mutationLocks = keyedlock.New(canonicalPath)

// Options controls permissions used when a mutation creates a new path.
// Existing file permissions are preserved.
type Options struct {
	FileMode      os.FileMode
	DirectoryMode os.FileMode
}

// Snapshot is the exact byte state observed under the path mutation lock.
type Snapshot struct {
	Content  []byte
	Revision string
	Exists   bool
	Mode     os.FileMode
}

// Result describes the committed state after a mutation.
type Result struct {
	Revision string
	Changed  bool
}

// Mutator derives replacement bytes from the latest locked snapshot.
type Mutator func(Snapshot) ([]byte, error)

// Revision returns the stable content-addressed revision used by file CAS.
func Revision(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", sum[:])
}

// ConflictError reports both sides of a failed compare-and-swap operation.
type ConflictError struct {
	Path     string
	Expected string
	Actual   string
}

func (e *ConflictError) Error() string {
	if e == nil {
		return ErrRevisionConflict.Error()
	}
	return fmt.Sprintf("%s: path=%s expected=%s actual=%s", ErrRevisionConflict, e.Path, e.Expected, e.Actual)
}

func (e *ConflictError) Unwrap() error {
	return ErrRevisionConflict
}

// Read returns one exact snapshot while excluding in-process mutations of the
// same canonical path.
func Read(ctx context.Context, path string) (Snapshot, error) {
	if strings.TrimSpace(path) == "" {
		return Snapshot{}, errors.New("revision file path is empty")
	}
	ctx = nonNilContext(ctx)
	unlock := mutationLocks.Lock(path)
	defer unlock()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return readSnapshot(path)
}

// Mutate holds the canonical path lock across read, domain transformation and
// atomic replacement. The callback must not call Read or Mutate for this path.
func Mutate(ctx context.Context, path string, options Options, mutate Mutator) (Result, error) {
	if strings.TrimSpace(path) == "" {
		return Result{}, errors.New("revision file path is empty")
	}
	if mutate == nil {
		return Result{}, errors.New("revision file mutator is nil")
	}
	ctx = nonNilContext(ctx)
	unlock := mutationLocks.Lock(path)
	defer unlock()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	current, err := readSnapshot(path)
	if err != nil {
		return Result{}, err
	}
	next, err := mutate(current)
	if err != nil {
		return Result{}, err
	}
	if current.Exists && bytes.Equal(current.Content, next) {
		return Result{Revision: current.Revision, Changed: false}, nil
	}
	if err := atomicReplace(path, next, mutationMode(current, options), directoryMode(options)); err != nil {
		return Result{}, err
	}
	return Result{Revision: Revision(next), Changed: true}, nil
}

// ReplaceIfRevision atomically replaces a file when expectedRevision still
// matches. An empty expected revision intentionally performs a blind write.
func ReplaceIfRevision(ctx context.Context, path, expectedRevision string, content []byte, options Options) (Result, error) {
	return Mutate(ctx, path, options, func(current Snapshot) ([]byte, error) {
		if expectedRevision != "" && current.Revision != expectedRevision {
			return nil, &ConflictError{
				Path:     path,
				Expected: expectedRevision,
				Actual:   current.Revision,
			}
		}
		return content, nil
	})
}

func readSnapshot(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Snapshot{Revision: MissingRevision}, nil
	}
	if err != nil {
		return Snapshot{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Snapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return Snapshot{}, fmt.Errorf("revision file path is not a regular file: %s", path)
	}
	return Snapshot{
		Content:  data,
		Revision: Revision(data),
		Exists:   true,
		Mode:     info.Mode(),
	}, nil
}

func mutationMode(current Snapshot, options Options) os.FileMode {
	if current.Exists {
		return current.Mode.Perm()
	}
	if options.FileMode != 0 {
		return options.FileMode.Perm()
	}
	return 0o644
}

func directoryMode(options Options) os.FileMode {
	if options.DirectoryMode != 0 {
		return options.DirectoryMode.Perm()
	}
	return 0o755
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
