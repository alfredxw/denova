package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// FollowUp accepts a distinct Run in the existing child Session. HostData is
// captured from this parent invocation, never from the child's first task.
func (tasks *LocalTasks) FollowUp(ctx context.Context, ref TaskRef, input agent.Input) (Task, error) {
	candidate, err := tasks.agent(ref.Agent)
	if err != nil {
		return Task{}, err
	}
	localTaskAdmissionMu.Lock()
	defer localTaskAdmissionMu.Unlock()
	session, err := tasks.openExisting(ctx, candidate, ref.Session)
	if err != nil {
		return Task{}, err
	}
	snapshot, err := session.Snapshot(ctx)
	if err != nil {
		return Task{}, err
	}
	if snapshot.ActiveRunID == "" {
		if err := tasks.checkCapacity(ctx); err != nil {
			return Task{}, err
		}
	}
	input.HostData = cloneTaskHostData(candidate.HostData)
	receipt, err := session.FollowUp(ctx, input)
	if err != nil {
		return Task{}, err
	}
	ref.Run = receipt.RunID
	run, found, err := session.AttachRun(ctx, ref.Run)
	if err != nil {
		return Task{}, err
	}
	if !found {
		return Task{}, errors.New("accepted child Run was not found")
	}
	if err := tasks.trackTaskCompletion(ctx, ref); err != nil {
		return Task{}, err
	}
	tasks.watchCompletion(ctx, run, ref)
	snapshot, err = session.Snapshot(ctx)
	if err != nil {
		return Task{}, err
	}
	task, err := taskFromSnapshot(ref, snapshot)
	task.Receipt = &receipt
	return task, err
}

// SendMessage persists supplemental input only. An idle child remains idle;
// a suspended child remains suspended until explicitly resumed.
func (tasks *LocalTasks) SendMessage(ctx context.Context, ref TaskRef, input agent.Input) (agent.CommandReceipt, error) {
	candidate, err := tasks.agent(ref.Agent)
	if err != nil {
		return agent.CommandReceipt{}, err
	}
	session, err := tasks.openExisting(ctx, candidate, ref.Session)
	if err != nil {
		return agent.CommandReceipt{}, err
	}
	input.HostData = nil
	if active, found, err := session.Active(ctx); err != nil {
		return agent.CommandReceipt{}, err
	} else if found {
		original, _, err := session.RunInput(ctx, active.ID())
		if err != nil {
			return agent.CommandReceipt{}, err
		}
		input.HostData = original.HostData
	}
	queued, err := session.Queue(ctx, input)
	if err != nil {
		return agent.CommandReceipt{}, err
	}
	return queued.Receipt(), nil
}

func (tasks *LocalTasks) Resume(ctx context.Context, ref TaskRef, request agent.ResumeRequest) (Task, error) {
	localTaskAdmissionMu.Lock()
	defer localTaskAdmissionMu.Unlock()
	_, session, err := tasks.open(ctx, ref)
	if err != nil {
		return Task{}, err
	}
	snapshot, err := session.Snapshot(ctx)
	if err != nil {
		return Task{}, err
	}
	if snapshot.ActiveStatus == agent.ResultSuspended || snapshot.ActiveRunID == "" {
		if err := tasks.checkCapacity(ctx); err != nil {
			return Task{}, err
		}
	}
	if request.RunID != "" && request.RunID != ref.Run {
		return Task{}, errors.New("resume Run ID does not match the task ref")
	}
	request.RunID = ref.Run
	run, err := session.ResumeRun(ctx, request)
	if err != nil {
		return Task{}, err
	}
	if err := tasks.trackTaskCompletion(ctx, ref); err != nil {
		return Task{}, err
	}
	tasks.watchCompletion(ctx, run, ref)
	snapshot, err = session.Snapshot(ctx)
	if err != nil {
		return Task{}, err
	}
	return taskFromSnapshot(ref, snapshot)
}

func (tasks *LocalTasks) checkCapacity(ctx context.Context) error {
	active, err := tasks.activeTaskCount(ctx)
	if err != nil {
		return fmt.Errorf("count active tasks: %w", err)
	}
	if active >= tasks.parallelism {
		return fmt.Errorf("%w: %d active tasks reached the configured limit of %d", ErrTaskCapacityExceeded, active, tasks.parallelism)
	}
	return nil
}

func validateTaskSessionRef(ref TaskRef) error {
	if err := validateTaskString("ref.agent", ref.Agent, 256); err != nil {
		return err
	}
	if err := validateTaskString("ref.session", ref.Session, 1024); err != nil {
		return err
	}
	if strings.TrimSpace(ref.Run) != "" && len(ref.Run) > 1024 {
		return fmt.Errorf("%w: ref.run exceeds 1024 bytes", ErrTaskInvalidInput)
	}
	return nil
}
