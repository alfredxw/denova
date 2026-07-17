package agent

import (
	"strings"
	"testing"
)

func TestComposeAgentInputInjectsOnlyResolvedReviewFeedbackWithSource(t *testing.T) {
	req := ChatRequest{
		Message: "Please revise this draft.",
		ReviewFeedback: ReviewFeedbackRef{
			ReviewThreadID: "thread-client",
			CommentIDs:     []string{"forged-client-id"},
		},
		ResolvedReviewFeedback: ReviewFeedbackContext{
			ReviewThreadID: "thread-ledger",
			Comments: []ReviewFeedbackComment{{
				ID:          "comment-ledger",
				GroupID:     "group-1",
				ChangeSetID: "change-1",
				Path:        "chapters/ch01.md",
				Body:        "Keep the point of view consistent.",
				Anchor: ReviewFeedbackAnchor{
					Side:     "after",
					Encoding: "utf8-bytes-v1",
					Revision: "sha256:after",
					Start:    12,
					End:      28,
					Quote:    "the quoted sentence",
				},
			}},
		},
	}

	composition := composeAgentInput(req, nil, nil, DefaultLoopPolicy())
	for _, expected := range []string{
		"Source: the active workspace's durable change ledger",
		`"review_thread_id":"thread-ledger"`,
		`"comment_id":"comment-ledger"`,
		`"path":"chapters/ch01.md"`,
		`"side":"after"`,
		`"encoding":"utf8-bytes-v1"`,
		"Keep the point of view consistent.",
	} {
		if !strings.Contains(composition.AgentMessage, expected) {
			t.Fatalf("agent message is missing %q: %s", expected, composition.AgentMessage)
		}
	}
	if strings.Contains(composition.AgentMessage, "forged-client-id") || strings.Contains(composition.AgentMessage, "thread-client") {
		t.Fatalf("unresolved client review data reached the model: %s", composition.AgentMessage)
	}
	if composition.OriginalMessage != req.Message {
		t.Fatalf("original message changed: %q", composition.OriginalMessage)
	}
}

func TestReviewFeedbackContextEnforcesWholeBlockByteLimit(t *testing.T) {
	feedback := ReviewFeedbackContext{
		ReviewThreadID: "thread-1",
		Comments: []ReviewFeedbackComment{{
			ID:      "comment-1",
			GroupID: "group-1",
			Body:    strings.Repeat("界", MaxReviewFeedbackContextBytes),
		}},
	}
	if got := feedback.EncodedSize(); got <= MaxReviewFeedbackContextBytes {
		t.Fatalf("oversized feedback reported %d bytes", got)
	}
	if got := appendReviewFeedbackContext("original", feedback); got != "original" {
		t.Fatal("oversized feedback should not be partially injected")
	}

	feedback.Comments[0].Body = "concise"
	block, err := reviewFeedbackContextBlock(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if len(block) == 0 || len(block) > MaxReviewFeedbackContextBytes {
		t.Fatalf("review feedback block bytes=%d", len(block))
	}
}
