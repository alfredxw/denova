package agents

import (
	"strings"
	"testing"

	agentcontext "denova/internal/agents/context"
)

func TestTurnInputProjectionInjectsOnlyResolvedReviewFeedbackWithSource(t *testing.T) {
	req := ChatRequest{
		Message: "Please revise this draft.",
		ReviewFeedback: ReviewFeedbackRefs{{
			ReviewThreadID: "thread-client",
			CommentIDs:     []string{"forged-client-id"},
		}},
		ResolvedReviewFeedback: ReviewFeedbackContexts{{
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
		}, {
			Source:         ReviewFeedbackSourceDocument,
			ReviewThreadID: "document-thread",
			Comments: []ReviewFeedbackComment{{
				ID: "document-comment", Path: "chapters/ch02.md", Body: "Make the image more concrete.",
			}, {
				ID: "lore-comment",
				Target: &ReviewFeedbackTarget{
					Kind: "lore_item", ID: "hero", Field: "content", Name: "Lin Chuan",
					Snapshot: &ReviewFeedbackSnapshot{Revision: "lore-r2", Content: "Canonical lore body."},
				},
				Body: "Explain the event behind this fear.",
			}},
		}},
	}

	composition, assembled := assembleTurnForTest(t, req, nil, nil, agentcontext.DefaultBudget())
	modelMessage := finalAssembledUserMessage(t, assembled)
	for _, expected := range []string{
		`"source":"workspace_change"`,
		`"review_thread_id":"thread-ledger"`,
		`"comment_id":"comment-ledger"`,
		`"path":"chapters/ch01.md"`,
		`"side":"after"`,
		`"encoding":"utf8-bytes-v1"`,
		"Keep the point of view consistent.",
		`"source":"document"`,
		`"review_thread_id":"document-thread"`,
		"Make the image more concrete.",
		`"target":{"kind":"lore_item","id":"hero","field":"content","name":"Lin Chuan","snapshot":{"revision":"lore-r2","content":"Canonical lore body."}}`,
		"Explain the event behind this fear.",
		"`read_lore_items` and `write_lore_items`",
		"If `target.snapshot` is present",
	} {
		if !strings.Contains(modelMessage, expected) {
			t.Fatalf("agent message is missing %q: %s", expected, modelMessage)
		}
	}
	if strings.Contains(modelMessage, "forged-client-id") || strings.Contains(modelMessage, "thread-client") {
		t.Fatalf("unresolved client review data reached the model: %s", modelMessage)
	}
	if composition.OriginalMessage != req.Message {
		t.Fatalf("original message changed: %q", composition.OriginalMessage)
	}
}

func TestReviewFeedbackContextEnforcesWholeBlockByteLimit(t *testing.T) {
	feedback := ReviewFeedbackContexts{{
		ReviewThreadID: "thread-1",
		Comments: []ReviewFeedbackComment{{
			ID:      "comment-1",
			GroupID: "group-1",
			Body:    strings.Repeat("界", MaxReviewFeedbackContextBytes),
		}},
	}}
	if got := feedback.EncodedSize(); got <= MaxReviewFeedbackContextBytes {
		t.Fatalf("oversized feedback reported %d bytes", got)
	}
	composition, _ := assembleTurnForTest(t, ChatRequest{ResolvedReviewFeedback: feedback}, nil, nil, agentcontext.DefaultBudget())
	for _, fragment := range composition.Fragments {
		if fragment.Source == "workspace.review.feedback" {
			t.Fatal("oversized feedback should not be partially projected")
		}
	}

	feedback[0].Comments[0].Body = "concise"
	block, err := reviewFeedbackContextBlock(feedback)
	if err != nil {
		t.Fatal(err)
	}
	if len(block) == 0 || len(block) > MaxReviewFeedbackContextBytes {
		t.Fatalf("review feedback block bytes=%d", len(block))
	}
}

func TestEmptyReviewFeedbackDoesNotProduceModelContext(t *testing.T) {
	if block, ok := reviewFeedbackContextBlockFromNormalized(nil); ok || block != "" {
		t.Fatalf("empty review feedback produced model context: ok=%v block=%q", ok, block)
	}
	composition, _ := assembleTurnForTest(t, ChatRequest{Message: "continue"}, nil, nil, agentcontext.DefaultBudget())
	for _, fragment := range composition.Fragments {
		if fragment.Source == "workspace.review.feedback" {
			t.Fatalf("empty review feedback produced an injected fragment: %#v", fragment)
		}
	}
}
