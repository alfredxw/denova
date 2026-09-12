package interactiveapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/canonicalstore"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	"denova/internal/interactive"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
)

// This uses the native Game tools and the same embedded Story journal as the
// product, then discards every process object and derived index before resume.
func TestGameDraftPauseAndColdResumeThroughCanonicalRuntime(t *testing.T) {
	for _, scenario := range []string{"repair-missing-modules", "accepted-output"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			workspace, dataDir := t.TempDir(), t.TempDir()
			registry := project.NewRegistry(dataDir)
			record, err := registry.Add(workspace, project.TypeGeneral, "Game draft recovery")
			if err != nil {
				t.Fatal(err)
			}
			store := interactive.NewStore(workspace)
			story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Draft recovery", StoryTellerID: "classic", PlanningMode: interactive.StoryPlanningModeEnabled})
			if err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{Workspace: workspace, ProjectID: record.ID}
			seed := NewConversation(store, "", workspace, story.ID, "main", "Begin", 800, cfg)
			submitTestTurnResult(t, seed, "Begin", "Establish the scene")
			if err := commitInteractiveAssistantForTest(t, seed, "The gate is closed.", ""); err != nil {
				t.Fatal(err)
			}
			before, err := store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdateBranchPlan(story.ID, interactive.UpdateBranchPlanRequest{BranchID: "main", BaseRevision: before.BranchPlan.Revision,
				Markdown: "## Direction\n\nOld direction.\n\n## Next\n\nOld next."})
			if err != nil {
				t.Fatal(err)
			}
			narrative := strings.TrimSpace(strings.Repeat("The gate opens and the path becomes clear. ", 700))
			partialArgs := `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前事件","value":"The gate is open"}],"plan_update":{"mode":"replace_sections","sections":[{"heading":"Direction","markdown":"Accepted direction."},{"heading":"Next","markdown":"## Invalid nested section\nRejected"}]}}`
			repairArgs := `{"choices":["Enter","Observe","Listen","Inspect","Wait"],"plan_update":{"mode":"replace_sections","sections":[{"heading":"Next","markdown":"Repaired next."}]}}`
			model := &draftRecoveryModel{responses: []*agent.Message{
				agent.AssistantMessage("", []agent.ToolCall{{ID: "locked-rule", Type: "function", Function: agent.FunctionCall{Name: "prepare_interactive_turn", Arguments: `{"action":"Open the gate","intent":"Reach the path","challenge":"The gate is stuck","cost":"Making noise","state":"Standing by the gate","difficulty":"normal","outcomes":{"critical_success":{"result":"The gate opens"},"success":{"result":"The gate opens"},"failure":{"result":"The gate opens with noise"},"critical_failure":{"result":"The gate opens with loud noise"}}}`}}}),
				agent.AssistantMessage(narrative, nil), draftSubmissionMessage("partial", partialArgs),
			}, blocked: make(chan struct{})}
			var submissions atomic.Int32
			var rulings atomic.Int32
			newCycle := func(request agentchat.ChatRequest) agentexecution.Cycle {
				conversation := NewConversation(store, "", workspace, story.ID, "main", request.Message, 800, cfg)
				definitions, err := producttools.NewInteractiveTurn(producttools.InteractiveContext{
					Store: store, StoryID: story.ID, BranchID: "main", RequestTurnCompletion: agentinteractive.RequestTurnCompletion,
					PrepareTurn: func(ctx context.Context, request interactive.TurnCheckRequest) (interactive.RuleResolution, error) {
						rulings.Add(1)
						return conversation.PrepareInteractiveTurn(ctx, request)
					},
					SubmitTurnResult: func(ctx context.Context, input interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
						submissions.Add(1)
						return conversation.SubmitTurnResult(ctx, input)
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				toolset, err := agent.StaticToolsIdentified(agent.CapabilityIdentity{Kind: "tools.test.draft", Version: 1}, definitions...)
				if err != nil {
					t.Fatal(err)
				}
				return agentexecution.Cycle{Definition: agent.Definition{
					Key: "test.game-draft", Name: "game", Model: model, Tools: toolset, Permission: agentpermission.FullAccess(),
					ModelIdentity: agent.CapabilityIdentity{Kind: "model.test.draft", Version: 1},
					Execution:     agent.ExecutionPolicy{ModelMaxAttempts: 4},
					Middlewares: []agent.Middleware{agentinteractive.NewTurnProtocolMiddleware(agentinteractive.InteractiveStoryToolContext{
						TurnResultReady: conversation.InteractiveNarrativeReady, LoadNarrativeCandidate: conversation.LoadNarrativeCandidate, AcceptNarrativeCandidate: conversation.AcceptNarrativeCandidate,
					})},
				}, Conversation: conversation, Request: request, Options: agentrun.Options{
					AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: record.ID, StoryID: story.ID, BranchID: "main", Workspace: workspace, Mode: "interactive",
				}}
			}
			newRuntime := func() *agentexecution.Runtime {
				journalStore, err := canonicalstore.New(dataDir, registry)
				if err != nil {
					t.Fatal(err)
				}
				runtime, err := agentexecution.NewAgentRuntime(ctx, dataDir, agentexecution.WithSessionStore(journalStore),
					agentexecution.WithProfiles(publicGameTestProfile{prepare: func(_ context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
						return newCycle(request.Request), nil
					}}), agentexecution.WithToolMutationApplier(func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil }))
				if err != nil {
					t.Fatal(err)
				}
				return runtime
			}
			runtime := newRuntime()
			t.Cleanup(func() { _ = runtime.Close(context.Background()); _ = store.Close() })
			cycle := newCycle(agentchat.ChatRequest{CommandID: "draft-command", Message: "Open the gate"})
			operation, err := runtime.Start(ctx, agentexecution.StartRequest{Cycle: cycle})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-model.blocked:
			case <-ctx.Done():
				t.Fatal("model did not reach the pause boundary")
			}
			status, err := runtime.RuntimeStatusProjection(ctx, cycle.Options)
			if err != nil {
				t.Fatal(err)
			}
			runID := status.ActiveOperation
			identity := interactive.DomainCommitIdentity{CommandID: "draft-command", OperationID: string(runID), Cycle: 1}
			draft, found, err := store.LoadTurnDraft(story.ID, "main", identity)
			if err != nil || !found || draft.Narrative != narrative || draft.Submission == nil {
				t.Fatalf("draft not accepted before pause: found=%t full_narrative=%t error=%v", found, draft.Narrative == narrative, err)
			}
			if submissions.Load() != 1 {
				t.Fatalf("submissions=%d", submissions.Load())
			}
			if draft.RuleResolution == nil {
				t.Fatal("rule resolution was not saved with the draft")
			}
			lockedRule := *draft.RuleResolution
			if _, err := runtime.SubmitCommand(ctx, agentexecution.CommandRequest{Kind: agentexecution.CommandSuspend, CommandID: "pause-draft", OperationID: runID, Options: cycle.Options}); err != nil {
				t.Fatal(err)
			}
			if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeSuspended {
				t.Fatalf("pause outcome=%+v", outcome)
			}
			if err := runtime.Close(ctx); err != nil {
				t.Fatal(err)
			}
			if scenario == "accepted-output" {
				// Simulate a process exit after the product accepted the final modules,
				// before the loop could publish its final output receipt.
				accepted := NewConversation(store, "", workspace, story.ID, "main", "Open the gate", 800, cfg)
				accepted.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "draft-command", OperationID: runID, Cycle: 1})
				receipt, err := accepted.SubmitTurnResult(ctx, interactive.DecodeInteractiveTurnSubmissionInput(repairArgs))
				if err != nil || !receipt.Ready {
					t.Fatalf("accept complete draft: receipt=%+v error=%v", receipt, err)
				}
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			// Only remove disposable indexes from these isolated test directories.
			for _, root := range []string{workspace, dataDir} {
				if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".idx.json") {
						return os.Remove(path)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			store = interactive.NewStore(workspace)
			model = &draftRecoveryModel{responses: []*agent.Message{draftSubmissionMessage("repair", repairArgs)}, blocked: make(chan struct{})}
			runtime = newRuntime()
			observation, err := runtime.OpenRecoveryObservation(ctx, cycle.Options)
			if err != nil {
				t.Fatal(err)
			}
			defer observation.Close()
			restored := observation.InitialStatus()
			if restored.Phase != agentrun.RunPhaseSuspended || restored.ActiveOperation != runID || model.count() != 0 {
				t.Fatalf("cold inspection changed execution: %+v calls=%d", restored, model.count())
			}
			draft, found, err = store.LoadTurnDraft(story.ID, "main", identity)
			if err != nil || !found || draft.Narrative != narrative {
				t.Fatalf("cold draft lost: %v", err)
			}
			actions := agentexecution.RuntimeRecoveryActions(restored)
			if len(actions) != 2 {
				t.Fatalf("actions=%+v", actions)
			}
			if _, err := observation.Resume(ctx, actions[0], "new-display", nil); err != nil {
				t.Fatal(err)
			}
			if outcome := observation.Wait(ctx, nil); outcome.Status != agentrun.OutcomeCompleted {
				t.Fatalf("resume outcome=%+v", outcome)
			}
			after, err := store.Snapshot(story.ID, "main")
			if err != nil {
				t.Fatal(err)
			}
			if after.TurnCount != before.TurnCount+1 || after.CurrentTurn.User != "Open the gate" || after.CurrentTurn.Narrative != narrative {
				t.Fatalf("canonical turn duplicated or changed: count=%d user=%q full_narrative=%t", after.TurnCount, after.CurrentTurn.User, after.CurrentTurn.Narrative == narrative)
			}
			if after.BranchPlan == nil || !strings.Contains(after.BranchPlan.Markdown, "Accepted direction.") || !strings.Contains(after.BranchPlan.Markdown, "Repaired next.") {
				t.Fatalf("partial plan lost: %+v", after.BranchPlan)
			}
			if rulings.Load() != 1 || after.CurrentTurn.RuleResolution == nil || after.CurrentTurn.RuleResolution.ID != lockedRule.ID || after.CurrentTurn.RuleResolution.Seed != lockedRule.Seed {
				t.Fatal("cold continuation changed or reran the locked rule")
			}
			wantSubmissions, wantRequests := int32(2), 1
			if scenario == "accepted-output" {
				wantSubmissions, wantRequests = 1, 0
			}
			if submissions.Load() != wantSubmissions || model.count() != wantRequests {
				t.Fatalf("confirmed tool or input replayed: submissions=%d requests=%d", submissions.Load(), model.count())
			}
			if wantRequests > 0 && (!containsMessageContent(model.inputs[0], "retry_modules") || !containsMessageContent(model.inputs[0], "already displayed")) {
				t.Fatal("cold model input did not retain repair context")
			}
			if _, found, err := store.LoadTurnDraft(story.ID, "main", identity); err != nil || found {
				t.Fatalf("completed draft remained pending: found=%t error=%v", found, err)
			}
		})
	}
}

func draftSubmissionMessage(id, args string) *agent.Message {
	return agent.AssistantMessage("", []agent.ToolCall{{ID: id, Type: "function", Function: agent.FunctionCall{Name: producttools.SubmitInteractiveTurnToolName, Arguments: args}}})
}

type draftRecoveryModel struct {
	mu        sync.Mutex
	responses []*agent.Message
	inputs    [][]*agent.Message
	blocked   chan struct{}
}

func (model *draftRecoveryModel) Generate(ctx context.Context, messages []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	model.mu.Lock()
	index := len(model.inputs)
	model.inputs = append(model.inputs, append([]*agent.Message(nil), messages...))
	if index < len(model.responses) {
		response := agent.CloneMessage(model.responses[index])
		model.mu.Unlock()
		return response, nil
	}
	if index == len(model.responses) {
		close(model.blocked)
	}
	model.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("unexpected additional Game model request")
	}
}

func (model *draftRecoveryModel) Stream(ctx context.Context, messages []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{response}), nil
}

func (model *draftRecoveryModel) count() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return len(model.inputs)
}
