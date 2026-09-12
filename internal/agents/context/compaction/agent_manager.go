package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/modelio"
	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
	publiccompaction "github.com/alfredxw/denova/agent/compaction"
)

// denovaManager applies the same product visibility policy to the selected
// source as the primary model request. Agent owns planning and model execution.
type denovaManager struct {
	delegate          agent.CompactionManager
	toolContextPolicy toolresult.ContextPolicy
	identity          agent.CapabilityIdentity
	initializeOnce    sync.Once
	initializeErr     error
}

func newDenovaManager(delegate agent.CompactionManager, policy toolresult.ContextPolicy) agent.CompactionManager {
	if delegate == nil {
		return nil
	}
	return &denovaManager{
		delegate: delegate, toolContextPolicy: policy,
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
			Delegate    agent.CapabilityIdentity
			ToolContext toolresult.ContextPolicy
		}{manager.delegate.Identity(), manager.toolContextPolicy})
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
	return manager.delegate.Plan(ctx, request)
}

func (manager *denovaManager) Compact(
	ctx context.Context,
	request agent.CompactionCompactRequest,
) (agent.CompactionCheckpoint, error) {
	if err := manager.InitializeDefinition(ctx); err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	request.Messages = toolresult.ApplyContextPolicy(request.Messages, manager.toolContextPolicy)
	return manager.delegate.Compact(ctx, request)
}

// NewAgentManager adapts Denova's context policy and model to the public Agent
// Compaction capability. Agent owns checkpoint durability and recovery; this
// package owns only Denova's planning configuration and summary semantics.
func NewAgentManager(
	cfg *config.Config,
	agentKind string,
) (agent.CompactionManager, error) {
	modelSettings := config.ResolveAgentModel(cfg, agentKind)
	return NewAgentManagerForModel(cfg, agentKind, modelSettings.ContextWindowTokens)
}

// NewAgentManagerForModel applies policy from policyKind while sizing every
// pressure, reserve, validation, and summarization decision for the concrete
// model used by this Definition. It is the correct seam for child Agents that
// inherit a product policy but override their model.
func NewAgentManagerForModel(
	cfg *config.Config,
	policyKind string,
	contextWindowTokens int,
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
		Prompt:       compactionDomainRequirements(policyKind) + "\n" + agentcontext.CompactionCheckpointSchema() + "\n" + settings.CheckpointGuidance,
		Execution:    modelio.ModelExecutionPolicy(cfg),
		TriggerBytes: trigger, KeepRecentBytes: keepRecent, HardLimitBytes: hardLimit,
		SummaryLimitBytes:   summaryLimit,
		ContextWindowTokens: contextWindowTokens,
		ReservedTokens:      completionReserve + toolReserve,
		TriggerRatio:        settings.CompactionThreshold,
		RecoveryBand:        config.DefaultContextCompactionRecoveryBand,
		MinimumChangeTokens: max(256, contextWindowTokens/100),
	})
	return newDenovaManager(manager, toolresult.ResolveContextPolicy(cfg, policyKind)), nil
}

func capabilityIdentity(kind string, configuration any) agent.CapabilityIdentity {
	encoded, _ := json.Marshal(configuration)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{Kind: kind, Version: 1, ConfigHash: hex.EncodeToString(digest[:])}
}

var _ agent.CompactionManager = (*denovaManager)(nil)
var _ agent.DefinitionInitializer = (*denovaManager)(nil)
