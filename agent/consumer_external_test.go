package agent_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/alfredxw/denova/agent"
	agentcontext "github.com/alfredxw/denova/agent/context"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type lookupInput struct {
	Key string `json:"key" jsonschema_description:"Stable key to retrieve."`
}

type lookupOutput struct {
	Value string `json:"value"`
}

type consumerModelCall struct {
	messages []*agent.Message
	tools    []*agent.ToolInfo
}

type consumerModel struct {
	mu    sync.Mutex
	calls []consumerModelCall
}

func (model *consumerModel) Generate(_ context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.Message, error) {
	model.mu.Lock()
	callIndex := len(model.calls)
	call := consumerModelCall{messages: clonePublicMessages(input)}
	call.tools = agent.GetCommonOptions(nil, options...).Tools
	model.calls = append(model.calls, call)
	model.mu.Unlock()

	switch callIndex {
	case 0:
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID:   "lookup-1",
			Type: "function",
			Function: agent.FunctionCall{
				Name:      "lookup",
				Arguments: `{"key":"answer"}`,
			},
		}}), nil
	case 1:
		return agent.AssistantMessage("The portable answer is available.", nil), nil
	default:
		return nil, errors.New("consumer model received an unexpected call")
	}
}

func (*consumerModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return nil, errors.New("consumer model does not support streaming")
}

func (model *consumerModel) snapshotCalls() []consumerModelCall {
	model.mu.Lock()
	defer model.mu.Unlock()
	calls := make([]consumerModelCall, len(model.calls))
	for index, call := range model.calls {
		calls[index] = consumerModelCall{
			messages: clonePublicMessages(call.messages),
			tools:    append([]*agent.ToolInfo(nil), call.tools...),
		}
	}
	return calls
}

func clonePublicMessages(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = agent.CloneMessage(message)
	}
	return result
}

func TestExternalConsumerComposesReusableAgent(t *testing.T) {
	ctx := context.Background()
	lookup, err := agent.InferTool("lookup", "Retrieve one value by stable key.", func(_ context.Context, input lookupInput) (lookupOutput, error) {
		if input.Key != "answer" {
			return lookupOutput{}, errors.New("unknown key")
		}
		return lookupOutput{Value: "portable answer"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	definition := agent.ToolDefinition{
		Tool: lookup,
		Descriptor: agent.ToolDescriptor{
			Source:           agent.ToolSourceRead,
			Capability:       "knowledge.read",
			Execution:        agent.ToolExecutionParallelRead,
			MutationScope:    agent.ToolMutationNone,
			PostCheck:        agent.ToolPostCheckNone,
			Recovery:         agent.ToolRecoveryReadOnly,
			ResultProjection: agent.ToolResultBoundedModelContext,
			Steering:         agent.SteeringFinishCurrent,
			MaxResultBytes:   4096,
		},
	}
	registry, err := agent.NewRegistry(ctx, definition)
	if err != nil {
		t.Fatal(err)
	}

	store := agentsession.NewMemoryStore()
	conversation, err := agentsession.Open("external-consumer", store)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := conversation.Append(ctx, 0, agent.UserMessage("What should I use?"))
	if err != nil {
		t.Fatal(err)
	}

	assembled, err := agentcontext.NewAssembler(agentcontext.Budget{
		MaxFragmentBytes:      256,
		MaxTotalBytes:         1024,
		MaxFragments:          4,
		MaxMetadataFieldBytes: 256,
	}).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: snapshot.EffectiveMessages(),
		Fragments: []agentcontext.Fragment{{
			ID:        "reference-1",
			Source:    "knowledge.record",
			Title:     "Reference",
			Purpose:   "answer the current request",
			Content:   "portable answer",
			Placement: agentcontext.PlacementFinalUserPrefix,
			Limit:     128,
			Included:  true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(assembled.Ledger) != 1 || assembled.Ledger[0].Source != "knowledge.record" ||
		assembled.Ledger[0].Purpose != "answer the current request" || assembled.Ledger[0].Hash == "" ||
		!assembled.Ledger[0].Included || assembled.InjectedBytes <= 0 {
		t.Fatalf("assembled context provenance = %#v", assembled)
	}
	boundedFragment := assembled.Fragments[0]
	if boundedFragment.Hash == "" {
		t.Fatal("assembled fragment has no content hash")
	}
	boundedFragment.Hash = ""
	if !reflect.DeepEqual(boundedFragment, agentcontext.Fragment{
		ID:        "reference-1",
		Source:    "knowledge.record",
		Title:     "Reference",
		Purpose:   "answer the current request",
		Content:   "portable answer",
		Placement: agentcontext.PlacementFinalUserPrefix,
		Limit:     128,
		Included:  true,
	}) {
		t.Fatalf("bounded fragment = %#v", boundedFragment)
	}

	model := &consumerModel{}
	native, err := agent.NewAgent(ctx, agent.AgentConfig{
		Name:        "portable-assistant",
		Instruction: "Use the registered tool before answering.",
		Model:       model,
		Tools:       registry.Definitions(),
	})
	if err != nil {
		t.Fatal(err)
	}
	iterator := agent.NewRunner(agent.RunnerConfig{Agent: native}).Run(ctx, assembled.Messages)
	emitted := make([]*agent.Message, 0, 3)
	for {
		event, available := iterator.Next()
		if !available {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		message, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			t.Fatal(err)
		}
		emitted = append(emitted, agent.CloneMessage(message))
	}

	expectedEmitted := []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "lookup-1", Type: "function",
			Function: agent.FunctionCall{Name: "lookup", Arguments: `{"key":"answer"}`},
		}}),
		agent.ToolMessage(agent.TextToolResult(`{"value":"portable answer"}`), "lookup-1", agent.WithToolName("lookup")),
		agent.AssistantMessage("The portable answer is available.", nil),
	}
	if !reflect.DeepEqual(emitted, expectedEmitted) {
		t.Fatalf("emitted transcript = %#v, want %#v", emitted, expectedEmitted)
	}

	mutations := make([]agentsession.Mutation, 0, len(emitted))
	for _, message := range emitted {
		mutations = append(mutations, agentsession.AppendMessage(message))
	}
	committed, err := conversation.Commit(ctx, snapshot.Revision, mutations...)
	if err != nil {
		t.Fatal(err)
	}
	expectedTranscript := append([]*agent.Message{agent.UserMessage("What should I use?")}, expectedEmitted...)
	if committed.ID != "external-consumer" || committed.Revision != 4 ||
		!reflect.DeepEqual(committed.EffectiveMessages(), expectedTranscript) {
		t.Fatalf("committed transcript = %#v", committed)
	}

	const assembledRequest = "# Reference\n\nportable answer\n\n---\n\n# User request\n\nWhat should I use?"
	calls := model.snapshotCalls()
	if len(calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(calls))
	}
	expectedSecondInput := []*agent.Message{
		agent.SystemMessage("Use the registered tool before answering."),
		agent.UserMessage(assembledRequest),
		expectedEmitted[0],
		expectedEmitted[1],
	}
	if len(calls[0].messages) != 2 || calls[0].messages[1].Content != assembledRequest ||
		!reflect.DeepEqual(calls[1].messages, expectedSecondInput) {
		t.Fatalf("model transcript = %#v", calls)
	}
	for index, call := range calls {
		if len(call.tools) != 1 || call.tools[0].Name != "lookup" {
			t.Fatalf("model call %d tools = %#v", index, call.tools)
		}
		if len(call.tools[0].Extra) != 0 {
			t.Fatalf("model call %d leaked descriptor metadata: %#v", index, call.tools[0].Extra)
		}
	}
	definitionSnapshot, ok := registry.Snapshot("lookup")
	if !ok || !reflect.DeepEqual(definitionSnapshot.Descriptor, definition.Descriptor) {
		t.Fatalf("registry snapshot = %#v, present=%v", definitionSnapshot, ok)
	}
}
