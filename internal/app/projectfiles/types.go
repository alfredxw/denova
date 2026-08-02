// Package projectfiles owns explicit Project-scoped file browsing and editing.
// It never changes the foreground Writing workspace: every operation resolves
// one stable Project ID to its current content directory and state root.
package projectfiles

type EntryType string

const (
	EntryFile      EntryType = "file"
	EntryDirectory EntryType = "dir"
)

// Entry is intentionally metadata-light. Tree resolution is a hot path and the
// explorer does not need a stat call for every visible child.
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Type    EntryType `json:"type"`
	Ignored bool      `json:"ignored,omitempty"`
	Symlink bool      `json:"symlink,omitempty"`
}

type DirectoryChildrenState string

const (
	DirectoryChildrenComplete DirectoryChildrenState = "complete"
	DirectoryChildrenPartial  DirectoryChildrenState = "partial"
)

// DirectoryPage is one stable, bounded page of an immediate directory. A
// continuation is opaque to callers and valid only while Revision is current.
type DirectoryPage struct {
	Path          string                 `json:"path"`
	Revision      string                 `json:"revision"`
	Entries       []Entry                `json:"entries"`
	ChildrenState DirectoryChildrenState `json:"children_state"`
	Continuation  string                 `json:"continuation,omitempty"`
}

// TreeResolveTarget lets clients batch unrelated expanded directories. ID is
// echoed so a malformed target can fail without invalidating its neighbours.
type TreeResolveTarget struct {
	ID     string `json:"id,omitempty"`
	Path   string `json:"path"`
	Cursor string `json:"cursor,omitempty"`
}

type TreeResolveRequest struct {
	Targets                      []TreeResolveTarget `json:"targets"`
	IncludeIgnored               bool                `json:"include_ignored,omitempty"`
	FollowSingleChildDirectories bool                `json:"follow_single_child_directories,omitempty"`
	EntryBudget                  int                 `json:"entry_budget,omitempty"`
}

type TreeResolveResult struct {
	ID          string          `json:"id,omitempty"`
	Path        string          `json:"path"`
	OK          bool            `json:"ok"`
	Directories []DirectoryPage `json:"directories,omitempty"`
	Code        string          `json:"code,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type TreeResolveResponse struct {
	ProjectID string              `json:"project_id"`
	Results   []TreeResolveResult `json:"results"`
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
