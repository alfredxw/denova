package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/model/openai"

	"denova/config"
)

const (
	contextCompactionPhasePreRun = "pre_run"
	contextCompactionPhaseMidRun = "mid_run"
	contextCompactionReasonLimit = "context_usage_threshold"

	contextCompactionSummaryPrefix = "[Denova Context Compaction]"
)

type contextCompactionPolicy struct {
	AgentKind           string
	Enabled             bool
	Strategy            string
	ContextWindowTokens int
	Threshold           float64
	RetainedTurns       int
	TargetMinRatio      float64
	TargetMaxRatio      float64
}

type ContextCompactionResult struct {
	Triggered                bool
	SkippedReason            string
	Phase                    string
	EstimatedTokensBefore    int
	ObservedPromptTokens     int
	ObservedEstimateTokens   int
	TokensBefore             int
	TokensAfter              int
	ProjectedTokensBefore    int
	ProjectedTokensAfter     int
	ReservedCompletionTokens int
	ReservedToolResultTokens int
	ContextWindowTokens      int
	Strategy                 string
	Threshold                float64
	Epoch                    int
	Summary                  string
	TargetRatio              float64
	SourceMessageCount       int
	MessageCountBefore       int
	MessageCountAfter        int
	RetainedTurns            int
}

type contextCompactionSummaryFunc func(ctx context.Context, cfg *config.Config, agentKind string, existingCheckpoint string, source []*agent.Message, referenceContext string, sourceTokens int, policy contextCompactionPolicy, emitDelta func(attempt int, delta string)) (string, int, error)

type contextCompactionController struct {
	conversation ContextCompactionConversation
}

// ContextCompactionConversation is implemented by conversations that can
// persist and rebuild model-visible compaction epochs.
type ContextCompactionConversation interface {
	CompactContextIfNeeded(ctx context.Context, input ContextCompactionInput) ([]*agent.Message, ContextCompactionResult, error)
}

type ContextCompactionInput struct {
	Messages            []*agent.Message
	SourceMessages      []*agent.Message
	Tools               []*agent.ToolInfo
	Phase               string
	Emit                func(Event)
	Force               bool
	ExistingCheckpoint  string
	ContextWindowTokens int
	// ObservedPromptTokens is exact provider usage for the previous request.
	// ObservedEstimateTokens is the local estimate of that same request; the
	// pair calibrates, but never replaces, projection of the next request.
	ObservedPromptTokens   int
	ObservedEstimateTokens int
	// ReservedCompletionTokens and ReservedToolResultTokens make compaction
	// decisions against projected context usage, not only the prompt assembled
	// before the next model/tool step.
	ReservedCompletionTokens int
	ReservedToolResultTokens int
	ReferenceContext         string
	KeepLatestUser           bool
}

type contextCompactionContextKey struct{}

var summarizeContextForCompaction contextCompactionSummaryFunc = generateContextCompactionSummary

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
		Strategy:            contextSettings.CompactionStrategy,
		ContextWindowTokens: modelSettings.ContextWindowTokens,
		Threshold:           contextSettings.CompactionThreshold,
		RetainedTurns:       compactionSettings.CompactionRecentTurns,
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

// PrepareContextCompaction performs bounded policy evaluation and summary
// generation without mutating Session or Story storage. Canonical publication
// belongs to a durable structural command's Commit phase.
func PrepareContextCompaction(ctx context.Context, cfg *config.Config, agentKind string, input ContextCompactionInput, epoch int) ([]*agent.Message, ContextCompactionResult, error) {
	policy := resolveContextCompactionPolicy(cfg, agentKind)
	if input.ContextWindowTokens > 0 {
		policy.ContextWindowTokens = input.ContextWindowTokens
	}
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = contextCompactionPhasePreRun
	}
	input = withDefaultContextProjectionReserves(cfg, agentKind, input, 0)
	estimatedTokensBefore := EstimateContextTokens(input.Messages, input.Tools)
	tokensBefore := calibratedContextTokens(estimatedTokensBefore, input)
	projectedTokensBefore := projectedContextTokens(tokensBefore, input)
	result := ContextCompactionResult{
		Phase:                    phase,
		EstimatedTokensBefore:    estimatedTokensBefore,
		ObservedPromptTokens:     input.ObservedPromptTokens,
		ObservedEstimateTokens:   input.ObservedEstimateTokens,
		TokensBefore:             tokensBefore,
		ProjectedTokensBefore:    projectedTokensBefore,
		ReservedCompletionTokens: input.ReservedCompletionTokens,
		ReservedToolResultTokens: input.ReservedToolResultTokens,
		ContextWindowTokens:      policy.ContextWindowTokens,
		Strategy:                 policy.Strategy,
		Threshold:                policy.Threshold,
		MessageCountBefore:       len(input.Messages),
		RetainedTurns:            policy.RetainedTurns,
	}
	shouldCompact, skipped := policy.shouldCompact(projectedTokensBefore, input.Force)
	if !shouldCompact {
		result.SkippedReason = skipped
		return input.Messages, result, nil
	}
	source := compactionSourceMessages(compactionSourceBaseMessages(input), input.KeepLatestUser)
	if len(source) == 0 && strings.TrimSpace(input.ExistingCheckpoint) == "" && strings.TrimSpace(input.ReferenceContext) == "" {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	sourceTokens := EstimateContextTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	summary, inputChars, err := summarizeContextInLayers(ctx, cfg, agentKind, input.ExistingCheckpoint, source, input.ReferenceContext, sourceTokens, policy, func(attempt int, delta string) {
		emitContextCompactionDeltaEvent(input.Emit, phase, result, attempt, delta)
	})
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	if epoch <= 0 {
		epoch = 1
	}
	newMessages := compactMessagesForModel(input.Messages, summary, epoch, policy.RetainedTurns)
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = summary
	result.TokensAfter = calibratedContextTokens(EstimateContextTokens(newMessages, input.Tools), input)
	result.ProjectedTokensAfter = projectedContextTokens(result.TokensAfter, input)
	result.TargetRatio = contextCompactionRatio(countRunes(summary), inputChars)
	result.SourceMessageCount = len(source)
	result.MessageCountAfter = len(newMessages)
	emitContextCompactionEvent(input.Emit, phase, "completed", result)
	return newMessages, result, nil
}

func generateContextCompactionSummary(ctx context.Context, cfg *config.Config, agentKind string, existingCheckpoint string, source []*agent.Message, referenceContext string, sourceTokens int, policy contextCompactionPolicy, emitDelta func(attempt int, delta string)) (string, int, error) {
	var runErr error
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, config.AgentKindContextCompaction, "context_compaction", "generate", map[string]any{
		"source_agent_kind": strings.TrimSpace(agentKind),
		"source_messages":   len(source),
		"source_tokens":     sourceTokens,
	})
	defer func() { finishTrace(runErr) }()
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindContextCompaction)
	inputChars := contextCompactionInputChars(existingCheckpoint, source, referenceContext)
	cm, err := openai.New(traceCtx, &modelCfg)
	if err != nil {
		runErr = err
		return "", inputChars, fmt.Errorf("创建上下文压缩模型失败: %w", err)
	}
	composition, err := composeBuiltinSystemInstruction(cfg, config.AgentKindContextCompaction, "context_compaction", cfg.Workspace, "builtin_base", "上下文压缩规则", "define the bounded context compaction task", contextCompactionSystemInstruction())
	if err != nil {
		runErr = err
		return "", inputChars, err
	}
	input := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(buildContextCompactionTranscript(source, existingCheckpoint, referenceContext, sourceTokens, inputChars, policy)),
	}
	resolvedContext := config.ResolveAgentContext(cfg, config.AgentKindContextCompaction)
	contextWindow := config.ResolveAgentModel(cfg, config.AgentKindContextCompaction).ContextWindowTokens
	if err := validateProviderInput(config.AgentKindContextCompaction, input, nil, resolvedContext.MaxProviderInputBytes, contextWindow); err != nil {
		runErr = err
		return "", inputChars, err
	}
	// The target ratio is a prompt contract and post-run quality metric. Do not
	// hide a bounded retry loop here: it duplicates provider cost, can still
	// discard a fact-dense valid summary, and turns a configured Agent run into
	// an unrelated hard-coded iteration policy.
	const attempt = 1
	const mode = "stream"
	span, callID, llmTraceCtx := beginLLMCallTrace(traceCtx, config.AgentKindContextCompaction, "context_compaction", mode, modelCfg, input, nil, true)
	msg, err := streamContextCompactionAttempt(llmTraceCtx, cm, input, attempt, emitDelta)
	if err != nil {
		finishLLMCallTrace(span, callID, config.AgentKindContextCompaction, "context_compaction", mode, modelCfg.Model, attempt, nil, err, nil)
		runErr = err
		return "", inputChars, fmt.Errorf("上下文压缩失败: %w", err)
	}
	finishLLMCallTrace(span, callID, config.AgentKindContextCompaction, "context_compaction", mode, modelCfg.Model, attempt, msg, nil, nil)
	summary := strings.TrimSpace(msg.Content)
	if summary == "" {
		runErr = fmt.Errorf("上下文压缩结果为空")
		return "", inputChars, runErr
	}
	return summary, inputChars, nil
}

func streamContextCompactionAttempt(ctx context.Context, cm *openai.ChatModel, input []*agent.Message, attempt int, emitDelta func(attempt int, delta string)) (*agent.Message, error) {
	stream, err := cm.Stream(ctx, input)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	var chunks []*agent.Message
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if msg == nil {
			continue
		}
		chunks = append(chunks, msg)
		if msg.Content != "" && emitDelta != nil {
			emitDelta(attempt, msg.Content)
		}
	}
	return agent.ConcatMessages(chunks)
}

func contextCompactionRatio(partChars, inputChars int) float64 {
	if inputChars <= 0 {
		return 0
	}
	return float64(partChars) / float64(inputChars)
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

func compactionTargetCharRange(inputChars int, policy contextCompactionPolicy) (int, int) {
	if inputChars <= 0 {
		return 0, 0
	}
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
	minChars := int(float64(inputChars)*minRatio + 0.5)
	maxChars := int(float64(inputChars)*maxRatio + 0.5)
	if minChars < 1 {
		minChars = 1
	}
	if maxChars < minChars {
		maxChars = minChars
	}
	return minChars, maxChars
}

func contextCompactionSystemInstruction() string {
	return strings.TrimSpace(`
你是 Denova 的独立“上下文 checkpoint 编译器”。输入会明确给出 source_agent_kind；你必须按来源模式生成可继续工作的增量 checkpoint，而不是写一段泛化摘要。

输入边界：
1. existing_checkpoint：同一来源链的旧 checkpoint，可能为空。
2. reference_context：调用点显式提供的有界参考，可能为空。
3. new_context：旧 checkpoint 之后新增的有效消息与稳定 tool receipt。
不得假定存在未提供的记忆；checkpoint 不是新的事实真源，原始 journal、工作区文件、Turn、Actor State、Lore、DirectorPlan 与 artifact 仍各自拥有其事实边界。

共同规则：
- 增量合并三类输入；不要重复旧 checkpoint 已覆盖且未变化的事实。
- 用户目标、明确约束、已确认决定、未完成事项、失败原因、矛盾、不确定性、不可逆副作用、验证结果和恢复引用必须保留。
- 新输入只有明确证明旧信息失效、解决或被推翻时，才能更新旧 checkpoint；保留原因与新证据。
- tool receipt 只提炼其中的状态、结论、证据 ID、artifact URI、文件/版本/Turn 引用和恢复提示；不得把已省略正文重新猜出来。
- 排除 thinking/reasoning、UI 日志、流式片段、重复工具卡片、无结论探索和传输噪音。
- 禁止编造；矛盾不得擅自裁决，不确定时明确标记。
- 目标长度由用户消息给出的范围控制，按三类输入总字符数计算；信息密度高时使用上半区，不得为达成比例丢失关键状态。

当 source_agent_kind 是 interactive_story 或 interactive_director 时，使用“叙事/游戏 checkpoint”：
- 保留事件顺序、用户行动与对白、因果后果、关系变化、任务、秘密、危险、倒计时和长期创作约束。
- 有 source turn_id 时必须保留；缺失时标记来源缺失，不得自造。
- 当前 Actor 数值/位置/资源以 Actor State 为准；未来安排以 DirectorPlan 为准；稳定设定以 Lore 为准。checkpoint 只保留历史原因和已发生变更，不复制当前真源，不把计划写成事实。
- 可以合并纯氛围、重复心理描写、无后果闲聊和修辞。

叙事/游戏输出格式：
【历史事件时间线】
- [source turn_id 或消息序号] 事件及长期影响
【历史因果与来源】
- 结论或变化 ← 原因与来源；含矛盾和不确定性
【未闭环事项】
- 伏笔、承诺、债务、秘密、危险、任务、倒计时及来源
【用户意图与创作约束】
- 仍影响后续行为的目标、偏好、拒绝、边界与决定

其他 source_agent_kind 使用“工作区任务 checkpoint”，同时适用于写作、配置、图像、自动化和工程任务：
- 保留用户目标与边界、创作/产品/技术决定及理由、当前实现或作品状态、文件与 artifact 引用、已确认发现、变更与验证、失败和被否决方案、未解决问题及下一步。
- 文件正文、日志和搜索结果只保留后续决策所需结论及可恢复引用；不要复制大段源内容。
- 已完成步骤可以合并，但必须保留结果、行为变化、兼容性影响和验证证据。

工作区任务输出格式：
【任务目标与用户约束】
- 当前目标、明确边界、偏好和验收条件
【决定与理由】
- 已确认的创作/产品/技术决定；被否决方案及原因
【当前状态】
- 已完成实现或作品状态、关键文件/版本/数据引用
【发现、证据与验证】
- 已确认事实、tool/artifact 引用、测试或检查结果
【未解决问题与下一步】
- 阻塞、风险、待验证事项和按依赖排序的下一步
`)
}

func buildContextCompactionTranscript(messages []*agent.Message, existingCheckpoint, referenceContext string, sourceTokens, inputChars int, policy contextCompactionPolicy) string {
	blocks := make([]string, 0, len(messages))
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		blocks = append(blocks, formatCompactionMessage(i+1, msg))
	}
	minChars, maxChars := compactionTargetCharRange(inputChars, policy)
	var sb strings.Builder
	sb.WriteString("请按系统要求增量编译以下 Denova 上下文 checkpoint。\n")
	sb.WriteString(fmt.Sprintf("Source agent kind: %s. 必须选择与该来源模式匹配的唯一输出格式。\n", firstNonEmpty(strings.TrimSpace(policy.AgentKind), "unknown")))
	sb.WriteString(fmt.Sprintf("Estimated new context tokens: %d. Input characters across existing checkpoint, reference context, and new context: %d. Target summary length: %d-%d characters (%s of input characters). 不得低于下限；信息密度高时使用目标范围上半区。\n\n", sourceTokens, inputChars, minChars, maxChars, compactionTargetRange(policy)))
	sb.WriteString("<existing_checkpoint>\n")
	if existingCheckpoint = strings.TrimSpace(existingCheckpoint); existingCheckpoint != "" {
		sb.WriteString(existingCheckpoint)
		sb.WriteString("\n")
	} else {
		sb.WriteString("（未提供；本次输入从新增上下文与有界参考上下文初始化 checkpoint。）\n")
	}
	sb.WriteString("</existing_checkpoint>\n\n")
	if referenceContext = strings.TrimSpace(referenceContext); referenceContext != "" {
		sb.WriteString("<reference_context>\n")
		sb.WriteString(referenceContext)
		sb.WriteString("\n</reference_context>\n\n")
	}
	sb.WriteString("<new_context>\n")
	if len(blocks) == 0 {
		sb.WriteString("（无新增原始消息。）\n")
	} else {
		for _, block := range blocks {
			sb.WriteString(block)
		}
	}
	sb.WriteString("</new_context>\n")
	return sb.String()
}

func contextCompactionInputChars(existingCheckpoint string, messages []*agent.Message, referenceContext string) int {
	total := countRunes(existingCheckpoint)
	total += countRunes(referenceContext)
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		total += countRunes(formatCompactionMessage(i+1, msg))
	}
	return total
}

func countRunes(value string) int {
	return utf8.RuneCountInString(strings.TrimSpace(value))
}

func formatCompactionMessage(index int, msg *agent.Message) string {
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
