package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
)

const (
	PhasePreRun    = "pre_run"
	PhaseMidRun    = "mid_run"
	PhaseModelStep = "model_step"

	reasonLimit = "context_usage_threshold"
)

// Policy is the normalized compaction policy shared by planning and durable
// conversation adapters.
type Policy struct {
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

type Result struct {
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

// NewCheckpoint freezes the durable semantic subset of a
// successful compaction result. Domain-specific stores add their own source
// cursors and commit metadata around this shared value.
func NewCheckpoint(agentKind string, result Result) agentcontext.CompactionCheckpoint {
	return agentcontext.CompactionCheckpoint{
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

// ResultFromCheckpoint restores the exact durable runtime
// view. Callers that need derived recovery-band fields recalculate them after
// applying any domain-specific post-context projection.
func ResultFromCheckpoint(checkpoint agentcontext.CompactionCheckpoint) Result {
	result := Result{
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
	applyContextCompactionRecovery(&result)
	return result
}

// SummaryRequest is the fully assembled, provider-neutral
// model request produced by the compaction domain. The injected summarizer is
// responsible only for executing it and returning checkpoint Markdown.
type SummaryRequest struct {
	SourceAgentKind string
	Messages        []*agent.Message
	SourceMessages  int
	SourceTokens    int
	InputChars      int
}

type SummaryFunc func(
	context.Context,
	*config.Config,
	SummaryRequest,
	func(attempt int, delta string),
) (string, error)

type Input struct {
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
	// Automatic model-step compaction supplies the exact current call snapshot.
	PrimaryRequestSnapshot *agent.ModelRequestSnapshot
	// ProviderSourceMessages is the assembler-produced projection of
	// SourceMessages. Durable adapters use it only to locate the exact source in
	// PrimaryRequestSnapshot; cold fallback still summarizes canonical source.
	ProviderSourceMessages []*agent.Message
	// Summarize executes provider-neutral cold fallback requests. The normal
	// cache-safe path reuses PrimaryRequestSnapshot and does not call it.
	Summarize SummaryFunc
	// ColdFallbackReason is an explicit internal capability for callers that
	// intentionally cannot provide a primary snapshot (for example, a source
	// larger than one model window). Ordinary manual compaction leaves it empty.
	ColdFallbackReason string
}

// ResolvePolicy applies model and context defaults for one Agent kind.
func ResolvePolicy(cfg *config.Config, agentKind string) Policy {
	contextSettings := config.ResolveAgentContext(cfg, agentKind)
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return Policy{
		AgentKind:              agentKind,
		Enabled:                contextSettings.CompactionEnabled,
		Strategy:               config.AgentContextCompactionStrategyCheckpointFork,
		ContextWindowTokens:    modelSettings.ContextWindowTokens,
		Threshold:              contextSettings.CompactionThreshold,
		RecoveryBand:           config.DefaultContextCompactionRecoveryBand,
		RetainedTurns:          config.DefaultContextCompactionRetainedTurns,
		TargetMinRatio:         config.DefaultContextCompactionTargetMinRatio,
		TargetMaxRatio:         config.DefaultContextCompactionTargetMaxRatio,
		MaxConsecutiveFailures: config.DefaultContextCompactionMaxConsecutiveFailures,
	}
}

func (p Policy) TriggerTokens() int {
	if !p.Enabled || p.ContextWindowTokens <= 0 || p.Threshold <= 0 {
		return 0
	}
	return int(float64(p.ContextWindowTokens) * p.Threshold)
}

func (p Policy) ShouldCompact(tokens int, force bool) (bool, string) {
	if force {
		return true, ""
	}
	if !p.Enabled {
		return false, "disabled"
	}
	if p.ContextWindowTokens <= 0 {
		return false, "context_window_tokens_missing"
	}
	trigger := p.TriggerTokens()
	if trigger <= 0 {
		return false, "threshold_invalid"
	}
	if tokens < trigger {
		return false, "below_threshold"
	}
	return true, ""
}

// normalizeInput gives every structural entry point the same
// provider-neutral protocol projection as the normal model middleware. It is
// intentionally deterministic: cache-safe forks and post-checkpoint token
// accounting must operate on one exact message shape.
func normalizeInput(input *Input) error {
	if input == nil {
		return nil
	}
	normalizedMessages, err := agentcontext.NormalizeModelContextMessages(input.Messages)
	if err != nil {
		return err
	}
	normalizedSource := input.SourceMessages
	if input.SourceMessagesSet || len(input.SourceMessages) > 0 {
		normalizedSource, err = agentcontext.NormalizeModelContextMessages(input.SourceMessages)
		if err != nil {
			return err
		}
	}
	normalizedProviderSource := input.ProviderSourceMessages
	if len(input.ProviderSourceMessages) > 0 {
		normalizedProviderSource, err = agentcontext.NormalizeModelContextMessages(input.ProviderSourceMessages)
		if err != nil {
			return err
		}
	}
	changed := !contextMessagesEqual(input.Messages, normalizedMessages) ||
		!contextMessagesEqual(input.SourceMessages, normalizedSource) ||
		!contextMessagesEqual(input.ProviderSourceMessages, normalizedProviderSource)
	before := len(input.Messages)
	input.Messages = normalizedMessages
	input.SourceMessages = normalizedSource
	input.ProviderSourceMessages = normalizedProviderSource
	if changed && input.Emit != nil {
		input.Emit(Event{Type: "context_normalizer", Data: map[string]any{
			"status": "repaired", "context_normalizer_repair_count": 1,
			"messages_before": before, "messages_after": len(normalizedMessages),
		}})
	}
	return nil
}

// Prepare performs bounded policy evaluation and summary
// generation without mutating Session or Story storage. Canonical publication
// belongs to a durable structural command's Commit phase.
func Prepare(ctx context.Context, cfg *config.Config, agentKind string, input Input, epoch int) ([]*agent.Message, Result, error) {
	policy := ResolvePolicy(cfg, agentKind)
	if input.ContextWindowTokens > 0 {
		policy.ContextWindowTokens = input.ContextWindowTokens
	}
	phase := strings.TrimSpace(input.Phase)
	if phase == "" {
		phase = PhasePreRun
	}
	originalMessages := input.Messages
	if err := normalizeInput(&input); err != nil {
		result := Result{Phase: phase, SkippedReason: "protocol_invalid"}
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return originalMessages, result, fmt.Errorf("normalize compaction input: %w", err)
	}
	input = withDefaultContextProjectionReserves(cfg, agentKind, input, 0)
	policy.CheckpointOutputReserve = max(policy.CheckpointOutputReserve, input.ReservedCompletionTokens)
	estimatedTokensBefore := agentcontext.EstimateTokens(input.Messages, input.Tools)
	tokensBefore := calibratedContextTokens(estimatedTokensBefore, input)
	projectedTokensBefore := projectedContextTokens(tokensBefore, input)
	result := Result{
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
	shouldCompact, skipped := policy.ShouldCompact(projectedTokensBefore, input.Force)
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
	sourceTokens := agentcontext.EstimateTokens(source, nil)
	emitContextCompactionEvent(input.Emit, phase, "started", result)
	coldFallbackReason := strings.TrimSpace(input.ColdFallbackReason)
	if input.Force && input.PrimaryRequestSnapshot == nil {
		if coldFallbackReason != "" {
			// Internal callers may explicitly select the exceptional cold path;
			// public/manual entry points never set this capability.
		} else if manualCompactionSourceExceedsSingleWindow(sourceTokens, policy.ContextWindowTokens) {
			coldFallbackReason = fallbackManualSourceWindow
		} else {
			return input.Messages, result, errors.New("manual context compaction requires the final primary request snapshot")
		}
	}
	providerSource := source
	if len(input.ProviderSourceMessages) > 0 {
		providerSource = input.ProviderSourceMessages
	}
	summary, inputChars, execution, err := summarizeContextInLayers(
		ctx, cfg, agentKind, input.ExistingCheckpoint, source, providerSource,
		input.ReferenceContext, sourceTokens, policy, input.PrimaryRequestSnapshot,
		coldFallbackReason, input.Summarize,
		func(attempt int, delta string) {
			emitContextCompactionDeltaEvent(input.Emit, phase, result, attempt, delta)
		},
	)
	applyContextCompactionExecution(&result, execution)
	if err != nil {
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	if epoch <= 0 {
		epoch = 1
	}
	sourceEndPosition := len(input.Messages)
	if positions, _, visible := locateCompactionSourceInPrimary(input.Messages, providerSource); visible && len(positions) > 0 {
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
	newMessages, err = agentcontext.NormalizeModelContextMessages(newMessages)
	if err != nil {
		result.SkippedReason = "protocol_invalid"
		emitContextCompactionEvent(input.Emit, phase, "failed", result)
		return input.Messages, result, err
	}
	result.CandidateFingerprint, result.CandidateGeneration = CandidateIdentity(newMessages, 0)
	result.Triggered = true
	result.Epoch = epoch
	result.Summary = checkpointPayload
	result.TokensAfter = calibratedContextTokens(agentcontext.EstimateTokens(newMessages, input.Tools), input)
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
	return reasonLimit
}

func applyContextCompactionRecovery(result *Result) {
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
	result.Degraded = !result.RecoveryBandMet && result.TokensAfter < PublishLimit(result.ContextWindowTokens, threshold)
}

func effectiveContextCompactionThreshold(threshold float64) float64 {
	if threshold <= 0 || threshold >= 1 {
		return config.DefaultContextCompactionThreshold
	}
	return threshold
}

// PublishLimit is the configured hard checkpoint boundary.
// Cleanup planning, degraded publication, durable health, and Game validation
// all use this one threshold rather than drifting to a hidden 85% constant.
func PublishLimit(contextWindow int, threshold float64) int {
	if contextWindow <= 0 {
		return 0
	}
	return int(float64(contextWindow) * effectiveContextCompactionThreshold(threshold))
}

func applyContextCompactionExecution(result *Result, execution contextCompactionSummaryExecution) {
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

func validateCompactedContextResult(result Result) error {
	if result.TokensAfter >= result.TokensBefore {
		return fmt.Errorf("context compaction made no progress: before=%d after=%d", result.TokensBefore, result.TokensAfter)
	}
	if publishLimit := PublishLimit(result.ContextWindowTokens, result.Threshold); publishLimit > 0 && result.TokensAfter >= publishLimit {
		return fmt.Errorf("context compaction post-context remains above hard publish band: after=%d window=%d", result.TokensAfter, result.ContextWindowTokens)
	}
	return nil
}

// ValidateResult validates the true post-context after a
// domain conversation has re-injected its deterministic bounded state. Callers
// must run this after, not before, those post-compaction providers.
func Validate(result Result) error {
	return validateCompactedContextResult(result)
}

func contextCompactionRatio(partChars, inputChars int) float64 {
	if inputChars <= 0 {
		return 0
	}
	return float64(partChars) / float64(inputChars)
}

func compactionTargetRange(policy Policy) string {
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

func compactionTargetCharRange(inputChars int, policy Policy) (int, int) {
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

func buildContextCompactionTranscript(messages []*agent.Message, existingCheckpoint, referenceContext string, sourceTokens, inputChars int, policy Policy) string {
	blocks := make([]string, 0, len(messages))
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		blocks = append(blocks, formatCompactionMessage(i+1, msg))
	}
	minChars, maxChars := compactionTargetCharRange(inputChars, policy)
	var sb strings.Builder
	sb.WriteString("Incrementally compile the following Denova context checkpoint according to the system instructions.\n")
	sb.WriteString(fmt.Sprintf("Source agent kind: %s. Apply the domain rules for this source and use the single Markdown checkpoint schema from the system message exactly.\n", firstNonEmpty(strings.TrimSpace(policy.AgentKind), "unknown")))
	sb.WriteString(fmt.Sprintf("Estimated new context tokens: %d. Input characters across existing checkpoint, reference context, and new context: %d. Target summary length: %d-%d characters (%s of input characters). Do not go below the lower bound; use the upper half of the range when information density is high.\n\n", sourceTokens, inputChars, minChars, maxChars, compactionTargetRange(policy)))
	sb.WriteString("<existing_checkpoint>\n")
	if existingCheckpoint = strings.TrimSpace(existingCheckpoint); existingCheckpoint != "" {
		sb.WriteString(existingCheckpoint)
		sb.WriteString("\n")
	} else {
		sb.WriteString("(Not provided; initialize the checkpoint from new context and bounded reference context.)\n")
	}
	sb.WriteString("</existing_checkpoint>\n\n")
	if referenceContext = strings.TrimSpace(referenceContext); referenceContext != "" {
		sb.WriteString("<reference_context>\n")
		sb.WriteString(referenceContext)
		sb.WriteString("\n</reference_context>\n\n")
	}
	sb.WriteString("<new_context>\n")
	if len(blocks) == 0 {
		sb.WriteString("(No new raw messages.)\n")
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
