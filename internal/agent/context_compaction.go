package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/observability"
	"nova/internal/session"
)

const (
	contextCompactionPhasePreRun = "pre_run"
	contextCompactionPhaseMidRun = "mid_run"
	contextCompactionReasonLimit = "context_usage_threshold"

	contextCompactionSummaryPrefix = "[Nova Context Compaction]"
	contextCompactionMaxInputBytes = 1024 * 1024
	contextCompactionTargetMinRatio = 0.05
	contextCompactionTargetMaxRatio = 0.20
	contextCompactionMaxAttempts = 2
)

type contextCompactionPolicy struct {
	AgentKind           string
	Enabled             bool
	ContextWindowTokens int
	Threshold           float64
	RetainedRecentTurns int
}

type ContextCompactionResult struct {
	Triggered           bool
	SkippedReason       string
	Phase               string
	TokensBefore        int
	TokensAfter         int
	ContextWindowTokens int
	Threshold           float64
	Epoch               int
	Summary             string
	TargetRatio         float64
	SourceMessageCount  int
	MessageCountBefore  int
	MessageCountAfter   int
}

type contextCompactionController struct {
	conversation ContextCompactionConversation
}

// ContextCompactionConversation is implemented by conversations that can
// persist and rebuild model-visible compaction epochs.
type ContextCompactionConversation interface {
	CompactContextIfNeeded(ctx context.Context, input ContextCompactionInput) ([]*schema.Message, ContextCompactionResult, error)
}

type ContextCompactionInput struct {
	Messages            []*schema.Message
	SourceMessages      []*schema.Message
	Tools               []*schema.ToolInfo
	AgentMessage        string
	Phase               string
	Emit                func(Event)
	Force               bool
	ContextWindowTokens int
	ReferenceContext    string
	KeepLatestUser      bool
}

type contextCompactionContextKey struct{}

var summarizeContextForCompaction = generateContextCompactionSummary

func contextWithCompactionController(ctx context.Context, conversation Conversation) context.Context {
	compaction, ok := conversation.(ContextCompactionConversation)
	if !ok || compaction == nil {
		return ctx
	}
	return context.WithValue(ctx, contextCompactionContextKey{}, &contextCompactionController{conversation: compaction})
}

func compactionControllerFromContext(ctx context.Context) *contextCompactionController {
	controller, _ := ctx.Value(contextCompactionContextKey{}).(*contextCompactionController)
	return controller
}

func resolveContextCompactionPolicy(cfg *config.Config, agentKind string) contextCompactionPolicy {
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return contextCompactionPolicy{
		AgentKind:           agentKind,
		Enabled:             contextSettings.CompactionEnabled,
		ContextWindowTokens: modelSettings.ContextWindowTokens,
		Threshold:           contextSettings.CompactionThreshold,
		RetainedRecentTurns: contextSettings.CompactionRecentTurns,
	}
}

func (p contextCompactionPolicy) triggerTokens() int {
	if !p.Enabled || p.ContextWindowTokens <= 0 || p.Threshold <= 0 {
		return 0
	}
	return int(float64(p.ContextWindowTokens) * p.Threshold)
}

func (p contextCompactionPolicy) shouldCompact(tokens int, force bool) (bool, string) {
	if force {
		return true, ""
	}
	if !p.Enabled {
		return false, "disabled"
	}
	if p.ContextWindowTokens <= 0 {
		return false, "context_window_tokens_missing"
	}
	trigger := p.triggerTokens()
	if trigger <= 0 {
		return false, "threshold_invalid"
	}
	if tokens < trigger {
		return false, "below_threshold"
	}
	return true, ""
}

func BuildContextCompaction(ctx context.Context, cfg *config.Config, agentKind string, input ContextCompactionInput, epoch int) ([]*schema.Message, ContextCompactionResult, error) {
	policy := resolveContextCompactionPolicy(cfg, agentKind)
	if input.ContextWindowTokens > 0 {
		policy.ContextWindowTokens = input.ContextWindowTokens
	}
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = contextCompactionPhasePreRun
	}
	tokensBefore := EstimateContextTokens(input.Messages, input.Tools)
	result := ContextCompactionResult{
		Phase:               phase,
		TokensBefore:        tokensBefore,
		ContextWindowTokens: policy.ContextWindowTokens,
		Threshold:           policy.Threshold,
		MessageCountBefore:  len(input.Messages),
	}
	shouldCompact, skipped := policy.shouldCompact(tokensBefore, input.Force)
	if !shouldCompact {
		result.SkippedReason = skipped
		return input.Messages, result, nil
	}
	source := compactionSourceMessages(compactionSourceBaseMessages(input), input.KeepLatestUser)
	if len(source) == 0 {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	sourceTokens := EstimateContextTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	summary, err := summarizeContextForCompaction(ctx, cfg, agentKind, source, input.ReferenceContext, sourceTokens, policy)
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	if epoch <= 0 {
		epoch = 1
	}
	newMessages := compactMessagesForModel(input.Messages, summary, epoch, policy.RetainedRecentTurns)
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = summary
	result.TokensAfter = EstimateContextTokens(newMessages, input.Tools)
	result.TargetRatio = contextCompactionRatio(estimateStringTokens(summary), sourceTokens)
	result.SourceMessageCount = len(source)
	result.MessageCountAfter = len(newMessages)
	emitContextCompactionEvent(input.Emit, phase, "completed", result)
	return newMessages, result, nil
}

func compactionSourceBaseMessages(input ContextCompactionInput) []*schema.Message {
	if len(input.SourceMessages) > 0 {
		return input.SourceMessages
	}
	return input.Messages
}

func EstimateContextTokens(messages []*schema.Message, tools []*schema.ToolInfo) int {
	tokens := 0
	for _, msg := range messages {
		tokens += estimateMessageTokens(msg)
	}
	if len(tools) > 0 {
		data, err := json.Marshal(tools)
		if err == nil {
			tokens += estimateStringTokens(string(data))
		} else {
			tokens += len(tools) * 128
		}
	}
	if tokens < 1 {
		return 1
	}
	return tokens
}

func estimateMessageTokens(msg *schema.Message) int {
	if msg == nil {
		return 0
	}
	tokens := 4 + estimateStringTokens(string(msg.Role)) + estimateStringTokens(msg.Content)
	tokens += estimateStringTokens(msg.ReasoningContent)
	if len(msg.ToolCalls) > 0 {
		if data, err := json.Marshal(msg.ToolCalls); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if len(msg.MultiContent) > 0 {
		if data, err := json.Marshal(msg.MultiContent); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if len(msg.UserInputMultiContent) > 0 {
		if data, err := json.Marshal(msg.UserInputMultiContent); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if len(msg.AssistantGenMultiContent) > 0 {
		if data, err := json.Marshal(msg.AssistantGenMultiContent); err == nil {
			tokens += estimateStringTokens(string(data))
		}
	}
	if msg.ToolName != "" {
		tokens += estimateStringTokens(msg.ToolName)
	}
	if msg.ToolCallID != "" {
		tokens += estimateStringTokens(msg.ToolCallID)
	}
	return tokens
}

func estimateStringTokens(content string) int {
	if content == "" {
		return 0
	}
	tokens := 0
	asciiRunes := 0
	flushASCII := func() {
		if asciiRunes == 0 {
			return
		}
		tokens += (asciiRunes + 3) / 4
		asciiRunes = 0
	}
	for _, r := range content {
		if r <= unicode.MaxASCII {
			asciiRunes++
			continue
		}
		flushASCII()
		tokens++
	}
	flushASCII()
	if tokens < 1 {
		return 1
	}
	return tokens
}

func NewContextCompactionSummaryMessage(epoch int, summary string) *schema.Message {
	return schema.UserMessage(fmt.Sprintf("%s epoch=%d\n\n%s", contextCompactionSummaryPrefix, epoch, strings.TrimSpace(summary)))
}

func isContextCompactionMessage(msg *schema.Message) bool {
	return msg != nil && strings.HasPrefix(strings.TrimSpace(msg.Content), contextCompactionSummaryPrefix)
}

func compactMessagesForModel(messages []*schema.Message, summary string, epoch, retainedTurns int) []*schema.Message {
	systemMessages := make([]*schema.Message, 0)
	contextMessages := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		if msg == nil || isContextCompactionMessage(msg) {
			continue
		}
		if msg.Role == schema.System {
			systemMessages = append(systemMessages, msg)
			continue
		}
		contextMessages = append(contextMessages, msg)
	}
	tail := limitMessagesByRecentTurns(contextMessages, retainedTurns)
	result := make([]*schema.Message, 0, len(systemMessages)+1+len(tail))
	result = append(result, systemMessages...)
	result = append(result, NewContextCompactionSummaryMessage(epoch, summary))
	result = append(result, tail...)
	return result
}

// BuildCompactedModelMessages rebuilds model-visible history after a compaction
// record is persisted and its final epoch is known.
func BuildCompactedModelMessages(messages []*schema.Message, summary string, epoch, retainedTurns int) []*schema.Message {
	return compactMessagesForModel(messages, summary, epoch, retainedTurns)
}

func generateContextCompactionSummary(ctx context.Context, cfg *config.Config, agentKind string, source []*schema.Message, referenceContext string, sourceTokens int, policy contextCompactionPolicy) (string, error) {
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindContextCompaction)
	maxTokens := contextCompactionSummaryMaxTokens(sourceTokens, policy.ContextWindowTokens)
	modelCfg.MaxTokens = &maxTokens
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建上下文压缩模型失败: %w", err)
	}
	systemPrompt := protectedSystemInstruction(cfg, config.AgentKindContextCompaction, contextCompactionSystemInstruction())
	var summary string
	var retryReason string
	for attempt := 1; attempt <= contextCompactionMaxAttempts; attempt++ {
		input := []*schema.Message{
			schema.SystemMessage(systemPrompt),
			schema.UserMessage(buildContextCompactionTranscript(source, referenceContext, sourceTokens, retryReason)),
		}
		msg, err := cm.Generate(ctx, input)
		if err != nil {
			return "", fmt.Errorf("上下文压缩失败: %w", err)
		}
		summary = strings.TrimSpace(msg.Content)
		if summary == "" {
			return "", fmt.Errorf("上下文压缩结果为空")
		}
		ratio := contextCompactionRatio(estimateStringTokens(summary), sourceTokens)
		if ratio >= contextCompactionTargetMinRatio && ratio <= contextCompactionTargetMaxRatio {
			return summary, nil
		}
		if attempt == contextCompactionMaxAttempts {
			break
		}
		if ratio > contextCompactionTargetMaxRatio {
			retryReason = fmt.Sprintf("The previous summary was too long: %.1f%% of source tokens. Compress it to 5%%-20%% while preserving required facts.", ratio*100)
		} else {
			retryReason = fmt.Sprintf("The previous summary was too short: %.1f%% of source tokens. Expand it to 5%%-20%% by restoring omitted user goals, events, relationships, tasks, and state changes.", ratio*100)
		}
	}
	return summary, nil
}

func contextCompactionSummaryMaxTokens(sourceTokens, contextWindowTokens int) int {
	if sourceTokens <= 0 {
		sourceTokens = contextWindowTokens
	}
	if sourceTokens <= 0 {
		return 6000
	}
	target := int(float64(sourceTokens) * contextCompactionTargetMaxRatio)
	if target < 128 {
		target = 128
	}
	if contextWindowTokens > 0 && target > contextWindowTokens/4 {
		target = contextWindowTokens / 4
	}
	if target > 24000 {
		target = 24000
	}
	return target
}

func contextCompactionRatio(partTokens, sourceTokens int) float64 {
	if sourceTokens <= 0 {
		return 0
	}
	return float64(partTokens) / float64(sourceTokens)
}

func contextCompactionSystemInstruction() string {
	return strings.TrimSpace(`
You are Nova's independent context compaction Agent. Compress prior model-visible conversation context for a future writing or interactive-story turn.

Rules:
- Preserve every user message's core intent in order. Do not drop user constraints, corrections, preferences, or rejected directions.
- Preserve all important events, important changes, character relationships, story events, tasks, unresolved decisions, commitments, files/resources mentioned, and state changes.
- For interactive stories, use the provided Story Memory reference as authoritative continuity context, especially plot_summary records, current state, important characters/relationships, and open threads.
- Exclude thinking/reasoning content, transport noise, display-only logs, repeated tool cards, and implementation chatter unless the outcome changes future behavior.
- Do not invent facts. If something is uncertain, mark it as uncertain.
- Target length: 5% to 20% of the source context. Prefer dense Markdown over prose padding.
- Output concise Markdown with these exact sections:
  User Messages and Intent
  Key Decisions and Constraints
  Workspace or Story State
  Important Events and State Changes
  Character Relationships and Open Threads
  Completed Work
  Pending Next Steps
  Important Evidence and Files
  Risks and Uncertainties
	`)
}

func buildContextCompactionTranscript(messages []*schema.Message, referenceContext string, sourceTokens int, retryInstruction string) string {
	remaining := contextCompactionMaxInputBytes
	omitted := 0
	blocks := make([]string, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg == nil {
			continue
		}
		block := formatCompactionMessage(i+1, msg)
		if len(block) > remaining {
			omitted = i + 1
			break
		}
		remaining -= len(block)
		blocks = append(blocks, block)
	}
	var sb strings.Builder
	sb.WriteString("Compress the following Nova transcript. Keep only information needed for future turns.\n")
	sb.WriteString(fmt.Sprintf("Estimated source tokens: %d. Target summary length: %.0f%%-%.0f%% of source tokens.\n\n", sourceTokens, contextCompactionTargetMinRatio*100, contextCompactionTargetMaxRatio*100))
	if retryInstruction = strings.TrimSpace(retryInstruction); retryInstruction != "" {
		sb.WriteString("Retry instruction:\n")
		sb.WriteString(retryInstruction)
		sb.WriteString("\n\n")
	}
	if referenceContext = strings.TrimSpace(referenceContext); referenceContext != "" {
		sb.WriteString("Reference context. Use it to preserve continuity, but do not copy unrelated noise:\n\n")
		sb.WriteString(referenceContext)
		sb.WriteString("\n\n")
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		sb.WriteString(blocks[i])
	}
	transcript := sb.String()
	if omitted > 0 {
		transcript = fmt.Sprintf("Older %d messages were omitted to keep compaction input bounded.\n\n%s", omitted, transcript)
	}
	return transcript
}

func formatCompactionMessage(index int, msg *schema.Message) string {
	role := string(msg.Role)
	content := strings.TrimSpace(msg.Content)
	if len(msg.ToolCalls) > 0 {
		data, _ := json.Marshal(msg.ToolCalls)
		content = strings.TrimSpace(content + "\nTool calls: " + string(data))
	}
	if msg.ToolName != "" {
		content = strings.TrimSpace(fmt.Sprintf("tool=%s call_id=%s\n%s", msg.ToolName, msg.ToolCallID, content))
	}
	return fmt.Sprintf("\n--- message %d role=%s ---\n%s\n", index, role, content)
}

func emitContextCompactionEvent(emit func(Event), phase, status string, result ContextCompactionResult) {
	if emit == nil {
		return
	}
	emit(Event{Type: "context_compaction", Data: map[string]any{
		"phase":                 phase,
		"status":                status,
		"tokens_before":         result.TokensBefore,
		"tokens_after":          result.TokensAfter,
		"context_window_tokens": result.ContextWindowTokens,
		"threshold":             result.Threshold,
		"target_ratio":          result.TargetRatio,
		"epoch":                 result.Epoch,
		"source_message_count":  result.SourceMessageCount,
		"message_count_before":  result.MessageCountBefore,
		"message_count_after":   result.MessageCountAfter,
		"skipped_reason":        result.SkippedReason,
	}})
}

type contextCompactionMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	agentKind string
}

func (m *contextCompactionMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.ChatModelAgentState, _ *adk.ModelContext) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil {
		return ctx, state, nil
	}
	controller := compactionControllerFromContext(ctx)
	if controller == nil || controller.conversation == nil {
		return ctx, state, nil
	}
	messages := append([]*schema.Message(nil), state.Messages...)
	newMessages, result, err := controller.conversation.CompactContextIfNeeded(ctx, ContextCompactionInput{
		Messages: messages,
		Tools:    state.ToolInfos,
		Phase:    contextCompactionPhaseMidRun,
	})
	if err != nil {
		observability.Logger("agent-run").Warn("mid_run_context_compaction_failed", slog.String("agent_kind", m.agentKind), slog.Any("error", err))
		return ctx, state, nil
	}
	if !result.Triggered {
		return ctx, state, nil
	}
	next := *state
	next.Messages = newMessages
	return ctx, &next, nil
}

type contextCompactionUsage struct {
	PromptTokens           int `json:"prompt_tokens,omitempty"`
	CachedPromptTokens     int `json:"cached_prompt_tokens,omitempty"`
	CompletionTokens       int `json:"completion_tokens,omitempty"`
	ReasoningTokens        int `json:"reasoning_tokens,omitempty"`
	TotalTokens            int `json:"total_tokens,omitempty"`
	ContextWindowTokens    int `json:"context_window_tokens,omitempty"`
	EstimatedContextTokens int `json:"estimated_context_tokens,omitempty"`
}

func usageFromMessage(msg *schema.Message, estimated, contextWindow int) (contextCompactionUsage, bool) {
	usage := contextCompactionUsage{EstimatedContextTokens: estimated, ContextWindowTokens: contextWindow}
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return usage, estimated > 0 || contextWindow > 0
	}
	tokenUsage := msg.ResponseMeta.Usage
	usage.PromptTokens = tokenUsage.PromptTokens
	usage.CachedPromptTokens = tokenUsage.PromptTokenDetails.CachedTokens
	usage.CompletionTokens = tokenUsage.CompletionTokens
	usage.ReasoningTokens = tokenUsage.CompletionTokensDetails.ReasoningTokens
	usage.TotalTokens = tokenUsage.TotalTokens
	return usage, true
}

func contextCompactionRecordFromResult(result ContextCompactionResult, agentKind string, sourceStart, sourceEnd, retainedTurns int, summary string) session.ContextCompaction {
	return session.ContextCompaction{
		Type:                "context_compaction",
		AgentKind:           agentKind,
		Epoch:               result.Epoch,
		Summary:             summary,
		SourceStartIndex:    sourceStart,
		SourceEndIndex:      sourceEnd,
		SourceMessageCount:  sourceEnd - sourceStart,
		RetainedTurns:       retainedTurns,
		TokensBefore:        result.TokensBefore,
		TokensAfter:         result.TokensAfter,
		TargetRatio:         result.TargetRatio,
		ContextWindowTokens: result.ContextWindowTokens,
		Threshold:           result.Threshold,
		Reason:              contextCompactionReasonLimit,
		Phase:               result.Phase,
		CreatedAt:           time.Now().UTC(),
	}
}
