// Package projectfiles owns explicit Project-scoped file browsing and editing.
// It never changes the foreground Writing workspace: every operation resolves
// one stable Project ID to its current content directory and state root.
package projectfiles

import "time"

type EntryType string

const (
	EntryFile      EntryType = "file"
	EntryDirectory EntryType = "dir"
)

// Entry is one immediate child of a listed directory. Directories are loaded
// lazily by callers, so children are deliberately absent from this contract.
type Entry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Type       EntryType `json:"type"`
	Size       int64     `json:"size,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
	Ignored    bool      `json:"ignored,omitempty"`
	Symlink    bool      `json:"symlink,omitempty"`
}

type Directory struct {
	ProjectID string  `json:"project_id"`
	Path      string  `json:"path"`
	Entries   []Entry `json:"entries"`
}

type DocumentKind string

const (
	DocumentText   DocumentKind = "text"
	DocumentImage  DocumentKind = "image"
	DocumentBinary DocumentKind = "binary"
)

type Document struct {
	ProjectID string       `json:"project_id"`
	Path      string       `json:"path"`
	Content   string       `json:"content,omitempty"`
	Revision  string       `json:"revision"`
	Kind      DocumentKind `json:"kind"`
	MIMEType  string       `json:"mime_type"`
	Size      int          `json:"size"`
	Editable  bool         `json:"editable"`
}

type SaveRequest struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	BaseRevision string `json:"base_revision"`
}

type SaveResult struct {
	ProjectID string `json:"project_id"`
	Path      string `json:"path"`
	Revision  string `json:"revision"`
	Changed   bool   `json:"changed"`
}

type OperationKind string

const (
	OperationCreate OperationKind = "create"
	OperationDelete OperationKind = "delete"
	OperationRename OperationKind = "rename"
	OperationCopy   OperationKind = "copy"
	OperationMove   OperationKind = "move"
)

// Operation is intentionally tolerant: fields irrelevant to a kind are
// ignored, while each malformed item fails independently from the rest.
type Operation struct {
	ID      string        `json:"id,omitempty"`
	Kind    OperationKind `json:"kind"`
	Path    string        `json:"path"`
	Type    string        `json:"type,omitempty"`
	Content string        `json:"content,omitempty"`
	NewName string        `json:"new_name,omitempty"`
	To      string        `json:"to,omitempty"`
}

type OperationResult struct {
	ID    string        `json:"id,omitempty"`
	Kind  OperationKind `json:"kind"`
	OK    bool          `json:"ok"`
	Path  string        `json:"path,omitempty"`
	Code  string        `json:"code,omitempty"`
	Error string        `json:"error,omitempty"`
}
