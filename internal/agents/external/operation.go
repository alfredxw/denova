package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	externaljournal "denova/internal/agents/external/journal"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/i18n"
	agent "github.com/alfredxw/denova/agent"
)

// Operation is one accepted command. Wait executes at most once and must run
// under the product task context, never an HTTP/SSE connection context.
type Operation struct {
	service         *Service
	request         StartRequest
	id, incarnation string
	receipt         agentrun.CommandReceipt
	replayed        bool
	definitions     map[string]preparedTool
	once            sync.Once
	outcome         agentrun.Outcome
	mu              sync.Mutex
	calls           map[string]*toolAttempt
	closed          bool
	workers         sync.WaitGroup
	runContext      context.Context
	mutations       []agenttool.Mutation
	usage           *agent.TokenUsage
	output          strings.Builder
}

func (operation *Operation) Receipt() agentrun.CommandReceipt { return operation.receipt }
func (operation *Operation) Replayed() bool                   { return operation.replayed }

func (operation *Operation) Wait(ctx context.Context) agentrun.Outcome {
	operation.once.Do(func() {
		if operation.replayed {
			operation.outcome = operation.savedOutcome(ctx)
			return
		}
		defer func() {
			operation.service.mu.Lock()
			delete(operation.service.active, operation.key())
			operation.service.mu.Unlock()
		}()
		var result Result
		var runErr error
		runCtx, cancel := context.WithCancel(ctx)
		operation.mu.Lock()
		operation.runContext = runCtx
		operation.mu.Unlock()
		func() {
			defer func() {
				if value := recover(); value != nil {
					runErr = fmt.Errorf("external adapter panic: %v", value)
					slog.ErrorContext(ctx, "External adapter panicked", "operation_id", operation.id, "error", runErr)
				}
			}()
			// Publish acceptance before provider setup or context maintenance. A
			// reconnect must become a live stream even while the model is silent.
			operation.send(agentrun.Event{Type: "agent_cycle_started", Data: map[string]any{
				"id": operation.id + "-output", "command_id": operation.request.CommandID,
				"run_started_at": time.Now().UTC().Format(time.RFC3339Nano),
			}})
			if effect := operation.request.InputCommitEffect; effect != nil {
				runErr = effect.Apply(runCtx, agentrun.InputCommitEffectRequest{CommandID: operation.request.CommandID,
					OperationID: operation.id, Cycle: 1, Hash: operation.request.Fingerprint})
				if runErr != nil {
					return
				}
			}
			input, err := operation.prepareHistory(runCtx)
			if err != nil {
				runErr = err
				return
			}
			result, runErr = operation.request.Adapter.Run(runCtx, input, operation)
			operation.addUsage(result.Usage)
		}()
		operation.mu.Lock()
		operation.closed = true
		// Adapters may return no Result on cancellation, disconnect or panic.
		// Preserve the public text already delivered to the user in every case.
		if (runErr != nil || ctx.Err() != nil) && operation.output.Len() > 0 {
			result.Text = operation.output.String()
		}
		operation.mu.Unlock()
		cancel()
		operation.workers.Wait()
		status := externaljournal.Completed
		if runErr != nil {
			status = externaljournal.Failed
		}
		if ctx.Err() != nil {
			status = externaljournal.Cancelled
			runErr = ctx.Err()
		}
		settled, err := operation.close(context.WithoutCancel(ctx), status, result)
		if err != nil {
			runErr = errors.Join(runErr, err)
			settled = externaljournal.Interrupted
			slog.ErrorContext(ctx, "External operation settlement failed", "operation_id", operation.id, "error", err)
		}
		operation.outcome = projectOutcome(settled, result.Text, runErr)
		if callback := operation.request.OnMutationsVerified; callback != nil && len(operation.mutations) > 0 {
			mutations := append([]agenttool.Mutation(nil), operation.mutations...)
			callback(context.WithoutCancel(ctx), mutations, agenttool.VerifyPostRunMutations(operation.request.BookService, mutations))
		}
		switch operation.outcome.Status {
		case agentrun.OutcomeCompleted:
			operation.send(agentrun.Event{Type: "done", Data: map[string]any{"content": result.Text}})
		case agentrun.OutcomeAborted:
			operation.send(agentrun.NewAbortedEvent(agentrun.AbortReasonUserRequested))
		case agentrun.OutcomeFailed:
			operation.send(agentrun.Event{Type: "error", Data: map[string]any{"error_key": "agentRuntime.operationFailed", "message": i18n.New(operation.request.Locale).T("agentRuntime.operationFailed")}})
		case agentrun.OutcomeSuspended:
			operation.send(agentrun.Event{Type: "error", Data: map[string]any{"error_key": "agentRuntime.interrupted", "message": i18n.New(operation.request.Locale).T("agentRuntime.interrupted")}})
		}
	})
	return operation.outcome
}

func (operation *Operation) Emit(event agentrun.Event) error {
	operation.mu.Lock()
	defer operation.mu.Unlock()
	if operation.closed {
		return errors.New("external operation is closed")
	}
	// The adapter's terminal notifications never acknowledge a product commit.
	if event.Type != "chunk" && event.Type != "thinking" {
		return fmt.Errorf("unsupported external adapter event %q", event.Type)
	}
	if event.Type == "chunk" {
		operation.output.WriteString(event.DataString("content"))
	}
	operation.send(event)
	return nil
}

func (operation *Operation) send(event agentrun.Event) {
	if operation.request.Emit != nil {
		// Keep display and canonical output scoped to the accepted operation.
		// Copy the payload because Task retains it for concurrent subscribers.
		if source, ok := event.Data.(map[string]any); ok {
			data := make(map[string]any, len(source)+3)
			for key, value := range source {
				data[key] = value
			}
			data["run_id"], data["operation_id"], data["cycle"] = operation.id, operation.id, 1
			event.Data = data
		}
		operation.request.Emit(event)
	}
}

func (operation *Operation) close(ctx context.Context, target externaljournal.Status, result Result) (externaljournal.Status, error) {
	status := target
	var wake []string
	var usageEvent *session.DisplayEvent
	err := operation.request.Session.UpdateExternal(ctx, operation.request.Revision, func(state session.ExternalState) (session.ExternalTransaction, error) {
		wake = nil
		current, err := operation.owned(state)
		if err != nil {
			return session.ExternalTransaction{}, err
		}
		if current.Status != externaljournal.Running {
			return session.ExternalTransaction{}, errors.New("external operation was already settled")
		}
		status = target
		var records []externaljournal.Record
		for executionID, tool := range current.Tools {
			if tool.Finished != nil {
				continue
			}
			if tool.Name == "ask" && target == externaljournal.Cancelled {
				source, err := state.Read(tool.Started)
				if err != nil {
					return session.ExternalTransaction{}, err
				}
				var started externaljournal.StartedTool
				if err := json.Unmarshal(source.Data, &started); err != nil {
					return session.ExternalTransaction{}, err
				}
				request, err := QuestionRequest(executionID, started.Arguments)
				if err != nil {
					return session.ExternalTransaction{}, err
				}
				reason := agentrun.AbortReasonUserRequested
				answer, err := resolveQuestion(ctx, request, nil, &reason)
				if err != nil {
					return session.ExternalTransaction{}, err
				}
				body, err := json.Marshal(answer)
				if err != nil {
					return session.ExternalTransaction{}, err
				}
				record, err := externaljournal.NewRecord(externaljournal.ToolFinished, operation.id, operation.request.Revision, externaljournal.FinishedTool{ExecutionID: executionID, Success: true, Result: string(body)})
				if err != nil {
					return session.ExternalTransaction{}, err
				}
				records = append(records, record)
				wake = append(wake, askWaitKey(operation.request.ProjectID, operation.incarnation, operation.id, executionID))
			} else {
				status = externaljournal.Interrupted
			}
		}
		messageID := ""
		transaction := session.ExternalTransaction{}
		if status == externaljournal.Completed || result.Text != "" {
			messageID = operation.id + "-output"
			transaction.Message = &agent.Message{Role: agent.Assistant, Content: result.Text}
			transaction.Metadata = session.MessageMetadata{MessageID: messageID, RunID: operation.id,
				AgentOperationID: operation.id, AgentCommandID: operation.request.CommandID, AgentCycle: 1, AgentKind: state.Config.AgentKind}
		}
		code := ""
		if status == externaljournal.Failed {
			code = "agentRuntime.operationFailed"
		}
		if status == externaljournal.Interrupted {
			code = "agentRuntime.interrupted"
		}
		record, err := externaljournal.NewRecord(externaljournal.OperationClosed, operation.id, operation.request.Revision, externaljournal.Closed{Status: status, MessageID: messageID, ErrorCode: code, AgentKind: state.Config.AgentKind, Usage: operation.usage})
		if err == nil {
			usageEvent, err = session.ExternalUsageDisplay(record)
		}
		transaction.Records = append(records, record)
		return transaction, err
	})
	if err == nil {
		if usageEvent != nil {
			body, _ := json.Marshal(usageEvent)
			var data map[string]any
			_ = json.Unmarshal(body, &data)
			operation.send(agentrun.Event{Type: "token_usage", Data: data})
		}
		for _, key := range wake {
			operation.service.Interactions.notify(key)
		}
	}
	return status, err
}

func (operation *Operation) addUsage(usage *agent.TokenUsage) {
	if usage == nil {
		return
	}
	if operation.usage == nil {
		operation.usage = &agent.TokenUsage{}
	}
	operation.usage.PromptTokens += usage.PromptTokens
	operation.usage.PromptTokenDetails.CachedTokens += usage.PromptTokenDetails.CachedTokens
	operation.usage.CompletionTokens += usage.CompletionTokens
	operation.usage.CompletionTokensDetails.ReasoningTokens += usage.CompletionTokensDetails.ReasoningTokens
	operation.usage.TotalTokens += usage.TotalTokens
}

func (operation *Operation) owned(state session.ExternalState) (*externaljournal.Operation, error) {
	current := state.Projection.Operations[operation.id]
	if state.Incarnation != operation.incarnation || current == nil || current.ConfigRevision != operation.request.Revision {
		return nil, errors.New("external operation source is no longer current")
	}
	return current, nil
}

func (operation *Operation) savedOutcome(ctx context.Context) agentrun.Outcome {
	status := externaljournal.Interrupted
	err := operation.request.Session.ReadExternal(ctx, func(state session.ExternalState) error {
		current, err := operation.owned(state)
		if err == nil {
			status = current.Status
		}
		return err
	})
	return projectOutcome(status, "", err)
}

func projectOutcome(status externaljournal.Status, content string, err error) agentrun.Outcome {
	switch status {
	case externaljournal.Completed:
		return agentrun.NewOutcome(agentrun.OutcomeCompleted, err, "", content, "")
	case externaljournal.Cancelled:
		return agentrun.NewOutcome(agentrun.OutcomeAborted, err, agentrun.AbortReasonUserRequested, "", "")
	case externaljournal.Failed:
		return agentrun.NewOutcome(agentrun.OutcomeFailed, err, "agentRuntime.operationFailed", "", "")
	case externaljournal.Running, externaljournal.Interrupted:
		return agentrun.NewOutcome(agentrun.OutcomeSuspended, err, "agentRuntime.interrupted", "", "")
	}
	return agentrun.NewOutcome(agentrun.OutcomeFailed, fmt.Errorf("invalid external outcome %q", status), "agentRuntime.operationFailed", "", "")
}
