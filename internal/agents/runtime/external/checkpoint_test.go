package external

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

func TestExternalCheckpointCoversLargeCanonicalMessageAndReusesVerifiedPrefix(t *testing.T) {
	service, request, _ := operationFixture(t)
	large := "Keep the document at chapters/opening.md.\n" + strings.Repeat("这是完整的历史资料。", 10000)
	if err := request.Session.Append(agent.UserMessage(large)); err != nil {
		t.Fatal(err)
	}
	maintenance, turns := 0, 0
	request.Adapter = adapterFunc(func(ctx context.Context, input Input, host Host) (Result, error) {
		if input.Instructions == checkpointInstruction {
			maintenance++
			textBytes := 0
			for _, message := range input.History {
				textBytes += len(message.Text)
				if !utf8.ValidString(message.Text) {
					t.Fatal("maintenance split a UTF-8 source character")
				}
			}
			if textBytes > maintenanceChunkBytes {
				t.Fatal("maintenance did not bound source text")
			}
			result, err := host.CallTool(ctx, ToolCall{ID: "unexpected", Name: "ask"})
			if err != nil || result.Success {
				t.Fatal("maintenance allowed interactive or domain execution")
			}
			return Result{Text: "Preserve the opening document at chapters/opening.md and the original history constraints."}, nil
		}
		turns++
		textBytes := 0
		for _, message := range input.History {
			textBytes += len(message.Text)
		}
		if textBytes > historyBudget || !strings.Contains(input.History[0].Text, "chapters/opening.md") {
			t.Fatalf("invalid compacted input: %#v", input.History)
		}
		return Result{Text: "Continued."}, nil
	})
	prepare := func(command string) {
		history, err := ReadHistory(t.Context(), request.Session)
		if err != nil {
			t.Fatal(err)
		}
		request.PreparedCursor, request.Checkpoint, request.Input.History = history.Cursor, history.Checkpoint, history.Messages
		request.CommandID, request.Fingerprint, request.Metadata.MessageID = command, command, command+"-input"
	}
	run := func() {
		operation, err := service.Start(t.Context(), request)
		if err != nil {
			t.Fatal(err)
		}
		if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("outcome: %#v", outcome)
		}
	}
	prepare("first")
	run()
	if maintenance < 2 {
		t.Fatal("large source did not require bounded maintenance calls")
	}
	firstCost := maintenance
	prepare("second")
	if request.Checkpoint == nil || request.Checkpoint.SourceStart != request.Checkpoint.SourceEnd || request.Checkpoint.EngineVersion != "test-engine-v1" {
		t.Fatalf("invalid checkpoint source: %#v", request.Checkpoint)
	}
	run()
	if maintenance != firstCost || turns != 2 {
		t.Fatalf("checkpoint was not reused: maintenance=%d turns=%d", maintenance, turns)
	}
	prepare("changed-source")
	request.Checkpoint.SourceHash = "invalid-restored-prefix"
	run()
	if maintenance <= firstCost {
		t.Fatal("invalid checkpoint was used without re-reading its source")
	}
	canonical, err := request.Session.ReadCanonicalMessages(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(canonical) != 7 || canonical[0].Content != large {
		t.Fatal("context maintenance removed canonical content")
	}
}
