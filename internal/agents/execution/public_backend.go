package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/harnessstate"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
	agentpermission "github.com/alfredxw/denova/agent/permission"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentstate "github.com/alfredxw/denova/agent/state"
)

type publicBackend struct {
	lifecycle context.Context
	cancel    context.CancelFunc
	agent     *agent.Agent
	profiles  *profileRegistry
	effects   agenttoolruntime.HostEffectReconciler
	state     *agentstate.Store

	mu            sync.RWMutex
	registrations map[string]*publicCycleRegistration
	cycles        map[string]map[int]*publicCycleRegistration
	runs          map[string]*publicRunHandle
	successors    map[string]*publicRunHandle
}

type publicCycleRegistration struct {
	mu               sync.RWMutex
	cycle            *Cycle
	request          agentchat.ChatRequest
	options          agentrun.Options
	emit             func(agentrun.Event)
	projector        *agentchat.PublicEventProjector
	mutations        map[string]agenttool.Mutation
	mutationOrder    []string
	verificationDone bool
	projectorBound   bool
}

type publicRunHandle struct {
	session      *agent.Session
	run          *agent.Run
	registration *publicCycleRegistration
	done         chan struct{}
}

// NewAgentRuntime constructs the Denova host on the public Agent -> Session ->
// Run lifecycle.
func NewAgentRuntime(ctx context.Context, dataDir string, options ...Option) (*Runtime, error) {
	root := strings.TrimSpace(dataDir)
	if root == "" {
		return nil, errors.New("Agent runtime data directory is required")
	}
	resolved := runtimeOptions{}
	for index, apply := range options {
		if apply == nil {
			return nil, fmt.Errorf("Agent runtime option %d is nil", index)
		}
		if err := apply(&resolved); err != nil {
			return nil, err
		}
	}
	profiles, err := newProfileRegistry(resolved.profiles)
	if err != nil {
		return nil, err
	}
	if resolved.hostEffectReconciler == nil {
		return nil, errors.New("Agent runtime requires a Tool effect reconciler")
	}
	stateStore, err := agentstate.Open(agentstate.Options{
		Root: filepath.Join(root, "state"), RuntimeRoot: filepath.Join(root, "runtime", "harness-state"),
	})
	if err != nil {
		return nil, fmt.Errorf("open Harness State Run pins: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lifecycle, cancel := context.WithCancel(ctx)
	backend := &publicBackend{
		lifecycle: lifecycle, cancel: cancel, profiles: profiles,
		effects:       resolved.hostEffectReconciler,
		state:         stateStore,
		registrations: make(map[string]*publicCycleRegistration),
		cycles:        make(map[string]map[int]*publicCycleRegistration), runs: make(map[string]*publicRunHandle),
		successors: make(map[string]*publicRunHandle),
	}
	source, err := agentlifecycle.NewSource(agentlifecycle.DefinitionResolverFunc(backend.resolveDefinition))
	if err != nil {
		cancel()
		return nil, err
	}
	owner, err := agentlifecycle.New(lifecycle, source, agentlifecycle.Config{StoreRoot: root + "/agent-sessions"})
	if err != nil {
		cancel()
		return nil, err
	}
	backend.agent = owner
	return &Runtime{public: backend}, nil
}

// NewEphemeralRuntime constructs the same public lifecycle over an in-memory
// Session store. It is intended for focused tests and local compositions that
// do not require cold restart recovery.
func NewEphemeralRuntime() *Runtime {
	lifecycle, cancel := context.WithCancel(context.Background())
	backend := &publicBackend{
		lifecycle: lifecycle, cancel: cancel,
		profiles:      &profileRegistry{profiles: make(map[ProfileID]Profile)},
		effects:       func(context.Context, agenttoolruntime.CommittedToolMutation) error { return nil },
		registrations: make(map[string]*publicCycleRegistration),
		cycles:        make(map[string]map[int]*publicCycleRegistration),
		runs:          make(map[string]*publicRunHandle),
		successors:    make(map[string]*publicRunHandle),
	}
	source, err := agentlifecycle.NewSource(agentlifecycle.DefinitionResolverFunc(backend.resolveDefinition))
	if err != nil {
		cancel()
		panic(err)
	}
	owner, err := agent.New(
		lifecycle, source,
		agent.WithSessionStore(agentsession.Memory()),
		agent.WithRunIDGenerator(agentlifecycle.DefaultRunIDGenerator),
	)
	if err != nil {
		cancel()
		panic(err)
	}
	backend.agent = owner
	return &Runtime{public: backend}
}

func (backend *publicBackend) close(ctx context.Context) error {
	if backend == nil {
		return nil
	}
	backend.cancel()
	if backend.agent == nil {
		return nil
	}
	return backend.agent.Close(ctx)
}

func (backend *publicBackend) resolveDefinition(
	ctx context.Context,
	request agentlifecycle.DefinitionRequest,
) (agent.Definition, error) {
	data, err := agentlifecycle.DecodeTurnHostDataFromPrepare(request.Agent)
	if err != nil {
		return agent.Definition{}, err
	}
	commandID := strings.TrimSpace(request.Agent.Run.CommandID)
	if commandID == "" {
		commandID = strings.TrimSpace(data.Caller.CommandID)
	}
	registration := backend.registration(request.Agent.Session.Key, commandID)
	var cycle Cycle
	if registration != nil {
		registration.mu.RLock()
		prepared := registration.cycle
		registration.mu.RUnlock()
		if prepared != nil {
			cycle = *prepared
		}
	}
	if cycle.Conversation == nil {
		cycle, err = backend.restoreCycle(ctx, request.Binding, request.Agent, data, commandID)
		if err != nil {
			return agent.Definition{}, err
		}
		if registration == nil {
			registration = &publicCycleRegistration{request: data.ChatRequest(), options: cycle.Options}
			backend.rememberRegistration(request.Agent.Session.Key, commandID, registration)
		}
		registration.mu.Lock()
		registration.cycle = &cycle
		registration.mu.Unlock()
	}
	if cycle.Definition.Model == nil {
		return agent.Definition{}, errors.New("Denova public Agent cycle has no Definition")
	}
	if backend.state != nil {
		if _, err := backend.state.BindRun(ctx, request.Agent.Run.ID, commandID); err != nil {
			return agent.Definition{}, fmt.Errorf("bind Harness State to public Agent Run: %w", err)
		}
	}
	definition, err := backend.bindDefinition(ctx, request.Agent, cycle, registration)
	if err != nil {
		return agent.Definition{}, err
	}
	backend.mu.Lock()
	if backend.cycles[request.Agent.Run.ID] == nil {
		backend.cycles[request.Agent.Run.ID] = make(map[int]*publicCycleRegistration)
	}
	backend.cycles[request.Agent.Run.ID][request.Agent.Run.Cycle] = registration
	backend.mu.Unlock()
	return definition, nil
}

func (backend *publicBackend) restoreCycle(
	ctx context.Context,
	binding agentrun.RuntimeBinding,
	request agent.PrepareRequest,
	data agentlifecycle.TurnHostData,
	commandID string,
) (Cycle, error) {
	ref, err := binding.Ref()
	if err != nil {
		return Cycle{}, err
	}
	profile, err := backend.profiles.profile(ref.Profile)
	if err != nil {
		return Cycle{}, err
	}
	queued, ok := profile.(QueuedCycleProfile)
	if !ok {
		return Cycle{}, fmt.Errorf("%w: profile %q cannot restore a public Agent cycle", ErrCyclePreparationUnavailable, profile.ID())
	}
	options := publicOptions(binding, data)
	chatRequest := data.ChatRequest()
	chatRequest.CommandID = commandID
	kind := commandKindFromPublicTurn(data.Kind)
	if kind == "" {
		return Cycle{}, fmt.Errorf("restore Denova Agent cycle: invalid turn kind %q", data.Kind)
	}
	ctx = harnessstate.WithRunID(ctx, request.Run.ID)
	return queued.PrepareCycle(ctx, CycleRestoreRequest{
		Binding: binding, Kind: kind, CommandID: agentrun.CommandID(commandID),
		OperationID: agentrun.OperationID(request.Run.ID), Request: chatRequest,
		Options: options, Deferred: request.Reason != agent.TurnReasonStart,
	})
}

func commandKindFromPublicTurn(kind agentlifecycle.TurnKind) CommandKind {
	switch kind {
	case agentlifecycle.TurnStart:
		return CommandStartTurn
	case agentlifecycle.TurnSteer:
		return CommandSteer
	case agentlifecycle.TurnFollowUp:
		return CommandFollowUp
	case agentlifecycle.TurnNext:
		return CommandNextTurn
	default:
		return ""
	}
}

func publicTurnKind(kind CommandKind) (agentlifecycle.TurnKind, error) {
	switch kind {
	case CommandStartTurn:
		return agentlifecycle.TurnStart, nil
	case CommandSteer:
		return agentlifecycle.TurnSteer, nil
	case CommandFollowUp:
		return agentlifecycle.TurnFollowUp, nil
	case CommandNextTurn:
		return agentlifecycle.TurnNext, nil
	default:
		return "", fmt.Errorf("command %q does not carry a Denova Agent turn", kind)
	}
}

func publicOptions(binding agentrun.RuntimeBinding, data agentlifecycle.TurnHostData) agentrun.Options {
	return agentrun.Options{
		AgentKind: binding.AgentKind, ProjectID: binding.ProjectID,
		AutomationTaskID: data.AutomationID,
		SessionID:        binding.SessionID, ReviewThreadID: data.ReviewThreadID,
		StoryID: binding.StoryID, BranchID: binding.BranchID, TurnID: data.TurnID,
		MaintenanceTask: data.MaintenanceTask, Workspace: binding.Workspace,
		Mode: data.Mode, WriteMode: data.WriteMode, WriteScope: data.WriteScope,
		RestoreData: data.RestoreData,
	}.Normalize(binding.Workspace)
}

func (backend *publicBackend) bindDefinition(
	ctx context.Context,
	request agent.PrepareRequest,
	cycle Cycle,
	registration *publicCycleRegistration,
) (agent.Definition, error) {
	options := cycle.Options.Normalize(cycle.Options.Workspace)
	registration.mu.Lock()
	if registration.projector == nil || !registration.projectorBound {
		registration.projector = agentchat.NewPublicEventProjector(cycle.Conversation, cycle.Request, options, registration.emit)
		registration.projectorBound = true
	}
	projector := registration.projector
	registration.request, registration.options = cycle.Request, options
	registration.mu.Unlock()
	effectApplier, err := agentlifecycle.NewToolEffectApplier(backend.effects, options, registration.recordMutation)
	if err != nil {
		return agent.Definition{}, err
	}
	var committer agentlifecycle.ConversationCommitter
	switch conversation := cycle.Conversation.(type) {
	case *agentconversation.SessionConversation:
		inputCommitted := options.OnUserMessageCommitted
		if inputCommitted != nil {
			inputCommitted = func(ctx context.Context) error {
				if err := options.OnUserMessageCommitted(ctx); err != nil {
					return err
				}
				projector.EmitProduct(agentrun.Event{Type: "workspace_change", Data: map[string]interface{}{
					"project_id":       options.ProjectID,
					"workspace":        options.Workspace,
					"review_thread_id": options.ReviewThreadID,
					"action":           "review_feedback_consumed",
				}})
				return nil
			}
		}
		committer, err = agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
			Conversation: conversation, Session: conversation.CanonicalSession(), Options: options,
			Request:        cycle.Request,
			ApplyEffects:   effectApplier,
			InputCommitted: inputCommitted,
			ProjectOutput: func(
				_ context.Context,
				_ agentchat.AgentContextPreparation,
				output agent.OutputCommitRequest,
				metadata session.MessageMetadata,
			) (agentlifecycle.SessionOutputCommit, error) {
				message, transcript := projector.ProjectCanonicalOutput(&output.Message)
				return agentlifecycle.SessionOutputCommit{Message: message, Metadata: metadata, Transcript: transcript}, nil
			},
		})
	case agentlifecycle.ConversationCommitterProvider:
		committer, err = conversation.NewAgentConversationCommitter(options, effectApplier)
	default:
		err = fmt.Errorf("Denova conversation %T has no public canonical committer", cycle.Conversation)
	}
	if err != nil {
		return agent.Definition{}, err
	}
	identityConfig := struct {
		Definition string
		Binding    agent.SessionKey
	}{cycle.Definition.Key, request.Session.Key}
	boundary, err := agentlifecycle.NewConversationBoundary(agentlifecycle.ConversationBoundaryConfig{
		Conversation: cycle.Conversation, BookService: cycle.BookService,
		Request: cycle.Request, Options: options, Committer: committer,
		ContextIdentity:   publicCapabilityIdentity("denova.context", identityConfig),
		CanonicalIdentity: publicCapabilityIdentity("denova.canonical", identityConfig),
	})
	if err != nil {
		return agent.Definition{}, err
	}
	definition := cycle.Definition
	definition.Compaction = agentlifecycle.BindConversationCompaction(definition.Compaction, cycle.Conversation)
	definition.Context = boundary.ContextSource()
	definition.Canonical = boundary.CanonicalAdapter()
	// Denova's orchestrator remains the product permission policy and durable
	// approval host. Select FullAccess at the generic fence to retain its
	// critical shell deny rules without prompting a second time after Denova has
	// already approved the exact tool call.
	definition.Permission = agentpermission.FullAccess()
	host := agentchat.NewPublicHostMiddleware(cycle.Conversation, cycle.Request, options, projector.EmitProduct)
	definition.Middlewares = append(definition.Middlewares, agent.IdentifyMiddleware(
		host, publicCapabilityIdentity("denova.public_host", identityConfig),
	))
	return definition, nil
}

func publicCapabilityIdentity(kind string, value any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

func (backend *publicBackend) registration(key agent.SessionKey, commandID string) *publicCycleRegistration {
	backend.mu.RLock()
	defer backend.mu.RUnlock()
	return backend.registrations[publicRegistrationKey(key, commandID)]
}

func (backend *publicBackend) rememberRegistration(key agent.SessionKey, commandID string, registration *publicCycleRegistration) {
	backend.mu.Lock()
	backend.registrations[publicRegistrationKey(key, commandID)] = registration
	backend.mu.Unlock()
}

func (backend *publicBackend) bindRecoveryRoute(
	key agent.SessionKey,
	commandID string,
	options agentrun.Options,
	emit func(agentrun.Event),
) *publicCycleRegistration {
	registration := backend.registration(key, commandID)
	if registration == nil {
		registration = &publicCycleRegistration{options: options, emit: emit}
		backend.rememberRegistration(key, commandID, registration)
		return registration
	}
	registration.mu.Lock()
	registration.options = options
	registration.emit = emit
	projector := registration.projector
	registration.mu.Unlock()
	if projector != nil {
		projector.SetEmit(emit)
	}
	return registration
}

func publicRegistrationKey(key agent.SessionKey, commandID string) string {
	encoded, _ := json.Marshal(key)
	return string(encoded) + "\x00" + strings.TrimSpace(commandID)
}

func publicResultOutcome(result agent.Result, err error, content, thinking string) agentrun.Outcome {
	status := agentrun.OutcomeFailed
	switch result.Status {
	case agent.ResultCompleted:
		status = agentrun.OutcomeCompleted
	case agent.ResultAborted:
		status = agentrun.OutcomeAborted
	case agent.ResultIncomplete, agent.ResultBlocked:
		status = agentrun.OutcomeFailed
	}
	return agentrun.NewOutcome(status, err, result.Reason, content, thinking)
}

func mapPublicReceipt(run *agent.Run) agentrun.CommandReceipt {
	if run == nil {
		return agentrun.CommandReceipt{}
	}
	receipt := run.Receipt()
	return agentrun.CommandReceipt{
		CommandID: agentrun.CommandID(receipt.CommandID), OperationID: agentrun.OperationID(receipt.RunID),
		Cursor: agentrun.Cursor(receipt.Cursor), Replayed: receipt.Replayed,
	}
}

func mapPublicCommandReceipt(receipt agent.CommandReceipt) agentrun.CommandReceipt {
	return agentrun.CommandReceipt{
		CommandID: agentrun.CommandID(receipt.CommandID), OperationID: agentrun.OperationID(receipt.RunID),
		Cursor: agentrun.Cursor(receipt.Cursor), Replayed: receipt.Replayed,
	}
}
