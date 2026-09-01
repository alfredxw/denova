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

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
	publiccompaction "github.com/alfredxw/denova/agent/compaction"
)

type denovaSummarizer struct {
	cfg                 *config.Config
	model               agent.BaseChatModel
	modelIdentity       agent.CapabilityIdentity
	agentKind           string
	contextWindowTokens int
}

// denovaManager preserves the generic manager contract while adding Denova's
// cache-safety preflight. It advances a checkpoint before the exact primary
// request no longer has room for the appended summary instruction and output
// reserve, avoiding an unnecessary cold-prefix fallback.
type denovaManager struct {
	delegate            agent.CompactionManager
	cfg                 *config.Config
	agentKind           string
	contextWindowTokens int
	toolContextPolicy   toolresult.ContextPolicy
	identity            agent.CapabilityIdentity
	initializeOnce      sync.Once
	initializeErr       error
}

func newDenovaManager(delegate agent.CompactionManager, cfg *config.Config, agentKind string, contextWindowTokens int) agent.CompactionManager {
	if delegate == nil {
		return nil
	}
	return &denovaManager{
		delegate: delegate, cfg: cfg, agentKind: agentKind, contextWindowTokens: contextWindowTokens,
		toolContextPolicy: toolresult.ResolveContextPolicy(cfg, agentKind),
	}
}

func (manager *denovaManager) InitializeDefinition(ctx context.Context) error {
	if manager == nil || manager.delegate == nil {
		return errors.New("Denova Compaction manager delegate is required")
	}
	manager.initializeOnce.Do(func() {
		if initializer, ok := manager.delegate.(agent.DefinitionInitializer); ok {
			if err := initializer.InitializeDefinition(ctx); err != nil {
				manager.initializeErr = fmt.Errorf("initialize delegate: %w", err)
				return
			}
		}
		manager.identity = capabilityIdentity("denova.compaction.manager", struct {
			Delegate            agent.CapabilityIdentity
			AgentKind           string
			ContextWindowTokens int
			ToolContext         toolresult.ContextPolicy
		}{manager.delegate.Identity(), manager.agentKind, manager.contextWindowTokens, manager.toolContextPolicy})
	})
	return manager.initializeErr
}

func (manager *denovaManager) Identity() agent.CapabilityIdentity {
	if err := manager.InitializeDefinition(context.Background()); err != nil {
		return agent.CapabilityIdentity{}
	}
	return manager.identity
}

func (manager *denovaManager) SummaryLimitBytes() int {
	if err := manager.InitializeDefinition(context.Background()); err != nil {
		return 0
	}
	return manager.delegate.SummaryLimitBytes()
}

func (manager *denovaManager) Plan(
	ctx context.Context,
	request agent.CompactionPlanRequest,
) (agent.CompactionPlan, error) {
	if err := manager.InitializeDefinition(ctx); err != nil {
		return agent.CompactionPlan{}, err
	}
	plan, err := manager.delegate.Plan(ctx, request)
	if err != nil || plan.Action != agent.CompactionNone || plan.SkippedReason != "below_trigger" || request.Force || request.ModelSnapshot == nil {
		return plan, err
	}
	options := request.ModelSnapshot.ResolvedOptions()
	window := manager.contextWindowTokens
	completionReserve, toolReserve := EstimateProjectionReservesForModel(manager.cfg, manager.agentKind, 0, window)
	observedPromptTokens, _ := LatestPromptUsageCalibration(request.ModelRequest, options.Tools)
	policy := ForkCapacityPolicy{
		AgentKind: manager.agentKind, ContextWindowTokens: window,
		ReservedTokens: completionReserve + toolReserve, ObservedPromptTokens: observedPromptTokens,
		CompactionPromptTokens:  ForkPromptReserve,
		CheckpointOutputReserve: max(1024, min(8192, window/25)),
		SafetyMarginTokens:      max(512, window/100),
	}
	if !ForkCapacityPressure(request.ModelRequest, options.Tools, policy, options) {
		return plan, nil
	}
	forced := request
	forced.Force = true
	return manager.delegate.Plan(ctx, forced)
}

func (manager *denovaManager) Compact(
	ctx context.Context,
	request agent.CompactionCompactRequest,
) (agent.CompactionCheckpoint, error) {
	if err := manager.InitializeDefinition(ctx); err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	// SourceFrom/SourceTo and SourceHash remain authenticated raw transcript
	// coordinates. Only the summarizer input is projected through the exact
	// product visibility policy, so a checkpoint cannot resurrect tool bodies
	// hidden from the provider while removal still restores complete raw data.
	request.SourceMessages = toolresult.ApplyContextPolicy(
		request.SourceMessages, manager.toolContextPolicy,
	)
	return manager.delegate.Compact(ctx, request)
}

func (summarizer denovaSummarizer) Identity() agent.CapabilityIdentity {
	checkpointGuidance := config.ResolveAgentContext(summarizer.cfg, summarizer.agentKind).CheckpointGuidance
	return capabilityIdentity("denova.compaction.summarizer", struct {
		Model               agent.CapabilityIdentity
		AgentKind           string
		ContextWindowTokens int
		CheckpointGuidance  string
	}{summarizer.modelIdentity, summarizer.agentKind, summarizer.contextWindowTokens, checkpointGuidance})
}

func (summarizer denovaSummarizer) Summarize(
	ctx context.Context,
	request publiccompaction.SummaryRequest,
) (publiccompaction.Summary, error) {
	if summarizer.model == nil {
		return publiccompaction.Summary{}, errors.New("source Agent model is unavailable for context checkpoint fallback")
	}
	policy := ResolvePolicy(summarizer.cfg, summarizer.agentKind)
	policy.ContextWindowTokens = summarizer.contextWindowTokens
	source := cloneMessages(request.Messages)
	sourceTokens := agentcontext.EstimateTokens(source, nil)
	coldFallback := func(
		forkCtx context.Context,
		_ *config.Config,
		input SummaryRequest,
		_ func(int, string),
	) (string, error) {
		messages := cloneMessages(input.Messages)
		result, err := summarizer.model.Generate(forkCtx, messages, agent.WithToolChoice(agent.ToolChoiceForbidden))
		if err != nil {
			return "", err
		}
		if result == nil || strings.TrimSpace(result.Content) == "" || len(result.ToolCalls) != 0 {
			return "", errors.New("source Agent model returned an invalid context checkpoint")
		}
		return strings.TrimSpace(result.Content), nil
	}
	coldReason := ""
	if request.ModelSnapshot == nil {
		// Explicit structural compaction has no active provider call to fork.
		// Preserve functionality through the bounded layered path; automatic
		// compaction always supplies an exact snapshot.
		coldReason = FallbackNoSnapshot
	}
	content, _, _, err := summarizeContextInLayers(
		ctx,
		summarizer.cfg,
		summarizer.agentKind,
		"",
		source,
		source,
		"",
		sourceTokens,
		policy,
		request.ModelSnapshot,
		coldReason,
		coldFallback,
		nil,
	)
	if err != nil {
		return publiccompaction.Summary{}, fmt.Errorf("generate Denova Compaction checkpoint: %w", err)
	}
	content = strings.TrimSpace(content)
	return publiccompaction.Summary{
		Content: content, TokenEstimate: agentcontext.EstimateStringTokens(content),
	}, nil
}

// NewAgentManager adapts Denova's context policy and model to the public Agent
// Compaction capability. Agent owns checkpoint durability and recovery; this
// package owns only Denova's planning configuration and summary semantics.
func NewAgentManager(
	cfg *config.Config,
	agentKind string,
	model agent.BaseChatModel,
	modelIdentity agent.CapabilityIdentity,
) (agent.CompactionManager, error) {
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return NewAgentManagerForModel(cfg, agentKind, modelSettings.ContextWindowTokens, model, modelIdentity)
}

// NewAgentManagerForModel applies policy from policyKind while sizing every
// pressure, reserve, validation, and summarization decision for the concrete
// model used by this Definition. It is the correct seam for child Agents that
// inherit a product policy but override their model.
func NewAgentManagerForModel(
	cfg *config.Config,
	policyKind string,
	contextWindowTokens int,
	model agent.BaseChatModel,
	modelIdentity agent.CapabilityIdentity,
) (agent.CompactionManager, error) {
	if contextWindowTokens <= 0 {
		return nil, errors.New("Denova Compaction context window must be positive")
	}
	settings := config.ResolveAgentContext(cfg, policyKind)
	hardLimit := settings.MaxProviderInputBytes
	summaryLimit := min(settings.MaxFragmentBytes, settings.MaxTotalInjectedBytes, settings.MaxProviderInputBytes)
	if !settings.CompactionEnabled {
		return publiccompaction.Disabled(hardLimit, summaryLimit), nil
	}
	completionReserve, toolReserve := EstimateProjectionReservesForModel(cfg, policyKind, 0, contextWindowTokens)
	trigger := int(float64(contextWindowTokens*4) * settings.CompactionThreshold)
	if trigger <= 0 || trigger >= hardLimit {
		trigger = int(float64(hardLimit) * settings.CompactionThreshold)
	}
	keepRecent := max(64<<10, trigger/5)
	if keepRecent >= trigger {
		keepRecent = trigger / 2
	}
	manager := publiccompaction.Standard(publiccompaction.StandardConfig{
		Summarizer: denovaSummarizer{
			cfg: cfg, model: model, modelIdentity: modelIdentity, agentKind: policyKind,
			contextWindowTokens: contextWindowTokens,
		},
		TriggerBytes: trigger, KeepRecentBytes: keepRecent, HardLimitBytes: hardLimit,
		SummaryLimitBytes:   summaryLimit,
		ContextWindowTokens: contextWindowTokens,
		ReservedTokens:      completionReserve + toolReserve,
		TriggerRatio:        settings.CompactionThreshold,
		RecoveryBand:        config.DefaultContextCompactionRecoveryBand,
		MinimumChangeTokens: max(256, contextWindowTokens/100),
	})
	return newDenovaManager(manager, cfg, policyKind, contextWindowTokens), nil
}

func cloneMessages(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}

func capabilityIdentity(kind string, configuration any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(configuration)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

var _ publiccompaction.Summarizer = denovaSummarizer{}
var _ agent.CompactionManager = (*denovaManager)(nil)
var _ agent.DefinitionInitializer = (*denovaManager)(nil)
