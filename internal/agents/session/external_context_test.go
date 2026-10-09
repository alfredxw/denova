package session

import (
	"context"
	"fmt"
	"testing"

	agentschema "github.com/alfredxw/denova/agent/schema"
)

func TestExternalContextReadsFullCanonicalHistoryAndExcludesPrivateState(t *testing.T) {
	directory := t.TempDir()
	store, err := NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("external-history")
	if err != nil {
		t.Fatal(err)
	}
	total := sessionRecentTransactionLimit + 25
	batch := make([]*agentschema.Message, total)
	for index := range batch {
		batch[index] = agentschema.UserMessage(fmt.Sprintf("canonical-%03d", index))
	}
	batch[0].ReasoningContent = "private reasoning must not migrate"
	batch[0].ToolCallID = "private-call-id"
	if err := sess.withCanonicalMutation(context.Background(), "publish history fixture", func() error {
		return sess.appendMessagesLocked(batch, make([]MessageMetadata, len(batch)), historyTypeMessage)
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err = store.Get("external-history")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.GetMessages()) >= total {
		t.Fatal("fixture did not exceed the resident history window")
	}
	var content []*agentschema.Message
	err = sess.ReadExternal(context.Background(), func(state ExternalState) error {
		return state.ScanContext(state.ContextSource, func(record ExternalContextRecord) error {
			if record.Message != nil {
				content = append(content, record.Message)
			}
			return nil
		})
	})
	if err != nil || len(content) != total || content[0].Content != "canonical-000" {
		t.Fatalf("full canonical history len=%d err=%v", len(content), err)
	}
	if content[0].ReasoningContent != "" || content[0].ToolCallID != "" {
		t.Fatal("private runtime state crossed the boundary")
	}
	call := agentschema.AssistantMessage("Checking the saved text.", []agentschema.ToolCall{{ID: "private-vendor-id", Type: "function", Function: agentschema.FunctionCall{Name: "read", Arguments: `{"path":"draft.md"}`}}})
	call.ReasoningContent = "private tool reasoning"
	result := agentschema.ToolMessage(agentschema.TextToolResult("Canonical batch observation."), "private-vendor-id", agentschema.WithToolName("read"))
	if _, err := sess.CommitContextBatch(t.Context(), sess.ContextCursor(), DomainCommitIdentity{CommandID: "batch-input", OperationID: "batch-operation", Cycle: 1}, 0, []*agentschema.Message{call, result}, nil); err != nil {
		t.Fatal(err)
	}
	content = nil
	if err := sess.ReadExternal(t.Context(), func(state ExternalState) error {
		return state.ScanContext(state.ContextSource, func(record ExternalContextRecord) error {
			if record.Message != nil {
				content = append(content, record.Message)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if len(content) != total+2 || content[len(content)-1].Content != "Canonical batch observation." || content[len(content)-2].ReasoningContent != "" || len(content[len(content)-2].ToolCalls) != 0 {
		t.Fatal("Native protocol batch lost public observations or leaked private state")
	}
	if err := appendReleasedClearMarker(sess); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agentschema.UserMessage("After clear.")); err != nil {
		t.Fatal(err)
	}
	content = nil
	if err := sess.ReadExternal(context.Background(), func(state ExternalState) error {
		return state.ScanContext(state.ContextSource, func(record ExternalContextRecord) error {
			if record.Message != nil {
				content = append(content, record.Message)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if len(content) != 1 || content[0].Content != "After clear." {
		t.Fatal("cleared context was revived")
	}
}
