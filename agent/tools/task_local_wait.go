package tools

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type localTaskWaitStream struct {
	index  int
	ref    TaskRef
	events <-chan agent.Event
	errors <-chan error
}

type localTaskPendingInteraction struct {
	index   int
	ref     TaskRef
	request agent.InteractionRequest
}

type localTaskWaitSource struct {
	stream  int
	isErr   bool
	mailbox bool
}

func (tasks *LocalTasks) Wait(ctx context.Context, refs []TaskRef) ([]TaskWaitOutcome, error) {
	if len(refs) == 0 {
		return nil, errors.New("task wait requires at least one ref")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	outcomes := make([]TaskWaitOutcome, len(refs))
	ready := make(map[int]bool)
	completionIndexes := make(map[string][]int, len(refs))
	completionIDs := make([]string, len(refs))
	for index, ref := range refs {
		completionIDs[index] = taskCompletionID(ref)
		completionIndexes[completionIDs[index]] = append(completionIndexes[completionIDs[index]], index)
	}
	streams := make([]localTaskWaitStream, 0, len(refs))
	var pending []localTaskPendingInteraction

	for index, ref := range refs {
		_, session, err := tasks.open(waitCtx, ref)
		if err != nil {
			outcomes[index].Err = err
			continue
		}
		snapshot, err := session.Snapshot(waitCtx)
		if err != nil {
			outcomes[index].Err = err
			continue
		}
		task, taskErr := taskFromSnapshot(ref, snapshot)
		if taskErr != nil {
			outcomes[index].Err = taskErr
			continue
		}
		outcomes[index].Task = &task
		if isTaskTerminal(task.Status) {
			ready[index] = true
			continue
		}
		observation, observeErr := session.Observe(waitCtx, snapshot.Cursor)
		if observeErr != nil {
			outcomes[index].Err = observeErr
			outcomes[index].Task = nil
			continue
		}
		task, taskErr = taskFromSnapshot(ref, observation.Snapshot)
		if taskErr != nil {
			outcomes[index].Err = taskErr
			outcomes[index].Task = nil
			continue
		}
		outcomes[index].Task = &task
		if isTaskTerminal(task.Status) {
			ready[index] = true
			continue
		}
		for _, request := range observation.Snapshot.PendingInteractions {
			pending = append(pending, localTaskPendingInteraction{index: index, ref: ref, request: request})
		}
		streams = append(streams, localTaskWaitStream{
			index: index, ref: ref, events: observation.Events, errors: observation.Errors,
		})
	}

	var completionWatch agent.TaskCompletionWatch
	if tasks.completionParent != nil {
		watch, watchErr := tasks.completionParent.WatchTaskCompletions(waitCtx, completionIDs)
		if watchErr != nil {
			return nil, fmt.Errorf("watch parent task completions: %w", watchErr)
		}
		completionWatch = watch
		markTaskCompletionReady(ready, completionIndexes, completionWatch.PendingIDs)
	}
	if len(ready) != 0 {
		cancel()
		return tasks.collectWaitOutcomes(ctx, refs, outcomes, ready), nil
	}
	if len(pending) != 0 {
		interaction := pending[0]
		resolution, err := agent.RequestInteraction(ctx, interaction.request)
		if err != nil {
			return nil, fmt.Errorf("route child Interaction: %w", err)
		}
		if err := tasks.Respond(ctx, interaction.ref, interaction.request.ID, responseFromResolution(resolution)); err != nil {
			return nil, fmt.Errorf("resolve child Interaction: %w", err)
		}
		ready[interaction.index] = true
		cancel()
		return tasks.collectWaitOutcomes(ctx, refs, outcomes, ready), nil
	}
	if len(streams) == 0 {
		return outcomes, nil
	}

	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())}}
	sources := []localTaskWaitSource{{stream: -1}}
	if completionWatch.Activity != nil {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(completionWatch.Activity)})
		sources = append(sources, localTaskWaitSource{stream: -1, mailbox: true})
	}
	for index, stream := range streams {
		cases = append(cases,
			reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(stream.events)},
			reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(stream.errors)},
		)
		sources = append(sources,
			localTaskWaitSource{stream: index},
			localTaskWaitSource{stream: index, isErr: true},
		)
	}
	openStreams := len(streams)
	for openStreams > 0 {
		chosen, received, ok := reflect.Select(cases)
		if chosen == 0 {
			return nil, ctx.Err()
		}
		source := sources[chosen]
		if source.mailbox {
			watch, watchErr := tasks.completionParent.WatchTaskCompletions(waitCtx, completionIDs)
			if watchErr != nil {
				return nil, fmt.Errorf("recheck parent task completions: %w", watchErr)
			}
			completionWatch = watch
			markTaskCompletionReady(ready, completionIndexes, completionWatch.PendingIDs)
			if len(ready) != 0 {
				cancel()
				return tasks.collectWaitOutcomes(ctx, refs, outcomes, ready), nil
			}
			cases[chosen].Chan = reflect.ValueOf(completionWatch.Activity)
			continue
		}
		stream := &streams[source.stream]
		if !ok {
			cases[chosen].Chan = reflect.Value{}
			if source.isErr {
				stream.errors = nil
			} else {
				stream.events = nil
			}
			if stream.events == nil && stream.errors == nil {
				if outcomes[stream.index].Err == nil {
					outcomes[stream.index].Err = errors.New("task event stream closed before the task became ready")
					outcomes[stream.index].Task = nil
				}
				openStreams--
			}
			continue
		}
		if source.isErr {
			streamErr, _ := received.Interface().(error)
			if streamErr != nil {
				outcomes[stream.index].Err = streamErr
				outcomes[stream.index].Task = nil
				stream.events, stream.errors = nil, nil
				cases[chosen].Chan = reflect.Value{}
				for caseIndex, candidate := range sources {
					if candidate.stream == source.stream {
						cases[caseIndex].Chan = reflect.Value{}
					}
				}
				openStreams--
			}
			continue
		}
		event, eventOK := received.Interface().(agent.Event)
		if !eventOK || event.RunID != stream.ref.Run {
			continue
		}
		if err := forwardTaskEvent(ctx, stream.ref, event); err != nil {
			return nil, err
		}
		switch payload := event.Payload.(type) {
		case agent.InteractionRequested:
			resolution, err := agent.RequestInteraction(ctx, payload.Request)
			if err != nil {
				return nil, fmt.Errorf("route child Interaction: %w", err)
			}
			if err := tasks.Respond(ctx, stream.ref, payload.Request.ID, responseFromResolution(resolution)); err != nil {
				return nil, fmt.Errorf("resolve child Interaction: %w", err)
			}
			ready[stream.index] = true
			cancel()
			return tasks.collectWaitOutcomes(ctx, refs, outcomes, ready), nil
		case agent.RunSettled:
			ready[stream.index] = true
			cancel()
			return tasks.collectWaitOutcomes(ctx, refs, outcomes, ready), nil
		}
	}
	return tasks.collectWaitOutcomes(ctx, refs, outcomes, ready), nil
}

func (tasks *LocalTasks) collectWaitOutcomes(
	ctx context.Context,
	refs []TaskRef,
	outcomes []TaskWaitOutcome,
	ready map[int]bool,
) []TaskWaitOutcome {
	for index, ref := range refs {
		if outcomes[index].Err != nil {
			continue
		}
		task, err := tasks.taskSnapshot(ctx, ref)
		if err != nil {
			outcomes[index] = TaskWaitOutcome{Err: err}
			continue
		}
		if isTaskTerminal(task.Status) {
			if err := tasks.enqueueTaskCompletion(ctx, task); err != nil {
				outcomes[index] = TaskWaitOutcome{Err: fmt.Errorf("queue task completion: %w", err)}
				continue
			}
		}
		task.Output = ""
		task.Reason = ""
		outcomes[index].Task = &task
		outcomes[index].Ready = ready[index] || isTaskTerminal(task.Status)
	}
	return outcomes
}

func markTaskCompletionReady(ready map[int]bool, indexes map[string][]int, ids []string) {
	for _, id := range ids {
		for _, index := range indexes[id] {
			ready[index] = true
		}
	}
}

func (tasks *LocalTasks) taskSnapshot(ctx context.Context, ref TaskRef) (Task, error) {
	_, session, err := tasks.open(ctx, ref)
	if err != nil {
		return Task{}, err
	}
	snapshot, err := session.Snapshot(ctx)
	if err != nil {
		return Task{}, err
	}
	task, err := tasks.taskFromSessionSnapshot(ctx, session, ref, snapshot)
	if err == nil && isTaskTerminal(task.Status) {
		err = errors.Join(err, session.Close(context.Background()))
	}
	return task, err
}

func forwardTaskEvent(ctx context.Context, ref TaskRef, event agent.Event) error {
	source := taskEventSource(event.Payload)
	childPath := append([]string(nil), source.Path...)
	if len(childPath) == 0 {
		childPath = []string{firstTaskEventValue(source.Name, ref.Agent)}
	}
	path := make([]string, 0, len(childPath)+2)
	if scope, ok := agent.InvocationScopeFromContext(ctx); ok {
		path = append(path, scope.RunPath...)
	}
	if len(path) != 0 && len(childPath) != 0 && path[len(path)-1] == childPath[0] {
		childPath = childPath[1:]
	}
	path = append(path, childPath...)
	name := firstTaskEventValue(source.Name, ref.Agent)
	if len(path) != 0 {
		name = path[len(path)-1]
	}
	return agent.ForwardNestedEvent(ctx, agent.NestedEvent{
		Source: agent.EventSource{
			Name: name, Path: path, InvocationID: ref.Session + "/" + ref.Run, InvocationType: "task",
		},
		SessionID: ref.Session, Child: event,
	})
}

func taskEventSource(payload agent.EventPayload) agent.EventSource {
	switch value := payload.(type) {
	case agent.AssistantDelta:
		return value.Source
	case agent.ThinkingDelta:
		return value.Source
	case agent.ToolStarted:
		return value.Source
	case agent.ToolProgress:
		return value.Source
	case agent.ToolFinished:
		return value.Source
	case agent.ToolInputStarted:
		return value.Source
	case agent.ToolInputDelta:
		return value.Source
	case agent.ModelCompleted:
		return value.Source
	case agent.ArtifactProduced:
		return value.Source
	case agent.NestedEvent:
		return value.Source
	default:
		return agent.EventSource{}
	}
}

func firstTaskEventValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "agent"
}

func responseFromResolution(resolution agent.InteractionResolution) agent.InteractionResponse {
	return agent.InteractionResponse{
		Answers:    append([]agent.InteractionAnswer(nil), resolution.Answers...),
		Permission: resolution.Permission, Cancelled: resolution.Cancelled,
	}
}
