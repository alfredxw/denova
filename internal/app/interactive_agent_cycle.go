package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	interactiveapp "denova/internal/app/interactive"
	appsettings "denova/internal/app/settings"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/interactive/director"
)

// SetInteractiveDirectorGeneratorForTest installs an App-scoped Director
// generator so tests do not share mutable package-level state.
func (a *App) SetInteractiveDirectorGeneratorForTest(generator interactiveapp.DirectorGenerator) func() {
	if a == nil {
		return func() {}
	}
	a.mu.Lock()
	previous := a.directorGenerator
	a.directorGenerator = generator
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		a.directorGenerator = previous
		a.mu.Unlock()
	}
}

func (a *App) interactiveDirectorGenerator() interactiveapp.DirectorGenerator {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.directorGenerator
}

// interactiveAgentCycle is the complete, process-local adapter state for one
// game model cycle. Durable commands retain only its bounded TurnSpecRef; the
// cycle owns the fresh conversation required to persist exactly one player
// turn against the branch head observed during preparation.
type interactiveAgentCycle struct {
	app              *App
	store            *interactive.Store
	state            *book.State
	bookService      *book.Service
	versionService   *book.VersionService
	executionRuntime *agentexecution.Runtime
	sessionStore     *session.Store
	runtimeCfg       config.Config
	workspace        string
	novaDir          string
	storyID          string
	branchID         string
	storyContext     interactive.StoryContext
	tellerInput      prompts.InteractiveStorySystemInstructionInput
	definition       agents.Definition
	systemPrompt     prompts.SystemPromptComposition
	conversation     *interactiveapp.Conversation
	request          agentchat.ChatRequest
}

type interactiveAgentCycleRequest struct {
	CommandID            string
	StoryID              string
	BranchID             string
	Message              string
	StyleScenes          []string
	Locale               string
	RegenerateFromTurnID string
}

func (s *InteractiveAppService) prepareInteractiveAgentCycle(ctx context.Context, request interactiveAgentCycleRequest) (*interactiveAgentCycle, error) {
	if s == nil || s.app == nil {
		return nil, ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil || a.executionRuntime == nil {
		a.mu.RUnlock()
		return nil, ErrNoWorkspace
	}
	cycle := &interactiveAgentCycle{
		app: a, store: a.interactive, state: a.bookState, bookService: a.bookService,
		versionService: a.versionService, executionRuntime: a.executionRuntime, sessionStore: a.sessionStore,
		runtimeCfg: *a.cfg, workspace: strings.TrimSpace(a.workspace),
		storyID: strings.TrimSpace(request.StoryID),
	}
	a.mu.RUnlock()
	if cycle.workspace == "" || cycle.storyID == "" {
		return nil, ErrNoWorkspace
	}
	cycle.runtimeCfg.Workspace = cycle.workspace
	cycle.novaDir = cycle.runtimeCfg.DataDir()

	if layered, err := config.LoadLayeredWithStartupConfigAt(
		cycle.novaDir, cycle.workspace, config.ProjectConfigPath(cycle.runtimeCfg.ProjectStateDir),
	); err == nil {
		appsettings.ApplyLayered(&cycle.runtimeCfg, layered)
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-cycle] loaded settings workspace=%s story_id=%s", cycle.workspace, cycle.storyID))
	} else {
		slog.ErrorContext(ctx, fmt.Sprintf("[interactive-agent-cycle] load settings failed workspace=%s story_id=%s err=%v", cycle.workspace, cycle.storyID, err))
	}
	appsettings.ApplyLocale(&cycle.runtimeCfg, request.Locale)

	canonicalContext, err := cycle.store.StoryContext(cycle.storyID, strings.TrimSpace(request.BranchID))
	if err != nil {
		return nil, err
	}
	cycle.branchID = strings.TrimSpace(canonicalContext.Snapshot.BranchID)
	branch, ok := canonicalContext.Meta.Branches[cycle.branchID]
	if !ok {
		return nil, fmt.Errorf("interactive branch metadata is missing: %s", cycle.branchID)
	}
	expectedHead := branch.Head
	storyContext := canonicalContext
	regenerateTurnID := strings.TrimSpace(request.RegenerateFromTurnID)
	if regenerateTurnID != "" {
		storyContext, err = cycle.store.StoryContextAtTurnParent(cycle.storyID, cycle.branchID, regenerateTurnID)
		if err != nil {
			return nil, err
		}
	}
	cycle.storyContext = storyContext
	if _, err := interactiveapp.ApplyConversationConfig(cycle.store, &cycle.runtimeCfg, cycle.storyID, cycle.branchID); err != nil {
		return nil, err
	}

	teller := interactiveapp.LoadGameTeller(cycle.novaDir, storyContext.Meta.StoryTellerID)
	cycle.runtimeCfg.InteractiveReplyTargetChars = storyContext.Meta.ReplyTargetChars
	styleRules := appagentruntime.StyleRules(cycle.novaDir, teller.StyleRefs, teller.StyleRules, request.StyleScenes)
	cycle.tellerInput = interactiveapp.StoryTellerSystemInput(teller, styleRules)
	cycle.tellerInput.ChoiceCount = storyContext.Meta.ChoiceCount
	cycle.request = agentchat.ChatRequest{
		Message: strings.TrimSpace(request.Message), StyleScenes: append([]string(nil), request.StyleScenes...),
		StyleRules: styleRules, Locale: strings.TrimSpace(request.Locale),
	}
	cycle.conversation = interactiveapp.NewConversation(
		cycle.store, cycle.novaDir, cycle.workspace, cycle.storyID, cycle.branchID,
		cycle.request.Message, cycle.runtimeCfg.InteractiveReplyTargetChars, &cycle.runtimeCfg,
	).BindDirectorRuntime(a.directorTasksForWorkspace(cycle.workspace), a.interactiveDirectorGenerator(), cycle.executionRuntime).WithBaseParentID(expectedHead).WithRegenerateTarget(regenerateTurnID).WithExecutionParentPinning().WithOpeningStateSchema(storyContext)
	cycle.bindDerivedProjectionBarrier()

	var submitOpeningStateSchema func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	if interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyContext.Meta.StateSchemaPolicy) &&
		storyContext.Meta.StateSchemaInitialization != nil &&
		storyContext.Meta.StateSchemaInitialization.Status == interactive.StateSchemaInitializationWaitingOpening &&
		interactiveapp.SnapshotTurnCount(storyContext.Snapshot) == 0 {
		submitOpeningStateSchema = cycle.conversation.SubmitOpeningStateSchemaBatch
	}
	builtAgent, err := appagentruntime.BuildInteractiveAgent(ctx, &cycle.runtimeCfg, cycle.state, cycle.tellerInput, agentinteractive.InteractiveStoryToolContext{
		Store:                  cycle.store,
		StoryID:                cycle.storyID,
		BranchID:               cycle.branchID,
		SubmitStateSchemaBatch: submitOpeningStateSchema,
		PrepareTurn:            cycle.conversation.PrepareInteractiveTurn,
		SubmitTurnResult:       cycle.conversation.SubmitTurnResult,
		TurnResultReady:        cycle.conversation.InteractiveNarrativeReady,
	})
	if err != nil {
		return nil, fmt.Errorf("build interactive story runner: %w", err)
	}
	cycle.definition, cycle.systemPrompt = builtAgent.Definition, builtAgent.Composition
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-cycle] prepared workspace=%s story_id=%s branch_id=%s message_bytes=%d style_rules=%d", cycle.workspace, cycle.storyID, cycle.branchID, len(cycle.request.Message), len(styleRules)))
	return cycle, nil
}

func (c *interactiveAgentCycle) options(taskID string) agentrun.Options {
	return agentrun.Options{
		AgentKind:          agentrun.AgentKindInteractiveStory,
		StateRoot:          c.runtimeCfg.ProjectStateDir,
		TaskID:             strings.TrimSpace(taskID),
		StoryID:            c.storyID,
		BranchID:           c.branchID,
		Workspace:          c.workspace,
		Mode:               "interactive",
		IdleTimeout:        appagentruntime.IdleTimeout(c.runtimeCfg),
		ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(c.runtimeCfg),
		SystemPromptLog:    c.systemPrompt,
		OnMutationsVerified: c.app.verifiedWorkspaceMutationCallback(
			"interactive_agent_post_run",
			c.versionService,
			versionAutoSettingsForConfig(&c.runtimeCfg),
		),
	}
}

// bindCommit installs the game projection commit before the execution cycle is
// registered. A fresh cycle/conversation therefore emits exactly its own turn
// for Start, Steer, and FollowUp commands.
func (c *interactiveAgentCycle) bindCommit(emit func(agentrun.Event)) {
	if c == nil || c.conversation == nil {
		return
	}
	c.conversation.WithAgentCycleCommit(func(_ context.Context, outcome agentrun.Outcome) error {
		if outcome.MaintenanceOnly {
			return nil
		}
		turn, _, persisted := c.conversation.LastTurnForState()
		if !persisted {
			if outcome.Status == agentrun.OutcomeCompleted {
				return fmt.Errorf("interactive agent cycle completed without a persisted turn")
			}
			return nil
		}
		snapshot, err := emitInteractiveTurnPersistedResult(c.store, c.storyID, c.conversation, emit)
		if err != nil {
			return err
		}
		c.scheduleDirectorMaintenance(turn, snapshot)
		return nil
	})
}

func (c *interactiveAgentCycle) scheduleDirectorMaintenance(turn interactive.TurnEvent, persistedSnapshot *interactive.Snapshot) <-chan struct{} {
	storyDirector := c.conversation.StoryDirectorForMeta(c.storyContext.Meta)
	policy := director.ResolveRunPolicy(c.storyContext.Meta.DirectorRunPolicy, storyDirector.Strategy.DirectorAgentMode)
	if persistedSnapshot == nil {
		loaded, err := c.store.Snapshot(c.storyID, turn.BranchID)
		if err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-director-agent] load scheduling snapshot failed story_id=%s branch_id=%s turn_id=%s err=%v", c.storyID, turn.BranchID, turn.ID, err))
		} else {
			persistedSnapshot = &loaded
		}
	}
	committedTurns := interactiveapp.SnapshotTurnCount(c.storyContext.Snapshot) + 1
	planStatus := ""
	if c.storyContext.Snapshot.DirectorPlanStatus != nil {
		planStatus = c.storyContext.Snapshot.DirectorPlanStatus.Status
	}
	if persistedSnapshot != nil {
		committedTurns = interactiveapp.SnapshotTurnCount(*persistedSnapshot)
		if persistedSnapshot.DirectorPlanStatus != nil {
			planStatus = persistedSnapshot.DirectorPlanStatus.Status
		}
	}
	materialUpdate := turn.TurnResult != nil && turn.TurnResult.DirectorUpdate != nil && turn.TurnResult.DirectorUpdate.Needed
	decision := director.DecideRunAfterTurn(storyDirector.Strategy.Enabled, policy, director.ScheduleContext{
		CommittedTurns: committedTurns, PlanStatus: planStatus, MaterialUpdate: materialUpdate,
	})
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-director-agent] maintenance decision story_id=%s branch_id=%s turn_id=%s policy_mode=%s interval_turns=%d committed_turns=%d plan_status=%s run_plan=%t reason=%s", c.storyID, turn.BranchID, turn.ID, policy.Mode, policy.IntervalTurns, committedTurns, planStatus, decision.ShouldRun, decision.Reason))
	return interactiveapp.StartDirectorMaintenanceTask(&c.runtimeCfg, c.state, c.conversation, turn, c.sessionStore, decision.ShouldRun)
}

func (c *interactiveAgentCycle) bindDerivedProjectionBarrier() {
	if c == nil || c.conversation == nil {
		return
	}
	c.conversation.WithAgentCyclePrepare(func(ctx context.Context) error {
		return c.reconcilePreviousAgentCommit(ctx)
	})
}

// reconcilePreviousAgentCommit drains the canonical turn -> Director
// projection outbox before the next model cycle begins. The canonical Agent
// turn is the durable work item and DirectorPlanMetadata.DerivedThroughTurnID
// is its receipt. This barrier also covers Steer/FollowUp specs prepared while
// the preceding cycle was still running.
func (c *interactiveAgentCycle) reconcilePreviousAgentCommit(ctx context.Context) error {
	if c == nil || c.conversation == nil || c.store == nil {
		return nil
	}
	storyContext, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return fmt.Errorf("load canonical turn before Director projection barrier: %w", err)
	}
	c.storyContext = storyContext
	turn := storyContext.Snapshot.CurrentTurn
	if turn == nil || strings.TrimSpace(turn.AgentCommandID) == "" || strings.TrimSpace(turn.AgentOperationID) == "" || turn.AgentCycle <= 0 {
		return nil
	}
	if directorProjectionAcknowledged(storyContext, turn.ID) {
		return nil
	}

	key := interactiveapp.DerivedMaintenanceKey(c.conversation, turn.BranchID)
	if tasks := c.conversation.DirectorTasks(); tasks != nil && tasks.HasKey(key) {
		slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-cycle] wait live Director projection workspace=%s story_id=%s branch_id=%s turn_id=%s", c.workspace, c.storyID, turn.BranchID, turn.ID))
		if err := tasks.WaitKey(ctx, key); err != nil {
			return fmt.Errorf("wait live Director projection for turn %s: %w", turn.ID, err)
		}
		storyContext, err = c.store.StoryContext(c.storyID, turn.BranchID)
		if err != nil {
			return fmt.Errorf("reload Director projection receipt for turn %s: %w", turn.ID, err)
		}
		c.storyContext = storyContext
		if directorProjectionAcknowledged(storyContext, turn.ID) {
			return nil
		}
	}

	// A successful run record is already a durable projection result. This can
	// happen when the process stopped after writing it but before the explicit
	// outbox receipt; acknowledge without repeating the model call. Failed and
	// conflicting runs stay pending so a later cycle can repair them.
	if directorProjectionSucceededForTurn(storyContext, turn.ID) {
		if err := c.store.MarkDirectorTurnDerived(c.storyID, turn.BranchID, turn.ID); err != nil {
			return fmt.Errorf("acknowledge terminal Director projection for turn %s: %w", turn.ID, err)
		}
		return nil
	}

	maintenanceConversation := interactiveapp.NewConversation(
		c.store, c.novaDir, c.workspace, c.storyID, turn.BranchID,
		turn.User, c.runtimeCfg.InteractiveReplyTargetChars, &c.runtimeCfg,
	).InheritDirectorRuntime(c.conversation)
	repair := *c
	repair.conversation = maintenanceConversation
	repair.storyContext = storyContext
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-cycle] drain persisted Director outbox workspace=%s story_id=%s branch_id=%s turn_id=%s command_id=%s operation_id=%s cycle=%d", c.workspace, c.storyID, turn.BranchID, turn.ID, turn.AgentCommandID, turn.AgentOperationID, turn.AgentCycle))
	done := repair.scheduleDirectorMaintenance(*turn, &storyContext.Snapshot)
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	storyContext, err = c.store.StoryContext(c.storyID, turn.BranchID)
	if err != nil {
		return fmt.Errorf("reload drained Director projection for turn %s: %w", turn.ID, err)
	}
	c.storyContext = storyContext
	if directorProjectionAcknowledged(storyContext, turn.ID) {
		return nil
	}
	if directorProjectionSucceededForTurn(storyContext, turn.ID) {
		if err := c.store.MarkDirectorTurnDerived(c.storyID, turn.BranchID, turn.ID); err == nil {
			return nil
		}
	}
	return fmt.Errorf("Director projection for canonical turn %s finished without a durable receipt", turn.ID)
}

func directorProjectionAcknowledged(storyContext interactive.StoryContext, turnID string) bool {
	return storyContext.Snapshot.DirectorPlan != nil &&
		strings.TrimSpace(storyContext.Snapshot.DirectorPlan.Metadata.DerivedThroughTurnID) == strings.TrimSpace(turnID)
}

func directorProjectionSucceededForTurn(storyContext interactive.StoryContext, turnID string) bool {
	status := storyContext.Snapshot.DirectorPlanStatus
	return status != nil && strings.TrimSpace(status.SourceTurnID) == strings.TrimSpace(turnID) &&
		(status.Status == director.PlanStatusReady || status.Status == director.PlanStatusSkipped)
}
