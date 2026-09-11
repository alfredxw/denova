package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type unconfirmedEffectStore struct{ agentsession.Store }
type unconfirmedEffectLog struct{ agentsession.Log }

func (store unconfirmedEffectStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	log, err := store.Store.Open(ctx, key)
	return unconfirmedEffectLog{log}, err
}

func (log unconfirmedEffectLog) Append(ctx context.Context, revision agentsession.Revision, records ...agentsession.Record) (agentsession.Revision, error) {
	for _, record := range records {
		if record.Kind != "turn.tool" {
			continue
		}
		var fact struct {
			Result *agent.ToolResult `json:"result"`
		}
		if err := json.Unmarshal(record.Data, &fact); err != nil {
			return revision, err
		}
		if fact.Result != nil {
			return revision, agentsession.ErrCommitUnknown
		}
	}
	return log.Log.Append(ctx, revision, records...)
}

func TestChildEffectVerificationRoutesAfterRootCompletionWithoutExecution(t *testing.T) {
	for _, kind := range []string{agentrun.AgentKindIDE, agentrun.AgentKindInteractiveStory} {
		for _, action := range []string{"executed", "task_aborted"} {
			t.Run(string(kind)+"/"+action, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				store := agentsession.Memory()
				var effects atomic.Int32
				tool, err := agent.InferTool("external_write", "Apply an external operation", func(context.Context, struct{}) (agent.ToolResult, error) {
					effects.Add(1)
					return agent.ToolResult{Status: agent.ToolResultSuccess, ModelContent: "applied"}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
				tools, err := agent.StaticTools(agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
					Source: agent.ToolSourceWrite, Execution: agent.ToolExecutionSessionExclusive,
					MutationScope: agent.ToolMutationExternal, PostCheck: agent.ToolPostCheckExternalReceipt,
					Recovery: agent.ToolRecoveryNonIdempotent, ResultProjection: agent.ToolResultBoundedModelContext,
					ResultRetention: agent.ToolResultDeferred, Steering: agent.SteeringFinishCurrent, MaxResultBytes: 1 << 20,
				}})
				if err != nil {
					t.Fatal(err)
				}
				model := &publicBackendTestModel{responses: []*agent.Message{
					agent.AssistantMessage("root finished", nil),
					agent.AssistantMessage("", []agent.ToolCall{{ID: "original-call", Type: "function", Function: agent.FunctionCall{Name: "external_write", Arguments: `{}`}}}),
				}}
				definition := agent.Definition{Name: "test", Model: model, Tools: tools, Permission: agentpermission.FullAccess()}
				owner, err := agent.New(ctx, definition, agent.WithSessionStore(unconfirmedEffectStore{store}))
				if err != nil {
					t.Fatal(err)
				}
				options := agentrun.Options{AgentKind: kind, ProjectID: "project", Workspace: t.TempDir(), SessionID: "session", StoryID: "story", BranchID: "main"}
				backend := &publicBackend{agent: owner}
				root, _, err := backend.openSession(ctx, options)
				if err != nil {
					t.Fatal(err)
				}
				rootRun, err := root.Run(ctx, agent.Text("delegate work"))
				if err != nil {
					t.Fatal(err)
				}
				if result, err := rootRun.Wait(ctx); err != nil || result.Status != agent.ResultCompleted {
					t.Fatalf("root=%#v error=%v", result, err)
				}
				childKey := agent.NamedSession("child")
				childKey.Attributes, err = agent.ChildSessionAttributes(root.Key())
				if err != nil {
					t.Fatal(err)
				}
				child, err := owner.Session(ctx, childKey)
				if err != nil {
					t.Fatal(err)
				}
				childRun, err := child.Run(ctx, agent.Text("apply operation"))
				if err != nil {
					t.Fatal(err)
				}
				if result, err := childRun.Wait(ctx); !errors.Is(err, agentsession.ErrCommitUnknown) || result.Status != agent.ResultSuspended {
					t.Fatalf("child=%#v error=%v", result, err)
				}
				_ = owner.Close(ctx)
				coldModel := &publicBackendTestModel{responses: []*agent.Message{agent.AssistantMessage("continued", nil)}}
				definition.Model = coldModel
				owner, err = agent.New(ctx, definition, agent.WithSessionStore(store))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = owner.Close(context.Background()) })
				backend = &publicBackend{agent: owner}
				runtime := &Runtime{public: backend}
				status, err := backend.status(ctx, options)
				if err != nil {
					t.Fatal(err)
				}
				if status.Phase != agentrun.RunPhaseIdle || len(status.PendingInteractions) != 1 || status.PendingInteractions[0].Verification == nil {
					t.Fatalf("status=%#v", status)
				}
				id := status.PendingInteractions[0].ID
				unknown := []agentconversation.HostAskAnswer{{QuestionID: "effect", SelectedOptionIDs: []string{"unknown"}}}
				result, err := runtime.ResolveAsk(ctx, options, id, session.AskAnswered, unknown, "")
				if err != nil || result.Status != session.AskPending {
					t.Fatalf("unknown=%#v error=%v", result, err)
				}
				result, err = runtime.ResolveAsk(ctx, options, id, session.AskCancelled, nil, "user_cancelled")
				if err != nil || result.Status != session.AskPending {
					t.Fatalf("cancel question=%#v error=%v", result, err)
				}
				answerStatus, answerReason := session.AskAnswered, ""
				answers := []agentconversation.HostAskAnswer{{QuestionID: "effect", SelectedOptionIDs: []string{"executed"}}}
				if action == "task_aborted" {
					answerStatus, answerReason, answers = session.AskCancelled, action, nil
				}
				for attempt := 0; attempt < 2; attempt++ {
					result, err = runtime.ResolveAsk(ctx, options, id, answerStatus, answers, answerReason)
					if err != nil || result.Status != answerStatus {
						t.Fatalf("answer=%#v error=%v", result, err)
					}
				}
				status, err = backend.status(ctx, options)
				if err != nil || len(status.PendingInteractions) != 0 {
					t.Fatalf("pending=%#v error=%v", status.PendingInteractions, err)
				}
				if effects.Load() != 1 || len(coldModel.inputs) != 0 {
					t.Fatal("interaction handling executed a model or repeated the external effect")
				}
			})
		}
	}
}
