package interactiveapp

import (
	"bytes"
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
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	agenttoolruntime "denova/internal/agents/toolruntime"
	producttools "denova/internal/agents/tools"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/project"

	"github.com/alfredxw/denova/agent"
	sdkexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentstream "github.com/alfredxw/denova/agent/model/stream"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agenttool "github.com/alfredxw/denova/agent/tool"
	agentpermission "github.com/alfredxw/denova/agent/tool/permission"
)

// This uses the native Game tools and the same embedded Story journal as the
// product, then discards every process object and derived index before resume.
func TestGameDraftPauseAndColdResumeThroughCanonicalRuntime(t *testing.T) {
	for _, scenario := range []string{"normal-flow", "prose-only", "repair-missing-modules", "accepted-output", "prose-with-tool", "released-draft"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			workspace, dataDir := t.TempDir(), t.TempDir()
			const originalContext = "Original accepted game instructions"
			const changedContext = "Changed game instructions for the next turn"
			if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(originalContext), 0600); err != nil {
				t.Fatal(err)
			}
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
			invalidSubmission := draftSubmissionMessage("invalid", `{"state_changes":"not-an-array"}`)
			responses := []*agentschema.Message{
				agentschema.AssistantMessage("", []agentschema.ToolCall{{ID: "locked-rule", Type: "function", Function: agentschema.FunctionCall{Name: "prepare_interactive_turn", Arguments: `{"action":"Open the gate","intent":"Reach the path","challenge":"The gate is stuck","cost":"Making noise","state":"Standing by the gate","difficulty":"normal","outcomes":{"critical_success":{"result":"The gate opens"},"success":{"result":"The gate opens"},"failure":{"result":"The gate opens with noise"},"critical_failure":{"result":"The gate opens with loud noise"}}}`}}}),
			}
			if scenario == "prose-with-tool" {
				invalidSubmission.Content = narrative
			} else {
				responses = append(responses, agentschema.AssistantMessage(narrative, nil))
			}
			if scenario != "prose-only" {
				responses = append(responses, invalidSubmission, draftSubmissionMessage("partial", partialArgs))
			}
			if scenario == "normal-flow" {
				responses = append(responses, draftSubmissionMessage("repair", repairArgs))
			}
			model := &draftRecoveryModel{responses: responses, blocked: make(chan struct{})}
			var submissions atomic.Int32
			var rulings atomic.Int32
			legacy := scenario == "released-draft"
			newCycle := func(request agentchat.ChatRequest) agentexecution.Cycle {
				conversation := NewConversation(store, "", workspace, story.ID, "main", request.Message, 800, cfg)
				contextSource, err := agentlifecycle.NewProjectInstructionsContextSource(cfg, agentrun.AgentKindInteractiveStory, book.NewState(workspace))
				if err != nil {
					t.Fatal(err)
				}
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
				toolset, err := agenttool.StaticToolsIdentified(agentschema.CapabilityIdentity{Kind: "tools.test.draft", Version: 1}, definitions...)
				if err != nil {
					t.Fatal(err)
				}
				var protocol agentmiddleware.Middleware = agentinteractive.NewTurnProtocolMiddleware(agentinteractive.InteractiveStoryToolContext{
					TurnResultReady: conversation.InteractiveNarrativeReady, LoadNarrativeCandidate: conversation.LoadAgentNarrativeCandidate,
				})
				attempts := 1
				if scenario == "released-draft" {
					attempts = 4
				}
				if legacy {
					protocol = &releasedDraftProtocol{Middleware: protocol, conversation: conversation}
				}
				return agentexecution.Cycle{Definition: agent.Definition{
					Key: "test.game-draft", Name: "game", Model: model, Tools: toolset, Context: contextSource, Permission: agentpermission.FullAccess(),
					ModelIdentity: agentschema.CapabilityIdentity{Kind: "model.test.draft", Version: 1},
					Execution:     sdkexecution.ExecutionPolicy{ModelMaxAttempts: attempts},
					Middlewares:   []agentmiddleware.Middleware{protocol},
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
			if scenario == "normal-flow" {
				if outcome := operation.Wait(ctx); outcome.Status != agentrun.OutcomeCompleted {
					t.Fatalf("normal outcome=%+v", outcome)
				}
				for _, messages := range model.inputs[2:] {
					assertDraftNarrativeMessageOrder(t, messages, narrative)
					assertGameContinuationHistory(t, messages)
				}
				after, err := store.Snapshot(story.ID, "main")
				if err != nil || after.TurnCount != before.TurnCount+1 || after.CurrentTurn.Narrative != narrative || submissions.Load() != 2 || rulings.Load() != 1 {
					t.Fatalf("normal turn changed or repeated: count=%d submissions=%d rulings=%d error=%v", after.TurnCount, submissions.Load(), rulings.Load(), err)
				}
				return
			}
			select {
			case <-model.blocked:
			case <-ctx.Done():
				t.Fatal("model did not reach the pause boundary")
			}
			// Both schema rejection and a partial domain receipt must keep the
			// accepted prose available for the next tool submission, exactly once.
			if !legacy {
				for _, messages := range model.inputs[2:] {
					assertDraftNarrativeMessageOrder(t, messages, narrative)
					if scenario != "prose-with-tool" {
						assertGameContinuationHistory(t, messages)
					}
				}
			}
			status, err := runtime.RuntimeStatusProjection(ctx, cycle.Options)
			if err != nil {
				t.Fatal(err)
			}
			runID := status.ActiveOperation
			identity := interactive.DomainCommitIdentity{CommandID: "draft-command", OperationID: string(runID), Cycle: 1}
			draft, found, err := store.LoadTurnDraft(story.ID, "main", identity)
			if err != nil || !found || draft.Narrative != narrative || (draft.Submission == nil) != (scenario == "prose-only") {
				t.Fatalf("draft not accepted before pause: found=%t full_narrative=%t error=%v", found, draft.Narrative == narrative, err)
			}
			wantBeforePause := int32(1)
			if scenario == "prose-only" {
				wantBeforePause = 0
			}
			if submissions.Load() != wantBeforePause {
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
			if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte(changedContext), 0600); err != nil {
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
			legacy = false
			model = &draftRecoveryModel{responses: []*agentschema.Message{draftSubmissionMessage("repair", repairArgs)}, blocked: make(chan struct{})}
			if scenario == "prose-only" {
				model.responses = []*agentschema.Message{draftSubmissionMessage("partial", partialArgs), draftSubmissionMessage("repair", repairArgs)}
			}
			runtime = newRuntime()
			storyPath := filepath.Join(workspace, "interactive", "story", "story-"+story.ID+".jsonl")
			beforeInspection, err := os.ReadFile(storyPath)
			if err != nil {
				t.Fatal(err)
			}
			key, err := (agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: record.ID, StoryID: story.ID, BranchID: "main", Workspace: workspace}).AgentSessionKey()
			if err != nil {
				t.Fatal(err)
			}
			reader, err := interactive.OpenAgentReader(ctx, workspace, dataDir, story.ID, key)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			afterInspection, err := os.ReadFile(storyPath)
			if err != nil || !bytes.Equal(beforeInspection, afterInspection) {
				t.Fatalf("read-only recovery inspection changed the Story journal: %v", err)
			}
			observation, err := runtime.OpenRecoveryObservation(ctx, cycle.Options)
			if err != nil {
				t.Fatal(err)
			}
			defer observation.Close()
			if scenario == "released-draft" {
				afterOpen, err := os.ReadFile(storyPath)
				if err != nil || !bytes.Equal(beforeInspection, afterOpen) {
					t.Fatalf("opening released recovery changed the Story journal: %v", err)
				}
			}
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
			switch scenario {
			case "accepted-output":
				wantSubmissions, wantRequests = 1, 0
			case "prose-only":
				wantRequests = 2
			}
			if submissions.Load() != wantSubmissions || model.count() != wantRequests {
				t.Fatalf("confirmed tool or input replayed: submissions=%d requests=%d", submissions.Load(), model.count())
			}
			if wantRequests > 0 && !containsMessageContent(model.inputs[0], "retry_modules") {
				t.Fatal("cold model input did not retain repair context")
			}
			if wantRequests > 0 {
				assertDraftNarrativeMessageOrder(t, model.inputs[0], narrative)
				if scenario != "prose-with-tool" && scenario != "released-draft" {
					assertGameContinuationHistory(t, model.inputs[0])
				}
			}
			if wantRequests > 0 && (!containsMessageContent(model.inputs[0], originalContext) || containsMessageContent(model.inputs[0], changedContext)) {
				t.Fatal("cold model input replaced the accepted project instructions")
			}
			if _, found, err := store.LoadTurnDraft(story.ID, "main", identity); err != nil || found {
				t.Fatalf("completed draft remained pending: found=%t error=%v", found, err)
			}
		})
	}
}

func assertGameContinuationHistory(t *testing.T, messages []*agentschema.Message) {
	t.Helper()
	count := 0
	for _, message := range messages {
		if agentschema.IsAgentContinuationMessage(message) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("accepted continuation count=%d, want 1", count)
	}
}

// Reproduce v0.6.1's split acceptance: save the prose draft, then reject the
// response through ephemeral feedback. Its tool-only journal needs upgrading.
type releasedDraftProtocol struct {
	agentmiddleware.Middleware
	conversation *Conversation
}

func (m *releasedDraftProtocol) ReviewModelOutput(ctx context.Context, output agentmodel.ModelOutput) (agentmodel.ModelOutputReview, error) {
	if agentinteractive.OutputContainsNarrativeCandidate(output.Message) {
		if err := m.conversation.AcceptNarrativeCandidate(ctx, output.Message.Content); err != nil {
			return agentmodel.ModelOutputReview{}, err
		}
	}
	if len(output.Message.ToolCalls) > 0 {
		return agentmodel.ModelOutputReview{Action: agentmodel.ModelOutputAccept}, nil
	}
	return agentmodel.ModelOutputReview{Action: agentmodel.ModelOutputRepair, Feedback: []agentschema.ContextFragment{{
		Source: "test.v0.6.1", Purpose: "retained narrative", Role: agentschema.Assistant,
		Content: output.Message.Content, HardLimit: 64 << 10,
	}, {
		Source: "test.v0.6.1", Purpose: "missing modules", Role: agentschema.User,
		Content: "Submit the remaining retry_modules", HardLimit: 1024,
	}}}, nil
}

func assertDraftNarrativeMessageOrder(t *testing.T, messages []*agentschema.Message, narrative string) {
	t.Helper()
	copies, narrativeIndex, feedbackIndex := 0, -1, -1
	for index, message := range messages {
		if message == nil {
			continue
		}
		if message.Role == agentschema.Assistant && strings.Contains(message.Content, narrative) {
			copies += strings.Count(message.Content, narrative)
			narrativeIndex = index
		}
		if message.Role == agentschema.User && strings.Contains(message.Content, "source=backend completion guard") {
			feedbackIndex = index
		}
	}
	if copies != 1 {
		t.Fatalf("model request contains %d copies of the complete accepted narrative, want 1", copies)
	}
	if feedbackIndex >= 0 && feedbackIndex < narrativeIndex {
		t.Fatalf("submission feedback at %d precedes the accepted narrative at %d", feedbackIndex, narrativeIndex)
	}
	last := messages[len(messages)-1]
	if last.Role != agentschema.User && last.Role != agentschema.ToolRole {
		t.Fatalf("submission request ends with %q, want user feedback or a tool result", last.Role)
	}
}

func draftSubmissionMessage(id, args string) *agentschema.Message {
	return agentschema.AssistantMessage("", []agentschema.ToolCall{{ID: id, Type: "function", Function: agentschema.FunctionCall{Name: producttools.SubmitInteractiveTurnToolName, Arguments: args}}})
}

type draftRecoveryModel struct {
	mu        sync.Mutex
	responses []*agentschema.Message
	inputs    [][]*agentschema.Message
	blocked   chan struct{}
}

func (model *draftRecoveryModel) Generate(ctx context.Context, messages []*agentschema.Message, _ ...agentmodel.ModelOption) (*agentschema.Message, error) {
	model.mu.Lock()
	index := len(model.inputs)
	model.inputs = append(model.inputs, append([]*agentschema.Message(nil), messages...))
	if index < len(model.responses) {
		response := agentschema.CloneMessage(model.responses[index])
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

func (model *draftRecoveryModel) Stream(ctx context.Context, messages []*agentschema.Message, options ...agentmodel.ModelOption) (*agentstream.StreamReader[*agentschema.Message], error) {
	response, err := model.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	return agentstream.StreamReaderFromArray([]*agentschema.Message{response}), nil
}

func (model *draftRecoveryModel) count() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return len(model.inputs)
}
