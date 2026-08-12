package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
)

const (
	maxCompactionContextDataBytes = 8 << 20
)

type compactionCommandEnvelope struct {
	Version                 uint16                   `json:"version"`
	DefinitionKey           string                   `json:"definition_key"`
	RestoreKey              string                   `json:"restore_key"`
	MaterializedFingerprint string                   `json:"materialized_fingerprint"`
	ModelRequestFingerprint string                   `json:"model_request_fingerprint,omitempty"`
	Manager                 CapabilityIdentity       `json:"manager"`
	Compact                 *CompactionRequest       `json:"compact,omitempty"`
	Remove                  *CompactionRemoveRequest `json:"remove,omitempty"`
}

const compactionCommandVersion = 2

func (engine *definitionEngine) RunStructural(
	ctx context.Context,
	request runstate.StructuralEngineRequest,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	if engine == nil || engine.source == nil || emit == nil {
		return runstate.EngineResult{}, ErrDefinitionUnavailable
	}
	transcript, err := decodeEngineTranscript(request.State)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&transcript, request.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	storage, storagePresent, raw, err := compactionStateFrom(request.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	current, present := storage, storagePresent
	current, present = clearCompaction(current, present, clearState, clearPresent)
	cleanupState, cleanupPresent, _, err := cleanupStateFrom(request.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	cleanupState, cleanupPresent = clearCleanup(cleanupState, cleanupPresent, clearState, clearPresent)
	var envelope compactionCommandEnvelope
	if err := json.Unmarshal(request.Snapshot.Ref.RestoreDescriptor, &envelope); err != nil {
		return runstate.EngineResult{}, fmt.Errorf("decode Compaction command: %w", err)
	}
	if envelope.Version != compactionCommandVersion || strings.TrimSpace(envelope.DefinitionKey) == "" ||
		strings.TrimSpace(envelope.RestoreKey) == "" || strings.TrimSpace(envelope.MaterializedFingerprint) == "" {
		return runstate.EngineResult{}, errors.New("Compaction command envelope is incomplete")
	}
	prepared, err := prepareDefinition(ctx, engine.source, PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     structuralDefinitionRun(request.Snapshot.CommandID),
		Reason:  TurnReasonStructural, DefinitionKey: envelope.DefinitionKey, RestoreKey: envelope.RestoreKey,
		HostData:   cloneHostData(transcript.HostData),
		Compaction: compactionStatePointer(current, present),
		Cleanup:    cloneCleanupStateIfPresent(cleanupState, cleanupPresent),
	})
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if engine.persistent {
		if err := validatePersistentDefinition(prepared.definition); err != nil {
			return runstate.EngineResult{}, err
		}
	}
	if prepared.definition.Compaction == nil {
		return runstate.EngineResult{}, ErrCapabilityUnsupported
	}
	materialized, materializedErr := materializedDefinitionFingerprint(prepared)
	if materializedErr != nil {
		return runstate.EngineResult{}, materializedErr
	}
	if prepared.definitionKey != envelope.DefinitionKey || prepared.restoreKey != envelope.RestoreKey ||
		materialized != envelope.MaterializedFingerprint {
		return runstate.EngineResult{}, ErrDefinitionMismatch
	}
	if envelope.Manager != prepared.definition.Compaction.Identity() {
		return runstate.EngineResult{}, fmt.Errorf("%w: Compaction Manager changed", ErrDefinitionMismatch)
	}
	session := SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)}
	run := runViewForStructural(request.Snapshot)
	switch request.Snapshot.Kind {
	case runstate.StructuralCompactContext:
		if envelope.Compact == nil || envelope.Remove != nil {
			return runstate.EngineResult{}, errors.New("Compaction command envelope does not match compact operation")
		}
		if current.ID == compactionID(request.Snapshot.OperationID) && !current.Removed {
			return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
		}
		forkCtx, cacheErr := contextWithProviderCacheKey(ctx, engine.key, engine.cacheKeys)
		if cacheErr != nil {
			return runstate.EngineResult{}, cacheErr
		}
		modelSnapshot, snapshotErr := prepareStructuralCompactionSnapshot(
			forkCtx, prepared, session, structuralDefinitionRun(request.Snapshot.CommandID),
			transcript.Messages, cleanupState, cleanupPresent, current, present,
		)
		if snapshotErr != nil {
			return runstate.EngineResult{}, snapshotErr
		}
		fingerprint, fingerprintErr := modelRequestSnapshotFingerprint(modelSnapshot)
		if fingerprintErr != nil {
			return runstate.EngineResult{}, fingerprintErr
		}
		if envelope.ModelRequestFingerprint == "" || fingerprint != envelope.ModelRequestFingerprint {
			return runstate.EngineResult{}, fmt.Errorf("%w: structural model request changed", ErrDefinitionMismatch)
		}
		buildAfter := func(next CompactionState) (*ModelRequestSnapshot, error) {
			nextPrepared := prepared
			nextCleanup, nextCleanupPresent := cleanupAfterCompaction(cleanupState, cleanupPresent, next, true)
			prepare := PrepareRequest{
				Session: session, Run: structuralDefinitionRun(request.Snapshot.CommandID), Reason: TurnReasonStructural,
				DefinitionKey: envelope.DefinitionKey, RestoreKey: envelope.RestoreKey,
				HostData: cloneHostData(transcript.HostData), Compaction: compactionStatePointer(next, true),
				Cleanup: cloneCleanupStateIfPresent(nextCleanup, nextCleanupPresent),
			}
			if err := rematerializeDefinitionContext(ctx, prepare, &nextPrepared); err != nil {
				return nil, err
			}
			return prepareStructuralCompactionSnapshot(
				forkCtx, nextPrepared, session, structuralDefinitionRun(request.Snapshot.CommandID),
				transcript.Messages, cleanupState, cleanupPresent, next, true,
			)
		}
		next, changed, _, compactErr := executeCompaction(
			ctx, prepared, session, run, transcript.Messages, "", current, present, storage.Revision,
			cleanupRevision(cleanupState, cleanupPresent),
			*envelope.Compact, compactionID(request.Snapshot.OperationID), modelSnapshot, buildAfter, nil, nil,
		)
		if compactErr != nil {
			return runstate.EngineResult{}, compactErr
		}
		if changed {
			encoded, encodeErr := json.Marshal(next)
			if encodeErr != nil {
				return runstate.EngineResult{}, encodeErr
			}
			if err := emit(runstate.EngineCapabilityState{
				Capability: compactionCapability, Expected: describeCapabilityState(raw), State: encoded,
			}); err != nil {
				return runstate.EngineResult{}, err
			}
		}
	case runstate.StructuralRemoveCompaction:
		if envelope.Remove == nil || envelope.Compact != nil {
			return runstate.EngineResult{}, errors.New("Compaction command envelope does not match remove operation")
		}
		remove := *envelope.Remove
		if !present || current.Removed {
			return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
		}
		if current.ID != remove.ID || remove.ExpectedRevision != 0 && current.Revision != remove.ExpectedRevision {
			return runstate.EngineResult{}, ErrDefinitionMismatch
		}
		current.Revision++
		current.Removed = true
		encoded, encodeErr := json.Marshal(current)
		if encodeErr != nil {
			return runstate.EngineResult{}, encodeErr
		}
		if err := emit(runstate.EngineCapabilityState{
			Capability: compactionCapability, Expected: describeCapabilityState(raw), State: encoded,
		}); err != nil {
			return runstate.EngineResult{}, err
		}
	default:
		return runstate.EngineResult{}, fmt.Errorf("unsupported structural operation %q", request.Snapshot.Kind)
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

func structuralDefinitionRun(commandID runstate.CommandID) RunView {
	return RunView{ID: string(commandID), CommandID: string(commandID), Cycle: 1}
}

func contextWithProviderCacheKey(ctx context.Context, key SessionKey, generate CacheKeyGenerator) (context.Context, error) {
	if generate == nil {
		return nil, errors.New("Agent provider Cache Key generator is unavailable")
	}
	cacheKey, err := generate(key)
	if err != nil {
		return nil, fmt.Errorf("derive Agent provider Cache Key: %w", err)
	}
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" || len(cacheKey) > 256 {
		return nil, errors.New("Agent provider Cache Key is empty or exceeds 256 bytes")
	}
	return ContextWithSessionKey(ctx, cacheKey), nil
}

func prepareStructuralCompactionSnapshot(
	ctx context.Context,
	prepared preparedDefinition,
	session SessionView,
	run RunView,
	raw []*Message,
	cleanup CleanupState,
	cleanupPresent bool,
	compaction CompactionState,
	compactionPresent bool,
) (*ModelRequestSnapshot, error) {
	visible, err := effectiveCleanupMessages(raw, cleanup, cleanupPresent, compaction, compactionPresent)
	if err != nil {
		return nil, err
	}
	effective, err := effectiveCompactionMessages(
		visible, compaction, compactionPresent, prepared.definition.Compaction.SummaryLimitBytes(),
	)
	checkpointVisible := err == nil
	if err != nil {
		if !errors.Is(err, ErrContextLimit) {
			return nil, err
		}
		effective = cloneMessages(visible)
	}
	messages := make([]*Message, 0, len(effective)+len(prepared.fragments))
	messages = append(messages, leadingContextMessages(prepared.fragments)...)
	messages = append(messages, effective...)
	stablePrefixMessages := stableContextPrefixMessages(prepared.fragments, compaction, compactionPresent)
	if !checkpointVisible && compactionPresent && !compaction.Removed && compaction.ReplacementFrom == 0 {
		stablePrefixMessages--
	}
	return prepareDefinitionModelRequest(ctx, prepared, session, run, messages, stablePrefixMessages)
}

func modelRequestSnapshotFingerprint(snapshot *ModelRequestSnapshot) (string, error) {
	if snapshot == nil {
		return "", errors.New("structural Compaction model request snapshot is unavailable")
	}
	return hashCanonical(struct {
		Messages             []*Message
		Options              *Options
		Streaming            bool
		StablePrefixMessages int
	}{snapshot.Messages(), snapshot.ResolvedOptions(), snapshot.Streaming(), snapshot.StablePrefixMessages()})
}

func executeCompaction(
	ctx context.Context,
	prepared preparedDefinition,
	session SessionView,
	run RunView,
	messages []*Message,
	currentInput string,
	current CompactionState,
	present bool,
	revisionBase uint64,
	cleanupRevisionAtCompaction uint64,
	request CompactionRequest,
	checkpointID string,
	modelSnapshot *ModelRequestSnapshot,
	buildAfter func(CompactionState) (*ModelRequestSnapshot, error),
	onSkip func(string, CompactionMetrics) error,
	onCreate func(CompactionMetrics) error,
) (CompactionState, bool, CompactionMetrics, error) {
	if present && current.ID == checkpointID && !current.Removed {
		return current, false, current.Metrics, nil
	}
	if request.ExpectedID != "" && (!present || current.ID != request.ExpectedID) ||
		request.ExpectedRevision != 0 && (!present || current.Revision != request.ExpectedRevision) {
		return CompactionState{}, false, CompactionMetrics{}, ErrDefinitionMismatch
	}
	summaryLimit := prepared.definition.Compaction.SummaryLimitBytes()
	var modelRequest []*Message
	if modelSnapshot != nil {
		modelRequest = modelSnapshot.Messages()
	} else {
		var err error
		modelRequest, err = compactionModelRequest(prepared, messages, currentInput, current, present)
		if err != nil {
			return CompactionState{}, false, CompactionMetrics{}, err
		}
	}
	plan, err := prepared.definition.Compaction.Plan(ctx, CompactionPlanRequest{
		Session: session, Run: run, Messages: cloneMessages(messages), ModelRequest: modelRequest,
		ModelSnapshot: modelSnapshot,
		Force:         request.Force || present && len(current.Summary) > summaryLimit,
		Current:       current, Present: present,
	})
	if err != nil {
		return CompactionState{}, false, CompactionMetrics{}, err
	}
	if plan.Action == CompactionNone {
		if onSkip != nil && strings.TrimSpace(plan.SkippedReason) != "" {
			if err := onSkip(plan.SkippedReason, plan.Metrics); err != nil {
				return CompactionState{}, false, plan.Metrics, err
			}
		}
		return current, false, plan.Metrics, nil
	}
	if plan.Action != CompactionCreate || plan.SourceFrom < 0 || plan.SourceTo <= plan.SourceFrom || plan.SourceTo > len(messages) {
		return CompactionState{}, false, plan.Metrics, errors.New("Compaction Manager returned an invalid source range")
	}
	wantHash, err := hashCanonical(messages[plan.SourceFrom:plan.SourceTo])
	if err != nil {
		return CompactionState{}, false, plan.Metrics, err
	}
	if strings.TrimSpace(plan.SourceHash) == "" {
		plan.SourceHash = wantHash
	} else if plan.SourceHash != wantHash {
		return CompactionState{}, false, plan.Metrics, errors.New("Compaction Manager source hash does not match the selected messages")
	}
	if strings.TrimSpace(plan.SourceRevision) == "" {
		plan.SourceRevision = fmt.Sprintf("session:%d", session.Revision)
	}
	if onCreate != nil {
		if err := onCreate(plan.Metrics); err != nil {
			return CompactionState{}, false, plan.Metrics, err
		}
	}
	checkpoint, err := prepared.definition.Compaction.Compact(ctx, CompactionCompactRequest{
		Session: session, Run: run, Messages: cloneMessages(messages), ModelRequest: modelRequest,
		SourceMessages: compactionIncrementalSource(messages, plan, current, present, summaryLimit),
		ModelSnapshot:  modelSnapshot, Plan: plan, Current: current, Present: present,
	})
	if err != nil {
		return CompactionState{}, false, plan.Metrics, err
	}
	checkpoint.Summary = strings.TrimSpace(checkpoint.Summary)
	if checkpoint.Summary == "" || checkpoint.TokenEstimate < 0 {
		return CompactionState{}, false, plan.Metrics, errors.New("Compaction Manager returned an invalid checkpoint")
	}
	if len(checkpoint.Summary) > summaryLimit {
		return CompactionState{}, false, plan.Metrics, fmt.Errorf("%w: Compaction checkpoint is %d bytes and exceeds the target Agent summary limit %d", ErrContextLimit, len(checkpoint.Summary), summaryLimit)
	}
	if err := validateCompactionContextData(checkpoint.ContextData); err != nil {
		return CompactionState{}, false, plan.Metrics, err
	}
	revision := max(uint64(1), revisionBase+1)
	if present && current.Revision >= revisionBase {
		revision = current.Revision + 1
	}
	next := CompactionState{
		ID: checkpointID, Revision: revision,
		SourceRevision: plan.SourceRevision, SourceHash: plan.SourceHash,
		Summary: checkpoint.Summary, SummaryTokenEstimate: checkpoint.TokenEstimate,
		CleanupRevisionAtCompaction: max(current.CleanupRevisionAtCompaction, cleanupRevisionAtCompaction),
		ReplacementFrom:             plan.SourceFrom, ReplacementTo: plan.SourceTo,
		CreatedAt:   time.Now().UTC(),
		ContextData: cloneHostData(checkpoint.ContextData),
	}
	if modelSnapshot == nil || buildAfter == nil {
		return CompactionState{}, false, plan.Metrics, errors.New("Compaction requires exact before and after model request snapshots")
	}
	after, err := buildAfter(next)
	if err != nil {
		return CompactionState{}, false, plan.Metrics, fmt.Errorf("rebuild post-Compaction model request: %w", err)
	}
	metrics, err := validateCompactionProjection(modelSnapshot, after, plan)
	if err != nil {
		return CompactionState{}, false, metrics, err
	}
	next.Metrics = metrics
	next.TokenEstimate = metrics.ProjectedTokensAfter
	return next, true, metrics, nil
}

func validateCompactionProjection(before, after *ModelRequestSnapshot, plan CompactionPlan) (CompactionMetrics, error) {
	metrics := plan.Metrics
	if before == nil || after == nil {
		return metrics, errors.New("Compaction validation requires exact before and after model request snapshots")
	}
	policy := plan.Validation
	if policy.ReservedTokens < 0 || policy.ContextWindowTokens < 0 || policy.HardLimitBytes < 0 {
		return metrics, errors.New("Compaction validation policy contains negative limits")
	}
	beforeMessages, afterMessages := before.Messages(), after.Messages()
	beforeTokens := estimateCompactionRequestTokens(beforeMessages, before.ResolvedOptions().Tools)
	afterTokens := estimateCompactionRequestTokens(afterMessages, after.ResolvedOptions().Tools)
	metrics.EstimatedTokensBefore = beforeTokens
	metrics.EstimatedTokensAfter = afterTokens
	metrics.ReservedTokens = policy.ReservedTokens
	metrics.ProjectedTokensBefore = metrics.CalibratedTokens(beforeTokens) + policy.ReservedTokens
	metrics.ProjectedTokensAfter = metrics.CalibratedTokens(afterTokens) + policy.ReservedTokens
	metrics.ContextWindowTokens = policy.ContextWindowTokens
	metrics.Threshold = policy.Threshold
	metrics.RecoveryBand = policy.RecoveryBand
	metrics.MessageCountBefore = len(beforeMessages)
	metrics.MessageCountAfter = len(afterMessages)
	metrics.SourceMessageCount = plan.SourceTo - plan.SourceFrom
	metrics.StablePrefixTokens = stableSnapshotTokens(after)
	metrics.CacheExpectedPrefixTokens = stableSnapshotTokens(before)
	metrics.CandidateFingerprint, metrics.CandidateGeneration = compactionCandidateIdentity(afterMessages)
	if policy.HardLimitBytes > 0 && compactionRequestBytes(after) > policy.HardLimitBytes {
		return metrics, fmt.Errorf("%w: post-Compaction request exceeds the %d-byte provider input limit", ErrContextLimit, policy.HardLimitBytes)
	}
	progress := metrics.ProjectedTokensBefore - metrics.ProjectedTokensAfter
	if progress <= 0 {
		return metrics, fmt.Errorf("Compaction made no progress: before=%d after=%d", metrics.ProjectedTokensBefore, metrics.ProjectedTokensAfter)
	}
	if policy.MinimumChangeTokens > 0 && progress < policy.MinimumChangeTokens {
		return metrics, fmt.Errorf("Compaction progress %d tokens is below the required minimum %d", progress, policy.MinimumChangeTokens)
	}
	if policy.ContextWindowTokens == 0 {
		return metrics, nil
	}
	if policy.Threshold <= 0 || policy.Threshold >= 1 || policy.RecoveryBand <= 0 || policy.RecoveryBand > 1 {
		return metrics, errors.New("Compaction validation requires threshold and recovery band within (0,1)")
	}
	publishLimit := int(float64(policy.ContextWindowTokens) * policy.Threshold)
	metrics.RecoveryTargetTokens = int(float64(publishLimit) * policy.RecoveryBand)
	metrics.RecoveryBandMet = metrics.ProjectedTokensAfter <= metrics.RecoveryTargetTokens
	metrics.Degraded = !metrics.RecoveryBandMet && metrics.ProjectedTokensAfter < publishLimit
	if metrics.ProjectedTokensAfter >= publishLimit {
		return metrics, fmt.Errorf("%w: post-Compaction request remains above hard publish band: after=%d limit=%d", ErrContextLimit, metrics.ProjectedTokensAfter, publishLimit)
	}
	return metrics, nil
}

func estimateCompactionRequestTokens(messages []*Message, tools []*ToolInfo) int {
	tokens := 0
	for _, message := range messages {
		if message == nil {
			continue
		}
		encoded, _ := json.Marshal(message)
		tokens += 4 + estimateCompactionStringTokens(string(encoded))
	}
	if encoded, err := json.Marshal(tools); err == nil && string(encoded) != "null" {
		tokens += estimateCompactionStringTokens(string(encoded))
	}
	return max(1, tokens)
}

func estimateCompactionStringTokens(value string) int {
	tokens, ascii := 0, 0
	flush := func() {
		if ascii > 0 {
			tokens += (ascii + 3) / 4
			ascii = 0
		}
	}
	for _, item := range value {
		if item <= unicode.MaxASCII {
			ascii++
		} else {
			flush()
			tokens++
		}
	}
	flush()
	return max(1, tokens)
}

func stableSnapshotTokens(snapshot *ModelRequestSnapshot) int {
	if snapshot == nil {
		return 0
	}
	messages := snapshot.Messages()
	boundary := min(snapshot.StablePrefixMessages(), len(messages))
	return estimateCompactionRequestTokens(messages[:boundary], snapshot.ResolvedOptions().Tools)
}

func compactionRequestBytes(snapshot *ModelRequestSnapshot) int {
	if snapshot == nil {
		return 0
	}
	encoded, _ := json.Marshal(struct {
		Messages []*Message
		Tools    []*ToolInfo
	}{snapshot.Messages(), snapshot.ResolvedOptions().Tools})
	return len(encoded)
}

func compactionCandidateIdentity(messages []*Message) (string, uint64) {
	type candidate struct {
		Index int
		Call  string
		Tool  string
		Bytes int
	}
	values := make([]candidate, 0)
	for index, message := range messages {
		if message != nil && message.Role == ToolRole {
			values = append(values, candidate{index, message.ToolCallID, message.ToolName, len(message.Content)})
		}
	}
	fingerprint, _ := hashCanonical(values)
	return fingerprint, uint64(len(values))
}

func runtimeCompactionMetrics(metrics CompactionMetrics) runstate.CompactionMetrics {
	return runstate.CompactionMetrics{
		EstimatedTokensBefore:     metrics.EstimatedTokensBefore,
		ObservedPromptTokens:      metrics.ObservedPromptTokens,
		ObservedEstimateTokens:    metrics.ObservedEstimateTokens,
		EstimatedTokensAfter:      metrics.EstimatedTokensAfter,
		ProjectedTokensBefore:     metrics.ProjectedTokensBefore,
		ProjectedTokensAfter:      metrics.ProjectedTokensAfter,
		ReservedTokens:            metrics.ReservedTokens,
		ContextWindowTokens:       metrics.ContextWindowTokens,
		Threshold:                 metrics.Threshold,
		RecoveryBand:              metrics.RecoveryBand,
		RecoveryTargetTokens:      metrics.RecoveryTargetTokens,
		RecoveryBandMet:           metrics.RecoveryBandMet,
		Degraded:                  metrics.Degraded,
		StablePrefixTokens:        metrics.StablePrefixTokens,
		SourceMessageCount:        metrics.SourceMessageCount,
		MessageCountBefore:        metrics.MessageCountBefore,
		MessageCountAfter:         metrics.MessageCountAfter,
		CacheExpectedPrefixTokens: metrics.CacheExpectedPrefixTokens,
		CacheReadTokens:           metrics.CacheReadTokens,
		CandidateFingerprint:      metrics.CandidateFingerprint,
		CandidateGeneration:       metrics.CandidateGeneration,
	}
}

func validateCompactionContextData(data *HostData) error {
	if data == nil {
		return nil
	}
	if strings.TrimSpace(data.Type) == "" || data.Version == 0 || !json.Valid(data.Data) {
		return errors.New("Compaction ContextData requires Type, Version, and valid JSON Data")
	}
	if len(data.Data) > maxCompactionContextDataBytes {
		return fmt.Errorf("Compaction ContextData exceeds %d bytes", maxCompactionContextDataBytes)
	}
	return nil
}

func compactionStatePointer(state CompactionState, present bool) *CompactionState {
	if !present || state.Removed {
		return nil
	}
	cloned := state
	cloned.ContextData = cloneHostData(state.ContextData)
	return &cloned
}

func cloneCompactionState(state *CompactionState) *CompactionState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.ContextData = cloneHostData(state.ContextData)
	return &cloned
}

func compactionID(operationID runstate.OperationID) string {
	return "compaction-" + string(operationID)
}

func (engine *definitionEngine) applyAutomaticCompaction(
	ctx context.Context,
	request runstate.EngineRequest,
	prepared preparedDefinition,
	messages []*Message,
	modelSnapshot *ModelRequestSnapshot,
	current CompactionState,
	present bool,
	storage CompactionState,
	raw json.RawMessage,
	cleanupRevisionAtCompaction uint64,
	buildAfter func(CompactionState) (*ModelRequestSnapshot, error),
	emit runstate.EngineEventSink,
) (CompactionState, bool, bool, CompactionMetrics, error) {
	if prepared.definition.Compaction == nil {
		return current, present, false, CompactionMetrics{}, nil
	}
	checkpointID := fmt.Sprintf("compaction-%s-%d", request.Snapshot.OperationID, request.Snapshot.Cycle)
	next, changed, metrics, err := executeCompaction(
		ctx, prepared,
		SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		runViewForTurn(request.Snapshot),
		messages, request.Snapshot.Input.Text, current, present, storage.Revision, cleanupRevisionAtCompaction,
		CompactionRequest{},
		checkpointID, modelSnapshot, buildAfter, func(reason string, metrics CompactionMetrics) error {
			if reason != "degraded_no_progress_latch" {
				return nil
			}
			return emit(runstate.EngineCompactionSkipped{
				ID: checkpointID, Reason: reason, Automatic: true, Metrics: runtimeCompactionMetrics(metrics),
			})
		}, func(metrics CompactionMetrics) error {
			return emit(runstate.EngineCompactionStarted{
				ID: checkpointID, Automatic: true, Metrics: runtimeCompactionMetrics(metrics),
			})
		},
	)
	if err != nil {
		return CompactionState{}, false, false, metrics, err
	}
	if !changed {
		return current, present, false, metrics, nil
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return CompactionState{}, false, false, metrics, err
	}
	if err := emit(runstate.EngineCapabilityState{
		Capability: compactionCapability, Expected: describeCapabilityState(raw), State: encoded,
	}); err != nil {
		return CompactionState{}, false, false, metrics, err
	}
	return next, true, true, metrics, nil
}

func automaticCompactionFingerprint(
	prepared preparedDefinition,
	current CompactionState,
	present bool,
	cleanup CleanupState,
	cleanupPresent bool,
	snapshot *ModelRequestSnapshot,
) (string, error) {
	if snapshot == nil {
		return "", errors.New("automatic Compaction requires a final model request snapshot")
	}
	candidateFingerprint, candidateGeneration := compactionCandidateIdentity(snapshot.Messages())
	return hashCanonical(struct {
		Model                CapabilityIdentity
		Manager              CapabilityIdentity
		PrefixFingerprint    string
		Options              *Options
		Compaction           *CompactionState
		Cleanup              *CleanupState
		CandidateFingerprint string
		CandidateGeneration  uint64
		ClearRevision        uint64
	}{
		Model: prepared.definition.ModelIdentity, Manager: prepared.definition.Compaction.Identity(),
		PrefixFingerprint: prepared.prefixFingerprint, Options: snapshot.ResolvedOptions(),
		Compaction: compactionStatePointer(current, present), Cleanup: cloneCleanupStateIfPresent(cleanup, cleanupPresent),
		CandidateFingerprint: candidateFingerprint, CandidateGeneration: candidateGeneration,
		ClearRevision: prepared.clearRevision,
	})
}

func compactionHealthStateFrom(states map[string]json.RawMessage) (compactionHealthState, bool, json.RawMessage, error) {
	raw, present := states[compactionHealthCapability]
	if !present {
		return compactionHealthState{}, false, nil, nil
	}
	var health compactionHealthState
	if err := json.Unmarshal(raw, &health); err != nil {
		return compactionHealthState{}, false, nil, fmt.Errorf("decode Compaction health: %w", err)
	}
	if strings.TrimSpace(health.Fingerprint) == "" || health.ConsecutiveFailures <= 0 {
		return compactionHealthState{}, false, nil, errors.New("durable Compaction health state is invalid")
	}
	return health, true, append(json.RawMessage(nil), raw...), nil
}

func nextCompactionHealth(previous compactionHealthState, present bool, fingerprint string, failure error) compactionHealthState {
	consecutive := 1
	if present && previous.Fingerprint == fingerprint {
		consecutive = previous.ConsecutiveFailures + 1
	}
	reason := strings.TrimSpace(failure.Error())
	if len(reason) > 512 {
		reason = reason[:512]
	}
	return compactionHealthState{
		Fingerprint: fingerprint, ConsecutiveFailures: consecutive, FailureCode: reason,
	}
}

func emitCompactionHealth(
	emit runstate.EngineEventSink,
	raw json.RawMessage,
	health compactionHealthState,
) error {
	encoded, err := json.Marshal(health)
	if err != nil {
		return err
	}
	return emit(runstate.EngineCapabilityState{
		Capability: compactionHealthCapability, Expected: describeCapabilityState(raw), State: encoded,
	})
}

func clearCompactionHealth(emit runstate.EngineEventSink, raw json.RawMessage, present bool) error {
	if !present {
		return nil
	}
	return emit(runstate.EngineCapabilityState{
		Capability: compactionHealthCapability, Expected: describeCapabilityState(raw), Delete: true,
	})
}

func compactionStateFrom(states map[string]json.RawMessage) (CompactionState, bool, json.RawMessage, error) {
	raw, present := states[compactionCapability]
	if !present {
		return CompactionState{}, false, nil, nil
	}
	state, err := decodeCompactionState(raw)
	return state, true, append(json.RawMessage(nil), raw...), err
}

func decodeCompactionState(raw json.RawMessage) (CompactionState, error) {
	var state CompactionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return CompactionState{}, fmt.Errorf("decode Compaction state: %w", err)
	}
	if strings.TrimSpace(state.ID) == "" || state.Revision == 0 || state.ReplacementFrom < 0 ||
		state.ReplacementTo <= state.ReplacementFrom || strings.TrimSpace(state.Summary) == "" {
		return CompactionState{}, errors.New("durable Compaction state is invalid")
	}
	return state, nil
}

func compactionModelRequest(
	prepared preparedDefinition,
	messages []*Message,
	currentInput string,
	current CompactionState,
	present bool,
) ([]*Message, error) {
	result := make([]*Message, 0, len(messages)+len(prepared.fragments)+2)
	if prepared.definition.Instructions != "" {
		result = append(result, SystemMessage(prepared.definition.Instructions))
	}
	effective, err := effectiveCompactionMessages(messages, current, present, prepared.definition.Compaction.SummaryLimitBytes())
	if err != nil {
		// Raw history is retained specifically so an oversized checkpoint can be
		// regenerated after the target Agent's configured limits are lowered.
		if !errors.Is(err, ErrContextLimit) {
			return nil, err
		}
		effective = cloneMessages(messages)
	}
	if strings.TrimSpace(currentInput) == "" {
		result = append(result, leadingContextMessages(prepared.fragments)...)
		result = append(result, effective...)
		return result, nil
	}
	cycle, _, err := assembleCycleMessages(effective, currentInput, prepared.fragments)
	if err != nil {
		return nil, err
	}
	result = append(result, cycle...)
	return result, nil
}

func effectiveCompactionMessages(messages []*Message, state CompactionState, present bool, summaryLimit int) ([]*Message, error) {
	if !present || state.Removed || state.ReplacementFrom < 0 || state.ReplacementTo > len(messages) || state.ReplacementTo <= state.ReplacementFrom {
		return cloneMessages(messages), nil
	}
	if summaryLimit <= 0 {
		return nil, errors.New("Compaction summary limit must be positive")
	}
	if len(state.Summary) > summaryLimit {
		return nil, fmt.Errorf("%w: durable Compaction checkpoint is %d bytes and exceeds the target Agent summary limit %d", ErrContextLimit, len(state.Summary), summaryLimit)
	}
	result := make([]*Message, 0, len(messages)-(state.ReplacementTo-state.ReplacementFrom)+1)
	result = append(result, cloneMessages(messages[:state.ReplacementFrom])...)
	result = append(result, compactionCheckpointMessage(state, summaryLimit))
	result = append(result, cloneMessages(messages[state.ReplacementTo:])...)
	return result, nil
}

func compactionIncrementalSource(
	messages []*Message,
	plan CompactionPlan,
	current CompactionState,
	present bool,
	summaryLimit int,
) []*Message {
	if present && !current.Removed && current.ReplacementFrom == plan.SourceFrom &&
		current.ReplacementTo >= plan.SourceFrom && current.ReplacementTo <= plan.SourceTo {
		result := []*Message{compactionCheckpointMessage(current, summaryLimit)}
		return append(result, cloneMessages(messages[current.ReplacementTo:plan.SourceTo])...)
	}
	return cloneMessages(messages[plan.SourceFrom:plan.SourceTo])
}

func compactionCheckpointMessage(state CompactionState, summaryLimit int) *Message {
	return SystemMessage(renderContextFragment(ContextFragment{
		Source: "agent.compaction", Purpose: "replace compacted conversation history",
		Resource: state.ID, Revision: fmt.Sprintf("%d", state.Revision),
		Placement: ContextCompactionCheckpoint, Content: state.Summary, HardLimit: summaryLimit,
	}))
}

var _ runstate.StructuralEngine = (*definitionEngine)(nil)
