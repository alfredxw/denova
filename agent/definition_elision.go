package agent

import (
	"context"
	"errors"
	"sort"
)

// prepareElision is a side-effect-free maintenance stage. It uses canonical
// coordinates for durability, then rebuilds through the active middleware and
// provider estimator. Only the caller may commit the returned projection.
func prepareElision(
	ctx context.Context,
	prepared preparedDefinition,
	raw []*Message,
	compaction compactionRecord,
	compactionPresent bool,
	before *ModelRequestSnapshot,
	buildAfter func(elisionRecord) (*preparedModelCall, error),
) (elisionRecord, *preparedModelCall, error) {
	policy := prepared.definition.Elision
	if policy == nil {
		return prepared.elision, nil, nil
	}
	size, err := before.EstimateInput()
	if err != nil {
		return elisionRecord{}, nil, err
	}
	calibration := elisionCalibration(before)
	reserve := policy.ReservedTokens + prepared.goalReservedTokens
	if output := before.ResolvedOptions().MaxTokens; output != nil {
		reserve = CapacityAwareTokenReserve(reserve, *output, policy.ContextWindowTokens, policy.TriggerRatio)
	}
	pressure := calibration.CalibratedTokens(size.Tokens) + reserve
	if pressure < int(float64(policy.ContextWindowTokens)*policy.TriggerRatio) {
		return prepared.elision, nil, nil
	}
	projected, err := prepared.elision.project(raw)
	if err != nil {
		return elisionRecord{}, nil, err
	}
	boundaries := interactionBoundaries(raw)
	// Incomplete batches cannot displace the two complete steps protected here.
	end := len(boundaries) - 1
	recentTokens := EstimateMessagesTextTokens(projected[boundaries[end]:])
	kept := 0
	for end > 0 && (kept < 2 || recentTokens < policy.ContextWindowTokens*3/10) {
		recentTokens += EstimateMessagesTextTokens(projected[boundaries[end-1]:boundaries[end]])
		end--
		kept++
	}
	start := 0
	if compactionPresent && !compaction.Removed {
		start = compaction.ReplacementTo
	}
	next := elisionRecord{Version: 1, Revision: prepared.elision.Revision + 1, ClearRevision: prepared.clearRevision}
	selected := make(map[int]bool)
	for _, replacement := range prepared.elision.Replacements {
		if replacement.MessageIndex >= start {
			next.Replacements = append(next.Replacements, replacement)
			selected[replacement.MessageIndex] = true
		}
	}
	previousCount := len(next.Replacements)
	targetSavings := max(policy.MinimumSavingsTokens, pressure-int(float64(policy.ContextWindowTokens)*policy.TriggerRatio*.85))
	saved := 0
	// Prefer the newest eligible groups: keep the longest possible unchanged
	// prefix, and batch enough savings to avoid repeated cache invalidations.
	for group := end - 1; group >= 0 && saved < targetSavings; group-- {
		if err := ctx.Err(); err != nil {
			return elisionRecord{}, nil, err
		}
		from, to := boundaries[group], boundaries[group+1]
		if from < start {
			break
		}
		calls := make(map[string]ToolCall)
		eligible := true
		for _, message := range raw[from:to] {
			if message == nil {
				continue
			}
			for _, call := range message.ToolCalls {
				calls[call.ID] = call
			}
			if message.Role == ToolRole && (elisionPlaceholder(message) == "" || !elisionRecoveryAvailable(message, calls[message.ToolCallID], prepared)) {
				eligible = false
				break
			}
		}
		if !eligible {
			continue
		}
		for index := from; index < to && len(next.Replacements) < maxElisionReplacements; index++ {
			message := raw[index]
			if message == nil || message.Role != ToolRole || selected[index] {
				continue
			}
			stub := elisionPlaceholder(message)
			gain := EstimateTextTokens(projected[index].Content) - EstimateTextTokens(stub)
			if gain < 256 {
				continue
			}
			fingerprint, err := hashCanonical(message)
			if err != nil {
				return elisionRecord{}, nil, err
			}
			next.Replacements = append(next.Replacements, elisionReplacement{MessageIndex: index, SourceHash: fingerprint})
			selected[index] = true
			saved += gain
		}
	}
	if len(next.Replacements) == previousCount || saved < policy.MinimumSavingsTokens {
		return prepared.elision, nil, nil
	}
	sort.Slice(next.Replacements, func(i, j int) bool { return next.Replacements[i].MessageIndex < next.Replacements[j].MessageIndex })
	after, err := buildAfter(next)
	if err != nil {
		return elisionRecord{}, nil, err
	}
	snapshot := after.call.Snapshot()
	afterSize, err := snapshot.EstimateInput()
	if err != nil {
		return elisionRecord{}, nil, err
	}
	if size.Bytes <= afterSize.Bytes || calibration.CalibratedTokens(size.Tokens)-calibration.CalibratedTokens(afterSize.Tokens) < policy.MinimumSavingsTokens {
		return prepared.elision, nil, nil
	}
	beforeMessages, afterMessages := before.Messages(), snapshot.Messages()
	common := 0
	for common < min(len(beforeMessages), len(afterMessages)) {
		equal, err := canonicalMessagesEqual(beforeMessages[common], afterMessages[common])
		if err != nil {
			return elisionRecord{}, nil, err
		}
		if !equal {
			break
		}
		common++
	}
	if common < before.StablePrefixMessages() {
		return elisionRecord{}, nil, errors.New("Elision changed the stable context prefix")
	}
	prefix, err := before.WithMessages(beforeMessages[:common]).EstimateInput()
	if err != nil {
		return elisionRecord{}, nil, err
	}
	next.Metrics = ElisionMetrics{
		TokensBefore: calibration.CalibratedTokens(size.Tokens), TokensAfter: calibration.CalibratedTokens(afterSize.Tokens),
		ResultsElided: len(next.Replacements) - previousCount, CacheExpectedPrefixTokens: prefix.Tokens,
	}
	return next, after, ctx.Err()
}

func elisionRecoveryAvailable(message *Message, call ToolCall, prepared preparedDefinition) bool {
	result := message.ToolResult
	kind := result.ContextHints.Recovery.Kind
	originalReadOnly := false
	for _, definition := range prepared.toolSnapshots {
		if definition.Info != nil && definition.Info.Name == call.Function.Name {
			originalReadOnly = definition.Descriptor.Recovery == ToolRecoveryReadOnly && definition.Descriptor.MutationScope == ToolMutationNone
			break
		}
	}
	for _, definition := range prepared.toolSnapshots {
		if definition.Info == nil {
			continue
		}
		descriptor := definition.Descriptor
		if kind == ToolResultRecoveryArtifact {
			// Complete artifacts are authenticated by result processing. Recovery
			// requires an ordinary read capability; side effects retain a receipt.
			if descriptor.ResultRecoveryKind == ToolResultRecoveryRead && descriptor.Recovery == ToolRecoveryReadOnly &&
				(originalReadOnly || result.ProtectedReceipt != nil && result.ProtectedReceipt.Outcome != "") {
				return true
			}
		} else if definition.Info.Name == call.Function.Name && descriptor.ResultRecoveryKind == kind &&
			originalReadOnly {
			return true
		}
	}
	return false
}

func elisionCalibration(snapshot *ModelRequestSnapshot) CompactionMetrics {
	messages := snapshot.Messages()
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil || message.Role != Assistant || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil {
			continue
		}
		estimate := message.ResponseMeta.InputEstimate
		if estimate != nil && estimate.Version == InputEstimateVersion && estimate.Model == snapshot.ModelIdentity() {
			return CompactionMetrics{ObservedPromptTokens: message.ResponseMeta.Usage.PromptTokens, ObservedEstimateTokens: estimate.Tokens}
		}
		break
	}
	return CompactionMetrics{}
}
