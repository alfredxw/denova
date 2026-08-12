// Package state manages revisioned, file-backed Agent harness state.
//
// Current files remain the source of truth. Draft isolation, crash recovery,
// and Run-scoped snapshots are hidden behind Store. Product applications own
// any history provider (Git, database, or remote service) independently.
package state

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

var (
	ErrConflict    = errors.New("agent state revision conflict")
	ErrInvalidPath = errors.New("agent state path is invalid")
	ErrDraftClosed = errors.New("agent state draft is closed")
)

// Diagnostic is one actionable validation failure. Code and Path are stable
// machine fields; Message is an English developer-facing explanation.
type Diagnostic struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// ValidationError returns every independent diagnostic discovered in one
// validation pass so an Agent can repair several files without costly retries.
type ValidationError struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

func (err *ValidationError) Error() string {
	if err == nil || len(err.Diagnostics) == 0 {
		return "agent state validation failed"
	}
	return fmt.Sprintf("agent state validation failed: %s", err.Diagnostics[0].Message)
}

// Validator defines product-owned file semantics without coupling this module
// to a particular harness schema.
type Validator interface {
	Validate(context.Context, Snapshot) []Diagnostic
}

type ValidatorFunc func(context.Context, Snapshot) []Diagnostic

func (validate ValidatorFunc) Validate(ctx context.Context, snapshot Snapshot) []Diagnostic {
	if validate == nil {
		return nil
	}
	return validate(ctx, snapshot)
}

type Options struct {
	Root string
	// RuntimeRoot stores private Run pins, immutable snapshot cache, drafts,
	// locks, and crash-recovery markers. It must be outside Root so an
	// application can independently version the visible State directory.
	RuntimeRoot string
	Validator   Validator
}

// File is one immutable State file. Content is defensively copied at every
// module seam.
type File struct {
	Path    string `json:"path"`
	Content []byte `json:"-"`
}

// Snapshot is an immutable, validated view of all current State files.
// Revision is content-derived. Token is an opaque Run recovery handle and is
// empty for snapshots that are not pinned to a Run.
type Snapshot struct {
	Revision string `json:"revision"`
	Token    string `json:"token,omitempty"`
	files    []File
}

func (snapshot Snapshot) Files() []File {
	return cloneFiles(snapshot.files)
}

func (snapshot Snapshot) Read(path string) ([]byte, error) {
	path, err := normalizePath(path)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(snapshot.files), func(index int) bool {
		return snapshot.files[index].Path >= path
	})
	if index >= len(snapshot.files) || snapshot.files[index].Path != path {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), snapshot.files[index].Content...), nil
}

func (snapshot Snapshot) Has(path string) bool {
	_, err := snapshot.Read(path)
	return err == nil
}

// RevisionForFiles returns the canonical content revision used by Snapshot.
// It lets application-owned version stores project their records without
// depending on Agent State's private filesystem implementation.
func RevisionForFiles(files []File) string {
	return snapshotFromFiles(files).Revision
}

// Change replaces or removes one file. Delete ignores Content. A ChangeSet is
// published atomically as one complete candidate snapshot.
type Change struct {
	Path    string
	Content []byte
	Delete  bool
}

type ChangeSet struct {
	BaseRevision string
	Changes      []Change
}

type Result struct {
	Snapshot Snapshot `json:"snapshot"`
	Changed  bool     `json:"changed"`

	// CleanupError reports non-fatal housekeeping after a State publication has
	// already committed. Callers must treat the update as successful and may
	// surface the error through operational logging.
	CleanupError error `json:"-"`
}

func cloneFiles(files []File) []File {
	result := make([]File, len(files))
	for index, file := range files {
		result[index] = File{Path: file.Path, Content: append([]byte(nil), file.Content...)}
	}
	return result
}
