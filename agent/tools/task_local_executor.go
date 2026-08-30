package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"
)

// SessionOpener is the narrow non-owning lifecycle seam needed by local
// delegation. agent.Agent implements it; an Executor never closes the owner.
type SessionOpener interface {
	Session(context.Context, agent.SessionKey) (*agent.Session, error)
	ListSessions(context.Context, agent.SessionSelector) ([]agent.SessionKey, error)
}

type activeSessionCounter interface {
	CountActiveSessions(context.Context, agent.SessionSelector) (int, error)
}

type LocalTaskOptions struct {
	Parallelism      int
	CompletionParent *agent.Session
	MaxResultBytes   int
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
	agents           map[string]LocalTaskAgent
	ordered          []TaskAgentInfo
	parallelism      int
	identity         agent.CapabilityIdentity
	completionParent *agent.Session
	maxResultBytes   int
	startMu          sync.Mutex
}

func NewLocalTasks(options LocalTaskOptions, candidates ...LocalTaskAgent) (*LocalTasks, error) {
	if len(candidates) == 0 {
		return nil, errors.New("local tasks require at least one Agent")
	}
	if options.Parallelism <= 0 {
		return nil, errors.New("local tasks require positive parallelism")
	}
	if options.MaxResultBytes <= 0 {
		options.MaxResultBytes = defaultResultBytes
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
		agents: resolved, ordered: ordered, parallelism: options.Parallelism,
		completionParent: options.CompletionParent, maxResultBytes: options.MaxResultBytes,
		identity: toolsetIdentity("tasks.local", struct {
			Parallelism    int
			MaxResultBytes int
			Agents         any
		}{options.Parallelism, options.MaxResultBytes, identities}),
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

	tasks.startMu.Lock()
	defer tasks.startMu.Unlock()
	if existing, found, existingErr := tasks.existingTask(ctx, candidate, sessionID, commandID); existingErr != nil {
		return Task{}, existingErr
	} else if found {
		if isTaskTerminal(existing.Status) {
			if completionErr := tasks.enqueueTaskCompletion(ctx, existing); completionErr != nil {
				return Task{}, completionErr
			}
		}
		return existing, nil
	}
	active, err := tasks.activeTaskCount(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("count active tasks: %w", err)
	}
	if active >= tasks.parallelism {
		return Task{}, fmt.Errorf("%w: %d active tasks reached the configured limit of %d", ErrTaskCapacityExceeded, active, tasks.parallelism)
	}
	session, err := candidate.Opener.Session(ctx, localTaskSessionKey(candidate, sessionID))
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
	tasks.watchCompletion(run, ref)
	return Task{Ref: ref, Status: "running"}, nil
}

func (tasks *LocalTasks) Observe(ctx context.Context, ref TaskRef, cursor string) (TaskObservation, error) {
	_, session, err := tasks.open(ctx, ref)
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
	task, err := taskFromSnapshot(ref, observation.Snapshot)
	if err != nil {
		return TaskObservation{}, err
	}
	result := TaskObservation{Task: task}
	result.Events, result.Cursor, result.Output, result.Incomplete, err = collectTaskEvents(ctx, observation, ref.Run, after)
	if result.Output == "" {
		result.Output = taskSnapshotOutput(observation.Snapshot, ref.Run)
	}
	terminal := isTaskTerminal(result.Task.Status)
	if err == nil && result.Output == "" && terminal {
		var replayIncomplete bool
		result.Output, replayIncomplete, err = replayTaskFinal(ctx, session, ref.Run, observation.Snapshot.RetentionStart)
		result.Incomplete = result.Incomplete || replayIncomplete
	}
	if result.Output != "" {
		result.Task.Output = result.Output
	}
	if terminal {
		err = errors.Join(err, session.Close(context.Background()))
	}
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

func (tasks *LocalTasks) existingTask(
	ctx context.Context,
	candidate LocalTaskAgent,
	sessionID string,
	commandID string,
) (Task, bool, error) {
	keys, err := taskSessionKeys(ctx, candidate, sessionID)
	if err != nil {
		return Task{}, false, err
	}
	switch len(keys) {
	case 0:
		return Task{}, false, nil
	case 1:
	default:
		return Task{}, false, errors.New("task Session identity is ambiguous")
	}
	session, err := candidate.Opener.Session(ctx, keys[0])
	if err != nil {
		return Task{}, false, err
	}
	snapshot, err := session.Snapshot(ctx)
	if err != nil {
		return Task{}, false, err
	}
	ref := TaskRef{Agent: candidate.Name, Session: sessionID}
	if snapshot.ActiveCommandID == commandID {
		ref.Run = snapshot.ActiveRunID
		task, taskErr := taskFromSnapshot(ref, snapshot)
		return task, true, taskErr
	}
	for index := len(snapshot.RecentRuns) - 1; index >= 0; index-- {
		if snapshot.RecentRuns[index].CommandID != commandID {
			continue
		}
		ref.Run = snapshot.RecentRuns[index].ID
		task, taskErr := tasks.taskFromSessionSnapshot(ctx, session, ref, snapshot)
		if closeErr := session.Close(context.Background()); closeErr != nil {
			taskErr = errors.Join(taskErr, closeErr)
		}
		return task, true, taskErr
	}
	return Task{}, false, errors.New("task Session idempotency identity is inconsistent")
}

func (tasks *LocalTasks) activeTaskCount(ctx context.Context) (int, error) {
	total := 0
	for _, candidate := range tasks.agents {
		selector := taskSessionSelector(candidate, "")
		if counter, ok := candidate.Opener.(activeSessionCounter); ok {
			count, err := counter.CountActiveSessions(ctx, selector)
			if err != nil {
				return 0, err
			}
			total += count
			continue
		}
		keys, err := candidate.Opener.ListSessions(ctx, selector)
		if err != nil {
			return 0, err
		}
		for _, key := range keys {
			session, openErr := candidate.Opener.Session(ctx, key)
			if openErr != nil {
				return 0, openErr
			}
			snapshot, snapshotErr := session.Snapshot(ctx)
			if snapshotErr != nil {
				return 0, snapshotErr
			}
			if snapshot.ActiveRunID != "" {
				total++
			} else if closeErr := session.Close(context.Background()); closeErr != nil {
				return 0, closeErr
			}
		}
	}
	return total, nil
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
	return candidate.Opener.ListSessions(ctx, taskSessionSelector(candidate, sessionID))
}

func taskSessionSelector(candidate LocalTaskAgent, sessionID string) agent.SessionSelector {
	attributes := cloneTaskAttributes(candidate.LookupAttributes)
	if attributes == nil {
		attributes = make(map[string]string, 1)
	}
	attributes["agent"] = candidate.Name
	return agent.SessionSelector{
		Namespace:  "task." + candidate.Name,
		ID:         sessionID,
		Attributes: attributes,
	}
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

var _ TaskExecutor = (*LocalTasks)(nil)
