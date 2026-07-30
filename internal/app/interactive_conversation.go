package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/prompts"
)

type interactiveConversation struct {
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
	mu                       sync.Mutex
	lastTurn                 *interactive.TurnEvent
	lastStateReady           bool
	lastSources              string
	lastContextSources       []interactiveContextSource
	lastContextLedgerParts   []agents.ContextLedgerPart
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
	directorTasks            *workspaceDirectorTaskGroup
	directorGenerator        interactiveDirectorGenerator
	directorChatService      *agents.ChatService
	customDirectorGenerator  bool
	agentCycleCommit         func(context.Context, agents.RunOutcome) error
	agentCyclePrepare        func(context.Context) error
	agentCycleIdentity       agents.HarnessCycleIdentity
	pendingDomainCommit      *interactive.DomainCommitIntent
	lastDomainReceipt        *interactive.DomainCommitReceipt
	pendingCompaction        *preparedInteractiveContextCompaction
	pendingCompactionHealth  *preparedInteractiveContextCompactionHealth
	pendingCleanup           *preparedInteractiveToolResultCleanup
	modelHistoryKey          string
	modelHistory             *interactive.StoryModelHistory
	openingStateSchemaDraft  *interactive.ActorStateSchemaBatchDraft
	openingStateSchemaAudit  interactive.ActorStateSchemaBatchAudit
}

var _ agents.ExplicitSkillResolver = (*interactiveConversation)(nil)

func (c *interactiveConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return agents.ContextBudgetForAgent(c.cfg, config.AgentKindInteractiveStory)
}

func (c *interactiveConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]agents.ExplicitSkillInvocation, error) {
	if c == nil {
		return nil, nil
	}
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	return agents.ResolveExplicitSkillInvocations(ctx, cfg, config.AgentKindInteractiveStory, message)
}

// BindAgentCycleIdentity receives the coordinator-selected identity before
// the model cycle starts. The turn store persists it with the canonical game
// event, providing an idempotency key across the domain/runtime commit seam.
func (c *interactiveConversation) BindAgentCycleIdentity(identity agents.HarnessCycleIdentity) {
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
	c.pendingCompaction = nil
	c.pendingCompactionHealth = nil
	c.pendingCleanup = nil
	c.mu.Unlock()
}

func (c *interactiveConversation) agentCycleIdentitySnapshot() agents.HarnessCycleIdentity {
	if c == nil {
		return agents.HarnessCycleIdentity{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.agentCycleIdentity
}

func (c *interactiveConversation) withAgentCycleCommit(commit func(context.Context, agents.RunOutcome) error) *interactiveConversation {
	if c != nil {
		c.mu.Lock()
		c.agentCycleCommit = commit
		c.mu.Unlock()
	}
	return c
}

func (c *interactiveConversation) withAgentCyclePrepare(prepare func(context.Context) error) *interactiveConversation {
	if c != nil {
		c.mu.Lock()
		c.agentCyclePrepare = prepare
		c.mu.Unlock()
	}
	return c
}

// PrepareAgentCycle implements agents.HarnessCyclePreparer. The durable
// coordinator calls it after binding command/operation/cycle identity and
// before any model or tool effect, so queued follow-ups cannot observe a stale
// cross-domain projection.
func (c *interactiveConversation) PrepareAgentCycle(ctx context.Context) error {
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

// CommitAgentCycle implements agents.HarnessCycleCommitter. The callback is
// immutable once execution begins and bridges the generic durable cycle
// boundary to the game domain's persisted-turn projection.
func (c *interactiveConversation) CommitAgentCycle(ctx context.Context, outcome agents.RunOutcome) error {
	return c.CommitAgentCycleStage(ctx, agents.HarnessDomainCommitOutput, outcome)
}

func (c *interactiveConversation) PendingAgentCycleCommit(stage agents.HarnessDomainCommitStage) (agents.HarnessDomainCommitIntent, bool, error) {
	if c == nil {
		return agents.HarnessDomainCommitIntent{}, false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if stage != agents.HarnessDomainCommitOutput || c.pendingDomainCommit == nil {
		return agents.HarnessDomainCommitIntent{}, false, nil
	}
	return agents.HarnessDomainCommitIntent{
		Identity: c.agentCycleIdentity, Stage: stage, Hash: c.pendingDomainCommit.Hash,
	}, true, nil
}

func (c *interactiveConversation) CommitAgentCycleStage(ctx context.Context, stage agents.HarnessDomainCommitStage, outcome agents.RunOutcome) error {
	if c == nil || stage != agents.HarnessDomainCommitOutput {
		return nil
	}
	c.mu.Lock()
	pending := c.pendingDomainCommit
	commit := c.agentCycleCommit
	if outcome.Status != agents.RunOutcomeCompleted && outcome.Status != agents.RunOutcomePreempted {
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

func (c *interactiveConversation) LastAgentCycleCommitReceipt(stage agents.HarnessDomainCommitStage) (agents.HarnessDomainCommitReceipt, bool) {
	if c == nil || stage != agents.HarnessDomainCommitOutput {
		return agents.HarnessDomainCommitReceipt{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastDomainReceipt == nil {
		return agents.HarnessDomainCommitReceipt{}, false
	}
	return agents.HarnessDomainCommitReceipt{
		Identity: c.agentCycleIdentity, Stage: stage,
		Hash: c.lastDomainReceipt.Hash, Revision: c.lastDomainReceipt.Revision,
	}, true
}

type interactiveDirectorGenerator func(context.Context, *config.Config, *book.State, agents.InteractiveStoryToolContext, string) (string, error)

func newInteractiveConversation(store *interactive.Store, novaDir, workspace, storyID, branchID, user string, replyTargetChars int, cfg *config.Config) *interactiveConversation {
	return &interactiveConversation{store: store, novaDir: novaDir, workspace: workspace, cfg: cfg, storyID: storyID, branchID: branchID, user: user, replyTargetChars: replyTargetChars}
}

func (c *interactiveConversation) bindDirectorRuntime(tasks *workspaceDirectorTaskGroup, generator interactiveDirectorGenerator, chatServices ...*agents.ChatService) *interactiveConversation {
	if c != nil {
		c.directorTasks = tasks
		if len(chatServices) > 0 {
			c.directorChatService = chatServices[0]
		}
		if generator != nil {
			c.directorGenerator = generator
			c.customDirectorGenerator = true
		} else if c.directorChatService != nil {
			service := c.directorChatService
			c.directorGenerator = func(ctx context.Context, cfg *config.Config, state *book.State, toolContext agents.InteractiveStoryToolContext, instruction string) (string, error) {
				return agents.GenerateInteractiveDirectorWithTools(ctx, service, cfg, state, toolContext, instruction)
			}
			c.customDirectorGenerator = false
		}
	}
	return c
}

func (c *interactiveConversation) inheritDirectorRuntime(source *interactiveConversation) *interactiveConversation {
	if c == nil || source == nil {
		return c
	}
	c.directorTasks = source.directorTasks
	c.directorGenerator = source.directorGenerator
	c.directorChatService = source.directorChatService
	c.customDirectorGenerator = source.customDirectorGenerator
	return c
}

func (c *interactiveConversation) withDirectorTask(task string) *interactiveConversation {
	if c != nil {
		c.directorTask = strings.TrimSpace(task)
	}
	return c
}

func (c *interactiveConversation) withBaseParentID(parentID string) *interactiveConversation {
	if c != nil {
		parentID = strings.TrimSpace(parentID)
		c.mu.Lock()
		c.baseParentID = &parentID
		c.mu.Unlock()
	}
	return c
}

func (c *interactiveConversation) withRegenerateTarget(turnID string) *interactiveConversation {
	if c != nil {
		c.mu.Lock()
		c.replaceTurnID = strings.TrimSpace(turnID)
		c.mu.Unlock()
	}
	return c
}

func (c *interactiveConversation) regenerateTargetSnapshot() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.replaceTurnID
}

func (c *interactiveConversation) storyContextForCycle() (interactive.StoryContext, error) {
	if c == nil || c.store == nil {
		return interactive.StoryContext{}, fmt.Errorf("互动故事不存在")
	}
	if target := c.regenerateTargetSnapshot(); target != "" {
		return c.store.StoryContextAtTurnParent(c.storyID, c.branchID, target)
	}
	return c.store.StoryContext(c.storyID, c.branchID)
}

func (c *interactiveConversation) withExecutionParentPinning() *interactiveConversation {
	if c != nil {
		c.mu.Lock()
		c.pinParentAtExecution = true
		c.mu.Unlock()
	}
	return c
}

func (c *interactiveConversation) withOpeningStateSchema(storyCtx interactive.StoryContext) *interactiveConversation {
	if c == nil || !interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) || storyCtx.Meta.StateSchemaInitialization == nil || storyCtx.Meta.StateSchemaInitialization.Status != interactive.StateSchemaInitializationWaitingOpening || storyCtx.Meta.ActorStateSchema == nil || interactiveSnapshotTurnCount(storyCtx.Snapshot) > 0 {
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
	log.Printf("[interactive-agent] enabled opening state schema draft story_id=%s branch_id=%s mode=%s base_revision=%d", c.storyID, storyCtx.Snapshot.BranchID, storyCtx.Meta.StateSchemaPolicy.Mode, storyCtx.Meta.ActorStateSchema.Revision)
	return c
}

func (c *interactiveConversation) refreshOpeningStateSchema(storyCtx interactive.StoryContext) {
	if c == nil || (interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) &&
		storyCtx.Meta.StateSchemaInitialization != nil &&
		storyCtx.Meta.StateSchemaInitialization.Status == interactive.StateSchemaInitializationWaitingOpening &&
		interactiveSnapshotTurnCount(storyCtx.Snapshot) == 0) {
		return
	}
	c.mu.Lock()
	hadDraft := c.openingStateSchemaDraft != nil
	c.openingStateSchemaDraft = nil
	c.openingStateSchemaAudit = interactive.ActorStateSchemaBatchAudit{}
	c.mu.Unlock()
	if hadDraft {
		log.Printf("[interactive-agent] cleared stale opening state schema draft story_id=%s branch_id=%s turns=%d", c.storyID, storyCtx.Snapshot.BranchID, len(storyCtx.Snapshot.Turns))
	}
}

func (c *interactiveConversation) SubmitOpeningStateSchemaBatch(ctx context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
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
	log.Printf("[interactive-agent] staged opening state schema story_id=%s branch_id=%s accepted=%d rejected=%d blocked=%d finalized=%t draft_items=%d", c.storyID, c.branchID, len(result.Accepted), len(result.Rejected), len(result.Blocked), result.Finalized, result.DraftAcceptedItems)
	return result, nil
}

func (c *interactiveConversation) openingStateSchemaProposalSnapshot() *interactive.ActorStateSchemaProposal {
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

func (c *interactiveConversation) effectiveTurnState(storyCtx interactive.StoryContext) (interactive.StoryDirectorActorStateSystem, map[string]any, error) {
	actorState := interactive.StoryDirectorActorStateSystem{}
	if storyCtx.Meta.ActorStateSchema != nil {
		actorState = storyCtx.Meta.ActorStateSchema.System
	} else {
		actorState = c.storyDirectorForMeta(storyCtx.Meta).ActorState
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

func (c *interactiveConversation) directorTaskHint() string {
	if c == nil {
		return ""
	}
	switch strings.TrimSpace(c.directorTask) {
	case interactiveDirectorTaskOpeningPlan:
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
	contextLedgerParts   []agents.ContextLedgerPart
	stableLeadingMessage string
}

func (c *interactiveConversation) AssembleModelContext(ctx context.Context, originalMessage string, input agents.ModelContextInput) (agents.ModelContextResult, error) {
	_ = originalMessage
	if c == nil || c.store == nil {
		return agents.ModelContextResult{}, fmt.Errorf("互动故事不存在")
	}
	if err := ctx.Err(); err != nil {
		return agents.ModelContextResult{}, err
	}
	storyCtx, err := c.storyContextForCycle()
	if err != nil {
		return agents.ModelContextResult{}, err
	}
	branch, ok := storyCtx.Meta.Branches[storyCtx.Snapshot.BranchID]
	if !ok {
		return agents.ModelContextResult{}, fmt.Errorf("互动故事分支元数据不存在: %s", storyCtx.Snapshot.BranchID)
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
	storyDirector := storyDirectorForSnapshot(c.storyDirectorForMeta(storyCtx.Meta), storyCtx.Meta.ActorStateSchema)
	tellerTurnContextPrompt := teller.PromptForTargets("turn_context")
	modelHistory, activeCompaction, err := c.modelHistoryForCycle(storyCtx)
	if err != nil {
		return agents.ModelContextResult{}, err
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
		directorPlanVisible = interactive.DirectorPlanVisibleContext(directorPlan, interactiveStoryRuntimeContextBytes)
	}
	loreRuntime, err := buildInteractiveStoryLoreContext(c.workspace, directorPlan, input.UserMessage)
	if err != nil {
		return agents.ModelContextResult{}, err
	}
	loreStore := book.NewLoreStore(c.workspace)
	residentLore, err := loreStore.ResidentContextMarkdown()
	if err != nil {
		return agents.ModelContextResult{}, fmt.Errorf("读取常驻资料失败: %w", err)
	}
	residentContentBytes, err := loreStore.ResidentContentBytes()
	if err != nil {
		return agents.ModelContextResult{}, fmt.Errorf("读取常驻资料预算失败: %w", err)
	}
	if residentContentBytes > book.ResidentLoreSafetyMaxBytes {
		return agents.ModelContextResult{}, fmt.Errorf("常驻资料正文异常过大（%d KB）；请检查是否误将大型文件设为常驻资料", (residentContentBytes+1023)/1024)
	}
	if len([]byte(residentLore)) > interactiveResidentLoreMessageMaxBytes {
		return agents.ModelContextResult{}, fmt.Errorf("常驻资料模型上下文过大: %d > %d bytes", len([]byte(residentLore)), interactiveResidentLoreMessageMaxBytes)
	}
	loreRevision, err := loreStore.Revision()
	if err != nil {
		return agents.ModelContextResult{}, fmt.Errorf("读取资料库 revision 失败: %w", err)
	}
	ruleSummary := interactive.StoryDirectorRuleSummary(storyDirector, interactiveStoryRuntimeContextBytes)
	actorStateRuntime := interactive.ActorStateRuntimeContext(storyDirector.ActorState, storyCtx.Snapshot.State, interactiveStoryRuntimeContextBytes, storyCtx.Meta.ChoiceCount)
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
	cycleIdentity := c.agentCycleIdentitySnapshot()
	modelProjection, err := buildInteractiveModelContextProjection(
		modelHistory, activeCompaction, storyCtx.Snapshot, c.ToolResultContextPolicy(), cycleIdentity,
	)
	if err != nil {
		return agents.ModelContextResult{}, err
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
			Content: prompts.InteractiveStoryTurnContextRule(tellerTurnContextPrompt), Placement: agentcontext.PlacementFinalUserPrefix, Limit: interactiveStoryRuntimeContextBytes, Included: true,
		})
	}
	if strings.TrimSpace(runtimeContext) != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_runtime", Source: "interactive.runtime", Title: "本轮互动运行时上下文",
			Purpose: "provide bounded story state, active lore, actor state, and turn policy",
			Content: runtimeContext, Placement: agentcontext.PlacementFinalUserPrefix, Limit: interactiveStoryRuntimeContextBytes, Included: true,
		})
	}
	baseInstruction := prompts.InteractiveStoryTurnInstruction(input.UserMessage, "", "")
	history = append(history, agents.UserMessage(baseInstruction))
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{Messages: history, Fragments: fragments})
	if err != nil {
		return agents.ModelContextResult{}, err
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
	log.Printf(
		"[interactive-agent] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s story_director_id=%s teller_slots=%s teller_turn_context=%s history_checkpoint=%s director_plan=%s turns=%d model_turns=%d history=%s turn_instruction=%s sources=%s",
		c.storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		storyCtx.Meta.StoryDirectorID,
		interactiveTellerSlotSummary(teller, "turn_context"),
		interactivePartSummary(tellerTurnContextPrompt),
		interactivePartSummary(checkpointSummary),
		interactivePartSummary(directorPlanVisible),
		modelHistory.TotalTurns,
		len(turnHistory.Turns),
		interactiveMessageListSummary(history),
		interactivePartSummary(history[len(history)-1].Content),
		sourceSummary,
	)
	return agents.ModelContextResult{
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

func (c *interactiveConversation) CommitModelInput(ctx context.Context, _ string, assembled agents.ModelContextResult) error {
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
		c.withBaseParentID(*state.baseParentID)
	}
	c.refreshOpeningStateSchema(state.storyContext)
	c.mu.Lock()
	c.lastSources = state.sourceSummary
	c.lastContextSources = cloneInteractiveContextSources(state.contextSources)
	c.lastContextLedgerParts = append([]agents.ContextLedgerPart(nil), state.contextLedgerParts...)
	c.stableLeadingMessage = state.stableLeadingMessage
	c.mu.Unlock()
	return nil
}

func (c *interactiveConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSources
}

func (c *interactiveConversation) ContextLedgerParts() []agents.ContextLedgerPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]agents.ContextLedgerPart(nil), c.lastContextLedgerParts...)
}

func (c *interactiveConversation) ContextLedgerPartsForMessages(messages []*agents.Message) []agents.ContextLedgerPart {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	sources := cloneInteractiveContextSources(c.lastContextSources)
	c.mu.Unlock()
	parts := interactiveContextLedgerParts(sources, messages, c.ToolResultContextPolicy())
	c.mu.Lock()
	c.lastContextLedgerParts = append([]agents.ContextLedgerPart(nil), parts...)
	c.mu.Unlock()
	return parts
}

func (c *interactiveConversation) RunTraceMetadata() agents.RunTraceMetadata {
	if c == nil {
		return agents.RunTraceMetadata{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	metadata := agents.RunTraceMetadata{
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
