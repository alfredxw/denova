package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

type taskWaitTarget struct {
	invalid error
	Ref     TaskRef `json:"ref,omitempty"`
}
type taskWaitInput struct {
	Targets   []taskWaitTarget `json:"targets" jsonschema:"minItems=1,maxItems=32"`
	Until     string           `json:"until,omitempty" jsonschema:"enum=any,enum=all" jsonschema_description:"Default any. Suspended work or unresolved interaction returns attention immediately even with all."`
	TimeoutMS *int             `json:"timeout_ms,omitempty" jsonschema:"minimum=0,maximum=60000" jsonschema_description:"Default 30000. Zero reads current status and bounded output without waiting. Positive waits return status only; results arrive separately."`
}
type agentRunSnapshot struct {
	Ref    TaskRef `json:"ref"`
	Status string  `json:"status"`
	Reason string  `json:"reason,omitempty"`
}
type boundedAgentOutput struct {
	Text       string `json:"text"`
	Incomplete bool   `json:"incomplete"`
}
type awaitResult struct {
	Index   int                 `json:"index"`
	Outcome string              `json:"outcome"`
	Run     *agentRunSnapshot   `json:"run,omitempty"`
	Ready   *bool               `json:"ready,omitempty"`
	Output  *boundedAgentOutput `json:"output,omitempty"`
	Error   *agentToolError     `json:"error,omitempty"`
}
type awaitReport struct {
	Reason  string        `json:"reason"`
	Results []awaitResult `json:"results"`
}

func newTaskWaitDefinition(executor TaskExecutor) (agent.ToolDefinition, error) {
	schema, err := batchToolSchema[taskWaitInput]("targets")
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	tool, err := newSchemaTool("await", "Wait for any or all existing child Runs at a real dependency. Readiness means completion, suspension, or an interaction boundary, not success. Use timeout_ms=0 to inspect bounded output. Positive waits return status only; completed results arrive through the existing mailbox. User steering interrupts waiting without aborting children.",
		schema,
		func(ctx context.Context, input taskWaitInput) (agent.ToolResult, error) {
			return awaitAgents(ctx, executor, input)
		})
	if err != nil {
		return agent.ToolDefinition{}, err
	}
	return agent.ToolDefinition{Tool: tool, Descriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceOther, Capability: "delegation", Execution: agent.ToolExecutionInteractiveWait,
		MutationScope: agent.ToolMutationNone, PostCheck: agent.ToolPostCheckNone, Recovery: agent.ToolRecoveryReadOnly,
		ResultProjection: agent.ToolResultBoundedModelContext, ResultRetention: agent.ToolResultProtected,
		Steering: agent.SteeringInterruptibleWait, MaxResultBytes: defaultResultBytes,
		Presentation: agent.UniformToolPresentation(agent.ToolPresentationDelegation),
	}}, nil
}

func awaitAgents(ctx context.Context, executor TaskExecutor, input taskWaitInput) (agent.ToolResult, error) {
	timeout := 30000
	if input.TimeoutMS != nil {
		timeout = *input.TimeoutMS
	}
	if len(input.Targets) < 1 || len(input.Targets) > 32 || timeout < 0 || timeout > 60000 || input.Until != "" && input.Until != "any" && input.Until != "all" {
		return agentFailure(fmt.Errorf("%w: await requires 1..32 targets, any/all, and timeout_ms in 0..60000", ErrTaskInvalidInput))
	}
	report := awaitReport{Reason: "error", Results: make([]awaitResult, len(input.Targets))}
	var pending []TaskRef
	var indices []int
	for index, target := range input.Targets {
		report.Results[index] = awaitResult{Index: index, Outcome: "error"}
		if target.invalid != nil {
			report.Results[index].Error = agentError(fmt.Errorf("%w: %v", ErrTaskInvalidInput, target.invalid))
			continue
		}
		if err := validateTaskRef(target.Ref); err != nil {
			report.Results[index].Error = agentError(err)
			continue
		}
		pending, indices = append(pending, target.Ref), append(indices, index)
	}
	if len(pending) == 0 {
		return awaitReportResult(report, executor)
	}
	if timeout == 0 {
		for offset, ref := range pending {
			observation, err := executor.Observe(ctx, ref, "")
			index := indices[offset]
			if err != nil {
				report.Results[index].Error = agentError(err)
				continue
			}
			ready := isTaskTerminal(observation.Task.Status) || observation.Task.Status == "suspended" || observation.Task.Status == "waiting_input"
			report.Results[index] = awaitObserved(index, observation.Task, ready)
			text, truncated := boundAgentText(observation.Output, defaultResultBytes/len(pending))
			report.Results[index].Output = &boundedAgentOutput{Text: text, Incomplete: observation.Incomplete || truncated}
		}
		report.Reason = awaitReason(report.Results, input.Until)
		return awaitReportResult(report, executor)
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	for len(pending) > 0 {
		outcomes, err := executor.Wait(waitCtx, pending)
		if len(outcomes) != len(pending) && err == nil {
			err = errors.New("executor returned an incomplete wait result")
		}
		nextRefs, nextIndices := []TaskRef{}, []int{}
		for offset, ref := range pending {
			index := indices[offset]
			if offset >= len(outcomes) {
				report.Results[index] = awaitResult{Index: index, Outcome: "error", Error: agentError(err)}
				continue
			}
			outcome := outcomes[offset]
			if outcome.Err != nil {
				report.Results[index] = awaitResult{Index: index, Outcome: "error", Error: agentError(outcome.Err)}
				continue
			}
			if outcome.Task == nil {
				report.Results[index] = awaitResult{Index: index, Outcome: "error", Error: agentError(errors.New("executor returned no Run snapshot"))}
				continue
			}
			report.Results[index] = awaitObserved(index, *outcome.Task, outcome.Ready)
			if !outcome.Ready {
				nextRefs, nextIndices = append(nextRefs, ref), append(nextIndices, index)
			}
		}
		report.Reason = awaitReason(report.Results, input.Until)
		if err != nil {
			switch {
			case ctx.Err() != nil:
				report.Reason = "interrupted"
			case errors.Is(err, context.DeadlineExceeded):
				report.Reason = "timeout"
			default:
				report.Reason = "error"
			}
			break
		}
		if report.Reason != "timeout" {
			break
		}
		pending, indices = nextRefs, nextIndices
	}
	return awaitReportResult(report, executor)
}

func awaitObserved(index int, task Task, ready bool) awaitResult {
	return awaitResult{Index: index, Outcome: "observed", Run: &agentRunSnapshot{Ref: task.Ref, Status: task.Status, Reason: task.Reason}, Ready: &ready}
}

// Explicit observations must remain valid JSON within the configured model
// budget. Bound the text, including JSON escaping, before generic projection.
func awaitReportResult(report awaitReport, executor TaskExecutor) (agent.ToolResult, error) {
	limit := agentResultLimit(executor)
	for {
		encoded, err := json.Marshal(report)
		if err != nil {
			return agent.ToolResult{}, err
		}
		if len(encoded) <= limit {
			return JSONResult(report)
		}
		changed := false
		for index := range report.Results {
			output := report.Results[index].Output
			if output != nil && output.Text != "" {
				output.Text, _ = boundAgentText(output.Text, len(output.Text)/2)
				output.Incomplete, changed = true, true
			}
		}
		if !changed {
			return agentFailure(ErrAgentResultTooLarge)
		}
	}
}
func awaitReason(results []awaitResult, until string) string {
	valid, ready := 0, 0
	for _, result := range results {
		if result.Run == nil {
			continue
		}
		valid++
		if result.Run.Status == "suspended" || result.Run.Status == "waiting_input" {
			return "attention"
		}
		if result.Ready != nil && *result.Ready {
			ready++
		}
	}
	if valid == 0 {
		return "error"
	}
	if ready > 0 && (until != "all" || ready == valid) {
		return "ready"
	}
	return "timeout"
}
func boundAgentText(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit], true
}

func (target *taskWaitTarget) UnmarshalJSON(data []byte) error {
	type plain taskWaitTarget
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	target.invalid = decoder.Decode((*plain)(target))
	return nil
}
