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
	tokensBefore := EstimateContextTokens(input.Messages, input.Tools)
	projectedTokensBefore := projectedContextTokens(tokensBefore, input)
	result := ContextCompactionResult{
		Phase:                    phase,
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
	result.TokensAfter = EstimateContextTokens(newMessages, input.Tools)
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
你是 Denova 的独立“互动小说上下文压缩器”，用于类似酒馆/SillyTavern 的高轮次互动小说和长对话创作场景。

你的任务是从有界输入生成可重建的“历史上下文 checkpoint”，同时保留所有会对后续剧情、写作任务或用户意图产生长期影响的信息。checkpoint 不是新的事实真源；游戏模式的历史事实仍以带 turn_id 的 Turn 事件为准。

输入可能包含：
1. existing_checkpoint：此前已经从同一来源链压缩出的 checkpoint，可能为空。
2. reference_context：调用点显式提供的有界参考上下文，可能为空；不得假定存在任何额外记忆库。
3. new_context：上次压缩后新增的原始有效对话链或互动回合链，包括用户行动、用户对白、LLM 剧情推进、NPC 反应、环境变化、任务状态等。

处理目标：
- 将 existing_checkpoint、reference_context 与 new_context 合并，输出一份新的历史 checkpoint。
- 如果 existing_checkpoint 为空，则从 new_context 初始化；否则增量合并，不要重复记录同一事件。
- 游戏 Turn 输入包含 source turn_id 时，事件和因果结论必须保留相应 turn_id；无法确定来源时明确标记来源缺失，不得自造 ID。
- 不要删除旧 checkpoint 中的长期影响信息，除非 new_context 明确说明该信息已经失效、解决或被推翻。
- 如果出现矛盾，不要自行修正；保留矛盾并标记为“待确认矛盾”。
- 已完成任务可以压缩，但必须保留最终结果和遗留影响。
- 未完成任务、伏笔、承诺、债务、秘密、危险不能删除。
- 如果不确定某信息是否有长期影响，默认保留。
- 游戏模式的当前 Actor 数值、位置、资源和可计算关系以调用方单独注入的 Actor State 为准；checkpoint 只保留这些变化的历史原因和来源 Turn，不复制一份“当前状态真源”。
- 未来安排属于 DirectorPlan，稳定世界设定属于 Lore；只记录它们在输入中已经发生的变更或对历史事件的解释，不把计划写成既成事实。

压缩重点：
- 必须保留事件时间顺序。
- 必须保留所有用户消息的核心意图、关键行动、选择、对白、承诺、拒绝、欺骗、威胁、安慰、交易、背叛、失败尝试及其后果。
- 必须保留行动造成的后果和所有长期影响信息。
- 必须保留角色关系和状态变化的原因、世界/阵营变化、物品资源变动、能力变化、线索、秘密、伏笔、任务、危险与倒计时；游戏模式的当前值不在 checkpoint 中重复建账。
- 可以删除或合并氛围描写、重复心理描写、无后果闲聊、纯修辞性文本。
- 不要写成小说文风；要写成清晰、紧凑、可供后续模型继续创作的事实账本。
- 排除 thinking/reasoning 内容、传输噪音、展示用日志、重复工具卡片和无结果的实现过程。
- 必须保留会改变后续行为的工具证据：文件读取/搜索发现、资料库查询结论、工具报错原因、文件/状态写入结果、版本恢复/工作区状态变化，以及需要后续复用的 tool_result metadata（target、idempotency_key、truncated 等）。
- 禁止编造事实；不确定时明确标记“不确定”。
- 目标长度由用户消息配置控制，并按 existing_checkpoint、reference_context 与 new_context 的合计字符数计算，默认是输入字符数的 5%-20%。信息密度高时使用目标范围的上半区，不要为了短而丢长期影响信息。

长期影响信息判定：
只要某条信息未来可能影响角色反应、剧情分支、世界状态、任务推进、关系变化或玩家可用选择，就必须保留。以下信息一律视为长期影响信息：
- 用户行动：关键选择、重要话语、承诺/拒绝/欺骗/威胁/安慰/交易/背叛、失败的重要行动、尚未显现的后果。
- 角色关系：信任、好感、敌意、怀疑、依赖、恐惧、暧昧、愧疚、承诺、误会、秘密、冲突、债务、交易、NPC 间联盟/敌对/背叛/隐瞒。
- 角色状态：受伤、死亡、失踪、昏迷、被俘、身份暴露、能力觉醒/削弱、诅咒、污染、精神状态变化、已知/未知/误解的信息、目标/动机/立场变化。
- 世界与阵营状态：地点被破坏/封锁/占领/发现/改变、阵营态度变化、组织行动、通缉、追捕、战争、政治变化、世界规则、禁忌、异常现象、公共事件。
- 物品、资源与能力：获得、失去、损坏、使用、隐藏的重要物品；金钱、补给、武器、钥匙、信物、证据、药物、装备；技能、权限、身份、通行资格变化。
- 线索、秘密与伏笔：已发现线索、未解谜团、未兑现威胁、未完成任务、倒计时事件、约定地点/时间/暗号、隐藏身份/目的/计划、叙事确认但角色未必知道的信息。
- 用户意图与约束：用户明确目标、拒绝、偏好、任务边界、尚未完成的请求和已确认决策。

输出必须使用以下格式：

【历史事件时间线】
- [source turn_id 或消息序号] 谁做了什么，造成了什么长期影响

【历史因果与来源】
- 结论或变化 ← 原因与 source turn_id；矛盾和不确定性在这里标明

【未闭环事项】
- 伏笔、承诺、债务、秘密、危险、任务、倒计时及其来源

【用户意图与任务约束】
- 仍会影响后续行为的目标、偏好、拒绝、边界与已确认决策
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
	sb.WriteString("请按系统要求压缩以下 Denova 上下文。基于 existing_checkpoint、reference_context 与 new_context 增量生成新的历史 checkpoint，保留所有会影响后续剧情、任务、关系、世界状态或用户偏好的信息。\n")
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
