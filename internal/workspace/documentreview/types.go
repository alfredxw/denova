package documentreview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	AnchorKindTextRange = "text-range"
	AnchorKindTextBlock = "text-block"
	AnchorEncodingUTF8  = "utf8-bytes-v1"

	ErrorCodeNotFound = "not_found"
	ErrorCodeConflict = "conflict"
	ErrorCodeInvalid  = "invalid_edit"

	TargetKindWorkspaceFile = "workspace_file"
	TargetKindLoreItem      = "lore_item"
	TargetFieldLoreContent  = "content"
)

// Target identifies the editable text resource that owns a review comment.
// It deliberately describes domain identity rather than a storage path so the
// same review workflow can support manuscripts and structured workspace data.
type Target struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Field string `json:"field,omitempty"`
}

// Anchor binds an author comment to one canonical text-resource revision.
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

// Comment is an author-owned, one-shot review instruction attached to one
// editable workspace resource. It remains visible until the author deletes it
// or a durable chat message consumes it.
type Comment struct {
	ID                 string    `json:"id"`
	ThreadID           string    `json:"thread_id"`
	Target             Target    `json:"target"`
	Body               string    `json:"body"`
	Anchor             Anchor    `json:"anchor"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Deleted            bool      `json:"deleted,omitempty"`
	AgentInputEffectID string    `json:"agent_input_effect_id,omitempty"`
}

// Thread is the hidden batching boundary for all pending text-resource comments in
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

// ResolvedTarget is the current canonical snapshot for a review target. Name
// is presentation context only; Target remains the stable resource identity.
type ResolvedTarget struct {
	Target   Target
	Name     string
	Snapshot Snapshot
	// ContextSnapshot is present only when the resource is intentionally
	// reviewable but unavailable through normal model read tools. The review
	// feedback budget still bounds the complete injected context.
	ContextSnapshot *Snapshot
}

// SnapshotResolver isolates comment/review orchestration from the storage
// details of each supported text resource.
type SnapshotResolver interface {
	ResolveReviewTarget(ctx context.Context, target Target) (ResolvedTarget, error)
}

type AddCommentRequest struct {
	Target Target `json:"target"`
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

// UnmarshalJSON keeps existing review ledgers readable after the target model
// replaced the old file-only `path` field. New writes contain only `target`.
func (c *Comment) UnmarshalJSON(data []byte) error {
	type commentAlias Comment
	var raw struct {
		commentAlias
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = Comment(raw.commentAlias)
	if strings.TrimSpace(c.Target.Kind) == "" && strings.TrimSpace(raw.Path) != "" {
		c.Target = Target{Kind: TargetKindWorkspaceFile, ID: raw.Path}
	}
	return nil
}

// UnmarshalJSON accepts the pre-target API shape during the beta transition.
// Callers constructing Go values use Target directly, keeping the domain model
// unambiguous while older frontends can still finish an in-flight request.
func (r *AddCommentRequest) UnmarshalJSON(data []byte) error {
	type requestAlias AddCommentRequest
	var raw struct {
		requestAlias
		Path string `json:"path"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*r = AddCommentRequest(raw.requestAlias)
	if strings.TrimSpace(r.Target.Kind) == "" && strings.TrimSpace(raw.Path) != "" {
		r.Target = Target{Kind: TargetKindWorkspaceFile, ID: raw.Path}
	}
	return nil
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
