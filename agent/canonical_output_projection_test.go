package agent

import (
	"context"
	"reflect"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestCanonicalOutputProjectionPreservesCurrentTurnFinishReason(t *testing.T) {
	for _, finish := range []string{"stop", "length"} {
		t.Run(finish, func(t *testing.T) {
			ctx := context.Background()
			response := AssistantMessage("approved answer", nil)
			response.ReasoningContent = "provider thinking"
			response.ResponseMeta = &ResponseMeta{FinishReason: finish, Usage: &TokenUsage{TotalTokens: 37}}
			canonical := []*Message{UserMessage("question"), AssistantMessage("approved answer", nil)}
			var committed Message
			adapter := CanonicalAdapterFuncs{
				CapabilityIdentity: CapabilityIdentity{Kind: "canonical.test.projected-output", Version: 1},
				MaterializeInputFn: func(context.Context, InputCommitRequest) (CommitReceipt, error) {
					return CommitReceipt{Revision: "input-1"}, nil
				},
				CommitOutputFn: func(_ context.Context, request OutputCommitRequest) (OutputCommitReceipt, error) {
					committed = request.Message
					return OutputCommitReceipt{
						Revision: "output-1",
						Transcript: &OutputProjection{
							Content: "approved answer", Thinking: "approved thinking", CanonicalMessages: canonical,
						},
					}, nil
				},
			}
			owner, err := New(ctx, Definition{
				Name: "projected output", Model: &lifecycleModel{responses: []*Message{response}}, Canonical: adapter,
			}, WithSessionStore(agentsession.Memory()))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = owner.Close(ctx) })
			session, err := owner.Session(ctx, NamedSession("projected-output"))
			if err != nil {
				t.Fatal(err)
			}
			run, err := session.Run(ctx, Text("question"))
			if err != nil {
				t.Fatal(err)
			}
			result, waitErr := run.Wait(ctx)
			if finish == "length" {
				if result.Status != ResultIncomplete || result.Reason != ModelOutputTruncatedReason || waitErr == nil {
					t.Fatalf("projection lost incomplete output classification: %+v, %v", result, waitErr)
				}
			} else if result.Status != ResultCompleted || waitErr != nil {
				t.Fatalf("projected output did not complete: %+v, %v", result, waitErr)
			}
			if !reflect.DeepEqual(committed.ResponseMeta, response.ResponseMeta) || committed.ReasoningContent != response.ReasoningContent {
				t.Fatalf("canonical commit lost provider metadata: %+v", committed)
			}
			snapshot, err := session.Snapshot(ctx)
			if err != nil || len(snapshot.RecentRuns) != 1 || snapshot.RecentRuns[0].Output != "approved answer" {
				t.Fatalf("current turn lost the approved output: %+v, %v", snapshot.RecentRuns, err)
			}
			session.mu.RLock()
			transcript, err := decodeEngineTranscript(session.engineState)
			session.mu.RUnlock()
			if err != nil || !reflect.DeepEqual(transcript.Messages, canonical) {
				t.Fatalf("retained transcript differs from the host projection: %+v, %v", transcript.Messages, err)
			}
		})
	}
}
