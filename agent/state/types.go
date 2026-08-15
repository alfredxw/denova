// Package state manages the live, file-backed Agent harness directory.
//
// The directory is always the source of truth. Revision hashes exist only as
// optimistic-concurrency tokens for management writes; runtime Agents never
// receive or restore a revisioned State snapshot. Product applications own
// history (Git, database, or remote service) independently.
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
	// RuntimeRoot stores only locks, rollback snapshots, and crash-recovery
	// markers. It must be outside Root so an application can independently
	// version the visible State directory.
	RuntimeRoot string
	Validator   Validator
}

// File is one immutable State file. Content is defensively copied at every
// module seam.
type File struct {
	Path    string `json:"path"`
	Content []byte `json:"-"`
}

// Snapshot is an immutable view of all current State files. Revision is a
// content-derived management token and is never injected into an Agent.
type Snapshot struct {
	Revision string `json:"revision"`
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
// applied atomically to the live directory as one complete candidate.
type Change struct {
	Path    string
	Content []byte
	Delete  bool
}

type ChangeSet struct {
	BaseRevision string
	Changes      []Change
}

// ValidateChanges reports every independent path or duplicate error before a
// caller attempts an atomic update. Schema-specific file validation remains
// owned by the Store validator after the complete candidate is assembled.
func ValidateChanges(changes []Change) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	seen := make(map[string]int, len(changes))
	for index, change := range changes {
		path, err := normalizePath(change.Path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "invalid_change_path", Path: change.Path,
				Message: fmt.Sprintf("State change %d has an invalid relative path", index),
			})
			continue
		}
		if previous, duplicate := seen[path]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "duplicate_change_path", Path: path,
				Message: fmt.Sprintf("State changes %d and %d target the same path", previous, index),
			})
			continue
		}
		seen[path] = index
	}
	return diagnostics
}

type Result struct {
	Snapshot Snapshot `json:"snapshot"`
	Changed  bool     `json:"changed"`

	// CleanupError reports non-fatal housekeeping after a State update has
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
