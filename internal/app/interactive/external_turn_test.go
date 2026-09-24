package interactiveapp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents"
	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/conversationconfig"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
)

type gameAdapterFunc func(context.Context, external.Input, external.Host) (external.Result, error)

func (gameAdapterFunc) Version() string { return "test" }
func (fn gameAdapterFunc) Run(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
	return fn(ctx, input, host)
}

func externalGameFixture(t *testing.T) (*interactive.Store, string, config.Config) {
	t.Helper()
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	t.Cleanup(func() { _ = store.Close() })
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Runtime switching", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Workspace: workspace, ProjectID: "project-test", ProjectStoreDir: store.Root(), Language: "en-US"}
	return store, story.ID, cfg
}

func externalGameForTest(t *testing.T, store *interactive.Store, story string, cfg config.Config, request agentchat.ChatRequest, adapter external.Adapter, configure ...func(*Conversation)) *ExternalTurn {
	t.Helper()
	c := NewConversation(store, t.TempDir(), cfg.Workspace, story, "main", request.Message, 800, &cfg)
	for _, apply := range configure {
		apply(c)
	}
	assembly, err := agents.BuildExternalGameAssembly(t.Context(), &cfg, nil, prompts.InteractiveStorySystemInstructionInput{}, agents.AgentHostCapabilities{Interactive: true}, agentinteractive.InteractiveStoryToolContext{
		Store: store, StoryID: story, BranchID: "main", PrepareTurn: c.PrepareInteractiveTurn, SubmitTurnResult: c.SubmitTurnResult,
		TurnResultReady: c.InteractiveNarrativeReady, LoadNarrativeCandidate: c.LoadNarrativeCandidate, AcceptNarrativeCandidate: c.AcceptNarrativeCandidate,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := StartExternalTurn(t.Context(), ExternalTurnConfig{Conversation: c, Request: request, Config: cfg, Assembly: assembly, Adapter: adapter})
	if err != nil {
		t.Fatal(err)
	}
	return turn
}

const gameStateArgs = `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前事件","value":"The gate opens"},{"op":"replace","actor_id":"story","field_id":"当前详细地点","value":"By the gate"}]}`
const gameCompleteArgs = `{"state_changes":[{"op":"replace","actor_id":"story","field_id":"当前事件","value":"The gate opens"}],"choices":["Enter","Observe","Listen","Inspect","Wait"]}`

func gameHostCall(t *testing.T, ctx context.Context, host external.Host, id, name, arguments string) external.ToolResult {
	t.Helper()
	result, err := host.CallTool(ctx, external.ToolCall{ID: id, Name: name, Arguments: []byte(arguments)})
	if err != nil || !result.Success {
		t.Fatalf("%s: result=%+v err=%v", name, result, err)
	}
	return result
}

func TestGameSwitchesNativeCodexClaudeNativeWithoutRewritingHistory(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	runtime := agentexecution.NewEphemeralRuntime()
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	native := func(command, text string) {
		c := NewConversation(store, t.TempDir(), cfg.Workspace, story, "main", command, 800, &cfg)
		model := &publicGameHistoryModel{narrative: text}
		op, err := runtime.Start(t.Context(), agentexecution.StartRequest{Cycle: agentexecution.Cycle{
			Definition:   agent.Definition{Key: "test.native", Name: "game", Model: model, ModelIdentity: agent.CapabilityIdentity{Kind: "model.test", Version: 1}, Middlewares: []agent.Middleware{gameSubmissionForTest(t, c, command, text)}},
			Conversation: c, Request: agentchat.ChatRequest{CommandID: command, Message: command}, Options: agentrun.Options{AgentKind: config.AgentKindInteractiveStory, ProjectID: cfg.ProjectID, Workspace: cfg.Workspace, StoryID: story, BranchID: "main", Mode: "interactive"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if outcome := op.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("Native outcome: %+v", outcome)
		}
		if command == "native-last" {
			var history strings.Builder
			for _, message := range model.lastInput(t) {
				history.WriteString(message.Content)
			}
			if !strings.Contains(history.String(), "The gate opens with codex.") || !strings.Contains(history.String(), "The gate opens with claude.") {
				t.Fatal("Native did not receive external turns")
			}
		}
	}
	native("native-first", "The gate is closed.")
	path := filepath.Join(store.Root(), "interactive", "story", "story-"+story+".jsonl")
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	selection := conversationconfig.Default(&cfg, config.AgentKindInteractiveStory)
	saved, err := store.EnsureBranchRuntimeConfig(story, "main", selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []config.RuntimeID{config.RuntimeCodex, config.RuntimeClaude} {
		runtimeSelection := config.RuntimeSelection{Kind: kind}
		if kind == config.RuntimeCodex {
			runtimeSelection.Codex = &config.CodexRuntimeSettings{Model: "test-model", Sandbox: config.CodexWorkspaceWrite}
		} else {
			runtimeSelection.Claude = &config.ClaudeRuntimeSettings{Model: "test-model"}
		}
		cfg.ActiveAgentRuntime = &runtimeSelection
		selection.Runtime = &runtimeSelection
		saved, err = store.SetBranchRuntimeConfig(story, "main", selection, saved.Revision)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		turn := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: string(kind), Message: "Continue"}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
			calls++
			if calls == 1 {
				found := false
				for _, message := range input.History {
					found = found || strings.Contains(message.Text, "The gate is closed.")
				}
				if !found {
					t.Fatal("external engine lost Native history")
				}
				// Finishing without submission requires a second attempt, using the retained prose.
				return external.Result{Text: "The gate opens with " + string(kind) + "."}, nil
			}
			if calls != 2 {
				t.Fatal("unexpected repeated repair")
			}
			gameHostCall(t, ctx, host, "submit", "submit_interactive_turn", gameCompleteArgs)
			return external.Result{}, ctx.Err()
		}))
		if outcome := turn.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
			t.Fatalf("external outcome: %+v", outcome)
		}
		next, err := os.ReadFile(path)
		if err != nil || !bytes.HasPrefix(next, previous) {
			t.Fatalf("history was rewritten: %v", err)
		}
		previous = next
	}
	cfg.ActiveAgentRuntime = nil
	selection.Runtime = nil
	if _, err := store.SetBranchRuntimeConfig(story, "main", selection, saved.Revision); err != nil {
		t.Fatal(err)
	}
	native("native-last", "The journey continues.")
	snapshot, err := store.Snapshot(story, "main")
	if err != nil || snapshot.TurnCount != 4 {
		t.Fatalf("same branch: turns=%d err=%v", snapshot.TurnCount, err)
	}
	next, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(next, previous) {
		t.Fatalf("return to Native rewrote history: %v", err)
	}
}

func TestExternalGameResumesDraftAndKeepsFirstNarrativeAndDice(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
	ruleArgs := `{"action":"Open the gate","intent":"Reach the path","challenge":"The gate is stuck","cost":"Making noise","state":"Standing by the gate","difficulty":"normal","outcomes":{"critical_success":{"result":"The gate opens"},"success":{"result":"The gate opens"},"failure":{"result":"The gate opens with noise"},"critical_failure":{"result":"The gate opens with loud noise"}}}`
	first := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "interrupted", Message: "Open the gate"}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		rule := gameHostCall(t, ctx, host, "rule", "prepare_interactive_turn", ruleArgs)
		if again := gameHostCall(t, ctx, host, "rule", "prepare_interactive_turn", ruleArgs); again.Text != rule.Text {
			t.Fatal("duplicate call rerolled dice")
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "The first narrative."}}); err != nil {
			t.Fatal(err)
		}
		gameHostCall(t, ctx, host, "partial", "submit_interactive_turn", gameStateArgs)
		return external.Result{}, errors.New("connection lost")
	}))
	if outcome := first.Wait(t.Context()); outcome.Status != agentrun.OutcomeFailed {
		t.Fatalf("expected interruption: %+v", outcome)
	}
	identity := interactive.DomainCommitIdentity{CommandID: "interrupted", OperationID: string(first.OperationID()), Cycle: 1}
	draft, found, err := store.LoadTurnDraft(story, "main", identity)
	if err != nil || !found || draft.RuleResolution == nil || draft.Narrative != "The first narrative." {
		t.Fatalf("draft: %+v err=%v", draft, err)
	}
	interruption, err := ExternalTurnInterruption(store, story, "main")
	if err != nil || interruption == nil {
		t.Fatalf("interruption: %v", err)
	}
	resumed := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "resume", ResumeInterruptionID: interruption.ID}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		if !strings.Contains(input.Text, "The first narrative.") {
			t.Fatal("resume lost locked prose")
		}
		gameHostCall(t, ctx, host, "rule-again", "prepare_interactive_turn", ruleArgs)
		_ = host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "Unwanted replacement."}})
		gameHostCall(t, ctx, host, "finish", "submit_interactive_turn", `{"choices":["Enter","Observe","Listen","Inspect","Wait"]}`)
		return external.Result{}, ctx.Err()
	}))
	if outcome := resumed.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != draft.Narrative {
		t.Fatalf("resume: %+v", outcome)
	}
	snapshot, err := store.Snapshot(story, "main")
	if err != nil || snapshot.TurnCount != 1 || snapshot.CurrentTurn.RuleResolution.ID != draft.RuleResolution.ID {
		t.Fatalf("resume changed dice or turn count: %+v err=%v", snapshot.CurrentTurn, err)
	}
}

func TestExternalGameRejectsSubmissionUntilNarrative(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
	var events []agentrun.Event
	turn := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "early-submission", Message: "Continue"}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		early := external.ToolCall{ID: "too-early", Name: "submit_interactive_turn", Arguments: []byte(gameCompleteArgs)}
		for range 2 {
			result, err := host.CallTool(ctx, early)
			if err != nil || result.Success || !strings.Contains(result.Text, "before") {
				t.Fatalf("early submission must request prose without accepting modules: %+v, %v", result, err)
			}
		}
		for _, event := range events {
			if event.Type == "tool_call" || event.Type == "tool_result" {
				t.Fatal("a rejected precondition was displayed as an executed Game tool")
			}
		}
		if err := host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "The gate opens."}}); err != nil {
			t.Fatal(err)
		}
		gameHostCall(t, ctx, host, "state", "submit_interactive_turn", gameStateArgs)
		gameHostCall(t, ctx, host, "choices", "submit_interactive_turn", `{"choices":["Enter","Observe","Listen","Inspect","Wait"]}`)
		return external.Result{}, ctx.Err()
	}))
	turn.config.Emit = func(event agentrun.Event) { events = append(events, event) }
	turn.config.Guidance = []agentchat.ChatRequest{{CommandID: "accepted-guidance", Message: "Keep the gate intact"}}
	if outcome := turn.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted || outcome.Content != "The gate opens." {
		t.Fatalf("narrative completion: %+v", outcome)
	}
	snapshot, err := store.Snapshot(story, "main")
	if err != nil || snapshot.TurnCount != 1 || snapshot.CurrentTurn.Narrative != "The gate opens." {
		t.Fatalf("repaired turn: %+v, %v", snapshot.CurrentTurn, err)
	}
	if got := snapshot.CurrentTurn.DisplayEvents; len(got) != 3 || got[0].Role != "narrative" || got[1].Status != "success" || got[2].Status != "success" {
		t.Fatalf("submission order did not survive persistence: %+v", got)
	}
	replayed := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "early-submission", Message: "Continue"}, gameAdapterFunc(func(context.Context, external.Input, external.Host) (external.Result, error) {
		t.Fatal("recovery reran a committed Game turn")
		return external.Result{}, nil
	}))
	replayed.config.Guidance = append(turn.config.Guidance, agentchat.ChatRequest{CommandID: "late-guidance", Message: "Inspect the courtyard next"})
	if outcome := replayed.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("replayed completion: %+v", outcome)
	}
	if receipt, ok := any(replayed).(interface{ ConsumedGuidance() int }); !ok || receipt.ConsumedGuidance() != 1 {
		t.Fatal("completed Game turn acknowledged unconsumed guidance")
	}
}

func TestExternalGameRegenerationResumesItsOriginalTarget(t *testing.T) {
	store, story, cfg := externalGameFixture(t)
	seed := NewConversation(store, "", cfg.Workspace, story, "main", "Open the gate", 800, &cfg)
	submitTestTurnResult(t, seed, "Open the gate", "The gate opens")
	if err := commitInteractiveAssistantForTest(t, seed, "Original narrative.", ""); err != nil {
		t.Fatal(err)
	}
	before, err := store.Snapshot(story, "main")
	if err != nil {
		t.Fatal(err)
	}
	target := before.CurrentTurn.ID
	cfg.ActiveAgentRuntime = &config.RuntimeSelection{Kind: config.RuntimeClaude, Claude: &config.ClaudeRuntimeSettings{Model: "test"}}
	first := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "regenerate", Message: "Open the gate"}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		_ = host.Emit(agentrun.Event{Type: "chunk", Data: map[string]any{"content": "Replacement narrative."}})
		gameHostCall(t, ctx, host, "state", "submit_interactive_turn", gameStateArgs)
		return external.Result{}, errors.New("connection lost")
	}), func(c *Conversation) { c.WithRegenerateTarget(target) })
	if outcome := first.Wait(t.Context()); outcome.Status != agentrun.OutcomeFailed {
		t.Fatalf("regeneration interruption: %+v", outcome)
	}
	pending, err := ExternalTurnInterruption(store, story, "main")
	if err != nil || pending == nil {
		t.Fatalf("regeneration interruption missing: %v", err)
	}
	restoredTarget, err := ExternalTurnReplacement(store, story, "main", pending.ID)
	if err != nil || restoredTarget != target {
		t.Fatalf("replacement target: %q %v", restoredTarget, err)
	}
	resumed := externalGameForTest(t, store, story, cfg, agentchat.ChatRequest{CommandID: "resume-regenerate", Message: "Continue", ResumeInterruptionID: pending.ID}, gameAdapterFunc(func(ctx context.Context, input external.Input, host external.Host) (external.Result, error) {
		gameHostCall(t, ctx, host, "choices", "submit_interactive_turn", `{"choices":["Enter","Observe","Listen","Inspect","Wait"]}`)
		return external.Result{}, ctx.Err()
	}), func(c *Conversation) { c.WithRegenerateTarget(restoredTarget) })
	if outcome := resumed.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("resume regeneration: %+v", outcome)
	}
	after, err := store.Snapshot(story, "main")
	if err != nil || after.TurnCount != 1 || after.CurrentTurn.Narrative != "Replacement narrative." {
		t.Fatalf("regeneration appended another turn: %+v %v", after.CurrentTurn, err)
	}
}
