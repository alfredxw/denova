package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
)

type externalCycleFunc func(context.Context) agentrun.Outcome

func (cycle externalCycleFunc) Wait(ctx context.Context) agentrun.Outcome { return cycle(ctx) }

func TestGameCapabilitiesReopenFromOnlyTheStoryJournal(t *testing.T) {
	store := interactive.NewStore(t.TempDir())
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Runtime Goal", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{ProjectID: "stable-project", AgentKind: "interactive_story", StoryID: story.ID, BranchID: "main", Mode: "interactive"}
	state, err := GameState(options, store)
	if err != nil {
		t.Fatal(err)
	}
	goal, err := state.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Complete and verify the scene"})
	if err != nil {
		t.Fatal(err)
	}
	if err := observePlanFixture(t, state, []agent.TodoItem{{ID: "verify", Text: "Verify the scene", Status: agent.TodoPending}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	journal := filepath.Join("interactive", "story", "story-"+story.ID+".jsonl")
	data, err := os.ReadFile(filepath.Join(store.Root(), journal))
	if err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(destination, journal)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, journal), data, 0600); err != nil {
		t.Fatal(err)
	}
	reopened := interactive.NewStore(destination)
	defer reopened.Close()
	state, err = GameState(options, reopened)
	if err != nil {
		t.Fatal(err)
	}
	got, present, err := state.Goal(t.Context())
	if err != nil || !present || !reflect.DeepEqual(got, goal) {
		t.Fatalf("restored Goal: %+v, %v", got, err)
	}
	raw, present, err := state.Read(t.Context(), agent.TodoCapability)
	if err != nil || !present {
		t.Fatalf("restored Todo missing: %v", err)
	}
	var todo agent.TodoState
	if err := json.Unmarshal(raw, &todo); err != nil {
		t.Fatal(err)
	}
	if todo.Revision != 1 || !reflect.DeepEqual(todo.Items, []agent.TodoItem{{ID: "verify", Text: "Verify the scene", Status: agent.TodoPending}}) {
		t.Fatalf("restored Todo: %+v", todo)
	}
}

func controlFixture(t *testing.T) (*session.Store, *session.Session, ProductState, agentrun.Options, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("control")
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{ProjectID: "stable-project", SessionID: sess.ID, AgentKind: agentrun.AgentKindIDE}
	state, err := SessionState(options, sess)
	if err != nil {
		t.Fatal(err)
	}
	return store, sess, state, options, dir
}

func TestExternalControlsPauseQueueReopenAndResume(t *testing.T) {
	store, _, state, options, dir := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	input := ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "initial", Message: "Continue the story", Locale: "zh-CN", InputVisibility: agentrun.InputModelOnly, AttachedFiles: []agent.Attachment{{ID: "attachment", Path: "attachments/source.txt"}}}}
	run, err := control.Start(t.Context(), input, func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
			close(started)
			<-ctx.Done()
			return agentrun.Outcome{Status: agentrun.OutcomeAborted, Error: ctx.Err()}
		}), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan agentrun.Outcome, 1)
	go func() {
		defer func() {
			if value := recover(); value != nil {
				done <- agentrun.Outcome{Status: agentrun.OutcomeFailed, Error: fmt.Errorf("test worker panic: %v", value)}
			}
		}()
		done <- run.Wait(t.Context())
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("cycle did not start")
	}
	command := Command{Kind: agentexecution.CommandFollowUp, CommandID: "queued", OperationID: run.Receipt().OperationID, Input: agentchat.ChatRequest{Message: "Then inspect the result", Locale: "en-US"}}
	first, err := control.Submit(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	again, err := control.Submit(t.Context(), command)
	if err != nil || first != again {
		t.Fatalf("idempotent queue receipt: %+v %+v %v", first, again, err)
	}
	if _, err := control.Submit(t.Context(), Command{Kind: agentexecution.CommandSuspend, CommandID: "pause", OperationID: run.Receipt().OperationID}); err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-done:
		if outcome.Status != agentrun.OutcomeSuspended {
			t.Fatalf("pause: %+v", outcome)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pause did not settle")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Copy only canonical data to another directory, without derived indexes.
	data, err := os.ReadFile(filepath.Join(dir, "control.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	moved := t.TempDir()
	if err := os.WriteFile(filepath.Join(moved, "control.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.NewStore(moved)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	sess, err := reopened.Get("control")
	if err != nil {
		t.Fatal(err)
	}
	state, err = SessionState(options, sess)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewEngines()
	defer restarted.Close()
	control, err = restarted.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	status, err := control.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != agentrun.RunPhaseSuspended || len(status.Queue) != 1 {
		t.Fatalf("reopened status: %+v", status)
	}
	var observed []ExternalCycleInput
	resumed, err := control.Resume(t.Context(), agentexecution.RuntimeRecoveryActions(status)[0], func(ctx context.Context, input ExternalCycleInput, _ func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		observed = append(observed, input)
		return externalCycleFunc(func(context.Context) agentrun.Outcome { return agentrun.Outcome{Status: agentrun.OutcomeCompleted} }), nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := resumed.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("resume: %+v", outcome)
	}
	if len(observed) != 2 || !observed[0].Resume || observed[0].Request.Locale != "zh-CN" || observed[0].Request.InputVisibility != agentrun.InputModelOnly || !reflect.DeepEqual(observed[0].Request.AttachedFiles, input.Request.AttachedFiles) || observed[1].Request.CommandID != "queued" {
		t.Fatalf("restored inputs: %+v", observed)
	}
	status, err = control.Status(t.Context())
	if err != nil || status.Phase != agentrun.RunPhaseIdle || len(status.Queue) != 0 {
		t.Fatalf("settled status: %+v %v", status, err)
	}
}

func TestExternalProductTodoAndGoalUseCanonicalState(t *testing.T) {
	_, sess, state, options, _ := controlFixture(t)
	goal, err := state.UpdateGoal(t.Context(), agent.GoalMutation{Kind: agent.GoalSet, Objective: "Verify the complete result"})
	if err != nil {
		t.Fatal(err)
	}
	if err := observePlanFixture(t, state, []agent.TodoItem{{ID: "first", Text: "Read", Status: agent.TodoInProgress}, {ID: "second", Text: "Verify", Status: agent.TodoPending}}); err != nil {
		t.Fatal(err)
	}
	if err := observePlanFixture(t, state, []agent.TodoItem{{ID: "second", Text: "Verify", Status: agent.TodoInProgress}, {ID: "first", Text: "Read", Status: agent.TodoCompleted}}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := SessionState(options, sess)
	if err != nil {
		t.Fatal(err)
	}
	got, present, err := reloaded.Goal(t.Context())
	if err != nil || !present || got.ID != goal.ID || got.Objective != goal.Objective {
		t.Fatalf("Goal: %+v %v", got, err)
	}
	raw, present, err := reloaded.Read(t.Context(), agent.TodoCapability)
	if err != nil || !present {
		t.Fatalf("Todo missing: %v", err)
	}
	var todo agent.TodoState
	if err := json.Unmarshal(raw, &todo); err != nil {
		t.Fatal(err)
	}
	if todo.Revision != 2 || !reflect.DeepEqual(todo.Items, []agent.TodoItem{{ID: "second", Text: "Verify", Status: agent.TodoInProgress}, {ID: "first", Text: "Read", Status: agent.TodoCompleted}}) {
		t.Fatalf("Todo: %+v", todo)
	}
}

func TestExternalFollowUpUsesToolBoundaryWhileNextTurnWaits(t *testing.T) {
	_, _, state, options, _ := controlFixture(t)
	engines := NewEngines()
	defer engines.Close()
	control, err := engines.ExternalControl(options, state)
	if err != nil {
		t.Fatal(err)
	}
	var run *ExternalRun
	var observed []ExternalCycleInput
	factory := func(ctx context.Context, input ExternalCycleInput, emit func(agentrun.Event), _ func(context.Context, *external.RuntimeSession) error) (ExternalCycle, error) {
		observed = append(observed, input)
		return externalCycleFunc(func(ctx context.Context) agentrun.Outcome {
			if len(observed) == 1 {
				for _, kind := range []agentexecution.CommandKind{agentexecution.CommandNextTurn, agentexecution.CommandFollowUp} {
					if _, err := control.Submit(ctx, Command{Kind: kind, CommandID: string(kind), OperationID: run.Receipt().OperationID, Input: agentchat.ChatRequest{Message: string(kind)}}); err != nil {
						t.Fatal(err)
					}
				}
				if ctx.Err() != nil {
					t.Fatal("follow-up interrupted a tool before its confirmation")
				}
				emit(agentrun.Event{Type: "tool_result", Data: map[string]any{"id": "settled-tool"}})
				steering := external.SteeringFromContext(ctx)
				guidance, pending, err := steering.Next(ctx)
				if err != nil || !pending || guidance.Request.CommandID != string(agentexecution.CommandFollowUp) || ctx.Err() != nil {
					t.Fatalf("follow-up was not offered to the native adapter: %+v, %v", guidance, err)
				}
				// The adapter chooses interrupt/resume when its native protocol
				// cannot acknowledge same-turn input.
				steering.Interrupt()
				if !errors.Is(context.Cause(ctx), external.ErrSteered) {
					t.Fatalf("follow-up did not join at the confirmed boundary: %v", context.Cause(ctx))
				}
				return agentrun.Outcome{Status: agentrun.OutcomeAborted, Error: ctx.Err()}
			}
			return agentrun.Outcome{Status: agentrun.OutcomeCompleted}
		}), nil
	}
	run, err = control.Start(t.Context(), ExternalCycleInput{Request: agentchat.ChatRequest{CommandID: "initial", Message: "Work"}}, factory, nil)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := run.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome: %+v", outcome)
	}
	if len(observed) != 3 || !observed[1].Resume || len(observed[1].Guidance) != 1 || observed[1].Guidance[0].CommandID != string(agentexecution.CommandFollowUp) || observed[2].Request.CommandID != string(agentexecution.CommandNextTurn) {
		t.Fatalf("wrong delivery order: %+v", observed)
	}
}

func observePlanFixture(t *testing.T, state ProductState, items []agent.TodoItem) error {
	t.Helper()
	return state.ObservePlan(t.Context(), external.PlanEvent(items))
}
