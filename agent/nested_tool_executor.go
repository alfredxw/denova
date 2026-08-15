package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// nestedToolDispatcher is scoped to one concrete parent tool execution. Its
// sequence spans every call/parallel invocation made by that parent so sibling
// display order remains stable even when completion order differs.
type nestedToolDispatcher struct {
	loop         *modelToolLoop
	registry     *Registry
	events       *asyncGenerator[*loopEvent]
	cancel       *cancelControl
	parentCallID string
	sequence     uint64
}

func (dispatcher *nestedToolDispatcher) call(
	ctx context.Context,
	calls []NestedToolCall,
) ([]NestedToolOutcome, error) {
	if dispatcher == nil || dispatcher.loop == nil || dispatcher.registry == nil || dispatcher.events == nil {
		return nil, errors.New("nested tool dispatcher is unavailable")
	}
	parentCallID := strings.TrimSpace(dispatcher.parentCallID)
	if parentCallID == "" {
		return nil, errors.New("nested tool dispatcher has no parent call identity")
	}
	prepared := make([]preparedToolCall, len(calls))
	for batchIndex, nested := range calls {
		dispatcher.sequence++
		sequence := dispatcher.sequence
		call := ToolCall{
			Type:     "function",
			Function: FunctionCall{Name: strings.TrimSpace(nested.Name), Arguments: string(nested.Arguments)},
		}
		prepared[batchIndex] = prepareToolCall(dispatcher.registry, call, batchIndex,
			hashedExecutionID("tool", parentCallID, fmt.Sprintf("nested:%d", sequence)), parentCallID)
	}
	results, err := dispatcher.loop.executePreparedToolBatch(
		ctx, prepared, dispatcher.events, dispatcher.cancel,
	)
	if err != nil {
		return nil, err
	}
	outcomes := make([]NestedToolOutcome, len(results))
	for index := range results {
		outcomes[index] = nestedOutcome(prepared[index], results[index].result)
	}
	return outcomes, nil
}

func nestedOutcome(prepared preparedToolCall, result ToolResult) NestedToolOutcome {
	output, _ := json.Marshal(result.ModelContent)
	if result.Status == ToolResultSuccess && !result.Metadata.ModelTruncated &&
		json.Valid([]byte(result.ModelContent)) {
		output = append(json.RawMessage(nil), result.ModelContent...)
	}
	artifacts := make([]ToolArtifactRef, len(result.Artifacts))
	copy(artifacts, result.Artifacts)
	return NestedToolOutcome{
		Name: prepared.call.Function.Name, Status: result.Status, Reason: result.SyntheticReason,
		Output: output, Truncated: result.Metadata.ModelTruncated, Artifacts: artifacts,
	}
}
