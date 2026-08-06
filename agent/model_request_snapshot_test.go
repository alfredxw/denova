package agent

import (
	"context"
	"reflect"
	"testing"
)

type optionCapturingModel struct {
	inputs  [][]*Message
	options []*Options
}

func (model *optionCapturingModel) Generate(_ context.Context, input []*Message, opts ...ModelOption) (*Message, error) {
	model.inputs = append(model.inputs, cloneMessages(input))
	model.options = append(model.options, GetCommonOptions(nil, opts...))
	return AssistantMessage("checkpoint", nil), nil
}

func (model *optionCapturingModel) Stream(_ context.Context, input []*Message, opts ...ModelOption) (*StreamReader[*Message], error) {
	model.inputs = append(model.inputs, cloneMessages(input))
	model.options = append(model.options, GetCommonOptions(nil, opts...))
	return StreamReaderFromArray([]*Message{AssistantMessage("checkpoint", nil)}), nil
}

func TestModelRequestSnapshotForkPreservesPrefixAndOptions(t *testing.T) {
	model := &optionCapturingModel{}
	tool := &ToolInfo{Name: "read", Desc: "read a file", Extra: map[string]any{"order": 1}}
	primary := []*Message{SystemMessage("system"), UserMessage("source")}
	call := &ModelCall{
		Model: model, Messages: primary,
		Options: []ModelOption{
			WithTools([]*ToolInfo{tool}),
			WithMaxTokens(4096),
			WithToolChoice(ToolChoiceAllowed, "read"),
			WithSessionKey("conversation-123"),
		},
	}
	snapshot := call.Snapshot()

	// Mutations after capture must not change the immutable request handle.
	primary[1].Content = "mutated"
	tool.Desc = "mutated"
	call.Options[0] = WithTools(nil)

	fork := snapshot.Append(UserMessage("stable checkpoint request"))
	if _, err := fork.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.inputs))
	}
	wantPrefix := []*Message{SystemMessage("system"), UserMessage("source")}
	if got := model.inputs[0][:len(wantPrefix)]; !reflect.DeepEqual(got, wantPrefix) {
		t.Fatalf("fork prefix changed:\ngot  %#v\nwant %#v", got, wantPrefix)
	}
	if got := model.inputs[0][len(wantPrefix):]; !reflect.DeepEqual(got, []*Message{UserMessage("stable checkpoint request")}) {
		t.Fatalf("fork suffix = %#v", got)
	}
	resolved := model.options[0]
	if len(resolved.Tools) != 1 || resolved.Tools[0].Name != "read" || resolved.Tools[0].Desc != "read a file" {
		t.Fatalf("tools changed after capture: %#v", resolved.Tools)
	}
	if resolved.MaxTokens == nil || *resolved.MaxTokens != 4096 {
		t.Fatalf("max tokens = %#v", resolved.MaxTokens)
	}
	if resolved.ToolChoice == nil || *resolved.ToolChoice != ToolChoiceAllowed || !reflect.DeepEqual(resolved.AllowedToolNames, []string{"read"}) {
		t.Fatalf("tool choice = %#v allowed=%#v", resolved.ToolChoice, resolved.AllowedToolNames)
	}
	if resolved.SessionKey != "conversation-123" {
		t.Fatalf("session key = %q", resolved.SessionKey)
	}
	if got := snapshot.Messages(); !reflect.DeepEqual(got, wantPrefix) {
		t.Fatalf("snapshot mutated after fork: %#v", got)
	}
}

type appendBeforeModelCallMiddleware struct{ BaseMiddleware }

func (*appendBeforeModelCallMiddleware) BeforeModelCall(ctx context.Context, call *ModelCall, _ *ModelContext) (context.Context, *ModelCall, error) {
	next := *call
	next.Messages = append(cloneMessages(call.Messages), UserMessage("maintenance"))
	return ctx, &next, nil
}

func TestBeforeModelCallRewritesTheRequestUsedByTheNativeLoop(t *testing.T) {
	model := &scriptedModel{responses: []scriptedModelResponse{{message: AssistantMessage("done", nil)}}}
	built, err := NewAgent(context.Background(), AgentConfig{
		Name: "model-call-seam", Model: model,
		Middlewares: []Middleware{&appendBeforeModelCallMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := built.Run(context.Background(), &AgentInput{Messages: []*Message{UserMessage("source")}})
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
	if len(inputs) != 1 {
		t.Fatalf("model calls = %d, want 1", len(inputs))
	}
	want := []*Message{UserMessage("source"), UserMessage("maintenance")}
	if !reflect.DeepEqual(inputs[0], want) {
		t.Fatalf("model input = %#v, want %#v", inputs[0], want)
	}
}

func TestRunnerPrepareModelRequestUsesFinalAssemblyWithoutCallingProvider(t *testing.T) {
	model := &optionCapturingModel{}
	built, err := NewAgent(context.Background(), AgentConfig{
		Name: "request-preparation", Instruction: "stable system", Model: model,
		Middlewares: []Middleware{&appendBeforeModelCallMiddleware{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(RunnerConfig{Agent: built, EnableStreaming: true})
	ctx := ContextWithSessionKey(context.Background(), "conversation-prepare")
	snapshot, err := runner.PrepareModelRequest(ctx, []*Message{UserMessage("source")})
	if err != nil {
		t.Fatal(err)
	}
	want := []*Message{SystemMessage("stable system"), UserMessage("source"), UserMessage("maintenance")}
	if got := snapshot.Messages(); !reflect.DeepEqual(got, want) || !snapshot.Streaming() {
		t.Fatalf("prepared request = %#v streaming=%t", got, snapshot.Streaming())
	}
	if len(model.inputs) != 0 {
		t.Fatalf("request preparation invoked provider %d time(s)", len(model.inputs))
	}
	if got := snapshot.ResolvedOptions().SessionKey; got != "conversation-prepare" {
		t.Fatalf("prepared session key = %q", got)
	}
}
