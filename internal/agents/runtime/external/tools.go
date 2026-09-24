package external

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/session"
	"denova/internal/agents/toolruntime"
	"denova/internal/i18n"
	agent "github.com/alfredxw/denova/agent"
	publicresult "github.com/alfredxw/denova/agent/toolresult"
)

type preparedTool struct {
	definition agent.ToolDefinition
	snapshot   agent.ToolDefinitionSnapshot
}

type toolAttempt struct {
	call   ToolCall
	done   chan struct{}
	result ToolResult
	err    error
}

func prepareTools(ctx context.Context, definitions []agent.ToolDefinition) (map[string]preparedTool, []Tool, error) {
	prepared := make(map[string]preparedTool, len(definitions))
	wire := make([]Tool, 0, len(definitions))
	for _, definition := range definitions {
		if err := definition.Validate(ctx); err != nil {
			return nil, nil, err
		}
		info, err := definition.Tool.Info(ctx)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := prepared[info.Name]; exists {
			return nil, nil, fmt.Errorf("duplicate external host tool %q", info.Name)
		}
		schema, err := info.ToJSONSchema()
		if err != nil {
			return nil, nil, err
		}
		body, err := json.Marshal(schema)
		if err != nil {
			return nil, nil, err
		}
		prepared[info.Name] = preparedTool{definition: definition, snapshot: agent.ToolDefinitionSnapshot{Info: info, Descriptor: definition.Descriptor}}
		wire = append(wire, Tool{Name: info.Name, Description: info.Desc, Schema: body})
	}
	if _, exists := prepared["ask"]; !exists {
		return nil, nil, errors.New("external product contract requires the ask tool")
	}
	return prepared, wire, nil
}

func (operation *Operation) CallTool(ctx context.Context, call ToolCall) (ToolResult, error) {
	if call.ID == "" || len(call.ID) > 4096 || len(call.Arguments) > 4<<20 || !json.Valid(call.Arguments) {
		return ToolResult{Text: "Invalid host tool call.", Success: false}, nil
	}
	definition, found := operation.definitions[call.Name]
	if !found {
		return ToolResult{Text: "Unknown host tool.", Success: false}, nil
	}
	operation.mu.Lock()
	if operation.closed {
		operation.mu.Unlock()
		return ToolResult{}, errors.New("external operation is closed")
	}
	if prior := operation.calls[call.ID]; prior != nil {
		operation.mu.Unlock()
		if prior.call.Name != call.Name || !bytes.Equal(prior.call.Arguments, call.Arguments) {
			return ToolResult{}, errors.New("provider call ID was reused with different arguments")
		}
		select {
		case <-ctx.Done():
			return ToolResult{}, ctx.Err()
		case <-prior.done:
			return prior.result, prior.err
		}
	}
	if operation.calls == nil {
		operation.calls = map[string]*toolAttempt{}
	}
	attempt := &toolAttempt{call: call, done: make(chan struct{})}
	operation.calls[call.ID] = attempt
	operation.workers.Add(1)
	runContext := operation.runContext
	operation.mu.Unlock()
	defer operation.workers.Done()
	if runContext == nil {
		attempt.err = errors.New("external operation has not started")
		close(attempt.done)
		return ToolResult{}, attempt.err
	}
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(runContext, cancel)
	defer stop()
	defer cancel()
	func() {
		defer close(attempt.done)
		defer func() {
			if value := recover(); value != nil {
				attempt.err = fmt.Errorf("host tool panic: %v", value)
			}
		}()
		attempt.result, attempt.err = operation.invoke(callCtx, call, definition)
	}()
	return attempt.result, attempt.err
}

func (operation *Operation) invoke(ctx context.Context, call ToolCall, tool preparedTool) (ToolResult, error) {
	digest := sha256.Sum256([]byte(call.ID))
	executionID := operation.id + "-" + hex.EncodeToString(digest[:])
	var question agent.InteractionRequest
	if call.Name == "ask" {
		var err error
		question, err = QuestionRequest(executionID, call.Arguments)
		if err != nil {
			return ToolResult{Text: "Invalid question: " + err.Error()}, nil
		}
	}
	recovery := externaljournal.NonReplayable
	switch tool.definition.Descriptor.Recovery {
	case agent.ToolRecoveryReadOnly:
		recovery = externaljournal.ReadOnly
	case agent.ToolRecoveryReconcilable:
		recovery = externaljournal.ReceiptVerifiable
	case agent.ToolRecoveryIdempotent, agent.ToolRecoveryNonIdempotent:
		// Idempotency without a durable domain receipt is not crash recovery.
	}
	if err := operation.transition(ctx, externaljournal.ToolStarted, externaljournal.StartedTool{ExecutionID: executionID, AgentKind: operation.request.ToolPolicy.AgentKind, Tool: call.Name, Arguments: call.Arguments, Recovery: recovery}); err != nil {
		return ToolResult{}, err
	}
	if call.Name == "ask" {
		pending := agentchat.ProjectPendingInteraction(question, agentrun.RuntimeStatus{ActiveCommandID: agentrun.CommandID(operation.request.CommandID), ActiveOperation: agentrun.OperationID(operation.id)})
		pending.ToolCallID = executionID
		pending.AgentKind = operation.request.ToolPolicy.AgentKind
		operation.send(agentrun.Event{Type: "ask_pending", Data: pending})
		answer, err := operation.service.Interactions.Wait(ctx, operation.request.ProjectID, operation.request.Session, operation.id, executionID)
		if err != nil {
			return ToolResult{}, err
		}
		body, err := json.Marshal(answer)
		if err != nil {
			return ToolResult{}, err
		}
		operation.send(agentrun.Event{Type: "ask_resolved", Data: answer})
		return ToolResult{Text: string(body), Success: true}, nil
	}
	operation.send(agentrun.Event{Type: "tool_call", Data: map[string]any{"id": executionID, "name": call.Name, "args": string(call.Arguments), "run_id": operation.id, "tool_presentation": tool.definition.Descriptor.Presentation}})
	ctx = agent.ContextWithToolArtifactBackend(ctx, operation.request.Session.ToolArtifactStore())
	// Review groups must refer to the same run as the visible and persisted
	// output. The controller's logical operation can span several such runs.
	identity := toolruntime.HostToolIdentity{OperationID: operation.id, ExecutionID: executionID, ProviderCallID: call.ID, SessionID: operation.request.Session.ID, ReviewThreadID: operation.request.ReviewThreadID}
	var result agent.ToolResult
	var callErr error
	var invocation toolruntime.HostToolInvocation
	if reason := operation.hostPermissionError(call, tool); reason != "" {
		result = agent.ToolErrorResult(reason, i18n.New(operation.request.Locale).T("agentRuntime.toolPermissionDenied", "tool", call.Name))
	} else {
		outcome, err := toolruntime.InvokeHostTool(ctx, operation.request.ToolPolicy, identity, tool.definition, string(call.Arguments))
		result, invocation, callErr = outcome.Result, outcome.Invocation, err
	}
	if callErr != nil {
		if invocation == toolruntime.HostToolNotInvoked {
			result = agent.ToolErrorResult("Tool execution did not start. No tool side effects occurred.", i18n.New(operation.request.Locale).T("agentRuntime.toolNotExecuted"))
		} else if tool.definition.Descriptor.MutationScope != agent.ToolMutationNone && len(result.Details) == 0 && len(result.Effects) == 0 {
			// No receipt can establish whether this call changed the domain. Keep
			// its start unresolved and interrupt the operation; never replay it.
			return ToolResult{}, callErr
		}
		if result.ModelContent == "" {
			result = agent.ToolErrorResult("Tool execution was interrupted.", "Tool execution was interrupted.")
		}
	}
	settleCtx := context.WithoutCancel(ctx)
	processor := publicresult.Standard(publicresult.Policy{MaxBytes: operation.request.ToolPolicy.ToolResultMaxBytes})
	processed, processErr := processor.Process(settleCtx, agent.ToolResultProcessRequest{ToolName: call.Name, Arguments: string(call.Arguments), ExecutionID: executionID, ProviderCallID: call.ID, Definition: tool.snapshot, Result: result})
	if processErr != nil {
		return ToolResult{}, processErr
	}
	processed, err := agent.NormalizeToolResult(processed, tool.definition.Descriptor)
	if err != nil {
		return ToolResult{}, err
	}
	// Domain receipts and portable artifact references survive independently of
	// the bounded model text. No provider continuation or private reasoning is stored.
	receipt := &externaljournal.ToolReceipt{
		Details: processed.Details, Effects: processed.Effects,
		Artifacts: processed.Artifacts, Attachments: processed.Attachments,
	}
	finished := externaljournal.FinishedTool{ExecutionID: executionID, Success: !processed.IsError(), Result: processed.ModelContent, Receipt: receipt}
	if err := operation.transition(settleCtx, externaljournal.ToolFinished, finished); err != nil {
		return ToolResult{}, err
	}
	for _, effect := range processed.Effects {
		if effect.Kind != toolruntime.AgentToolMutationEffectKind {
			continue
		}
		mutation, err := toolruntime.DecodeAgentToolMutationEffect(effect)
		if err != nil {
			return ToolResult{}, err
		}
		operation.mu.Lock()
		operation.mutations = append(operation.mutations, mutation)
		operation.mu.Unlock()
	}
	data := map[string]any{"id": executionID, "name": call.Name, "content": processed.DisplayContent, "status": string(processed.Status), "run_id": operation.id, "tool_presentation": tool.definition.Descriptor.Presentation}
	domainPayload := processed.ModelContent
	if len(processed.Details) != 0 && json.Valid(processed.Details) {
		domainPayload = string(processed.Details)
	}
	for _, warning := range agentchat.ProjectToolResult(operation.request.ProjectID, call.Name, domainPayload, data, operation.send) {
		slog.WarnContext(ctx, "[external-runtime] project ToolResult display data failed", "tool", call.Name, "error", warning)
	}
	operation.send(agentrun.Event{Type: "tool_result", Data: data})
	images, err := projectToolImages(settleCtx, operation.request.Session, processed.Attachments)
	if err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Text: finished.Result, Success: finished.Success, Images: images}, callErr
}

func (operation *Operation) transition(ctx context.Context, kind externaljournal.Kind, data any) error {
	record, err := externaljournal.NewRecord(kind, operation.id, operation.request.Revision, data)
	if err != nil {
		return err
	}
	return operation.request.Session.UpdateExternal(ctx, operation.request.Revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
		if _, err := operation.owned(state); err != nil {
			return session.ExternalTransaction{}, err
		}
		return session.ExternalTransaction{Records: []externaljournal.Record{record}}, nil
	})
}
