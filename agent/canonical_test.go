package agent

import (
	"strings"
	"testing"
)

func TestCanonicalContextHashValidatesEveryBatchKind(t *testing.T) {
	identity := CapabilityIdentity{Kind: "test.canonical", Version: 1}
	state := newContextStateMessage(
		testContextStateFragment("revision-1", "workspace"),
		strings.Repeat("a", 64),
		"initialize",
		"",
	)
	call := AssistantMessage("", []ToolCall{{
		ID: "call-1", Type: "function", Function: FunctionCall{Name: "inspect", Arguments: `{}`},
	}})
	result := ToolMessage(TextToolResult("complete"), "call-1", WithToolName("inspect"))
	completion := UserMessage("child result")
	completion.TaskCompletion = &TaskCompletionMessageMeta{
		CompletionID: "completion-1", Author: "researcher", Recipient: "parent",
	}

	valid := []struct {
		name     string
		kind     ContextCommitKind
		messages []*Message
	}{
		{name: "state", kind: ContextCommitState, messages: []*Message{state}},
		{name: "tool batch", kind: ContextCommitToolBatch, messages: []*Message{call, result}},
		{name: "task completion", kind: ContextCommitTaskCompletion, messages: []*Message{completion}},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := canonicalContextHash(test.kind, 0, test.messages, identity); err != nil {
				t.Fatalf("valid canonical context rejected: %v", err)
			}
		})
	}

	invalid := []struct {
		name     string
		kind     ContextCommitKind
		messages []*Message
	}{
		{name: "unknown kind", kind: ContextCommitKind("unknown"), messages: []*Message{state}},
		{name: "state carrying ordinary user input", kind: ContextCommitState, messages: []*Message{UserMessage("request")}},
		{name: "tool batch missing result", kind: ContextCommitToolBatch, messages: []*Message{call}},
		{name: "tool batch mismatched result", kind: ContextCommitToolBatch, messages: []*Message{call, ToolMessage(TextToolResult("complete"), "call-2")}},
		{name: "task completion without identity", kind: ContextCommitTaskCompletion, messages: []*Message{UserMessage("child result")}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, err := canonicalContextHash(test.kind, 0, test.messages, identity); err == nil {
				t.Fatal("invalid canonical context was accepted")
			}
		})
	}
}
