package agent

import (
	"context"
	"testing"
)

func TestArtifactProducedIsDurableAndPrecedesToolFinished(t *testing.T) {
	artifact := ToolArtifactRef{
		ID: "artifact-1", Purpose: ToolArtifactPurposeAttachment,
		ReadablePath: ".agent/artifacts/result.json", ContentType: "application/json",
		EstimatedBytes: 17, Complete: true,
	}
	tool, err := InferTool("artifact", "produce an artifact", func(context.Context, struct{}) (ToolResult, error) {
		result := TextToolResult("created")
		result.Artifacts = []ToolArtifactRef{artifact}
		return result, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("", []ToolCall{{ID: "artifact-call", Type: "function", Function: FunctionCall{Name: "artifact", Arguments: `{}`}}}),
		AssistantMessage("done", nil),
	}}
	owner, err := New(context.Background(), Definition{
		Model: model,
		Tools: StaticTools(ToolDefinition{Tool: tool, Descriptor: ToolDescriptor{
			Source: ToolSourceRead, Execution: ToolExecutionParallelRead,
			MutationScope: ToolMutationNone, PostCheck: ToolPostCheckNone,
			Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
			ResultRetention: ToolResultDeferred, Steering: SteeringFinishCurrent,
			MaxResultBytes: 64 << 10,
		}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("produce it"))
	if err != nil {
		t.Fatal(err)
	}
	artifactIndex, finishedIndex := -1, -1
	artifactCallID := ""
	index := 0
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case ArtifactProduced:
			artifactIndex = index
			artifactCallID = payload.CallID
			if event.Durability != DurableEvent || payload.CallID == "" || payload.Artifact.ID != artifact.ID ||
				payload.Artifact.ReadablePath != artifact.ReadablePath || !payload.Artifact.Complete {
				t.Fatalf("artifact event = %#v durability=%q", payload, event.Durability)
			}
		case ToolFinished:
			if payload.CallID == artifactCallID {
				finishedIndex = index
			}
		}
		index++
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if artifactIndex < 0 || finishedIndex < 0 || artifactIndex >= finishedIndex {
		t.Fatalf("artifact index=%d tool finished index=%d", artifactIndex, finishedIndex)
	}
}
