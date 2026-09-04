package agent

import (
	"strings"
	"testing"
)

func TestValidateContextCommitDerivesEveryBatchShape(t *testing.T) {
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
		messages []*Message
	}{
		{name: "state", messages: []*Message{state}},
		{name: "tool batch", messages: []*Message{call, result}},
		{name: "task completion", messages: []*Message{completion}},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			request := ContextCommitRequest{
				Identity: CommitIdentity{Stage: CommitContext}, Sequence: 0,
				Messages: messageValues(test.messages),
			}
			if err := ValidateContextCommit(request); err != nil {
				t.Fatalf("valid canonical context rejected: %v", err)
			}
		})
	}

	invalid := []struct {
		name     string
		messages []*Message
	}{
		{name: "ordinary user input", messages: []*Message{UserMessage("request")}},
		{name: "tool batch missing result", messages: []*Message{call}},
		{name: "tool batch mismatched result", messages: []*Message{call, ToolMessage(TextToolResult("complete"), "call-2")}},
		{name: "task completion without identity", messages: []*Message{UserMessage("child result")}},
		{name: "mixed state and completion", messages: []*Message{state, completion}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateContextCommitMessages(test.messages); err == nil {
				t.Fatal("invalid canonical context was accepted")
			}
		})
	}
}

func TestValidateContextCommitRejectsInvalidProtocolIdentity(t *testing.T) {
	message := UserMessage("child result")
	message.TaskCompletion = &TaskCompletionMessageMeta{
		CompletionID: "completion-1", Author: "researcher", Recipient: "parent",
	}
	for _, request := range []ContextCommitRequest{
		{Identity: CommitIdentity{Stage: CommitOutput}, Sequence: 0, Messages: messageValues([]*Message{message})},
		{Identity: CommitIdentity{Stage: CommitContext}, Sequence: -1, Messages: messageValues([]*Message{message})},
	} {
		if err := ValidateContextCommit(request); err == nil {
			t.Fatal("invalid context commit identity was accepted")
		}
	}
}

func messageValues(messages []*Message) []Message {
	values := make([]Message, len(messages))
	for index, message := range messages {
		values[index] = *message.Clone()
	}
	return values
}
