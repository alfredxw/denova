package chat

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"
	"denova/internal/interactive"
)

type ContextAnalysis struct {
	AgentKind                string                     `json:"agent_kind"`
	Mode                     string                     `json:"mode"`
	SystemPrompt             string                     `json:"system_prompt"`
	SystemPromptParts        []ContextAnalysisPart      `json:"system_prompt_parts"`
	ContextParts             []ContextAnalysisPart      `json:"context_parts"`
	ContextMessages          []ContextAnalysisPart      `json:"context_messages"`
	MessageCount             int                        `json:"message_count"`
	TokenEstimate            int                        `json:"token_estimate"`
	ProjectedTokenEstimate   int                        `json:"projected_token_estimate"`
	ReservedCompletionTokens int                        `json:"reserved_completion_tokens"`
	ReservedToolResultTokens int                        `json:"reserved_tool_result_tokens"`
	ContextWindowTokens      int                        `json:"context_window_tokens"`
	ContextUsageRatio        float64                    `json:"context_usage_ratio"`
	CompactionEpoch          int                        `json:"compaction_epoch,omitempty"`
	CompactionActive         bool                       `json:"compaction_active,omitempty"`
	WouldCompact             bool                       `json:"would_compact,omitempty"`
	Compaction               *ContextAnalysisCompaction `json:"compaction,omitempty"`
}

type ContextAnalysisCompaction struct {
	ID                 string  `json:"id,omitempty"`
	Epoch              int     `json:"epoch"`
	Summary            string  `json:"summary"`
	TokensBefore       int     `json:"tokens_before"`
	TokensAfter        int     `json:"tokens_after"`
	TargetRatio        float64 `json:"target_ratio,omitempty"`
	SourceMessageCount int     `json:"source_message_count,omitempty"`
	SourceTurnCount    int     `json:"source_turn_count,omitempty"`
	Removable          bool    `json:"removable"`
}

type ContextAnalysisPart struct {
	ID         string `json:"id,omitempty"`
	Source     string `json:"source"`
	Title      string `json:"title"`
	Role       string `json:"role,omitempty"`
	Kind       string `json:"kind,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Content    string `json:"content"`
	Note       string `json:"note,omitempty"`
	Bytes      int    `json:"bytes"`
	Chars      int    `json:"chars"`
	// Parts decomposes provider-neutral message fields and safe opaque-state
	// metadata for diagnostics. Content remains the copyable message body.
	Parts []ContextAnalysisPart `json:"parts,omitempty"`
}

type ContextAnalysisPartInput struct {
	ID         string
	Source     string
	Title      string
	Role       string
	Kind       string
	ToolName   string
	ToolCallID string
	Content    string
	Note       string
}

func NewContextAnalysisPart(in ContextAnalysisPartInput) ContextAnalysisPart {
	content := in.Content
	return ContextAnalysisPart{
		ID:         strings.TrimSpace(in.ID),
		Source:     strings.TrimSpace(in.Source),
		Title:      strings.TrimSpace(in.Title),
		Role:       strings.TrimSpace(in.Role),
		Kind:       strings.TrimSpace(in.Kind),
		ToolName:   strings.TrimSpace(in.ToolName),
		ToolCallID: strings.TrimSpace(in.ToolCallID),
		Content:    content,
		Note:       strings.TrimSpace(in.Note),
		Bytes:      len(content),
		Chars:      utf8.RuneCountInString(content),
	}
}

func contextAnalysisPartFromMessage(id, source, title string, msg *agent.Message) ContextAnalysisPart {
	if msg == nil {
		return NewContextAnalysisPart(ContextAnalysisPartInput{ID: id, Source: source, Title: title})
	}
	input := ContextAnalysisPartInput{
		ID:      id,
		Source:  source,
		Title:   title,
		Role:    string(msg.Role),
		Kind:    string(msg.Role),
		Content: msg.Content,
	}
	switch msg.Role {
	case agent.User:
		input.Kind = "body"
	case agent.Assistant:
		input.Kind = "body"
		if len(msg.ToolCalls) > 0 {
			input.Kind = "tool_call"
			input.ToolName = contextAnalysisToolCallNames(msg.ToolCalls)
			input.ToolCallID = contextAnalysisToolCallIDs(msg.ToolCalls)
			if strings.TrimSpace(msg.Content) == "" {
				input.Title = "工具调用：" + firstNonEmpty(input.ToolName, "unknown_tool")
				input.Content = contextAnalysisToolCallsContent(msg.ToolCalls)
			} else {
				input.Title = "助手正文与工具调用：" + firstNonEmpty(input.ToolName, "unknown_tool")
				input.Content = strings.TrimRight(msg.Content, "\n") + "\n\n" + contextAnalysisToolCallsContent(msg.ToolCalls)
			}
		}
	case agent.ToolRole:
		input.Kind = "tool_result"
		input.ToolName = msg.ToolName
		input.ToolCallID = msg.ToolCallID
		input.Title = "工具结果：" + firstNonEmpty(strings.TrimSpace(msg.ToolName), "unknown_tool")
		if strings.TrimSpace(input.ToolCallID) != "" {
			input.Note = "tool_call_id=" + strings.TrimSpace(input.ToolCallID)
		}
	}
	part := NewContextAnalysisPart(input)
	if parts := contextAnalysisAssistantParts(id, source, msg); len(parts) > 0 {
		part.Parts = parts
	}
	return part
}

func contextAnalysisToolCallNames(calls []agent.ToolCall) string {
	names := make([]string, 0, len(calls))
	seen := make(map[string]bool, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

func contextAnalysisToolCallIDs(calls []agent.ToolCall) string {
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		if id := strings.TrimSpace(call.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return strings.Join(ids, ", ")
}

func contextAnalysisToolCallsContent(calls []agent.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[工具调用]\n")
	for i, call := range calls {
		if i > 0 {
			sb.WriteString("\n")
		}
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			name = "unknown_tool"
		}
		sb.WriteString(fmt.Sprintf("%d. %s", i+1, name))
		if id := strings.TrimSpace(call.ID); id != "" {
			sb.WriteString(" (id: ")
			sb.WriteString(id)
			sb.WriteString(")")
		}
		sb.WriteString("\narguments:\n")
		sb.WriteString(strings.TrimSpace(call.Function.Arguments))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func contextAnalysisWorkspace(cfg *config.Config, bookService *book.Service) string {
	if cfg != nil && strings.TrimSpace(cfg.Workspace) != "" {
		return cfg.Workspace
	}
	if bookService != nil {
		return bookService.Workspace()
	}
	return ""
}

func BuildInteractiveStoryContextAnalysis(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, bookService *book.Service, req ChatRequest, compaction *interactive.ContextCompactionProjection, conversation Conversation) (ContextAnalysis, error) {
	if len(teller.StyleRules) == 0 && len(req.StyleRules) > 0 {
		teller.StyleRules = req.StyleRules
	}
	systemPrompt, systemParts, err := buildInteractiveStorySystemPromptAnalysis(cfg, state, teller)
	if err != nil {
		return ContextAnalysis{}, err
	}
	policy := agentrun.DefaultLoopPolicy().Normalize()
	turn, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation,
		Request:      req,
		BookService:  bookService,
		Environment:  newTurnRuntimeEnvironment(contextAnalysisWorkspace(cfg, bookService)),
	})
	if err != nil {
		return ContextAnalysis{}, err
	}
	messages := turn.ModelContext.Messages
	contextMessages := make([]ContextAnalysisPart, 0, len(messages))
	compactionEpoch := 0
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		source := "互动历史回合"
		title := fmt.Sprintf("历史回合消息 %d", i+1)
		switch {
		case agentcontext.IsCompactionSummaryMessage(msg):
			source = "上下文压缩"
			title = "模型可见历史检查点"
			compactionEpoch = parseCompactionEpoch(msg.Content)
		case i == len(messages)-1:
			source = "本轮互动指令"
			title = "本轮互动指令与动态上下文"
		}
		part := contextAnalysisPartFromMessage(fmt.Sprintf("message_%d", i+1), source, title, msg)
		if i == len(messages)-1 {
			part.Parts = buildInteractiveStoryInstructionContextParts(part.Content)
		}
		contextMessages = append(contextMessages, part)
	}
	usage := analyzeContextUsage(cfg, config.AgentKindInteractiveStory, systemPrompt, messages, teller.ReplyTargetChars)
	return ContextAnalysis{
		AgentKind:                config.AgentKindInteractiveStory,
		Mode:                     "interactive",
		SystemPrompt:             systemPrompt,
		SystemPromptParts:        systemParts,
		ContextParts:             contextBuildLogFromAssembly(policy.ContextLedger, turn.OriginalMessage, turn.ModelContext.Context).FullParts(),
		ContextMessages:          contextMessages,
		MessageCount:             len(contextMessages),
		TokenEstimate:            usage.tokens,
		ProjectedTokenEstimate:   usage.projectedTokens,
		ReservedCompletionTokens: usage.completionReserve,
		ReservedToolResultTokens: usage.toolResultReserve,
		ContextWindowTokens:      usage.window,
		ContextUsageRatio:        usage.ratio,
		CompactionEpoch:          interactiveCompactionEpoch(compaction, compactionEpoch),
		CompactionActive:         compaction != nil && strings.TrimSpace(compaction.Summary) != "",
		WouldCompact:             usage.wouldCompact,
		Compaction:               contextAnalysisCompactionFromInteractive(compaction),
	}, nil
}

func interactiveCompactionEpoch(compaction *interactive.ContextCompactionProjection, fallback int) int {
	if compaction == nil {
		return fallback
	}
	return compaction.Epoch
}

func contextAnalysisCompactionFromInteractive(compaction *interactive.ContextCompactionProjection) *ContextAnalysisCompaction {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return nil
	}
	return &ContextAnalysisCompaction{
		ID:              compaction.ID,
		Epoch:           compaction.Epoch,
		Summary:         compaction.Summary,
		TokensBefore:    compaction.TokensBefore,
		TokensAfter:     compaction.TokensAfter,
		TargetRatio:     compaction.TargetRatio,
		SourceTurnCount: compaction.SourceTurnCount,
		Removable:       true,
	}
}

type contextUsageAnalysis struct {
	tokens            int
	projectedTokens   int
	completionReserve int
	toolResultReserve int
	window            int
	ratio             float64
	wouldCompact      bool
}

func analyzeContextUsage(cfg *config.Config, agentKind, systemPrompt string, messages []*agent.Message, expectedOutputChars int) contextUsageAnalysis {
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	estimatedMessages := make([]*agent.Message, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		estimatedMessages = append(estimatedMessages, agent.SystemMessage(systemPrompt))
	}
	estimatedMessages = append(estimatedMessages, messages...)
	tokens := agentcontext.EstimateTokens(estimatedMessages, nil)
	completionReserve, toolResultReserve := agentcompaction.EstimateProjectionReserves(cfg, agentKind, expectedOutputChars)
	if maxTokens := modelSettings.MaxTokens; maxTokens != nil {
		totalReserve := agent.CapacityAwareTokenReserve(
			completionReserve+toolResultReserve, *maxTokens,
			modelSettings.ContextWindowTokens, contextSettings.CompactionThreshold,
		)
		completionReserve = max(0, totalReserve-toolResultReserve)
	}
	usage := contextUsageAnalysis{
		tokens:            tokens,
		projectedTokens:   tokens + completionReserve + toolResultReserve,
		completionReserve: completionReserve,
		toolResultReserve: toolResultReserve,
		window:            modelSettings.ContextWindowTokens,
	}
	if usage.window > 0 {
		usage.ratio = float64(usage.projectedTokens) / float64(usage.window)
		usage.wouldCompact = contextSettings.CompactionEnabled && usage.ratio >= contextSettings.CompactionThreshold
	}
	return usage
}

func parseCompactionEpoch(content string) int {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, agentcontext.CompactionSummaryPrefix) {
		return 0
	}
	var epoch int
	if _, err := fmt.Sscanf(content, agentcontext.CompactionSummaryPrefix+" epoch=%d", &epoch); err != nil {
		return 0
	}
	return epoch
}

func buildIDESystemPromptAnalysis(cfg *config.Config, state *book.State, teller prompts.IDEStoryTeller) (string, []ContextAnalysisPart, error) {
	composition, err := prompts.ComposeInstruction(cfg, state, teller)
	if err != nil {
		return "", nil, err
	}
	return composition.Instruction(), systemPromptAnalysisParts(composition), nil
}

func buildInteractiveStorySystemPromptAnalysis(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (string, []ContextAnalysisPart, error) {
	composition, err := prompts.ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		return "", nil, err
	}
	return composition.Instruction(), systemPromptAnalysisParts(composition), nil
}

func systemPromptAnalysisParts(composition prompts.SystemPromptComposition) []ContextAnalysisPart {
	resolvedFragments := composition.Fragments()
	fragments := make(map[string]prompts.SystemPromptFragment, len(resolvedFragments))
	for _, fragment := range resolvedFragments {
		fragments[fragment.ID] = fragment
	}
	manifest := composition.Manifest()
	parts := make([]ContextAnalysisPart, 0, len(manifest))
	for _, entry := range manifest {
		fragment, included := fragments[entry.ID]
		content := ""
		if included {
			content = fragment.Prefix + fragment.Content + fragment.Suffix
		}
		note := fmt.Sprintf("included=%t; original_bytes=%d; included_bytes=%d; original_sha=%s; included_sha=%s", entry.Included, entry.OriginalBytes, entry.IncludedBytes, entry.OriginalSHA, entry.IncludedSHA)
		if entry.Truncated {
			note += "; truncated=true"
		}
		if entry.Rejected {
			note += "; rejected=true"
		}
		if entry.Reason != "" {
			note += "; reason=" + entry.Reason
		}
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID: entry.ID, Source: entry.Source, Title: entry.Title, Role: "system", Kind: "system_fragment",
			Content: content, Note: note,
		}))
	}
	return parts
}

func styleRuleContextAnalysisParts(rules []prompts.StyleRule) []ContextAnalysisPart {
	parts := make([]ContextAnalysisPart, 0, len(rules))
	for i, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		content := prompts.StyleRulesInstruction([]prompts.StyleRule{rule})
		if strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      fmt.Sprintf("style_rule_%d", i+1),
			Source:  "当前叙事风格",
			Title:   "文风参考：" + styleRuleAnalysisTitle(rule),
			Content: content,
			Note:    "system prompt",
		}))
	}
	return parts
}

func styleRuleAnalysisTitle(rule prompts.StyleRule) string {
	if rule.Global {
		return "全局"
	}
	return strings.TrimSpace(rule.Scene)
}
