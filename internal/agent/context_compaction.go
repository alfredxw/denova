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
	contextCompactionMaxAttempts   = 2
)

type contextCompactionPolicy struct {
	AgentKind           string
	Enabled             bool
	ContextWindowTokens int
	Threshold           float64
	RetainedRecentTurns int
	TargetMinRatio      float64
	TargetMaxRatio      float64
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
	compactionSettings := config.ResolveAgentContext(cfg, config.AgentKindContextCompaction)
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return contextCompactionPolicy{
		AgentKind:           agentKind,
		Enabled:             contextSettings.CompactionEnabled,
		ContextWindowTokens: modelSettings.ContextWindowTokens,
		Threshold:           contextSettings.CompactionThreshold,
		RetainedRecentTurns: compactionSettings.CompactionRecentTurns,
		TargetMinRatio:      compactionSettings.CompactionTargetMin,
		TargetMaxRatio:      compactionSettings.CompactionTargetMax,
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
	maxTokens := contextCompactionSummaryMaxTokens(sourceTokens, policy.ContextWindowTokens, policy.TargetMaxRatio)
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
			schema.UserMessage(buildContextCompactionTranscript(source, referenceContext, sourceTokens, retryReason, policy)),
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
		if ratio >= policy.TargetMinRatio && ratio <= policy.TargetMaxRatio {
			return summary, nil
		}
		if attempt == contextCompactionMaxAttempts {
			break
		}
		if ratio > policy.TargetMaxRatio {
			retryReason = fmt.Sprintf("The previous summary was too long: %.1f%% of source tokens. Compress it to %s while preserving required facts.", ratio*100, compactionTargetRange(policy))
		} else {
			retryReason = fmt.Sprintf("The previous summary was too short: %.1f%% of source tokens. Expand it to %s by restoring omitted user goals, events, relationships, tasks, and state changes.", ratio*100, compactionTargetRange(policy))
		}
	}
	return summary, nil
}

func contextCompactionSummaryMaxTokens(sourceTokens, contextWindowTokens int, targetMaxRatio float64) int {
	if sourceTokens <= 0 {
		sourceTokens = contextWindowTokens
	}
	if sourceTokens <= 0 {
		return 6000
	}
	if targetMaxRatio <= 0 {
		targetMaxRatio = 0.20
	}
	target := int(float64(sourceTokens) * targetMaxRatio)
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

func compactionTargetRange(policy contextCompactionPolicy) string {
	minRatio := policy.TargetMinRatio
	if minRatio <= 0 {
		minRatio = 0.05
	}
	maxRatio := policy.TargetMaxRatio
	if maxRatio <= 0 {
		maxRatio = 0.20
	}
	if maxRatio < minRatio {
		maxRatio = minRatio
	}
	return fmt.Sprintf("%.0f%%-%.0f%%", minRatio*100, maxRatio*100)
}

func contextCompactionSystemInstruction() string {
	return strings.TrimSpace(`
你是 Nova 的独立“互动小说上下文压缩器”，用于类似酒馆/SillyTavern 的高轮次互动小说和长对话创作场景。

你的任务是把输入上下文压缩成更精简的“事件时间线记忆”，同时保留所有会对后续剧情、写作任务或用户意图产生长期影响的信息。

输入可能包含：
1. existing_memory：此前已经压缩过的记忆，可能为空。
2. reference_context：有界参考上下文，例如 Story Memory。互动模式必须参考其中的故事记忆，尤其 plot_summary / 剧情纪要。
3. new_context：需要压缩的原始有效对话链或互动回合链，包括用户行动、用户对白、LLM 剧情推进、NPC 反应、环境变化、任务状态等。

处理目标：
- 将 existing_memory 与 new_context 合并，输出一份新的压缩记忆。
- 如果 existing_memory 为空，则从 new_context 初始化压缩记忆。
- 如果 existing_memory 不为空，则合并新信息；不要重复记录同一事件，新信息补充旧事件时应更新旧事件。
- 不要删除旧记忆中的长期影响信息，除非 new_context 明确说明该信息已经失效、解决或被推翻。
- 如果出现矛盾，不要自行修正；保留矛盾并标记为“待确认矛盾”。
- 已完成任务可以压缩，但必须保留最终结果和遗留影响。
- 未完成任务、伏笔、承诺、债务、秘密、危险不能删除。
- 如果不确定某信息是否有长期影响，默认保留。

压缩重点：
- 必须保留事件时间顺序。
- 必须保留所有用户消息的核心意图、关键行动、选择、对白、承诺、拒绝、欺骗、威胁、安慰、交易、背叛、失败尝试及其后果。
- 必须保留行动造成的后果和所有长期影响信息。
- 必须保留角色关系、角色状态、世界/阵营状态、物品资源、能力、线索、秘密、伏笔、任务、危险、倒计时和当前阶段信息。
- 可以删除或合并氛围描写、重复心理描写、无后果闲聊、纯修辞性文本。
- 不要写成小说文风；要写成清晰、紧凑、可供后续模型继续创作的事实账本。
- 排除 thinking/reasoning 内容、传输噪音、展示用日志、重复工具卡片和无结果的实现过程，除非其结果会改变后续行为。
- 禁止编造事实；不确定时明确标记“不确定”。
- 目标长度由用户消息配置，默认是源上下文的 5%-20%。信息密度高时使用目标范围的上半区，不要为了短而丢长期影响信息。

长期影响信息判定：
只要某条信息未来可能影响角色反应、剧情分支、世界状态、任务推进、关系变化或玩家可用选择，就必须保留。以下信息一律视为长期影响信息：
- 用户行动：关键选择、重要话语、承诺/拒绝/欺骗/威胁/安慰/交易/背叛、失败的重要行动、尚未显现的后果。
- 角色关系：信任、好感、敌意、怀疑、依赖、恐惧、暧昧、愧疚、承诺、误会、秘密、冲突、债务、交易、NPC 间联盟/敌对/背叛/隐瞒。
- 角色状态：受伤、死亡、失踪、昏迷、被俘、身份暴露、能力觉醒/削弱、诅咒、污染、精神状态变化、已知/未知/误解的信息、目标/动机/立场变化。
- 世界与阵营状态：地点被破坏/封锁/占领/发现/改变、阵营态度变化、组织行动、通缉、追捕、战争、政治变化、世界规则、禁忌、异常现象、公共事件。
- 物品、资源与能力：获得、失去、损坏、使用、隐藏的重要物品；金钱、补给、武器、钥匙、信物、证据、药物、装备；技能、权限、身份、通行资格变化。
- 线索、秘密与伏笔：已发现线索、未解谜团、未兑现威胁、未完成任务、倒计时事件、约定地点/时间/暗号、隐藏身份/目的/计划、叙事确认但角色未必知道的信息。
- 当前阶段：当前地点、时间/阶段、在场角色、主角状态、NPC 态度、当前目标、危险、限制、用户最后行动、LLM 最后反馈、剧情停顿点、下一轮应从哪里继续。

输出必须使用以下格式：

【事件时间线】
- [重要级别][事件类型] 事件描述
  - 触发：谁做了什么
  - 结果：造成了什么变化
  - 长期影响：为什么后续必须记住
  - 相关实体：角色 / 地点 / 物品 / 阵营 / 线索

重要级别：
- P0：永久核心事实。世界观、角色身份、主线设定、绝对不能丢。
- P1：长期影响事件。会影响后续剧情、关系、任务或世界状态，必须保留。
- P2：当前阶段重要信息。影响当前场景连续性，必须保留。
- P3：低影响信息。只在必要时合并，不单独展开。

事件类型可选：
用户行动 / 剧情后果 / 角色关系 / 角色状态 / 世界状态 / 物品资源 / 任务目标 / 线索伏笔 / 冲突危险 / 当前阶段 / 待确认矛盾

【长期影响账本】
- 角色关系：
- 角色状态：
- 世界/阵营状态：
- 物品/资源/能力：
- 线索/秘密/伏笔：
- 未闭环事项：

【当前阶段快照】
- 当前地点：
- 当前时间/阶段：
- 当前在场角色：
- 主角当前状态：
- NPC当前态度：
- 当前目标：
- 当前危险：
- 当前限制：
- 用户最后行动：
- LLM最后反馈：
- 剧情停顿点：
- 下一轮应从哪里继续：

【已合并或舍弃的信息】
简要说明哪些信息被合并或舍弃，以及原因。只允许舍弃无长期影响、无当前阶段影响的信息。
	`)
}

func buildContextCompactionTranscript(messages []*schema.Message, referenceContext string, sourceTokens int, retryInstruction string, policy contextCompactionPolicy) string {
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
	sb.WriteString("请按系统要求压缩以下 Nova 上下文。保留所有会影响后续剧情、任务、关系、世界状态或用户偏好的信息。\n")
	sb.WriteString(fmt.Sprintf("Estimated source tokens: %d. Target summary length: %s of source tokens. 信息密度高时使用目标范围上半区。\n\n", sourceTokens, compactionTargetRange(policy)))
	if retryInstruction = strings.TrimSpace(retryInstruction); retryInstruction != "" {
		sb.WriteString("Retry instruction:\n")
		sb.WriteString(retryInstruction)
		sb.WriteString("\n\n")
	}
	sb.WriteString("<existing_memory>\n")
	sb.WriteString("（未提供；本次输入从原始有效上下文重新生成压缩记忆。）\n")
	sb.WriteString("</existing_memory>\n\n")
	if referenceContext = strings.TrimSpace(referenceContext); referenceContext != "" {
		sb.WriteString("<reference_context>\n")
		sb.WriteString(referenceContext)
		sb.WriteString("\n</reference_context>\n\n")
	}
	sb.WriteString("<new_context>\n")
	for i := len(blocks) - 1; i >= 0; i-- {
		sb.WriteString(blocks[i])
	}
	sb.WriteString("</new_context>\n")
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
