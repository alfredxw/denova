package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

const (
	maxCleanupStateBytes       = 8 << 20
	maxCleanupPlaceholderBytes = 8 << 10
)

type stagedCleanup struct {
	plan              CleanupPlan
	replacements      []CleanupReplacement
	projectionTargets []cleanupProjectionTarget
	current           CleanupState
	present           bool
	mergeExisting     bool
	raw               json.RawMessage
}

type cleanupProjectionTarget struct {
	Replacement CleanupReplacement
	VisibleHash string
	HashOrdinal int
	CallOrdinal int
	ToolName    string
}

type frozenCleanupTargets struct {
	raw        []CleanupReplacement
	projection []cleanupProjectionTarget
}

func cloneCleanupStateIfPresent(state CleanupState, present bool) *CleanupState {
	if !present || state.Removed {
		return nil
	}
	return cloneCleanupState(&state)
}

func runtimeCleanupMetrics(metrics CleanupMetrics) runstate.CleanupMetrics {
	return runstate.CleanupMetrics{
		EstimatedTokensBefore:      metrics.EstimatedTokensBefore,
		LocalProjectedTokens:       metrics.LocalProjectedTokens,
		ObservedPromptTokens:       metrics.ObservedPromptTokens,
		EffectiveTokens:            metrics.EffectiveTokens,
		EstimatedTokensAfter:       metrics.EstimatedTokensAfter,
		ReclaimedTokens:            metrics.ReclaimedTokens,
		ContextWindowTokens:        metrics.ContextWindowTokens,
		PressureBefore:             metrics.PressureBefore,
		PressureAfter:              metrics.PressureAfter,
		BodyPressureBefore:         metrics.BodyPressureBefore,
		BodyPressureAfter:          metrics.BodyPressureAfter,
		StablePrefixTokens:         metrics.StablePrefixTokens,
		CandidateTokens:            metrics.CandidateTokens,
		CacheViableCandidateTokens: metrics.CacheViableCandidateTokens,
		SkippedBelowMinimumCount:   metrics.SkippedBelowMinimumCount,
		SkippedWarmSuffixCount:     metrics.SkippedWarmSuffixCount,
		EagerCandidateCount:        metrics.EagerCandidateCount,
		EagerSelectedCount:         metrics.EagerSelectedCount,
		SupersededCandidateCount:   metrics.SupersededCandidateCount,
		DiscardableCandidateCount:  metrics.DiscardableCandidateCount,
		MinimumCleanupTokens:       metrics.MinimumCleanupTokens,
		ProtectedResults:           metrics.ProtectedResults,
		EarliestChanged:            metrics.EarliestChanged,
		WarmSuffixTokens:           metrics.WarmSuffixTokens,
		PlaceholderTokens:          metrics.PlaceholderTokens,
		ReplacementCount:           metrics.ReplacementCount,
		EagerOnly:                  metrics.EagerOnly,
		PressureScope:              metrics.PressureScope,
		ProviderCacheState:         metrics.ProviderCacheState,
		ExecutionMode:              metrics.ExecutionMode,
		RendererVersion:            metrics.RendererVersion,
	}
}

func cleanupStateFrom(states map[string]json.RawMessage) (CleanupState, bool, json.RawMessage, error) {
	raw, present := states[cleanupCapability]
	if !present {
		return CleanupState{}, false, nil, nil
	}
	state, err := decodeCleanupState(raw)
	return state, true, append(json.RawMessage(nil), raw...), err
}

func decodeCleanupState(raw json.RawMessage) (CleanupState, error) {
	if len(raw) == 0 || len(raw) > maxCleanupStateBytes {
		return CleanupState{}, errors.New("durable Cleanup state exceeds its byte boundary")
	}
	var state CleanupState
	if err := json.Unmarshal(raw, &state); err != nil {
		return CleanupState{}, fmt.Errorf("decode Cleanup state: %w", err)
	}
	if strings.TrimSpace(state.ID) == "" || state.Revision == 0 || strings.TrimSpace(state.SourceHash) == "" ||
		state.SourceStart < 0 || state.SourceEnd <= state.SourceStart || len(state.Replacements) == 0 ||
		strings.TrimSpace(state.Renderer) == "" || state.CreatedAt.IsZero() || state.UpdatedAt.IsZero() {
		return CleanupState{}, errors.New("durable Cleanup state is invalid")
	}
	previous := -1
	for _, replacement := range state.Replacements {
		if replacement.MessageIndex < state.SourceStart || replacement.MessageIndex >= state.SourceEnd ||
			replacement.MessageIndex <= previous || strings.TrimSpace(replacement.ToolCallID) == "" ||
			replacement.Placeholder == "" || len(replacement.Placeholder) > maxCleanupPlaceholderBytes {
			return CleanupState{}, errors.New("durable Cleanup replacements are invalid")
		}
		previous = replacement.MessageIndex
	}
	return state, nil
}

func clearCleanup(current CleanupState, present bool, clearState ClearState, clearPresent bool) (CleanupState, bool) {
	if clearPresent && present && current.Revision <= clearState.CleanupRevisionAtClear {
		return CleanupState{}, false
	}
	return current, present
}

func cleanupAfterCompaction(
	current CleanupState,
	present bool,
	compaction CompactionState,
	compactionPresent bool,
) (CleanupState, bool) {
	if present && compactionPresent && current.Revision <= compaction.CleanupRevisionAtCompaction {
		return CleanupState{}, false
	}
	return current, present
}

func cleanupRevision(state CleanupState, present bool) uint64 {
	if !present || state.Removed {
		return 0
	}
	return state.Revision
}

func effectiveCleanupMessages(
	messages []*Message,
	state CleanupState,
	present bool,
	compaction CompactionState,
	compactionPresent bool,
) ([]*Message, error) {
	state, present = cleanupAfterCompaction(state, present, compaction, compactionPresent)
	if !present || state.Removed {
		return cloneMessages(messages), nil
	}
	if state.SourceEnd > len(messages) {
		return nil, errors.New("durable Cleanup source extends beyond raw Agent history")
	}
	wantHash, err := hashCanonical(messages[:state.SourceEnd])
	if err != nil {
		return nil, err
	}
	if wantHash != state.SourceHash {
		return nil, errors.New("durable Cleanup source no longer matches raw Agent history")
	}
	result := cloneMessages(messages)
	for _, replacement := range state.Replacements {
		message := result[replacement.MessageIndex]
		if message == nil || message.Role != ToolRole || strings.TrimSpace(message.ToolCallID) != strings.TrimSpace(replacement.ToolCallID) {
			return nil, errors.New("durable Cleanup replacement no longer matches its raw tool result")
		}
		result[replacement.MessageIndex] = cleanupProjectedMessage(message, replacement.Placeholder)
	}
	return result, nil
}

func cleanupProjectedMessage(message *Message, placeholder string) *Message {
	next := message.Clone()
	next.Content = placeholder
	if next.ToolResult != nil {
		next.ToolResult.ContextHints = nil
		next.ToolResult.ResultRetention = ToolResultProtected
	}
	return next
}

func applyCleanupPlan(messages []*Message, plan CleanupPlan) ([]*Message, error) {
	result := cloneMessages(messages)
	seen := make(map[int]struct{}, len(plan.Replacements))
	for index, replacement := range plan.Replacements {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= len(result) ||
			strings.TrimSpace(replacement.ToolCallID) == "" || replacement.Placeholder == "" || len(replacement.Placeholder) > maxCleanupPlaceholderBytes {
			return nil, fmt.Errorf("Cleanup replacement %d is invalid", index)
		}
		message := result[replacement.MessageIndex]
		if message == nil || message.Role != ToolRole || strings.TrimSpace(message.ToolCallID) != strings.TrimSpace(replacement.ToolCallID) {
			return nil, fmt.Errorf("Cleanup replacement %d does not match the exact model request", index)
		}
		if _, duplicate := seen[replacement.MessageIndex]; duplicate {
			return nil, errors.New("Cleanup plan contains duplicate replacement targets")
		}
		seen[replacement.MessageIndex] = struct{}{}
		result[replacement.MessageIndex] = cleanupProjectedMessage(message, replacement.Placeholder)
	}
	return result, nil
}

func resolveCleanupTargets(visible, raw []*Message, replacements []CleanupReplacement) ([]CleanupReplacement, error) {
	resolved := make([]CleanupReplacement, 0, len(replacements))
	used := make(map[int]struct{}, len(replacements))
	for _, replacement := range replacements {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= len(visible) {
			return nil, errors.New("Cleanup replacement is outside its staged model request")
		}
		target := visible[replacement.MessageIndex]
		if target == nil || target.Role != ToolRole || strings.TrimSpace(target.ToolCallID) != strings.TrimSpace(replacement.ToolCallID) {
			return nil, errors.New("Cleanup replacement lost its staged tool-result identity")
		}
		ordinal := cleanupOccurrenceFromEnd(visible, replacement.MessageIndex, target.ToolCallID, target.ToolName)
		rawIndex := cleanupIndexFromEnd(raw, ordinal, target.ToolCallID, target.ToolName)
		if rawIndex < 0 {
			return nil, fmt.Errorf("Cleanup tool call %q cannot be resolved to final raw history", target.ToolCallID)
		}
		if _, duplicate := used[rawIndex]; duplicate {
			return nil, errors.New("Cleanup replacements resolve to one raw message")
		}
		used[rawIndex] = struct{}{}
		next := replacement
		next.MessageIndex = rawIndex
		resolved = append(resolved, next)
	}
	return resolved, nil
}

// freezeCleanupTargets validates caller middleware output before the provider
// call and converts every target to its final raw transcript coordinate. The
// initial loop prefix can contain Context/Compaction projections, while every
// later loop message is an Agent-generated assistant/tool message that will be
// appended after baseRaw on successful settlement.
func freezeCleanupTargets(
	visible, beforeMiddleware, baseRaw []*Message,
	initialLoopMessages int,
	replacements []CleanupReplacement,
) (frozenCleanupTargets, error) {
	baseTargets, err := resolveCleanupTargets(visible, beforeMiddleware, replacements)
	if err != nil {
		return frozenCleanupTargets{}, fmt.Errorf("Cleanup targets must originate in the Agent loop before caller middleware: %w", err)
	}
	frozen := make([]CleanupReplacement, 0, len(baseTargets))
	projection := make([]cleanupProjectionTarget, 0, len(baseTargets))
	used := make(map[int]struct{}, len(baseTargets))
	for index, replacement := range baseTargets {
		visibleIndex := replacements[index].MessageIndex
		visibleHash, hashErr := cleanupProjectionFingerprint(visible[visibleIndex])
		if hashErr != nil {
			return frozenCleanupTargets{}, hashErr
		}
		hashOrdinal := 0
		callOrdinal := 0
		for candidate := 0; candidate <= visibleIndex; candidate++ {
			candidateHash, candidateErr := cleanupProjectionFingerprint(visible[candidate])
			if candidateErr != nil {
				return frozenCleanupTargets{}, candidateErr
			}
			if candidateHash == visibleHash {
				hashOrdinal++
			}
			if sameCleanupToolResult(visible[candidate], replacements[index].ToolCallID, visible[visibleIndex].ToolName) {
				callOrdinal++
			}
		}
		projection = append(projection, cleanupProjectionTarget{
			Replacement: replacements[index], VisibleHash: visibleHash, HashOrdinal: hashOrdinal,
			CallOrdinal: callOrdinal, ToolName: visible[visibleIndex].ToolName,
		})
		loopIndex := replacement.MessageIndex
		if loopIndex < initialLoopMessages {
			resolved, resolveErr := resolveCleanupTargets(beforeMiddleware, baseRaw, []CleanupReplacement{replacement})
			if resolveErr != nil {
				return frozenCleanupTargets{}, fmt.Errorf("Cleanup target is model-only context and cannot be persisted: %w", resolveErr)
			}
			replacement.MessageIndex = resolved[0].MessageIndex
		} else {
			replacement.MessageIndex = len(baseRaw) + loopIndex - initialLoopMessages
		}
		if _, duplicate := used[replacement.MessageIndex]; duplicate {
			return frozenCleanupTargets{}, errors.New("Cleanup targets resolve to one raw transcript message")
		}
		used[replacement.MessageIndex] = struct{}{}
		frozen = append(frozen, replacement)
	}
	sort.Slice(frozen, func(left, right int) bool { return frozen[left].MessageIndex < frozen[right].MessageIndex })
	return frozenCleanupTargets{raw: frozen, projection: projection}, nil
}

// applyStagedCleanupProjection reapplies one already-staged cleanup at every
// later model seam. Exact provider-visible message hashes disambiguate reused
// call IDs without allowing newly injected tool messages to become durable
// cleanup targets.
func applyStagedCleanupProjection(messages []*Message, targets []cleanupProjectionTarget) ([]*Message, error) {
	plan := CleanupPlan{Action: CleanupProject, Replacements: make([]CleanupReplacement, 0, len(targets))}
	for _, target := range targets {
		ordinal := target.HashOrdinal
		index := -1
		for candidate, message := range messages {
			hash, err := cleanupProjectionFingerprint(message)
			if err != nil {
				return nil, err
			}
			if hash != target.VisibleHash {
				continue
			}
			ordinal--
			if ordinal == 0 {
				index = candidate
				break
			}
		}
		if index < 0 {
			ordinal = target.CallOrdinal
			for candidate, message := range messages {
				if !sameCleanupToolResult(message, target.Replacement.ToolCallID, target.ToolName) {
					continue
				}
				ordinal--
				if ordinal == 0 {
					if message.Content != target.Replacement.Placeholder {
						return nil, errors.New("staged Cleanup target changed after its provider projection")
					}
					index = candidate
					break
				}
			}
			if index < 0 {
				return nil, errors.New("staged Cleanup target no longer matches the provider-visible request")
			}
		}
		replacement := target.Replacement
		replacement.MessageIndex = index
		if messages[index].Content != replacement.Placeholder {
			plan.Replacements = append(plan.Replacements, replacement)
		}
	}
	if len(plan.Replacements) == 0 {
		return cloneMessages(messages), nil
	}
	return applyCleanupPlan(messages, plan)
}

func cleanupProjectionFingerprint(message *Message) (string, error) {
	if message == nil {
		return hashCanonical((*Message)(nil))
	}
	// Provider adapters may add response metadata between seams. Cleanup target
	// identity deliberately uses only model-visible tool-result fields; call ID
	// alone is insufficient because providers are allowed to reuse it.
	return hashCanonical(struct {
		Role       RoleType
		Content    string
		ToolCallID string
		ToolName   string
		Multi      []json.RawMessage
	}{message.Role, message.Content, message.ToolCallID, message.ToolName, message.MultiContent})
}

func cleanupOccurrenceFromEnd(messages []*Message, target int, callID, toolName string) int {
	ordinal := 0
	for index := len(messages) - 1; index >= target; index-- {
		if sameCleanupToolResult(messages[index], callID, toolName) {
			ordinal++
		}
	}
	return ordinal
}

func cleanupIndexFromEnd(messages []*Message, ordinal int, callID, toolName string) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if !sameCleanupToolResult(messages[index], callID, toolName) {
			continue
		}
		ordinal--
		if ordinal == 0 {
			return index
		}
	}
	return -1
}

func sameCleanupToolResult(message *Message, callID, toolName string) bool {
	if message == nil || message.Role != ToolRole || strings.TrimSpace(message.ToolCallID) != strings.TrimSpace(callID) {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	return toolName == "" || strings.EqualFold(strings.TrimSpace(message.ToolName), toolName)
}

func (staged stagedCleanup) finalState(transcript []*Message, operationID runstate.OperationID, cycle int) (CleanupState, error) {
	for _, replacement := range staged.replacements {
		if replacement.MessageIndex < 0 || replacement.MessageIndex >= len(transcript) ||
			!sameCleanupToolResult(transcript[replacement.MessageIndex], replacement.ToolCallID, "") {
			return CleanupState{}, errors.New("frozen Cleanup target does not match final raw transcript")
		}
	}
	merged := make(map[int]CleanupReplacement, len(staged.current.Replacements)+len(staged.replacements))
	if staged.mergeExisting && staged.present && !staged.current.Removed {
		for _, replacement := range staged.current.Replacements {
			merged[replacement.MessageIndex] = replacement
		}
	}
	for _, replacement := range staged.replacements {
		merged[replacement.MessageIndex] = replacement
	}
	ordered := make([]CleanupReplacement, 0, len(merged))
	for _, replacement := range merged {
		ordered = append(ordered, replacement)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].MessageIndex < ordered[right].MessageIndex })
	if len(ordered) == 0 {
		return CleanupState{}, errors.New("staged Cleanup contains no final replacements")
	}
	createdAt := time.Now().UTC()
	revision := uint64(1)
	if staged.present {
		revision = staged.current.Revision + 1
		if !staged.current.CreatedAt.IsZero() {
			createdAt = staged.current.CreatedAt
		}
	}
	sourceHash, err := hashCanonical(transcript)
	if err != nil {
		return CleanupState{}, err
	}
	state := CleanupState{
		ID: fmt.Sprintf("cleanup-%s-%d", operationID, cycle), Revision: revision,
		SourceRevision: fmt.Sprintf("operation:%s;cycle:%d", operationID, cycle), SourceHash: sourceHash,
		SourceStart: ordered[0].MessageIndex, SourceEnd: len(transcript), Replacements: ordered,
		Renderer: staged.plan.Renderer, Metrics: staged.plan.Metrics,
		CreatedAt: createdAt, UpdatedAt: time.Now().UTC(),
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return CleanupState{}, err
	}
	if len(encoded) > maxCleanupStateBytes {
		return CleanupState{}, fmt.Errorf("Cleanup state exceeds %d bytes", maxCleanupStateBytes)
	}
	return state, nil
}
