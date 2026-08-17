package interactiveapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/prompts"
	"denova/internal/book/lore"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

func (c *Conversation) BuildDirectorInstruction(turn interactive.TurnEvent) (string, error) {
	_, instruction, err := c.BuildDirectorModelInput(turn)
	return instruction, err
}

func (c *Conversation) BuildDirectorModelInput(turn interactive.TurnEvent) (DirectorStableContext, string, error) {
	stableContext, err := buildInteractiveDirectorStableContext(c.workspace)
	if err != nil {
		return DirectorStableContext{}, "", err
	}
	instruction, err := c.buildDirectorInstruction(turn, stableContext)
	if err != nil {
		return DirectorStableContext{}, "", err
	}
	assembledRevision, err := lore.NewStore(c.workspace).Revision()
	if err != nil {
		return DirectorStableContext{}, "", fmt.Errorf("read lore revision after Director context assembly: %w", err)
	}
	if strings.TrimSpace(assembledRevision) != strings.TrimSpace(stableContext.Revision) {
		return DirectorStableContext{}, "", fmt.Errorf("lore changed during Director context assembly: stable=%s dynamic=%s", strings.TrimSpace(stableContext.Revision), strings.TrimSpace(assembledRevision))
	}
	return stableContext, instruction, nil
}

func (c *Conversation) buildDirectorInstruction(turn interactive.TurnEvent, stableContext DirectorStableContext) (string, error) {
	if c == nil || c.store == nil {
		return "", fmt.Errorf("interactive story does not exist")
	}
	storyCtx, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return "", err
	}
	storyDirector := storyDirectorForSnapshot(c.StoryDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	modelHistory, activeCompaction, err := c.modelHistoryForCycle(storyCtx)
	if err != nil {
		return "", err
	}
	visibleHistory := buildInteractiveModelVisibleHistory(modelHistory, activeCompaction)
	historyText := formatInteractiveTurnHistoryWithCheckpoint(visibleHistory, activeCompaction, "No historical turns are available. Update the Director plan from this turn's audit.")
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
	} else if plan, err := c.store.DirectorPlan(c.storyID, storyCtx.Snapshot.BranchID); err == nil {
		directorPlan = plan
	}
	loreContext, err := buildInteractiveDirectorLoreContext(c.workspace, directorPlan, turn)
	if err != nil {
		return "", err
	}
	actorStateSnapshot := interactive.ActorStateRuntimeProjection(storyDirector.ActorState, storyCtx.Snapshot.State)
	openingInitialization := strings.TrimSpace(c.directorTask) == DirectorTaskOpeningPlan
	budget, err := newDirectorContextBudget(c.cfg, c.directorTask, stableContext)
	if err != nil {
		return "", err
	}
	title := budget.take("story.title", storyCtx.Meta.Title, 512)
	turnAudit := ""
	if !openingInitialization {
		turnAudit = budget.take("turn.audit", boundedJSON(interactiveDirectorTurnAudit(turn), interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	}
	planDocsMarkdown := formatDirectorDocumentsContext(directorPlan.Docs, directorPlan.Metadata.Docs)
	planDocs := budget.take("director_plan.docs", planDocsMarkdown, interactiveDirectorContextBytes)
	actorState := budget.take("actor_state.snapshot", boundedJSON(actorStateSnapshot, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	actorStateSchema := budget.take("actor_state.schema", interactive.ActorStateSchemaContext(storyDirector.ActorState, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	lore := budget.take("lore.relevant", loreContext, interactiveDirectorContextBytes)
	history := budget.take("turn.history", historyText, interactiveDirectorContextBytes)
	origin := budget.take("story.origin", storyCtx.Meta.Origin, interactiveDirectorContextBytes)
	planningTemplates := budget.take("director.strategy.templates", boundedJSON(storyDirector.Strategy.PlanningTemplates, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	planningSummary := budget.take("director.planning_summary", interactive.StoryDirectorPlanningSummary(storyDirector, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	strategyContext := budget.take("director.strategy.prompt", strategyPrompt, interactiveDirectorContextBytes)
	openingContext := ""
	if openingInitialization {
		openingContext = budget.take("story.opening_input", turn.User, 4*1024)
	}
	eventOpportunity, eventRuntime, eventIndex, eventErr := c.store.DirectorEventContext(c.storyID, storyCtx.Snapshot.BranchID, turn.ID)
	if eventErr != nil {
		return "", fmt.Errorf("read event-orchestration context: %w", eventErr)
	}
	eventCatalog := ""
	if len(eventIndex) > 0 {
		eventCatalog = budget.take("director.events", boundedJSON(eventIndex, interactiveDirectorContextBytes), interactiveDirectorContextBytes)
	}
	instruction := prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{
		Title:                       title,
		Origin:                      origin,
		OpeningContext:              openingContext,
		OpeningInitialization:       openingInitialization,
		StoryTellerID:               budget.take("story.teller_id", storyCtx.Meta.StoryTellerID, 128),
		StoryDirectorID:             budget.take("story.director_id", storyCtx.Meta.StoryDirectorID, 128),
		BranchID:                    budget.take("story.branch_id", storyCtx.Snapshot.BranchID, 128),
		TaskHint:                    budget.take("director.task", c.directorTaskHint(), 1024),
		DirectorPlanDocs:            planDocs,
		PlanningTemplates:           planningTemplates,
		BranchPlanningTurns:         storyDirector.Strategy.BranchPlanningTurns,
		LoreContext:                 lore,
		TurnAuditJSON:               turnAudit,
		TurnHistory:                 history,
		ActorStateSchema:            actorStateSchema,
		ActorState:                  actorState,
		StoryDirectorPlan:           planningSummary,
		StoryDirectorStrategyPrompt: strategyContext,
		DirectorEventCatalog:        eventCatalog,
		EventOpportunity:            budget.take("director.event_opportunity", boundedJSON(eventOpportunity, 4*1024), 4*1024),
		EventRuntime:                budget.take("director.event_runtime", boundedJSON(eventRuntime, 8*1024), 8*1024),
	})
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-director-agent] context budget story_id=%s branch_id=%s turn_id=%s instruction_bytes=%d stable_bytes=%d model_window_tokens=%d threshold_tokens=%d source_budget_tokens=%d fragments=%s", c.storyID, storyCtx.Snapshot.BranchID, turn.ID, len(instruction), len([]byte(stableContext.Content)), budget.contextWindowTokens, budget.thresholdTokens, budget.initialTokens, budget.trace()))
	slog.InfoContext(context.Background(), fmt.Sprintf(
		"[interactive-director-agent] context composition story_id=%s branch_id=%s turn_id=%s teller_id=%s story_director_id=%s director_plan=%s lore=%s turn_audit=%s actor_state=%s history=%s instruction=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		turn.ID,
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		PartSummary(planDocsMarkdown),
		PartSummary(loreContext),
		PartSummary(turnAudit),
		PartSummary(boundedJSON(actorStateSnapshot, interactiveDirectorContextBytes)),
		PartSummary(historyText),
		PartSummary(instruction),
	))
	return instruction, nil
}

func interactiveDirectorTurnAudit(turn interactive.TurnEvent) map[string]any {
	return map[string]any{
		"turn_id":          turn.ID,
		"branch_id":        turn.BranchID,
		"user_action":      boundedText(turn.User, 4*1024),
		"narrative":        boundedText(turn.Narrative, 16*1024),
		"rule_resolution":  turn.RuleResolution,
		"turn_result":      turn.TurnResult,
		"state_delta":      turn.StateDelta,
		"terminal_outcome": turn.TerminalOutcome,
	}
}

func interactiveDirectorEventCatalog(director interactive.StoryDirector) []interactive.DirectorEvent {
	events := interactive.DirectorEventCatalogFromStoryDirector(director)
	if len(events) > 32 {
		return events[:32]
	}
	return events
}

func boundedJSON(value any, limit int) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return boundedText(string(data), limit)
}

func boundedText(value string, limit int) string {
	trimmed, truncated := agentcontext.TrimUTF8Bytes(value, limit)
	if truncated {
		const marker = "\n... [truncated at context limit]"
		prefix, _ := agentcontext.TrimUTF8Bytes(value, max(0, limit-len(marker)))
		markerPart, _ := agentcontext.TrimUTF8Bytes(marker, limit-len(prefix))
		return prefix + markerPart
	}
	return trimmed
}

type directorContextBudget struct {
	remainingTokens     int
	initialTokens       int
	contextWindowTokens int
	thresholdTokens     int
	parts               []string
}

func newDirectorContextBudget(cfg *config.Config, task string, stableContext DirectorStableContext) (*directorContextBudget, error) {
	model := config.ResolveAgentModel(cfg, config.AgentKindInteractiveDirector)
	window := model.ContextWindowTokens
	if window <= 0 {
		window = config.DefaultContextWindowTokens
	}
	contextSettings := config.ResolveAgentContext(cfg, config.AgentKindInteractiveDirector)
	threshold := contextSettings.CompactionThreshold
	if threshold <= 0 {
		threshold = 0.80
	}
	thresholdTokens := int(float64(window) * threshold)
	composition, err := prompts.ComposeInteractiveDirectorInstruction(cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("compose interactive Director system prompt: %w", err)
	}
	emptyPrompt := prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{})
	if task == DirectorTaskOpeningPlan {
		emptyPrompt = prompts.InteractiveDirectorInstruction(prompts.InteractiveDirectorPromptInput{OpeningInitialization: true})
	}
	overheadMessages := []*agents.Message{
		agents.SystemMessage(composition.Instruction()),
		agents.UserMessage(emptyPrompt),
	}
	if stable := strings.TrimSpace(stableContext.Content); stable != "" {
		title := strings.TrimSpace(stableContext.Title)
		if title == "" {
			title = "Stable Model Context"
		}
		overheadMessages = append(overheadMessages, agents.UserMessage(agentcontext.StandaloneMessage(title, stable, "")))
	}
	overheadTokens := agentcontext.EstimateTokens(overheadMessages, nil)
	completionReserve, toolReserve := agentcompaction.EstimateProjectionReserves(cfg, config.AgentKindInteractiveDirector, 1024)
	toolSchemaAndRuntimeHeadroom := max(2048, window/100)
	available := max(0, thresholdTokens-overheadTokens-completionReserve-toolReserve-toolSchemaAndRuntimeHeadroom)
	return &directorContextBudget{
		remainingTokens:     available,
		initialTokens:       available,
		contextWindowTokens: window,
		thresholdTokens:     thresholdTokens,
	}, nil
}

func (b *directorContextBudget) take(source, value string, fragmentLimit int) string {
	originalBytes := len(value)
	if fragmentLimit <= 0 || fragmentLimit > interactive.DirectorContextMaxBytes {
		fragmentLimit = interactive.DirectorContextMaxBytes
	}
	kept := boundedText(value, fragmentLimit)
	kept = fitTextToTokenBudget(kept, b.remainingTokens)
	usedTokens := agentcontext.EstimateTokens([]*agents.Message{agents.UserMessage(kept)}, nil)
	if strings.TrimSpace(kept) == "" {
		usedTokens = 0
	}
	b.remainingTokens = max(0, b.remainingTokens-usedTokens)
	b.parts = append(b.parts, fmt.Sprintf("%s:%dB->%dB/%dt", source, originalBytes, len(kept), usedTokens))
	return kept
}

func (b *directorContextBudget) trace() string {
	return strings.Join(b.parts, ",")
}

func fitTextToTokenBudget(value string, tokenBudget int) string {
	if tokenBudget <= 0 || strings.TrimSpace(value) == "" {
		return ""
	}
	if agentcontext.EstimateTokens([]*agents.Message{agents.UserMessage(value)}, nil) <= tokenBudget {
		return value
	}
	low, high := 0, len(value)
	for low < high {
		mid := low + (high-low+1)/2
		candidate, _ := agentcontext.TrimUTF8Bytes(value, mid)
		if agentcontext.EstimateTokens([]*agents.Message{agents.UserMessage(candidate)}, nil) <= tokenBudget {
			low = mid
		} else {
			high = mid - 1
		}
	}
	trimmed, _ := agentcontext.TrimUTF8Bytes(value, low)
	return trimmed
}

func (c *Conversation) teller(tellerID string) teller.Definition {
	return LoadGameTeller(c.novaDir, tellerID)
}

func (c *Conversation) storyDirector(directorID string) interactive.StoryDirector {
	return loadStoryDirector(c.novaDir, directorID)
}

func (c *Conversation) StoryDirectorForMeta(meta interactive.StoryMeta) interactive.StoryDirector {
	return LoadStoryDirectorForMeta(c.novaDir, meta)
}

func LoadStoryDirectorForMeta(novaDir string, meta interactive.StoryMeta) interactive.StoryDirector {
	director := loadStoryDirector(novaDir, meta.StoryDirectorID)
	if meta.ModuleRefs == nil {
		return director
	}
	director.ModuleRefs = interactive.NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	director.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	return interactive.ResolveStoryDirectorModules(novaDir, director)
}

func storyDirectorForSnapshot(director interactive.StoryDirector, snapshot *interactive.ActorStateSchemaSnapshot) interactive.StoryDirector {
	if snapshot == nil || len(snapshot.System.Templates) == 0 {
		return director
	}
	director.ActorState = snapshot.System
	if len(snapshot.TRPGSystem.RuleTemplates) > 0 {
		director.TRPGSystem = snapshot.TRPGSystem
	}
	return director
}

func LoadWritingTeller(novaDir, tellerID string) teller.Definition {
	return loadInteractiveTeller(novaDir, tellerID, style.ModeWriting)
}

func LoadGameTeller(novaDir, tellerID string) teller.Definition {
	return loadInteractiveTeller(novaDir, tellerID, style.ModeGame)
}

func loadInteractiveTeller(novaDir, tellerID, mode string) teller.Definition {
	if novaDir == "" {
		return teller.Definition{}
	}
	library := teller.NewLibrary(novaDir)
	selected, err := library.Get(tellerID)
	if err == nil && selected.SupportsMode(mode) {
		return selected
	}
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load narrative style failed id=%s mode=%s err=%v", tellerID, mode, err))
	} else {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-agent] narrative style is unavailable in mode id=%s mode=%s", tellerID, mode))
	}
	fallback, fallbackErr := library.Get(style.DefaultID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load default narrative style failed id=%s mode=%s err=%v", style.DefaultID, mode, fallbackErr))
		return teller.Definition{}
	}
	if !fallback.SupportsMode(mode) {
		slog.WarnContext(context.Background(), fmt.Sprintf("[interactive-agent] default narrative style is unavailable in mode id=%s mode=%s", fallback.ID, mode))
		return teller.Definition{}
	}
	return fallback
}

func loadStoryDirector(novaDir, directorID string) interactive.StoryDirector {
	if novaDir == "" {
		return interactive.DefaultStoryDirector()
	}
	director, err := interactive.NewStoryDirectorLibrary(novaDir).Get(directorID)
	if err == nil {
		return director
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load story director failed id=%s err=%v", directorID, err))
	fallback, fallbackErr := interactive.NewStoryDirectorLibrary(novaDir).Get(interactive.DefaultStoryDirectorID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load fallback story director failed err=%v", fallbackErr))
		return interactive.DefaultStoryDirector()
	}
	return fallback
}

func StoryTellerSystemInput(teller teller.Definition, styleRules ...[]prompts.StyleRule) prompts.InteractiveStorySystemInstructionInput {
	var rules []prompts.StyleRule
	if len(styleRules) > 0 {
		rules = styleRules[0]
	}
	return prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           teller.ID,
		StoryTellerName:         teller.Name,
		StoryTellerDescription:  teller.Description,
		StoryTellerSystemPrompt: teller.PromptForTargets("system"),
		StyleRules:              rules,
	}
}
