package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	"fmt"
	agent "github.com/alfredxw/denova/agent"
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
)

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
	ResumeInterruptionID string
	StyleScenes          []string
	Locale               string
	InputVisibility      agentrun.InputVisibility
	RegenerateFromTurnID string
	AttachmentIDs        []string
	AttachedFiles        []agent.Attachment
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
	if storyContext.Meta.PlanningMode == interactive.StoryPlanningModeEnabled {
		gamePreset := interactiveapp.LoadStoryDirectorForMeta(cycle.novaDir, storyContext.Meta)
		cycle.tellerInput.PlanningGuide = interactive.StoryPlanningGuideMarkdown(gamePreset, interactiveapp.StoryRuntimeContextMaxBytes)
	}
	cycle.request = agentchat.ChatRequest{
		Message: strings.TrimSpace(request.Message), ResumeInterruptionID: strings.TrimSpace(request.ResumeInterruptionID),
		StyleScenes:   append([]string(nil), request.StyleScenes...),
		AttachmentIDs: append([]string(nil), request.AttachmentIDs...),
		AttachedFiles: append([]agent.Attachment(nil), request.AttachedFiles...),
		StyleRules:    styleRules, Locale: strings.TrimSpace(request.Locale), InputVisibility: request.InputVisibility,
	}
	cycle.conversation = interactiveapp.NewConversation(
		cycle.store, cycle.novaDir, cycle.workspace, cycle.storyID, cycle.branchID,
		cycle.request.Message, cycle.runtimeCfg.InteractiveReplyTargetChars, &cycle.runtimeCfg,
	).WithInputVisibility(cycle.request.InputVisibility).WithBaseParentID(expectedHead).WithRegenerateTarget(regenerateTurnID).WithExecutionParentPinning().WithOpeningStateSchema(storyContext)

	var submitOpeningStateSchema func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	if interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyContext.Meta.StateSchemaPolicy) &&
		storyContext.Meta.StateSchemaInitialization != nil &&
		storyContext.Meta.StateSchemaInitialization.Status == interactive.StateSchemaInitializationWaitingOpening &&
		interactiveapp.SnapshotTurnCount(storyContext.Snapshot) == 0 {
		submitOpeningStateSchema = cycle.conversation.SubmitOpeningStateSchemaBatch
	}
	agentHost, err := a.HarnessAgentHostCapabilities(ctx, &cycle.runtimeCfg, config.AgentKindInteractiveStory)
	if err != nil {
		return nil, err
	}
	builtAgent, err := appagentruntime.BuildInteractiveAgent(ctx, &cycle.runtimeCfg, cycle.state, cycle.tellerInput, agentHost, agentinteractive.InteractiveStoryToolContext{
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
		ProjectID:          c.runtimeCfg.ProjectID,
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
		_, _, persisted := c.conversation.LastTurnForState()
		if !persisted {
			if outcome.Status == agentrun.OutcomeCompleted {
				return fmt.Errorf("interactive agent cycle completed without a persisted turn")
			}
			return nil
		}
		_, err := emitInteractiveTurnPersistedResult(c.store, c.storyID, c.conversation, emit)
		if err != nil {
			return err
		}
		return nil
	})
}
