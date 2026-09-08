package agent

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestInvalidCanonicalImportPreservesLoadedSession(t *testing.T) {
	ctx := context.Background()
	owner, err := New(ctx, Definition{Name: "history-validation", Model: &lifecycleModel{}})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	session, err := owner.Session(ctx, NamedSession("history-validation"))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.LoadCanonicalMessages(ctx, []*Message{UserMessage("valid history")}); err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(session.engineState)
	call := AssistantMessage("", []ToolCall{{ID: "call", Function: FunctionCall{Name: "read", Arguments: `{}`}}})
	for name, messages := range map[string][]*Message{
		"split batch":     {call, UserMessage("continue")},
		"unfinished tail": {call},
		"orphan result":   {ToolMessage(TextToolResult("result"), "orphan")},
		"nil message":     {nil},
	} {
		t.Run(name, func(t *testing.T) {
			if err := session.LoadCanonicalMessages(ctx, messages); !errors.Is(err, ErrInvalidCanonicalMessages) {
				t.Fatalf("invalid history error = %v", err)
			}
			if !bytes.Equal(before, session.engineState) || session.active != nil {
				t.Fatal("invalid import changed the previously loaded Session")
			}
		})
	}
}
