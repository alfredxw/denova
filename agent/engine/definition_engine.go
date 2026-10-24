package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentexecution "github.com/alfredxw/denova/agent/engine/execution"
	agentgoal "github.com/alfredxw/denova/agent/engine/goal"
	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentasync "github.com/alfredxw/denova/agent/internal/async"
	agentevent "github.com/alfredxw/denova/agent/lifecycle/event"
	agentinteraction "github.com/alfredxw/denova/agent/lifecycle/interaction"
	agenttrace "github.com/alfredxw/denova/agent/lifecycle/trace"
	agentmodel "github.com/alfredxw/denova/agent/model"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
	agenttool "github.com/alfredxw/denova/agent/tool"
	agentpermission "github.com/alfredxw/denova/agent/tool/permission"
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
	HistoryHead             agentcanonical.CanonicalHistoryHead `json:"history_head,omitempty"`
	Version                 uint16                              `json:"version"`
	DefinitionKey           string                              `json:"definition_key"`
	BehaviorKey             string                              `json:"behavior_key"`
	PrefixFingerprint       string                              `json:"prefix_fingerprint"`
	MaterializedFingerprint string                              `json:"materialized_fingerprint,omitempty"`
	DefinitionOperationID   string                              `json:"definition_operation_id,omitempty"`
	DefinitionCommandID     string                              `json:"definition_command_id,omitempty"`
	DefinitionCycle         int                                 `json:"definition_cycle,omitempty"`
	PreparationStage        enginePreparationStage              `json:"preparation_stage,omitempty"`
	PreparedContext         *preparedContext                    `json:"prepared_context,omitempty"`
	Archive                 *agenthistory.HistoryArchive        `json:"archive,omitempty"`
	Messages                []*agentschema.Message              `json:"messages,omitempty"`
	ContextState            agenthistory.ContextStateSnapshot   `json:"context_state,omitempty"`
	// ContextSequence is the next idempotency slot for this active cycle. It is
	// checkpointed with the transcript so a resumed run cannot shift sequence
	// numbers when an earlier context-state batch is already present.
	ContextSequence     int `json:"context_sequence,omitempty"`
	LastResponseOrdinal int `json:"last_response_ordinal,omitempty"`
	// ActiveModelUser is the model-only rendering of the accepted raw user
	// message while a tool batch or interaction is still active.
	// Messages remains the canonical raw transcript. Once the cycle settles,
	// this transient projection is discarded so canonical maintenance always
	// addresses stable raw messages.
	ActiveModelUser *agentschema.Message  `json:"active_model_user,omitempty"`
	ActiveUserIndex int                   `json:"active_user_index,omitempty"`
	HostData        *agentschema.HostData `json:"host_data,omitempty"`
	ClearRevision   uint64                `json:"clear_revision,omitempty"`
}

// Config binds a Native execution engine to one stable Session identity.
// The host owns admission, journal commits, and serialization of calls for
// that Session; lifecycle supplies those guarantees for normal SDK use.
type Config struct {
	Source  Source
	Session agentsession.Key
	Trace   agenttrace.TraceSink
	// CacheKeys overrides the stable Session-based provider cache identity.
	// Nil uses the built-in identity shared by execution and inspection.
	CacheKeys agentschema.CacheKeyGenerator
}

// New creates a Native engine without starting work or opening storage.
// Capabilities are prepared from Source for each admitted execution cycle.
func New(config Config) (*Engine, error) {
	if config.Source == nil {
		return nil, agentschema.ErrDefinitionUnavailable
	}
	key, err := agentsession.NormalizeKey(config.Session)
	if err != nil {
		return nil, err
	}
	return &Engine{
		source: config.Source, key: key, trace: config.Trace,
		cacheKeys: config.CacheKeys,
	}, nil
}

// Engine executes Native model/tool cycles. Requests carry recovery state and
// controls; the Engine does not own a Session, journal, queue, or task tree.
type Engine struct {
	source    Source
	key       agentsession.Key
	trace     agenttrace.TraceSink
	cacheKeys agentschema.CacheKeyGenerator
}

func (engine *Engine) Run(
	ctx context.Context,
	request Request,
	emit EventSink,
) (result Result, resultErr error) {
	if engine == nil || engine.source == nil {
		return Result{}, agentschema.ErrDefinitionUnavailable
	}
	if emit == nil {
		return Result{}, errors.New("Agent Engine Event sink is required")
	}
	ctx, controls := startDefinitionEngineControls(ctx, request.Controls)
	loopBound := false
	var preparationCheckpoint func() error
	defer func() {
		controls.close()
		if !loopBound {
			if controlled, controlledErr, handled := controls.controlledPreparationResult(resultErr); handled {
				// Suspension resumes this exact cycle from its last accepted
				// checkpoint. Rebuilding a partial preparation transcript here can
				// lose the active input boundary or overwrite committed context.
				// Abort and preemption instead retain the abandoned raw input.
				if controlled.Status != Suspended && preparationCheckpoint != nil {
					if checkpointErr := preparationCheckpoint(); checkpointErr != nil {
						result, resultErr = Result{}, checkpointErr
						return
					}
				}
				result, resultErr = controlled, controlledErr
			}
		}
	}()
	input, err := DecodeInput(request.Snapshot.Input)
	if err != nil {
		return Result{}, err
	}
	input.IdempotencyKey = string(request.Snapshot.CommandID)
	state, err := decodeEngineTranscript(request.Snapshot.State)
	if err != nil {
		return Result{}, err
	}
	clearState, clearPresent, err := applyClearToTranscript(&state, request.Snapshot.Capabilities)
	if err != nil {
		return Result{}, err
	}
	continuingInput := state.ownsDefinition(request.Snapshot) && state.ActiveModelUser != nil
	controlTranscript := agentschema.CloneMessages(state.Messages)
	if !continuingInput {
		controlTranscript = append(controlTranscript, agentschema.UserMessageWithAttachments(strings.TrimSpace(input.Text), input.Attachments))
	}
	var controlPrepared *preparedDefinition
	preparationCheckpoint = func() error {
		var encoded json.RawMessage
		var checkpointErr error
		if controlPrepared != nil {
			encoded, checkpointErr = encodeEngineTranscript(*controlPrepared, controlTranscript)
		} else {
			interrupted := state
			interrupted.Messages = agentschema.CloneMessages(controlTranscript)
			interrupted.HostData = agentschema.CloneHostData(input.HostData)
			encoded, checkpointErr = json.Marshal(interrupted)
		}
		if checkpointErr != nil {
			return fmt.Errorf("encode controlled Agent preparation transcript: %w", checkpointErr)
		}
		return emit(TranscriptUpdated{State: encoded})
	}
	currentCompactionStorage, currentCompactionStoragePresent, err := agenthistory.CompactionStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return Result{}, err
	}
	currentCompaction, currentCompactionPresent := currentCompactionStorage, currentCompactionStoragePresent
	currentCompaction, currentCompactionPresent = agenthistory.ClearCompaction(
		currentCompaction, currentCompactionPresent, clearState, clearPresent,
	)
	reason, err := turnReasonForSnapshot(request.Snapshot)
	if err != nil {
		return Result{}, err
	}
	prepareRequest := PrepareRequest{
		Session: agentschema.SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   input, Reason: reason,
		DefinitionKey: state.DefinitionKey, BehaviorKey: state.BehaviorKey,
		HostData:   agentschema.CloneHostData(input.HostData),
		Compaction: agenthistory.CompactionStatePointer(currentCompaction, currentCompactionPresent),
	}
	sameCycle := state.ownsDefinition(request.Snapshot)
	if !sameCycle {
		prepareRequest.DefinitionKey = ""
		prepareRequest.BehaviorKey = ""
	}
	prepared, err := prepareDefinitionBase(ctx, engine.source, prepareRequest)
	if err != nil {
		return Result{}, err
	}
	if sameCycle {
		prepared.contextSequence = state.ContextSequence
		prepared.lastResponseOrdinal = state.LastResponseOrdinal
		prepared.activeModelUser, prepared.activeUserIndex = agentschema.CloneMessage(state.ActiveModelUser), state.ActiveUserIndex
	}
	prepared.hostData = agentschema.CloneHostData(input.HostData)
	prepared.clearRevision = state.ClearRevision
	prepared.contextState = agenthistory.CloneContextStateSnapshot(state.ContextState)
	prepared.archive = state.Archive
	prepared.historyHead = state.HistoryHead
	prepared.elision, err = agenthistory.ElisionStateFrom(request.Snapshot.Capabilities)
	if err != nil {
		return Result{}, err
	}
	prepared.definitionOperationID = string(request.Snapshot.OperationID)
	prepared.definitionCommandID = string(request.Snapshot.CommandID)
	prepared.definitionCycle = request.Snapshot.Cycle
	prepared.preparationStage = enginePreparationBase
	resumeMaterialized := sameCycle && state.PreparationStage == enginePreparationMaterialized
	if sameCycle && state.DefinitionKey != "" && state.DefinitionKey != prepared.definitionKey {
		return Result{}, fmt.Errorf("%w: definition_key have=%q want=%q", agentschema.ErrDefinitionMismatch, prepared.definitionKey, state.DefinitionKey)
	}
	if sameCycle && state.BehaviorKey != "" && state.BehaviorKey != prepared.behaviorKey {
		return Result{}, fmt.Errorf("%w: behavior_key changed", agentschema.ErrDefinitionMismatch)
	}
	// Persist the exact base Definition before materializing dynamic capability
	// state. The Run has already committed canonical accepted input; the
	// prepared Definition must prove it resolves the same canonical boundary.
	if !resumeMaterialized {
		controlPrepared = &prepared
		preparedCheckpoint, err := encodeEngineTranscriptState(prepared, state.Messages, prepared.activeModelUser, prepared.activeUserIndex)
		if err != nil {
			return Result{}, fmt.Errorf("encode pre-commit Agent transcript: %w", err)
		}
		if err := emit(TranscriptUpdated{State: preparedCheckpoint}); err != nil {
			return Result{}, err
		}
	}
	if err := engine.verifyCanonicalInputCommit(request.Snapshot, input, prepared.definition.Canonical); err != nil {
		return Result{}, err
	}
	if request.Snapshot.OutputCommit != nil {
		return engine.resumeCommittedOutput(ctx, request, input, prepared, state, emit)
	}
	var savedContext *preparedContext
	if resumeMaterialized {
		savedContext = state.PreparedContext
	}
	if err := engine.materializeCycleCapabilities(ctx, prepareRequest, request.Snapshot, savedContext, &prepared); err != nil {
		return Result{}, err
	}
	materializedFingerprint, err := materializedDefinitionFingerprint(prepared)
	if err != nil {
		return Result{}, err
	}
	if sameCycle && state.PreparationStage == enginePreparationMaterialized &&
		state.MaterializedFingerprint != materializedFingerprint {
		return Result{}, fmt.Errorf("%w: materialized Definition changed", agentschema.ErrDefinitionMismatch)
	}
	prepared.materializedFingerprint = materializedFingerprint
	prepared.preparationStage = enginePreparationMaterialized
	controlPrepared = &prepared
	materializedCheckpoint, err := encodeEngineTranscriptState(prepared, state.Messages, prepared.activeModelUser, prepared.activeUserIndex)
	if err != nil {
		return Result{}, fmt.Errorf("encode materialized Agent transcript: %w", err)
	}
	if err := emit(TranscriptUpdated{State: materializedCheckpoint}); err != nil {
		return Result{}, err
	}
	if continuingInput {
		if err := engine.restorePendingToolBatch(ctx, request, &prepared, &state, emit); err != nil {
			return Result{}, err
		}
		if accepted, ok := prepared.definition.Canonical.(agentcanonical.CanonicalPreparedOutput); ok {
			final, err := accepted.PendingOutput(ctx, canonicalCommitIdentity(engine.key, request.Snapshot, agentcanonical.CommitOutput))
			if err != nil {
				return Result{}, err
			}
			if final != nil {
				if final.Role != agentschema.Assistant || len(final.ToolCalls) != 0 {
					return Result{}, errors.New("prepared product output must be a final assistant message")
				}
				if err := agentasync.AdmitWork(ctx); err != nil {
					return Result{}, err
				}
				committed, err := engine.commitCanonicalOutput(ctx, request, final, append(agentschema.CloneMessages(state.Messages), final), prepared.archive.Local(state.ActiveUserIndex), prepared.definition.Canonical)
				if err != nil {
					return Result{}, err
				}
				state.Messages = append(state.Messages, committed.output)
				if committed.canonicalMessages != nil {
					state.Messages = committed.canonicalMessages
				}
				return engine.settleCommittedOutput(ctx, request, input, prepared, state, emit)
			}
		}
	}
	// Closures below retain the request throughout the loop. The decoded
	// active window owns recovery now; do not pin its original serialized body.
	request.Snapshot.State = nil
	compaction, compactionPresent := currentCompaction, currentCompactionPresent
	stateMessages, nextContextState, err := prepared.archive.AdvanceContextState(
		state.Messages, prepared.fragments, prepared.contextState, compaction, compactionPresent,
	)
	if err != nil {
		return Result{}, err
	}
	prepared.contextState = nextContextState
	cycleStateTranscript := append(agentschema.CloneMessages(state.Messages), agentschema.CloneMessages(stateMessages)...)
	if len(stateMessages) > 0 {
		sequence := prepared.contextSequence
		prepared.contextSequence++
		checkpoint, err := encodeEngineTranscriptState(prepared, cycleStateTranscript, prepared.activeModelUser, prepared.activeUserIndex)
		if err != nil {
			return Result{}, err
		}
		if err := engine.commitCanonicalContext(
			ctx, request, prepared.definition.Canonical, sequence, stateMessages, TranscriptUpdated{State: checkpoint},
		); err != nil {
			return Result{}, err
		}
	}

	summaryLimit := 0
	if prepared.definition.Compaction != nil {
		summaryLimit = prepared.definition.Compaction.SummaryLimitBytes()
	}
	effectiveTranscript, err := prepared.archive.EffectiveHistoryMessages(cycleStateTranscript, prepared.elision, compaction, compactionPresent, summaryLimit)
	if err != nil {
		return Result{}, err
	}
	activeUserIndex := prepared.archive.Count(cycleStateTranscript)
	var resumedTail []*agentschema.Message
	if continuingInput {
		activeUserIndex = state.ActiveUserIndex
		modelUserIndex := prepared.archive.CompactionMessageIndex(cycleStateTranscript, compaction, compactionPresent, activeUserIndex)
		if modelUserIndex < 0 || modelUserIndex >= len(effectiveTranscript) {
			return Result{}, errors.New("active Agent input was removed from the recoverable context")
		}
		resumedTail = agentschema.CloneMessages(effectiveTranscript[modelUserIndex+1:])
		effectiveTranscript = effectiveTranscript[:modelUserIndex]
	}
	modelMessages, activeModelUser, err := assembleCycleMessages(effectiveTranscript, input.Text, input.Attachments, prepared.fragments, prepared.definition.AttachmentRoot)
	if err != nil {
		return Result{}, err
	}
	modelMessages = append(modelMessages, resumedTail...)
	prepared.activeModelUser, prepared.activeUserIndex = activeModelUser, activeUserIndex
	stablePrefixMessages := stableContextPrefixMessages(prepared.fragments, compaction, compactionPresent)
	baseTranscript := agentschema.CloneMessages(cycleStateTranscript)
	if !continuingInput {
		baseTranscript = append(baseTranscript, agentschema.UserMessageWithAttachments(strings.TrimSpace(input.Text), input.Attachments))
	}
	activeCheckpoint, err := encodeActiveEngineTranscript(prepared, baseTranscript, activeModelUser, activeUserIndex)
	if err != nil {
		return Result{}, err
	}
	if err := emit(TranscriptUpdated{State: activeCheckpoint}); err != nil {
		return Result{}, err
	}
	controlTranscript = agentschema.CloneMessages(baseTranscript)
	agenttrace.EmitTrace(ctx, engine.trace, agenttrace.TraceEvent{
		Kind: agenttrace.TraceCycleStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	agenttrace.EmitTrace(ctx, engine.trace, agenttrace.TraceEvent{
		Kind: agenttrace.TraceModelStarted, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	middlewares := append([]agentmiddleware.Middleware(nil), prepared.definition.Middlewares...)
	permission := agentpermission.EffectivePermissionPolicy(prepared.definition.Permission)
	permissionStage := &permissionMiddleware{
		BaseMiddleware: &agentmiddleware.BaseMiddleware{}, policy: permission,
		session:     agentschema.SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		run:         runViewForTurn(request.Snapshot),
		attachments: agentschema.AttachmentsFromMessages(modelMessages),
	}
	transcript := agentschema.CloneMessages(baseTranscript)
	pendingToolTranscriptIndex := -1
	controlledTranscript := func() []*agentschema.Message {
		if pendingToolTranscriptIndex >= 0 {
			return agentschema.CloneMessages(transcript[:pendingToolTranscriptIndex+1])
		}
		return agentschema.CloneMessages(transcript)
	}
	maintenanceGate := modelCallGate(nil)
	if len(prepared.definition.Middlewares) != 0 || prepared.definition.Compaction != nil || prepared.definition.Elision != nil {
		maintenanceGate = func(
			gateCtx context.Context,
			call *modelCall,
			modelContext *modelStepContext,
		) (*preparedModelCall, error) {
			if metrics, ok := modelContext.TakeContextNormalization(); ok {
				if err := emit(ContextNormalized{
					RepairCount: metrics.RepairCount, MessagesBefore: metrics.MessagesBefore, MessagesAfter: metrics.MessagesAfter,
				}); err != nil {
					return nil, err
				}
			}
			if call == nil {
				return nil, errors.New("Agent maintenance gate received a nil model call")
			}
			// Freeze the actual provider projection before taking the side fork.
			// Portable loop/journal messages remain separate from runtime paths.
			providerMessages, projectionErr := projectToolArtifactPaths(gateCtx, prepared.definition.Artifacts, call.Messages)
			if projectionErr != nil {
				return nil, projectionErr
			}
			call.providerMessages = providerMessages
			nextElision, elided, elisionErr := prepareElision(gateCtx, prepared, controlledTranscript(), compaction, compactionPresent, call.Snapshot(),
				func(next agenthistory.ElisionRecord) (*preparedModelCall, error) {
					nextPrepared := prepared
					nextPrepared.elision = next
					candidate, _, err := prepareHistoryModelCall(nextPrepared, transcript, compaction, compactionPresent, input, activeUserIndex, modelContext)
					return candidate, err
				})
			if gateCtx.Err() != nil {
				return nil, gateCtx.Err()
			}
			if elisionErr != nil {
				slog.WarnContext(gateCtx, "Agent Elision preparation failed; retaining tool bodies", "session", engine.key, "error", elisionErr)
			} else if elided != nil {
				encoded, err := json.Marshal(nextElision)
				if err != nil {
					return nil, err
				}
				if err := emit(CapabilityState{Capability: agenthistory.ElisionCapability, State: encoded}); err != nil {
					return nil, err
				}
				prepared.elision = nextElision
				call = elided.call
				slog.InfoContext(gateCtx, "Agent Elision committed", "session", engine.key, "revision", nextElision.Revision,
					"results_elided", nextElision.Metrics.ResultsElided, "tokens_before", nextElision.Metrics.TokensBefore,
					"tokens_after", nextElision.Metrics.TokensAfter, "cache_prefix_tokens", nextElision.Metrics.CacheExpectedPrefixTokens)
			}
			if prepared.definition.Compaction == nil {
				return elided, nil
			}
			compactionContext, projectionErr := agenthistory.ElisionForHistory(prepared.elision, compaction, compactionPresent).ProjectArchive(controlledTranscript(), prepared.archive)
			if projectionErr != nil {
				return nil, projectionErr
			}
			// Preserve the exact rendered active instruction in the summary source.
			compactionContext[prepared.archive.Local(activeUserIndex)] = activeModelUser.Clone()
			compactionContext, projectionErr = projectToolArtifactPaths(gateCtx, prepared.definition.Artifacts, compactionContext)
			if projectionErr != nil {
				return nil, projectionErr
			}
			modelSnapshot := call.Snapshot()
			fingerprint, fingerprintErr := automaticCompactionFingerprint(
				prepared, compaction, compactionPresent, modelSnapshot,
			)
			if fingerprintErr != nil {
				return nil, fingerprintErr
			}
			health, healthPresent, healthErr := compactionHealthStateFrom(request.Snapshot.Capabilities)
			if healthErr != nil {
				return nil, healthErr
			}
			failureLimit := normalizedAutomaticCompactionFailureLimit(prepared.definition.Execution)
			checkpointID := fmt.Sprintf("compaction-%s-%d-%d", request.Snapshot.OperationID, request.Snapshot.Cycle, max(compaction.Revision, currentCompactionStorage.Revision)+1)
			if healthPresent && health.Fingerprint == fingerprint && health.ConsecutiveFailures >= failureLimit {
				if err := emit(CompactionSkipped{
					ID: checkpointID, Reason: "consecutive_failure_fuse", Automatic: true,
					ConsecutiveFailures: health.ConsecutiveFailures, FailureFuseOpen: true,
					Metrics: runtimeCompactionMetrics(compaction.Metrics),
				}); err != nil {
					return nil, err
				}
				return elided, nil
			}
			var candidate *preparedModelCall
			var candidatePrepared preparedDefinition
			var candidateStateMessages []*agentschema.Message
			var candidateModelUser *agentschema.Message
			buildAfter := func(next agenthistory.CompactionRecord) (*agentmodel.ModelRequestSnapshot, error) {
				nextPrepared := prepared
				nextRequest := prepareRequest
				nextRequest.Compaction = agenthistory.CompactionStatePointer(next, true)
				if err := rematerializeDefinitionContext(gateCtx, nextRequest, &nextPrepared); err != nil {
					return nil, err
				}
				stateMessages, contextState, err := prepared.archive.AdvanceContextState(
					transcript, nextPrepared.fragments, nextPrepared.contextState, next, true,
				)
				if err != nil {
					return nil, err
				}
				nextPrepared.contextState = contextState
				// Replace only the selected historical prefix. The active user and
				// all settled assistant/tool/task messages stay in their raw order,
				// including a tail restored from an interrupted invocation.
				candidateRaw := append(agentschema.CloneMessages(transcript), agentschema.CloneMessages(stateMessages)...)
				var modelUser *agentschema.Message
				candidate, modelUser, err = prepareHistoryModelCall(nextPrepared, candidateRaw, next, true, input, activeUserIndex, modelContext)
				if err != nil {
					return nil, err
				}
				candidatePrepared, candidateStateMessages, candidateModelUser = nextPrepared, stateMessages, modelUser
				return candidate.call.Snapshot(), nil
			}
			next, nextPresent, changed, compactMetrics, compactErr := engine.prepareAutomaticCompaction(
				gateCtx, request, prepared, controlledTranscript(), compactionContext, modelSnapshot,
				compaction, compactionPresent,
				currentCompactionStorage,
				buildAfter,
				emit,
			)
			if compactErr != nil {
				if gateCtx.Err() != nil {
					return nil, gateCtx.Err()
				}
				nextHealth := nextCompactionHealth(health, healthPresent, fingerprint, compactErr)
				if err := emitCompactionHealth(emit, nextHealth); err != nil {
					return nil, err
				}
				if request.Snapshot.Capabilities == nil {
					request.Snapshot.Capabilities = make(map[string]json.RawMessage)
				}
				request.Snapshot.Capabilities[agenthistory.CompactionHealthCapability], _ = json.Marshal(nextHealth)
				if err := emit(CompactionFailed{
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
				// Automatic maintenance is a recoverable side fork. The unchanged
				// request still passes through the provider input guard, which owns
				// the non-negotiable hard limit.
				return elided, nil
			}
			if err := clearCompactionHealth(emit, healthPresent); err != nil {
				return nil, err
			}
			delete(request.Snapshot.Capabilities, agenthistory.CompactionHealthCapability)
			next, nextPresent = agenthistory.ClearCompaction(next, nextPresent, clearState, clearPresent)
			if !changed {
				return elided, nil
			}
			candidateTranscript := append(agentschema.CloneMessages(transcript), agentschema.CloneMessages(candidateStateMessages)...)
			sequence := candidatePrepared.contextSequence
			if len(candidateStateMessages) > 0 {
				candidatePrepared.contextSequence++
			}
			candidatePrepared.activeModelUser, candidatePrepared.activeUserIndex = candidateModelUser, activeUserIndex
			checkpoint, err := encodeActiveEngineTranscript(candidatePrepared, candidateTranscript, candidateModelUser, activeUserIndex)
			if err != nil {
				return nil, err
			}
			compactionState, err := json.Marshal(next)
			if err != nil {
				return nil, err
			}
			transition := TranscriptUpdated{State: checkpoint, CapabilityStates: map[string]json.RawMessage{agenthistory.CompactionCapability: compactionState}}
			if err := engine.commitCanonicalContext(gateCtx, request, prepared.definition.Canonical, sequence, candidateStateMessages, transition); err != nil {
				return nil, err
			}
			if err := emit(transition); err != nil {
				return nil, err
			}
			compaction, compactionPresent = next, nextPresent
			prepareRequest.Compaction = agenthistory.CompactionStatePointer(compaction, compactionPresent)
			prepared, transcript, activeModelUser = candidatePrepared, candidateTranscript, candidateModelUser
			if prepared.historyHead.Identity != "" {
				transcript, prepared.archive = agenthistory.ArchiveHistory(transcript, prepared.archive, compaction, prepared.contextState)
				localActiveUser := prepared.archive.Local(activeUserIndex) + 1
				if localActiveUser < 0 {
					localActiveUser = 0
				} else if localActiveUser > len(transcript) {
					localActiveUser = len(transcript)
				}
				baseTranscript = transcript[:localActiveUser]
			}
			currentCompactionStorage = compaction
			return candidate, nil
		}
	}
	var finalModelRequest *agentmodel.ModelRequestSnapshot
	modelCallGate := maintenanceGate
	if rawGoal, goalPresent := request.Snapshot.Capabilities[agentgoal.GoalCapability]; prepared.definition.Goal != nil && goalPresent {
		activeGoal, goalErr := agentgoal.DecodeGoalState(rawGoal)
		if goalErr != nil {
			return Result{}, goalErr
		}
		if activeGoal.Active() {
			modelCallGate = func(gateCtx context.Context, call *modelCall, modelContext *modelStepContext) (*preparedModelCall, error) {
				if maintenanceGate != nil {
					restart, gateErr := maintenanceGate(gateCtx, call, modelContext)
					if gateErr != nil {
						return nil, gateErr
					}
					if restart != nil {
						finalModelRequest = restart.call.Snapshot()
						return restart, nil
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
		return Result{}, err
	}

	runOption, cancelLoop := newLoopCancellation()
	completion := &runCompletionControl{cancel: cancelLoop}
	control := controls.state
	interactions := newEngineInteractionClient(agentinteraction.EffectiveInteractionPolicy(prepared.definition.Interaction), emit)
	acceptedControl := controls.bindLoop(cancelLoop, interactions)
	loopBound = true
	state.Messages, controlTranscript, cycleStateTranscript, effectiveTranscript = nil, nil, nil, nil
	if acceptedControl == ControlPreempt {
		return engine.controlledResult(Preempted, prepared, baseTranscript, emit)
	}
	if acceptedControl == ControlAbort {
		return engine.controlledResult(Aborted, prepared, baseTranscript, emit)
	}
	if acceptedControl == ControlSuspend {
		return engine.controlledResult(Suspended, prepared, baseTranscript, emit)
	}

	capabilities := newCapabilityStateClient(request.Snapshot.Capabilities, emit)
	loopCtx := agentexecution.ContextWithCapabilityState(ctx, capabilities)
	loopCtx = agentinteraction.ContextWithInteractionClient(loopCtx, interactions)
	// Concrete tools are not allowed to run until their queued start event has
	// crossed the Run boundary. This also orders model checkpoints and
	// ToolCallStarted before an Ask/Permission interaction emitted by the tool.
	loopCtx = contextWithToolStartReceipt(loopCtx)
	loopCtx = agentexecution.ContextWithCompletionRequest(loopCtx, completion.requestCompletion)
	scope, _ := agentsession.CanonicalKey(engine.key)
	loopCtx = agentexecution.ContextWithInvocationIdentity(loopCtx, agentexecution.InvocationIdentity{
		Scope: scope, OperationID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	loopCtx = context.WithValue(loopCtx, agentexecution.ModelResponseSeedKey{}, prepared.lastResponseOrdinal)
	loopCtx, err = contextWithProviderCacheKey(loopCtx, engine.key, engine.cacheKeys)
	if err != nil {
		return Result{}, err
	}
	iterator := loop.Run(loopCtx, &loopInput{
		Messages: modelMessages, EnableStreaming: true,
		stablePrefixMessages: stablePrefixMessages,
	}, runOption)
	defer func() {
		// Even when journal/event delivery fails, all concrete tool producers
		// must stop before the Session may release its writer lease.
		controls.cancel()
		for {
			event, ok := iterator.Next()
			if !ok {
				break
			}
			if event != nil && event.Output != nil && event.Output.MessageOutput != nil {
				if stream := event.Output.MessageOutput.MessageStream; stream != nil {
					stream.Close()
				}
			}
		}
	}()
	startedTools := make(map[string]bool)
	var final *agentschema.Message
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
			return Result{}, event.Err
		}
		if event.Output == nil {
			continue
		}
		source := runtimeEventSource(event)
		rootEvent := rootAgentEvent(event, prepared.definition.Name)
		if boundary := event.Output.ModelAttempt; boundary != nil {
			prepared.lastResponseOrdinal = boundary.Ordinal
			checkpoint, err := encodeActiveEngineTranscript(prepared, transcript, activeModelUser, activeUserIndex)
			if err == nil {
				err = emit(TranscriptUpdated{State: checkpoint})
			}
			boundary.Receipt <- err
			if err != nil {
				controls.stop()
				return Result{}, err
			}
			continue
		}
		if boundary := event.Output.TaskCompletions; boundary != nil {
			if !rootEvent {
				err := errors.New("nested Agent emitted a task completion boundary into the root transcript")
				boundary.acknowledge(err)
				controls.stop()
				return Result{}, err
			}
			ids, messages := boundary.snapshot()
			sequence := prepared.contextSequence
			prepared.contextSequence++
			transcript = append(transcript, agentschema.CloneMessages(messages)...)
			checkpoint, checkpointErr := encodeActiveEngineTranscript(
				prepared, transcript, activeModelUser, activeUserIndex,
			)
			if checkpointErr == nil {
				checkpointErr = engine.commitCanonicalContext(ctx, request, prepared.definition.Canonical, sequence, messages, TranscriptUpdated{State: checkpoint})
			}
			if checkpointErr == nil {
				checkpointErr = emit(TranscriptUpdated{
					State: checkpoint, TaskCompletionIDs: append([]string(nil), ids...),
				})
			}
			boundary.acknowledge(checkpointErr)
			if checkpointErr != nil {
				controls.stop()
				return Result{}, checkpointErr
			}
			continue
		}
		if boundary := event.Output.Continuation; boundary != nil {
			var continuationErr error
			if !rootEvent {
				continuationErr = errors.New("nested Agent emitted a continuation into the root transcript")
			} else if len(transcript) == 0 || transcript[len(transcript)-1].Role != agentschema.Assistant || pendingToolTranscriptIndex >= 0 {
				continuationErr = errors.New("Agent continuation has no accepted assistant response")
			} else {
				continuationErr = agentcanonical.ValidateContextCommitMessages(boundary.messages)
			}
			if continuationErr == nil {
				// The ordinary response event already appended this assistant. The
				// canonical batch records it once together with the new user feedback.
				transcript = append(transcript[:len(transcript)-1], agentschema.CloneMessages(boundary.messages)...)
				sequence := prepared.contextSequence
				prepared.contextSequence++
				var checkpoint json.RawMessage
				checkpoint, continuationErr = encodeActiveEngineTranscript(prepared, transcript, activeModelUser, activeUserIndex)
				if continuationErr == nil {
					continuationErr = engine.commitCanonicalContext(ctx, request, prepared.definition.Canonical, sequence, boundary.messages, TranscriptUpdated{State: checkpoint})
				}
				if continuationErr == nil {
					continuationErr = emit(TranscriptUpdated{State: checkpoint})
				}
			}
			boundary.receipt <- continuationErr
			if continuationErr != nil {
				controls.stop()
				return Result{}, continuationErr
			}
			continue
		}
		if boundary := event.Output.ToolBatch; boundary != nil {
			if !rootEvent {
				err := errors.New("nested Agent emitted a canonical tool batch boundary into the root transcript")
				boundary.acknowledge(err)
				controls.stop()
				return Result{}, err
			}
			phase, messages := boundary.snapshot()
			var boundaryErr error
			if pendingToolTranscriptIndex < 0 || pendingToolTranscriptIndex != len(transcript)-1 {
				boundaryErr = errors.New("canonical tool batch has no pending transcript owner")
			} else {
				switch phase {
				case toolBatchPrepared:
					if len(messages) != 1 {
						boundaryErr = errors.New("prepared canonical tool batch requires one assistant message")
						break
					}
					var canonical *agentschema.Message
					canonical, boundaryErr = canonicalToolBatchAssistant(transcript[pendingToolTranscriptIndex], messages[0])
					if boundaryErr == nil {
						transcript[pendingToolTranscriptIndex] = canonical
						final = canonical.Clone()
					}
				case toolBatchCompleted:
					var completed []*agentschema.Message
					completed, boundaryErr = completedCanonicalToolBatch(transcript[pendingToolTranscriptIndex], messages)
					if boundaryErr == nil {
						sequence := prepared.contextSequence
						prepared.contextSequence++
						transcript = append(transcript[:pendingToolTranscriptIndex], completed...)
						final = completed[0].Clone()
						pendingToolTranscriptIndex = -1
						var checkpoint json.RawMessage
						checkpoint, boundaryErr = encodeActiveEngineTranscript(prepared, transcript, activeModelUser, activeUserIndex)
						if boundaryErr == nil {
							boundaryErr = engine.commitCanonicalContext(ctx, request, prepared.definition.Canonical, sequence, completed, TranscriptUpdated{State: checkpoint})
						}
					}
				default:
					boundaryErr = fmt.Errorf("unsupported canonical tool batch phase %q", phase)
				}
			}
			if boundaryErr == nil {
				var checkpoint []byte
				checkpoint, boundaryErr = encodeActiveEngineTranscript(
					prepared, transcript, activeModelUser, activeUserIndex,
				)
				if boundaryErr == nil {
					boundaryErr = emit(TranscriptUpdated{State: checkpoint})
				}
			}
			boundary.acknowledge(boundaryErr)
			if boundaryErr != nil {
				controls.stop()
				return Result{}, boundaryErr
			}
			continue
		}
		if nested := event.Output.NestedEvent; nested != nil {
			record, encodeErr := agentevent.EncodeNestedEvent(*nested)
			if encodeErr != nil {
				controls.stop()
				return Result{}, encodeErr
			}
			if emitErr := emit(NestedEvent{
				Source: EventSource{
					Name: record.Source.Name, Path: append([]string(nil), record.Source.Path...),
					InvocationID: record.Source.InvocationID, InvocationType: record.Source.InvocationType,
				},
				ParentCallID: record.ParentCallID, SessionID: record.SessionID, ChildCursor: Cursor(record.ChildCursor),
				ChildRunID: record.ChildRunID, PayloadType: record.PayloadType,
				Payload: append(json.RawMessage(nil), record.Payload...),
			}); emitErr != nil {
				controls.stop()
				return Result{}, emitErr
			}
			continue
		}
		if retry := event.Output.ModelRetry; retry != nil {
			if err := emit(ModelRetry{
				Source: source, Attempt: retry.Attempt, MaxAttempts: retry.MaxAttempts,
				ResponseOrdinal: retry.ResponseOrdinal, OutputState: string(retry.OutputState),
				Delay: retry.Delay, Reason: retry.Reason,
			}); err != nil {
				controls.stop()
				return Result{}, err
			}
			continue
		}
		if execution := event.Output.ToolExecution; execution != nil {
			emitErr := engine.emitToolExecution(ctx, request, execution, source, prepared.definition.Effects, startedTools, emit)
			if execution.Phase == toolExecutionStarted {
				execution.acknowledgeStart(emitErr)
			}
			if execution.finishReceipt != nil {
				execution.finishReceipt <- emitErr
			}
			if emitErr != nil {
				controls.stop()
				return Result{}, emitErr
			}
		}
		if variant := event.Output.MessageOutput; variant != nil {
			message, err := consumeMessageVariant(variant, source, !rootEvent, emit)
			if err != nil {
				controls.stop()
				if controlled, controlledErr, handled := engine.controlledLoopResult(controls, prepared, controlledTranscript(), emit); handled {
					return controlled, controlledErr
				}
				return Result{}, err
			}
			if message == nil {
				continue
			}
			// Nested Agent messages are live display events. The enclosing task
			// tool returns the only result that belongs in the root transcript.
			if !rootEvent {
				continue
			}
			if message.Role == agentschema.Assistant && variant.ModelResponseOrdinal > 0 {
				message.AgentMeta = &agentschema.AgentMessageMeta{ModelResponseOrdinal: variant.ModelResponseOrdinal}
			}
			if message.Role == agentschema.Assistant {
				usage := ModelUsage{}
				finishReason := ""
				if message.ResponseMeta != nil {
					finishReason = message.ResponseMeta.FinishReason
					if value := message.ResponseMeta.Usage; value != nil {
						usage = ModelUsage{
							PromptTokens: value.PromptTokens, CachedPromptTokens: value.PromptTokenDetails.CachedTokens,
							CompletionTokens: value.CompletionTokens, ReasoningTokens: value.CompletionTokensDetails.ReasoningTokens,
							TotalTokens: value.TotalTokens,
						}
					}
				}
				if err := emit(ModelCompleted{
					Usage: usage, FinishReason: finishReason,
					RequestedTools: modelRequestedToolNames(message.ToolCalls), Source: source,
				}); err != nil {
					controls.stop()
					return Result{}, err
				}
			}
			if variant.discarded {
				continue
			}
			if message.Role == agentschema.ToolRole {
				if pendingToolTranscriptIndex < 0 {
					controls.stop()
					return Result{}, errors.New("tool result arrived without a canonical tool batch boundary")
				}
				// The completed boundary owns the whole canonical batch. Tool message
				// events remain live display/lifecycle notifications only.
				continue
			}
			transcript = append(transcript, agentschema.CloneMessage(message))
			if message.Role == agentschema.Assistant && len(message.ToolCalls) > 0 {
				if pendingToolTranscriptIndex >= 0 {
					controls.stop()
					return Result{}, errors.New("assistant tool batch arrived before the prior batch completed")
				}
				pendingToolTranscriptIndex = len(transcript) - 1
			}
			if message.Role == agentschema.Assistant {
				final = agentschema.CloneMessage(message)
				if len(message.ToolCalls) > 0 {
					checkpoint, checkpointErr := encodeActiveEngineTranscript(
						prepared, transcript, activeModelUser, activeUserIndex,
					)
					if checkpointErr != nil {
						controls.stop()
						return Result{}, checkpointErr
					}
					if err := emit(TranscriptUpdated{State: checkpoint}); err != nil {
						controls.stop()
						return Result{}, err
					}
				}
			}
		}
	}

	controls.stop()

loopControlsStopped:
	if controlErr := control.err(); controlErr != nil {
		return Result{}, controlErr
	}
	switch control.kind() {
	case ControlPreempt:
		return engine.controlledResult(Preempted, prepared, controlledTranscript(), emit)
	case ControlAbort:
		return engine.controlledResult(Aborted, prepared, controlledTranscript(), emit)
	case ControlSuspend:
		return engine.controlledResult(Suspended, prepared, controlledTranscript(), emit)
	}
	if completion.requestedCompletion() && final != nil && len(final.ToolCalls) != 0 {
		final = completionFinalAssistant(transcript[len(baseTranscript):], final)
		transcript = append(transcript, final.Clone())
	}
	if final == nil || len(final.ToolCalls) != 0 {
		return Result{}, errors.New("Agent modelToolLoop completed without a final assistant message")
	}
	agenttrace.EmitTrace(ctx, engine.trace, agenttrace.TraceEvent{
		Kind: agenttrace.TraceModelFinished, Session: engine.key, RunID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle,
	})
	committed, err := engine.commitCanonicalOutput(ctx, request, final, transcript, prepared.archive.Local(activeUserIndex), prepared.definition.Canonical)
	if err != nil {
		return Result{}, err
	}
	final = committed.output
	if len(transcript) == 0 || transcript[len(transcript)-1] == nil || transcript[len(transcript)-1].Role != agentschema.Assistant {
		return Result{}, errors.New("Agent transcript lost the final assistant message")
	}
	transcript[len(transcript)-1] = agentschema.CloneMessage(final)
	if committed.canonicalMessages != nil {
		transcript = committed.canonicalMessages
	}
	_, finishClass := agentmodel.ClassifyResponseFinishReason(final.ResponseMeta)
	incomplete := finishClass.Incomplete()
	var continuation *Continuation
	if !incomplete {
		continuation, err = engine.evaluateGoal(
			ctx, request, input, prepared, capabilities, finalModelRequest, final, emit,
		)
		if err != nil {
			return Result{}, err
		}
	}
	encoded, err := encodeEngineTranscript(prepared, transcript)
	if err != nil {
		return Result{}, fmt.Errorf("encode Agent transcript: %w", err)
	}
	if err := emit(AssistantFinal{
		Content: final.Content, Thinking: final.ReasoningContent, State: encoded,
		Continuation: continuation,
	}); err != nil {
		return Result{}, err
	}
	if incomplete {
		return Result{Status: Incomplete, Reason: finishClass.TerminalReason()}, nil
	}
	return Result{Status: Completed}, nil
}

// controlledLoopResult translates an error from either the loop event lane or
// its public message stream only when a Run control actually caused it.
// Provider and projection errors remain ordinary failures.
func (engine *Engine) controlledLoopResult(
	controls *definitionEngineControls,
	prepared preparedDefinition,
	baseTranscript []*agentschema.Message,
	emit EventSink,
) (Result, error, bool) {
	if controlErr := controls.state.err(); controlErr != nil {
		return Result{}, controlErr, true
	}
	switch controls.state.kind() {
	case ControlPreempt:
		result, err := engine.controlledResult(Preempted, prepared, baseTranscript, emit)
		return result, err, true
	case ControlAbort:
		result, err := engine.controlledResult(Aborted, prepared, baseTranscript, emit)
		return result, err, true
	case ControlSuspend:
		result, err := engine.controlledResult(Suspended, prepared, baseTranscript, emit)
		return result, err, true
	default:
		return Result{}, nil, false
	}
}

// completionFinalAssistant turns the tool-call boundary that requested
// completion into the canonical assistant output. Some provider protocols
// emit the player-visible prose in an earlier assistant message and then send
// a tool-only submission message. Preserve that prose instead of publishing an
// empty final response.
func completionFinalAssistant(transcript []*agentschema.Message, final *agentschema.Message) *agentschema.Message {
	completed := final.Clone()
	completed.ToolCalls = nil
	if strings.TrimSpace(completed.Content) != "" {
		return completed
	}
	for index := len(transcript) - 1; index >= 0; index-- {
		candidate := transcript[index]
		if candidate == nil || candidate.Role != agentschema.Assistant || strings.TrimSpace(candidate.Content) == "" {
			continue
		}
		completed.Content = candidate.Content
		return completed
	}
	return completed
}

func (engine *Engine) evaluateGoal(
	ctx context.Context,
	request Request,
	acceptedInput agentschema.Input,
	prepared preparedDefinition,
	capabilities *capabilityStateClient,
	modelRequest *agentmodel.ModelRequestSnapshot,
	final *agentschema.Message,
	emit EventSink,
) (*Continuation, error) {
	manager := prepared.definition.Goal
	if manager == nil {
		return nil, nil
	}
	state, present, err := capabilities.goal()
	if err != nil {
		return nil, err
	}
	decision, err := manager.AfterRun(ctx, agentgoal.GoalAfterRunRequest{
		Session: agentschema.SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
		Run:     runViewForTurn(request.Snapshot),
		Input:   acceptedInput,
		State:   state, Present: present, Result: agentschema.Result{Status: agentschema.ResultCompleted},
		ModelRequest: modelRequest, Final: agentschema.CloneMessage(final),
	})
	if decision.Usage != nil {
		usage := decision.Usage
		if emitErr := emit(ModelCompleted{
			Usage: ModelUsage{
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
		if emitErr := emit(GoalEvaluationFailed{
			GoalID: state.ID, GoalRevision: state.Revision,
			Code: agentevent.GoalEvaluationFailedCode, Detail: err.Error(),
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
	case agentgoal.GoalVerdictComplete, agentgoal.GoalVerdictBlocked:
		kind := agentschema.GoalComplete
		if decision.Verdict == agentgoal.GoalVerdictBlocked {
			kind = agentschema.GoalBlock
		}
		_, updateErr := capabilities.updateGoal(ctx, manager,
			agentschema.SessionView{Key: engine.key, Revision: uint64(request.Snapshot.ContextCursor)},
			runViewForTurn(request.Snapshot), agentschema.GoalMutation{
				Kind: kind, ExpectedID: state.ID, ExpectedRevision: state.Revision,
				Report:     decision.Reason,
				MutationID: fmt.Sprintf("goal-evaluation-%s-%d-%s", request.Snapshot.OperationID, request.Snapshot.Cycle, decision.Verdict),
			})
		if errors.Is(updateErr, agentexecution.ErrCapabilityStateConflict) {
			slog.InfoContext(ctx, "discarded stale Agent Goal terminal evaluation",
				"session", engine.key, "run_id", request.Snapshot.OperationID, "cycle", request.Snapshot.Cycle,
				"goal_id", state.ID, "goal_revision", state.Revision, "verdict", decision.Verdict)
			return nil, nil
		}
		if updateErr != nil {
			return nil, fmt.Errorf("commit Goal evaluation: %w", updateErr)
		}
		return nil, nil
	case agentgoal.GoalVerdictContinue:
		if strings.TrimSpace(decision.Input.Text) == "" {
			return nil, errors.New("Goal continuation requires a non-empty prompt")
		}
		if fenceErr := capabilities.assertGoalCurrent(); errors.Is(fenceErr, agentexecution.ErrCapabilityStateConflict) {
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
	encoded, runInput, err := EncodeInput(input)
	if err != nil {
		return nil, fmt.Errorf("encode Goal continuation: %w", err)
	}
	runInput.Envelope = encoded
	fingerprint, err := agentschema.HashCanonical(struct {
		OperationID string
		Cycle       int
		GoalID      string
		Revision    uint64
		Input       json.RawMessage
	}{string(request.Snapshot.OperationID), request.Snapshot.Cycle, state.ID, state.Revision, encoded})
	if err != nil {
		return nil, err
	}
	return &Continuation{
		CommandID: CommandID("goal-continuation-" + fingerprint[:32]),
		Input:     runInput, Autonomous: true,
	}, nil
}

func (engine *Engine) controlledResult(
	status Status,
	prepared preparedDefinition,
	messages []*agentschema.Message,
	emit EventSink,
) (Result, error) {
	encoded, err := encodeEngineTranscriptState(prepared, messages, prepared.activeModelUser, prepared.activeUserIndex)
	if err != nil {
		return Result{}, err
	}
	if err := emit(TranscriptUpdated{State: encoded}); err != nil {
		return Result{}, err
	}
	return Result{Status: status}, nil
}

func encodeEngineTranscript(prepared preparedDefinition, messages []*agentschema.Message) (json.RawMessage, error) {
	return encodeEngineTranscriptState(prepared, messages, nil, 0)
}

func encodeActiveEngineTranscript(
	prepared preparedDefinition,
	messages []*agentschema.Message,
	activeModelUser *agentschema.Message,
	activeUserIndex int,
) (json.RawMessage, error) {
	if activeModelUser == nil || activeModelUser.Role != agentschema.User {
		return nil, errors.New("encode active Agent transcript requires a model user projection")
	}
	if activeUserIndex < 0 || activeUserIndex >= prepared.archive.Count(messages) || !prepared.archive.Contains(activeUserIndex) ||
		messages[prepared.archive.Local(activeUserIndex)] == nil || messages[prepared.archive.Local(activeUserIndex)].Role != agentschema.User || agenthistory.IsContextStateMessage(messages[prepared.archive.Local(activeUserIndex)]) {
		return nil, errors.New("encode active Agent transcript requires an exact raw user boundary")
	}
	return encodeEngineTranscriptState(prepared, messages, activeModelUser, activeUserIndex)
}

func encodeEngineTranscriptState(
	prepared preparedDefinition,
	messages []*agentschema.Message,
	activeModelUser *agentschema.Message,
	activeUserIndex int,
) (json.RawMessage, error) {
	encoded, err := json.Marshal(engineTranscript{
		Version: transcriptVersion(prepared.archive), HistoryHead: prepared.historyHead, DefinitionKey: prepared.definitionKey,
		BehaviorKey: prepared.behaviorKey, PrefixFingerprint: prepared.prefixFingerprint,
		MaterializedFingerprint: prepared.materializedFingerprint,
		DefinitionOperationID:   prepared.definitionOperationID,
		DefinitionCommandID:     prepared.definitionCommandID,
		DefinitionCycle:         prepared.definitionCycle, PreparationStage: prepared.preparationStage,
		PreparedContext: snapshotPreparedContext(prepared),
		Archive:         prepared.archive, Messages: agentschema.CloneMessages(messages), ContextState: agenthistory.CloneContextStateSnapshot(prepared.contextState),
		ContextSequence:     prepared.contextSequence,
		LastResponseOrdinal: prepared.lastResponseOrdinal,
		ActiveModelUser:     agentschema.CloneMessage(activeModelUser), ActiveUserIndex: activeUserIndex,
		HostData: agentschema.CloneHostData(prepared.hostData), ClearRevision: prepared.clearRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Agent transcript: %w", err)
	}
	return encoded, nil
}

func (state engineTranscript) ownsDefinition(snapshot TurnSnapshot) bool {
	return state.DefinitionOperationID != "" && state.DefinitionOperationID == string(snapshot.OperationID) &&
		state.DefinitionCommandID == string(snapshot.CommandID) && state.DefinitionCycle == snapshot.Cycle
}

// materializedDefinitionFingerprint freezes every cycle-specific Tool and
// Context value that can affect model-visible behavior. PrefixFingerprint is
// intentionally narrower and only protects the provider cache prefix.
func materializedDefinitionFingerprint(prepared preparedDefinition) (string, error) {
	return agentschema.HashCanonical(struct {
		BehaviorKey        string
		Tools              []agenttool.ToolDefinitionSnapshot
		Context            []agenthistory.ContextFragmentIdentity
		GoalReservedTokens int
	}{
		BehaviorKey: prepared.behaviorKey,
		Tools:       append([]agenttool.ToolDefinitionSnapshot(nil), prepared.toolSnapshots...),
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
	if header.Version != engineTranscriptVersion && header.Version != 2 {
		return engineTranscript{}, &unsupportedEngineTranscriptVersionError{version: header.Version}
	}
	var state engineTranscript
	if err := json.Unmarshal(encoded, &state); err != nil {
		return engineTranscript{}, fmt.Errorf("decode Agent transcript: %w", err)
	}
	if state.ContextSequence < 0 {
		return engineTranscript{}, errors.New("decode Agent transcript: context sequence cannot be negative")
	}
	if state.PreparedContext != nil {
		if state.PreparationStage != enginePreparationMaterialized {
			return engineTranscript{}, errors.New("prepared Agent context has no materialized cycle")
		}
		if err := state.PreparedContext.validate(); err != nil {
			return engineTranscript{}, err
		}
	}
	if err := state.Archive.Validate(state.Messages); err != nil {
		return engineTranscript{}, err
	}
	state.Messages = agentschema.CloneMessages(state.Messages)
	state.ContextState = agenthistory.CloneContextStateSnapshot(state.ContextState)
	state.ActiveModelUser = agentschema.CloneMessage(state.ActiveModelUser)
	state.HostData = agentschema.CloneHostData(state.HostData)
	if state.ActiveModelUser != nil {
		if state.ActiveModelUser.Role != agentschema.User || state.ActiveUserIndex < 0 ||
			state.ActiveUserIndex >= state.Archive.Count(state.Messages) || !state.Archive.Contains(state.ActiveUserIndex) || state.Messages[state.Archive.Local(state.ActiveUserIndex)] == nil ||
			state.Messages[state.Archive.Local(state.ActiveUserIndex)].Role != agentschema.User || agenthistory.IsContextStateMessage(state.Messages[state.Archive.Local(state.ActiveUserIndex)]) {
			return engineTranscript{}, errors.New("Agent transcript has an invalid active model user projection")
		}
	}
	if err := agenthistory.ValidateContextStateSnapshotInArchive(state.ContextState, state.Messages, state.Archive); err != nil {
		return engineTranscript{}, err
	}
	return state, nil
}
