package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/prompts"
	"nova/internal/session"
)

type ContextAnalysis struct {
	AgentKind           string                     `json:"agent_kind"`
	Mode                string                     `json:"mode"`
	SystemPrompt        string                     `json:"system_prompt"`
	SystemPromptParts   []ContextAnalysisPart      `json:"system_prompt_parts"`
	ContextParts        []ContextAnalysisPart      `json:"context_parts"`
	ContextMessages     []ContextAnalysisPart      `json:"context_messages"`
	MessageCount        int                        `json:"message_count"`
	TokenEstimate       int                        `json:"token_estimate"`
	ContextWindowTokens int                        `json:"context_window_tokens"`
	ContextUsageRatio   float64                    `json:"context_usage_ratio"`
	CompactionEpoch     int                        `json:"compaction_epoch,omitempty"`
	CompactionActive    bool                       `json:"compaction_active,omitempty"`
	WouldCompact        bool                       `json:"would_compact,omitempty"`
	Compaction          *ContextAnalysisCompaction `json:"compaction,omitempty"`
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
	ID      string `json:"id,omitempty"`
	Source  string `json:"source"`
	Title   string `json:"title"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content"`
	Note    string `json:"note,omitempty"`
	Bytes   int    `json:"bytes"`
	Chars   int    `json:"chars"`
}

type ContextAnalysisPartInput struct {
	ID      string
	Source  string
	Title   string
	Role    string
	Content string
	Note    string
}

func NewContextAnalysisPart(in ContextAnalysisPartInput) ContextAnalysisPart {
	content := in.Content
	return ContextAnalysisPart{
		ID:      strings.TrimSpace(in.ID),
		Source:  strings.TrimSpace(in.Source),
		Title:   strings.TrimSpace(in.Title),
		Role:    strings.TrimSpace(in.Role),
		Content: content,
		Note:    strings.TrimSpace(in.Note),
		Bytes:   len(content),
		Chars:   utf8.RuneCountInString(content),
	}
}

func BuildIDEContextAnalysis(cfg *config.Config, state *book.State, teller IDEStoryTeller, bookService *book.Service, effectiveMessages []*schema.Message, totalMessages int, compaction *session.ContextCompaction, pending *session.Interruption, req ChatRequest) (ContextAnalysis, error) {
	if len(teller.StyleRules) == 0 && len(req.StyleRules) > 0 {
		teller.StyleRules = req.StyleRules
	}
	systemPrompt, systemParts := buildIDESystemPromptAnalysis(cfg, state, teller)
	policy := DefaultLoopPolicy().normalized()
	composition := composeAgentInput(req, pending, bookService, policy)
	messages := buildIDEAnalysisMessages(cfg, effectiveMessages, totalMessages, compaction)
	runtimeContexts := IDEWorkspaceRuntimeContextsForRequest(state, req)
	if strings.TrimSpace(runtimeContexts.Stable) != "" {
		messages = append([]*schema.Message{schema.UserMessage(standaloneRuntimeContextMessage(runtimeContexts.StableTitle, runtimeContexts.Stable, ""))}, messages...)
	}
	messages = append(messages, schema.UserMessage(prependRuntimeContextToAgentMessage(
		composition.AgentMessage,
		runtimeContexts.DynamicTitle,
		runtimeContexts.Dynamic,
	)))
	contextMessages := make([]ContextAnalysisPart, 0, len(messages))
	stableMessageCount := 0
	if strings.TrimSpace(runtimeContexts.Stable) != "" {
		stableMessageCount = 1
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
			title = "模型可见压缩摘要"
		} else if i == len(messages)-1 {
			source = "本轮上下文"
			if strings.TrimSpace(runtimeContexts.Dynamic) != "" {
				title = "动态作品状态与本轮用户请求"
			} else {
				title = "本轮发送给 Agent 的用户消息"
			}
		}
		contextMessages = append(contextMessages, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      fmt.Sprintf("message_%d", i+1),
			Source:  source,
			Title:   title,
			Role:    string(msg.Role),
			Content: msg.Content,
		}))
	}
	usage := analyzeContextUsage(cfg, config.AgentKindIDE, systemPrompt, messages)
	return ContextAnalysis{
		AgentKind:           config.AgentKindIDE,
		Mode:                "ide",
		SystemPrompt:        systemPrompt,
		SystemPromptParts:   systemParts,
		ContextParts:        composition.ContextLog.FullParts(),
		ContextMessages:     contextMessages,
		MessageCount:        len(contextMessages),
		TokenEstimate:       usage.tokens,
		ContextWindowTokens: usage.window,
		ContextUsageRatio:   usage.ratio,
		CompactionEpoch:     usage.compactionEpoch(compaction),
		CompactionActive:    compaction != nil && strings.TrimSpace(compaction.Summary) != "",
		WouldCompact:        usage.wouldCompact,
		Compaction:          contextAnalysisCompactionFromSession(compaction),
	}, nil
}

func BuildInteractiveStoryContextAnalysis(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput, bookService *book.Service, req ChatRequest, compaction *interactive.ContextCompactionEvent, prepareMessages func(originalMessage, agentMessage string) ([]*schema.Message, error)) (ContextAnalysis, error) {
	if len(teller.StyleRules) == 0 && len(req.StyleRules) > 0 {
		teller.StyleRules = req.StyleRules
	}
	systemPrompt, systemParts := buildInteractiveStorySystemPromptAnalysis(cfg, state, teller)
	policy := DefaultLoopPolicy().normalized()
	composition := composeAgentInput(req, nil, bookService, policy)
	messages, err := prepareMessages(composition.OriginalMessage, composition.AgentMessage)
	if err != nil {
		return ContextAnalysis{}, err
	}
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
			title = "模型可见压缩摘要"
			compactionEpoch = parseCompactionEpoch(msg.Content)
		case i == len(messages)-1:
			source = "本轮互动指令"
			title = "本轮互动指令与动态上下文"
		}
		contextMessages = append(contextMessages, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      fmt.Sprintf("message_%d", i+1),
			Source:  source,
			Title:   title,
			Role:    string(msg.Role),
			Content: msg.Content,
		}))
	}
	usage := analyzeContextUsage(cfg, config.AgentKindInteractiveStory, systemPrompt, messages)
	return ContextAnalysis{
		AgentKind:           config.AgentKindInteractiveStory,
		Mode:                "interactive",
		SystemPrompt:        systemPrompt,
		SystemPromptParts:   systemParts,
		ContextParts:        composition.ContextLog.FullParts(),
		ContextMessages:     contextMessages,
		MessageCount:        len(contextMessages),
		TokenEstimate:       usage.tokens,
		ContextWindowTokens: usage.window,
		ContextUsageRatio:   usage.ratio,
		CompactionEpoch:     interactiveCompactionEpoch(compaction, compactionEpoch),
		CompactionActive:    compaction != nil && strings.TrimSpace(compaction.Summary) != "",
		WouldCompact:        usage.wouldCompact,
		Compaction:          contextAnalysisCompactionFromInteractive(compaction),
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

func buildIDEAnalysisMessages(cfg *config.Config, effectiveMessages []*schema.Message, totalMessages int, compaction *session.ContextCompaction) []*schema.Message {
	messages := make([]*schema.Message, 0, len(effectiveMessages)+1)
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		effectiveStart := totalMessages - len(effectiveMessages)
		retainedTurns := compaction.RetainedTurns
		if retainedTurns <= 0 {
			retainedTurns = config.DefaultContextCompactionRetainedTurns
		}
		tail := compactedMessagesAfterSource(effectiveMessages, effectiveStart, compaction.SourceEndIndex, retainedTurns)
		messages = append(messages, NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
		messages = append(messages, tail...)
		return messages
	}
	for _, msg := range effectiveMessages {
		if msg != nil {
			messages = append(messages, msg)
		}
	}
	return messages
}

type contextUsageAnalysis struct {
	tokens       int
	window       int
	ratio        float64
	wouldCompact bool
}

func (u contextUsageAnalysis) compactionEpoch(compaction *session.ContextCompaction) int {
	if compaction == nil {
		return 0
	}
	return compaction.Epoch
}

func analyzeContextUsage(cfg *config.Config, agentKind, systemPrompt string, messages []*schema.Message) contextUsageAnalysis {
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	estimatedMessages := make([]*schema.Message, 0, len(messages)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		estimatedMessages = append(estimatedMessages, schema.SystemMessage(systemPrompt))
	}
	estimatedMessages = append(estimatedMessages, messages...)
	tokens := EstimateContextTokens(estimatedMessages, nil)
	usage := contextUsageAnalysis{tokens: tokens, window: modelSettings.ContextWindowTokens}
	if usage.window > 0 {
		usage.ratio = float64(tokens) / float64(usage.window)
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

func buildIDESystemPromptAnalysis(cfg *config.Config, state *book.State, teller IDEStoryTeller) (string, []ContextAnalysisPart) {
	builtIn, workspace, creator, _ := buildIDEBuiltinInstruction(cfg, state, teller)
	systemPrompt := protectedSystemInstruction(cfg, config.AgentKindIDE, builtIn)
	resolved := config.ResolveAgentPrompt(cfg, config.AgentKindIDE)
	parts := []ContextAnalysisPart{
		NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "runtime_contract",
			Source:  "Nova runtime",
			Title:   "运行契约",
			Content: runtimeContractForAgent(cfg, config.AgentKindIDE),
		}),
	}
	if outputProtocol := strings.TrimSpace(outputProtocolForAgent(config.AgentKindIDE)); outputProtocol != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "output_protocol",
			Source:  "Nova runtime",
			Title:   "输出格式",
			Content: outputProtocol,
		}))
	}
	if flow := strings.TrimSpace(resolved.FlowPrompt); flow != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "custom_flow",
			Source:  "user/workspace config",
			Title:   "用户自定义流程规则",
			Content: flow,
		}))
	}
	if custom := strings.TrimSpace(resolved.SystemPrompt); custom != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "custom_system",
			Source:  "user/workspace config",
			Title:   "用户自定义系统提示",
			Content: custom,
		}))
	}
	if strings.TrimSpace(creator) != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "creator",
			Source:  "CREATOR.md",
			Title:   "创作者指令",
			Content: creator,
		}))
	}
	if strings.TrimSpace(teller.Prompt) != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "ide_teller",
			Source:  teller.ID,
			Title:   "写作模式默认导演规则",
			Content: teller.Prompt,
		}))
	}
	if strings.TrimSpace(teller.ImagePresetSystemPrompt) != "" {
		title := "图像方案系统规则"
		if strings.TrimSpace(teller.ImagePresetName) != "" {
			title = "图像方案系统规则：" + strings.TrimSpace(teller.ImagePresetName)
		}
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "image_preset_system",
			Source:  teller.ImagePresetID,
			Title:   title,
			Content: teller.ImagePresetSystemPrompt,
			Note:    "仅用于图像生成 system prompt",
		}))
	}
	parts = append(parts, styleRuleContextAnalysisParts(teller.StyleRules)...)
	parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
		ID:      "flow",
		Source:  "Nova built-in",
		Title:   "写作模式流程配置",
		Content: ideFlowInstruction(cfg, workspace),
	}))
	return systemPrompt, parts
}

func buildInteractiveStorySystemPromptAnalysis(cfg *config.Config, state *book.State, teller prompts.InteractiveStorySystemInstructionInput) (string, []ContextAnalysisPart) {
	builtIn, workspace, creator := buildInteractiveStoryBuiltinInstruction(cfg, state, teller)
	systemPrompt := protectedSystemInstruction(cfg, config.AgentKindInteractiveStory, builtIn)
	resolved := config.ResolveAgentPrompt(cfg, config.AgentKindInteractiveStory)
	parts := []ContextAnalysisPart{
		NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "runtime_contract",
			Source:  "Nova runtime",
			Title:   "运行契约",
			Content: runtimeContractForAgent(cfg, config.AgentKindInteractiveStory),
		}),
	}
	if outputProtocol := strings.TrimSpace(outputProtocolForAgent(config.AgentKindInteractiveStory)); outputProtocol != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "output_protocol",
			Source:  "Nova runtime",
			Title:   "输出格式",
			Content: outputProtocol,
		}))
	}
	if flow := strings.TrimSpace(resolved.FlowPrompt); flow != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "custom_flow",
			Source:  "user/workspace config",
			Title:   "用户自定义流程规则",
			Content: flow,
		}))
	}
	if custom := strings.TrimSpace(resolved.SystemPrompt); custom != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "custom_system",
			Source:  "user/workspace config",
			Title:   "用户自定义系统提示",
			Content: custom,
		}))
	}
	if strings.TrimSpace(creator) != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "creator",
			Source:  "CREATOR.md",
			Title:   "创作者指令",
			Content: creator,
		}))
	}
	if strings.TrimSpace(teller.StoryTellerSystemPrompt) != "" {
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      "interactive_teller",
			Source:  teller.StoryTellerID,
			Title:   "互动叙事方案系统规则",
			Content: teller.StoryTellerSystemPrompt,
		}))
	}
	parts = append(parts, styleRuleContextAnalysisParts(teller.StyleRules)...)
	parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
		ID:      "flow",
		Source:  "Nova built-in",
		Title:   "互动故事流程规则",
		Content: interactiveStoryFlowInstruction(cfg, workspace),
	}))
	return systemPrompt, parts
}

func styleRuleContextAnalysisParts(rules []StyleRule) []ContextAnalysisPart {
	rules = boundedStyleRules(rules, maxStyleRuleContextChars)
	parts := make([]ContextAnalysisPart, 0, len(rules))
	for i, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || len(rule.StyleContents) == 0 {
			continue
		}
		content := styleRulesSystemInstruction([]StyleRule{rule})
		if strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, NewContextAnalysisPart(ContextAnalysisPartInput{
			ID:      fmt.Sprintf("style_rule_%d", i+1),
			Source:  "当前叙事方案",
			Title:   "场景化风格规则：" + scene,
			Content: content,
			Note:    "system prompt",
		}))
	}
	return parts
}

type agentInputComposition struct {
	OriginalMessage    string
	Request            ChatRequest
	AgentMessage       string
	ContextLog         *contextBuildLog
	ResumeInterruption *session.Interruption
}

func composeAgentInput(req ChatRequest, pending *session.Interruption, bookService *book.Service, policy LoopPolicy) agentInputComposition {
	originalMessage := req.Message
	resumeInterruption := pending
	if !shouldResumeInterruptedRequest(req.Message) {
		resumeInterruption = nil
	}
	if resumeInterruption != nil {
		req.Message = buildInterruptedResumeMessage(req.Message, resumeInterruption)
	}
	agentMessage := req.Message
	contextLog := newContextBuildLog(policy.ContextLedger)
	contextLog.add("用户输入", "本轮原始请求", originalMessage, "")
	if resumeInterruption != nil {
		contextLog.add("运行时恢复", "异常中断恢复上下文", req.Message, "包含上一轮原始请求、已生成助手内容和中断原因")
	}
	if req.PlanMode {
		agentMessage = appendPlanModeInstruction(agentMessage)
		contextLog.add("注入规则", "规划模式", "[规划模式] 请你先制定计划，不要执行任何写操作。", "")
	}
	if strings.TrimSpace(req.WritingSkill) != "" {
		agentMessage = appendWritingSkillLoadHint(agentMessage, req.WritingSkill, contextLog)
	}
	if len(req.References) > 0 {
		agentMessage = appendReferenceContext(bookService, agentMessage, req.References, contextLog)
	}
	if len(req.LoreReferences) > 0 {
		agentMessage = appendLoreReferenceContext(bookService, agentMessage, req.LoreReferences, contextLog)
	}
	if len(req.Selections) > 0 {
		agentMessage = appendSelectionContext(agentMessage, req.Selections)
		contextLog.addSelections(req.Selections)
	}
	agentMessage = appendContextBoundaryInstruction(agentMessage)
	contextLog.add("注入规则", "上下文边界", "[上下文边界] 当前用户请求是“这次要做什么”", "")
	return agentInputComposition{
		OriginalMessage:    originalMessage,
		Request:            req,
		AgentMessage:       agentMessage,
		ContextLog:         contextLog,
		ResumeInterruption: resumeInterruption,
	}
}
