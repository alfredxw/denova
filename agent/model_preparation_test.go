package agent

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

type changingArtifactResolver struct {
	artifactStorageProbe
	calls int
}

func (resolver *changingArtifactResolver) ResolveToolArtifactPath(context.Context, string) (string, error) {
	resolver.calls++
	return fmt.Sprintf("/runtime/artifacts/projection-%d.txt", resolver.calls), nil
}

type preparationStateObserver struct {
	BaseMiddleware
	messages []*Message
}

func (observer *preparationStateObserver) AfterAgent(ctx context.Context, state *RunState) (context.Context, error) {
	observer.messages = cloneMessages(state.Messages)
	return ctx, nil
}

func TestPreparedCompactionFreezesArtifactPathsAndKeepsPortableLoopState(t *testing.T) {
	resolver := &changingArtifactResolver{}
	observer := &preparationStateObserver{}
	answer := AssistantMessage("done", nil)
	answer.ResponseMeta = &ResponseMeta{Usage: &TokenUsage{PromptTokens: 200}}
	model := &scriptedModel{responses: []scriptedModelResponse{{message: answer}}}
	identity := CapabilityIdentity{Kind: "test.artifact-projection", Version: 1}
	messages := []*Message{
		UserMessage("Continue from the checkpoint"),
		AssistantMessage("", []ToolCall{{ID: "saved", Type: "function", Function: FunctionCall{Name: "read", Arguments: `{}`}}}),
		ToolMessage(TextToolResult("Read saved.txt"), "saved", WithToolName("read")),
	}
	messages[2].ToolResult.Artifacts = []ToolArtifactRef{{ID: "saved", ReadablePath: "saved.txt"}}
	var validated []*Message
	loop, err := newModelToolLoop(context.Background(), loopConfig{
		Model: model, Artifacts: resolver, Middlewares: []Middleware{observer},
		ModelIdentity: identity,
		modelCallGate: func(_ context.Context, _ *ModelCall, metadata *ModelContext) (*preparedModelCall, error) {
			replacement, err := metadata.prepareCompaction(messages, 0)
			if err == nil {
				validated = replacement.call.Snapshot().Messages()
			}
			return replacement, err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := loop.Run(context.Background(), &loopInput{Messages: messages})
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
	}
	inputs := model.capturedInputs()
	if len(inputs) != 1 || !reflect.DeepEqual(inputs[0], validated) || resolver.calls != 1 {
		t.Fatalf("validated provider projection was rebuilt: inputs=%d resolutions=%d", len(inputs), resolver.calls)
	}
	if got := validated[2].ToolResult.Artifacts[0].ReadablePath; got != "/runtime/artifacts/projection-1.txt" {
		t.Fatalf("provider artifact path=%q", got)
	}
	if observer.messages[2].Content != "Read saved.txt" || observer.messages[2].ToolResult.Artifacts[0].ReadablePath != "saved.txt" {
		t.Fatal("runtime artifact paths entered portable loop state")
	}
	meta := observer.messages[len(observer.messages)-1].ResponseMeta
	if meta == nil || meta.InputEstimate == nil || meta.InputEstimate.Model != identity || meta.InputEstimate.Tokens != EstimateRequestTokens(validated, []*ToolInfo{}) {
		t.Fatalf("input estimate did not use the frozen provider projection: %+v", meta)
	}
}
