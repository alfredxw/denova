package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"
	cleanup "github.com/alfredxw/denova/agent/cleanup"
)

type SummaryRequest struct {
	Session       agent.SessionView
	Run           agent.RunView
	Messages      []*agent.Message
	ModelRequest  []*agent.Message
	ModelSnapshot *agent.ModelRequestSnapshot
	Plan          agent.CompactionPlan
}

type Summary struct {
	Content       string
	TokenEstimate int
}

type Summarizer interface {
	Identity() agent.CapabilityIdentity
	Summarize(context.Context, SummaryRequest) (Summary, error)
}

type SummarizerFunc struct {
	Capability agent.CapabilityIdentity
	Func       func(context.Context, SummaryRequest) (Summary, error)
}

func (summarizer SummarizerFunc) Identity() agent.CapabilityIdentity { return summarizer.Capability }

func (summarizer SummarizerFunc) Summarize(ctx context.Context, request SummaryRequest) (Summary, error) {
	if summarizer.Func == nil {
		return Summary{}, errors.New("Compaction Summarizer function is nil")
	}
	return summarizer.Func(ctx, request)
}

type StandardConfig struct {
	Summarizer        Summarizer
	TriggerBytes      int
	KeepRecentBytes   int
	KeepRecentTurns   int
	HardLimitBytes    int
	SummaryLimitBytes int

	ContextWindowTokens int
	ReservedTokens      int
	TriggerRatio        float64
	RecoveryBand        float64
	MinimumChangeTokens int
}

type standardManager struct {
	config   StandardConfig
	identity agent.CapabilityIdentity
}

type standardDefinition struct {
	config StandardConfig
	once   sync.Once
	value  *standardManager
	err    error
}

// Standard declares the built-in Compaction policy. Agent validates and
// resolves it together with the rest of the Definition in agent.New.
func Standard(config StandardConfig) agent.CompactionManager {
	return &standardDefinition{config: config}
}

func newStandard(config StandardConfig) (*standardManager, error) {
	if config.Summarizer == nil {
		return nil, errors.New("standard Compaction requires a Summarizer")
	}
	if err := validateIdentity(config.Summarizer.Identity()); err != nil {
		return nil, fmt.Errorf("Compaction Summarizer: %w", err)
	}
	if config.TriggerBytes <= 0 {
		config.TriggerBytes = 2 << 20
	}
	if config.KeepRecentBytes <= 0 {
		config.KeepRecentBytes = 512 << 10
	}
	if config.KeepRecentTurns <= 0 {
		config.KeepRecentTurns = 1
	}
	if config.HardLimitBytes <= 0 {
		return nil, errors.New("Compaction HardLimitBytes must be positive")
	}
	if config.SummaryLimitBytes <= 0 || config.SummaryLimitBytes > config.HardLimitBytes {
		return nil, errors.New("Compaction SummaryLimitBytes must be positive and no larger than HardLimitBytes")
	}
	if config.KeepRecentBytes >= config.TriggerBytes || config.TriggerBytes >= config.HardLimitBytes {
		return nil, errors.New("Compaction requires KeepRecentBytes < TriggerBytes < HardLimitBytes")
	}
	if config.ContextWindowTokens < 0 || config.ReservedTokens < 0 || config.MinimumChangeTokens < 0 {
		return nil, errors.New("Compaction token limits cannot be negative")
	}
	if config.ContextWindowTokens > 0 {
		if config.TriggerRatio <= 0 || config.TriggerRatio >= 1 {
			config.TriggerRatio = .85
		}
		if config.RecoveryBand <= 0 || config.RecoveryBand > 1 {
			config.RecoveryBand = .80
		}
		if config.MinimumChangeTokens == 0 {
			config.MinimumChangeTokens = max(256, config.ContextWindowTokens/100)
		}
	}
	encoded, _ := json.Marshal(struct {
		Summarizer          agent.CapabilityIdentity
		TriggerBytes        int
		KeepRecentBytes     int
		KeepRecentTurns     int
		HardLimitBytes      int
		SummaryLimitBytes   int
		ContextWindowTokens int
		ReservedTokens      int
		TriggerRatio        float64
		RecoveryBand        float64
		MinimumChangeTokens int
	}{config.Summarizer.Identity(), config.TriggerBytes, config.KeepRecentBytes, config.KeepRecentTurns, config.HardLimitBytes, config.SummaryLimitBytes,
		config.ContextWindowTokens, config.ReservedTokens, config.TriggerRatio, config.RecoveryBand, config.MinimumChangeTokens})
	digest := sha256.Sum256(encoded)
	return &standardManager{config: config, identity: agent.CapabilityIdentity{
		Kind: "compaction.standard", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}}, nil
}

func (definition *standardDefinition) InitializeDefinition(context.Context) error {
	if definition == nil {
		return errors.New("standard Compaction Definition is nil")
	}
	definition.once.Do(func() {
		definition.value, definition.err = newStandard(definition.config)
	})
	return definition.err
}

func (definition *standardDefinition) Identity() agent.CapabilityIdentity {
	if err := definition.InitializeDefinition(context.Background()); err != nil {
		return agent.CapabilityIdentity{}
	}
	return definition.value.Identity()
}

func (definition *standardDefinition) SummaryLimitBytes() int {
	if err := definition.InitializeDefinition(context.Background()); err != nil {
		return 0
	}
	return definition.value.SummaryLimitBytes()
}

func (definition *standardDefinition) Plan(
	ctx context.Context,
	request agent.CompactionPlanRequest,
) (agent.CompactionPlan, error) {
	if err := definition.InitializeDefinition(ctx); err != nil {
		return agent.CompactionPlan{}, err
	}
	return definition.value.Plan(ctx, request)
}

func (definition *standardDefinition) Compact(
	ctx context.Context,
	request agent.CompactionCompactRequest,
) (agent.CompactionCheckpoint, error) {
	if err := definition.InitializeDefinition(ctx); err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	return definition.value.Compact(ctx, request)
}

var _ agent.CompactionManager = (*standardDefinition)(nil)
var _ agent.DefinitionInitializer = (*standardDefinition)(nil)

func (manager *standardManager) Identity() agent.CapabilityIdentity { return manager.identity }

func (manager *standardManager) SummaryLimitBytes() int {
	if manager == nil {
		return 0
	}
	return manager.config.SummaryLimitBytes
}

func (manager *standardManager) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	if request.LifecycleReservedTokens < 0 || request.LifecycleReservedTokens > int(^uint(0)>>1)-manager.config.ReservedTokens {
		return agent.CompactionPlan{}, errors.New("Compaction lifecycle token reserve is invalid")
	}
	reservedTokens := manager.config.ReservedTokens + request.LifecycleReservedTokens
	if request.ModelSnapshot != nil {
		if maxTokens := request.ModelSnapshot.ResolvedOptions().MaxTokens; maxTokens != nil {
			reservedTokens = agent.CapacityAwareTokenReserve(
				reservedTokens, *maxTokens, manager.config.ContextWindowTokens, manager.config.TriggerRatio,
			)
		}
	}
	bytes := messageBytes(request.ModelRequest)
	if len(request.ModelRequest) == 0 {
		bytes = messageBytes(request.Messages)
	}
	metrics := compactionPlanMetrics(request)
	policy := agent.CompactionValidationPolicy{
		ContextWindowTokens: manager.config.ContextWindowTokens,
		ReservedTokens:      reservedTokens,
		Threshold:           manager.config.TriggerRatio,
		RecoveryBand:        manager.config.RecoveryBand,
		MinimumChangeTokens: manager.config.MinimumChangeTokens,
		HardLimitBytes:      manager.config.HardLimitBytes,
	}
	metrics.ReservedTokens = policy.ReservedTokens
	metrics.ContextWindowTokens = policy.ContextWindowTokens
	metrics.Threshold = policy.Threshold
	metrics.RecoveryBand = policy.RecoveryBand
	metrics.ProjectedTokensBefore = metrics.CalibratedTokens(metrics.EstimatedTokensBefore) + policy.ReservedTokens
	triggered := bytes > manager.config.TriggerBytes
	if manager.config.ContextWindowTokens > 0 {
		trigger := int(float64(manager.config.ContextWindowTokens) * manager.config.TriggerRatio)
		triggered = metrics.ProjectedTokensBefore >= trigger
	}
	if !request.Force && !triggered {
		return agent.CompactionPlan{Action: agent.CompactionNone, SkippedReason: "below_trigger", Validation: policy, Metrics: metrics}, nil
	}
	if bytes > manager.config.HardLimitBytes && len(request.Messages) < 2 {
		return agent.CompactionPlan{}, fmt.Errorf("%w: %d bytes exceed the %d-byte Compaction limit", agent.ErrContextLimit, bytes, manager.config.HardLimitBytes)
	}
	keep := 0
	sourceTo := len(request.Messages)
	for sourceTo > 1 && keep < manager.config.KeepRecentBytes {
		sourceTo--
		keep += messageBytes(request.Messages[sourceTo : sourceTo+1])
	}
	// A checkpoint range ends at a complete user-turn boundary. This keeps an
	// assistant tool-call batch and every result on the same side of the
	// replacement, even when the byte target lands inside a very large batch.
	// Move forward to the next boundary rather than retaining half of the batch;
	// KeepRecentTurns remains the non-negotiable semantic minimum.
	turnBoundary := retainedTurnBoundary(request.Messages, manager.config.KeepRecentTurns)
	aligned := -1
	for index := sourceTo; index < len(request.Messages); index++ {
		if request.Messages[index] != nil && request.Messages[index].Role == agent.User && !agent.IsContextStateMessage(request.Messages[index]) {
			aligned = index
			break
		}
	}
	if aligned < 0 || turnBoundary < aligned {
		aligned = turnBoundary
	}
	sourceTo = aligned
	newSourceTokens := 0
	if request.Present && request.Current.ReplacementTo >= 0 && request.Current.ReplacementTo < sourceTo && sourceTo <= len(request.Messages) {
		newSourceTokens = cleanup.EstimateMessages(request.Messages[request.Current.ReplacementTo:sourceTo])
	}
	if !request.Force && request.Present && !request.Current.Removed &&
		request.Current.ReplacementFrom == 0 && newSourceTokens < manager.config.MinimumChangeTokens &&
		request.Current.Metrics.Degraded &&
		request.Current.Metrics.CandidateFingerprint != "" &&
		request.Current.Metrics.CandidateFingerprint == metrics.CandidateFingerprint &&
		request.Current.Metrics.CandidateGeneration == metrics.CandidateGeneration {
		// The active checkpoint already covers every raw message that can be
		// removed without entering the protected recent suffix. Retrying it
		// cannot reduce the provider-visible request.
		return agent.CompactionPlan{Action: agent.CompactionNone, SkippedReason: "degraded_no_progress_latch", Validation: policy, Metrics: metrics}, nil
	}
	if sourceTo <= 0 {
		if bytes > manager.config.HardLimitBytes {
			return agent.CompactionPlan{}, fmt.Errorf("%w: final model request cannot be reduced below the %d-byte limit", agent.ErrContextLimit, manager.config.HardLimitBytes)
		}
		return agent.CompactionPlan{Action: agent.CompactionNone, SkippedReason: "insufficient_history", Validation: policy, Metrics: metrics}, nil
	}
	metrics.SourceMessageCount = sourceTo
	return agent.CompactionPlan{
		Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: sourceTo,
		Validation: policy, Metrics: metrics,
	}, nil
}

func compactionPlanMetrics(request agent.CompactionPlanRequest) agent.CompactionMetrics {
	messages := request.ModelRequest
	if len(messages) == 0 {
		messages = request.Messages
	}
	estimated := cleanup.EstimateTokens(messages, request.ModelSnapshot)
	observed, observedEstimate, cached := latestPromptUsage(messages, request.ModelSnapshot)
	metrics := agent.CompactionMetrics{
		EstimatedTokensBefore: estimated, ObservedPromptTokens: observed, ObservedEstimateTokens: observedEstimate,
		MessageCountBefore: len(messages), CacheReadTokens: cached,
	}
	metrics.ProjectedTokensBefore = metrics.CalibratedTokens(estimated)
	if request.ModelSnapshot != nil {
		boundary := min(request.ModelSnapshot.StablePrefixMessages(), len(messages))
		metrics.StablePrefixTokens = cleanup.EstimateTokens(messages[:boundary], request.ModelSnapshot)
		metrics.CacheExpectedPrefixTokens = metrics.StablePrefixTokens
	}
	metrics.CandidateFingerprint, metrics.CandidateGeneration = candidateIdentity(messages)
	return metrics
}

func latestPromptUsage(messages []*agent.Message, snapshot *agent.ModelRequestSnapshot) (prompt, estimated, cached int) {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil || message.ResponseMeta.Usage.PromptTokens <= 0 {
			continue
		}
		return message.ResponseMeta.Usage.PromptTokens,
			cleanup.EstimateTokens(messages[:index], snapshot),
			message.ResponseMeta.Usage.PromptTokenDetails.CachedTokens
	}
	return 0, 0, 0
}

func candidateIdentity(messages []*agent.Message) (string, uint64) {
	type candidate struct {
		Index  int    `json:"index"`
		CallID string `json:"call_id,omitempty"`
		Tool   string `json:"tool,omitempty"`
		Bytes  int    `json:"bytes"`
	}
	values := make([]candidate, 0)
	for index, message := range messages {
		if message == nil || message.Role != agent.ToolRole {
			continue
		}
		values = append(values, candidate{Index: index, CallID: message.ToolCallID, Tool: message.ToolName, Bytes: len(message.Content)})
	}
	encoded, _ := json.Marshal(values)
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), uint64(len(values))
}

func (manager *standardManager) Compact(ctx context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	if request.Plan.SourceFrom < 0 || request.Plan.SourceTo > len(request.Messages) || request.Plan.SourceTo <= request.Plan.SourceFrom {
		return agent.CompactionCheckpoint{}, errors.New("Compaction source range is invalid")
	}
	source := request.SourceMessages
	if len(source) == 0 {
		source = request.Messages[request.Plan.SourceFrom:request.Plan.SourceTo]
	}
	summary, err := manager.config.Summarizer.Summarize(ctx, SummaryRequest{
		Session: request.Session, Run: request.Run,
		Messages:      cloneMessages(source),
		ModelRequest:  cloneMessages(request.ModelRequest),
		ModelSnapshot: request.ModelSnapshot,
		Plan:          request.Plan,
	})
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	summary.Content = mergeProtectedReceiptContext(
		summary.Content, request.Current.Summary,
		request.Messages[request.Plan.SourceFrom:request.Plan.SourceTo],
		manager.config.SummaryLimitBytes,
	)
	summary.Content = strings.TrimSpace(summary.Content)
	if summary.Content == "" || summary.TokenEstimate < 0 {
		return agent.CompactionCheckpoint{}, errors.New("Compaction Summarizer returned an invalid result")
	}
	if len(summary.Content) > manager.config.SummaryLimitBytes {
		return agent.CompactionCheckpoint{}, fmt.Errorf("%w: Compaction summary is %d bytes and exceeds the target Agent limit %d", agent.ErrContextLimit, len(summary.Content), manager.config.SummaryLimitBytes)
	}
	return agent.CompactionCheckpoint{Summary: summary.Content, TokenEstimate: summary.TokenEstimate}, nil
}

func retainedTurnBoundary(messages []*agent.Message, turns int) int {
	seen := 0
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] == nil || messages[index].Role != agent.User || agent.IsContextStateMessage(messages[index]) {
			continue
		}
		seen++
		if seen == turns {
			return index
		}
	}
	return 0
}

type disabledManager struct {
	hardLimit    int
	summaryLimit int
	identity     agent.CapabilityIdentity
}

func Disabled(hardLimitBytes, summaryLimitBytes int) agent.CompactionManager {
	return &disabledManager{hardLimit: hardLimitBytes, summaryLimit: summaryLimitBytes, identity: agent.CapabilityIdentity{
		Kind: "compaction.disabled", Version: 1, ConfigHash: fmt.Sprintf("input:%d;summary:%d", hardLimitBytes, summaryLimitBytes),
	}}
}

func (manager *disabledManager) Identity() agent.CapabilityIdentity { return manager.identity }

func (manager *disabledManager) SummaryLimitBytes() int {
	if manager == nil {
		return 0
	}
	return manager.summaryLimit
}

func (manager *disabledManager) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	bytes := messageBytes(request.Messages)
	if bytes > manager.hardLimit {
		return agent.CompactionPlan{}, fmt.Errorf("%w: %d bytes exceed disabled Compaction limit %d", agent.ErrContextLimit, bytes, manager.hardLimit)
	}
	return agent.CompactionPlan{Action: agent.CompactionNone, SkippedReason: "disabled"}, nil
}

func (*disabledManager) Compact(context.Context, agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	return agent.CompactionCheckpoint{}, agent.ErrCapabilityUnsupported
}

func messageBytes(messages []*agent.Message) int {
	encoded, _ := json.Marshal(messages)
	return len(encoded)
}

func cloneMessages(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}

func validateIdentity(identity agent.CapabilityIdentity) error {
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return errors.New("capability identity is incomplete")
	}
	return nil
}

var _ agent.CompactionManager = (*standardManager)(nil)
var _ agent.CompactionManager = (*disabledManager)(nil)
