package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/contextmaintenance"
)

const (
	contextCompactionPhasePreRun    = "pre_run"
	contextCompactionPhaseMidRun    = "mid_run"
	contextCompactionPhaseModelStep = "model_step"
	contextCompactionReasonLimit    = "context_usage_threshold"

	contextCompactionSummaryPrefix = "[Denova Context Compaction]"
)

type contextCompactionPolicy struct {
	AgentKind               string
	Enabled                 bool
	Strategy                string
	ContextWindowTokens     int
	Threshold               float64
	RecoveryBand            float64
	RetainedTurns           int
	TargetMinRatio          float64
	TargetMaxRatio          float64
	MaxConsecutiveFailures  int
	CheckpointOutputReserve int
}

type ContextCompactionResult struct {
	Triggered                 bool
	SkippedReason             string
	Phase                     string
	EstimatedTokensBefore     int
	ObservedPromptTokens      int
	ObservedEstimateTokens    int
	TokensBefore              int
	TokensAfter               int
	ProjectedTokensBefore     int
	ProjectedTokensAfter      int
	ReservedCompletionTokens  int
	ReservedToolResultTokens  int
	ContextWindowTokens       int
	Strategy                  string
	Threshold                 float64
	TriggerReason             string
	RecoveryBand              float64
	RecoveryTargetTokens      int
	RecoveryBandMet           bool
	Degraded                  bool
	Epoch                     int
	Summary                   string
	TargetRatio               float64
	SourceMessageCount        int
	MessageCountBefore        int
	MessageCountAfter         int
	RetainedTurns             int
	ExecutionMode             string
	FallbackReason            string
	CompactionInputTokens     int
	CompactionPromptTokens    int
	CheckpointOutputReserve   int
	SafetyMarginTokens        int
	CacheExpectedPrefixTokens int
	CacheReadTokens           int
	CacheWriteTokens          int
	CacheWriteTokensKnown     bool
	CacheIdentityStatus       string
	CacheUsageStatus          string
	CacheMissReason           string
	LayerCount                int
	CandidateFingerprint      string
	CandidateGeneration       uint64
	ConsecutiveFailures       int
	FailureFuseOpen           bool
}

// NewContextCompactionCheckpoint freezes the durable semantic subset of a
// successful compaction result. Domain-specific stores add their own source
// cursors and commit metadata around this shared value.
func NewContextCompactionCheckpoint(agentKind string, result ContextCompactionResult) contextmaintenance.CompactionCheckpoint {
	return contextmaintenance.CompactionCheckpoint{
		AgentKind:              strings.TrimSpace(agentKind),
		Epoch:                  result.Epoch,
		Summary:                result.Summary,
		RetainedTurns:          result.RetainedTurns,
		EstimatedTokensBefore:  result.EstimatedTokensBefore,
		ObservedPromptTokens:   result.ObservedPromptTokens,
		ObservedEstimateTokens: result.ObservedEstimateTokens,
		TokensBefore:           result.TokensBefore,
		TokensAfter:            result.TokensAfter,
		TargetRatio:            result.TargetRatio,
		ContextWindowTokens:    result.ContextWindowTokens,
		Strategy:               result.Strategy,
		Threshold:              result.Threshold,
		TriggerReason:          strings.TrimSpace(result.TriggerReason),
		Phase:                  result.Phase,
		RecoveryBand:           result.RecoveryBand,
		CandidateFingerprint:   result.CandidateFingerprint,
		CandidateGeneration:    result.CandidateGeneration,
	}
}

// ContextCompactionResultFromCheckpoint restores the exact durable runtime
// view. Callers that need derived recovery-band fields recalculate them after
// applying any domain-specific post-context projection.
func ContextCompactionResultFromCheckpoint(checkpoint contextmaintenance.CompactionCheckpoint) ContextCompactionResult {
	result := ContextCompactionResult{
		Triggered:              true,
		Phase:                  checkpoint.Phase,
		EstimatedTokensBefore:  checkpoint.EstimatedTokensBefore,
		ObservedPromptTokens:   checkpoint.ObservedPromptTokens,
		ObservedEstimateTokens: checkpoint.ObservedEstimateTokens,
		TokensBefore:           checkpoint.TokensBefore,
		TokensAfter:            checkpoint.TokensAfter,
		ContextWindowTokens:    checkpoint.ContextWindowTokens,
		Strategy:               checkpoint.Strategy,
		Threshold:              checkpoint.Threshold,
		TriggerReason:          checkpoint.TriggerReason,
		RecoveryBand:           checkpoint.RecoveryBand,
		Epoch:                  checkpoint.Epoch,
		Summary:                checkpoint.Summary,
		TargetRatio:            checkpoint.TargetRatio,
		RetainedTurns:          checkpoint.RetainedTurns,
		CandidateFingerprint:   checkpoint.CandidateFingerprint,
		CandidateGeneration:    checkpoint.CandidateGeneration,
	}
	return result
}

type contextCompactionSummaryFunc func(ctx context.Context, cfg *config.Config, agentKind string, existingCheckpoint string, source []*agent.Message, referenceContext string, sourceTokens int, policy contextCompactionPolicy, emitDelta func(attempt int, delta string)) (string, int, error)

type contextCompactionController struct {
	conversation ContextCompactionConversation
	emit         func(Event)
}

// ContextCompactionConversation is implemented by conversations that can
// persist and rebuild model-visible compaction epochs.
type ContextCompactionConversation interface {
	CompactContextIfNeeded(ctx context.Context, input ContextCompactionInput) ([]*agent.Message, ContextCompactionResult, error)
}

type ContextCompactionInput struct {
	Messages       []*agent.Message
	SourceMessages []*agent.Message
	// SourceMessagesSet distinguishes an intentionally empty canonical source
	// from the legacy "derive the source from Messages" behavior. Domain
	// adapters with their own durable boundary must set this even when the
	// boundary currently contains no messages; otherwise an unsettled suffix
	// could be mistaken for checkpoint source.
	SourceMessagesSet bool
	Tools             []*agent.ToolInfo
	Phase             string
	Emit              func(Event)
	Force             bool
	// Planned means the unified pressure planner has already selected
	// compaction over cleanup. It bypasses duplicate threshold evaluation while
	// remaining an automatic, post-settlement structural operation.
	Planned             bool
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
	// Automatic distinguishes the unified model-step planner from explicit
	// user compaction. Automatic maintenance may be gated by health latches;
	// explicit requests always re-evaluate immediately.
	Automatic bool
	// CandidateTokens lets a degraded checkpoint retry when the cleanup
	// candidate set has materially changed, even if canonical tail growth alone
	// is still small.
	CandidateTokens int
	// CandidateFingerprint and CandidateGeneration identify the actual cleanup
	// candidate set. A non-zero token count alone must not release a degraded
	// no-progress latch when the candidates are unchanged.
	CandidateFingerprint string
	CandidateGeneration  uint64
	// TriggerReason is selected once by the unified pressure planner and is
	// persisted/observed with the checkpoint. Manual callers may leave it empty.
	TriggerReason string
	// PreflightSkipReason is selected by a durable conversation adapter when a
	// no-progress latch or consecutive-failure fuse blocks automatic work.
	PreflightSkipReason string
	ConsecutiveFailures int
	FailureFuseOpen     bool
	// PrimaryRequestSnapshot is supplied by explicit host compaction after it
	// runs the normal Agent request assembler without invoking the provider.
	// Automatic model-step compaction receives the same value through context.
	PrimaryRequestSnapshot *agent.ModelRequestSnapshot
}

type contextCompactionContextKey struct{}

var summarizeContextForCompaction contextCompactionSummaryFunc = generateContextCompactionSummary

func contextWithCompactionController(ctx context.Context, conversation Conversation, emit ...func(Event)) context.Context {
	compaction, ok := conversation.(ContextCompactionConversation)
	if !ok || compaction == nil {
		return ctx
	}
	controller := &contextCompactionController{conversation: compaction}
	if len(emit) > 0 {
		controller.emit = emit[0]
	}
	return context.WithValue(ctx, contextCompactionContextKey{}, controller)
}

func compactionControllerFromContext(ctx context.Context) *contextCompactionController {
	controller, _ := ctx.Value(contextCompactionContextKey{}).(*contextCompactionController)
	return controller
}

func resolveContextCompactionPolicy(cfg *config.Config, agentKind string) contextCompactionPolicy {
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return contextCompactionPolicy{
		AgentKind:              agentKind,
		Enabled:                contextSettings.CompactionEnabled,
		Strategy:               config.AgentContextCompactionStrategySummaryAgent,
		ContextWindowTokens:    modelSettings.ContextWindowTokens,
		Threshold:              contextSettings.CompactionThreshold,
		RecoveryBand:           config.DefaultContextCompactionRecoveryBand,
		RetainedTurns:          config.DefaultContextCompactionRetainedTurns,
		TargetMinRatio:         config.DefaultContextCompactionTargetMinRatio,
		TargetMaxRatio:         config.DefaultContextCompactionTargetMaxRatio,
		MaxConsecutiveFailures: config.DefaultContextCompactionMaxConsecutiveFailures,
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

// normalizeContextCompactionInput gives every structural entry point the same
// provider-neutral protocol projection as the normal model middleware. It is
// intentionally deterministic: cache-safe forks and post-checkpoint token
// accounting must operate on one exact message shape.
func normalizeContextCompactionInput(input *ContextCompactionInput) error {
	if input == nil {
		return nil
	}
	normalizedMessages, err := NormalizeModelContextMessages(input.Messages)
	if err != nil {
		return err
	}
	normalizedSource := input.SourceMessages
	if input.SourceMessagesSet || len(input.SourceMessages) > 0 {
		normalizedSource, err = NormalizeModelContextMessages(input.SourceMessages)
		if err != nil {
			return err
		}
	}
	changed := !contextMessagesEqual(input.Messages, normalizedMessages) ||
		!contextMessagesEqual(input.SourceMessages, normalizedSource)
	before := len(input.Messages)
	input.Messages = normalizedMessages
	input.SourceMessages = normalizedSource
	if changed && input.Emit != nil {
		input.Emit(Event{Type: "context_normalizer", Data: map[string]any{
			"status": "repaired", "context_normalizer_repair_count": 1,
			"messages_before": before, "messages_after": len(normalizedMessages),
		}})
	}
	return nil
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
	originalMessages := input.Messages
	if err := normalizeContextCompactionInput(&input); err != nil {
		result := ContextCompactionResult{Phase: phase, SkippedReason: "protocol_invalid"}
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return originalMessages, result, fmt.Errorf("normalize compaction input: %w", err)
	}
	input = withDefaultContextProjectionReserves(cfg, agentKind, input, 0)
	policy.CheckpointOutputReserve = max(policy.CheckpointOutputReserve, input.ReservedCompletionTokens)
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
		TriggerReason:            contextCompactionTriggerReason(input.TriggerReason, phase),
		RecoveryBand:             policy.RecoveryBand,
		MessageCountBefore:       len(input.Messages),
		RetainedTurns:            policy.RetainedTurns,
		CandidateFingerprint:     strings.TrimSpace(input.CandidateFingerprint),
		CandidateGeneration:      input.CandidateGeneration,
		ConsecutiveFailures:      input.ConsecutiveFailures,
		FailureFuseOpen:          input.FailureFuseOpen,
	}
	if reason := strings.TrimSpace(input.PreflightSkipReason); reason != "" {
		result.SkippedReason = reason
		emitContextCompactionEvent(input.Emit, phase, "skipped", result)
		return input.Messages, result, nil
	}
	shouldCompact, skipped := policy.shouldCompact(projectedTokensBefore, input.Force)
	if input.Planned {
		shouldCompact, skipped = true, ""
	}
	if !shouldCompact {
		result.SkippedReason = skipped
		return input.Messages, result, nil
	}
	source := compactionSourceMessages(compactionSourceBaseMessages(input), input.KeepLatestUser)
	if input.SourceMessagesSet && len(source) == 0 {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	if len(source) == 0 && strings.TrimSpace(input.ExistingCheckpoint) == "" && strings.TrimSpace(input.ReferenceContext) == "" {
		result.SkippedReason = "empty_source"
		return input.Messages, result, nil
	}
	sourceTokens := EstimateContextTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	compactionCtx := ctx
	if input.PrimaryRequestSnapshot != nil {
		compactionCtx = contextWithCompactionRequestSnapshot(compactionCtx, input.PrimaryRequestSnapshot)
	}
	if input.Force && compactionRequestSnapshotFromContext(compactionCtx) == nil {
		if _, allowed := standaloneCompactionFallbackReason(compactionCtx); allowed {
			// Internal callers may explicitly select the exceptional cold path;
			// public/manual entry points never receive this unexported capability.
		} else if manualCompactionSourceExceedsSingleWindow(sourceTokens, policy.ContextWindowTokens) {
			compactionCtx = contextWithStandaloneCompactionFallback(compactionCtx, contextCompactionFallbackManualSourceWindow)
		} else {
			return input.Messages, result, errors.New("manual context compaction requires the final primary request snapshot")
		}
	}
	summary, inputChars, execution, err := summarizeContextInLayers(compactionCtx, cfg, agentKind, input.ExistingCheckpoint, source, input.ReferenceContext, sourceTokens, policy, func(attempt int, delta string) {
		emitContextCompactionDeltaEvent(input.Emit, phase, result, attempt, delta)
	})
	applyContextCompactionExecution(&result, execution)
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	if epoch <= 0 {
		epoch = 1
	}
	sourceEndPosition := len(input.Messages)
	if positions, _, visible := locateCompactionSourceInPrimary(input.Messages, source); visible && len(positions) > 0 {
		sourceEndPosition = positions[len(positions)-1] + 1
	}
	newMessages, checkpointPayload := compactMessagesForModelThroughSource(
		input.Messages,
		summary,
		input.ExistingCheckpoint,
		epoch,
		policy.RetainedTurns,
		sourceEndPosition,
	)
	newMessages, err = NormalizeModelContextMessages(newMessages)
	if err != nil {
		result.SkippedReason = "protocol_invalid"
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	result.CandidateFingerprint, result.CandidateGeneration = ContextCompactionCandidateIdentity(newMessages, 0)
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = checkpointPayload
	result.TokensAfter = calibratedContextTokens(EstimateContextTokens(newMessages, input.Tools), input)
	result.ProjectedTokensAfter = projectedContextTokens(result.TokensAfter, input)
	result.TargetRatio = contextCompactionRatio(countRunes(summary), inputChars)
	result.SourceMessageCount = len(source)
	result.MessageCountAfter = len(newMessages)
	applyContextCompactionRecovery(&result)
	if err := validateCompactedContextResult(result); err != nil {
		result.Triggered = false
		result.SkippedReason = "no_progress"
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	emitContextCompactionEvent(input.Emit, phase, "completed", result)
	return newMessages, result, nil
}

func contextCompactionTriggerReason(reason, phase string) string {
	if reason = strings.TrimSpace(reason); reason != "" {
		return reason
	}
	if strings.TrimSpace(phase) == "manual" {
		return "manual"
	}
	return contextCompactionReasonLimit
}

func applyContextCompactionRecovery(result *ContextCompactionResult) {
	if result == nil || result.ContextWindowTokens <= 0 {
		return
	}
	recoveryBand := result.RecoveryBand
	if recoveryBand <= 0 || recoveryBand > 1 {
		recoveryBand = config.DefaultContextCompactionRecoveryBand
		result.RecoveryBand = recoveryBand
	}
	threshold := effectiveContextCompactionThreshold(result.Threshold)
	result.Threshold = threshold
	result.RecoveryTargetTokens = int(float64(result.ContextWindowTokens) * threshold * recoveryBand)
	result.RecoveryBandMet = result.TokensAfter <= result.RecoveryTargetTokens
	result.Degraded = !result.RecoveryBandMet && result.TokensAfter < ContextCompactionPublishLimit(result.ContextWindowTokens, threshold)
}

func effectiveContextCompactionThreshold(threshold float64) float64 {
	if threshold <= 0 || threshold >= 1 {
		return config.DefaultContextCompactionThreshold
	}
	return threshold
}

// ContextCompactionPublishLimit is the configured hard checkpoint boundary.
// Cleanup planning, degraded publication, durable health, and Game validation
// all use this one threshold rather than drifting to a hidden 85% constant.
func ContextCompactionPublishLimit(contextWindow int, threshold float64) int {
	if contextWindow <= 0 {
		return 0
	}
	return int(float64(contextWindow) * effectiveContextCompactionThreshold(threshold))
}

func applyContextCompactionExecution(result *ContextCompactionResult, execution contextCompactionSummaryExecution) {
	if result == nil {
		return
	}
	result.ExecutionMode = execution.Mode
	result.FallbackReason = execution.FallbackReason
	result.CompactionInputTokens = execution.InputTokens
	result.CompactionPromptTokens = execution.PromptTokens
	result.CheckpointOutputReserve = execution.CheckpointOutputReserve
	result.SafetyMarginTokens = execution.SafetyMarginTokens
	result.CacheExpectedPrefixTokens = execution.ExpectedCachedPrefixTokens
	result.CacheReadTokens = execution.CacheReadTokens
	result.CacheWriteTokens = execution.CacheWriteTokens
	result.CacheWriteTokensKnown = execution.CacheWriteTokensKnown
	result.CacheIdentityStatus = execution.CacheIdentityStatus
	result.CacheUsageStatus = execution.CacheUsageStatus
	result.CacheMissReason = execution.CacheMissReason
	result.LayerCount = execution.LayerCount
}

func validateCompactedContextResult(result ContextCompactionResult) error {
	if result.TokensAfter >= result.TokensBefore {
		return fmt.Errorf("context compaction made no progress: before=%d after=%d", result.TokensBefore, result.TokensAfter)
	}
	if publishLimit := ContextCompactionPublishLimit(result.ContextWindowTokens, result.Threshold); publishLimit > 0 && result.TokensAfter >= publishLimit {
		return fmt.Errorf("context compaction post-context remains above hard publish band: after=%d window=%d", result.TokensAfter, result.ContextWindowTokens)
	}
	return nil
}

// ValidateContextCompactionResult validates the true post-context after a
// domain conversation has re-injected its deterministic bounded state. Callers
// must run this after, not before, those post-compaction providers.
func ValidateContextCompactionResult(result ContextCompactionResult) error {
	return validateCompactedContextResult(result)
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
	cm, err := newChatModel(traceCtx, modelCfg)
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

func streamContextCompactionAttempt(ctx context.Context, cm agent.BaseChatModel, input []*agent.Message, attempt int, emitDelta func(attempt int, delta string)) (*agent.Message, error) {
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
	base := strings.TrimSpace(`
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
- tool receipt 只提炼其中的状态、结论、证据 ID、artifact 可读路径、文件/版本/Turn 引用和恢复提示；不得把已省略正文重新猜出来。
- 排除 thinking/reasoning、UI 日志、流式片段、重复工具卡片、无结论探索和传输噪音。
- 禁止编造；矛盾不得擅自裁决，不确定时明确标记。
- 目标长度由用户消息给出的范围控制，按三类输入总字符数计算；信息密度高时使用上半区，不得为达成比例丢失关键状态。
- checkpoint 必须覆盖 new_context 全部 durable facts，包括会作为 verbatim convenience tail 暂时保留的最近回合；简洁提炼这些事实，不要逐字复制 tail。后续压缩会让旧 tail 退出，因此不得依赖 tail 代替 checkpoint 记忆。

当 source_agent_kind 是 interactive_story 或 interactive_director 时，使用“叙事/游戏 checkpoint”：
- 保留事件顺序、用户行动与对白、因果后果、关系变化、任务、秘密、危险、倒计时和长期创作约束。
- 有 source turn_id 时必须保留；缺失时标记来源缺失，不得自造。
- 当前 Actor 数值/位置/资源以 Actor State 为准；未来安排以 DirectorPlan 为准；稳定设定以 Lore 为准。checkpoint 只保留历史原因和已发生变更，不复制当前真源，不把计划写成事实。
- 可以合并纯氛围、重复心理描写、无后果闲聊和修辞。

其他 source_agent_kind 使用“工作区任务 checkpoint”，同时适用于写作、配置、图像、自动化和工程任务：
- 保留用户目标与边界、创作/产品/技术决定及理由、当前实现或作品状态、文件与 artifact 引用、已确认发现、变更与验证、失败和被否决方案、未解决问题及下一步。
- 文件正文、日志和搜索结果只保留后续决策所需结论及可恢复引用；不要复制大段源内容。
- 已完成步骤可以合并，但必须保留结果、行为变化、兼容性影响和验证证据。
`)
	return base + "\n\n所有来源模式都使用以下唯一、稳定的 Markdown checkpoint schema；不适用的 section 可以为空，但不要改名或另造一套格式：\n" +
		contextCompactionCheckpointSchema()
}

var contextCompactionCheckpointHeadings = []string{
	"## Goal",
	"## Constraints",
	"## Current state",
	"## Decisions and rationale",
	"## Confirmed facts and sources",
	"## Tool outcomes and readable artifacts",
	"## Failures and rejected approaches",
	"## Unresolved issues",
	"## Next actions",
	"## Critical context that must not be lost",
}

func contextCompactionCheckpointSchema() string {
	return strings.Join(contextCompactionCheckpointHeadings, "\n") + "\n"
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
	sb.WriteString(fmt.Sprintf("Source agent kind: %s. 请应用该来源的领域规则，并严格使用系统消息中的统一 Markdown checkpoint schema。\n", firstNonEmpty(strings.TrimSpace(policy.AgentKind), "unknown")))
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
