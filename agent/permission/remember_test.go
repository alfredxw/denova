package permission_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/permission"
)

type rememberStore struct {
	allowed atomic.Bool
}

func (*rememberStore) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "permission.remember-test", Version: 1}
}

func (store *rememberStore) Allowed(context.Context, permission.Rule) (bool, error) {
	return store.allowed.Load(), nil
}

func (store *rememberStore) Remember(context.Context, permission.Rule) error {
	store.allowed.Store(true)
	return nil
}

type permissionModel struct {
	mu        sync.Mutex
	responses []*agent.Message
}

func (model *permissionModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return model.next()
}

func (model *permissionModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.next()
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (model *permissionModel) next() (*agent.Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.responses) == 0 {
		return nil, errors.New("Permission model exhausted")
	}
	message := model.responses[0]
	model.responses = model.responses[1:]
	return message.Clone(), nil
}

func TestRememberedRuleCommitsBeforeResolutionAndFutureToolExecution(t *testing.T) {
	store := &rememberStore{}
	model := &permissionModel{responses: []*agent.Message{
		toolCall("write-1"), agent.AssistantMessage("first done", nil),
		toolCall("write-2"), agent.AssistantMessage("second done", nil),
	}}
	tool, err := agent.InferTool("write_test", "write a test resource", func(context.Context, struct{}) (string, error) {
		if !store.allowed.Load() {
			return "", errors.New("tool ran before remembered permission was durable")
		}
		return "written", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	toolset := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.permission-remember", Version: 1}, agent.ToolDefinition{
		Tool: tool, Descriptor: agent.ToolDescriptor{
			Source: agent.ToolSourceWrite, Execution: agent.ToolExecutionWorkspaceExclusive,
			MutationScope: agent.ToolMutationWorkspace, PostCheck: agent.ToolPostCheckWorkspaceChange,
			Recovery: agent.ToolRecoveryIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
			ResultRetention: agent.ToolResultProtected, Steering: agent.SteeringFinishCurrent,
			MaxResultBytes: 64 << 10,
		},
	})
	owner, err := agent.New(context.Background(), agent.Definition{
		Model: model, Tools: toolset, Permission: permission.Coding(store),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("permission-remember"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := session.Run(context.Background(), agent.Text("first write"))
	if err != nil {
		t.Fatal(err)
	}
	var interactionID string
	for event := range first.Events() {
		if request, ok := event.Payload.(agent.InteractionRequested); ok {
			interactionID = request.Request.ID
			break
		}
	}
	if interactionID == "" {
		t.Fatal("write did not request permission")
	}
	if err := first.Respond(context.Background(), interactionID, agent.InteractionResponse{Permission: agent.PermissionRemember}); err != nil {
		t.Fatal(err)
	}
	for event := range first.Events() {
		if _, ok := event.Payload.(agent.InteractionResolved); ok && !store.allowed.Load() {
			t.Fatal("InteractionResolved was published before RuleStore.Remember")
		}
	}
	if result, err := first.Wait(context.Background()); err != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("first result = %#v error = %v", result, err)
	}
	second, err := session.Run(context.Background(), agent.Text("second write"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := second.Wait(context.Background()); err != nil || result.Status != agent.ResultCompleted {
		t.Fatalf("second result = %#v error = %v", result, err)
	}
	for event := range second.Events() {
		if _, ok := event.Payload.(agent.InteractionRequested); ok {
			t.Fatal("remembered rule requested permission again")
		}
	}
}

func toolCall(id string) *agent.Message {
	return agent.AssistantMessage("", []agent.ToolCall{{
		ID: id, Type: "function", Function: agent.FunctionCall{Name: "write_test", Arguments: `{}`},
	}})
}
