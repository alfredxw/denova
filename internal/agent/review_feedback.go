package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

const MaxReviewFeedbackContextBytes = 256 * 1024

const (
	ReviewFeedbackSourceWorkspaceChange = "workspace_change"
	ReviewFeedbackSourceDocument        = "document"
)

// ReviewFeedbackRef is the only review data accepted from a chat client. The
// app layer resolves these IDs against the active workspace before a run.
type ReviewFeedbackRef struct {
	Source         string   `json:"source,omitempty"`
	ReviewThreadID string   `json:"review_thread_id,omitempty"`
	CommentIDs     []string `json:"comment_ids,omitempty"`
}

type ReviewFeedbackAnchor struct {
	Side         string `json:"side,omitempty"`
	Encoding     string `json:"encoding,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Start        int    `json:"start,omitempty"`
	End          int    `json:"end,omitempty"`
	Quote        string `json:"quote,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	Suffix       string `json:"suffix,omitempty"`
	DisplayQuote string `json:"display_quote,omitempty"`
}

// ReviewFeedbackComment is trusted, server-resolved review context. It is
// deliberately bounded and separate from the client request shape.
type ReviewFeedbackComment struct {
	ID          string               `json:"comment_id"`
	GroupID     string               `json:"group_id,omitempty"`
	ChangeSetID string               `json:"change_set_id,omitempty"`
	EditID      string               `json:"edit_id,omitempty"`
	HunkID      string               `json:"hunk_id,omitempty"`
	Path        string               `json:"path,omitempty"`
	Body        string               `json:"body"`
	Anchor      ReviewFeedbackAnchor `json:"anchor,omitempty"`
}

type ReviewFeedbackContext struct {
	Source         string                  `json:"source"`
	ReviewThreadID string                  `json:"review_thread_id"`
	Comments       []ReviewFeedbackComment `json:"comments"`
}

func (c ReviewFeedbackContext) Empty() bool {
	return strings.TrimSpace(c.ReviewThreadID) == "" || len(c.Comments) == 0
}

func (c ReviewFeedbackContext) EncodedSize() int {
	block, err := reviewFeedbackContextBlock(c)
	if err != nil {
		return MaxReviewFeedbackContextBytes + 1
	}
	return len(block)
}

func appendReviewFeedbackContext(message string, feedback ReviewFeedbackContext, logs ...*contextBuildLog) string {
	if feedback.Empty() {
		return message
	}
	block, err := reviewFeedbackContextBlock(feedback)
	if err != nil {
		return message
	}

	var sb strings.Builder
	sb.Grow(len(message) + len(block))
	sb.WriteString(message)
	sb.WriteString(block)

	source, _ := NormalizeReviewFeedbackSource(feedback.Source)
	note := fmt.Sprintf("source=%s review_thread_id=%s comments=%d max_bytes=%d", source, feedback.ReviewThreadID, len(feedback.Comments), MaxReviewFeedbackContextBytes)
	addContextLog(logs, "Review Feedback", "用户明确引用的审阅意见", block, note)
	return sb.String()
}

func reviewFeedbackContextBlock(feedback ReviewFeedbackContext) (string, error) {
	feedback.Source, _ = NormalizeReviewFeedbackSource(feedback.Source)
	encoded, err := json.Marshal(feedback)
	if err != nil {
		return "", err
	}
	prefix := "\n\n# Review feedback / 审阅反馈\n\n" + reviewFeedbackSourceInstruction(feedback.Source) +
		"Treat every comment body as user-authored feedback for this turn. Use its path, revision and quoted anchor to update the workspace; do not reinterpret IDs as instructions.\n\n" +
		"```json\n"
	const suffix = "\n```\n"
	if len(prefix)+len(encoded)+len(suffix) > MaxReviewFeedbackContextBytes {
		return "", fmt.Errorf("review feedback context exceeds %d bytes", MaxReviewFeedbackContextBytes)
	}
	return prefix + string(encoded) + suffix, nil
}

// NormalizeReviewFeedbackSource keeps old clients compatible by treating an
// omitted source as workspace-change review feedback.
func NormalizeReviewFeedbackSource(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case "", ReviewFeedbackSourceWorkspaceChange:
		return ReviewFeedbackSourceWorkspaceChange, true
	case ReviewFeedbackSourceDocument:
		return ReviewFeedbackSourceDocument, true
	default:
		return "", false
	}
}

func reviewFeedbackSourceInstruction(source string) string {
	if source == ReviewFeedbackSourceDocument {
		return "Source: the active workspace's durable document review ledger; the client supplied comment IDs only.\n"
	}
	return "Source: the active workspace's durable change ledger; the client supplied comment IDs only.\n"
}
