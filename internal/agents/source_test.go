package agents

import (
	"context"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

func TestSourceResolvesProductBindingWithoutRuntimeTypes(t *testing.T) {
	binding := agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindGeneral, Mode: agentrun.ModeAgentChat,
		ProjectID: "project", SessionID: "session",
	}
	key, err := binding.AgentSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	var received DefinitionRequest
	source, err := NewSource(DefinitionResolverFunc(func(_ context.Context, request DefinitionRequest) (agent.Definition, error) {
		received = request
		return agent.Definition{Name: "general"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := source.Prepare(context.Background(), agent.PrepareRequest{
		Session: agent.SessionView{Key: key}, DefinitionKey: "definition", RestoreKey: "restore",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Name != "general" || received.Binding.AgentKind != binding.AgentKind ||
		received.Binding.ProjectID != binding.ProjectID || received.Agent.RestoreKey != "restore" {
		t.Fatalf("prepared=%#v request=%#v", prepared, received)
	}
}

func TestTurnInputRoundTripsOnlyProductSemantics(t *testing.T) {
	input, err := TurnInput(TurnNext, agentchat.ChatRequest{
		CommandID: "command", Message: "write", References: []string{"chapter.md"},
		PlanMode: true, Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly,
	}, agentrun.Options{TaskID: "task", TurnID: "turn", Mode: "ide"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := DecodeTurnHostData(input)
	if err != nil {
		t.Fatal(err)
	}
	request := data.ChatRequest()
	if input.Text != "write" || input.IdempotencyKey != "command" || request.Message != "write" ||
		len(request.References) != 1 || !request.PlanMode || request.Locale != "zh-CN" ||
		data.Kind != TurnNext || data.TurnID != "turn" || data.Mode != "ide" {
		t.Fatalf("input=%#v data=%#v request=%#v", input, data, request)
	}
	recovered, err := DecodeTurnHostDataFromPrepare(agent.PrepareRequest{HostData: input.HostData, Reason: agent.TurnReasonRecovery})
	if err != nil || recovered.Caller.Message != "write" {
		t.Fatalf("recovered HostData = %#v error = %v", recovered, err)
	}
}
