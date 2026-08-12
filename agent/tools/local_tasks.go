package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// SessionOpener is the narrow non-owning lifecycle seam needed by local
// delegation. agent.Agent implements it; an Executor never closes the owner.
type SessionOpener interface {
	Session(context.Context, agent.SessionKey) (*agent.Session, error)
	ListSessions(context.Context, agent.SessionSelector) ([]agent.SessionKey, error)
}

// LocalTaskAgent binds one stable selector to an Agent owner. Different
// selectors may share one owner or use independent Sources/Stores.
type LocalTaskAgent struct {
	Name        string
	Description string
	Opener      SessionOpener
	Identity    agent.CapabilityIdentity
	// Attributes are immutable parent-routing identity copied into every child
	// Session. They let the Agent Source rebuild the selected Definition after
	// a process restart without an executor-local task registry.
	Attributes map[string]string
	// LookupAttributes are the parent-stable subset used to find the exact
	// durable child key again. Per-turn route attributes deliberately stay out
	// of this selector, so an earlier TaskRef remains usable from later parent
	// cycles and after a cold restart.
	LookupAttributes map[string]string
}

// LocalTasks executes named children through Agent -> Session -> Run. The
// executor keeps no authoritative task map: TaskRef is sufficient to reopen a
// Session and attach a Run after a cold restart.
type LocalTasks struct {
	agents   map[string]LocalTaskAgent
	ordered  []TaskAgentInfo
	identity agent.CapabilityIdentity
}

func NewLocalTasks(candidates ...LocalTaskAgent) (*LocalTasks, error) {
	if len(candidates) == 0 {
		return nil, errors.New("local tasks require at least one Agent")
	}
	resolved := make(map[string]LocalTaskAgent, len(candidates))
	identities := make([]struct {
		Name, Description string
		Identity          agent.CapabilityIdentity
	}, 0, len(candidates))
	for index, candidate := range candidates {
		candidate.Name = strings.TrimSpace(candidate.Name)
		candidate.Description = strings.TrimSpace(candidate.Description)
		if candidate.Name == "" || candidate.Opener == nil || candidate.Identity.Kind == "" || candidate.Identity.Version == 0 {
			return nil, fmt.Errorf("local task Agent %d requires name, opener, and stable identity", index)
		}
		if _, duplicate := resolved[candidate.Name]; duplicate {
			return nil, fmt.Errorf("local task Agent %q is duplicated", candidate.Name)
		}
		candidate.Attributes = cloneTaskAttributes(candidate.Attributes)
		candidate.LookupAttributes = cloneTaskAttributes(candidate.LookupAttributes)
		for name, value := range candidate.LookupAttributes {
			if candidate.Attributes[name] != value {
				return nil, fmt.Errorf("local task Agent %q lookup attribute %q is not immutable", candidate.Name, name)
			}
		}
		resolved[candidate.Name] = candidate
		identities = append(identities, struct {
			Name, Description string
			Identity          agent.CapabilityIdentity
		}{candidate.Name, candidate.Description, candidate.Identity})
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i].Name < identities[j].Name })
	ordered := make([]TaskAgentInfo, len(identities))
	for index, candidate := range identities {
		ordered[index] = TaskAgentInfo{Name: candidate.Name, Description: candidate.Description}
	}
	return &LocalTasks{
		agents: resolved, ordered: ordered,
		identity: toolsetIdentity("tasks.local", identities),
	}, nil
}

func (tasks *LocalTasks) Identity() agent.CapabilityIdentity {
	if tasks == nil {
		return agent.CapabilityIdentity{}
	}
	return tasks.identity
}

func (tasks *LocalTasks) TaskAgents() []TaskAgentInfo {
	if tasks == nil {
		return nil
	}
	return append([]TaskAgentInfo(nil), tasks.ordered...)
}

func (tasks *LocalTasks) Start(ctx context.Context, request TaskRequest) (Task, error) {
	candidate, err := tasks.agent(request.Agent)
	if err != nil {
		return Task{}, err
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return Task{}, errors.New("task prompt is required")
	}
	commandID := strings.TrimSpace(request.IdempotencyKey)
	if commandID == "" {
		commandID, err = localTaskID("command")
		if err != nil {
			return Task{}, err
		}
	}
	sessionID := localTaskSessionID(candidate.Name, commandID)
	session, err := tasks.openOrCreate(ctx, candidate, sessionID)
	if err != nil {
		return Task{}, fmt.Errorf("open task Session: %w", err)
	}
	run, err := session.Run(ctx, agent.Input{Text: prompt, IdempotencyKey: commandID})
	if err != nil {
		if deleteErr := session.Delete(context.Background()); deleteErr != nil {
			return Task{}, errors.Join(
				fmt.Errorf("start task Run: %w", err),
				fmt.Errorf("delete rejected task Session: %w", deleteErr),
			)
		}
		return Task{}, fmt.Errorf("start task Run: %w", err)
	}
	ref := TaskRef{Agent: candidate.Name, Session: sessionID, Run: run.ID()}
	if request.Detached {
		// Session.Close is a structural binding close, not a handle release: it
		// durably aborts an active Run. Leave detached bindings under their Agent
		// owner's normal idle-capacity policy; TaskRef can reopen them at any time.
		return Task{Ref: ref, Status: "running"}, nil
	}
	return tasks.wait(ctx, session, run, ref)
}

func (tasks *LocalTasks) Observe(ctx context.Context, ref TaskRef, cursor string) (TaskObservation, error) {
	candidate, session, err := tasks.open(ctx, ref)
	if err != nil {
		return TaskObservation{}, err
	}
	after, err := parseTaskCursor(cursor)
	if err != nil {
		return TaskObservation{}, err
	}
	observation, err := session.Observe(ctx, after)
	if err != nil {
		return TaskObservation{}, err
	}
	result := TaskObservation{Task: Task{Ref: ref, Status: taskStatus(observation.Snapshot, ref.Run)}}
	result.Events, result.Cursor, result.Output, result.Incomplete, err = collectTaskEvents(ctx, observation, ref.Run, after)
	result.Interactions = cloneTaskInteractions(observation.Snapshot.PendingInteractions)
	result.RecoveryActions = append([]agent.RecoveryAction(nil), observation.Snapshot.RecoveryActions...)
	if result.Output == "" {
		result.Output = taskSnapshotOutput(observation.Snapshot, ref.Run)
	}
	if err == nil && result.Output == "" && result.Task.Status != "running" {
		var replayIncomplete bool
		result.Output, replayIncomplete, err = replayTaskFinal(ctx, session, ref.Run, observation.Snapshot.RetentionStart)
		result.Incomplete = result.Incomplete || replayIncomplete
	}
	if result.Output != "" {
		result.Task.Output = result.Output
	}
	_ = candidate
	return result, err
}

func (tasks *LocalTasks) Steer(ctx context.Context, ref TaskRef, input agent.Input) error {
	_, session, err := tasks.open(ctx, ref)
	if err != nil {
		return err
	}
	run, found, err := session.AttachRun(ctx, ref.Run)
	if err != nil || !found {
		if err == nil {
			err = errors.New("task Run was not found")
		}
		return err
	}
	_, err = run.Steer(ctx, input)
	return err
}

func (tasks *LocalTasks) Respond(
	ctx context.Context,
	ref TaskRef,
	interactionID string,
	response agent.InteractionResponse,
) error {
	_, session, err := tasks.open(ctx, ref)
	if err != nil {
		return err
	}
	run, found, err := session.AttachRun(ctx, ref.Run)
	if err != nil || !found {
		if err == nil {
			err = errors.New("task Run was not found")
		}
		return err
	}
	return run.Respond(ctx, strings.TrimSpace(interactionID), response)
}

func (tasks *LocalTasks) Abort(ctx context.Context, ref TaskRef, request agent.AbortRequest) error {
	_, session, err := tasks.open(ctx, ref)
	if err != nil {
		return err
	}
	run, found, err := session.AttachRun(ctx, ref.Run)
	if err != nil || !found {
		if err == nil {
			err = errors.New("task Run was not found")
		}
		return err
	}
	_, err = run.Abort(ctx, request)
	return err
}

func (tasks *LocalTasks) wait(ctx context.Context, session *agent.Session, run *agent.Run, ref TaskRef) (Task, error) {
	var output strings.Builder
	events := run.Events()
	for events != nil {
		var event agent.Event
		var ok bool
		select {
		case event, ok = <-events:
			if !ok {
				events = nil
				continue
			}
		case <-ctx.Done():
			_, abortErr := run.Abort(context.WithoutCancel(ctx), agent.AbortRequest{
				Reason:         "parent task context cancelled",
				IdempotencyKey: ref.Session + ":" + ref.Run + ":parent-cancel",
			})
			if abortErr != nil {
				return Task{Ref: ref, Status: "failed", Output: output.String()}, errors.Join(ctx.Err(), abortErr)
			}
			return Task{Ref: ref, Status: "aborting", Output: output.String()}, ctx.Err()
		}
		if err := forwardTaskEvent(ctx, ref, event); err != nil {
			return Task{Ref: ref, Status: "failed", Output: output.String()}, err
		}
		switch payload := event.Payload.(type) {
		case agent.AssistantDelta:
			if payload.Delta != "" {
				output.WriteString(payload.Delta)
			}
		case agent.InteractionRequested:
			resolution, interactionErr := agent.RequestInteraction(ctx, payload.Request)
			if interactionErr != nil {
				return Task{Ref: ref, Status: "blocked"}, fmt.Errorf("route child Interaction: %w", interactionErr)
			}
			if err := run.Respond(ctx, payload.Request.ID, responseFromResolution(resolution)); err != nil {
				return Task{Ref: ref, Status: "failed"}, err
			}
		case agent.AssistantFinal:
			output.Reset()
			output.WriteString(payload.Content)
		}
	}
	result, err := run.Wait(ctx)
	if closeErr := session.Close(context.Background()); closeErr != nil {
		err = errors.Join(err, closeErr)
	}
	status := string(result.Status)
	if err != nil {
		return Task{Ref: ref, Status: status, Output: output.String()}, err
	}
	return Task{Ref: ref, Status: status, Output: output.String()}, nil
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

func (tasks *LocalTasks) open(ctx context.Context, ref TaskRef) (LocalTaskAgent, *agent.Session, error) {
	candidate, err := tasks.agent(ref.Agent)
	if err != nil {
		return LocalTaskAgent{}, nil, err
	}
	if strings.TrimSpace(ref.Session) == "" || strings.TrimSpace(ref.Run) == "" {
		return LocalTaskAgent{}, nil, errors.New("task ref requires agent, session, and run")
	}
	session, err := tasks.openExisting(ctx, candidate, ref.Session)
	return candidate, session, err
}

func (tasks *LocalTasks) openOrCreate(
	ctx context.Context,
	candidate LocalTaskAgent,
	sessionID string,
) (*agent.Session, error) {
	keys, err := taskSessionKeys(ctx, candidate, sessionID)
	if err != nil {
		return nil, err
	}
	switch len(keys) {
	case 0:
		return candidate.Opener.Session(ctx, localTaskSessionKey(candidate, sessionID))
	case 1:
		return candidate.Opener.Session(ctx, keys[0])
	default:
		return nil, errors.New("task Session identity is ambiguous")
	}
}

func (tasks *LocalTasks) openExisting(
	ctx context.Context,
	candidate LocalTaskAgent,
	sessionID string,
) (*agent.Session, error) {
	keys, err := taskSessionKeys(ctx, candidate, sessionID)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, errors.New("task Session was not found")
	}
	if len(keys) != 1 {
		return nil, errors.New("task Session identity is ambiguous")
	}
	return candidate.Opener.Session(ctx, keys[0])
}

func taskSessionKeys(
	ctx context.Context,
	candidate LocalTaskAgent,
	sessionID string,
) ([]agent.SessionKey, error) {
	attributes := cloneTaskAttributes(candidate.LookupAttributes)
	if attributes == nil {
		attributes = make(map[string]string, 1)
	}
	attributes["agent"] = candidate.Name
	return candidate.Opener.ListSessions(ctx, agent.SessionSelector{
		Namespace:  "task." + candidate.Name,
		ID:         sessionID,
		Attributes: attributes,
	})
}

func (tasks *LocalTasks) agent(name string) (LocalTaskAgent, error) {
	if tasks == nil {
		return LocalTaskAgent{}, errors.New("local tasks are unavailable")
	}
	name = strings.TrimSpace(name)
	candidate, ok := tasks.agents[name]
	if !ok {
		return LocalTaskAgent{}, fmt.Errorf("task Agent %q was not found", name)
	}
	return candidate, nil
}

func collectTaskEvents(ctx context.Context, observation agent.Observation, runID string, after agent.Cursor) ([]TaskEvent, string, string, bool, error) {
	var events []TaskEvent
	var output string
	target := observation.Snapshot.Cursor
	cursor := after
	if cursor >= target {
		return nil, strconv.FormatUint(uint64(target), 10), "", observation.Snapshot.MessagesTruncated, nil
	}
	for {
		select {
		case event, ok := <-observation.Events:
			if !ok {
				return events, strconv.FormatUint(uint64(cursor), 10), output, true, nil
			}
			cursor = event.Cursor
			if event.RunID != runID {
				if cursor >= target {
					return events, strconv.FormatUint(uint64(target), 10), output, observation.Snapshot.MessagesTruncated, nil
				}
				continue
			}
			projected := TaskEvent{
				Cursor: strconv.FormatUint(uint64(event.Cursor), 10), Type: taskEventType(event.Payload),
				Durability: event.Durability, Run: event.RunID, Event: event,
			}
			switch payload := event.Payload.(type) {
			case agent.AssistantDelta:
				projected.Type, projected.Text = "assistant_delta", payload.Delta
			case agent.ThinkingDelta:
				projected.Type, projected.Text = "thinking_delta", payload.Delta
			case agent.AssistantFinal:
				projected.Type, projected.Text, output = "assistant_final", payload.Content, payload.Content
			case agent.ToolStarted:
				projected.Type, projected.Tool = "tool_started", payload.Name
			case agent.ToolProgress:
				projected.Type, projected.Tool, projected.Text = "tool_progress", payload.Name, payload.Delta
			case agent.ToolFinished:
				projected.Type, projected.Tool, projected.Text = "tool_finished", payload.Name, payload.Result
			case agent.InteractionRequested:
				projected.Type = "interaction_requested"
			case agent.RunSettled:
				projected.Type, projected.Text = "run_settled", string(payload.Status)
			}
			events = append(events, projected)
			if cursor >= target {
				return events, strconv.FormatUint(uint64(target), 10), output, observation.Snapshot.MessagesTruncated, nil
			}
		case err, ok := <-observation.Errors:
			if ok && err != nil {
				return events, strconv.FormatUint(uint64(cursor), 10), output, true, err
			}
			observation.Errors = nil
		case <-ctx.Done():
			return events, strconv.FormatUint(uint64(cursor), 10), output, true, ctx.Err()
		}
	}
}

func taskEventType(payload agent.EventPayload) string {
	switch payload.(type) {
	case agent.RunAccepted:
		return "run_accepted"
	case agent.RunStarted:
		return "run_started"
	case agent.AssistantDelta:
		return "assistant_delta"
	case agent.ThinkingDelta:
		return "thinking_delta"
	case agent.ModelCompleted:
		return "model_completed"
	case agent.ContextNormalized:
		return "context_normalized"
	case agent.AssistantFinal:
		return "assistant_final"
	case agent.ToolInputStarted:
		return "tool_input_started"
	case agent.ToolInputDelta:
		return "tool_input_delta"
	case agent.ToolStarted:
		return "tool_started"
	case agent.ToolProgress:
		return "tool_progress"
	case agent.ToolFinished:
		return "tool_finished"
	case agent.ArtifactProduced:
		return "artifact_produced"
	case agent.RecoveryRequired:
		return "recovery_required"
	case agent.RecoveryResumed:
		return "recovery_resumed"
	case agent.EventStreamGap:
		return "event_stream_gap"
	case agent.GoalUpdated:
		return "goal_updated"
	case agent.TodoUpdated:
		return "todo_updated"
	case agent.InteractionRequested:
		return "interaction_requested"
	case agent.InteractionResolved:
		return "interaction_resolved"
	case agent.CompactionStarted:
		return "compaction_started"
	case agent.CompactionCommitted:
		return "compaction_committed"
	case agent.CompactionRemoved:
		return "compaction_removed"
	case agent.CompactionFailed:
		return "compaction_failed"
	case agent.CompactionSkipped:
		return "compaction_skipped"
	case agent.CleanupStarted:
		return "cleanup_started"
	case agent.CleanupCompleted:
		return "cleanup_completed"
	case agent.CleanupFailed:
		return "cleanup_failed"
	case agent.CleanupSkipped:
		return "cleanup_skipped"
	case agent.CleanupCommitted:
		return "cleanup_committed"
	case agent.SessionCleared:
		return "session_cleared"
	case agent.TranscriptSynchronized:
		return "transcript_synchronized"
	case agent.ContextLimitReached:
		return "context_limit_reached"
	case agent.RunSettled:
		return "run_settled"
	case agent.NestedEvent:
		return "nested"
	default:
		return "unknown"
	}
}

func taskStatus(snapshot agent.SessionSnapshot, runID string) string {
	if snapshot.ActiveRunID == runID {
		return "running"
	}
	for _, recent := range snapshot.RecentRuns {
		if recent.ID == runID {
			return string(recent.Status)
		}
	}
	return "unknown"
}

func taskSnapshotOutput(snapshot agent.SessionSnapshot, runID string) string {
	if snapshot.ActiveRunID == runID {
		return snapshot.ActiveOutput.Content
	}
	return ""
}

func parseTaskCursor(value string) (agent.Cursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid task cursor: %w", err)
	}
	return agent.Cursor(parsed), nil
}

func replayTaskFinal(
	ctx context.Context,
	session *agent.Session,
	runID string,
	retentionStart agent.Cursor,
) (string, bool, error) {
	after := agent.Cursor(0)
	if retentionStart > 0 {
		after = retentionStart - 1
	}
	observation, err := session.Observe(ctx, after)
	if err != nil {
		return "", true, err
	}
	_, _, output, incomplete, err := collectTaskEvents(ctx, observation, runID, after)
	return output, incomplete || output == "", err
}

func localTaskSessionID(agentName, commandID string) string {
	return toolsetIdentity("task.session", struct{ Agent, Command string }{agentName, commandID}).ConfigHash[:32]
}

func localTaskID(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate task id: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(value[:]), nil
}

func responseFromResolution(resolution agent.InteractionResolution) agent.InteractionResponse {
	return agent.InteractionResponse{
		Answers:    append([]agent.InteractionAnswer(nil), resolution.Answers...),
		Permission: resolution.Permission, Cancelled: resolution.Cancelled,
	}
}

func localTaskSessionKey(candidate LocalTaskAgent, sessionID string) agent.SessionKey {
	attributes := cloneTaskAttributes(candidate.Attributes)
	if attributes == nil {
		attributes = make(map[string]string, 1)
	}
	attributes["agent"] = candidate.Name
	return agent.SessionKey{Namespace: "task." + candidate.Name, ID: sessionID, Attributes: attributes}
}

func cloneTaskAttributes(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for name, value := range source {
		cloned[name] = value
	}
	return cloned
}

func cloneTaskInteractions(source []agent.InteractionRequest) []agent.InteractionRequest {
	if len(source) == 0 {
		return nil
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		return append([]agent.InteractionRequest(nil), source...)
	}
	var cloned []agent.InteractionRequest
	if json.Unmarshal(encoded, &cloned) != nil {
		return append([]agent.InteractionRequest(nil), source...)
	}
	return cloned
}

var _ TaskExecutor = (*LocalTasks)(nil)
