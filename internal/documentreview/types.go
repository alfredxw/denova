package documentreview

import (
	"fmt"
	"time"
)

const (
	AnchorKindTextRange = "text-range"
	AnchorKindTextBlock = "text-block"
	AnchorEncodingUTF8  = "utf8-bytes-v1"

	ErrorCodeNotFound = "not_found"
	ErrorCodeConflict = "conflict"
	ErrorCodeInvalid  = "invalid_edit"
)

// Anchor binds an author comment to one canonical workspace-file revision.
// Markdown byte offsets are authoritative; editor positions are UI hints that
// are only reused while the same revision is displayed.
type Anchor struct {
	Kind         string `json:"kind"`
	Encoding     string `json:"encoding"`
	Revision     string `json:"revision"`
	Start        int    `json:"start"`
	End          int    `json:"end"`
	Quote        string `json:"quote,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	Suffix       string `json:"suffix,omitempty"`
	DisplayQuote string `json:"display_quote,omitempty"`
	EditorFrom   int    `json:"editor_from,omitempty"`
	EditorTo     int    `json:"editor_to,omitempty"`
}

// Comment is an author-owned, one-shot review instruction. It remains visible
// until the author deletes it or a durable chat message consumes it.
type Comment struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"thread_id"`
	Path      string    `json:"path"`
	Body      string    `json:"body"`
	Anchor    Anchor    `json:"anchor"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Deleted   bool      `json:"deleted,omitempty"`
}

// Thread is the hidden batching boundary for all pending document comments in
// one workspace. Its ID becomes the review thread for Agent-authored changes.
type Thread struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Comments  []Comment `json:"comments"`
}

type Snapshot struct {
	Content  string
	Revision string
}

type AddCommentRequest struct {
	Path   string `json:"path"`
	Body   string `json:"body"`
	Anchor Anchor `json:"anchor"`
}

type UpdateCommentRequest struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

type DeleteCommentRequest struct {
	ID string `json:"id"`
}

// Error keeps HTTP/API error handling explicit without coupling this domain to
// the workspace-change package.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func newError(code, message string, details map[string]any) error {
	return &Error{Code: code, Message: message, Details: details}
}
