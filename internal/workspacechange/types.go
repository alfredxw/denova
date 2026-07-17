package workspacechange

import (
	"fmt"
	"time"
)

const (
	ErrorCodeInvalidEdit       = "invalid_edit"
	ErrorCodeRevisionConflict  = "revision_conflict"
	ErrorCodeNotFound          = "not_found"
	ErrorCodeConflict          = "conflict"
	ErrorCodeNoRedo            = "no_redo"
	ErrorCodeDurabilityPending = "durability_pending"
)

const (
	ReviewStatusPending  = "pending"
	ReviewStatusAccepted = "accepted"
	ReviewStatusRejected = "rejected"
	ReviewStatusMixed    = "mixed"

	ApplyStatePrepared   = "prepared"
	ApplyStateApplied    = "applied"
	ApplyStateReverted   = "reverted"
	ApplyStateConflicted = "conflicted"
)

const (
	ReviewDecisionAccept = "accept"
	ReviewDecisionReject = "reject"
)

const (
	OriginAgent   = "agent"
	OriginUser    = "user"
	OriginReview  = "review"
	OriginUndo    = "undo"
	OriginRedo    = "redo"
	OriginRestore = "restore"
)

// Error is a stable, transport-neutral failure returned by the change service.
type Error struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func newError(code, message string, details map[string]any) *Error {
	return &Error{Code: code, Message: message, Details: details}
}

type TextEdit struct {
	ID         string `json:"id,omitempty"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

type ApplyEditsRequest struct {
	Path         string         `json:"path"`
	BaseRevision string         `json:"base_revision"`
	Edits        []TextEdit     `json:"edits"`
	Metadata     ChangeMetadata `json:"metadata"`
}

type ReplaceFileRequest struct {
	Path         string         `json:"path"`
	Content      string         `json:"content"`
	BaseRevision string         `json:"base_revision"`
	Metadata     ChangeMetadata `json:"metadata"`
}

// SaveResult reports whether a local editor save changed workspace bytes.
type SaveResult struct {
	Revision string `json:"revision"`
	Changed  bool   `json:"changed"`
}

type ChangeMetadata struct {
	Origin         string `json:"origin"`
	ChangeGroupID  string `json:"change_group_id,omitempty"`
	ReviewThreadID string `json:"review_thread_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	AutoAccept     bool   `json:"auto_accept,omitempty"`
}

type Hunk struct {
	ID          string `json:"id"`
	BeforeStart int    `json:"before_start"`
	BeforeEnd   int    `json:"before_end"`
	AfterStart  int    `json:"after_start"`
	AfterEnd    int    `json:"after_end"`
}

type AppliedEdit struct {
	ID           string `json:"id"`
	OldString    string `json:"old_string"`
	NewString    string `json:"new_string"`
	ReplaceAll   bool   `json:"replace_all,omitempty"`
	Hunks        []Hunk `json:"hunks"`
	ReviewStatus string `json:"review_status,omitempty"`
}

type ChangeSet struct {
	ID             string        `json:"id"`
	Sequence       uint64        `json:"sequence"`
	GroupID        string        `json:"group_id"`
	Path           string        `json:"path"`
	BaseRevision   string        `json:"base_revision"`
	Revision       string        `json:"revision"`
	BeforeBlob     string        `json:"before_blob"`
	AfterBlob      string        `json:"after_blob"`
	BeforeExists   bool          `json:"before_exists"`
	AfterExists    bool          `json:"after_exists"`
	BeforeContent  string        `json:"before_content,omitempty"`
	AfterContent   string        `json:"after_content,omitempty"`
	Edits          []AppliedEdit `json:"edits"`
	ReviewStatus   string        `json:"review_status"`
	ApplyState     string        `json:"apply_state"`
	RevertsID      string        `json:"reverts_id,omitempty"`
	ReplaysID      string        `json:"replays_id,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	Origin         string        `json:"origin,omitempty"`
	ReviewThreadID string        `json:"review_thread_id,omitempty"`
	RunID          string        `json:"run_id,omitempty"`
	SessionID      string        `json:"session_id,omitempty"`
	ToolCallID     string        `json:"tool_call_id,omitempty"`
}

type ChangeFilter struct {
	Status         string `json:"status,omitempty"`
	Path           string `json:"path,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	ReviewThreadID string `json:"review_thread_id,omitempty"`
}

type ChangeGroupSummary struct {
	ID                     string    `json:"id"`
	Origin                 string    `json:"origin"`
	ReviewThreadID         string    `json:"review_thread_id"`
	RunID                  string    `json:"run_id,omitempty"`
	SessionID              string    `json:"session_id,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	ReviewStatus           string    `json:"review_status"`
	ApplyState             string    `json:"apply_state"`
	CanUndo                bool      `json:"can_undo"`
	CanRedo                bool      `json:"can_redo"`
	PendingEditCount       int       `json:"pending_edit_count"`
	UnresolvedCommentCount int       `json:"unresolved_comment_count"`
	ChangeSetCount         int       `json:"change_set_count"`
	Paths                  []string  `json:"paths,omitempty"`
}

type ChangeGroup struct {
	ID                     string      `json:"id"`
	Origin                 string      `json:"origin"`
	ReviewThreadID         string      `json:"review_thread_id"`
	RunID                  string      `json:"run_id,omitempty"`
	SessionID              string      `json:"session_id,omitempty"`
	CreatedAt              time.Time   `json:"created_at"`
	ReviewStatus           string      `json:"review_status"`
	ApplyState             string      `json:"apply_state"`
	CanUndo                bool        `json:"can_undo"`
	CanRedo                bool        `json:"can_redo"`
	PendingEditCount       int         `json:"pending_edit_count"`
	UnresolvedCommentCount int         `json:"unresolved_comment_count"`
	ChangeSets             []ChangeSet `json:"change_sets"`
	Comments               []Comment   `json:"comments"`
}

const (
	ReviewThreadContinuityContinuous = "continuous"
	ReviewThreadContinuityConflicted = "conflicted"
)

// ReviewThread is a derived, cross-run review projection. Each group remains
// the independent history/undo boundary; the thread only composes their review
// presentation and feedback.
type ReviewThread struct {
	ID                     string               `json:"id"`
	CreatedAt              time.Time            `json:"created_at"`
	UpdatedAt              time.Time            `json:"updated_at"`
	LatestGroupID          string               `json:"latest_group_id"`
	ReviewStatus           string               `json:"review_status"`
	ApplyState             string               `json:"apply_state"`
	PendingEditCount       int                  `json:"pending_edit_count"`
	UnresolvedCommentCount int                  `json:"unresolved_comment_count"`
	Groups                 []ChangeGroupSummary `json:"groups"`
	Comments               []Comment            `json:"comments"`
	Files                  []ReviewThreadFile   `json:"files"`
}

// ReviewThreadFile exposes the latest contiguous cumulative segment for one
// path. A conflicted continuity value means an intervening write prevented a
// safe endpoint fold; OmittedIterationCount reports how many earlier change
// sets were intentionally excluded from BeforeContent/AfterContent.
type ReviewThreadFile struct {
	Path                  string   `json:"path"`
	BeforeExists          bool     `json:"before_exists"`
	AfterExists           bool     `json:"after_exists"`
	BaseRevision          string   `json:"base_revision"`
	Revision              string   `json:"revision"`
	BeforeContent         string   `json:"before_content"`
	AfterContent          string   `json:"after_content"`
	BaseGroupID           string   `json:"base_group_id"`
	BaseChangeSetID       string   `json:"base_change_set_id"`
	LatestGroupID         string   `json:"latest_group_id"`
	LatestChangeSetID     string   `json:"latest_change_set_id"`
	GroupIDs              []string `json:"group_ids"`
	ChangeSetIDs          []string `json:"change_set_ids"`
	PendingEditIDs        []string `json:"pending_edit_ids"`
	ReviewStatus          string   `json:"review_status"`
	ApplyState            string   `json:"apply_state"`
	Continuity            string   `json:"continuity"`
	OmittedIterationCount int      `json:"omitted_iteration_count,omitempty"`
}

// ReviewFeedbackComment is the trusted ledger comment payload resolved for an
// Agent feedback turn. Path is empty only for a group-level comment.
type ReviewFeedbackComment struct {
	Comment Comment `json:"comment"`
	Path    string  `json:"path,omitempty"`
}

type ReviewRequest struct {
	GroupID      string   `json:"group_id"`
	ChangeSetID  string   `json:"change_set_id,omitempty"`
	Decision     string   `json:"decision"`
	EditIDs      []string `json:"edit_ids,omitempty"`
	BaseRevision string   `json:"base_revision,omitempty"`
}

// ReviewResult separates the full review projection from the paths whose
// visible bytes changed during this specific decision. Callers must use the
// receipt paths for downstream side effects instead of inferring them from the
// group's complete history.
type ReviewResult struct {
	Group         ChangeGroup
	AffectedPaths []string
}

type HistoryRequest struct {
	GroupID string `json:"group_id"`
}

const (
	CommentAnchorSideBefore       = "before"
	CommentAnchorSideAfter        = "after"
	CommentAnchorEncodingUTF8Byte = "utf8-bytes-v1"
)

type CommentAnchor struct {
	Side     string `json:"side,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Revision string `json:"revision,omitempty"`
	Start    int    `json:"start,omitempty"`
	End      int    `json:"end,omitempty"`
	Quote    string `json:"quote,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	Suffix   string `json:"suffix,omitempty"`
}

type Comment struct {
	ID          string        `json:"id"`
	GroupID     string        `json:"group_id"`
	ChangeSetID string        `json:"change_set_id,omitempty"`
	EditID      string        `json:"edit_id,omitempty"`
	HunkID      string        `json:"hunk_id,omitempty"`
	Body        string        `json:"body"`
	Author      string        `json:"author,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Resolved    bool          `json:"resolved,omitempty"`
	Deleted     bool          `json:"deleted,omitempty"`
	Anchor      CommentAnchor `json:"anchor,omitempty"`
}

type AddCommentRequest struct {
	GroupID     string        `json:"group_id"`
	ChangeSetID string        `json:"change_set_id,omitempty"`
	EditID      string        `json:"edit_id,omitempty"`
	HunkID      string        `json:"hunk_id,omitempty"`
	Body        string        `json:"body"`
	Author      string        `json:"author,omitempty"`
	Anchor      CommentAnchor `json:"anchor,omitempty"`
}

type UpdateCommentRequest struct {
	ID     string         `json:"id"`
	Body   string         `json:"body"`
	Author string         `json:"author,omitempty"`
	Anchor *CommentAnchor `json:"anchor,omitempty"`
}

type ResolveCommentRequest struct {
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
}

type DeleteCommentRequest struct {
	ID string `json:"id"`
}

func (c ChangeSet) String() string {
	return fmt.Sprintf("%s:%s@%s", c.GroupID, c.Path, c.Revision)
}
