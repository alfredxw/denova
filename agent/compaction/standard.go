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
)

// SummaryRequest is selected semantic source plus the exact active model and
// output limits. Summarizers may use a different model, return structured state,
// or implement an extractive algorithm without implementing planning.
type SummaryRequest struct {
	Session             agent.SessionView
	Run                 agent.RunView
	Messages            []*agent.Message
	ModelSnapshot       *agent.ModelRequestSnapshot
	Current             *agent.CompactionState
	SummaryLimitBytes   int
	HardLimitBytes      int
	ContextWindowTokens int
}

type Summarizer interface {
	Identity() agent.CapabilityIdentity
	Summarize(context.Context, SummaryRequest) (agent.CompactionCheckpoint, error)
}

type SummarizerFunc struct {
	Capability agent.CapabilityIdentity
	Func       func(context.Context, SummaryRequest) (agent.CompactionCheckpoint, error)
}

func (s SummarizerFunc) Identity() agent.CapabilityIdentity { return s.Capability }
func (s SummarizerFunc) Summarize(ctx context.Context, request SummaryRequest) (agent.CompactionCheckpoint, error) {
	if s.Func == nil {
		return agent.CompactionCheckpoint{}, errors.New("Compaction Summarizer function is nil")
	}
	return s.Func(ctx, request)
}

type StandardConfig struct {
	Summarizer Summarizer
	// Prompt adds domain guidance to the built-in summary instruction.
	Prompt            string
	Execution         agent.ExecutionPolicy
	TriggerBytes      int
	KeepRecentBytes   int
	KeepRecentGroups  int
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
		if config.ContextWindowTokens <= 0 {
			return nil, errors.New("built-in Compaction requires ContextWindowTokens")
		}
		var err error
		config.Summarizer, err = ModelSummarizer(ModelSummarizerConfig{Prompt: config.Prompt, Execution: config.Execution})
		if err != nil {
			return nil, err
		}
	}
	if err := validateIdentity(config.Summarizer.Identity()); err != nil {
		return nil, fmt.Errorf("Compaction Summarizer: %w", err)
	}
	if config.TriggerBytes <= 0 {
		config.TriggerBytes = 2 << 20
	}
	if config.KeepRecentBytes <= 0 {
		config.KeepRecentBytes = 512 << 10
		if config.ContextWindowTokens > 0 {
			config.KeepRecentBytes = min(config.KeepRecentBytes, max(1, config.ContextWindowTokens*4/5))
		}
	}
	if config.KeepRecentGroups <= 0 {
		config.KeepRecentGroups = 1
	}
	if config.HardLimitBytes <= 0 {
		config.HardLimitBytes = 8 << 20
	}
	if config.SummaryLimitBytes == 0 {
		config.SummaryLimitBytes = min(64<<10, config.HardLimitBytes)
	}
	if config.SummaryLimitBytes < 0 || config.SummaryLimitBytes > config.HardLimitBytes {
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
		KeepRecentGroups    int
		HardLimitBytes      int
		SummaryLimitBytes   int
		ContextWindowTokens int
		ReservedTokens      int
		TriggerRatio        float64
		RecoveryBand        float64
		MinimumChangeTokens int
	}{config.Summarizer.Identity(), config.TriggerBytes, config.KeepRecentBytes, config.KeepRecentGroups, config.HardLimitBytes, config.SummaryLimitBytes,
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
	bytes := messageBytes(request.ModelSnapshot.Messages())
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
		if _, builtin := manager.config.Summarizer.(*modelSummarizer); builtin {
			output, safety := summaryReserves(manager.config.ContextWindowTokens, manager.config.SummaryLimitBytes)
			triggered = triggered || metrics.ProjectedTokensBefore+2048+output+safety >= manager.config.ContextWindowTokens
		}
	}
	if !request.Force && !triggered {
		return agent.CompactionPlan{Action: agent.CompactionNone, SkippedReason: "below_trigger", Validation: policy, Metrics: metrics}, nil
	}
	count := max(0, len(request.Groups)-max(0, manager.config.KeepRecentGroups-1))
	kept := request.RetainedBytes
	for count > 0 && kept < manager.config.KeepRecentBytes {
		size := messageBytes(request.Groups[count-1].Messages)
		if kept+size > manager.config.KeepRecentBytes {
			break
		}
		kept += size
		count--
	}
	// A forced command may relax the optional byte tail, but never the
	// runtime-protected newest step or explicitly retained additional groups.
	if count == 0 && request.Force && len(request.Groups) > max(0, manager.config.KeepRecentGroups-1) {
		count = 1
	}
	if count == 0 {
		if bytes > manager.config.HardLimitBytes {
			return agent.CompactionPlan{}, fmt.Errorf("%w: no eligible Compaction groups", agent.ErrContextLimit)
		}
		return agent.CompactionPlan{Action: agent.CompactionNone, SkippedReason: "no_new_complete_groups", Validation: policy, Metrics: metrics}, nil
	}
	return agent.CompactionPlan{Action: agent.CompactionCreate, GroupCount: count, Validation: policy, Metrics: metrics}, nil
}

func compactionPlanMetrics(request agent.CompactionPlanRequest) agent.CompactionMetrics {
	messages := request.ModelSnapshot.Messages()
	estimated := estimateSnapshotTokens(messages, request.ModelSnapshot)
	observed, observedEstimate, cached := latestPromptUsage(messages, request.ModelSnapshot)
	metrics := agent.CompactionMetrics{
		EstimatedTokensBefore: estimated, ObservedPromptTokens: observed, ObservedEstimateTokens: observedEstimate,
		MessageCountBefore: len(messages), CacheReadTokens: cached,
	}
	metrics.ProjectedTokensBefore = metrics.CalibratedTokens(estimated)
	if request.ModelSnapshot != nil {
		boundary := min(request.ModelSnapshot.StablePrefixMessages(), len(messages))
		metrics.StablePrefixTokens = estimateSnapshotTokens(messages[:boundary], request.ModelSnapshot)
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
			estimateSnapshotTokens(messages[:index], snapshot),
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
	return manager.config.Summarizer.Summarize(ctx, SummaryRequest{
		Session: request.Session, Run: request.Run, Messages: cloneMessages(request.Messages),
		ModelSnapshot: request.ModelSnapshot, Current: request.Current,
		SummaryLimitBytes: manager.config.SummaryLimitBytes, HardLimitBytes: manager.config.HardLimitBytes,
		ContextWindowTokens: manager.config.ContextWindowTokens,
	})
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
	bytes := messageBytes(request.ModelSnapshot.Messages())
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

func estimateSnapshotTokens(messages []*agent.Message, snapshot *agent.ModelRequestSnapshot) int {
	var tools []*agent.ToolInfo
	if snapshot != nil {
		tools = snapshot.ResolvedOptions().Tools
	}
	return agent.EstimateRequestTokens(messages, tools)
}
