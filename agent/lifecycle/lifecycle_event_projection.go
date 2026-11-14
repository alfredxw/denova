package lifecycle

import (
	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentengine "github.com/alfredxw/denova/agent/engine"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
)

func publicCompactionMetrics(metrics agentengine.CompactionMetrics) agenthistory.CompactionMetrics {
	return agenthistory.CompactionMetrics{
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

func publicEventSource(source agentengine.EventSource) agentevent.EventSource {
	return agentevent.EventSource{
		Name: source.Name, Path: append([]string(nil), source.Path...),
		InvocationID: source.InvocationID, InvocationType: source.InvocationType,
	}
}
