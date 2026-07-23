package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	adk "github.com/alfredxw/denova/adk"

	"denova/config"
	agentcontext "denova/internal/agent/context"
	runstate "denova/internal/agent/runtime"
	"denova/internal/agent/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

const interactiveDirectorAgentLabel = "interactive-director-agent"
const interactiveDirectorToolResultMaxBytes = interactive.DirectorContextMaxBytes

const (
	directorPlanHiddenNotice = "chapter_body_hidden"
	directorPlanHiddenReason = "director_plan_body"
	directorPlanProgressStep = 100
)

func GenerateInteractiveDirectorWithTools(ctx context.Context, chatService *ChatService, cfg *config.Config, state *book.State, toolContext InteractiveStoryToolContext, instruction string) (string, error) {
	if chatService == nil || chatService.harness == nil {
		return "", fmt.Errorf("互动导演运行时不可用")
	}
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	if state == nil {
		return "", fmt.Errorf("互动导演故事状态不存在")
	}
	toolContext.CommandID = strings.TrimSpace(toolContext.CommandID)
	if err := runstate.ValidateCommandID(toolContext.CommandID, runstate.DefaultInputLimits()); err != nil {
		return "", fmt.Errorf("互动导演 command_id 无效: %w", err)
	}
	builtAgent, systemPrompt, err := BuildInteractiveDirectorWithComposition(ctx, cfg, state, toolContext)
	if err != nil {
		return "", fmt.Errorf("构建互动导演 Agent 失败: %w", err)
	}
	runOptions := RunOptions{
		AgentKind:       config.AgentKindInteractiveDirector,
		StoryID:         toolContext.StoryID,
		BranchID:        toolContext.BranchID,
		TurnID:          toolContext.TurnID,
		MaintenanceTask: toolContext.MaintenanceTask,
		Workspace:       cfg.Workspace,
		SystemPromptLog: systemPrompt,
	}
	runner := NewRunnerWithOptions(ctx, builtAgent, runOptions)
	conversation := &singleInstructionConversation{
		instruction:           instruction,
		stableContextTitle:    toolContext.StableContextTitle,
		stableContext:         toolContext.StableContext,
		stableContextMaxBytes: toolContext.StableContextMaxBytes,
		contextBudget:         contextBudgetForAgent(cfg, config.AgentKindInteractiveDirector),
		display:               toolContext.DisplayConversation,
		domainCommit:          toolContext.DomainCommitParticipant,
		hideDirectorToolInput: cfg.HideChapterBodyLiveOutput,
		directorTools:         map[string]*directorToolDisplayState{},
	}
	bookService := book.NewService(state.Workspace())
	var runErr error
	runOptions.ToolResultMaxBytes = interactiveDirectorToolResultMaxBytes
	outcome := chatService.RunWithOptions(ctx, runner, conversation, bookService, ChatRequest{CommandID: toolContext.CommandID, Message: instruction}, runOptions, func(event Event) {
		if event.Type != "error" {
			return
		}
		if data, ok := event.Data.(map[string]string); ok {
			runErr = fmt.Errorf("%s", data["message"])
		}
	})
	if outcome.Status == RunOutcomeFailed && outcome.Error != nil {
		runErr = outcome.Error
	}
	if runErr != nil {
		return "", runErr
	}
	output := strings.TrimSpace(conversation.output)
	if output == "" {
		output = strings.TrimSpace(outcome.Content)
	}
	if output == "" && !isInteractiveDirectorPlanTask(toolContext.MaintenanceTask) {
		return "", fmt.Errorf("互动导演 Agent 返回为空")
	}
	return output, nil
}

type singleInstructionConversation struct {
	instruction           string
	stableContextTitle    string
	stableContext         string
	stableContextMaxBytes int
	contextBudget         agentcontext.Budget
	lastContext           agentcontext.Result
	output                string
	display               Conversation
	domainCommit          HarnessDomainCommitParticipant
	hideDirectorToolInput bool
	mu                    sync.Mutex
	directorTools         map[string]*directorToolDisplayState
}

// BindAgentCycleIdentity forwards the durable coordinator identity to the
// App-owned Director commit participant. The model conversation itself never
// invents a domain identity.
func (c *singleInstructionConversation) BindAgentCycleIdentity(identity HarnessCycleIdentity) {
	if c == nil || c.domainCommit == nil {
		return
	}
	if binder, ok := c.domainCommit.(HarnessCycleIdentityBinder); ok {
		binder.BindAgentCycleIdentity(identity)
	}
}

func (c *singleInstructionConversation) PendingAgentCycleCommit(stage HarnessDomainCommitStage) (HarnessDomainCommitIntent, bool, error) {
	if c == nil || c.domainCommit == nil {
		return HarnessDomainCommitIntent{}, false, nil
	}
	return c.domainCommit.PendingAgentCycleCommit(stage)
}

func (c *singleInstructionConversation) CommitAgentCycleStage(ctx context.Context, stage HarnessDomainCommitStage, outcome RunOutcome) error {
	if c == nil || c.domainCommit == nil {
		return nil
	}
	return c.domainCommit.CommitAgentCycleStage(ctx, stage, outcome)
}

func (c *singleInstructionConversation) LastAgentCycleCommitReceipt(stage HarnessDomainCommitStage) (HarnessDomainCommitReceipt, bool) {
	if c == nil || c.domainCommit == nil {
		return HarnessDomainCommitReceipt{}, false
	}
	return c.domainCommit.LastAgentCycleCommitReceipt(stage)
}

const maxSingleInstructionStableContextTitleBytes = 512

type singleInstructionContextCommitState struct {
	context agentcontext.Result
}

func (c *singleInstructionConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return c.contextBudget
}

func (c *singleInstructionConversation) AssembleModelContext(ctx context.Context, _ string, input ModelContextInput) (ModelContextResult, error) {
	message := strings.TrimSpace(input.UserMessage)
	if message == "" {
		message = c.instruction
	}
	stable := strings.TrimSpace(c.stableContext)
	fragments := append([]agentcontext.Fragment(nil), input.Fragments...)
	if stable != "" {
		if c.stableContextMaxBytes <= 0 {
			return ModelContextResult{}, fmt.Errorf("稳定模型上下文缺少大小上限")
		}
		if len([]byte(stable)) > c.stableContextMaxBytes {
			return ModelContextResult{}, fmt.Errorf("稳定模型上下文超过上限: %d > %d bytes", len([]byte(stable)), c.stableContextMaxBytes)
		}
		title := strings.TrimSpace(c.stableContextTitle)
		if title == "" {
			title = "稳定模型上下文"
		}
		if len([]byte(title)) > maxSingleInstructionStableContextTitleBytes {
			return ModelContextResult{}, fmt.Errorf("稳定模型上下文标题超过上限: %d > %d bytes", len([]byte(title)), maxSingleInstructionStableContextTitleBytes)
		}
		stableMessage := agentcontext.StandaloneMessage(title, stable, "")
		if len([]byte(stableMessage)) > c.stableContextMaxBytes {
			return ModelContextResult{}, fmt.Errorf("稳定模型上下文最终消息超过上限: %d > %d bytes", len([]byte(stableMessage)), c.stableContextMaxBytes)
		}
		fragments = append(fragments, agentcontext.Fragment{
			ID: "interactive_director_resident_lore", Source: "interactive.director.resident_lore", Title: title,
			Purpose: "provide complete enabled resident lore as the director's stable model prefix",
			Content: stable, Placement: agentcontext.PlacementLeadingMessage, Limit: c.stableContextMaxBytes, Included: true,
			Note: "source=enabled resident lore; complete=true",
		})
	}
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*adk.Message{adk.UserMessage(message)}, Fragments: fragments,
	})
	if err != nil {
		return ModelContextResult{}, err
	}
	for _, fragment := range assembled.Fragments {
		if fragment.Source == "interactive.director.resident_lore" && (!fragment.Included || fragment.Truncated) {
			return ModelContextResult{}, fmt.Errorf("稳定模型上下文超过 Agent 上下文注入预算；请提高单片段和单轮总注入上限")
		}
	}
	return ModelContextResult{
		Messages:    assembled.Messages,
		Context:     assembled,
		CommitState: singleInstructionContextCommitState{context: assembled},
	}, nil
}

func (c *singleInstructionConversation) CommitModelInput(ctx context.Context, _ string, assembled ModelContextResult) error {
	if c == nil {
		return fmt.Errorf("director conversation is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	state, ok := assembled.CommitState.(singleInstructionContextCommitState)
	if !ok {
		return fmt.Errorf("director model context is missing commit state")
	}
	c.mu.Lock()
	c.lastContext = state.context
	c.mu.Unlock()
	return nil
}

func (c *singleInstructionConversation) ContextSourceSummary() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	fragments := append([]agentcontext.Fragment(nil), c.lastContext.Fragments...)
	c.mu.Unlock()
	if len(fragments) > 0 {
		sources := make([]agentcontext.Source, 0, len(fragments))
		for _, fragment := range fragments {
			sources = append(sources, agentcontext.Source{
				Source: fragment.Source, Title: fragment.Title, Purpose: fragment.Purpose,
				Content: fragment.Content, Placement: fragment.Placement, Limit: fragment.Limit,
				Included: fragment.Included, Truncated: fragment.Truncated, Note: fragment.Note,
			})
		}
		return agentcontext.SourceSummary(sources, agentcontext.DefaultPreviewChars)
	}
	if strings.TrimSpace(c.stableContext) == "" {
		return ""
	}
	return fmt.Sprintf("stable_context title=%q max_bytes=%d content=%s", strings.TrimSpace(c.stableContextTitle), c.stableContextMaxBytes, promptPartSummary(c.stableContext))
}

func (c *singleInstructionConversation) ContextLedgerParts() []ContextLedgerPart {
	if c == nil || strings.TrimSpace(c.stableContext) == "" {
		return nil
	}
	stableMessage := c.stableContextModelMessage()
	return c.ContextLedgerPartsForMessages([]*adk.Message{adk.UserMessage(stableMessage)})
}

func (c *singleInstructionConversation) ContextLedgerPartsForMessages(messages []*adk.Message) []ContextLedgerPart {
	if c == nil || strings.TrimSpace(c.stableContext) == "" {
		return nil
	}
	stableMessage := c.stableContextModelMessage()
	included := false
	for _, message := range messages {
		if message != nil && message.Role == adk.User && strings.TrimSpace(message.Content) == stableMessage {
			included = true
			break
		}
	}
	ledger := NewContextLedger(DefaultLoopPolicy().ContextLedger)
	title := strings.TrimSpace(c.stableContextTitle)
	if title == "" {
		title = "稳定模型上下文"
	}
	bodyBytes := len([]byte(strings.TrimSpace(c.stableContext)))
	messageBytes := len([]byte(stableMessage))
	messageLimit := c.stableContextMaxBytes
	note := fmt.Sprintf("complete=true; source=enabled resident lore; body_bytes=%d; message_bytes=%d; message_max_bytes=%d; final_message=true", bodyBytes, messageBytes, messageLimit)
	if !included {
		note += "; not_present_after_final_compaction"
	}
	ledger.AddPart("ResidentLore", title, "stable model prefix", stableMessage, note, included, !included, messageLimit)
	return ledger.Parts()
}

func (c *singleInstructionConversation) stableContextModelMessage() string {
	stable := strings.TrimSpace(c.stableContext)
	if stable == "" {
		return ""
	}
	title := strings.TrimSpace(c.stableContextTitle)
	if title == "" {
		title = "稳定模型上下文"
	}
	return agentcontext.StandaloneMessage(title, stable, "")
}

func (c *singleInstructionConversation) AppendAssistant(content string) error {
	c.output = content
	return nil
}

func (c *singleInstructionConversation) MarkInterrupted(_, assistantContent, _ string) error {
	c.output = assistantContent
	return nil
}

func (c *singleInstructionConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *singleInstructionConversation) ResolveInterruption(string) error {
	return nil
}
