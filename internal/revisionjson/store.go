// Package revisionjson persists one JSON document behind content-addressed CAS
// and atomic file replacement. Domain libraries own normalization and
// validation through the codec; callers never perform a separate read/check/
// write sequence.
package revisionjson

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"denova/internal/revisionfile"
)

var (
	ErrRevisionRequired = errors.New("JSON resource revision is required")
	ErrAlreadyExists    = errors.New("JSON resource already exists")
)

// Codec keeps domain normalization and validation inside the owning package
// while Store owns serialization order, CAS, and durable replacement.
type Codec[T any] struct {
	Decode func([]byte) (T, error)
	Encode func(T) ([]byte, error)
}

// Document is the domain value paired with the exact revision of its persisted
// bytes. Revision is never embedded in those bytes.
type Document[T any] struct {
	Value    T
	Revision string
}

// Store is a per-path deep module. Construct it at the domain library seam;
// copying Store is safe because path locking is owned by revisionfile.
type Store[T any] struct {
	path    string
	codec   Codec[T]
	options revisionfile.Options
}

func NewStore[T any](path string, codec Codec[T]) Store[T] {
	return Store[T]{path: path, codec: codec}
}

func (s Store[T]) Read(ctx context.Context) (Document[T], error) {
	if err := s.validate(); err != nil {
		return Document[T]{}, err
	}
	snapshot, err := revisionfile.Read(ctx, s.path)
	if err != nil {
		return Document[T]{}, err
	}
	if !snapshot.Exists {
		return Document[T]{}, &os.PathError{Op: "read", Path: s.path, Err: os.ErrNotExist}
	}
	value, err := s.codec.Decode(snapshot.Content)
	if err != nil {
		return Document[T]{}, err
	}
	return Document[T]{Value: value, Revision: snapshot.Revision}, nil
}

// Create atomically commits value only when the path is still absent.
func (s Store[T]) Create(ctx context.Context, value T) (Document[T], error) {
	return s.mutate(ctx, revisionfile.MissingRevision, false, func(_ T) (T, error) {
		return value, nil
	})
}

// Replace atomically commits value without a revision precondition. It is for
// deterministic built-in materialization and restore paths, never user edits.
func (s Store[T]) Replace(ctx context.Context, value T) (Document[T], error) {
	return s.mutate(ctx, "", false, func(_ T) (T, error) {
		return value, nil
	})
}

// Update requires the caller's exact content revision and runs transform while
// holding the canonical path lock. This makes read/merge/validate/write one CAS
// operation even when UI and Agent callers race.
func (s Store[T]) Update(ctx context.Context, expectedRevision string, transform func(T) (T, error)) (Document[T], error) {
	expectedRevision = strings.TrimSpace(expectedRevision)
	if expectedRevision == "" {
		return Document[T]{}, ErrRevisionRequired
	}
	if transform == nil {
		return Document[T]{}, errors.New("JSON resource transform is nil")
	}
	return s.mutate(ctx, expectedRevision, true, transform)
}

func (s Store[T]) mutate(ctx context.Context, expectedRevision string, requireExisting bool, transform func(T) (T, error)) (Document[T], error) {
	if err := s.validate(); err != nil {
		return Document[T]{}, err
	}
	var next T
	result, err := revisionfile.Mutate(ctx, s.path, s.options, func(snapshot revisionfile.Snapshot) ([]byte, error) {
		if expectedRevision != "" && snapshot.Revision != expectedRevision {
			if expectedRevision == revisionfile.MissingRevision && snapshot.Exists {
				return nil, ErrAlreadyExists
			}
			return nil, &revisionfile.ConflictError{Path: s.path, Expected: expectedRevision, Actual: snapshot.Revision}
		}
		var current T
		if snapshot.Exists {
			decoded, decodeErr := s.codec.Decode(snapshot.Content)
			if decodeErr != nil {
				return nil, decodeErr
			}
			current = decoded
		} else if requireExisting {
			return nil, &os.PathError{Op: "read", Path: s.path, Err: os.ErrNotExist}
		}
		transformed, transformErr := transform(current)
		if transformErr != nil {
			return nil, transformErr
		}
		encoded, encodeErr := s.codec.Encode(transformed)
		if encodeErr != nil {
			return nil, encodeErr
		}
		next = transformed
		return encoded, nil
	})
	if err != nil {
		return Document[T]{}, err
	}
	return Document[T]{Value: next, Revision: result.Revision}, nil
}

func (s Store[T]) validate() error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("JSON resource path is empty")
	}
	if s.codec.Decode == nil || s.codec.Encode == nil {
		return fmt.Errorf("JSON resource codec is incomplete: %s", s.path)
	}
	return nil
}
