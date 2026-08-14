package execution

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
)

type publicInspectionMiddleware struct {
	agent.BaseMiddleware
	called *atomic.Bool
}

func (middleware publicInspectionMiddleware) BeforeModelCall(
	ctx context.Context,
	call *agent.ModelCall,
	_ *agent.ModelContext,
) (context.Context, *agent.ModelCall, error) {
	if !agent.IsInspection(ctx) {
		return ctx, call, nil
	}
	middleware.called.Store(true)
	next := *call
	next.Messages = append(call.Snapshot().Messages(), agent.UserMessage("INSPECTION_PRODUCT_MIDDLEWARE"))
	return ctx, &next, nil
}

func TestRuntimeInspectUsesExactPublicDefinitionWithoutRegistrationOrProductMutation(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-inspection")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.UserMessage("durable question")); err != nil {
		t.Fatal(err)
	}
	if err := sess.Append(agent.AssistantMessage("durable answer", nil)); err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{Workspace: workspace}, agentrun.AgentKindIDE)
	model := &publicBackendTestModel{}
	var middlewareCalled atomic.Bool
	cycle := Cycle{
		Definition: agent.Definition{
			Key: "denova.test.inspection", Name: "root", Instructions: "INSPECTION_PRODUCT_INSTRUCTION",
			Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "model.denova.inspection", Version: 1},
			Middlewares: []agent.Middleware{agent.IdentifyMiddleware(
				&publicInspectionMiddleware{called: &middlewareCalled},
				agent.CapabilityIdentity{Kind: "middleware.denova.inspection", Version: 1},
			)},
		},
		Conversation: conversation,
		Request:      agentchatRequest("inspect-command", "prospective request"),
		Options: agentrun.Options{
			AgentKind: agentrun.AgentKindIDE, SessionID: sess.ID, Workspace: workspace,
			TaskID: "inspection-task", RootAgentName: "root", Mode: "ide",
		},
	}
	runtime, err := NewAgentRuntime(ctx, t.TempDir(), WithToolMutationApplier(
		func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	before := sess.GetMessages()
	inspection, err := runtime.Inspect(ctx, cycle)
	if err != nil {
		t.Fatal(err)
	}
	if !middlewareCalled.Load() {
		t.Fatal("Denova inspection skipped the configured public Middleware")
	}
	if calls := model.inputs; len(calls) != 0 {
		t.Fatalf("Denova inspection invoked its model: %#v", calls)
	}
	if !reflect.DeepEqual(sess.GetMessages(), before) {
		t.Fatalf("Denova inspection changed the product Session: before=%#v after=%#v", before, sess.GetMessages())
	}
	if got := runtime.public.registration(inspection.Session.Key, cycle.Request.CommandID); got != nil {
		t.Fatalf("Denova inspection retained a cycle registration: %#v", got)
	}
	if len(runtime.public.cycles) != 0 || len(runtime.public.runs) != 0 {
		t.Fatalf("Denova inspection admitted lifecycle state: cycles=%#v runs=%#v", runtime.public.cycles, runtime.public.runs)
	}
	joined := ""
	for _, message := range inspection.ModelRequest.Messages {
		if message != nil {
			joined += message.Content + "\n"
		}
	}
	for _, want := range []string{
		"INSPECTION_PRODUCT_INSTRUCTION", "durable question", "durable answer",
		"prospective request", "INSPECTION_PRODUCT_MIDDLEWARE",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Denova inspection request is missing %q:\n%s", want, joined)
		}
	}
}
