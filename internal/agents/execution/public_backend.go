package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentdelegation "denova/internal/agents/delegation"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolartifact "denova/internal/agents/toolartifact"
	agenttoolruntime "denova/internal/agents/toolruntime"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
	publictools "github.com/alfredxw/denova/agent/tools"
)

type publicBackend struct {
	lifecycle           context.Context
	cancel              context.CancelFunc
	agent               *agent.Agent
	profiles            *profileRegistry
	effects             agenttoolruntime.ToolMutationApplier
	permissionRuleStore PermissionRuleStore
	childDefinitions    ChildDefinitionResolver

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
	commandKind      CommandKind
	emit             func(agentrun.Event)
	projector        *agentchat.PublicEventProjector
	mutations        map[string]agenttool.Mutation
	mutationOrder    []string
	verificationDone bool
	projectorBound   bool
	trace            *publicAgentRunTrace
	pendingRunStart  *pendingPublicRunStart
}

type pendingPublicRunStart struct {
	runID   string
	started agent.RunStarted
}

type publicRunHandle struct {
	session      *agent.Session
	run          *agent.Run
	registration *publicCycleRegistration
	trace        *publicAgentRunTrace
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
	if resolved.toolMutationApplier == nil {
		return nil, errors.New("Agent runtime requires a Tool mutation applier")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lifecycle, cancel := context.WithCancel(ctx)
	backend := &publicBackend{
		lifecycle: lifecycle, cancel: cancel, profiles: profiles,
		effects: resolved.toolMutationApplier, permissionRuleStore: resolved.permissionRuleStore,
		childDefinitions: resolved.childDefinitions,
		registrations:    make(map[string]*publicCycleRegistration),
		cycles:           make(map[string]map[int]*publicCycleRegistration), runs: make(map[string]*publicRunHandle),
		successors: make(map[string]*publicRunHandle),
	}
	source, err := agentlifecycle.NewSource(backend)
	if err != nil {
		cancel()
		return nil, err
	}
	owner, err := agentlifecycle.New(lifecycle, source, agentlifecycle.Config{
		StoreRoot: root + "/agent-transcripts", CacheKeyGenerator: denovaProviderCacheKey,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	backend.agent = owner
	return &Runtime{public: backend}, nil
}

// NewEphemeralRuntime constructs the same public lifecycle over an in-memory
// Session store. It is intended for focused tests and local compositions that
// do not require transcript persistence.
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
	source, err := agentlifecycle.NewSource(backend)
	if err != nil {
		cancel()
		panic(err)
	}
	owner, err := agent.New(
		lifecycle, source,
		agent.WithSessionStore(agentsession.Memory()),
		agent.WithRunIDGenerator(agentlifecycle.DefaultRunIDGenerator),
		agent.WithCacheKeyGenerator(denovaProviderCacheKey),
	)
	if err != nil {
		cancel()
		panic(err)
	}
	backend.agent = owner
	return &Runtime{public: backend}
}

func denovaProviderCacheKey(key agent.SessionKey) (string, error) {
	providerKey := key
	if strings.HasPrefix(key.Namespace, "task.") {
		parent, err := agentdelegation.ParentSession(key)
		if err != nil {
			return "", err
		}
		providerKey = parent
	}
	root, err := agentrun.SessionKeyForAgentSession(providerKey)
	if err != nil {
		return "", err
	}
	if providerKey.Namespace == key.Namespace && providerKey.ID == key.ID {
		return root, nil
	}
	encoded, _ := json.Marshal(key)
	digest := sha256.Sum256(encoded)
	return root + "-task-" + hex.EncodeToString(digest[:16]), nil
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
	if strings.HasPrefix(request.Agent.Session.Key.Namespace, "task.") {
		return backend.resolveTaskDefinition(ctx, request.Agent)
	}
	if request.Agent.Reason == agent.TurnReasonGoalMutation {
		// Goal mutations are Session capability operations, not product turns.
		// They deliberately carry no opaque turn HostData and need only the
		// stable Denova Goal adapter to apply their durable CAS transition.
		return agent.Definition{Goal: agentlifecycle.NewGoalManager()}, nil
	}
	data, err := agentlifecycle.DecodeTurnHostDataFromPrepare(request.Agent)
	if err != nil {
		return agent.Definition{}, err
	}
	// Run.CommandID identifies the operation's initial admission. Every later
	// cycle carries its own caller command in host data and must bind that exact
	// Denova request (references, Skills, Plan mode, and delivery metadata).
	commandID := strings.TrimSpace(data.Caller.CommandID)
	if commandID == "" {
		commandID = strings.TrimSpace(request.Agent.Run.CommandID)
	}
	inspection := agent.IsInspection(ctx)
	var registration *publicCycleRegistration
	if inspection {
		registration, err = inspectionRegistrationFromContext(ctx, request.Agent.Session.Key, data)
		if err != nil {
			return agent.Definition{}, err
		}
	} else {
		registration = backend.registration(request.Agent.Session.Key, commandID)
	}
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
		routedOptions := agentrun.Options{}
		var routedEmit func(agentrun.Event)
		if registration != nil {
			registration.mu.RLock()
			routedOptions = registration.options
			routedEmit = registration.emit
			registration.mu.RUnlock()
		}
		cycle, err = backend.restoreCycle(ctx, request.Binding, request.Agent, data, commandID, routedOptions, routedEmit)
		if err != nil {
			return agent.Definition{}, err
		}
		if registration == nil {
			registration = &publicCycleRegistration{
				request: data.ChatRequest(), options: cycle.Options,
				commandKind: commandKindFromPublicTurn(data.Kind),
			}
			if !inspection {
				backend.rememberRegistration(request.Agent.Session.Key, commandID, registration)
			}
		}
		registration.mu.Lock()
		registration.cycle = &cycle
		if registration.commandKind == "" {
			registration.commandKind = commandKindFromPublicTurn(data.Kind)
		}
		registration.mu.Unlock()
	}
	if cycle.Definition.Model == nil {
		return agent.Definition{}, errors.New("Denova public Agent cycle has no Definition")
	}
	if !inspection {
		backend.bindSharedRunTrace(request.Agent.Run.ID, registration)
	}
	definition, err := backend.bindDefinition(ctx, request.Agent, cycle, registration)
	if err != nil {
		return agent.Definition{}, err
	}
	if !inspection {
		backend.mu.Lock()
		if backend.cycles[request.Agent.Run.ID] == nil {
			backend.cycles[request.Agent.Run.ID] = make(map[int]*publicCycleRegistration)
		}
		backend.cycles[request.Agent.Run.ID][request.Agent.Run.Cycle] = registration
		backend.mu.Unlock()
	}
	return definition, nil
}

func (backend *publicBackend) ResolveDefinition(
	ctx context.Context,
	request agentlifecycle.DefinitionRequest,
) (agent.Definition, error) {
	return backend.resolveDefinition(ctx, request)
}

func (backend *publicBackend) ResolveCanonicalInput(
	ctx context.Context,
	request agentlifecycle.DefinitionRequest,
) (agent.CanonicalAdapter, error) {
	if strings.HasPrefix(request.Agent.Session.Key.Namespace, "task.") {
		// A child Session owns its prompt as ordinary public Agent transcript.
		// It must never append that prompt through the parent product canonical
		// adapter; the immutable parent route is used only to rebuild Definition.
		return nil, nil
	}
	data, err := agentlifecycle.DecodeTurnHostDataFromPrepare(request.Agent)
	if err != nil {
		return nil, err
	}
	commandID := strings.TrimSpace(data.Caller.CommandID)
	if commandID == "" {
		commandID = strings.TrimSpace(request.Agent.Run.CommandID)
	}
	registration := backend.registration(request.Agent.Session.Key, commandID)
	if registration != nil {
		registration.mu.RLock()
		prepared := registration.cycle
		registration.mu.RUnlock()
		if prepared != nil && prepared.Conversation != nil {
			cycle := *prepared
			// Keep the server-resolved trusted request captured at admission.
			// HostData intentionally carries only caller-controlled identity and
			// cannot reconstruct review bodies/references or other canonical data.
			if caller := agentchat.CallerView(cycle.Request); caller.CommandID != commandID || caller.Message != request.Agent.Input.Text {
				return nil, errors.New("registered canonical input changed after acceptance")
			}
			return backend.preparedCycleCanonicalInput(ctx, request.Agent, cycle, registration)
		}
	}
	profileID, err := request.Binding.ProfileID()
	if err != nil {
		return nil, err
	}
	profile, err := backend.profiles.profile(profileID)
	if err != nil {
		return nil, err
	}
	inputProfile, ok := profile.(CanonicalInputProfile)
	if !ok {
		return nil, fmt.Errorf("%w: profile %q has no provider-free canonical input boundary", ErrProfileInvalid, profile.ID())
	}
	routedOptions := agentrun.Options{}
	if registration != nil {
		registration.mu.RLock()
		routedOptions = registration.options
		registration.mu.RUnlock()
	}
	options := mergePublicCycleRoute(publicOptions(request.Binding, data), routedOptions)
	chatRequest := data.ChatRequest()
	chatRequest.CommandID = commandID
	kind := commandKindFromPublicTurn(data.Kind)
	if kind == "" {
		return nil, fmt.Errorf("resolve Denova Agent canonical input: invalid turn kind %q", data.Kind)
	}
	if request.Agent.Input.Text != agentchat.CallerView(chatRequest).Message {
		return nil, errors.New("Denova Agent canonical input does not match its durable caller payload")
	}
	adapter, err := inputProfile.CanonicalInput(ctx, CanonicalInputRequest{
		Session: request.Agent.Session.Key, Identity: denovaCanonicalIdentity(request.Agent.Session.Key),
		Binding: request.Binding, Kind: kind, CommandID: agentrun.CommandID(commandID),
		RunID: agentrun.OperationID(request.Agent.Run.ID), Cycle: request.Agent.Run.Cycle,
		Request: chatRequest, Options: options, Input: request.Agent.Input,
	})
	if err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, fmt.Errorf("%w: profile %q returned no canonical input adapter", ErrProfileInvalid, profile.ID())
	}
	return adapter, nil
}

// preparedCycleCanonicalInput supports the already-composed Start path without
// consulting a Profile. It constructs only the product committer and never
// assembles Context, Tools, or a model request.
func (backend *publicBackend) preparedCycleCanonicalInput(
	ctx context.Context,
	request agent.PrepareRequest,
	cycle Cycle,
	registration *publicCycleRegistration,
) (agent.CanonicalAdapter, error) {
	options := cycle.Options.Normalize(cycle.Options.Workspace)
	effectApplier, err := agentlifecycle.NewToolEffectApplier(backend.effects, options, registration.recordMutation)
	if err != nil {
		return nil, err
	}
	var committer agentlifecycle.ConversationCommitter
	switch conversation := cycle.Conversation.(type) {
	case *agentconversation.SessionConversation:
		registration.mu.RLock()
		projector := registration.projector
		registration.mu.RUnlock()
		committer, err = agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
			Conversation: conversation,
			Session:      conversation.CanonicalSession(),
			Options:      options,
			Request:      cycle.Request,
			ApplyEffects: effectApplier,
			InputEffect:  projectInputCommitEffect(options.InputCommitEffect, projector, options),
		})
	case agentlifecycle.ConversationCommitterProvider:
		committer, err = conversation.NewAgentConversationCommitter(options, effectApplier)
	default:
		err = fmt.Errorf("Denova conversation %T has no public canonical committer", cycle.Conversation)
	}
	if err != nil {
		return nil, err
	}
	boundary, err := agentlifecycle.NewConversationBoundary(agentlifecycle.ConversationBoundaryConfig{
		Conversation: cycle.Conversation,
		BookService:  cycle.BookService,
		Request:      cycle.Request,
		Options:      options,
		Committer:    committer,
		ContextIdentity: publicCapabilityIdentity(
			"denova.admission_context", request.Session.Key,
		),
		CanonicalIdentity: denovaCanonicalIdentity(request.Session.Key),
	})
	if err != nil {
		return nil, err
	}
	return boundary.CanonicalAdapter(), nil
}

func (backend *publicBackend) resolveTaskDefinition(
	ctx context.Context,
	request agent.PrepareRequest,
) (agent.Definition, error) {
	parent, err := agentdelegation.ParentSession(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	childName, err := agentdelegation.ChildName(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	binding, err := agentrun.RuntimeBindingFromAgentSessionKey(parent)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("resolve delegated parent binding: %w", err)
	}
	profileID, err := binding.ProfileID()
	if err != nil {
		return agent.Definition{}, err
	}
	if backend.childDefinitions == nil {
		return agent.Definition{}, fmt.Errorf("%w: profile %q cannot rebuild delegated Agents", ErrCyclePreparationUnavailable, profileID)
	}
	route, err := agentdelegation.ParentRoute(request.Session.Key)
	if err != nil {
		return agent.Definition{}, err
	}
	definition, err := backend.childDefinitions.PrepareChildDefinition(ctx, ChildDefinitionRequest{
		Parent: parent, Child: childName, HostData: route,
	})
	if err != nil {
		return agent.Definition{}, err
	}
	if definition.Model == nil || definition.Name != childName {
		return agent.Definition{}, errors.New("delegated Agent Definition does not match its durable selector")
	}
	definition.Permission = agentlifecycle.BindPermissionRuleStore(
		definition.Permission, backend.permissionRuleStore.Load, backend.permissionRuleStore.Persist,
	)
	if strings.TrimSpace(definition.AttachmentRoot) != "" {
		canonical, _ := agentsession.CanonicalKey(request.Session.Key)
		store, storeErr := agenttoolartifact.NewStateStore(definition.AttachmentRoot, canonical)
		if storeErr != nil {
			return agent.Definition{}, fmt.Errorf("create delegated Agent artifact Store: %w", storeErr)
		}
		definition.Artifacts, err = agent.IdentifyToolArtifactStorage(
			store, publicCapabilityIdentity("denova.task.tool_artifacts", request.Session.Key),
		)
		if err != nil {
			return agent.Definition{}, err
		}
	}
	return definition, nil
}

func (backend *publicBackend) bindSharedRunTrace(runID string, registration *publicCycleRegistration) {
	if backend == nil || registration == nil || strings.TrimSpace(runID) == "" {
		return
	}
	backend.mu.RLock()
	handle := backend.runs[runID]
	backend.mu.RUnlock()
	if handle == nil || handle.trace == nil {
		return
	}
	registration.mu.Lock()
	if registration.trace == nil {
		registration.trace = handle.trace
	}
	registration.mu.Unlock()
}

func (backend *publicBackend) restoreCycle(
	ctx context.Context,
	binding agentrun.RuntimeBinding,
	request agent.PrepareRequest,
	data agentlifecycle.TurnHostData,
	commandID string,
	routedOptions agentrun.Options,
	emit func(agentrun.Event),
) (Cycle, error) {
	profileID, err := binding.ProfileID()
	if err != nil {
		return Cycle{}, err
	}
	profile, err := backend.profiles.profile(profileID)
	if err != nil {
		return Cycle{}, err
	}
	queued, ok := profile.(QueuedCycleProfile)
	if !ok {
		return Cycle{}, fmt.Errorf("%w: profile %q cannot restore a public Agent cycle", ErrCyclePreparationUnavailable, profile.ID())
	}
	options := mergePublicCycleRoute(publicOptions(binding, data), routedOptions)
	chatRequest := data.ChatRequest()
	chatRequest.CommandID = commandID
	kind := commandKindFromPublicTurn(data.Kind)
	if kind == "" {
		return Cycle{}, fmt.Errorf("restore Denova Agent cycle: invalid turn kind %q", data.Kind)
	}
	return queued.PrepareCycle(ctx, CycleRestoreRequest{
		Binding: binding, Kind: kind, CommandID: agentrun.CommandID(commandID),
		OperationID: agentrun.OperationID(request.Run.ID), Request: chatRequest,
		Options: options, Deferred: request.Reason != agent.TurnReasonStart, Emit: emit,
	})
}

// mergePublicCycleRoute restores process-local display and callback routing
// without allowing it to replace durable product identity. Profiles still own
// the final Definition and may rebuild these values from current application
// state; this seed keeps recovery and queued cycles attached to the accepted
// task before profile preparation begins.
func mergePublicCycleRoute(durable, routed agentrun.Options) agentrun.Options {
	if routed.TaskID != "" {
		durable.TaskID = routed.TaskID
	}
	if routed.RootAgentName != "" {
		durable.RootAgentName = routed.RootAgentName
	}
	if routed.StateRoot != "" {
		durable.StateRoot = routed.StateRoot
	}
	if routed.IdleTimeout != 0 {
		durable.IdleTimeout = routed.IdleTimeout
	}
	if routed.ToolResultMaxBytes > 0 {
		durable.ToolResultMaxBytes = routed.ToolResultMaxBytes
	}
	if routed.Controls != nil {
		durable.Controls = routed.Controls
	}
	if routed.OnMutationsVerified != nil {
		durable.OnMutationsVerified = routed.OnMutationsVerified
	}
	if routed.InputCommitEffect != nil {
		durable.InputCommitEffect = routed.InputCommitEffect
	}
	return durable.Normalize(durable.Workspace)
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
	pendingRunStart := registration.pendingRunStart
	commandKind := registration.commandKind
	registration.pendingRunStart = nil
	registration.request, registration.options = cycle.Request, options
	registration.mu.Unlock()
	if pendingRunStart != nil {
		projector.ProjectRunStarted(
			pendingRunStart.runID,
			pendingRunStart.started.Cycle,
			firstPublicCycleValue(pendingRunStart.started.CommandID, cycle.Request.CommandID),
			firstPublicCycleValue(pendingRunStart.started.Delivery, string(commandKind)),
			pendingRunStart.started.StartedAt,
		)
	}
	effectApplier, err := agentlifecycle.NewToolEffectApplier(backend.effects, options, registration.recordMutation)
	if err != nil {
		return agent.Definition{}, err
	}
	var committer agentlifecycle.ConversationCommitter
	switch conversation := cycle.Conversation.(type) {
	case *agentconversation.SessionConversation:
		inputEffect := projectInputCommitEffect(options.InputCommitEffect, projector, options)
		committer, err = agentlifecycle.NewSessionConversationCommitter(agentlifecycle.SessionCommitterConfig{
			Conversation: conversation, Session: conversation.CanonicalSession(), Options: options,
			Request:      cycle.Request,
			ApplyEffects: effectApplier,
			InputEffect:  inputEffect,
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
		CanonicalIdentity: denovaCanonicalIdentity(request.Session.Key),
		OnPrepared:        projector.ProjectPreparedContext,
		ProjectOutput:     projector.ProjectCanonicalOutput,
	})
	if err != nil {
		return agent.Definition{}, err
	}
	definition := cycle.Definition
	definition.AttachmentRoot = options.StateRoot
	definition.Execution.IdleTimeout = options.IdleTimeout
	if taskCatalog, ok := agentdelegation.AsCatalog(definition.Tools); ok {
		parentAttributes, attributeErr := agentdelegation.ParentAttributes(request.Session.Key)
		if attributeErr != nil {
			return agent.Definition{}, attributeErr
		}
		route := request.HostData
		if route == nil {
			route = request.Input.HostData
		}
		attributes, attributeErr := agentdelegation.WithParentRoute(parentAttributes, route)
		if attributeErr != nil {
			return agent.Definition{}, attributeErr
		}
		children := taskCatalog.Children()
		candidates := make([]publictools.LocalTaskAgent, len(children))
		for index, child := range children {
			candidates[index] = publictools.LocalTaskAgent{
				Name: child.Name, Description: child.Description,
				Opener: backend.agent, Identity: child.Identity,
				Attributes: attributes, LookupAttributes: parentAttributes,
			}
		}
		executor, taskErr := publictools.NewLocalTasks(publictools.LocalTaskOptions{
			Parallelism: taskCatalog.Parallelism(),
		}, candidates...)
		if taskErr != nil {
			return agent.Definition{}, fmt.Errorf("bind delegated Agent executor: %w", taskErr)
		}
		definition.Tools, taskErr = taskCatalog.Bind(executor)
		if taskErr != nil {
			return agent.Definition{}, fmt.Errorf("bind delegated Agent Toolset: %w", taskErr)
		}
	}
	if provider, ok := cycle.Conversation.(agentchat.ToolArtifactStoreProvider); ok {
		store := provider.ToolArtifactStore()
		if store != nil {
			definition.Artifacts, err = agent.IdentifyToolArtifactStorage(
				store, publicCapabilityIdentity("denova.tool_artifacts", identityConfig),
			)
			if err != nil {
				return agent.Definition{}, err
			}
		}
	}
	definition.Compaction = agentlifecycle.BindConversationCompaction(definition.Compaction, cycle.Conversation)
	definition.Context, err = agent.CombineContextSources(definition.Context, boundary.ContextSource())
	if err != nil {
		return agent.Definition{}, fmt.Errorf("compose project and conversation ContextSources: %w", err)
	}
	definition.Canonical = boundary.CanonicalAdapter()
	definition.Permission = agentlifecycle.BindPermissionRuleStore(
		definition.Permission, backend.permissionRuleStore.Load, backend.permissionRuleStore.Persist,
	)
	var trace agentchat.PublicRunTraceBinder = registration
	if agent.IsInspection(ctx) {
		trace = nil
	}
	host := agentchat.NewPublicHostMiddleware(cycle.Request, options, trace)
	definition.Middlewares = append(definition.Middlewares, agent.IdentifyMiddleware(
		host, publicCapabilityIdentity("denova.public_host", identityConfig),
	))
	return definition, nil
}

func projectInputCommitEffect(
	effect agentrun.InputCommitEffect,
	projector *agentchat.PublicEventProjector,
	options agentrun.Options,
) agentrun.InputCommitEffect {
	if effect == nil {
		return nil
	}
	return agentrun.InputCommitEffectFuncs{
		ApplyFunc: func(ctx context.Context, request agentrun.InputCommitEffectRequest) error {
			if err := effect.Apply(ctx, request); err != nil {
				return err
			}
			if projector != nil {
				projector.EmitProduct(agentrun.Event{Type: "workspace_change", Data: map[string]interface{}{
					"project_id":       options.ProjectID,
					"review_thread_id": options.ReviewThreadID, "action": "review_feedback_consumed",
				}})
			}
			return nil
		},
	}
}

func firstPublicCycleValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func publicCapabilityIdentity(kind string, value any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

func denovaCanonicalIdentity(key agent.SessionKey) agent.CapabilityIdentity {
	return publicCapabilityIdentity("denova.canonical", key)
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
		Cursor: agentrun.Cursor(receipt.Cursor),
	}
}

func mapPublicCommandReceipt(receipt agent.CommandReceipt) agentrun.CommandReceipt {
	return agentrun.CommandReceipt{
		CommandID: agentrun.CommandID(receipt.CommandID), OperationID: agentrun.OperationID(receipt.RunID),
		Cursor: agentrun.Cursor(receipt.Cursor),
	}
}
