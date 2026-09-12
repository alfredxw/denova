package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

const turnToolRecord = "turn.tool"

// persistedTool confirms individual effects without making a second message
// history. The canonical transcript still owns the complete call/result batch.
type persistedTool struct {
	RunID          string          `json:"run_id"`
	Cycle          int             `json:"cycle"`
	CallID         string          `json:"call_id"`
	ProviderCallID string          `json:"provider_call_id"`
	Name           string          `json:"name"`
	Index          int             `json:"index"`
	Arguments      json.RawMessage `json:"arguments"`
	Descriptor     *ToolDescriptor `json:"descriptor,omitempty"`
	Started        bool            `json:"started"`
	Result         *ToolResult     `json:"result,omitempty"`
	Source         string          `json:"source,omitempty"`
}

func (run *Run) recordToolStart(event runstate.EngineToolStarted) error {
	if event.ExecutionAuthorized {
		if err := run.admitWork(context.Background()); err != nil {
			return err
		}
	}
	if event.CallID == "" {
		return errors.New("tool intent requires an execution ID")
	}
	fact := persistedTool{RunID: run.id, Cycle: run.cycleValue(), CallID: event.CallID, ProviderCallID: event.ProviderCallID,
		Name: event.Name, Index: event.Index, Arguments: append(json.RawMessage(nil), event.Arguments...),
		Descriptor: decodeToolDescriptorMetadata(event.Metadata), Started: event.ExecutionAuthorized}
	if len(fact.Arguments) == 0 {
		fact.Arguments = json.RawMessage(`{}`)
	}
	run.session.mu.Lock()
	defer run.session.mu.Unlock()
	if previous, found := run.tools[fact.CallID]; found && previous.Started {
		return errors.New("tool execution ID was already started")
	}
	if err := run.session.appendRecordLocked(context.Background(), turnToolRecord, fact); err != nil {
		return err
	}
	run.tools[fact.CallID] = fact
	return nil
}

func (run *Run) recordToolResult(event runstate.EngineToolFinished, result *ToolResult) error {
	run.session.mu.Lock()
	defer run.session.mu.Unlock()
	fact, found := run.tools[event.CallID]
	if !found {
		return errors.New("tool result has no durable intent")
	}
	if result == nil {
		value := ToolResult{Status: ToolResultSuccess, ModelContent: event.Result, DisplayContent: event.Result}
		if event.IsError {
			value.Status = ToolResultError
		}
		result = &value
	}
	// Interrupted Ask/permission calls have no completed result yet. Their
	// durable request and response survive the process waiter.
	run.mu.RLock()
	for _, interaction := range run.interactions {
		if interaction.snapshot.ToolCallID == event.CallID && run.suspendReason != "" && result.Status != ToolResultSuccess {
			run.mu.RUnlock()
			return nil
		}
	}
	run.mu.RUnlock()
	fact.Result, fact.Source = result, "tool"
	if err := run.session.appendRecordLocked(context.Background(), turnToolRecord, fact); err != nil {
		return err
	}
	run.tools[event.CallID] = fact
	return nil
}

func (session *Session) replayTool(data json.RawMessage) error {
	var fact persistedTool
	if err := json.Unmarshal(data, &fact); err != nil {
		return err
	}
	run := session.runs[fact.RunID]
	if run == nil || fact.CallID == "" || fact.Name == "" || !json.Valid(fact.Arguments) {
		return errors.New("invalid persisted tool fact")
	}
	run.tools[fact.CallID] = fact
	if fact.Started && fact.Result == nil {
		run.openTools[fact.CallID] = OpenToolSnapshot{CallID: fact.CallID, Name: fact.Name, RunID: run.id, Cycle: fact.Cycle}
	}
	if fact.Result != nil {
		delete(run.openTools, fact.CallID)
	}
	return nil
}

func (run *Run) restoreEffectInteractions() error {
	run.session.mu.Lock()
	defer run.session.mu.Unlock()
	ids := make([]string, 0, len(run.tools))
	for id := range run.tools {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		fact := run.tools[id]
		if !fact.Started || fact.Result != nil || (fact.Descriptor != nil && fact.Descriptor.MutationScope == ToolMutationNone) {
			continue
		}
		interactionID := "verify-" + id
		run.mu.RLock()
		_, pending := run.interactions[interactionID]
		run.mu.RUnlock()
		if pending {
			continue
		}
		request := InteractionRequest{ID: interactionID, Kind: InteractionAsk,
			Questions: []InteractionQuestion{{ID: "effect", Prompt: "Did this tool operation take effect before execution stopped?",
				Options: []InteractionOption{{Value: "executed", Label: "It took effect"}, {Value: "not_executed", Label: "It did not take effect"}, {Value: "unknown", Label: "I cannot determine yet", Recommended: true}}}},
			Verification: &ToolEffectVerification{ExecutionID: id, Tool: fact.Name, Arguments: append(json.RawMessage(nil), fact.Arguments...)},
		}
		encoded, err := json.Marshal(request)
		if err != nil {
			return err
		}
		stored := persistedInteraction{RunID: run.id, Cycle: fact.Cycle, InteractionID: interactionID, ToolCallID: id, Request: encoded}
		if err := run.session.appendRecordLocked(context.Background(), turnInteractionRecord, stored); err != nil {
			return err
		}
		run.mu.Lock()
		run.interactions[interactionID] = stored.pending(request)
		run.mu.Unlock()
	}
	return nil
}

// awaitRecoveryInput keeps the whole execution entrance closed until saved Ask
// requests and unknown effects are resolved. Queue and Steer cannot bypass it.
func (run *Run) awaitRecoveryInput() (runstate.EngineStatus, error) {
	if err := run.restoreEffectInteractions(); err != nil {
		return "", err
	}
	for _, request := range run.pendingInteractionRequests() {
		run.publish(InteractionRequested{Request: request})
	}
	for len(run.pendingInteractionRequests()) > 0 {
		select {
		case control := <-run.controls:
			switch control.Kind {
			case runstate.EngineControlAbort:
				return runstate.EngineAborted, nil
			case runstate.EngineControlSuspend:
				return runstate.EngineSuspended, nil
			case runstate.EngineControlPreempt:
			case runstate.EngineControlInteractionResolved:
			default:
				return "", fmt.Errorf("unsupported recovery control %q", control.Kind)
			}
		case <-run.ctx.Done():
			return "", run.ctx.Err()
		}
	}
	return "", nil
}

// restorePendingToolBatch pairs confirmed individual results without executing
// tools. Unstarted/read-only interrupted calls become explicit skipped results;
// unknown writes must already have passed awaitRecoveryInput.
func (engine *definitionEngine) restorePendingToolBatch(ctx context.Context, request runstate.EngineRequest, prepared *preparedDefinition, state *engineTranscript, emit runstate.EngineEventSink) error {
	run, _ := ctx.Value(canonicalRunKey{}).(*Run)
	if run == nil {
		return nil
	}
	checkpoint, err := canonicalMessageCheckpoint(request.Snapshot.State)
	if err != nil || len(checkpoint.Pending) == 0 {
		return err
	}
	assistant := checkpoint.Pending[0]
	if assistant.Role != Assistant || len(assistant.ToolCalls) == 0 {
		return errors.New("pending tool checkpoint has no assistant calls")
	}
	batch := []*Message{assistant.Clone()}
	if assistant.AgentMeta == nil || assistant.AgentMeta.ModelResponseOrdinal < 1 {
		return errors.New("pending tool checkpoint has no model response identity")
	}
	scope, err := agentsession.CanonicalKey(engine.key)
	if err != nil {
		return err
	}
	namespace := rootToolNamespace(InvocationIdentity{Scope: scope, OperationID: string(request.Snapshot.OperationID), Cycle: request.Snapshot.Cycle}, prepared.definition.Name)
	run.session.mu.RLock()
	for index, call := range assistant.ToolCalls {
		executionID := executionIDForNamespace(namespace, assistant.AgentMeta.ModelResponseOrdinal, index)
		fact, found := run.tools[executionID]
		result := SyntheticToolResult(ToolResultSkipped, ToolSyntheticSteeringBeforeStart, "The tool did not start before execution stopped. No effect was applied.")
		if found && fact.Result != nil {
			result = *fact.Result
		} else if found && fact.Started {
			if fact.Descriptor == nil || fact.Descriptor.MutationScope != ToolMutationNone {
				run.session.mu.RUnlock()
				return errors.New("unverified tool effect cannot enter model execution")
			}
			result = SyntheticToolResult(ToolResultSkipped, ToolSyntheticSteeringInterrupted, "The read or wait stopped without a confirmed result. It may be requested again if still needed.")
		}
		batch = append(batch, ToolMessage(result, call.ID, WithToolName(call.Function.Name)))
	}
	run.session.mu.RUnlock()
	state.Messages = append(cloneMessages(state.Messages[:checkpoint.MessageCount]), batch...)
	sequence := prepared.contextSequence
	prepared.contextSequence++
	encoded, err := encodeEngineTranscriptState(*prepared, state.Messages, state.ActiveModelUser, state.ActiveUserIndex)
	if err != nil {
		return err
	}
	if err := engine.commitCanonicalContext(ctx, request, prepared.definition.Canonical, sequence, batch, encoded); err != nil {
		return err
	}
	return emit(runstate.EngineTranscriptUpdated{State: encoded})
}
