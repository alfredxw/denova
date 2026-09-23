package interactiveapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"denova/config"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/runtime/external"
	"denova/internal/agents/session"
	"denova/internal/agents/toolruntime"
	"denova/internal/i18n"
	agent "github.com/alfredxw/denova/agent"
	publicresult "github.com/alfredxw/denova/agent/toolresult"
)

type externalGameCall struct {
	call   external.ToolCall
	result external.ToolResult
	err    error
}

// Game tools are serialized against one product draft. A completed tool batch
// is stored before its observation is returned to the disposable engine.
func (turn *ExternalTurn) CallTool(ctx context.Context, call external.ToolCall) (result external.ToolResult, err error) {
	turn.mu.Lock()
	defer turn.mu.Unlock()
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("external Game tool panic: %v", value)
			turn.toolError = err
			turn.cancel()
		}
	}()
	if turn.closed {
		return result, errors.New("external Game turn is closed")
	}
	if err := turn.ctx.Err(); err != nil {
		return result, err
	}
	if call.ID == "" || len(call.ID) > 4096 || len(call.Arguments) > 4<<20 || !json.Valid(call.Arguments) {
		return external.ToolResult{Text: "Invalid host tool call."}, nil
	}
	if prior, ok := turn.calls[call.ID]; ok {
		if prior.call.Name != call.Name || !bytes.Equal(prior.call.Arguments, call.Arguments) {
			return result, errors.New("provider call ID was reused with different arguments")
		}
		return prior.result, prior.err
	}
	definition, ok := turn.tools[call.Name]
	if !ok {
		return external.ToolResult{Text: "Unknown host tool."}, nil
	}
	result, err = turn.invokeTool(ctx, call, definition)
	if err != nil {
		turn.toolError = err
		turn.cancel()
	}
	turn.calls[call.ID] = externalGameCall{call: call, result: result, err: err}
	return result, err
}

func (turn *ExternalTurn) invokeTool(ctx context.Context, call external.ToolCall, definition agent.ToolDefinition) (external.ToolResult, error) {
	c := turn.config.Conversation
	if call.Name == "submit_interactive_turn" && turn.segment != "" {
		if err := c.AcceptNarrativeCandidate(ctx, turn.segment); err != nil {
			return external.ToolResult{}, err
		}
	}
	if call.Name == "submit_interactive_turn" {
		narrative, err := c.LoadNarrativeCandidate(ctx)
		if err != nil {
			return external.ToolResult{}, err
		}
		if narrative == "" {
			// Reject before dispatch: no modules or successful submission card may
			// precede the prose they describe. The engine can repair in the same turn.
			slog.WarnContext(ctx, "[interactive-agent] rejected turn submission before narrative", "story_id", c.storyID, "operation_id", turn.identity.OperationID)
			return external.ToolResult{Text: "Output the complete player-visible narrative before calling submit_interactive_turn. No submission modules have been accepted. Write the prose now, then retry this tool with the state_changes and choices established by that prose."}, nil
		}
	}
	sum := sha256.Sum256([]byte(call.ID))
	executionID := fmt.Sprintf("%s-%d-%s", turn.identity.OperationID, c.modelContextBatchSequence, hex.EncodeToString(sum[:16]))
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(turn.ctx, cancel)
	defer stop()
	defer cancel()
	ctx = agent.ContextWithToolArtifactBackend(ctx, c.ToolArtifactStore())
	resultLimit := turn.config.Config.AgentToolResultLimitKB
	if resultLimit <= 0 {
		resultLimit = config.DefaultAgentToolResultLimitKB
	}
	policy := toolruntime.OrchestratorConfig{AgentKind: config.AgentKindInteractiveStory, PolicyKind: config.AgentKindInteractiveStory,
		Workspace: c.workspace, ToolSettings: turn.config.Assembly.ToolSettings, EnforceToolSettings: true,
		ToolResultMaxBytes: resultLimit * 1024}
	identity := toolruntime.HostToolIdentity{OperationID: string(turn.identity.OperationID), ExecutionID: executionID, ProviderCallID: call.ID, SessionID: c.storyID + ":" + c.branchID}
	if err := c.AppendDisplayEvent(session.DisplayEvent{ID: executionID, Role: "tool_call", Name: call.Name, Args: string(call.Arguments), RunID: identity.OperationID, AgentKind: config.AgentKindInteractiveStory, ToolPresentation: &definition.Descriptor.Presentation}); err != nil {
		return external.ToolResult{}, err
	}
	turn.send(agentrun.Event{Type: "tool_call", Data: map[string]any{"id": executionID, "name": call.Name, "args": string(call.Arguments), "tool_presentation": definition.Descriptor.Presentation}})
	var result agent.ToolResult
	var callErr error
	files := append([]agent.Attachment(nil), turn.config.Request.AttachedFiles...)
	for _, guidance := range turn.config.Guidance {
		files = append(files, guidance.AttachedFiles...)
	}
	if reason := external.HostPermissionError(*turn.config.Config.ActiveAgentRuntime, turn.config.Config.ProjectID, c.workspace, call, definition.Descriptor, files); reason != "" {
		result = agent.ToolErrorResult(reason, i18n.New(turn.config.Config.Language).T("agentRuntime.toolPermissionDenied", "tool", call.Name))
	} else {
		result, callErr = toolruntime.InvokeHostTool(ctx, policy, identity, definition, string(call.Arguments))
	}
	if callErr != nil && result.ModelContent == "" {
		return external.ToolResult{}, callErr
	}
	info, err := definition.Tool.Info(context.WithoutCancel(ctx))
	if err != nil {
		return external.ToolResult{}, err
	}
	processed, err := publicresult.Standard(publicresult.Policy{MaxBytes: policy.ToolResultMaxBytes}).Process(context.WithoutCancel(ctx), agent.ToolResultProcessRequest{
		ToolName: call.Name, Arguments: string(call.Arguments), ExecutionID: executionID, ProviderCallID: call.ID,
		Definition: agent.ToolDefinitionSnapshot{Info: info, Descriptor: definition.Descriptor}, Result: result})
	if err != nil {
		return external.ToolResult{}, err
	}
	processed, err = agent.NormalizeToolResult(processed, definition.Descriptor)
	if err != nil {
		return external.ToolResult{}, err
	}
	invocation := agent.AssistantMessage(turn.segment, []agent.ToolCall{{ID: executionID, Type: "function", Function: agent.FunctionCall{Name: call.Name, Arguments: string(call.Arguments)}}})
	observation := agent.ToolMessage(processed, executionID)
	observation.ToolName = call.Name
	if err := c.AppendContextMessages(invocation, observation); err != nil {
		return external.ToolResult{}, err
	}
	turn.observations = append(turn.observations, externalGameMessages([]*agent.Message{invocation, observation})...)
	turn.segment = ""
	if err := c.UpdateDisplayToolResult(executionID, call.Name, string(processed.Status), processed.DisplayContent, &definition.Descriptor.Presentation); err != nil {
		return external.ToolResult{}, err
	}
	turn.send(agentrun.Event{Type: "tool_result", Data: map[string]any{"id": executionID, "name": call.Name, "content": processed.DisplayContent, "status": string(processed.Status), "tool_presentation": definition.Descriptor.Presentation}})
	images, err := turn.projectToolImages(context.WithoutCancel(ctx), processed.Attachments)
	if err != nil {
		return external.ToolResult{}, err
	}
	if candidate, err := c.LoadNarrativeCandidate(context.WithoutCancel(ctx)); err != nil {
		return external.ToolResult{}, err
	} else if candidate != "" && c.InteractiveNarrativeReady() && len(turn.config.Guidance) == 0 {
		// Accepted native steering may still await the next model step. Let
		// that provider turn settle itself instead of interrupting its pending input.
		turn.cancel()
	}
	return external.ToolResult{Text: processed.ModelContent, Success: !processed.IsError(), Images: images}, callErr
}
