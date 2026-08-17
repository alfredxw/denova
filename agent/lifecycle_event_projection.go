package agent

import (
	"encoding/json"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
)

func publicCleanupMetrics(metrics runstate.CleanupMetrics) CleanupMetrics {
	return CleanupMetrics{
		EstimatedTokensBefore: metrics.EstimatedTokensBefore, LocalProjectedTokens: metrics.LocalProjectedTokens,
		ObservedPromptTokens: metrics.ObservedPromptTokens, EffectiveTokens: metrics.EffectiveTokens,
		EstimatedTokensAfter: metrics.EstimatedTokensAfter, ReclaimedTokens: metrics.ReclaimedTokens,
		ContextWindowTokens: metrics.ContextWindowTokens, PressureBefore: metrics.PressureBefore,
		PressureAfter: metrics.PressureAfter, BodyPressureBefore: metrics.BodyPressureBefore,
		BodyPressureAfter: metrics.BodyPressureAfter, StablePrefixTokens: metrics.StablePrefixTokens,
		CandidateTokens: metrics.CandidateTokens, CacheViableCandidateTokens: metrics.CacheViableCandidateTokens,
		SkippedBelowMinimumCount: metrics.SkippedBelowMinimumCount, SkippedWarmSuffixCount: metrics.SkippedWarmSuffixCount,
		EagerCandidateCount: metrics.EagerCandidateCount, EagerSelectedCount: metrics.EagerSelectedCount,
		SupersededCandidateCount: metrics.SupersededCandidateCount, DiscardableCandidateCount: metrics.DiscardableCandidateCount,
		MinimumCleanupTokens: metrics.MinimumCleanupTokens, ProtectedResults: metrics.ProtectedResults,
		EarliestChanged: metrics.EarliestChanged, WarmSuffixTokens: metrics.WarmSuffixTokens,
		PlaceholderTokens: metrics.PlaceholderTokens, ReplacementCount: metrics.ReplacementCount,
		EagerOnly: metrics.EagerOnly, PressureScope: metrics.PressureScope,
		ProviderCacheState: metrics.ProviderCacheState, ExecutionMode: metrics.ExecutionMode,
		RendererVersion: metrics.RendererVersion,
	}
}

func publicCompactionMetrics(metrics runstate.CompactionMetrics) CompactionMetrics {
	return CompactionMetrics{
		EstimatedTokensBefore: metrics.EstimatedTokensBefore, ObservedPromptTokens: metrics.ObservedPromptTokens,
		ObservedEstimateTokens: metrics.ObservedEstimateTokens, EstimatedTokensAfter: metrics.EstimatedTokensAfter,
		ProjectedTokensBefore: metrics.ProjectedTokensBefore, ProjectedTokensAfter: metrics.ProjectedTokensAfter,
		ReservedTokens: metrics.ReservedTokens, ContextWindowTokens: metrics.ContextWindowTokens,
		Threshold: metrics.Threshold, RecoveryBand: metrics.RecoveryBand,
		RecoveryTargetTokens: metrics.RecoveryTargetTokens, RecoveryBandMet: metrics.RecoveryBandMet,
		Degraded: metrics.Degraded, StablePrefixTokens: metrics.StablePrefixTokens,
		SourceMessageCount: metrics.SourceMessageCount, MessageCountBefore: metrics.MessageCountBefore,
		MessageCountAfter: metrics.MessageCountAfter, CacheExpectedPrefixTokens: metrics.CacheExpectedPrefixTokens,
		CacheReadTokens: metrics.CacheReadTokens, CandidateFingerprint: metrics.CandidateFingerprint,
		CandidateGeneration: metrics.CandidateGeneration,
	}
}

func decodeToolDescriptorMetadata(metadata json.RawMessage) *ToolDescriptor {
	if len(metadata) == 0 {
		return nil
	}
	var decoded struct {
		ToolDescriptor
		Presentation ToolPresentation `json:"presentation"`
	}
	if err := json.Unmarshal(metadata, &decoded); err != nil || decoded.Execution == "" {
		return nil
	}
	presentation, err := decoded.Presentation.Normalize()
	if err != nil {
		return nil
	}
	decoded.ToolDescriptor.Presentation = presentation
	return &decoded.ToolDescriptor
}

func publicEventSource(source runstate.EventSource) EventSource {
	return EventSource{
		Name: source.Name, Path: append([]string(nil), source.Path...),
		InvocationID: source.InvocationID, InvocationType: source.InvocationType,
	}
}

func eventSourceEmpty(source EventSource) bool {
	return source.Name == "" && len(source.Path) == 0 && source.InvocationID == "" && source.InvocationType == ""
}
