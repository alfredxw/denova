package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const engineTranscriptVersion = 1

type unsupportedEngineTranscriptVersionError struct {
	version uint16
}

func (err *unsupportedEngineTranscriptVersionError) Error() string {
	return fmt.Sprintf("unsupported Agent transcript version %d", err.version)
}

type enginePreparationStage string

const (
	enginePreparationBase         enginePreparationStage = "base"
	enginePreparationMaterialized enginePreparationStage = "materialized"
)

type engineTranscript struct {
	Version                 uint16                 `json:"version"`
	DefinitionKey           string                 `json:"definition_key"`
	BehaviorKey             string                 `json:"behavior_key"`
	PrefixFingerprint       string                 `json:"prefix_fingerprint"`
	MaterializedFingerprint string                 `json:"materialized_fingerprint,omitempty"`
	DefinitionOperationID   string                 `json:"definition_operation_id,omitempty"`
	DefinitionCommandID     string                 `json:"definition_command_id,omitempty"`
	DefinitionCycle         int                    `json:"definition_cycle,omitempty"`
	PreparationStage        enginePreparationStage `json:"preparation_stage,omitempty"`
	Messages                []*Message             `json:"messages,omitempty"`
	ContextState            contextStateSnapshot   `json:"context_state,omitempty"`
	// ActiveModelUser is the model-only rendering of the accepted raw user
	// message while a tool batch or interaction is still active.
	// Messages remains the canonical raw transcript. Once the cycle settles,
	// this transient projection is discarded so canonical maintenance always
	// addresses stable raw messages.
	ActiveModelUser *Message  `json:"active_model_user,omitempty"`
	ActiveUserIndex int       `json:"active_user_index,omitempty"`
	HostData        *HostData `json:"host_data,omitempty"`
	ClearRevision   uint64    `json:"clear_revision,omitempty"`
}

type definitionEngineFactory struct {
	source    Source
	trace     TraceSink
	cacheKeys CacheKeyGenerator
}

func (factory *definitionEngineFactory) NewEngine(_ context.Context, binding runstate.BindingRef) (runstate.Engine, error) {
	if factory == nil || factory.source == nil {
		return nil, ErrDefinitionUnavailable
	}
	key, err := sessionKeyFromBinding(binding)
	if err != nil {
		return nil, err
	}
	return &definitionEngine{
		source: factory.source, key: key, trace: factory.trace,
		cacheKeys: factory.cacheKeys,
	}, nil
}

type definitionEngine struct {
	source    Source
	key       SessionKey
	trace     TraceSink
	cacheKeys CacheKeyGenerator
}

func (engine *definitionEngine) Run(
	ctx context.Context,
	request runstate.EngineRequest,
	emit runstate.EngineEventSink,
) (result runstate.EngineResult, resultErr error) {
	if engine == nil || engine.source == nil {
		return runstate.EngineResult{}, ErrDefinitionUnavailable
	}
	if emit == nil {
		return runstate.EngineResult{}, errors.New("Agent Engine Event sink is required")
	}
	ctx, controls := startDefinitionEngineControls(ctx, request.Controls)
	loopBound := false
	var preparationCheckpoint func() error
	defer func() {
		controls.close()
		if !loopBound {
			if controlled, controlledErr, handled := controls.controlledPreparationResult(resultErr); handled {
				if preparationCheckpoint != nil {
					if checkpointErr := preparationCheckpoint(); checkpointErr != nil {
						result, resultErr = runstate.EngineResult{}, checkpointErr
						return
					}
				}
				result, resultErr = controlled, controlledErr
			}
		}
	}()
	input, err := decodeInput(request.Snapshot.Input)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	input.IdempotencyKey = string(request.Snapshot.CommandID)
	state, err := decodeEngineTranscript(request.Snapshot.State)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&state, request.Snapshot.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	controlTranscript := append(cloneMessages(state.Messages), UserMessageWithAttachments(strings.TrimSpace(input.Text), input.Attachments))
	var controlPrepared *preparedDefinition
	preparationCheckpoint = func() error {
		var encoded json.RawMessage
		var checkpointErr error
		if controlPrepared != nil {
			encoded, checkpointErr = encodeEngineTranscript(*controlPrepared, controlTranscript)
		} else {
			interrupted := state
			interrupted.Messages = cloneMessages(controlTranscript)
			interrupted.HostData = cloneHostData(input.HostData)
			encoded, checkpointErr = json.Marshal(interrupted)
		}
		if checkpointErr != nil {
			return fmt.Errorf("encode controlled Agent preparation transcript: %w", checkpointErr)
		}
		return emit(runstate.EngineTranscriptUpdated{State: encoded})
	}
	currentCompactionStorage, currentCompactionStoragePresent, currentCompactionRaw, err := compactionStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	currentCompaction, currentCompactionPresent := currentCompactionStorage, currentCompactionStoragePresent
	currentCompaction, currentCompactionPresent = clearCompaction(
		currentCompaction, currentCompactionPresent, clearState, clearPresent,
	)
	currentCleanupStorage, currentCleanupStoragePresent, currentCleanupRaw, err := cleanupStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	currentCleanup, currentCleanupPresent := currentCleanupStorage, currentCleanupStoragePresent
	currentCleanup, currentCleanupPresent = clearCleanup(currentCleanup, currentCleanupPresent, clearState, clearPresent)
	currentCleanup, currentCleanupPresent = cleanupAfterCompaction(
		currentCleanup, currentCleanupPresent, currentCompaction, currentCompactionPresent,
	)
	reason, err := turnReasonForSnapshot(request.Snapshot)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	prepareRequest := PrepareRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   input, Reason: reason,
		DefinitionKey: state.DefinitionKey, BehaviorKey: state.BehaviorKey,
		HostData:   cloneHostData(input.HostData),
		Compaction: compactionStatePointer(currentCompaction, currentCompactionPresent),
		Cleanup:    cloneCleanupStateIfPresent(currentCleanup, currentCleanupPresent),
	}
	sameCycle := state.ownsDefinition(request.Snapshot)
	if !sameCycle {
		prepareRequest.DefinitionKey = ""
		prepareRequest.BehaviorKey = ""
	}
	prepared, err := prepareDefinitionBase(ctx, engine.source, prepareRequest)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	prepared.hostData = cloneHostData(input.HostData)
	prepared.clearRevision = state.ClearRevision
	prepared.contextState = cloneContextStateSnapshot(state.ContextState)
	prepared.definitionOperationID = string(request.Snapshot.OperationID)
	prepared.definitionCommandID = string(request.Snapshot.CommandID)
	prepared.definitionCycle = request.Snapshot.Cycle
	prepared.preparationStage = enginePreparationBase
	controlPrepared = &prepared
	if sameCycle && state.DefinitionKey != "" && state.DefinitionKey != prepared.definitionKey {
		return runstate.EngineResult{}, fmt.Errorf("%w: definition_key have=%q want=%q", ErrDefinitionMismatch, prepared.definitionKey, state.DefinitionKey)
	}
	if sameCycle && state.BehaviorKey != "" && state.BehaviorKey != prepared.behaviorKey {
		return runstate.EngineResult{}, fmt.Errorf("%w: behavior_key changed", ErrDefinitionMismatch)
	}
	// Persist the exact base Definition before materializing dynamic capability
	// state. The Run has already committed canonical accepted input; the
	// prepared Definition must prove it resolves the same canonical boundary.
	preparedCheckpoint, err := encodeEngineTranscript(prepared, state.Messages)
	if err != nil {
		return runstate.EngineResult{}, fmt.Errorf("encode pre-commit Agent transcript: %w", err)
	}
	if err := emit(runstate.EngineTranscriptUpdated{State: preparedCheckpoint}); err != nil {
		return runstate.EngineResult{}, err
	}
	if err := engine.verifyCanonicalInputCommit(request.Snapshot, input, prepared.definition.Canonical); err != nil {
		return runstate.EngineResult{}, err
	}
	if err := materializeDefinitionCapabilities(ctx, prepareRequest, &prepared); err != nil {
		return runstate.EngineResult{}, err
	}
	if err := engine.applyGoalPreparation(ctx, request, &prepared); err != nil {
		return runstate.EngineResult{}, err
	}
	materializedFingerprint, err := materializedDefinitionFingerprint(prepared)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if sameCycle && state.PreparationStage == enginePreparationMaterialized &&
		state.MaterializedFingerprint != materializedFingerprint {
		return runstate.EngineResult{}, fmt.Errorf("%w: materialized Definition changed", ErrDefinitionMismatch)
	}
	prepared.materializedFingerprint = materializedFingerprint
	prepared.preparationStage = enginePreparationMaterialized
	materializedCheckpoint, err := encodeEngineTranscript(prepared, state.Messages)
	if err != nil {
		return runstate.EngineResult{}, fmt.Errorf("encode materialized Agent transcript: %w", err)
	}
	if err := emit(runstate.EngineTranscriptUpdated{State: materializedCheckpoint}); err != nil {
		return runstate.EngineResult{}, err
	}
	compaction, compactionPresent := currentCompaction, currentCompactionPresent
	cleanupState, cleanupPresent := currentCleanup, currentCleanupPresent
	stateMessages, nextContextState, err := advanceContextState(
		state.Messages, prepared.fragments, prepared.contextState, compaction, compactionPresent,
	)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	contextOrdinal := 0
	if len(stateMessages) > 0 {
		if err := engine.commitCanonicalContext(
			ctx, request, prepared.definition.Canonical, ContextCommitState, contextOrdinal, stateMessages,
		); err != nil {
			return runstate.EngineResult{}, err
		}
		contextOrdinal++
	}
	prepared.contextState = nextContextState
	cycleStateTranscript := append(cloneMessages(state.Messages), cloneMessages(stateMessages)...)

	summaryLimit := 0
	if prepared.definition.Compaction != nil {
		summaryLimit = prepared.definition.Compaction.SummaryLimitBytes()
	}
	cleanupTranscript, err := effectiveCleanupMessages(cycleStateTranscript, cleanupState, cleanupPresent, compaction, compactionPresent)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	effectiveTranscript, err := effectiveCompactionMessages(cleanupTranscript, compaction, compactionPresent, summaryLimit)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	modelMessages, activeModelUser, err := assembleCycleMessages(effectiveTranscript, input.Text, input.Attachments, prepared.fragments, prepared.definition.AttachmentRoot)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	activeUserIndex := len(cycleStateTranscript)
	stablePrefixMessages := stableContextPrefixMessages(prepared.fragments, compaction, compactionPresent)
	baseTranscript := cloneMessages(cycleStateTranscript)
	baseTranscript = append(baseTranscript, UserMessageWithAttachments(strings.TrimSpace(input.Text), input.Attachments))
	controlTranscript = cloneMessages(baseTranscript)
	initialLoopMessageCount := len(modelMessages)
	if prepared.definition.Instructions != "" {
		initialLoopMessageCount++
	}
	emitTrace(ctx, engine.trace, TraceEvent{
		Kind: TraceCycleStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	emitTrace(ctx, engine.trace, TraceEvent{
		Kind: TraceModelStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	middlewares := append([]Middleware(nil), prepared.definition.Middlewares...)
	permission := effectivePermissionPolicy(prepared.definition.Permission)
	permissionStage := &permissionMiddleware{
		BaseMiddleware: &BaseMiddleware{}, policy: permission,
		session:     SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		run:         runViewForTurn(request.Snapshot),
		attachments: attachmentsFromMessages(modelMessages),
	}
	maintenanceSelected := false
	var pendingCleanup *stagedCleanup
	transcript := cloneMessages(baseTranscript)
	var taskCompletionMessages []*Message
	controlledTranscript := func() []*Message {
		messages := cloneMessages(baseTranscript)
		return append(messages, cloneMessages(taskCompletionMessages)...)
	}
	maintenanceGate := modelCallGate(nil)
	if len(prepared.definition.Middlewares) != 0 || prepared.definition.Cleanup != nil || prepared.definition.Compaction != nil {
		maintenanceGate = func(
			gateCtx context.Context,
			call *ModelCall,
			modelContext *ModelContext,
		) (*modelCallRestart, error) {
			if metrics, ok := modelContext.takeContextNormalization(); ok {
				if err := emit(runstate.EngineContextNormalized{
					RepairCount: metrics.RepairCount, MessagesBefore: metrics.MessagesBefore, MessagesAfter: metrics.MessagesAfter,
				}); err != nil {
					return nil, err
				}
			}
			if prepared.definition.Cleanup == nil && prepared.definition.Compaction == nil {
				return nil, nil
			}
			if pendingCleanup != nil {
				if call == nil {
					return nil, errors.New("Agent staged-Cleanup gate received a nil model call")
				}
				projected, projectErr := applyStagedCleanupProjection(call.Snapshot().Messages(), pendingCleanup.projectionTargets)
				if projectErr != nil {
					if err := emit(runstate.EngineCleanupFailed{
						ID:     fmt.Sprintf("cleanup-%s-%d", request.Snapshot.OperationID, request.Snapshot.Cycle),
						Reason: projectErr.Error(), Automatic: true, Metrics: runtimeCleanupMetrics(pendingCleanup.plan.Metrics),
					}); err != nil {
						return nil, err
					}
					return nil, projectErr
				}
				call.Messages = projected
				return nil, nil
			}
			if maintenanceSelected {
				return nil, nil
			}
			if call == nil {
				return nil, errors.New("Agent context-maintenance gate received a nil model call")
			}
			cleanupID := fmt.Sprintf("cleanup-%s-%d", request.Snapshot.OperationID, request.Snapshot.Cycle)
			if manager := prepared.definition.Cleanup; manager != nil {
				modelSnapshot := call.Snapshot()
				plan, planErr := manager.Plan(gateCtx, CleanupPlanRequest{
					Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
					Run:     runViewForTurn(request.Snapshot), Messages: cloneMessages(cycleStateTranscript),
					ModelRequest: modelSnapshot.Messages(), ModelInspection: modelRequestInspection(modelSnapshot),
					Current: cleanupState, Present: cleanupPresent,
					CompactionAvailable: prepared.definition.Compaction != nil,
				})
				cleanupPlanFailed := planErr != nil
				if planErr != nil {
					if err := emit(runstate.EngineCleanupFailed{
						ID: cleanupID, Reason: planErr.Error(), Automatic: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
					}); err != nil {
						return nil, err
					}
					slog.WarnContext(gateCtx, "automatic Agent Cleanup planning failed",
						"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle, "error", planErr)
					// An actual failed maintenance attempt is not retried after a tool
					// call in the same run. At checkpoint pressure the pure planner can
					// still route this exact seam through Compaction.
					maintenanceSelected = true
					if !plan.FallbackToCompaction && plan.Action != CleanupCompact {
						return nil, nil
					}
				}
				if !cleanupPlanFailed {
					switch plan.Action {
					case CleanupProject:
						if err := emit(runstate.EngineCleanupStarted{
							ID: cleanupID, Reason: plan.Reason, Automatic: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
						}); err != nil {
							return nil, err
						}
						frozen, freezeErr := freezeCleanupTargets(
							modelSnapshot.Messages(), modelContext.contextMaintenanceMessages(), controlledTranscript(),
							initialLoopMessageCount, plan.Replacements,
						)
						if freezeErr != nil {
							if err := emit(runstate.EngineCleanupFailed{
								ID: cleanupID, Reason: freezeErr.Error(), Automatic: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
							}); err != nil {
								return nil, err
							}
							maintenanceSelected = true
							if plan.FallbackToCompaction {
								break
							}
							// A target that cannot be frozen to raw history is a Manager
							// contract violation. Do not let the provider answer from a
							// projection that can never settle durably.
							return nil, freezeErr
						}
						projected, projectErr := applyCleanupPlan(modelSnapshot.Messages(), plan)
						if projectErr != nil {
							if err := emit(runstate.EngineCleanupFailed{
								ID: cleanupID, Reason: projectErr.Error(), Automatic: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
							}); err != nil {
								return nil, err
							}
							maintenanceSelected = true
							if plan.FallbackToCompaction {
								break
							}
							return nil, projectErr
						}
						call.Messages = projected
						pendingCleanup = &stagedCleanup{
							plan: plan, replacements: frozen.raw, projectionTargets: frozen.projection,
							current: currentCleanupStorage, present: currentCleanupStoragePresent,
							mergeExisting: cleanupPresent, raw: currentCleanupRaw,
						}
						return nil, nil
					case CleanupCompact:
						maintenanceSelected = true
						// If the exact request already overflows, safely recoverable tool
						// bodies may be shrunk in this side fork before checkpointing. The
						// cleanup itself is not durable; the checkpoint remains the run's
						// sole maintenance mutation.
						if plan.Metrics.PressureBefore >= 1 && len(plan.Replacements) > 0 {
							if err := emit(runstate.EngineCleanupStarted{
								ID: cleanupID, Reason: plan.Reason, Automatic: true, Transient: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
							}); err != nil {
								return nil, err
							}
							projected, projectErr := applyCleanupPlan(modelSnapshot.Messages(), plan)
							if projectErr != nil {
								if err := emit(runstate.EngineCleanupFailed{
									ID: cleanupID, Reason: projectErr.Error(), Automatic: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
								}); err != nil {
									return nil, err
								}
							} else {
								call.Messages = projected
								if err := emit(runstate.EngineCleanupCompleted{
									ID: cleanupID, Reason: plan.Reason, Automatic: true, Transient: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
								}); err != nil {
									return nil, err
								}
							}
						}
					case CleanupNone:
						if plan.Metrics.CandidateTokens > 0 && plan.Reason != "below_cleanup_threshold" {
							if err := emit(runstate.EngineCleanupSkipped{
								ID: cleanupID, Reason: plan.Reason, Automatic: true, Metrics: runtimeCleanupMetrics(plan.Metrics),
							}); err != nil {
								return nil, err
							}
						}
					default:
						return nil, fmt.Errorf("Cleanup Manager returned invalid action %q", plan.Action)
					}
				}
			}
			if prepared.definition.Compaction == nil {
				return nil, nil
			}
			modelSnapshot := call.Snapshot()
			fingerprint, fingerprintErr := automaticCompactionFingerprint(
				prepared, compaction, compactionPresent, cleanupState, cleanupPresent, modelSnapshot,
			)
			if fingerprintErr != nil {
				return nil, fingerprintErr
			}
			health, healthPresent, healthRaw, healthErr := compactionHealthStateFrom(request.Snapshot.Capabilities)
			if healthErr != nil {
				return nil, healthErr
			}
			failureLimit := normalizedAutomaticCompactionFailureLimit(prepared.definition.Execution)
			checkpointID := fmt.Sprintf("compaction-%s-%d", request.Snapshot.OperationID, request.Snapshot.Cycle)
			if healthPresent && health.Fingerprint == fingerprint && health.ConsecutiveFailures >= failureLimit {
				maintenanceSelected = true
				if err := emit(runstate.EngineCompactionSkipped{
					ID: checkpointID, Reason: "consecutive_failure_fuse", Automatic: true,
					ConsecutiveFailures: health.ConsecutiveFailures, FailureFuseOpen: true,
					Metrics: runtimeCompactionMetrics(compaction.Metrics),
				}); err != nil {
					return nil, err
				}
				return nil, nil
			}
			buildAfter := func(next CompactionState) (*ModelRequestSnapshot, error) {
				nextPrepared := prepared
				nextRequest := prepareRequest
				nextRequest.Compaction = compactionStatePointer(next, true)
				nextCleanup, nextCleanupPresent := cleanupAfterCompaction(cleanupState, cleanupPresent, next, true)
				nextRequest.Cleanup = cloneCleanupStateIfPresent(nextCleanup, nextCleanupPresent)
				if err := rematerializeDefinitionContext(gateCtx, nextRequest, &nextPrepared); err != nil {
					return nil, err
				}
				candidateStateMessages, candidateContextState, err := advanceContextState(
					cycleStateTranscript, nextPrepared.fragments, nextPrepared.contextState, next, true,
				)
				if err != nil {
					return nil, err
				}
				nextPrepared.contextState = candidateContextState
				candidateRaw := append(cloneMessages(cycleStateTranscript), cloneMessages(candidateStateMessages)...)
				projectedRaw, err := effectiveCleanupMessages(candidateRaw, cleanupState, cleanupPresent, next, true)
				if err != nil {
					return nil, err
				}
				effective, err := effectiveCompactionMessages(
					projectedRaw, next, true, nextPrepared.definition.Compaction.SummaryLimitBytes(),
				)
				if err != nil {
					return nil, err
				}
				messages, _, err := assembleCycleMessages(effective, input.Text, input.Attachments, nextPrepared.fragments, nextPrepared.definition.AttachmentRoot)
				if err != nil {
					return nil, err
				}
				messages = append(messages, cloneMessages(taskCompletionMessages)...)
				loop, err := newPreparedDefinitionLoop(gateCtx, nextPrepared, middlewares, permissionStage, nil)
				if err != nil {
					return nil, err
				}
				stable := stableContextPrefixMessages(nextPrepared.fragments, next, true)
				return newLoopRunner(loopRunnerConfig{Agent: loop, EnableStreaming: true}).prepareModelRequest(gateCtx, messages, stable)
			}
			next, nextPresent, changed, compactMetrics, compactErr := engine.applyAutomaticCompaction(
				gateCtx, request, prepared, cycleStateTranscript, modelSnapshot,
				compaction, compactionPresent,
				currentCompactionStorage, currentCompactionRaw,
				cleanupRevision(cleanupState, cleanupPresent),
				buildAfter,
				emit,
			)
			if compactErr != nil {
				if gateCtx.Err() != nil {
					return nil, gateCtx.Err()
				}
				nextHealth := nextCompactionHealth(health, healthPresent, fingerprint, compactErr)
				if err := emitCompactionHealth(emit, healthRaw, nextHealth); err != nil {
					return nil, err
				}
				if err := emit(runstate.EngineCompactionFailed{
					ID: checkpointID, Reason: nextHealth.FailureCode, Automatic: true,
					ConsecutiveFailures: nextHealth.ConsecutiveFailures,
					FailureFuseOpen:     nextHealth.ConsecutiveFailures >= failureLimit,
					Metrics:             runtimeCompactionMetrics(compactMetrics),
				}); err != nil {
					return nil, err
				}
				slog.WarnContext(gateCtx, "automatic Agent Compaction failed; continuing with the unchanged model request",
					"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
					"consecutive_failures", nextHealth.ConsecutiveFailures, "failure_fuse_open", nextHealth.ConsecutiveFailures >= failureLimit,
					"error", compactErr,
				)
				maintenanceSelected = true
				// Automatic maintenance is a recoverable side fork. The unchanged
				// request still passes through the provider input guard, which owns
				// the non-negotiable hard limit.
				return nil, nil
			}
			if err := clearCompactionHealth(emit, healthRaw, healthPresent); err != nil {
				return nil, err
			}
			next, nextPresent = clearCompaction(next, nextPresent, clearState, clearPresent)
			if !changed {
				return nil, nil
			}
			maintenanceSelected = true
			compaction, compactionPresent = next, nextPresent
			prepareRequest.Compaction = compactionStatePointer(compaction, compactionPresent)
			nextCleanup, nextCleanupPresent := cleanupAfterCompaction(cleanupState, cleanupPresent, compaction, compactionPresent)
			prepareRequest.Cleanup = cloneCleanupStateIfPresent(nextCleanup, nextCleanupPresent)
			if rematerializeErr := rematerializeDefinitionContext(gateCtx, prepareRequest, &prepared); rematerializeErr != nil {
				return nil, rematerializeErr
			}
			restoredStateMessages, restoredContextState, stateErr := advanceContextState(
				cycleStateTranscript, prepared.fragments, prepared.contextState, compaction, compactionPresent,
			)
			if stateErr != nil {
				return nil, stateErr
			}
			if len(restoredStateMessages) > 0 {
				insertAt := activeUserIndex
				cycleStateTranscript = append(cycleStateTranscript, cloneMessages(restoredStateMessages)...)
				baseTranscript = insertMessagesAt(baseTranscript, insertAt, restoredStateMessages)
				transcript = insertMessagesAt(transcript, insertAt, restoredStateMessages)
				activeUserIndex += len(restoredStateMessages)
			}
			prepared.contextState = restoredContextState
			var restarted []*Message
			projectedRaw, projectionErr := effectiveCleanupMessages(cycleStateTranscript, cleanupState, cleanupPresent, compaction, compactionPresent)
			if projectionErr != nil {
				return nil, projectionErr
			}
			effective, effectiveErr := effectiveCompactionMessages(
				projectedRaw, compaction, compactionPresent, prepared.definition.Compaction.SummaryLimitBytes(),
			)
			if effectiveErr != nil {
				return nil, effectiveErr
			}
			restarted, _, effectiveErr = assembleCycleMessages(effective, input.Text, input.Attachments, prepared.fragments, prepared.definition.AttachmentRoot)
			if effectiveErr != nil {
				return nil, effectiveErr
			}
			if prepared.definition.Instructions != "" {
				restarted = append([]*Message{SystemMessage(prepared.definition.Instructions)}, restarted...)
			}
			restarted = append(restarted, cloneMessages(taskCompletionMessages)...)
			restartStablePrefix := stableContextPrefixMessages(prepared.fragments, compaction, compactionPresent)
			if prepared.definition.Instructions != "" {
				restartStablePrefix++
			}
			return &modelCallRestart{Messages: restarted, stablePrefixMessages: restartStablePrefix}, nil
		}
	}
	var finalModelRequest *ModelRequestSnapshot
	modelCallGate := maintenanceGate
	if rawGoal, goalPresent := request.Snapshot.Capabilities[goalCapability]; prepared.definition.Goal != nil && goalPresent {
		activeGoal, goalErr := decodeGoalState(rawGoal)
		if goalErr != nil {
			return runstate.EngineResult{}, goalErr
		}
		if activeGoal.Active() {
			modelCallGate = func(gateCtx context.Context, call *ModelCall, modelContext *ModelContext) (*modelCallRestart, error) {
				if maintenanceGate != nil {
					restart, gateErr := maintenanceGate(gateCtx, call, modelContext)
					if gateErr != nil || restart != nil {
						return restart, gateErr
					}
				}
				if call == nil || call.Model == nil {
					return nil, errors.New("Goal evaluation received no final model request")
				}
				finalModelRequest = call.Snapshot()
				return nil, nil
			}
		}
	}
	loop, err := newPreparedDefinitionLoop(ctx, prepared, middlewares, permissionStage, modelCallGate)
	if err != nil {
		return runstate.EngineResult{}, err
	}

	runOption, cancelLoop := newLoopCancellation()
	completion := &runCompletionControl{cancel: cancelLoop}
	control := controls.state
	interactions := newEngineInteractionClient(effectiveInteractionPolicy(prepared.definition.Interaction), emit)
	acceptedControl := controls.bindLoop(cancelLoop, interactions)
	loopBound = true
	if acceptedControl == runstate.EngineControlPreempt {
		return engine.controlledResult(runstate.EnginePreempted, prepared, baseTranscript, emit)
	}
	if acceptedControl == runstate.EngineControlAbort {
		return engine.controlledResult(runstate.EngineAborted, prepared, baseTranscript, emit)
	}

	capabilities := newCapabilityStateClient(request.Snapshot.Capabilities, emit)
	loopCtx := contextWithCapabilityState(ctx, capabilities)
	loopCtx = contextWithInteractionClient(loopCtx, interactions)
	// Concrete tools are not allowed to run until their queued start event has
	// crossed the Run boundary. This also orders model checkpoints and
	// ToolCallStarted before an Ask/Permission interaction emitted by the tool.
	loopCtx = contextWithToolStartReceipt(loopCtx)
	loopCtx = context.WithValue(loopCtx, runCompletionControlKey{}, completion)
	scope, _ := agentsession.CanonicalKey(engine.key)
	loopCtx = ContextWithInvocationIdentity(loopCtx, InvocationIdentity{
		Scope: scope, OperationID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	loopCtx, err = contextWithProviderCacheKey(loopCtx, engine.key, engine.cacheKeys)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	iterator := loop.Run(loopCtx, &loopInput{
		Messages: modelMessages, EnableStreaming: true,
		stablePrefixMessages: stablePrefixMessages,
	}, runOption)
	startedTools := make(map[string]bool)
	var final *Message
	var pendingToolMessages []*Message
	var pendingToolCalls map[string]struct{}
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			controls.stop()
			if controlled, controlledErr, handled := engine.controlledLoopResult(controls, prepared, controlledTranscript(), emit); handled {
				return controlled, controlledErr
			}
			var cancelErr *cancelError
			if completion.requestedCompletion() && errors.As(event.Err, &cancelErr) && cancelErr.Info != nil && cancelErr.Info.Mode&cancelAfterTools != 0 {
				goto loopControlsStopped
			}
			return runstate.EngineResult{}, event.Err
		}
		if event.Output == nil {
			continue
		}
		source := runtimeEventSource(event)
		rootEvent := rootAgentEvent(event, prepared.definition.Name)
		if boundary := event.Output.TaskCompletions; boundary != nil {
			if !rootEvent {
				err := errors.New("nested Agent emitted a task completion boundary into the root transcript")
				boundary.acknowledge(err)
				controls.stop()
				return runstate.EngineResult{}, err
			}
			ids, messages := boundary.snapshot()
			if err := engine.commitCanonicalContext(
				ctx, request, prepared.definition.Canonical, ContextCommitTaskCompletion, contextOrdinal, messages,
			); err != nil {
				boundary.acknowledge(err)
				controls.stop()
				return runstate.EngineResult{}, err
			}
			contextOrdinal++
			transcript = append(transcript, cloneMessages(messages)...)
			taskCompletionMessages = append(taskCompletionMessages, cloneMessages(messages)...)
			checkpoint, checkpointErr := encodeActiveEngineTranscript(
				prepared, transcript, activeModelUser, activeUserIndex,
			)
			if checkpointErr == nil {
				checkpointErr = emit(runstate.EngineTranscriptUpdated{
					State: checkpoint, TaskCompletionIDs: append([]string(nil), ids...),
				})
			}
			boundary.acknowledge(checkpointErr)
			if checkpointErr != nil {
				controls.stop()
				return runstate.EngineResult{}, checkpointErr
			}
			continue
		}
		if nested := event.Output.NestedEvent; nested != nil {
			record, encodeErr := encodeNestedEvent(*nested)
			if encodeErr != nil {
				controls.stop()
				return runstate.EngineResult{}, encodeErr
			}
			if emitErr := emit(runstate.EngineNestedEvent{
				Source: runstate.EventSource{
					Name: record.Source.Name, Path: append([]string(nil), record.Source.Path...),
					InvocationID: record.Source.InvocationID, InvocationType: record.Source.InvocationType,
				},
				ParentCallID: record.ParentCallID, SessionID: record.SessionID, ChildCursor: runstate.Cursor(record.ChildCursor),
				ChildRunID: record.ChildRunID, PayloadType: record.PayloadType,
				Payload: append(json.RawMessage(nil), record.Payload...),
			}); emitErr != nil {
				controls.stop()
				return runstate.EngineResult{}, emitErr
			}
			continue
		}
		if execution := event.Output.ToolExecution; execution != nil {
			emitErr := engine.emitToolExecution(ctx, request, execution, source, prepared.definition.Canonical, startedTools, emit)
			if execution.Phase == toolExecutionStarted {
				execution.acknowledgeStart(emitErr)
			}
			if emitErr != nil {
				controls.stop()
				return runstate.EngineResult{}, emitErr
			}
		}
		if variant := event.Output.MessageOutput; variant != nil {
			message, err := consumeMessageVariant(variant, source, !rootEvent, emit)
			if err != nil {
				controls.stop()
				if controlled, controlledErr, handled := engine.controlledLoopResult(controls, prepared, controlledTranscript(), emit); handled {
					return controlled, controlledErr
				}
				return runstate.EngineResult{}, err
			}
			if message == nil {
				continue
			}
			// Nested Agent messages are live display events. The enclosing task
			// tool returns the only result that belongs in the root transcript.
			if !rootEvent {
				continue
			}
			if message.Role == Assistant && variant.ModelResponseOrdinal > 0 {
				message.AgentMeta = &AgentMessageMeta{ModelResponseOrdinal: variant.ModelResponseOrdinal}
			}
			if message.Role == Assistant {
				usage := runstate.ModelUsage{}
				finishReason := ""
				if message.ResponseMeta != nil {
					finishReason = message.ResponseMeta.FinishReason
					if value := message.ResponseMeta.Usage; value != nil {
						usage = runstate.ModelUsage{
							PromptTokens: value.PromptTokens, CachedPromptTokens: value.PromptTokenDetails.CachedTokens,
							CompletionTokens: value.CompletionTokens, ReasoningTokens: value.CompletionTokensDetails.ReasoningTokens,
							TotalTokens: value.TotalTokens,
						}
					}
				}
				if err := emit(runstate.EngineModelCompleted{
					Usage: usage, FinishReason: finishReason,
					RequestedTools: modelRequestedToolNames(message.ToolCalls), Source: source,
				}); err != nil {
					controls.stop()
					return runstate.EngineResult{}, err
				}
			}
			transcript = append(transcript, CloneMessage(message))
			if message.Role == Assistant && len(message.ToolCalls) > 0 {
				pendingToolMessages = []*Message{message.Clone()}
				pendingToolCalls = make(map[string]struct{}, len(message.ToolCalls))
				for _, call := range message.ToolCalls {
					pendingToolCalls[strings.TrimSpace(call.ID)] = struct{}{}
				}
			} else if message.Role == ToolRole && len(pendingToolMessages) > 0 {
				pendingToolMessages = append(pendingToolMessages, message.Clone())
				delete(pendingToolCalls, strings.TrimSpace(message.ToolCallID))
				if len(pendingToolCalls) == 0 {
					if err := engine.commitCanonicalContext(
						ctx, request, prepared.definition.Canonical, ContextCommitToolBatch, contextOrdinal, pendingToolMessages,
					); err != nil {
						controls.stop()
						return runstate.EngineResult{}, err
					}
					contextOrdinal++
					pendingToolMessages = nil
					pendingToolCalls = nil
				}
			}
			if message.Role == Assistant {
				final = CloneMessage(message)
				if len(message.ToolCalls) > 0 {
					checkpoint, checkpointErr := encodeActiveEngineTranscript(
						prepared, transcript, activeModelUser, activeUserIndex,
					)
					if checkpointErr != nil {
						controls.stop()
						return runstate.EngineResult{}, checkpointErr
					}
					if err := emit(runstate.EngineTranscriptUpdated{State: checkpoint}); err != nil {
						controls.stop()
						return runstate.EngineResult{}, err
					}
				}
			}
		}
	}

	controls.stop()

loopControlsStopped:
	if controlErr := control.err(); controlErr != nil {
		return runstate.EngineResult{}, controlErr
	}
	switch control.kind() {
	case runstate.EngineControlPreempt:
		return engine.controlledResult(runstate.EnginePreempted, prepared, controlledTranscript(), emit)
	case runstate.EngineControlAbort:
		return engine.controlledResult(runstate.EngineAborted, prepared, controlledTranscript(), emit)
	}
	if completion.requestedCompletion() && final != nil && len(final.ToolCalls) != 0 {
		final = completionFinalAssistant(transcript[len(baseTranscript):], final)
		transcript = append(transcript, final.Clone())
	}
	if final == nil || len(final.ToolCalls) != 0 {
		return runstate.EngineResult{}, errors.New("Agent modelToolLoop completed without a final assistant message")
	}
	var finalCapabilityUpdates []runstate.EngineCapabilityState
	var finalCleanupCompleted *runstate.EngineCleanupCompleted
	if pendingCleanup != nil {
		nextCleanup, cleanupErr := pendingCleanup.finalState(transcript, request.Snapshot.OperationID, request.Snapshot.Cycle)
		if cleanupErr != nil {
			return runstate.EngineResult{}, fmt.Errorf("settle Agent Cleanup projection: %w", cleanupErr)
		}
		encodedCleanup, cleanupErr := json.Marshal(nextCleanup)
		if cleanupErr != nil {
			return runstate.EngineResult{}, cleanupErr
		}
		finalCapabilityUpdates = append(finalCapabilityUpdates, runstate.EngineCapabilityState{
			Capability: cleanupCapability, State: encodedCleanup,
		})
		finalCleanupCompleted = &runstate.EngineCleanupCompleted{
			ID: nextCleanup.ID, Reason: pendingCleanup.plan.Reason, Automatic: true,
			Metrics: runtimeCleanupMetrics(pendingCleanup.plan.Metrics),
		}
		cleanupState, cleanupPresent = nextCleanup, true
	}
	emitTrace(ctx, engine.trace, TraceEvent{
		Kind: TraceModelFinished, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	final, err = engine.commitCanonicalOutput(ctx, request, final, prepared.definition.Canonical)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	_, finishClass := classifyResponseFinishReason(final.ResponseMeta)
	incomplete := finishClass.Incomplete()
	var continuation *runstate.EngineContinuation
	if !incomplete {
		continuation, err = engine.evaluateGoal(
			ctx, request, input, prepared, capabilities, finalModelRequest, final, emit,
		)
		if err != nil {
			return runstate.EngineResult{}, err
		}
	}
	if len(transcript) == 0 || transcript[len(transcript)-1] == nil || transcript[len(transcript)-1].Role != Assistant {
		return runstate.EngineResult{}, errors.New("Agent transcript lost the final assistant message")
	}
	transcript[len(transcript)-1] = CloneMessage(final)
	encoded, err := encodeEngineTranscript(prepared, transcript)
	if err != nil {
		return runstate.EngineResult{}, fmt.Errorf("encode Agent transcript: %w", err)
	}
	if err := emit(runstate.EngineAssistantFinal{
		Content: final.Content, Thinking: final.ReasoningContent, State: encoded,
		CapabilityUpdates: finalCapabilityUpdates, CleanupCompleted: finalCleanupCompleted, Continuation: continuation,
	}); err != nil {
		return runstate.EngineResult{}, err
	}
	if incomplete {
		return runstate.EngineResult{Status: runstate.EngineIncomplete, Reason: finishClass.TerminalReason()}, nil
	}
	return runstate.EngineResult{Status: runstate.EngineCompleted}, nil
}

// controlledLoopResult translates an error from either the loop event lane or
// its public message stream only when a Run control actually caused it.
// Provider and projection errors remain ordinary failures.
func (engine *definitionEngine) controlledLoopResult(
	controls *definitionEngineControls,
	prepared preparedDefinition,
	baseTranscript []*Message,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error, bool) {
	if controlErr := controls.state.err(); controlErr != nil {
		return runstate.EngineResult{}, controlErr, true
	}
	switch controls.state.kind() {
	case runstate.EngineControlPreempt:
		result, err := engine.controlledResult(runstate.EnginePreempted, prepared, baseTranscript, emit)
		return result, err, true
	case runstate.EngineControlAbort:
		result, err := engine.controlledResult(runstate.EngineAborted, prepared, baseTranscript, emit)
		return result, err, true
	default:
		return runstate.EngineResult{}, nil, false
	}
}

// completionFinalAssistant turns the tool-call boundary that requested
// completion into the canonical assistant output. Some provider protocols
// emit the player-visible prose in an earlier assistant message and then send
// a tool-only submission message. Preserve that prose instead of publishing an
// empty final response.
func completionFinalAssistant(transcript []*Message, final *Message) *Message {
	completed := final.Clone()
	completed.ToolCalls = nil
	if strings.TrimSpace(completed.Content) != "" {
		return completed
	}
	for index := len(transcript) - 1; index >= 0; index-- {
		candidate := transcript[index]
		if candidate == nil || candidate.Role != Assistant || strings.TrimSpace(candidate.Content) == "" {
			continue
		}
		completed.Content = candidate.Content
		return completed
	}
	return completed
}

func (engine *definitionEngine) evaluateGoal(
	ctx context.Context,
	request runstate.EngineRequest,
	acceptedInput Input,
	prepared preparedDefinition,
	capabilities *capabilityStateClient,
	modelRequest *ModelRequestSnapshot,
	final *Message,
	emit runstate.EngineEventSink,
) (*runstate.EngineContinuation, error) {
	manager := prepared.definition.Goal
	if manager == nil {
		return nil, nil
	}
	state, present, err := capabilities.goal()
	if err != nil {
		return nil, err
	}
	decision, err := manager.AfterRun(ctx, GoalAfterRunRequest{
		Session: SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   acceptedInput,
		State:   state, Present: present, Result: Result{Status: ResultCompleted},
		ModelRequest: modelRequest, Final: CloneMessage(final),
	})
	if decision.Usage != nil {
		usage := decision.Usage
		if emitErr := emit(runstate.EngineModelCompleted{
			Usage: runstate.ModelUsage{
				PromptTokens: usage.PromptTokens, CachedPromptTokens: usage.PromptTokenDetails.CachedTokens,
				CompletionTokens: usage.CompletionTokens, ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
				TotalTokens: usage.TotalTokens,
			},
			FinishReason: decision.FinishReason,
		}); emitErr != nil {
			return nil, emitErr
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if emitErr := emit(runstate.EngineGoalEvaluationFailed{
			GoalID: state.ID, GoalRevision: state.Revision,
			Code: GoalEvaluationFailedCode, Detail: err.Error(),
		}); emitErr != nil {
			return nil, emitErr
		}
		slog.WarnContext(ctx, "Agent Goal evaluation failed; stopping autonomous continuation without changing Goal state",
			"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
			"goal_id", state.ID, "goal_revision", state.Revision, "error", err)
		return nil, nil
	}
	slog.InfoContext(ctx, "Agent Goal evaluation completed",
		"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
		"goal_id", state.ID, "goal_revision", state.Revision, "verdict", decision.Verdict,
		"reason", decision.Reason)
	switch decision.Verdict {
	case GoalVerdictComplete, GoalVerdictBlocked:
		kind := GoalComplete
		if decision.Verdict == GoalVerdictBlocked {
			kind = GoalBlock
		}
		_, updateErr := capabilities.updateGoal(ctx, manager,
			SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
			runViewForTurn(request.Snapshot), GoalMutation{
				Kind: kind, ExpectedID: state.ID, ExpectedRevision: state.Revision,
				Report:     decision.Reason,
				MutationID: fmt.Sprintf("goal-evaluation-%s-%d-%s", request.Snapshot.OperationID, request.Snapshot.Cycle, decision.Verdict),
			})
		if errors.Is(updateErr, ErrCapabilityStateConflict) {
			slog.InfoContext(ctx, "discarded stale Agent Goal terminal evaluation",
				"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
				"goal_id", state.ID, "goal_revision", state.Revision, "verdict", decision.Verdict)
			return nil, nil
		}
		if updateErr != nil {
			return nil, fmt.Errorf("commit Goal evaluation: %w", updateErr)
		}
		return nil, nil
	case GoalVerdictContinue:
		if strings.TrimSpace(decision.Input.Text) == "" {
			return nil, errors.New("Goal continuation requires a non-empty prompt")
		}
		if fenceErr := capabilities.assertGoalCurrent(); errors.Is(fenceErr, ErrCapabilityStateConflict) {
			slog.InfoContext(ctx, "discarded stale Agent Goal continuation",
				"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
				"goal_id", state.ID, "goal_revision", state.Revision)
			return nil, nil
		} else if fenceErr != nil {
			return nil, fmt.Errorf("fence Goal continuation: %w", fenceErr)
		}
	default:
		slog.WarnContext(ctx, "Agent Goal evaluator returned no actionable verdict; stopping autonomous continuation",
			"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
			"goal_id", state.ID, "goal_revision", state.Revision, "verdict", decision.Verdict)
		return nil, nil
	}
	input := decision.Input
	input.IdempotencyKey = ""
	encoded, runInput, err := encodeInput(input)
	if err != nil {
		return nil, fmt.Errorf("encode Goal continuation: %w", err)
	}
	runInput.Envelope = encoded
	fingerprint, err := hashCanonical(struct {
		OperationID string
		Cycle       int
		GoalID      string
		Revision    uint64
		Input       json.RawMessage
	}{string(request.Snapshot.OperationID), request.Snapshot.Cycle, state.ID, state.Revision, encoded})
	if err != nil {
		return nil, err
	}
	return &runstate.EngineContinuation{
		CommandID: runstate.CommandID("goal-continuation-" + fingerprint[:32]),
		Input:     runInput, Autonomous: true,
	}, nil
}

func (engine *definitionEngine) controlledResult(
	status runstate.EngineStatus,
	prepared preparedDefinition,
	messages []*Message,
	emit runstate.EngineEventSink,
) (runstate.EngineResult, error) {
	encoded, err := encodeEngineTranscript(prepared, messages)
	if err != nil {
		return runstate.EngineResult{}, err
	}
	if err := emit(runstate.EngineTranscriptUpdated{State: encoded}); err != nil {
		return runstate.EngineResult{}, err
	}
	return runstate.EngineResult{Status: status}, nil
}

func encodeEngineTranscript(prepared preparedDefinition, messages []*Message) (json.RawMessage, error) {
	return encodeEngineTranscriptState(prepared, messages, nil, 0)
}

func encodeActiveEngineTranscript(
	prepared preparedDefinition,
	messages []*Message,
	activeModelUser *Message,
	activeUserIndex int,
) (json.RawMessage, error) {
	if activeModelUser == nil || activeModelUser.Role != User {
		return nil, errors.New("encode active Agent transcript requires a model user projection")
	}
	if activeUserIndex < 0 || activeUserIndex >= len(messages) ||
		messages[activeUserIndex] == nil || messages[activeUserIndex].Role != User || IsContextStateMessage(messages[activeUserIndex]) {
		return nil, errors.New("encode active Agent transcript requires an exact raw user boundary")
	}
	return encodeEngineTranscriptState(prepared, messages, activeModelUser, activeUserIndex)
}

func encodeEngineTranscriptState(
	prepared preparedDefinition,
	messages []*Message,
	activeModelUser *Message,
	activeUserIndex int,
) (json.RawMessage, error) {
	encoded, err := json.Marshal(engineTranscript{
		Version: engineTranscriptVersion, DefinitionKey: prepared.definitionKey,
		BehaviorKey: prepared.behaviorKey, PrefixFingerprint: prepared.prefixFingerprint,
		MaterializedFingerprint: prepared.materializedFingerprint,
		DefinitionOperationID:   prepared.definitionOperationID,
		DefinitionCommandID:     prepared.definitionCommandID,
		DefinitionCycle:         prepared.definitionCycle, PreparationStage: prepared.preparationStage,
		Messages: cloneMessages(messages), ContextState: cloneContextStateSnapshot(prepared.contextState),
		ActiveModelUser: CloneMessage(activeModelUser), ActiveUserIndex: activeUserIndex,
		HostData: cloneHostData(prepared.hostData), ClearRevision: prepared.clearRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Agent transcript: %w", err)
	}
	return encoded, nil
}

func (state engineTranscript) ownsDefinition(snapshot runstate.TurnSnapshot) bool {
	return state.DefinitionOperationID != "" && state.DefinitionOperationID == string(snapshot.OperationID) &&
		state.DefinitionCommandID == string(snapshot.CommandID) && state.DefinitionCycle == snapshot.Cycle
}

// materializedDefinitionFingerprint freezes every cycle-specific Tool and
// Context value that can affect model-visible behavior. PrefixFingerprint is
// intentionally narrower and only protects the provider cache prefix.
func materializedDefinitionFingerprint(prepared preparedDefinition) (string, error) {
	return hashCanonical(struct {
		BehaviorKey        string
		Tools              []ToolDefinitionSnapshot
		Context            []contextFragmentIdentity
		GoalReservedTokens int
	}{
		BehaviorKey: prepared.behaviorKey,
		Tools:       append([]ToolDefinitionSnapshot(nil), prepared.toolSnapshots...),
		Context:     contextFragmentIdentities(prepared.fragments), GoalReservedTokens: prepared.goalReservedTokens,
	})
}

func decodeEngineTranscript(encoded json.RawMessage) (engineTranscript, error) {
	if len(encoded) == 0 || string(encoded) == "null" {
		return engineTranscript{Version: engineTranscriptVersion}, nil
	}
	var header struct {
		Version uint16 `json:"version"`
	}
	if err := json.Unmarshal(encoded, &header); err != nil {
		return engineTranscript{}, fmt.Errorf("decode Agent transcript header: %w", err)
	}
	if header.Version != engineTranscriptVersion {
		return engineTranscript{}, &unsupportedEngineTranscriptVersionError{version: header.Version}
	}
	var state engineTranscript
	if err := json.Unmarshal(encoded, &state); err != nil {
		return engineTranscript{}, fmt.Errorf("decode Agent transcript: %w", err)
	}
	state.Messages = cloneMessages(state.Messages)
	state.ContextState = cloneContextStateSnapshot(state.ContextState)
	state.ActiveModelUser = CloneMessage(state.ActiveModelUser)
	state.HostData = cloneHostData(state.HostData)
	if state.ActiveModelUser != nil {
		if state.ActiveModelUser.Role != User || state.ActiveUserIndex < 0 ||
			state.ActiveUserIndex >= len(state.Messages) || state.Messages[state.ActiveUserIndex] == nil ||
			state.Messages[state.ActiveUserIndex].Role != User || IsContextStateMessage(state.Messages[state.ActiveUserIndex]) {
			return engineTranscript{}, errors.New("Agent transcript has an invalid active model user projection")
		}
	}
	if err := validateContextStateSnapshot(state.ContextState, state.Messages); err != nil {
		return engineTranscript{}, err
	}
	return state, nil
}
