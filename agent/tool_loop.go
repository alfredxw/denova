package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type toolExecutionResult struct {
	message *Message
	err     error
}

type indexedToolExecutionResult struct {
	index  int
	result toolExecutionResult
}

func (agent *Agent) executeToolBatch(
	ctx context.Context,
	registry *Registry,
	calls []ToolCall,
	events *AsyncGenerator[*AgentEvent],
) ([]toolExecutionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]toolExecutionResult, len(calls))
	completed := make(chan indexedToolExecutionResult, len(calls))
	for index := range calls {
		index := index
		call := cloneToolCalls([]ToolCall{calls[index]})[0]
		safeGo(func() {
			result := agent.executeTool(ctx, registry, call, events)
			if agent.emitToolCompletions {
				events.Send(agent.customEvent(&ToolCompletion{
					Index:    index,
					CallID:   call.ID,
					ToolName: call.Function.Name,
					Err:      result.err,
				}))
			}
			completed <- indexedToolExecutionResult{index: index, result: result}
		}, func(err error) {
			result := toolErrorResult(call, err)
			if agent.emitToolCompletions {
				events.Send(agent.customEvent(&ToolCompletion{
					Index:    index,
					CallID:   call.ID,
					ToolName: call.Function.Name,
					Err:      err,
				}))
			}
			completed <- indexedToolExecutionResult{index: index, result: result}
		})
	}
	var interruptErr error
	for range calls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		select {
		case completion := <-completed:
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if interruptErr == nil && IsInterruptError(completion.result.err) {
				// Every call in the batch is already running. Preserve all sibling
				// results before surfacing the interruption so a completed side
				// effect never loses its observable transcript receipt. The caller
				// may still cancel ctx when it wants to abandon the remaining work.
				interruptErr = completion.result.err
			}
			results[completion.index] = completion.result
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return results, interruptErr
}

func (agent *Agent) executeTool(
	ctx context.Context,
	registry *Registry,
	call ToolCall,
	events *AsyncGenerator[*AgentEvent],
) toolExecutionResult {
	name := call.Function.Name
	if call.Type != "" && call.Type != "function" {
		return toolErrorResult(call, fmt.Errorf("tool %q has unsupported call type %q", name, call.Type))
	}
	current, exists := registry.Lookup(name)
	if !exists {
		return toolErrorResult(call, fmt.Errorf("unknown tool %q", name))
	}
	callContext := ContextWithToolCall(ctx, call.ID, name)
	callContext = ContextWithEventSink(callContext, func(event *AgentEvent) {
		if event != nil {
			events.Send(event)
		}
	})
	info, _ := registry.Info(name)
	toolContext := &ToolContext{Name: name, CallID: call.ID, Info: info}

	if streamable, ok := current.(StreamableTool); ok {
		endpoint := streamable.StreamableRun
		for index := len(agent.middlewares) - 1; index >= 0; index-- {
			wrapped, err := agent.middlewares[index].WrapStreamableToolCall(callContext, endpoint, toolContext)
			if err != nil {
				return toolErrorResult(call, fmt.Errorf("wrap streamable tool %q: %w", name, err))
			}
			if wrapped == nil {
				return toolErrorResult(call, fmt.Errorf("wrap streamable tool %q returned nil endpoint", name))
			}
			endpoint = wrapped
		}
		if err := callContext.Err(); err != nil {
			return toolErrorResult(call, err)
		}
		stream, err := endpoint(callContext, call.Function.Arguments)
		if err != nil {
			return toolErrorResult(call, err)
		}
		if stream == nil {
			return toolErrorResult(call, fmt.Errorf("streamable tool %q returned nil reader", name))
		}
		defer func() {
			if callContext.Err() == nil {
				stream.Close()
				return
			}
			safeGo(stream.Close, func(error) {})
		}()
		var output strings.Builder
		for {
			fragment, err := awaitContextCall(callContext, stream.Recv, stream.Close, nil)
			if errors.Is(err, io.EOF) {
				return toolSuccessResult(call, output.String())
			}
			if err != nil {
				return toolErrorResult(call, err)
			}
			output.WriteString(fragment)
		}
	}

	invokable, ok := current.(InvokableTool)
	if !ok {
		return toolErrorResult(call, fmt.Errorf("tool %q is not executable", name))
	}
	endpoint := invokable.InvokableRun
	for index := len(agent.middlewares) - 1; index >= 0; index-- {
		wrapped, err := agent.middlewares[index].WrapInvokableToolCall(callContext, endpoint, toolContext)
		if err != nil {
			return toolErrorResult(call, fmt.Errorf("wrap tool %q: %w", name, err))
		}
		if wrapped == nil {
			return toolErrorResult(call, fmt.Errorf("wrap tool %q returned nil endpoint", name))
		}
		endpoint = wrapped
	}
	if err := callContext.Err(); err != nil {
		return toolErrorResult(call, err)
	}
	output, err := endpoint(callContext, call.Function.Arguments)
	if err != nil {
		return toolErrorResult(call, err)
	}
	return toolSuccessResult(call, output)
}

func toolSuccessResult(call ToolCall, output string) toolExecutionResult {
	return toolExecutionResult{message: ToolMessage(output, call.ID, WithToolName(call.Function.Name))}
}

func toolErrorResult(call ToolCall, err error) toolExecutionResult {
	payload, marshalErr := json.Marshal(map[string]any{
		"error": map[string]string{
			"message": err.Error(),
			"tool":    call.Function.Name,
		},
	})
	if marshalErr != nil {
		payload = []byte(fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()))
	}
	return toolExecutionResult{
		message: ToolMessage(string(payload), call.ID, WithToolName(call.Function.Name)),
		err:     err,
	}
}

func lengthToolResults(calls []ToolCall) []toolExecutionResult {
	results := make([]toolExecutionResult, len(calls))
	for index, call := range calls {
		results[index] = toolErrorResult(call, errors.New("tool call was not executed because the model response ended with finish_reason length; arguments may be incomplete"))
	}
	return results
}
