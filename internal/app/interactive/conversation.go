package interactiveapp

import (
	"context"
	agentexecution "denova/internal/agents/execution"
	agentinteractive "denova/internal/agents/interactive"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	"denova/internal/book"
	"denova/internal/book/lore"
	"denova/internal/interactive"
)

type Conversation struct {
	store                    *interactive.Store
	novaDir                  string
	workspace                string
	cfg                      *config.Config
	storyID                  string
	branchID                 string
	user                     string
	replyTargetChars         int
	directorTask             string
	modelContextAppendMu     sync.Mutex
	turnCheckMu              sync.Mutex
	mu                       sync.Mutex
	lastTurn                 *interactive.TurnEvent
	lastStateReady           bool
	lastSources              string
	lastContextSources       []interactiveContextSource
	lastContextLedgerParts   []agentcontext.AuditPart
	stableLeadingMessage     string
	assistantMetadata        session.MessageMetadata
	displayEvents            []interactive.DisplayEvent
	modelContextMessages     []interactive.ModelContextMessage
	modelContextBatchOrdinal int
	ruleResolution           *interactive.RuleResolution
	turnProtocol             interactiveTurnProtocol
	baseParentID             *string
	replaceTurnID            string
	pinParentAtExecution     bool
	directorTasks            *DirectorTaskGroup
	directorGenerator        DirectorGenerator
	directorExecutionRuntime *agentexecution.Runtime
	customDirectorGenerator  bool
	agentCycleCommit         func(context.Context, agentrun.Outcome) error
	agentCyclePrepare        func(context.Context) error
	agentCycleIdentity       agentrun.CycleIdentity
	pendingDomainCommit      *interactive.DomainCommitIntent
	lastDomainReceipt        *interactive.DomainCommitReceipt
	agentCompaction          *interactive.ContextCompactionProjection
	modelHistoryKey          string
	modelHistory             *interactive.StoryModelHistory
	openingStateSchemaDraft  *interactive.ActorStateSchemaBatchDraft
	openingStateSchemaAudit  interactive.ActorStateSchemaBatchAudit
}

var _ novaskills.ExplicitResolver = (*Conversation)(nil)

func (c *Conversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return agentcontext.ContextBudgetForAgent(c.cfg, config.AgentKindInteractiveStory)
}

func (c *Conversation) ResolveExplicitSkills(ctx context.Context, message string) ([]novaskills.Invocation, error) {
	if c == nil {
		return nil, nil
	}
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	return novaskills.ResolveConfiguredInvocations(ctx, cfg, config.AgentKindInteractiveStory, message)
}

// BindAgentCycleIdentity receives the coordinator-selected identity before
// the model cycle starts. The turn store persists it with the canonical game
// event, providing an idempotency key across the domain/runtime commit seam.
func (c *Conversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c == nil {
		return
	}
	c.modelContextAppendMu.Lock()
	defer c.modelContextAppendMu.Unlock()
	c.mu.Lock()
	c.agentCycleIdentity = identity
	c.modelContextMessages = nil
	c.modelContextBatchOrdinal = 0
	c.pendingDomainCommit = nil
	c.lastDomainReceipt = nil
	c.mu.Unlock()
}

func (c *Conversation) AgentCycleIdentitySnapshot() agentrun.CycleIdentity {
	if c == nil {
		return agentrun.CycleIdentity{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentCycleIdentity
}

func (c *Conversation) WithAgentCycleCommit(commit func(context.Context, agentrun.Outcome) error) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.agentCycleCommit = commit
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) WithAgentCyclePrepare(prepare func(context.Context) error) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.agentCyclePrepare = prepare
		c.mu.Unlock()
	}
	return c
}

// PrepareAgentCycle implements agentrun.CyclePreparer. The durable
// coordinator calls it after binding command/operation/cycle identity and
// before any model or tool effect, so queued follow-ups cannot observe a stale
// cross-domain projection.
func (c *Conversation) PrepareAgentCycle(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	prepare := c.agentCyclePrepare
	c.mu.Unlock()
	if prepare == nil {
		return nil
	}
	return prepare(ctx)
}

// CommitAgentCycle implements agentrun.CycleCommitter. The callback is
// immutable once execution begins and bridges the generic durable cycle
// boundary to the game domain's persisted-turn projection.
func (c *Conversation) CommitAgentCycle(ctx context.Context, outcome agentrun.Outcome) error {
	return c.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, outcome)
}

func (c *Conversation) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if c == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if stage != agentrun.DomainCommitOutput || c.pendingDomainCommit == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	return agentrun.DomainCommitIntent{
		Identity: c.agentCycleIdentity, Stage: stage, Hash: c.pendingDomainCommit.Hash,
	}, true, nil
}

func (c *Conversation) CommitAgentCycleStage(ctx context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return nil
	}
	c.mu.Lock()
	pending := c.pendingDomainCommit
	commit := c.agentCycleCommit
	if outcome.Status != agentrun.OutcomeCompleted && outcome.Status != agentrun.OutcomePreempted {
		c.pendingDomainCommit = nil
		c.lastDomainReceipt = nil
		c.mu.Unlock()
		if commit != nil {
			return commit(ctx, outcome)
		}
		return nil
	}
	c.mu.Unlock()
	if pending == nil {
		if commit != nil {
			return commit(ctx, outcome)
		}
		return nil
	}
	receipt, err := c.store.CommitDomainTurn(c.storyID, *pending)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.pendingDomainCommit = nil
	c.lastDomainReceipt = &receipt
	turn := receipt.Turn
	c.lastTurn = &turn
	c.lastStateReady = turn.StateStatus == "ready"
	c.turnProtocol.markCommitted()
	c.mu.Unlock()
	if commit != nil {
		return commit(ctx, outcome)
	}
	return nil
}

func (c *Conversation) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitReceipt{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastDomainReceipt == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	return agentrun.DomainCommitReceipt{
		Identity: c.agentCycleIdentity, Stage: stage,
		Hash: c.lastDomainReceipt.Hash, Revision: c.lastDomainReceipt.Revision,
	}, true
}

type DirectorGenerator func(context.Context, *config.Config, *book.State, agentinteractive.InteractiveStoryToolContext, string) (string, error)

func NewConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int, cfg *config.Config) *Conversation {
	return &Conversation{store: store, novaDir: novaDir, workspace: workspace, cfg: cfg, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars}
}

func (c *Conversation) BindDirectorRuntime(tasks *DirectorTaskGroup, generator DirectorGenerator, executionRuntimes ...*agentexecution.Runtime) *Conversation {
	if c != nil {
		c.directorTasks = tasks
		if len(executionRuntimes) > 0 {
			c.directorExecutionRuntime = executionRuntimes[0]
		}
		if generator != nil {
			c.directorGenerator = generator
			c.customDirectorGenerator = true
		} else if c.directorExecutionRuntime != nil {
			service := c.directorExecutionRuntime
			c.directorGenerator = func(ctx context.Context, cfg *config.Config, state *book.State, toolContext agentinteractive.InteractiveStoryToolContext, instruction string) (string, error) {
				return agents.GenerateInteractiveDirectorWithTools(ctx, service, cfg, state, toolContext, instruction)
			}
			c.customDirectorGenerator = false
		}
	}
	return c
}

// DirectorTasks returns the workspace-generation owner for background Director
// projection work. A nil result means the conversation is not runtime-bound.
func (c *Conversation) DirectorTasks() *DirectorTaskGroup {
	if c == nil {
		return nil
	}
	return c.directorTasks
}

func (c *Conversation) InheritDirectorRuntime(source *Conversation) *Conversation {
	if c == nil || source == nil {
		return c
	}
	c.directorTasks = source.directorTasks
	c.directorGenerator = source.directorGenerator
	c.directorExecutionRuntime = source.directorExecutionRuntime
	c.customDirectorGenerator = source.customDirectorGenerator
	return c
}

func (c *Conversation) WithDirectorTask(task string) *Conversation {
	if c != nil {
		c.directorTask = strings.TrimSpace(task)
	}
	return c
}

func (c *Conversation) WithBaseParentID(parentID string) *Conversation {
	if c != nil {
		parentID = strings.TrimSpace(parentID)
		c.mu.Lock()
		c.baseParentID = &parentID
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) WithRegenerateTarget(turnID string) *Conversation {
	if c != nil {
		c.mu.Lock()
		c.replaceTurnID = strings.TrimSpace(turnID)
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) regenerateTargetSnapshot() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.replaceTurnID
}

func (c *Conversation) storyContextForCycle() (interactive.StoryContext, error) {
	if c == nil || c.store == nil {
		return interactive.StoryContext{}, fmt.Errorf("互动故事不存在")
	}
	if target := c.regenerateTargetSnapshot(); target != "" {
		return c.store.StoryContextAtTurnParent(c.storyID, c.branchID, target)
	}
	return c.store.StoryContext(c.storyID, c.branchID)
}

func (c *Conversation) WithExecutionParentPinning() *Conversation {
	if c != nil {
		c.mu.Lock()
		c.pinParentAtExecution = true
		c.mu.Unlock()
	}
	return c
}

func (c *Conversation) WithOpeningStateSchema(storyCtx interactive.StoryContext) *Conversation {
	if c == nil || !interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) || storyCtx.Meta.StateSchemaInitialization == nil || storyCtx.Meta.StateSchemaInitialization.Status != interactive.StateSchemaInitializationWaitingOpening || storyCtx.Meta.ActorStateSchema == nil || SnapshotTurnCount(storyCtx.Snapshot) > 0 {
		return c
	}
	trpgSourceIDs := make([]string, 0, len(storyCtx.Meta.ActorStateSchema.TRPGSystem.RuleTemplates))
	for _, rule := range storyCtx.Meta.ActorStateSchema.TRPGSystem.RuleTemplates {
		if id := strings.TrimSpace(rule.ID); id != "" {
			trpgSourceIDs = append(trpgSourceIDs, id)
		}
	}
	c.mu.Lock()
	c.openingStateSchemaDraft = interactive.NewOpeningActorStateSchemaBatchDraft(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem)
	c.openingStateSchemaAudit = interactive.ActorStateSchemaBatchAudit{
		OpeningSourceIDs: []string{"opening-draft"},
		TRPGSourceIDs:    trpgSourceIDs,
		CurrentState:     storyCtx.Snapshot.State,
	}
	c.mu.Unlock()
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] enabled opening state schema draft story_id=%s branch_id=%s mode=%s base_revision=%d", c.storyID, storyCtx.Snapshot.BranchID, storyCtx.Meta.StateSchemaPolicy.Mode, storyCtx.Meta.ActorStateSchema.Revision))
	return c
}

func (c *Conversation) refreshOpeningStateSchema(storyCtx interactive.StoryContext) {
	if c == nil || (interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) &&
		storyCtx.Meta.StateSchemaInitialization != nil &&
		storyCtx.Meta.StateSchemaInitialization.Status == interactive.StateSchemaInitializationWaitingOpening &&
		SnapshotTurnCount(storyCtx.Snapshot) == 0) {
		return
	}
	c.mu.Lock()
	hadDraft := c.openingStateSchemaDraft != nil
	c.openingStateSchemaDraft = nil
	c.openingStateSchemaAudit = interactive.ActorStateSchemaBatchAudit{}
	c.mu.Unlock()
	if hadDraft {
		slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-agent] cleared stale opening state schema draft story_id=%s branch_id=%s turns=%d", c.storyID, storyCtx.Snapshot.BranchID, len(storyCtx.Snapshot.Turns)))
	}
}

func (c *Conversation) SubmitOpeningStateSchemaBatch(ctx context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
	if c == nil {
		return interactive.ActorStateSchemaBatchResult{}, fmt.Errorf("互动故事不存在")
	}
	select {
	case <-ctx.Done():
		return interactive.ActorStateSchemaBatchResult{}, ctx.Err()
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.openingStateSchemaDraft == nil {
		return interactive.ActorStateSchemaBatchResult{}, fmt.Errorf("当前故事不需要 Game Agent 初始化状态结构")
	}
	result := c.openingStateSchemaDraft.SubmitStructureOnly(batch, c.openingStateSchemaAudit)
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent] staged opening state schema story_id=%s branch_id=%s accepted=%d rejected=%d blocked=%d finalized=%t draft_items=%d", c.storyID, c.branchID, len(result.Accepted), len(result.Rejected), len(result.Blocked), result.Finalized, result.DraftAcceptedItems))
	return result, nil
}

func (c *Conversation) openingStateSchemaProposalSnapshot() *interactive.ActorStateSchemaProposal {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	proposal, ok := c.openingStateSchemaDraft.FinalProposal()
	if !ok {
		return nil
	}
	return &proposal
}

func (c *Conversation) effectiveTurnState(storyCtx interactive.StoryContext) (interactive.StoryDirectorActorStateSystem, map[string]any, error) {
	actorState := interactive.StoryDirectorActorStateSystem{}
	if storyCtx.Meta.ActorStateSchema != nil {
		actorState = storyCtx.Meta.ActorStateSchema.System
	} else {
		actorState = c.StoryDirectorForMeta(storyCtx.Meta).ActorState
	}
	state := storyCtx.Snapshot.State
	c.mu.Lock()
	proposal, hasProposal := c.openingStateSchemaDraft.FinalProposal()
	draftRequired := c.openingStateSchemaDraft != nil
	c.mu.Unlock()
	if draftRequired && !hasProposal {
		return interactive.StoryDirectorActorStateSystem{}, nil, fmt.Errorf("请先调用 initialize_story_state_schema 并完成 finalize，再提交开局状态")
	}
	if hasProposal {
		if storyCtx.Meta.ActorStateSchema == nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, fmt.Errorf("故事缺少开局状态结构基线")
		}
		target, _, err := interactive.ApplyActorStateSchemaAdaptation(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem, proposal.Adaptation)
		if err != nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, err
		}
		initialState, err := interactive.BuildActorStateInitialSnapshot(target, storyCtx.Meta.InitialTraitRolls)
		if err != nil {
			return interactive.StoryDirectorActorStateSystem{}, nil, err
		}
		return target, initialState, nil
	}
	if interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) {
		rawActors, _ := state["actors"].(map[string]any)
		if len(rawActors) == 0 {
			initialState, err := interactive.BuildActorStateInitialSnapshot(actorState, storyCtx.Meta.InitialTraitRolls)
			if err != nil {
				return interactive.StoryDirectorActorStateSystem{}, nil, err
			}
			state = initialState
		}
	}
	return actorState, state, nil
}

func (c *Conversation) directorTaskHint() string {
	if c == nil {
		return ""
	}
	switch strings.TrimSpace(c.directorTask) {
	case DirectorTaskOpeningPlan:
		return "opening_plan：在首个 Game Agent 回合前建立 director.md、agent-brief.md 与 lore-context.md；基于开局设定和资料名称目录完成初始选角、场景与分支规划。"
	case "director_plan_update":
		return "director_plan_update：Game Agent 已提示本回合对后续规划有实质影响；判断 keep、patch 或 replan。普通更新默认只 Patch agent-brief.md，只有重大偏差才修改 director.md，只有资料工作集变化才修改 lore-context.md。"
	default:
		return "director_plan_update：观察已提交事实并判断 keep、patch 或 replan；只 Patch 实际变化的导演 Markdown 文件，不得改写历史 Turn 或 Actor State。"
	}
}

type interactiveModelContextCommitState struct {
	storyContext         interactive.StoryContext
	baseParentID         *string
	sourceSummary        string
	contextSources       []interactiveContextSource
	contextLedgerParts   []agentcontext.AuditPart
	stableLeadingMessage string
}

// StableLeadingMessage returns the stable prefix captured by AssembleModelContext.
// Callers use it only when validating a manually prepared compaction candidate.
func StableLeadingMessage(commitState any) (string, bool) {
	state, ok := commitState.(interactiveModelContextCommitState)
	if !ok {
		return "", false
	}
	return state.stableLeadingMessage, true
}

func (c *Conversation) AssembleModelContext(ctx context.Context, originalMessage string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	_ = originalMessage
	if c == nil || c.store == nil {
		return agentcontext.ModelContextResult{}, fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	branch, ok := storyCtx.Meta.Branches[storyCtx.Snapshot.BranchID]
	if !ok {
		return agentcontext.ModelContextResult{}, fmt.Errorf("互动故事分支元数据不存在: %s", storyCtx.Snapshot.BranchID)
	}
	// Commands may wait behind an active cycle. Pin the compare-and-swap parent
	// at actual model-context assembly time, not HTTP acceptance time, so a
	// queued FollowUp sees the turn committed by the preceding cycle while still
	// rejecting unrelated branch writes that race the model run.
	c.mu.Lock()
	pinParent := c.pinParentAtExecution
	c.mu.Unlock()
	var baseParentID *string
	if pinParent && c.regenerateTargetSnapshot() == "" {
		parent := strings.TrimSpace(branch.Head)
		baseParentID = &parent
	}
	teller := c.teller(storyCtx.Meta.StoryTellerID)
	storyDirector := storyDirectorForSnapshot(c.StoryDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	modelHistory, activeCompaction, err := c.modelHistoryForCycle(storyCtx)
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	turnHistory := buildInteractiveModelVisibleHistory(modelHistory, activeCompaction)
	checkpointSummary := ""
	if activeCompaction != nil {
		checkpointSummary = strings.TrimSpace(activeCompaction.Summary)
	}
	directorPlanVisible := ""
	directorPlan := interactive.DirectorPlan{}
	if storyCtx.Snapshot.DirectorPlan != nil {
		directorPlan = *storyCtx.Snapshot.DirectorPlan
		directorPlanVisible = interactive.DirectorPlanVisibleContext(directorPlan, StoryRuntimeContextMaxBytes)
	}
	loreRuntime, err := buildInteractiveStoryLoreContext(c.workspace, directorPlan, input.UserMessage)
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	loreStore := lore.NewStore(c.workspace)
	residentLore, err := loreStore.ResidentContextMarkdown()
	if err != nil {
		return agentcontext.ModelContextResult{}, fmt.Errorf("读取常驻资料失败: %w", err)
	}
	residentContentBytes, err := loreStore.ResidentContentBytes()
	if err != nil {
		return agentcontext.ModelContextResult{}, fmt.Errorf("读取常驻资料预算失败: %w", err)
	}
	if residentContentBytes > lore.ResidentLoreSafetyMaxBytes {
		return agentcontext.ModelContextResult{}, fmt.Errorf("常驻资料正文异常过大（%d KB）；请检查是否误将大型文件设为常驻资料", (residentContentBytes+1023)/1024)
	}
	if len([]byte(residentLore)) > interactiveResidentLoreMessageMaxBytes {
		return agentcontext.ModelContextResult{}, fmt.Errorf("常驻资料模型上下文过大: %d > %d bytes", len([]byte(residentLore)), interactiveResidentLoreMessageMaxBytes)
	}
	loreRevision, err := loreStore.Revision()
	if err != nil {
		return agentcontext.ModelContextResult{}, fmt.Errorf("读取资料库 revision 失败: %w", err)
	}
	ruleSummary := interactive.StoryDirectorRuleSummary(storyDirector, StoryRuntimeContextMaxBytes)
	actorStateRuntime := interactive.ActorStateRuntimeContext(storyDirector.ActorState, storyCtx.Snapshot.State, StoryRuntimeContextMaxBytes, storyCtx.Meta.ChoiceCount)
	stateSchemaInitialization := interactive.OpeningGameStateSchemaInstruction(storyCtx.Meta)
	strategyPrompt := interactive.StoryDirectorStrategyPromptMarkdown(storyDirector)
	runtimeContext := prompts.InteractiveStoryRuntimeContext(prompts.InteractiveStoryPromptInput{
		Title:                       storyCtx.Meta.Title,
		Origin:                      storyCtx.Meta.Origin,
		StoryTellerID:               storyCtx.Meta.StoryTellerID,
		StoryDirectorID:             storyCtx.Meta.StoryDirectorID,
		BranchID:                    storyCtx.Snapshot.BranchID,
		ReplyTargetChars:            c.replyTargetChars,
		ChoiceCount:                 storyCtx.Meta.ChoiceCount,
		DirectorPlanVisible:         directorPlanVisible,
		StoryDirectorRules:          ruleSummary,
		ActorState:                  actorStateRuntime,
		StateSchemaInitialization:   stateSchemaInitialization,
		StoryDirectorStrategyPrompt: strategyPrompt,
		PreviousTurnsSummary:        turnHistory.PreviousSummary,
		LoreContext:                 loreRuntime,
	})
	cycleIdentity := c.AgentCycleIdentitySnapshot()
	modelProjection, err := BuildModelContextProjection(
		modelHistory, activeCompaction, storyCtx.Snapshot, c.ToolResultContextPolicy(), cycleIdentity,
	)
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	history := modelProjection.Messages
	pendingInputMessages := modelProjection.PendingInputMessages
	fragments := append([]agentcontext.Fragment(nil), input.Fragments...)
	if strings.TrimSpace(residentLore) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_resident_lore", Source: "interactive.resident_lore", Title: "常驻资料库",
			Purpose: "provide complete enabled resident lore as a stable model prefix",
			Content: residentLore, Placement: agentcontext.PlacementLeadingMessage, Limit: interactiveResidentLoreMessageMaxBytes, Included: true,
			Note: "source=enabled resident lore; stable leading context; revision=" + strings.TrimSpace(loreRevision),
		})
	}
	if strings.TrimSpace(tellerTurnContextPrompt) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_turn_rules", Source: "interactive.turn_rules", Title: "导演本轮上下文规则",
			Purpose: "apply the selected storyteller rules to this game turn",
			Content: prompts.InteractiveStoryTurnContextRule(tellerTurnContextPrompt), Placement: agentcontext.PlacementFinalUserPrefix, Limit: StoryRuntimeContextMaxBytes, Included: true,
		})
	}
	if strings.TrimSpace(runtimeContext) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_runtime", Source: "interactive.runtime", Title: "本轮互动运行时上下文",
			Purpose: "provide bounded story state, active lore, actor state, and turn policy",
			Content: runtimeContext, Placement: agentcontext.PlacementFinalUserPrefix, Limit: StoryRuntimeContextMaxBytes, Included: true,
		})
	}
	baseInstruction := prompts.InteractiveStoryTurnInstruction(input.UserMessage, "", "")
	history = append(history, agents.UserMessage(baseInstruction))
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{Messages: history, Fragments: fragments})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	history = assembled.Messages
	stableLeadingMessage := ""
	residentVisible := residentLore
	for _, fragment := range assembled.Fragments {
		if fragment.Source == "interactive.resident_lore" {
			residentVisible = fragment.Content
			if fragment.Included && fragment.Content != "" {
				stableLeadingMessage = agentcontext.StandaloneMessage(fragment.Title, fragment.Content, "")
			}
			break
		}
	}
	sourceParts := interactiveStoryContextSources(storyCtx.Meta.Title, storyCtx.Meta.Origin, teller, checkpointSummary, directorPlanVisible, residentVisible, loreRevision, loreRuntime, ruleSummary, actorStateRuntime, stateSchemaInitialization, strategyPrompt, turnHistory, input.UserMessage)
	for index, message := range pendingInputMessages {
		sourceParts = append(sourceParts, interactiveContextSource{
			Source: "InterruptedPlayerInput", Title: fmt.Sprintf("已接收但未产出剧情的玩家输入 %d", index+1),
			Purpose: "retain accepted player intent after an interrupted cycle",
			Content: message, ExactMessage: true,
		})
	}
	sourceParts = resolveInteractiveContextSources(sourceParts, history)
	sourceSummary := interactiveContextSourceListSummary(sourceParts, assembled.Fragments)
	contextLedgerParts := interactiveContextLedgerParts(sourceParts, history, c.ToolResultContextPolicy())
	slog.InfoContext(ctx, fmt.Sprintf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s story_director_id=%s teller_slots=%s teller_turn_context=%s history_checkpoint=%s director_plan=%s turns=%d model_turns=%d history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		PartSummary(storyCtx.Meta.Title),
		PartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		PartSummary(tellerTurnContextPrompt),
		PartSummary(checkpointSummary),
		PartSummary(directorPlanVisible),
		modelHistory.TotalTurns,
		len(turnHistory.Turns),
		interactiveMessageListSummary(history),
		PartSummary(history[len(history)-1].Content),
		sourceSummary,
	))
	return agentcontext.ModelContextResult{
		Messages: history,
		Context:  assembled,
		CommitState: interactiveModelContextCommitState{
			storyContext: storyCtx, baseParentID: baseParentID, sourceSummary: sourceSummary,
			contextSources: cloneInteractiveContextSources(sourceParts), contextLedgerParts: contextLedgerParts,
			stableLeadingMessage: stableLeadingMessage,
		},
	}, nil
}

func interruptedPlayerInputModelMessage(input interactive.PlayerInputAcceptedEvent) string {
	return "[Accepted player input from an interrupted turn / 已接收但中断的玩家输入]\n" +
		"No narrative was produced for this input. Treat it as player intent, not as a completed Turn. / 此输入尚未产出剧情，只代表玩家意图，不是已完成回合。\n\n" +
		strings.TrimSpace(input.Text)
}

func (c *Conversation) CommitModelInput(ctx context.Context, _ string, assembled agentcontext.ModelContextResult) error {
	if c == nil {
		return fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok := assembled.CommitState.(interactiveModelContextCommitState)
	if !ok {
		return fmt.Errorf("互动故事模型上下文缺少提交状态")
	}
	if state.baseParentID != nil {
		c.WithBaseParentID(*state.baseParentID)
	}
	c.refreshOpeningStateSchema(state.storyContext)
	c.mu.Lock()
	c.lastSources = state.sourceSummary
	c.lastContextSources = cloneInteractiveContextSources(state.contextSources)
	c.lastContextLedgerParts = append([]agentcontext.AuditPart(nil), state.contextLedgerParts...)
	c.stableLeadingMessage = state.stableLeadingMessage
	c.mu.Unlock()
	return nil
}

// MaterializeAgentCanonicalInput records the accepted player input before
// model-context assembly. The matching live cycle is excluded from interrupted
// input projection when its context is assembled immediately afterwards.
func (c *Conversation) MaterializeAgentCanonicalInput(
	ctx context.Context,
	agentCanonicalHash string,
) (interactive.PlayerInputReceipt, error) {
	if c == nil || c.store == nil {
		return interactive.PlayerInputReceipt{}, fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	identity := c.AgentCycleIdentitySnapshot()
	if !agentrun.ValidCycleIdentity(identity) {
		return interactive.PlayerInputReceipt{}, fmt.Errorf("canonical game input requires an exact Agent cycle identity")
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
	}, c.branchID, c.user)
	if err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	intent, err = intent.WithAgentCanonicalHash(agentCanonicalHash)
	if err != nil {
		return interactive.PlayerInputReceipt{}, err
	}
	return c.store.CommitPlayerInput(c.storyID, intent)
}

func (c *Conversation) FindRecentAgentCanonicalInput(
	identity interactive.DomainCommitIdentity,
	hash string,
) (interactive.PlayerInputReceipt, bool, error) {
	if c == nil || c.store == nil {
		return interactive.PlayerInputReceipt{}, false, fmt.Errorf("互动故事不存在")
	}
	return c.store.FindRecentAgentCanonicalPlayerInputCommit(c.storyID, c.branchID, identity, hash)
}

func (c *Conversation) FindRecentAgentCanonicalOutput(
	identity interactive.DomainCommitIdentity,
	hash string,
) (interactive.DomainCommitReceipt, bool, error) {
	if c == nil || c.store == nil {
		return interactive.DomainCommitReceipt{}, false, fmt.Errorf("互动故事不存在")
	}
	return c.store.FindRecentAgentCanonicalDomainTurnCommit(c.storyID, c.branchID, identity, hash)
}

func (c *Conversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSources
}

func (c *Conversation) ContextLedgerParts() []agentcontext.AuditPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]agentcontext.AuditPart(nil), c.lastContextLedgerParts...)
}

func (c *Conversation) ContextLedgerPartsForMessages(messages []*agents.Message) []agentcontext.AuditPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	sources := cloneInteractiveContextSources(c.lastContextSources)
	c.mu.Unlock()
	parts := interactiveContextLedgerParts(sources, messages, c.ToolResultContextPolicy())
	c.mu.Lock()
	c.lastContextLedgerParts = append([]agentcontext.AuditPart(nil), parts...)
	c.mu.Unlock()
	return parts
}

func (c *Conversation) RunTraceMetadata() agentrun.TraceMetadata {
	if c == nil {
		return agentrun.TraceMetadata{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := agentrun.TraceMetadata{
		StoryID:         c.storyID,
		BranchID:        c.branchID,
		MaintenanceTask: c.directorTask,
	}
	if c.lastTurn != nil {
		metadata.BranchID = c.lastTurn.BranchID
		metadata.TurnID = c.lastTurn.ID
	}
	return metadata
}
