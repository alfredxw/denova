package tools

import (
	"context"
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func TestTaskReadRetryStopsOnCancellationAndStorageErrors(t *testing.T) {
	storageErr := errors.New("journal is corrupt")
	for _, readErr := range []error{agent.ErrSessionClosed, storageErr} {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		_, err := retryClosedTaskRead(ctx, func() (Task, error) {
			calls++
			cancel()
			return Task{}, readErr
		})
		want := readErr
		if errors.Is(readErr, agent.ErrSessionClosed) {
			want = context.Canceled
		}
		if !errors.Is(err, want) || calls != 1 {
			t.Fatalf("cancelled read: err=%v calls=%d", err, calls)
		}
	}
}

type closingTaskSessionOpener struct {
	*agent.Agent
	closeNext bool
}

func (opener *closingTaskSessionOpener) Session(ctx context.Context, key agent.SessionKey) (*agent.Session, error) {
	session, err := opener.Agent.Session(ctx, key)
	if err == nil && opener.closeNext {
		opener.closeNext = false
		err = session.Close(ctx)
	}
	return session, err
}

func TestTaskReadsReopenSessionClosedByAnotherObserver(t *testing.T) {
	for _, operation := range []string{"observe", "snapshot"} {
		t.Run(operation, func(t *testing.T) {
			ctx := t.Context()
			owner := newTaskAgent(t, agentsession.Memory(), &taskModel{
				responses: []*agent.Message{agent.AssistantMessage("completed output", nil)},
			})
			defer owner.Close(context.Background())
			opener := &closingTaskSessionOpener{Agent: owner}
			executor, err := NewLocalTasks(LocalTaskOptions{Parallelism: 1}, LocalTaskAgent{
				Name: "reader", Description: "Read task results", Opener: opener,
				Identity: agent.CapabilityIdentity{Kind: "test.task.closed-read", Version: 1},
			})
			if err != nil {
				t.Fatal(err)
			}
			started, err := executor.Start(ctx, TaskRequest{Agent: "reader", Prompt: "inspect", IdempotencyKey: "read"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := executor.Wait(ctx, []TaskRef{started.Ref}); err != nil {
				t.Fatal(err)
			}
			opener.closeNext = true
			var result Task
			if operation == "observe" {
				observation, observeErr := executor.Observe(ctx, started.Ref, "0")
				result, err = observation.Task, observeErr
			} else {
				result, err = executor.taskSnapshot(ctx, started.Ref)
			}
			if err != nil || result.Status != string(agent.ResultCompleted) || result.Output != "completed output" {
				t.Fatalf("read after concurrent close: result=%#v err=%v", result, err)
			}
		})
	}
}
