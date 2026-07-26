package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"
)

const toolProgressTruncatedMarker = "\n[tool progress truncated]"

type toolExecutionResult struct {
	message        *Message
	result         ToolResult
	executionID    string
	providerCallID string
	err            error
}

type preparedToolCall struct {
	index       int
	call        ToolCall
	executionID string
	definition  ToolDefinition
	snapshot    ToolDefinitionSnapshot
	precomputed *toolExecutionResult
}

type indexedToolExecutionResult struct {
	index  int
	result toolExecutionResult
}

const fallbackToolResultMaxBytes = 128 * 1024

var fallbackToolResultDescriptor = ToolDescriptor{
	Source: ToolSourceOther, Execution: ToolExecutionParallelRead,
	MutationScope: ToolMutationNone, PostCheck: ToolPostCheckNone,
	Recovery: ToolRecoveryReadOnly, ResultProjection: ToolResultBoundedModelContext,
	Steering: SteeringFinishCurrent, MaxResultBytes: fallbackToolResultMaxBytes,
}

// executeToolBatch schedules calls in descriptor-defined stages. Parallel reads
// share one bounded stage; exclusive and child calls are source-order barriers.
func (agent *Agent) executeToolBatch(
	ctx context.Context,
	registry *Registry,
	calls []ToolCall,
	modelResponseOrdinal int,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
) ([]toolExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	prepared := agent.prepareToolCalls(ctx, registry, calls, modelResponseOrdinal)
	results := make([]toolExecutionResult, len(calls))

	for index := 0; index < len(prepared); {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		if cancel.pending(CancelAfterToolCalls | CancelAfterChatModel) {
			agent.fillSteeringSkipped(prepared[index:], results, events)
			return results, nil
		}
		current := prepared[index]
		if current.precomputed != nil {
			results[index] = *current.precomputed
			agent.emitToolFinished(events, current, current.precomputed.result)
			index++
			continue
		}

		if current.snapshot.Descriptor.Execution != ToolExecutionParallelRead {
			completion, err := agent.runOneToolCall(ctx, current, events, cancel)
			results[index] = completion
			if err != nil {
				agent.fillPolicySkipped(prepared[index+1:], results, events, "a durability or control failure stopped later tool stages")
				return results, err
			}
			index++
			continue
		}

		end := index
		for end < len(prepared) && prepared[end].precomputed == nil &&
			prepared[end].snapshot.Descriptor.Execution == ToolExecutionParallelRead {
			end++
		}
		stageResults, started, err := agent.runParallelToolStage(ctx, prepared[index:end], events, cancel)
		for offset, completion := range stageResults {
			results[index+offset] = completion
		}
		if err != nil {
			if started < end-index {
				agent.fillPolicySkipped(prepared[index+started:end], results, events, "a durability or control failure stopped the parallel stage")
			}
			agent.fillPolicySkipped(prepared[end:], results, events, "a durability or control failure stopped later tool stages")
			return results, err
		}
		if started < end-index {
			// Steering stops the whole remaining batch, not just the tail of the
			// current parallel stage. Every assistant call must still receive a
			// paired result before the safe point ends this run.
			agent.fillSteeringSkipped(prepared[index+started:], results, events)
			return results, nil
		}
		index = end
	}
	return results, nil
}

func (agent *Agent) prepareToolCalls(ctx context.Context, registry *Registry, calls []ToolCall, modelResponseOrdinal int) []preparedToolCall {
	prepared := make([]preparedToolCall, len(calls))
	for index, call := range calls {
		call = cloneToolCalls([]ToolCall{call})[0]
		item := preparedToolCall{
			index: index, call: call,
			executionID: ToolExecutionIDForOrdinal(ctx, modelResponseOrdinal, index),
		}
		name := call.Function.Name
		switch {
		case call.Type != "" && call.Type != "function":
			result := syntheticCallResult(call, ToolResultError, ToolSyntheticInvalidCall,
				fmt.Sprintf("tool %q has unsupported call type %q", name, call.Type))
			item.precomputed = &result
		case strings.TrimSpace(name) == "":
			result := syntheticCallResult(call, ToolResultError, ToolSyntheticInvalidCall, "tool call has no name")
			item.precomputed = &result
		default:
			definition, exists := registry.Lookup(name)
			if !exists {
				result := syntheticCallResult(call, ToolResultError, ToolSyntheticUnknownTool,
					fmt.Sprintf("unknown tool %q", name))
				item.precomputed = &result
				break
			}
			snapshot, _ := registry.Snapshot(name)
			item.definition = definition
			item.snapshot = snapshot
			if err := ValidateToolArguments(snapshot.Info, call.Function.Arguments); err != nil {
				result := syntheticCallResult(call, ToolResultError, ToolSyntheticInvalidArguments,
					fmt.Sprintf("invalid arguments for tool %q: %v", name, err))
				item.precomputed = &result
			}
		}
		if item.precomputed != nil {
			bindToolExecutionIdentity(item.precomputed, item)
			descriptor := fallbackToolResultDescriptor
			if item.snapshot.Info != nil {
				descriptor = item.snapshot.Descriptor
			}
			normalizeToolExecutionResult(item.precomputed, descriptor)
		}
		prepared[index] = item
	}
	return prepared
}

func (agent *Agent) runOneToolCall(
	ctx context.Context,
	call preparedToolCall,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
) (toolExecutionResult, error) {
	completed := make(chan toolExecutionResult, 1)
	agent.launchToolCall(ctx, call, events, cancel, completed)
	select {
	case result := <-completed:
		return result, result.err
	case <-ctx.Done():
		return toolExecutionResult{}, ctx.Err()
	}
}

func (agent *Agent) runParallelToolStage(
	ctx context.Context,
	calls []preparedToolCall,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
) ([]toolExecutionResult, int, error) {
	results := make([]toolExecutionResult, len(calls))
	completed := make(chan indexedToolExecutionResult, len(calls))
	started := 0
	running := 0
	var terminalErr error

	launch := func(index int) {
		call := calls[index]
		output := make(chan toolExecutionResult, 1)
		agent.launchToolCall(ctx, call, events, cancel, output)
		safeGo(func() {
			result := <-output
			completed <- indexedToolExecutionResult{index: index, result: result}
		}, func(err error) {
			result := toolFailureResult(call.call, err)
			bindToolExecutionIdentity(&result, call)
			completed <- indexedToolExecutionResult{index: index, result: result}
		})
		started++
		running++
	}

	for running < agent.toolParallelism && started < len(calls) && !cancel.pending(CancelAfterToolCalls|CancelAfterChatModel) {
		launch(started)
	}
	for running > 0 {
		select {
		case completion := <-completed:
			running--
			results[completion.index] = completion.result
			if terminalErr == nil && completion.result.err != nil {
				terminalErr = completion.result.err
			}
			for terminalErr == nil && running < agent.toolParallelism && started < len(calls) &&
				!cancel.pending(CancelAfterToolCalls|CancelAfterChatModel) {
				launch(started)
			}
		case <-ctx.Done():
			return results, started, ctx.Err()
		}
	}
	return results, started, terminalErr
}

func (agent *Agent) launchToolCall(
	ctx context.Context,
	call preparedToolCall,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
	completed chan<- toolExecutionResult,
) {
	safeGo(func() {
		completed <- agent.executePreparedTool(ctx, call, events, cancel)
	}, func(err error) {
		result := toolFailureResultForDescriptor(call.call, call.snapshot.Descriptor, err)
		bindToolExecutionIdentity(&result, call)
		agent.emitToolFinished(events, call, result.result)
		completed <- result
	})
}

func (agent *Agent) executePreparedTool(
	ctx context.Context,
	prepared preparedToolCall,
	events *AsyncGenerator[*AgentEvent],
	cancel *cancelControl,
) toolExecutionResult {
	callCtx := contextWithToolExecution(ctx, prepared.executionID, prepared.call.ID, prepared.call.Function.Name)
	callCtx = ContextWithEventSink(callCtx, func(event *AgentEvent) {
		if event != nil {
			events.Send(event)
		}
	})
	callCtx = contextWithToolSteering(callCtx, toolSteeringSignal{
		done:    cancel.requestedSignal(),
		pending: func() bool { return cancel.pending(CancelAfterToolCalls | CancelAfterChatModel) },
	})

	var progressMu sync.Mutex
	var progress strings.Builder
	progressTruncated := false
	limit := prepared.snapshot.Descriptor.MaxResultBytes
	callCtx = contextWithToolProgress(callCtx, func(delta string) {
		delta = strings.ToValidUTF8(delta, "\uFFFD")
		progressMu.Lock()
		if progressTruncated {
			progressMu.Unlock()
			return
		}
		remaining := max(0, limit-progress.Len())
		emitted := delta
		if len(emitted) > remaining {
			emitted = emitted[:remaining]
			for len(emitted) > 0 && !utf8.ValidString(emitted) {
				emitted = emitted[:len(emitted)-1]
			}
			emitted += toolProgressTruncatedMarker
			progressTruncated = true
		}
		content := emitted
		if progressTruncated {
			content = strings.TrimSuffix(emitted, toolProgressTruncatedMarker)
		}
		progress.WriteString(content)
		progressMu.Unlock()
		if emitted != "" {
			events.Send(agent.toolExecutionEvent(prepared, ToolExecutionProgress, emitted, nil))
		}
	})

	if prepared.snapshot.Descriptor.Steering == SteeringInterruptibleWait {
		interruptibleCtx, stop := context.WithCancel(callCtx)
		callCtx = interruptibleCtx
		defer stop()
		if signal := cancel.requestedSignal(); signal != nil {
			safeGo(func() {
				select {
				case <-signal:
					if cancel.pending(CancelAfterToolCalls | CancelAfterChatModel) {
						stop()
					}
				case <-interruptibleCtx.Done():
				}
			}, func(error) {})
		}
	}

	events.Send(agent.toolExecutionEvent(prepared, ToolExecutionStarted, "", nil))
	endpoint := ToolCallEndpoint(func(runCtx context.Context, arguments string, options ...ToolOption) (ToolResult, error) {
		if err := runCtx.Err(); err != nil {
			return ToolResult{}, err
		}
		if err := ValidateToolArguments(prepared.snapshot.Info, arguments); err != nil {
			return ToolResult{}, fmt.Errorf("validate final arguments for tool %q: %w", prepared.call.Function.Name, err)
		}
		result, err := runToolSafely(prepared.definition.Tool, runCtx, arguments, options...)
		if err == nil && result.ModelContent == "" && result.DisplayContent == "" {
			progressMu.Lock()
			progressContent := progress.String()
			if progressTruncated {
				marker := toolProgressTruncatedMarker
				if len(marker) >= limit {
					progressContent = marker[:limit]
				} else {
					end := limit - len(marker)
					if len(progressContent) > end {
						progressContent = progressContent[:end]
						for len(progressContent) > 0 && !utf8.ValidString(progressContent) {
							progressContent = progressContent[:len(progressContent)-1]
						}
					}
					progressContent += marker
				}
			}
			progressMu.Unlock()
			if progressContent != "" {
				result.ModelContent = progressContent
				result.DisplayContent = progressContent
			}
		}
		return result, err
	})
	toolContext := &ToolContext{
		Index: prepared.index, Name: prepared.call.Function.Name,
		ExecutionID: prepared.executionID, ProviderCallID: prepared.call.ID,
		Definition: prepared.snapshot,
	}
	for index := len(agent.middlewares) - 1; index >= 0; index-- {
		wrapped, err := agent.middlewares[index].WrapToolCall(callCtx, endpoint, toolContext)
		if err != nil {
			result := toolFailureResult(prepared.call, fmt.Errorf("wrap tool %q: %w", prepared.call.Function.Name, err))
			bindToolExecutionIdentity(&result, prepared)
			agent.emitToolFinished(events, prepared, result.result)
			return result
		}
		if wrapped == nil {
			result := toolFailureResult(prepared.call, fmt.Errorf("wrap tool %q returned nil endpoint", prepared.call.Function.Name))
			bindToolExecutionIdentity(&result, prepared)
			agent.emitToolFinished(events, prepared, result.result)
			return result
		}
		endpoint = wrapped
	}

	result, err := endpoint(callCtx, prepared.call.Function.Arguments)
	var terminalErr error
	if err != nil {
		if prepared.snapshot.Descriptor.Steering == SteeringInterruptibleWait &&
			cancel.pending(CancelAfterToolCalls|CancelAfterChatModel) && errors.Is(callCtx.Err(), context.Canceled) {
			result = SyntheticToolResult(ToolResultSkipped, ToolSyntheticSteeringInterrupted,
				fmt.Sprintf("tool %q was interrupted to apply pending user steering", prepared.call.Function.Name))
			err = nil
		} else if IsToolControlError(err) {
			terminalErr = err
			if result.Status == "" {
				result = ToolErrorResult(toolErrorContent(prepared.call, err), err.Error())
			}
			err = nil
		} else if IsInterruptError(err) {
			terminalErr = err
			if result.Status == "" {
				result = ToolErrorResult(toolErrorContent(prepared.call, err), err.Error())
			}
			err = nil
		} else if ctx.Err() != nil {
			return toolExecutionResult{executionID: prepared.executionID, providerCallID: prepared.call.ID, err: err}
		} else {
			result = ToolErrorResult(toolErrorContent(prepared.call, err), err.Error())
			err = nil
		}
	}
	if result.Status == "" {
		result.Status = ToolResultSuccess
	}
	normalized, normalizeErr := NormalizeToolResult(result, prepared.snapshot.Descriptor)
	if normalizeErr != nil {
		normalized, _ = NormalizeToolResult(
			ToolErrorResult(toolErrorContent(prepared.call, normalizeErr), normalizeErr.Error()),
			prepared.snapshot.Descriptor,
		)
	}
	completion := toolExecutionResult{
		result: normalized, message: ToolMessage(normalized, prepared.call.ID, WithToolName(prepared.call.Function.Name)),
		executionID: prepared.executionID, providerCallID: prepared.call.ID, err: terminalErr,
	}
	agent.emitToolFinished(events, prepared, normalized)
	return completion
}

// runToolSafely converts a concrete extension panic before it can jump across
// lifecycle middleware. That lets the outer durable wrapper record a bounded
// error completion for a tool that already has a start receipt.
func runToolSafely(tool Tool, ctx context.Context, arguments string, options ...ToolOption) (
	result ToolResult,
	err error,
) {
	defer func() {
		if value := recover(); value != nil {
			result = ToolResult{}
			err = recoveredPanic(value)
		}
	}()
	return tool.Run(ctx, arguments, options...)
}

func (agent *Agent) fillSteeringSkipped(
	calls []preparedToolCall,
	results []toolExecutionResult,
	events *AsyncGenerator[*AgentEvent],
) {
	for _, prepared := range calls {
		if prepared.precomputed != nil {
			results[prepared.index] = *prepared.precomputed
			agent.emitToolFinished(events, prepared, prepared.precomputed.result)
			continue
		}
		result := syntheticCallResult(prepared.call, ToolResultSkipped, ToolSyntheticSteeringBeforeStart,
			fmt.Sprintf("tool %q was not started because user steering is pending", prepared.call.Function.Name))
		if normalized, err := NormalizeToolResult(result.result, prepared.snapshot.Descriptor); err == nil {
			result.result = normalized
			result.message = ToolMessage(normalized, prepared.call.ID, WithToolName(prepared.call.Function.Name))
		}
		bindToolExecutionIdentity(&result, prepared)
		results[prepared.index] = result
		agent.emitToolFinished(events, prepared, result.result)
	}
}

func (agent *Agent) fillPolicySkipped(
	calls []preparedToolCall,
	results []toolExecutionResult,
	events *AsyncGenerator[*AgentEvent],
	reason string,
) {
	for _, prepared := range calls {
		if prepared.precomputed != nil {
			results[prepared.index] = *prepared.precomputed
			agent.emitToolFinished(events, prepared, prepared.precomputed.result)
			continue
		}
		result := syntheticCallResult(prepared.call, ToolResultSkipped, ToolSyntheticPolicyBlocked,
			fmt.Sprintf("tool %q was not started because %s", prepared.call.Function.Name, reason))
		if normalized, err := NormalizeToolResult(result.result, prepared.snapshot.Descriptor); err == nil {
			result.result = normalized
			result.message = ToolMessage(normalized, prepared.call.ID, WithToolName(prepared.call.Function.Name))
		}
		bindToolExecutionIdentity(&result, prepared)
		results[prepared.index] = result
		agent.emitToolFinished(events, prepared, result.result)
	}
}

func (agent *Agent) emitToolFinished(events *AsyncGenerator[*AgentEvent], prepared preparedToolCall, result ToolResult) {
	events.Send(agent.toolExecutionEvent(prepared, ToolExecutionFinished, "", &result))
}

func (agent *Agent) toolExecutionEvent(
	prepared preparedToolCall,
	phase ToolExecutionPhase,
	delta string,
	result *ToolResult,
) *AgentEvent {
	var cloned *ToolResult
	if result != nil {
		value := *result
		value.Details = append(json.RawMessage(nil), result.Details...)
		cloned = &value
	}
	return &AgentEvent{
		AgentName: agent.name,
		RunPath:   []RunStep{NewRunStep(agent.name)},
		Output: &AgentOutput{ToolExecution: &ToolExecutionEvent{
			Phase: phase, Index: prepared.index, ExecutionID: prepared.executionID,
			ProviderCallID: prepared.call.ID,
			ToolName:       prepared.call.Function.Name, Definition: prepared.snapshot,
			Delta: delta, Result: cloned,
		}},
	}
}

func syntheticCallResult(call ToolCall, status ToolResultStatus, reason ToolSyntheticReason, message string) toolExecutionResult {
	result := SyntheticToolResult(status, reason, toolErrorContent(call, errors.New(message)))
	return toolExecutionResult{
		result:  result,
		message: ToolMessage(result, call.ID, WithToolName(call.Function.Name)),
	}
}

func toolFailureResult(call ToolCall, err error) toolExecutionResult {
	result := ToolErrorResult(toolErrorContent(call, err), err.Error())
	var terminalErr error
	if IsToolControlError(err) || IsInterruptError(err) || errors.Is(err, context.Canceled) {
		terminalErr = err
	}
	return toolExecutionResult{
		result:  result,
		message: ToolMessage(result, call.ID, WithToolName(call.Function.Name)),
		err:     terminalErr,
	}
}

func toolFailureResultForDescriptor(call ToolCall, descriptor ToolDescriptor, err error) toolExecutionResult {
	result := toolFailureResult(call, err)
	normalizeToolExecutionResult(&result, descriptor)
	return result
}

func normalizeToolExecutionResult(result *toolExecutionResult, descriptor ToolDescriptor) {
	if result == nil {
		return
	}
	normalized, err := NormalizeToolResult(result.result, descriptor)
	if err != nil {
		fallback := ToolErrorResult(toolErrorContent(ToolCall{}, err), err.Error())
		normalized, _ = NormalizeToolResult(fallback, fallbackToolResultDescriptor)
	}
	result.result = normalized
	toolName := ""
	toolCallID := ""
	if result.message != nil {
		toolName = result.message.ToolName
		toolCallID = result.message.ToolCallID
	}
	result.message = ToolMessage(normalized, toolCallID, WithToolName(toolName))
}

func bindToolExecutionIdentity(result *toolExecutionResult, prepared preparedToolCall) {
	if result == nil {
		return
	}
	result.executionID = prepared.executionID
	result.providerCallID = prepared.call.ID
}

func toolErrorContent(call ToolCall, err error) string {
	message := "tool execution failed"
	if err != nil {
		message = err.Error()
	}
	payload, marshalErr := json.Marshal(map[string]any{
		"error": map[string]string{"message": message, "tool": call.Function.Name},
	})
	if marshalErr != nil {
		return fmt.Sprintf(`{"error":{"message":%q}}`, message)
	}
	return string(payload)
}

func (agent *Agent) lengthToolResults(ctx context.Context, calls []ToolCall, registry *Registry, modelResponseOrdinal int, events *AsyncGenerator[*AgentEvent]) []toolExecutionResult {
	results := make([]toolExecutionResult, len(calls))
	for index, call := range calls {
		prepared := preparedToolCall{index: index, call: call, executionID: ToolExecutionIDForOrdinal(ctx, modelResponseOrdinal, index)}
		result := syntheticCallResult(call, ToolResultSkipped, ToolSyntheticModelIncomplete,
			"tool call was not executed because the model response ended with finish_reason length; arguments may be incomplete")
		if snapshot, ok := registry.Snapshot(call.Function.Name); ok {
			prepared.snapshot = snapshot
			normalizeToolExecutionResult(&result, snapshot.Descriptor)
		} else {
			normalizeToolExecutionResult(&result, fallbackToolResultDescriptor)
		}
		bindToolExecutionIdentity(&result, prepared)
		results[index] = result
		agent.emitToolFinished(events, prepared, result.result)
	}
	return results
}
