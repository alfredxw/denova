package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
	filesession "github.com/alfredxw/denova/agent/session/file"
)

func TestAgentControlsKeepRunIdentityAndReceipts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release := make(chan struct{})
	owner := newTaskAgent(t, agentsession.Memory(), &blockingTaskModel{release: release, response: "done"})
	defer owner.Close(context.Background())
	executor := newTaskExecutor(t, owner)
	first, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "first", IdempotencyKey: "first"})
	if err != nil {
		t.Fatal(err)
	}
	retried, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "first", IdempotencyKey: "first"})
	if err != nil || retried.Receipt == nil || *retried.Receipt != *first.Receipt || retried.Ref != first.Ref {
		t.Fatalf("delegate retry=%#v err=%v", retried, err)
	}
	next, err := executor.FollowUp(ctx, first.Ref, agent.Input{Text: "next", IdempotencyKey: "next"})
	if err != nil || next.Ref.Run == first.Ref.Run {
		t.Fatalf("followup=%#v err=%v", next, err)
	}
	pause, err := executor.Interrupt(ctx, first.Ref, agent.SuspendRequest{Reason: "review", IdempotencyKey: "pause"})
	if err != nil || pause.CommandID != "pause" {
		t.Fatalf("pause=%#v err=%v", pause, err)
	}
	if _, err := executor.Steer(ctx, first.Ref, agent.Input{Text: "focus", IdempotencyKey: "steer"}); err != nil {
		t.Fatal(err)
	}
	zero := 0
	result, err := awaitAgents(ctx, executor, taskWaitInput{Targets: []taskWaitTarget{{Ref: first.Ref}}, TimeoutMS: &zero})
	if err != nil || !strings.Contains(result.ModelContent, `"reason":"attention"`) {
		t.Fatalf("paused observation=%s err=%v", result.ModelContent, err)
	}
	resumed, err := executor.Resume(ctx, first.Ref, agent.ResumeRequest{IdempotencyKey: "resume"})
	if err != nil || resumed.Receipt == nil || resumed.Receipt.CommandID != "resume" || resumed.Ref != first.Ref {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
	if _, err := executor.Interrupt(ctx, first.Ref, agent.SuspendRequest{Reason: "review again", IdempotencyKey: "pause-again"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Resume(ctx, first.Ref, agent.ResumeRequest{IdempotencyKey: "resume"}); err != nil {
		t.Fatal(err)
	}
	paused, err := executor.Observe(ctx, first.Ref, "")
	if err != nil || paused.Task.Status != "suspended" {
		t.Fatalf("old resume changed newer pause: %#v %v", paused.Task, err)
	}
	if _, err := executor.Resume(ctx, next.Ref, agent.ResumeRequest{IdempotencyKey: "bad-resume"}); !errors.Is(err, agent.ErrNoActiveRun) {
		t.Fatalf("queued resume error=%v", err)
	}
	close(release)
	abort, err := executor.Abort(ctx, first.Ref, agent.AbortRequest{Reason: "obsolete", IdempotencyKey: "abort"})
	if err != nil || abort.CommandID != "abort" {
		t.Fatalf("abort=%#v err=%v", abort, err)
	}
	outcomes, err := executor.Wait(ctx, []TaskRef{next.Ref})
	if err != nil || !outcomes[0].Ready || outcomes[0].Task.Status != "completed" {
		t.Fatalf("abort lost queued followup: %#v %v", outcomes, err)
	}
	retry, err := executor.Resume(ctx, first.Ref, agent.ResumeRequest{IdempotencyKey: "resume"})
	if err != nil || retry.Receipt == nil || retry.Receipt.CommandID != "resume" {
		t.Fatalf("settled resume retry=%#v %v", retry, err)
	}
	if _, err := executor.Interrupt(ctx, first.Ref, agent.SuspendRequest{Reason: "stale", IdempotencyKey: "stale"}); !errors.Is(err, agent.ErrNoActiveRun) {
		t.Fatalf("stale pause=%v", err)
	}
}

func TestListAgentsReplaysWithoutWriterLeaseAndPagesWithinScope(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	store, err := filesession.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := newTaskAgent(t, store, &taskModel{responses: []*agent.Message{agent.AssistantMessage("one", nil), agent.AssistantMessage("two", nil)}})
	executor := newTaskExecutor(t, owner)
	for _, id := range []string{"one", "two"} {
		task, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: id, IdempotencyKey: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = executor.Wait(ctx, []TaskRef{task.Ref}); err != nil {
			t.Fatal(err)
		}
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	cold := newTaskAgent(t, store, &taskModel{})
	defer cold.Close(context.Background())
	reader := newTaskExecutor(t, cold)
	reader.self = TaskRef{Agent: "parent", Session: "parent-session"}
	page, err := reader.ListAgents(ctx, ListAgentsInput{Limit: 1})
	if err != nil || len(page.Agents) != 1 || page.NextCursor == "" || page.Agents[0].LastSettledRun == nil {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	key := localTaskSessionKey(reader.agents["researcher"], page.Agents[0].Ref.Session)
	leaseCtx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	lease, err := store.Open(leaseCtx, key)
	stop()
	if err != nil {
		t.Fatalf("listing took a writer lease: %v", err)
	}
	_ = lease.Close()
	next, err := reader.ListAgents(ctx, ListAgentsInput{Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Agents) != 1 || next.NextCursor != "" || next.Agents[0].Ref == page.Agents[0].Ref {
		t.Fatalf("next=%#v %v", next, err)
	}
	if _, err := reader.ListAgents(ctx, ListAgentsInput{Kind: "definitions", Cursor: page.NextCursor}); !errors.Is(err, ErrTaskInvalidInput) {
		t.Fatalf("cross-kind cursor=%v", err)
	}
	reader.self.Session = "another-parent"
	if _, err := reader.ListAgents(ctx, ListAgentsInput{Cursor: page.NextCursor}); !errors.Is(err, ErrTaskInvalidInput) {
		t.Fatalf("cross-scope cursor=%v", err)
	}
}

type stagedWaitExecutor struct {
	schemaTaskExecutor
	calls         [][]TaskRef
	attention     bool
	failRemaining bool
}

func (executor *stagedWaitExecutor) Wait(_ context.Context, refs []TaskRef) ([]TaskWaitOutcome, error) {
	executor.calls = append(executor.calls, append([]TaskRef(nil), refs...))
	results := make([]TaskWaitOutcome, len(refs))
	for index, ref := range refs {
		if executor.failRemaining && len(executor.calls) > 1 {
			results[index].Err = ErrTaskNotFound
			continue
		}
		status, ready := "running", false
		if index == 0 {
			status, ready = "completed", true
		}
		if executor.attention && index == 0 {
			status = "suspended"
		}
		results[index] = TaskWaitOutcome{Task: &Task{Ref: ref, Status: status}, Ready: ready}
	}
	return results, nil
}

func TestAwaitDropsStaleSnapshotWhenRemainingTargetFails(t *testing.T) {
	executor := &stagedWaitExecutor{failRemaining: true}
	result, err := awaitAgents(t.Context(), executor, taskWaitInput{Until: "all", Targets: []taskWaitTarget{
		{Ref: TaskRef{Agent: "a", Session: "s", Run: "one"}}, {Ref: TaskRef{Agent: "a", Session: "s", Run: "two"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var report awaitReport
	if err := json.Unmarshal([]byte(result.ModelContent), &report); err != nil {
		t.Fatal(err)
	}
	failed := report.Results[1]
	if report.Reason != "ready" || failed.Outcome != "error" || failed.Run != nil || failed.Ready != nil || failed.Error.Code != "not_found" {
		t.Fatal(result.ModelContent)
	}
}

func TestAwaitTimeoutAndOutputBudget(t *testing.T) {
	ctx := t.Context()
	release := make(chan struct{})
	owner := newTaskAgent(t, agentsession.Memory(), &blockingTaskModel{release: release, response: strings.Repeat("\x01界", 2000)})
	defer owner.Close(context.Background())
	executor := newTaskExecutor(t, owner)
	executor.maxResultBytes = 2048
	started, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "inspect", IdempotencyKey: "budget"})
	if err != nil {
		t.Fatal(err)
	}
	timeout := 20
	input := taskWaitInput{Targets: []taskWaitTarget{{Ref: started.Ref}}, TimeoutMS: &timeout}
	result, err := awaitAgents(ctx, executor, input)
	if err != nil {
		t.Fatal(err)
	}
	var report awaitReport
	if err := json.Unmarshal([]byte(result.ModelContent), &report); err != nil {
		t.Fatal(err)
	}
	if report.Reason != "timeout" || report.Results[0].Run.Status != "running" || report.Results[0].Output != nil {
		t.Fatal(result.ModelContent)
	}
	close(release)
	if _, err := executor.Wait(ctx, []TaskRef{started.Ref}); err != nil {
		t.Fatal(err)
	}
	timeout = 0
	result, err = awaitAgents(ctx, executor, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(result.ModelContent), &report); err != nil {
		t.Fatal(err)
	}
	if len(result.ModelContent) > executor.maxResultBytes || report.Reason != "ready" || report.Results[0].Output == nil || !report.Results[0].Output.Incomplete || report.Results[0].Output.Text == "" {
		t.Fatal(result.ModelContent)
	}
}

func TestResumedChildStillDeliversCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	parentOwner := newTaskAgent(t, agentsession.Memory(), &taskModel{})
	defer parentOwner.Close(context.Background())
	parent, err := parentOwner.Session(ctx, agent.NamedSession("resume-parent"))
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	child := newTaskAgent(t, agentsession.Memory(), &blockingTaskModel{release: release, response: "resumed result"})
	defer child.Close(context.Background())
	executor := newTaskExecutor(t, child)
	executor.completionParent = parent
	started, err := executor.Start(ctx, TaskRequest{Agent: "researcher", Prompt: "inspect", IdempotencyKey: "start"})
	if err != nil {
		t.Fatal(err)
	}
	for index := range 12 {
		key := fmt.Sprintf("pause-%d", index)
		if _, err := executor.Interrupt(ctx, started.Ref, agent.SuspendRequest{Reason: "review", IdempotencyKey: key}); err != nil {
			t.Fatal(err)
		}
		if _, err := executor.Resume(ctx, started.Ref, agent.ResumeRequest{IdempotencyKey: "resume-" + key}); err != nil {
			t.Fatal(err)
		}
	}
	close(release)
	for {
		watch, err := parent.WatchTaskCompletions(ctx, []string{taskCompletionID(started.Ref)})
		if err != nil {
			t.Fatal(err)
		}
		if len(watch.PendingIDs) == 1 {
			break
		}
		select {
		case <-watch.Activity:
		case <-ctx.Done():
			t.Fatal("resumed child did not deliver its completion")
		}
	}
}
func TestAwaitAllRemovesReadyTargetsAndAttentionShortCircuits(t *testing.T) {
	for _, attention := range []bool{false, true} {
		executor := &stagedWaitExecutor{attention: attention}
		result, err := awaitAgents(context.Background(), executor, taskWaitInput{Until: "all", Targets: []taskWaitTarget{
			{Ref: TaskRef{Agent: "a", Session: "s", Run: "one"}}, {Ref: TaskRef{Agent: "a", Session: "s", Run: "two"}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		var report awaitReport
		if err := json.Unmarshal([]byte(result.ModelContent), &report); err != nil {
			t.Fatal(err)
		}
		if attention {
			if len(executor.calls) != 1 || report.Reason != "attention" {
				t.Fatal(result.ModelContent)
			}
		} else {
			if len(executor.calls) != 2 || len(executor.calls[1]) != 1 || executor.calls[1][0].Run != "two" || report.Reason != "ready" {
				t.Fatalf("calls=%#v report=%s", executor.calls, result.ModelContent)
			}
		}
	}
}
