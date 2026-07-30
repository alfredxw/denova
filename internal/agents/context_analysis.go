package agents

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/prompts"
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
	// Parts decomposes one exact model-visible message for diagnostics; Content remains the copy/source-of-truth payload.
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
	return NewContextAnalysisPart(input)
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

func BuildIDEContextAnalysis(cfg *config.Config, state *book.State, teller IDEStoryTeller, bookService *book.Service, compaction *session.ContextCompaction, pending *session.Interruption, req ChatRequest, conversation Conversation) (ContextAnalysis, error) {
	if len(teller.StyleRules) == 0 && len(req.StyleRules) > 0 {
		teller.StyleRules = req.StyleRules
	}
	systemPrompt, systemParts, err := buildIDESystemPromptAnalysis(cfg, state, teller)
	if err != nil {
		return ContextAnalysis{}, err
	}
	policy := DefaultLoopPolicy().normalized()
	turn, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation:        conversation,
		Request:             req,
		PendingInterruption: pending,
		BookService:         bookService,
		Environment:         newTurnRuntimeEnvironment(contextAnalysisWorkspace(cfg, bookService)),
	})
	if err != nil {
		return ContextAnalysis{}, err
	}
	messages := turn.ModelContext.Messages
	runtimeContexts := IDEWorkspaceRuntimeContextsForRequest(state, req)
	contextMessages := make([]ContextAnalysisPart, 0, len(messages))
	stableMessageCount := 0
	for _, fragment := range turn.ModelContext.Context.Fragments {
		if fragment.Included && fragment.Placement == agentcontext.PlacementLeadingMessage {
			stableMessageCount++
		}
	}
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		source := "会话历史"
		title := fmt.Sprintf("历史消息 %d", i+1)
		if i < stableMessageCount {
			source = "稳定作品上下文"
			title = runtimeContexts.StableTitle
		} else if isContextCompactionMessage(msg) {
			source = "上下文压缩"
			title = "模型可见历史检查点"
		} else if i == len(messages)-1 {
			source = "本轮上下文"
			if strings.TrimSpace(runtimeContexts.Dynamic) != "" {
				title = "动态作品状态与本轮用户请求"
			} else {
				title = "本轮发送给 Agent 的用户消息"
			}
		}
		contextMessages = append(contextMessages, contextAnalysisPartFromMessage(fmt.Sprintf("message_%d", i+1), source, title, msg))
	}
	usage := analyzeContextUsage(cfg, config.AgentKindIDE, systemPrompt, messages, 0)
	return ContextAnalysis{
		AgentKind:                config.AgentKindIDE,
		Mode:                     "ide",
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
		CompactionEpoch:          usage.compactionEpoch(compaction),
		CompactionActive:         compaction != nil && strings.TrimSpace(compaction.Summary) != "",
		WouldCompact:             usage.wouldCompact,
		Compaction:               contextAnalysisCompactionFromSession(compaction),
	}, nil
}

// BuildGeneralContextAnalysis mirrors the exact General Agent turn assembly
// without injecting book-only creator, lore, teller, or dynamic writing state.
func BuildGeneralContextAnalysis(cfg *config.Config, bookService *book.Service, compaction *session.ContextCompaction, pending *session.Interruption, req ChatRequest, conversation Conversation) (ContextAnalysis, error) {
	composition, err := ComposeGeneralInstruction(cfg)
	if err != nil {
		return ContextAnalysis{}, err
	}
	systemPrompt := composition.Instruction()
	policy := DefaultLoopPolicy().normalized()
	turn, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation, Request: req, PendingInterruption: pending,
		BookService: bookService,
		Environment: newTurnRuntimeEnvironment(contextAnalysisWorkspace(cfg, bookService)),
	})
	if err != nil {
		return ContextAnalysis{}, err
	}
	messages := turn.ModelContext.Messages
	contextMessages := make([]ContextAnalysisPart, 0, len(messages))
	for index, message := range messages {
		if message == nil {
			continue
		}
		source := "会话历史"
		title := fmt.Sprintf("历史消息 %d", index+1)
		if isContextCompactionMessage(message) {
			source = "上下文压缩"
			title = "模型可见历史检查点"
		} else if index == len(messages)-1 {
			source = "本轮上下文"
			title = "本轮发送给 General Agent 的用户消息"
		}
		contextMessages = append(contextMessages, contextAnalysisPartFromMessage(
			fmt.Sprintf("message_%d", index+1), source, title, message,
		))
	}
	usage := analyzeContextUsage(cfg, config.AgentKindGeneral, systemPrompt, messages, 0)
	return ContextAnalysis{
		AgentKind: config.AgentKindGeneral, Mode: "general",
		SystemPrompt: systemPrompt, SystemPromptParts: systemPromptAnalysisParts(composition),
		ContextParts:    contextBuildLogFromAssembly(policy.ContextLedger, turn.OriginalMessage, turn.ModelContext.Context).FullParts(),
		ContextMessages: contextMessages, MessageCount: len(contextMessages),
		TokenEstimate: usage.tokens, ProjectedTokenEstimate: usage.projectedTokens,
		ReservedCompletionTokens: usage.completionReserve, ReservedToolResultTokens: usage.toolResultReserve,
		ContextWindowTokens: usage.window, ContextUsageRatio: usage.ratio,
		CompactionEpoch:  usage.compactionEpoch(compaction),
		CompactionActive: compaction != nil && strings.TrimSpace(compaction.Summary) != "",
		WouldCompact:     usage.wouldCompact, Compaction: contextAnalysisCompactionFromSession(compaction),
	}, nil
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

func BuildInteractiveStoryContextAnalysis(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, bookService *book.Service, req ChatRequest, compaction *interactive.ContextCompactionEvent, conversation Conversation) (ContextAnalysis, error) {
	if len(teller.StyleRules) == 0 && len(req.StyleRules) > 0 {
		teller.StyleRules = req.StyleRules
	}
	systemPrompt, systemParts, err := buildInteractiveStorySystemPromptAnalysis(cfg, state, teller)
	if err != nil {
		return ContextAnalysis{}, err
	}
	policy := DefaultLoopPolicy().normalized()
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
		case isContextCompactionMessage(msg):
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

func BuildInteractiveDirectorContextAnalysis(cfg *config.Config, instruction string) (ContextAnalysis, error) {
	return BuildInteractiveDirectorContextAnalysisWithStableContext(cfg, "", "", 0, instruction)
}

// BuildInteractiveDirectorContextAnalysisWithStableContext mirrors the exact
// two-message layout used by the tool-enabled Director when resident Lore is
// present, rather than hiding that stable prefix from context diagnostics.
func BuildInteractiveDirectorContextAnalysisWithStableContext(cfg *config.Config, stableTitle, stableContext string, stableMaxBytes int, instruction string) (ContextAnalysis, error) {
	systemPrompt, systemParts, err := buildInteractiveDirectorSystemPromptAnalysis(cfg)
	if err != nil {
		return ContextAnalysis{}, err
	}
	conversation := &singleInstructionConversation{
		instruction:           instruction,
		stableContextTitle:    stableTitle,
		stableContext:         stableContext,
		stableContextMaxBytes: stableMaxBytes,
		contextBudget:         contextBudgetForAgent(cfg, config.AgentKindInteractiveDirector),
	}
	turn, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation,
		Request:      ChatRequest{Message: instruction},
		Environment:  newTurnRuntimeEnvironment(contextAnalysisWorkspace(cfg, nil)),
	})
	if err != nil {
		return ContextAnalysis{}, err
	}
	messages := turn.ModelContext.Messages
	contextMessages := make([]ContextAnalysisPart, 0, len(messages)+8)
	if len(messages) > 1 {
		part := contextAnalysisPartFromMessage("resident_lore", "enabled resident lore", strings.TrimSpace(stableTitle), messages[0])
		part.Note = fmt.Sprintf("stable_model_prefix; complete=true; max_bytes=%d", stableMaxBytes)
		contextMessages = append(contextMessages, part)
	}
	instructionParts := buildInteractiveDirectorInstructionContextParts(instruction)
	if len(instructionParts) == 0 {
		instructionParts = append(instructionParts, contextAnalysisPartFromMessage("director_instruction", "本轮导演指令", "后台导演规划指令", messages[len(messages)-1]))
	}
	contextMessages = append(contextMessages, instructionParts...)
	usage := analyzeContextUsage(cfg, config.AgentKindInteractiveDirector, systemPrompt, messages, 1024)
	return ContextAnalysis{
		AgentKind:                config.AgentKindInteractiveDirector,
		Mode:                     "interactive_director",
		SystemPrompt:             systemPrompt,
		SystemPromptParts:        systemParts,
		ContextParts:             contextBuildLogFromAssembly(DefaultLoopPolicy().ContextLedger, turn.OriginalMessage, turn.ModelContext.Context).FullParts(),
		ContextMessages:          contextMessages,
		MessageCount:             len(messages),
		TokenEstimate:            usage.tokens,
		ProjectedTokenEstimate:   usage.projectedTokens,
		ReservedCompletionTokens: usage.completionReserve,
		ReservedToolResultTokens: usage.toolResultReserve,
		ContextWindowTokens:      usage.window,
		ContextUsageRatio:        usage.ratio,
		WouldCompact:             usage.wouldCompact,
	}, nil
}

func interactiveCompactionEpoch(compaction *interactive.ContextCompactionEvent, fallback int) int {
	if compaction == nil {
		return fallback
	}
	return compaction.Epoch
}

func contextAnalysisCompactionFromSession(compaction *session.ContextCompaction) *ContextAnalysisCompaction {
	if compaction == nil || strings.TrimSpace(compaction.Summary) == "" {
		return nil
	}
	return &ContextAnalysisCompaction{
		ID:                 compaction.ID,
		Epoch:              compaction.Epoch,
		Summary:            compaction.Summary,
		TokensBefore:       compaction.TokensBefore,
		TokensAfter:        compaction.TokensAfter,
		TargetRatio:        compaction.TargetRatio,
		SourceMessageCount: compaction.SourceMessageCount,
		Removable:          true,
	}
}

func contextAnalysisCompactionFromInteractive(compaction *interactive.ContextCompactionEvent) *ContextAnalysisCompaction {
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

func (u contextUsageAnalysis) compactionEpoch(compaction *session.ContextCompaction) int {
	if compaction == nil {
		return 0
	}
	return compaction.Epoch
}

func analyzeContextUsage(cfg *config.Config, agentKind, systemPrompt string, messages []*agent.Message, expectedOutputChars int) contextUsageAnalysis {
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	estimatedMessages := make([]*agent.Message, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		estimatedMessages = append(estimatedMessages, agent.SystemMessage(systemPrompt))
	}
	estimatedMessages = append(estimatedMessages, messages...)
	tokens := EstimateContextTokens(estimatedMessages, nil)
	completionReserve, toolResultReserve := EstimateContextProjectionReserves(cfg, agentKind, expectedOutputChars)
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
	if !strings.HasPrefix(content, contextCompactionSummaryPrefix) {
		return 0
	}
	var epoch int
	if _, err := fmt.Sscanf(content, contextCompactionSummaryPrefix+" epoch=%d", &epoch); err != nil {
		return 0
	}
	return epoch
}

func buildIDESystemPromptAnalysis(cfg *config.Config, state *book.State, teller IDEStoryTeller) (string, []ContextAnalysisPart, error) {
	composition, err := ComposeInstruction(cfg, state, teller)
	if err != nil {
		return "", nil, err
	}
	return composition.Instruction(), systemPromptAnalysisParts(composition), nil
}

func buildInteractiveStorySystemPromptAnalysis(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (string, []ContextAnalysisPart, error) {
	composition, err := ComposeInteractiveStoryInstruction(cfg, state, teller)
	if err != nil {
		return "", nil, err
	}
	return composition.Instruction(), systemPromptAnalysisParts(composition), nil
}

func buildInteractiveDirectorSystemPromptAnalysis(cfg *config.Config) (string, []ContextAnalysisPart, error) {
	composition, err := ComposeInteractiveDirectorInstruction(cfg, nil)
	if err != nil {
		return "", nil, err
	}
	return composition.Instruction(), systemPromptAnalysisParts(composition), nil
}

func systemPromptAnalysisParts(composition SystemPromptComposition) []ContextAnalysisPart {
	fragments := make(map[string]SystemPromptFragment, len(composition.fragments))
	for _, fragment := range composition.fragments {
		fragments[fragment.ID] = fragment
	}
	parts := make([]ContextAnalysisPart, 0, len(composition.manifest))
	for _, entry := range composition.manifest {
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

func buildInteractiveDirectorInstructionContextParts(instruction string) []ContextAnalysisPart {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return nil
	}
	segments := strings.Split("\n"+instruction, "\n## ")
	parts := make([]ContextAnalysisPart, 0, len(segments))
	if preamble := strings.TrimSpace(strings.TrimPrefix(segments[0], "\n")); preamble != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "director_instruction_preamble",
			Source:  "本轮导演指令",
			Title:   "后台导演任务与约束",
			Role:    "user",
			Kind:    "body",
			Content: preamble,
			Note:    "final_user_message",
		}))
	}
	for _, segment := range segments[1:] {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		heading, content, _ := strings.Cut(segment, "\n")
		title, source, note := directorInstructionHeadingMeta(heading)
		role := ""
		if len(parts) == 0 {
			role = "user"
		}
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      fmt.Sprintf("director_instruction_part_%02d", len(parts)+1),
			Source:  source,
			Title:   title,
			Role:    role,
			Kind:    "body",
			Content: strings.TrimSpace(content),
			Note:    note,
		}))
	}
	return parts
}

func directorInstructionHeadingMeta(heading string) (title, source, note string) {
	title = strings.TrimSpace(heading)
	source = "后台导演上下文"
	if strings.Contains(title, "（source:") {
		if before, after, ok := strings.Cut(title, "（source:"); ok {
			title = strings.TrimSpace(before)
			source = strings.TrimSpace(strings.TrimSuffix(after, "）"))
		}
	} else if strings.Contains(title, "(source:") {
		if before, after, ok := strings.Cut(title, "(source:"); ok {
			title = strings.TrimSpace(before)
			source = strings.TrimSpace(strings.TrimSuffix(after, ")"))
		}
	}
	if title == "" {
		title = "导演上下文片段"
	}
	if source == "" {
		source = "后台导演上下文"
	}
	if strings.Contains(source, "bounded") || strings.Contains(title, "上限") {
		note = "bounded"
	}
	switch title {
	case "文件操作要求", "固定标题", "更新原则":
		source = "Denova built-in"
		if note == "" {
			note = "final_user_message"
		} else {
			note += " · final_user_message"
		}
	default:
		if note == "" {
			note = "final_user_message"
		} else {
			note += " · final_user_message"
		}
	}
	return title, source, note
}

func styleRuleContextAnalysisParts(rules []StyleRule) []ContextAnalysisPart {
	parts := make([]ContextAnalysisPart, 0, len(rules))
	for i, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		content := styleRulesSystemInstruction([]StyleRule{rule})
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

func styleRuleAnalysisTitle(rule StyleRule) string {
	if rule.Global {
		return "全局"
	}
	return strings.TrimSpace(rule.Scene)
}
